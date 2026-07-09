package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// ── syncStatus tracks the last sync result ────────────────────────────────────

type syncCounts struct {
	Products  int
	Customers int
}

type syncStatus struct {
	mu          sync.RWMutex
	running     bool
	startedAt   time.Time
	current     syncCounts // live progress during active sync
	lastRun     time.Time
	lastDur     time.Duration
	lastErrs    []string
	lastCounts  syncCounts
	totalRuns   int
}

func (s *syncStatus) setRunning(v bool) {
	s.mu.Lock()
	if v {
		s.startedAt = time.Now()
		s.current = syncCounts{}
	}
	s.running = v
	s.mu.Unlock()
}

func (s *syncStatus) addProducts(n int) {
	s.mu.Lock()
	s.current.Products += n
	s.mu.Unlock()
}

func (s *syncStatus) addCustomers(n int) {
	s.mu.Lock()
	s.current.Customers += n
	s.mu.Unlock()
}

func (s *syncStatus) isRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

func (s *syncStatus) record(dur time.Duration, counts syncCounts, errs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastRun = time.Now()
	s.lastDur = dur
	s.lastCounts = counts
	s.lastErrs = errs
	s.totalRuns++
}

func (s *syncStatus) snapshot() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var lastRunStr string
	if !s.lastRun.IsZero() {
		lastRunStr = s.lastRun.Format("2006-01-02 15:04:05")
	}
	snap := map[string]any{
		"running":     s.running,
		"last_run":    lastRunStr,
		"last_dur":    s.lastDur.String(),
		"last_errors": s.lastErrs,
		"last_synced": map[string]int{
			"products":  s.lastCounts.Products,
			"customers": s.lastCounts.Customers,
		},
		"total_runs": s.totalRuns,
	}
	if s.running {
		snap["current_progress"] = map[string]any{
			"started_at":       s.startedAt.Format("2006-01-02 15:04:05"),
			"elapsed":          time.Since(s.startedAt).Round(time.Second).String(),
			"products_indexed": s.current.Products,
			"customers_indexed": s.current.Customers,
		}
	}
	return snap
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func jsonResp(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}

func apiAuth(key string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		k := r.Header.Get("X-Internal-Key")
		if k == "" {
			k = r.Header.Get("Authorization")
			if len(k) > 7 {
				k = k[7:] // strip "Bearer "
			}
		}
		if k != key {
			jsonResp(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	port             := envOr("PORT", "8084")
	internalKey      := envOr("INTERNAL_KEY", "ob-jurnal-internal-2026")
	jurnalURL        := envOr("JURNAL_API_URL", "https://api.jurnal.id/partner/core/api/v1/")
	jurnalToken      := os.Getenv("JURNAL_BEARER_TOKEN")
	meiliURL         := envOr("MEILI_URL", "http://127.0.0.1:7700")
	meiliKey         := os.Getenv("MEILI_KEY")
	meiliProductIdx  := envOr("MEILI_PRODUCT_INDEX", "jurnal_products")
	meiliCustomerIdx := envOr("MEILI_CUSTOMER_INDEX", "customers")
	intervalHrs      := envOr("SYNC_INTERVAL_HOURS", "12")
	hours, _         := strconv.Atoi(intervalHrs)
	if hours <= 0 {
		hours = 12
	}

	jurnal := newJurnal(jurnalURL, jurnalToken)
	meili  := newMeili(meiliURL, meiliKey)
	meili.productIndex  = meiliProductIdx
	meili.customerIndex = meiliCustomerIdx
	status := &syncStatus{}

	// Initial sync on startup
	go runFullSync(context.Background(), jurnal, meili, status)

	// Periodic sync
	go func() {
		ticker := time.NewTicker(time.Duration(hours) * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			if !status.isRunning() {
				go runFullSync(context.Background(), jurnal, meili, status)
			} else {
				log.Println("[scheduler] skipping — sync already running")
			}
		}
	}()

	// ── Routes ──────────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		jsonResp(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /status", apiAuth(internalKey, func(w http.ResponseWriter, r *http.Request) {
		jsonResp(w, http.StatusOK, status.snapshot())
	}))

	mux.HandleFunc("POST /sync-now", apiAuth(internalKey, func(w http.ResponseWriter, r *http.Request) {
		if status.isRunning() {
			jsonResp(w, http.StatusConflict, map[string]string{"status": "already running"})
			return
		}
		go runFullSync(context.Background(), jurnal, meili, status)
		jsonResp(w, http.StatusOK, map[string]string{"status": "sync started"})
	}))

	log.Printf("[go-jurnal-sync] listening on :%s — sync every %dh", port, hours)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
