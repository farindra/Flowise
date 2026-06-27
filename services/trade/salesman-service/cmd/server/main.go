package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"salesman-service/internal/jurnal"
	"salesman-service/internal/logviewer"
	"salesman-service/internal/meilisearch"
	"salesman-service/internal/product"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	// ── Config ────────────────────────────────────────────────────────────────
	port             := envOr("PORT", "8200")
	internalKey      := envOr("INTERNAL_KEY", "ob-jurnal-internal-2026")
	jurnalURL        := envOr("JURNAL_API_URL", "https://api.jurnal.id/partner/core/api/v1/")
	jurnalToken      := os.Getenv("JURNAL_BEARER_TOKEN")
	meiliURL         := envOr("MEILI_URL", "http://meilisearch:7700")
	meiliKey         := os.Getenv("MEILI_MASTER_KEY")
	productSearchURL := envOr("PRODUCT_SEARCH_URL", "http://product-search-service:8101")
	syncIntervalStr  := envOr("CUSTOMER_SYNC_INTERVAL_HOURS", "6")
	syncInterval, _  := strconv.Atoi(syncIntervalStr)
	if syncInterval <= 0 {
		syncInterval = 6
	}
	errorLogPath     := envOr("ERROR_LOG_PATH", "/data/error.log")
	flowiseLogsDir   := envOr("FLOWISE_LOGS_DIR", "/flowise-logs")

	// ── Init error logger ─────────────────────────────────────────────────────
	if err := logviewer.Init(errorLogPath); err != nil {
		log.Printf("[warn] cannot open error log %s: %v", errorLogPath, err)
	}

	// ── Services ──────────────────────────────────────────────────────────────
	meiliSyncer  := meilisearch.NewSyncer(meiliURL, meiliKey)
	jurnalClient := jurnal.NewClient(jurnalURL, jurnalToken)
	jurnalH      := jurnal.NewHandler(jurnalClient, meiliSyncer)
	productH     := product.NewHandler(productSearchURL)
	logH         := logviewer.NewHandler(errorLogPath, flowiseLogsDir)

	// ── Initial customer sync ──────────────────────────────────────────────────
	go func() {
		if err := meiliSyncer.SyncCustomers(context.Background(), jurnalURL, jurnalToken); err != nil {
			log.Printf("[startup] customer sync error: %v", err)
		}
	}()

	// ── Periodic customer sync ─────────────────────────────────────────────────
	go func() {
		ticker := time.NewTicker(time.Duration(syncInterval) * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if err := meiliSyncer.SyncCustomers(context.Background(), jurnalURL, jurnalToken); err != nil {
				log.Printf("[sync] customer sync error: %v", err)
			}
		}
	}()

	// ── Auth middleware ───────────────────────────────────────────────────────
	auth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			key := r.Header.Get("X-Internal-Key")
			if key == "" {
				// Also accept Bearer
				auth := r.Header.Get("Authorization")
				key = strings.TrimPrefix(auth, "Bearer ")
			}
			if key != internalKey {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				w.Write([]byte(`{"error":"unauthorized"}`))
				return
			}
			next(w, r)
		}
	}

	// ── Routes ────────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// Jurnal tools (authenticated)
	mux.HandleFunc("GET /customers", auth(jurnalH.HandleCustomers))
	mux.HandleFunc("POST /quotation", auth(jurnalH.HandleQuotation))
	mux.HandleFunc("POST /sales-order", auth(jurnalH.HandleSalesOrder))

	// Product search (authenticated)
	mux.HandleFunc("GET /products", auth(productH.HandleProducts))

	// Manual trigger: re-sync customers
	mux.HandleFunc("POST /sync-customers", auth(func(w http.ResponseWriter, r *http.Request) {
		go meiliSyncer.SyncCustomers(context.Background(), jurnalURL, jurnalToken)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"sync started"}`))
	}))

	// Log viewer UI (auth via internal key)
	mux.HandleFunc("GET /logs", auth(logH.HandleUI))
	mux.HandleFunc("GET /logs/search", auth(logH.HandleSearch))

	// Health
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"status":"ok"}`))
	})

	addr := ":" + port
	log.Printf("[salesman-service] listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}
