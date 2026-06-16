package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"customer-pricing-service/internal/handler"
	"customer-pricing-service/internal/jurnal"
	"customer-pricing-service/internal/pricing"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	port := getenv("PORT", "8102")
	name := getenv("SERVICE_NAME", "customer-pricing-service")
	dataDir := getenv("DATA_DIR", "/data")

	apiKeyCust1 := os.Getenv("API_KEY_CUST1")
	apiKeyCust2 := os.Getenv("API_KEY_CUST2")
	profileURL := getenv("CUSTOMER_PROFILE_API_URL", "https://api.jurnal.id/partner/core/api/v1/contacts/{id}")

	syncIntervalMs, err := strconv.Atoi(getenv("CACHE_SYNC_INTERVAL", "1800000"))
	if err != nil || syncIntervalMs <= 0 {
		syncIntervalMs = 1800000
	}
	syncInterval := time.Duration(syncIntervalMs) * time.Millisecond

	jurnalClient := jurnal.New(apiKeyCust1, apiKeyCust2, profileURL)

	store := pricing.NewStore(dataDir, jurnalClient)
	if err := store.LoadSnapshot(); err != nil {
		log.Printf("warning: failed to load customer snapshot: %v", err)
	}
	log.Printf("loaded %d customers from snapshot", store.Count())

	vipStore := pricing.NewVipStore(filepath.Join(dataDir, "customer_vip.json"))
	if err := vipStore.Load(); err != nil {
		log.Printf("warning: failed to load VIP customers: %v", err)
	}
	log.Printf("loaded %d VIP customers", len(vipStore.All()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go vipStore.WatchAndReload(ctx)

	// Background customer sync: run once at startup, then every syncInterval.
	go func() {
		runSync := func() {
			log.Printf("starting customer sync...")
			start := time.Now()
			if err := store.Sync(ctx); err != nil {
				log.Printf("customer sync failed: %v", err)
				return
			}
			log.Printf("customer sync completed: %d customers in %s", store.Count(), time.Since(start))
		}

		runSync()

		ticker := time.NewTicker(syncInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runSync()
			}
		}
	}()

	h := handler.New(store, vipStore, name)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/customer", h.Customer)
	mux.HandleFunc("/customer/by-company", h.CustomerByCompany)
	mux.HandleFunc("/customer/vip", h.CustomerVip)
	mux.HandleFunc("/price", h.Price)

	log.Printf("%s listening on :%s", name, port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
