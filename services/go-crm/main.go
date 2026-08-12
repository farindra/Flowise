package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"

	// Embed the timezone database: the binary is deployed standalone via
	// start.sh, and broadcast scheduling depends on Asia/Jakarta resolving.
	_ "time/tzdata"
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

	startImportJobJanitor()

	initThrottleDefaults()
	startBroadcastWorker()

	internalKey := envOr("INTERNAL_API_KEY", "")
	port := envOr("PORT", "8083")

	if v, err := strconv.ParseFloat(envOr("DEFAULT_MARKUP_PERCENT", "23"), 64); err == nil {
		unregisteredCustomerMarkup = v
	}
	log.Printf("unregistered customer markup: %.2f%%", unregisteredCustomerMarkup)

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

	// Customers (VIP / Blacklist)
	mux.HandleFunc("GET /api/customers/tier", apiAuth(internalKey, handleCustomerTier))
	mux.HandleFunc("GET /api/customers/wilayah", apiAuth(internalKey, handleCustomerWilayah))
	mux.HandleFunc("GET /api/customers/template", apiAuth(internalKey, handleCustomerTemplate))
	mux.HandleFunc("GET /api/customers/export", apiAuth(internalKey, handleCustomerExport))
	mux.HandleFunc("POST /api/customers/import", apiAuth(internalKey, handleCustomerImport))
	mux.HandleFunc("GET /api/customers/import/{jobId}", apiAuth(internalKey, handleCustomerImportStatus))
	mux.HandleFunc("GET /api/customers", apiAuth(internalKey, handleListCustomers))
	mux.HandleFunc("POST /api/customers", apiAuth(internalKey, handleCreateCustomer))
	mux.HandleFunc("GET /api/customers/{id}", apiAuth(internalKey, handleGetCustomer))
	mux.HandleFunc("PUT /api/customers/{id}", apiAuth(internalKey, handleUpdateCustomer))
	mux.HandleFunc("DELETE /api/customers/{id}", apiAuth(internalKey, handleDeleteCustomer))

	// Broadcast — literal paths first so they aren't swallowed by /{id}
	mux.HandleFunc("GET /api/broadcasts/throttle-defaults", apiAuth(internalKey, handleBroadcastThrottleDefaults))
	mux.HandleFunc("GET /api/broadcasts/senders", apiAuth(internalKey, handleBroadcastSenders))
	mux.HandleFunc("GET /api/broadcasts/chat-sources", apiAuth(internalKey, handleBroadcastChatSources))
	mux.HandleFunc("GET /api/broadcasts/template", apiAuth(internalKey, handleBroadcastTemplate))
	mux.HandleFunc("GET /api/broadcasts/optouts", apiAuth(internalKey, handleListOptOuts))
	mux.HandleFunc("POST /api/broadcasts/optouts", apiAuth(internalKey, handleCreateOptOut))
	mux.HandleFunc("DELETE /api/broadcasts/optouts/{phone}", apiAuth(internalKey, handleDeleteOptOut))
	mux.HandleFunc("POST /api/broadcasts/preview-audience", apiAuth(internalKey, handlePreviewAudience))
	mux.HandleFunc("POST /api/broadcasts/audience-file", apiAuth(internalKey, handleAudienceFile))
	// Media lives on its own prefix: /api/broadcasts/media/{name} would be
	// ambiguous against /api/broadcasts/{id}/recipients (Go 1.22 mux rejects it).
	mux.HandleFunc("POST /api/broadcast-media", apiAuth(internalKey, handleBroadcastMediaUpload))
	mux.HandleFunc("GET /api/broadcast-media/{name}", apiAuth(internalKey, handleBroadcastMediaGet))
	mux.HandleFunc("GET /api/broadcasts", apiAuth(internalKey, handleListBroadcasts))
	mux.HandleFunc("POST /api/broadcasts", apiAuth(internalKey, handleCreateBroadcast))
	mux.HandleFunc("GET /api/broadcasts/{id}", apiAuth(internalKey, handleGetBroadcast))
	mux.HandleFunc("PUT /api/broadcasts/{id}", apiAuth(internalKey, handleUpdateBroadcast))
	mux.HandleFunc("DELETE /api/broadcasts/{id}", apiAuth(internalKey, handleDeleteBroadcast))
	mux.HandleFunc("POST /api/broadcasts/{id}/audience", apiAuth(internalKey, handleBuildAudience))
	mux.HandleFunc("GET /api/broadcasts/{id}/audience/{jobId}", apiAuth(internalKey, handleBuildAudienceStatus))
	mux.HandleFunc("GET /api/broadcasts/{id}/recipients", apiAuth(internalKey, handleListBroadcastRecipients))
	mux.HandleFunc("GET /api/broadcasts/{id}/export", apiAuth(internalKey, handleExportBroadcastRecipients))
	mux.HandleFunc("POST /api/broadcasts/{id}/test-send", apiAuth(internalKey, handleBroadcastTestSend))
	mux.HandleFunc("POST /api/broadcasts/{id}/start", apiAuth(internalKey, handleStartBroadcast))
	mux.HandleFunc("POST /api/broadcasts/{id}/pause", apiAuth(internalKey, handlePauseBroadcast))
	mux.HandleFunc("POST /api/broadcasts/{id}/resume", apiAuth(internalKey, handleResumeBroadcast))
	mux.HandleFunc("POST /api/broadcasts/{id}/cancel", apiAuth(internalKey, handleCancelBroadcast))
	mux.HandleFunc("POST /api/broadcasts/{id}/retry", apiAuth(internalKey, handleRetryBroadcast))

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
