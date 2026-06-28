package main

import (
	"context"
	"log"
	"net/http"
	"os"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	ctx := context.Background()

	if err := initDB(ctx); err != nil {
		log.Fatalf("db init: %v", err)
	}
	if err := migrateDB(ctx); err != nil {
		log.Fatalf("db migrate: %v", err)
	}

	internalKey := envOr("INTERNAL_API_KEY", "")
	port := envOr("PORT", "8084")

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/zones", apiAuth(internalKey, handleListZones))
	mux.HandleFunc("POST /api/readings", apiAuth(internalKey, handlePostReading))
	mux.HandleFunc("GET /api/readings/history", apiAuth(internalKey, handleReadingsHistory))
	mux.HandleFunc("GET /api/alerts", apiAuth(internalKey, handleListAlerts))
	mux.HandleFunc("PUT /api/alerts/{id}/resolve", apiAuth(internalKey, handleResolveAlert))

	log.Printf("go-iot listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
