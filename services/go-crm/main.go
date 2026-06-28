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
	port := envOr("PORT", "8083")

	mux := http.NewServeMux()

	// Leads
	mux.HandleFunc("GET /api/leads", apiAuth(internalKey, handleListLeads))
	mux.HandleFunc("POST /api/leads", apiAuth(internalKey, handleCreateLead))
	mux.HandleFunc("GET /api/leads/{id}", apiAuth(internalKey, handleGetLead))
	mux.HandleFunc("PUT /api/leads/{id}", apiAuth(internalKey, handleUpdateLead))

	// Kavlings
	mux.HandleFunc("GET /api/kavlings", apiAuth(internalKey, handleListKavlings))
	mux.HandleFunc("GET /api/kavlings/{id}", apiAuth(internalKey, handleGetKavling))
	mux.HandleFunc("PUT /api/kavlings/{id}", apiAuth(internalKey, handleUpdateKavling))

	// Buyers
	mux.HandleFunc("GET /api/buyers", apiAuth(internalKey, handleListBuyers))
	mux.HandleFunc("POST /api/buyers", apiAuth(internalKey, handleCreateBuyer))
	mux.HandleFunc("GET /api/buyers/{id}", apiAuth(internalKey, handleGetBuyer))

	// Notifications queue
	mux.HandleFunc("GET /api/notifications/pending", apiAuth(internalKey, handleListPendingNotifs))
	mux.HandleFunc("POST /api/notifications", apiAuth(internalKey, handleCreateNotif))
	mux.HandleFunc("PUT /api/notifications/{id}/sent", apiAuth(internalKey, handleMarkNotifSent))

	// Lead nurturing
	mux.HandleFunc("GET /api/leads/unresponded", apiAuth(internalKey, handleUnrespondedLeads))

	// Stats for dashboard
	mux.HandleFunc("GET /api/stats", apiAuth(internalKey, handleStats))

	log.Printf("go-crm listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
