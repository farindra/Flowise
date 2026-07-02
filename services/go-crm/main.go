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

	// Salesmen
	mux.HandleFunc("GET /api/salesmen", apiAuth(internalKey, handleListSalesmen))
	mux.HandleFunc("POST /api/salesmen", apiAuth(internalKey, handleCreateSalesman))
	mux.HandleFunc("GET /api/salesmen/{id}", apiAuth(internalKey, handleGetSalesman))
	mux.HandleFunc("PUT /api/salesmen/{id}", apiAuth(internalKey, handleUpdateSalesman))
	mux.HandleFunc("DELETE /api/salesmen/{id}", apiAuth(internalKey, handleDeleteSalesman))

	// Campaigns (internal CRUD)
	mux.HandleFunc("GET /api/campaigns/check-slug", apiAuth(internalKey, handleCheckSlug))
	mux.HandleFunc("GET /api/campaigns", apiAuth(internalKey, handleListCampaigns))
	mux.HandleFunc("POST /api/campaigns", apiAuth(internalKey, handleCreateCampaign))
	mux.HandleFunc("GET /api/campaigns/{id}", apiAuth(internalKey, handleGetCampaign))
	mux.HandleFunc("PUT /api/campaigns/{id}", apiAuth(internalKey, handleUpdateCampaign))
	mux.HandleFunc("DELETE /api/campaigns/{id}", apiAuth(internalKey, handleDeleteCampaign))

	// Public campaign endpoints (no auth — untuk landing page & form submit)
	mux.HandleFunc("GET /api/public/campaigns/{slug}", handlePublicGetCampaign)
	mux.HandleFunc("POST /api/public/campaigns/{slug}/submit", handlePublicSubmitCampaign)
	mux.HandleFunc("OPTIONS /api/public/campaigns/{slug}/submit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.WriteHeader(http.StatusNoContent)
	})

	// Static files (logo, image assets)
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	// Landing page HTML — serve langsung dari go-crm tanpa Flowise
	mux.HandleFunc("GET /{slug}", handlePublicLandingPage)

	// Stats for dashboard
	mux.HandleFunc("GET /api/stats", apiAuth(internalKey, handleStats))

	log.Printf("go-crm listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
