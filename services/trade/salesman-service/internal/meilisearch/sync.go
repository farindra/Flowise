package meilisearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type Syncer struct {
	meiliURL string
	meiliKey string
	http     *http.Client
}

func NewSyncer(meiliURL, meiliKey string) *Syncer {
	return &Syncer{
		meiliURL: meiliURL,
		meiliKey: meiliKey,
		http:     &http.Client{Timeout: 30 * time.Second},
	}
}

type CustomerDoc struct {
	ID           int    `json:"id"`
	DisplayName  string `json:"display_name"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	Address      string `json:"address"`
	CustomerType string `json:"customer_type"`
	NPWP         string `json:"npwp"`
	SyncedAt     string `json:"synced_at"`
}

// SyncCustomers fetches all pages from Jurnal and indexes into Meilisearch.
func (s *Syncer) SyncCustomers(ctx context.Context, jurnalURL, jurnalToken string) error {
	log.Println("[customer-sync] starting...")

	// Ensure index exists with correct settings
	if err := s.ensureIndex(ctx); err != nil {
		log.Printf("[customer-sync] ensureIndex warning: %v", err)
	}

	jClient := &http.Client{Timeout: 30 * time.Second}
	page := 1
	total := 0

	for {
		u := fmt.Sprintf("%scustomers?page=%d&per_page=100", jurnalURL, page)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		req.Header.Set("Authorization", "Bearer "+jurnalToken)

		resp, err := jClient.Do(req)
		if err != nil {
			return fmt.Errorf("fetch page %d: %w", page, err)
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var result map[string]any
		if err := json.Unmarshal(raw, &result); err != nil {
			return fmt.Errorf("parse page %d: %w", page, err)
		}

		batch, _ := result["customers"].([]any)
		if len(batch) == 0 {
			break
		}

		docs := make([]CustomerDoc, 0, len(batch))
		now := time.Now().Format("2006-01-02 15:04:05")
		for _, item := range batch {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["id"].(float64)
			name := strVal(m, "display_name")
			if name == "" {
				name = strVal(m, "name")
			}
			docs = append(docs, CustomerDoc{
				ID:           int(id),
				DisplayName:  name,
				Name:         strVal(m, "name"),
				Email:        strVal(m, "email"),
				Phone:        strVal(m, "phone"),
				Address:      strVal(m, "address"),
				CustomerType: strVal(m, "customer_type"),
				NPWP:         strVal(m, "tax_no"),
				SyncedAt:     now,
			})
		}

		if err := s.upsertDocs(ctx, docs); err != nil {
			return fmt.Errorf("upsert page %d: %w", page, err)
		}

		total += len(docs)
		log.Printf("[customer-sync] page %d: %d docs (total %d)", page, len(docs), total)

		if len(batch) < 100 {
			break
		}
		page++
		time.Sleep(300 * time.Millisecond)
	}

	log.Printf("[customer-sync] done: %d customers synced", total)
	return nil
}

func (s *Syncer) ensureIndex(ctx context.Context) error {
	// Create index (ok if already exists)
	s.meiliPost(ctx, "/indexes", map[string]any{"uid": "customers", "primaryKey": "id"})

	// Searchable attributes
	s.meiliPut(ctx, "/indexes/customers/settings/searchable-attributes",
		[]string{"display_name", "name", "email", "phone", "address", "npwp"})

	// Filterable
	s.meiliPut(ctx, "/indexes/customers/settings/filterable-attributes",
		[]string{"customer_type"})

	return nil
}

func (s *Syncer) upsertDocs(ctx context.Context, docs []CustomerDoc) error {
	_, err := s.meiliPost(ctx, "/indexes/customers/documents", docs)
	return err
}

func (s *Syncer) meiliPost(ctx context.Context, path string, body any) (map[string]any, error) {
	return s.meiliReq(ctx, http.MethodPost, path, body)
}

func (s *Syncer) meiliPut(ctx context.Context, path string, body any) (map[string]any, error) {
	return s.meiliReq(ctx, http.MethodPut, path, body)
}

func (s *Syncer) meiliReq(ctx context.Context, method, path string, body any) (map[string]any, error) {
	b, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, method, s.meiliURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.meiliKey)
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out map[string]any
	json.Unmarshal(raw, &out)
	return out, nil
}

// SearchCustomers searches the Meilisearch customers index.
func (s *Syncer) SearchCustomers(ctx context.Context, q string, limit int) ([]map[string]any, error) {
	payload := map[string]any{"q": q, "limit": limit}
	result, err := s.meiliPost(ctx, "/indexes/customers/search", payload)
	if err != nil {
		return nil, err
	}
	hits, _ := result["hits"].([]any)
	out := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		if m, ok := h.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out, nil
}

func strVal(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
