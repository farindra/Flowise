package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := initDB(ctx); err != nil {
		log.Fatalf("DB init: %v", err)
	}
	log.Println("DB connected")

	if err := migrateDB(ctx); err != nil {
		log.Fatalf("DB migrate: %v", err)
	}

	flowiseBaseURL := mustEnv("FLOWISE_BASE_URL")
	flowiseAPIKey := envOr("FLOWISE_API_KEY", "")
	dataDir := envOr("DATA_DIR", "/data/wa-sessions")
	internalKey := envOr("INTERNAL_API_KEY", "ob-wa-internal-2026")
	port := envOr("PORT", "8082")
	timeoutSec := parseTimeoutSec(envOr("FLOWISE_TIMEOUT", "120"))

	mgr := NewSessionManager(flowiseBaseURL, flowiseAPIKey, dataDir, time.Duration(timeoutSec)*time.Second)

	if err := mgr.LoadAll(ctx); err != nil {
		log.Printf("LoadAll warning: %v", err)
	}

	mux := http.NewServeMux()

	// Session CRUD
	mux.HandleFunc("GET /api/sessions", apiAuth(internalKey, handleListSessions(mgr)))
	mux.HandleFunc("POST /api/sessions", apiAuth(internalKey, handleCreateSession(mgr)))
	mux.HandleFunc("PUT /api/sessions/{id}", apiAuth(internalKey, handleUpdateSession(mgr)))
	mux.HandleFunc("DELETE /api/sessions/{id}", apiAuth(internalKey, handleDeleteSession(mgr)))

	// Per-session control
	mux.HandleFunc("GET /api/sessions/{id}/status", apiAuth(internalKey, handleSessionStatus(mgr)))
	mux.HandleFunc("GET /api/sessions/{id}/qr", apiAuth(internalKey, handleSessionQR(mgr)))
	mux.HandleFunc("POST /api/sessions/{id}/connect", apiAuth(internalKey, handleSessionConnect(mgr)))
	mux.HandleFunc("POST /api/sessions/{id}/logout", apiAuth(internalKey, handleSessionLogout(mgr)))
	mux.HandleFunc("POST /api/sessions/{id}/pair-phone", apiAuth(internalKey, handleSessionPairPhone(mgr)))
	mux.HandleFunc("POST /api/sessions/{id}/send", apiAuth(internalKey, handleSessionSend(mgr)))

	// Health
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	srv := &http.Server{Addr: ":" + port, Handler: mux}
	go func() {
		log.Printf("go-wa listening on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down go-wa...")

	shutCtx, sc := context.WithTimeout(context.Background(), 10*time.Second)
	defer sc()
	_ = srv.Shutdown(shutCtx)

	// Disconnect all sessions
	mgr.mu.RLock()
	for _, s := range mgr.sessions {
		s.Disconnect()
	}
	mgr.mu.RUnlock()
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("env %s is required", key)
	}
	return v
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseTimeoutSec(s string) int {
	var n int
	_, _ = parseIntStr(s, &n)
	if n <= 0 {
		n = 120
	}
	return n
}

func parseIntStr(s string, out *int) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	*out = n
	return n, nil
}
