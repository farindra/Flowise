package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	ctx := context.Background()

	// ── Config ────────────────────────────────────────────────────────────────
	port         := envOr("PORT", "8081")
	flowiseBase  := envOr("FLOWISE_BASE_URL", "https://agentic.oceanbearings.co.id")
	flowiseKey   := os.Getenv("FLOWISE_API_KEY")
	webhookBase  := os.Getenv("WEBHOOK_BASE_URL")
	internalKey  := envOr("INTERNAL_API_KEY", "ob-tg-internal-2026")
	timeout      := parseTimeoutSec(envOr("FLOWISE_TIMEOUT", "60"))
	waitInterval := parseTimeoutSec(envOr("WAIT_MSG_INTERVAL", "6"))

	// ── Database ──────────────────────────────────────────────────────────────
	if err := initDB(ctx); err != nil {
		log.Fatalf("DB init failed: %v", err)
	}
	log.Println("DB connected")

	if err := migrateDB(ctx); err != nil {
		log.Fatalf("DB migrate failed: %v", err)
	}

	// Seed legacy env-based bots if not yet in DB
	seedFromEnv(ctx)

	// ── Bot manager ───────────────────────────────────────────────────────────
	mgr := NewBotManager(flowiseBase, flowiseKey, webhookBase, timeout, waitInterval)
	if err := mgr.LoadAll(ctx); err != nil {
		log.Printf("LoadAll error (non-fatal): %v", err)
	}

	// ── HTTP routes ───────────────────────────────────────────────────────────
	mux := http.NewServeMux()

	// Webhook per bot: /webhook/<bot-uuid>
	mux.HandleFunc("POST /webhook/{id}", mgr.Handler())

	// API (protected by internal key)
	mux.HandleFunc("GET /api/bots",                    apiAuth(internalKey, handleListBots(mgr)))
	mux.HandleFunc("POST /api/bots",                   apiAuth(internalKey, handleCreateBot(mgr)))
	mux.HandleFunc("PUT /api/bots/{id}",               apiAuth(internalKey, handleUpdateBot(mgr)))
	mux.HandleFunc("DELETE /api/bots/{id}",            apiAuth(internalKey, handleDeleteBot(mgr)))
	mux.HandleFunc("POST /api/bots/{id}/register",     apiAuth(internalKey, handleRegisterWebhook(mgr)))

	// Health
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "OK")
	})

	log.Printf("go-telegram listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

// warmup gibt time for webhook registration goroutines to fire.
func init() {
	_ = time.Second // suppress unused import
}
