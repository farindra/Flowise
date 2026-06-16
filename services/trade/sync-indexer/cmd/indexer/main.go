package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"sync-indexer/internal/meili"
	"sync-indexer/internal/sourceapi"
	"sync-indexer/internal/transform"
)

var errEmptyResponse = errors.New("empty API response - refusing to overwrite existing data")

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	name := getenv("SERVICE_NAME", "sync-indexer")

	produkAPIURL := os.Getenv("PRODUK_API_URL")
	produkAPIURL2 := os.Getenv("PRODUK_API_URL2")
	meiliURL := getenv("MEILI_URL", "http://meilisearch:7700")
	meiliAPIKey := os.Getenv("MEILI_API_KEY")

	syncIntervalMs, err := strconv.Atoi(getenv("CACHE_SYNC_INTERVAL", "1800000"))
	if err != nil || syncIntervalMs <= 0 {
		syncIntervalMs = 1800000
	}
	syncInterval := time.Duration(syncIntervalMs) * time.Millisecond

	if produkAPIURL == "" {
		log.Fatal("PRODUK_API_URL is required")
	}

	src := sourceapi.New()
	meiliClient := meili.New(meiliURL, meiliAPIKey)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := meiliClient.EnsureIndex(ctx); err != nil {
		log.Fatalf("meili: failed to ensure index: %v", err)
	}

	runSync := func() {
		log.Printf("starting product sync...")
		start := time.Now()
		if err := syncProducts(ctx, src, meiliClient, produkAPIURL, produkAPIURL2); err != nil {
			log.Printf("product sync failed: %v", err)
			return
		}
		log.Printf("product sync completed in %s", time.Since(start))
	}

	runSync()

	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()

	log.Printf("%s started, syncing every %s", name, syncInterval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("%s shutting down", name)
			return
		case <-ticker.C:
			runSync()
		}
	}
}

// syncProducts - ported from syncProducts in local-sync-system.js: fetch
// primary (+ optional secondary) product list, merge by kode||name, transform
// to Meilisearch documents, and replace the index contents (with a safety
// check against wiping a good index with a too-small fetch).
func syncProducts(ctx context.Context, src *sourceapi.Client, meiliClient *meili.Client, primaryURL, secondaryURL string) error {
	primary, err := src.FetchProducts(ctx, primaryURL)
	if err != nil {
		return err
	}
	log.Printf("fetched %d products from primary endpoint", len(primary))

	var merged []transform.RawProduct
	if secondaryURL != "" {
		secondary, err := src.FetchProducts(ctx, secondaryURL)
		if err != nil {
			log.Printf("secondary endpoint fetch failed, continuing with primary only: %v", err)
			merged = primary
		} else {
			log.Printf("fetched %d products from secondary endpoint", len(secondary))
			merged = transform.MergeProducts(primary, secondary)
		}
	} else {
		merged = primary
	}

	log.Printf("total products after merging: %d", len(merged))

	if len(merged) == 0 {
		return errEmptyResponse
	}

	existingCount, err := meiliClient.DocumentCount(ctx)
	if err != nil {
		log.Printf("warning: failed to get existing document count: %v", err)
		existingCount = 0
	}
	if existingCount > 100 && float64(len(merged)) < float64(existingCount)*0.5 {
		log.Printf("new data (%d) is significantly smaller than existing (%d), skipping save", len(merged), existingCount)
		return nil
	}

	docs := make([]transform.ProductDoc, 0, len(merged))
	for _, p := range merged {
		docs = append(docs, transform.ToDoc(p))
	}

	if err := meiliClient.ReplaceAllDocuments(ctx, docs); err != nil {
		return err
	}

	log.Printf("indexed %d products", len(docs))
	return nil
}
