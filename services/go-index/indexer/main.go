package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/meilisearch/meilisearch-go"
)

// ── Config ────────────────────────────────────────────────────────────────────

type StoreConfig struct {
	Name   string // e.g. "prime_ob"
	DSN    string
	Prefix string
}

var (
	meiliURL      = envOr("MEILI_URL", "http://meilisearch:7700")
	meiliKey      = mustEnv("MEILI_MASTER_KEY")
	reindexEvery  = parseMinutes(envOr("REINDEX_INTERVAL", "10"))
	stores        []StoreConfig
)

func init() {
	s1 := StoreConfig{
		Name:   mustEnv("DB_PRIME_OB_STORE"),
		DSN:    mustEnv("DB_PRIME_OB_DSN"),
		Prefix: envOr("DB_PRIME_OB_PREFIX", "ps_"),
	}
	s2 := StoreConfig{
		Name:   mustEnv("DB_PRIME_SBB_STORE"),
		DSN:    mustEnv("DB_PRIME_SBB_DSN"),
		Prefix: envOr("DB_PRIME_SBB_PREFIX", "ps_"),
	}
	stores = []StoreConfig{s1, s2}
}

func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("missing env: %s", k)
	}
	return v
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func parseMinutes(s string) time.Duration {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 10 * time.Minute
	}
	return time.Duration(n) * time.Minute
}

// ── Document types ────────────────────────────────────────────────────────────

type ProductDoc struct {
	ID          string  `json:"id"`
	IDProduct   int64   `json:"id_product"`
	Reference   string  `json:"reference"`
	EAN13       string  `json:"ean13"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	LinkRewrite string  `json:"link_rewrite"`
	Price       float64 `json:"price"`
	Stock       int64   `json:"stock"`
	Categories  string  `json:"categories"`
	Active      bool    `json:"active"`
	Store       string  `json:"store"`
}

type OrderDoc struct {
	ID           string  `json:"id"`
	IDOrder      int64   `json:"id_order"`
	Reference    string  `json:"reference"`
	IDCustomer   int64   `json:"id_customer"`
	CustomerName string  `json:"customer_name"`
	Email        string  `json:"email"`
	TotalPaid    float64 `json:"total_paid"`
	Status       string  `json:"status"`
	DateAdd      string  `json:"date_add"`
	Items        string  `json:"items"`
	Store        string  `json:"store"`
}

type PromotionDoc struct {
	ID                string  `json:"id"`
	IDCartRule        int64   `json:"id_cart_rule"`
	Code              string  `json:"code"`
	Name              string  `json:"name"`
	Description       string  `json:"description"`
	ReductionPercent  float64 `json:"reduction_percent"`
	ReductionAmount   float64 `json:"reduction_amount"`
	DateFrom          string  `json:"date_from"`
	DateTo            string  `json:"date_to"`
	Active            bool    `json:"active"`
	Quantity          int64   `json:"quantity"`
	Store             string  `json:"store"`
}

type CartDoc struct {
	ID         string  `json:"id"`
	IDCart     int64   `json:"id_cart"`
	IDCustomer int64   `json:"id_customer"`
	Email      string  `json:"email"`
	TotalCart  float64 `json:"total_cart"`
	Items      string  `json:"items"`
	DateAdd    string  `json:"date_add"`
	Store      string  `json:"store"`
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	meili := meilisearch.New(meiliURL, meilisearch.WithAPIKey(meiliKey))

	// HTTP server untuk manual trigger reindex
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "OK")
	})
	mux.HandleFunc("/reindex", func(w http.ResponseWriter, r *http.Request) {
		go runAllStores(meili)
		fmt.Fprint(w, `{"status":"reindex started"}`)
	})
	go func() {
		log.Println("indexer HTTP listening on :8082")
		if err := http.ListenAndServe(":8082", mux); err != nil { log.Printf("indexer HTTP stopped: %v", err) }
	}()

	// Run indexing pertama kali
	runAllStores(meili)

	// Periodic re-index
	for range time.Tick(reindexEvery) {
		runAllStores(meili)
	}
}

func runAllStores(meili meilisearch.ServiceManager) {
	log.Println("=== starting full reindex ===")
	for _, store := range stores {
		db, err := sql.Open("mysql", store.DSN)
		if err != nil {
			log.Printf("[%s] failed to open db: %v", store.Name, err)
			continue
		}
		db.SetMaxOpenConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)

		indexProducts(meili, db, store)
		indexOrders(meili, db, store)
		indexPromotions(meili, db, store)
		indexCarts(meili, db, store)

		db.Close()
		log.Printf("[%s] done", store.Name)
	}
	log.Println("=== reindex complete ===")
}

// ── Products ──────────────────────────────────────────────────────────────────

func indexProducts(meili meilisearch.ServiceManager, db *sql.DB, s StoreConfig) {
	p := s.Prefix
	q := fmt.Sprintf(`
		SELECT
			p.id_product,
			IFNULL(p.reference,''),
			IFNULL(p.ean13,''),
			IFNULL(pl.name,''),
			IFNULL(pl.description_short,''),
			IFNULL(pl.link_rewrite,''),
			IFNULL(p.price,0),
			COALESCE(sa.quantity,0),
			IFNULL(GROUP_CONCAT(DISTINCT cl.name ORDER BY cl.name SEPARATOR ', '),''),
			p.active
		FROM %sproduct p
		JOIN %sproduct_lang pl ON p.id_product = pl.id_product AND pl.id_lang = 1
		LEFT JOIN %sstock_available sa ON p.id_product = sa.id_product AND sa.id_product_attribute = 0
		LEFT JOIN %scategory_product cp ON p.id_product = cp.id_product
		LEFT JOIN %scategory_lang cl ON cp.id_category = cl.id_category AND cl.id_lang = 1
		GROUP BY p.id_product, pl.name, pl.description_short, pl.link_rewrite, p.price, sa.quantity, p.active, p.reference, p.ean13
	`, p, p, p, p, p)

	rows, err := db.Query(q)
	if err != nil {
		log.Printf("[%s] products query error: %v", s.Name, err)
		return
	}
	defer rows.Close()

	var docs []interface{}
	for rows.Next() {
		var d ProductDoc
		var activeInt int
		if err := rows.Scan(&d.IDProduct, &d.Reference, &d.EAN13, &d.Name, &d.Description,
			&d.LinkRewrite, &d.Price, &d.Stock, &d.Categories, &activeInt); err != nil {
			log.Printf("[%s] products scan error: %v", s.Name, err)
			continue
		}
		d.Active = activeInt == 1
		d.Store = s.Name
		d.ID = fmt.Sprintf("%s_%d", s.Name, d.IDProduct)
		d.Description = stripHTML(d.Description)
		docs = append(docs, d)
	}

	pushToMeili(meili, s.Name+"_products", "id", docs)
	log.Printf("[%s] indexed %d products", s.Name, len(docs))
}

// ── Orders ────────────────────────────────────────────────────────────────────

func indexOrders(meili meilisearch.ServiceManager, db *sql.DB, s StoreConfig) {
	p := s.Prefix
	q := fmt.Sprintf(`
		SELECT
			o.id_order,
			IFNULL(o.reference,''),
			o.id_customer,
			IFNULL(CONCAT(IFNULL(a.firstname,''), ' ', IFNULL(a.lastname,'')), ''),
			IFNULL(c.email,''),
			IFNULL(o.total_paid,0),
			IFNULL(osl.name,''),
			DATE_FORMAT(o.date_add, '%%Y-%%m-%%d %%H:%%i:%%s'),
			IFNULL(GROUP_CONCAT(DISTINCT od.product_name ORDER BY od.id_order_detail SEPARATOR ' | '),'')
		FROM %sorders o
		LEFT JOIN %scustomer c ON o.id_customer = c.id_customer
		LEFT JOIN %saddress a ON o.id_address_delivery = a.id_address
		LEFT JOIN %sorder_state_lang osl ON o.current_state = osl.id_order_state AND osl.id_lang = 1
		LEFT JOIN %sorder_detail od ON o.id_order = od.id_order
		GROUP BY o.id_order, o.reference, o.id_customer, a.firstname, a.lastname, c.email,
		         o.total_paid, osl.name, o.date_add
		ORDER BY o.date_add DESC
		LIMIT 5000
	`, p, p, p, p, p)

	rows, err := db.Query(q)
	if err != nil {
		log.Printf("[%s] orders query error: %v", s.Name, err)
		return
	}
	defer rows.Close()

	var docs []interface{}
	for rows.Next() {
		var d OrderDoc
		if err := rows.Scan(&d.IDOrder, &d.Reference, &d.IDCustomer, &d.CustomerName,
			&d.Email, &d.TotalPaid, &d.Status, &d.DateAdd, &d.Items); err != nil {
			log.Printf("[%s] orders scan error: %v", s.Name, err)
			continue
		}
		d.Store = s.Name
		d.CustomerName = strings.TrimSpace(d.CustomerName)
		d.ID = fmt.Sprintf("%s_%d", s.Name, d.IDOrder)
		docs = append(docs, d)
	}

	pushToMeili(meili, s.Name+"_orders", "id", docs)
	log.Printf("[%s] indexed %d orders", s.Name, len(docs))
}

// ── Promotions ────────────────────────────────────────────────────────────────

func indexPromotions(meili meilisearch.ServiceManager, db *sql.DB, s StoreConfig) {
	p := s.Prefix
	q := fmt.Sprintf(`
		SELECT
			cr.id_cart_rule,
			IFNULL(cr.code,''),
			IFNULL(crl.name,''),
			IFNULL(cr.reduction_percent,0),
			IFNULL(cr.reduction_amount,0),
			IFNULL(DATE_FORMAT(cr.date_from,'%%Y-%%m-%%d'),''),
			IFNULL(DATE_FORMAT(cr.date_to,'%%Y-%%m-%%d'),''),
			cr.active,
			IFNULL(cr.quantity,0)
		FROM %scart_rule cr
		LEFT JOIN %scart_rule_lang crl ON cr.id_cart_rule = crl.id_cart_rule AND crl.id_lang = 1
	`, p, p)

	rows, err := db.Query(q)
	if err != nil {
		log.Printf("[%s] promotions query error: %v", s.Name, err)
		return
	}
	defer rows.Close()

	var docs []interface{}
	for rows.Next() {
		var d PromotionDoc
		var activeInt int
		if err := rows.Scan(&d.IDCartRule, &d.Code, &d.Name,
			&d.ReductionPercent, &d.ReductionAmount, &d.DateFrom, &d.DateTo,
			&activeInt, &d.Quantity); err != nil {
			log.Printf("[%s] promotions scan error: %v", s.Name, err)
			continue
		}
		d.Active = activeInt == 1
		d.Store = s.Name
		d.ID = fmt.Sprintf("%s_%d", s.Name, d.IDCartRule)
		docs = append(docs, d)
	}

	pushToMeili(meili, s.Name+"_promotions", "id", docs)
	log.Printf("[%s] indexed %d promotions", s.Name, len(docs))
}

// ── Carts ─────────────────────────────────────────────────────────────────────

func indexCarts(meili meilisearch.ServiceManager, db *sql.DB, s StoreConfig) {
	p := s.Prefix
	q := fmt.Sprintf(`
		SELECT
			c.id_cart,
			c.id_customer,
			IFNULL(cu.email,''),
			IFNULL(GROUP_CONCAT(DISTINCT CONCAT(pl.name,' (x',cp.quantity,')') ORDER BY cp.id_product SEPARATOR ' | '),''),
			DATE_FORMAT(c.date_add,'%%Y-%%m-%%d %%H:%%i:%%s')
		FROM %scart c
		LEFT JOIN %scustomer cu ON c.id_customer = cu.id_customer
		LEFT JOIN %scart_product cp ON c.id_cart = cp.id_cart
		LEFT JOIN %sproduct_lang pl ON cp.id_product = pl.id_product AND pl.id_lang = 1
		WHERE c.id_customer > 0
		GROUP BY c.id_cart, c.id_customer, cu.email, c.date_add
		ORDER BY c.date_add DESC
		LIMIT 2000
	`, p, p, p, p)

	rows, err := db.Query(q)
	if err != nil {
		log.Printf("[%s] carts query error: %v", s.Name, err)
		return
	}
	defer rows.Close()

	var docs []interface{}
	for rows.Next() {
		var d CartDoc
		if err := rows.Scan(&d.IDCart, &d.IDCustomer, &d.Email,
			&d.Items, &d.DateAdd); err != nil {
			log.Printf("[%s] carts scan error: %v", s.Name, err)
			continue
		}
		d.Store = s.Name
		d.ID = fmt.Sprintf("%s_%d", s.Name, d.IDCart)
		docs = append(docs, d)
	}

	pushToMeili(meili, s.Name+"_carts", "id", docs)
	log.Printf("[%s] indexed %d carts", s.Name, len(docs))
}

// ── Meilisearch helper ────────────────────────────────────────────────────────

// filterableFields per index — field yang bisa dipakai di filter query Flowise
var filterableFields = map[string][]interface{}{
	"products":   {"active", "categories", "reference", "stock"},
	"orders":     {"id_order", "id_customer", "status"},
	"promotions": {"active", "code"},
	"carts":      {"id_customer"},
}

func pushToMeili(meili meilisearch.ServiceManager, indexName, primaryKey string, docs []interface{}) {
	if len(docs) == 0 {
		return
	}
	idx := meili.Index(indexName)
	meili.CreateIndex(&meilisearch.IndexConfig{Uid: indexName, PrimaryKey: primaryKey}) //nolint

	// Set filterable attributes berdasarkan tipe index (ambil suffix setelah store prefix)
	for suffix, fields := range filterableFields {
		if strings.HasSuffix(indexName, "_"+suffix) {
			idx.UpdateFilterableAttributes(&fields) //nolint
			break
		}
	}

	pk := primaryKey
	opts := &meilisearch.DocumentOptions{PrimaryKey: &pk}
	const batchSize = 500
	for i := 0; i < len(docs); i += batchSize {
		end := i + batchSize
		if end > len(docs) {
			end = len(docs)
		}
		if _, err := idx.AddDocuments(docs[i:end], opts); err != nil {
			log.Printf("[meili] addDocuments error on %s: %v", indexName, err)
		}
	}
}

// ── Util ──────────────────────────────────────────────────────────────────────

func stripHTML(s string) string {
	// Simple HTML tag stripper
	out := strings.Builder{}
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			out.WriteRune(' ')
			continue
		}
		if !inTag {
			out.WriteRune(r)
		}
	}
	result := strings.Join(strings.Fields(out.String()), " ")
	if len(result) > 500 {
		return result[:500]
	}
	return result
}

// jsonStr is a debug helper — keeps the binary small in prod by using it only in logs
func jsonStr(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
