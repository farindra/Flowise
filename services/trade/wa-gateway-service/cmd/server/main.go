package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"wa-gateway-service/internal/client"
	mongostore "wa-gateway-service/internal/mongo"
	"wa-gateway-service/internal/router"
	"wa-gateway-service/internal/state"
	"wa-gateway-service/internal/waclient"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8100"
	}
	name := os.Getenv("SERVICE_NAME")
	if name == "" {
		name = "wa-gateway-service"
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}

	qrToken := os.Getenv("QR_TOKEN")
	if qrToken == "" {
		b := make([]byte, 16)
		if _, err := rand.Read(b); err != nil {
			log.Fatalf("failed to generate qr token: %v", err)
		}
		qrToken = hex.EncodeToString(b)
	}
	log.Printf("QR token (pakai ?token=... di /qr dan /pair-phone): %s", qrToken)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	store, err := state.Open(dataDir)
	if err != nil {
		log.Fatalf("failed to open state store: %v", err)
	}
	defer store.Close()

	// MongoDB Atlas sync (Phase 4) — optional, skip gracefully when URI absent.
	mongoURI := os.Getenv("MONGODB_URI")
	mongoDBName := os.Getenv("MONGODB_DB_NAME")
	if mongoDBName == "" {
		mongoDBName = "ocean_bearings"
	}
	collHistory := os.Getenv("MONGODB_COLLECTION_HISTORY")
	collUsers := os.Getenv("MONGODB_COLLECTION_USERS")
	mc := mongostore.New(collHistory, collUsers)
	if mongoURI != "" {
		if err := mc.Connect(ctx, mongoURI, mongoDBName); err != nil {
			log.Printf("MongoDB connect failed (non-fatal): %v", err)
		} else {
			store.SetMongoSyncer(mc)
			defer mc.Disconnect(context.Background())
		}
	} else {
		log.Println("MONGODB_URI not set — MongoDB sync disabled")
	}

	pricingURL := os.Getenv("CUSTOMER_PRICING_URL")
	if pricingURL == "" {
		pricingURL = "http://customer-pricing-service:8102"
	}
	aiURL := os.Getenv("AI_VISION_URL")
	if aiURL == "" {
		aiURL = "http://ai-vision-service:8103"
	}
	searchURL := os.Getenv("PRODUCT_SEARCH_URL")
	if searchURL == "" {
		searchURL = "http://product-search-service:8101"
	}

	pricingClient := client.NewCustomerPricingClient(pricingURL)
	aiClient := client.NewAIVisionClient(aiURL)
	searchClient := client.NewProductSearchClient(searchURL)

	// Flowise integration (Phase 7) — optional.
	var flowiseClient *client.FlowiseClient
	flowiseURL := os.Getenv("FLOWISE_URL")
	flowiseChatflowID := os.Getenv("FLOWISE_CHATFLOW_ID")
	flowiseAPIKey := os.Getenv("FLOWISE_API_KEY")
	if flowiseURL != "" && flowiseChatflowID != "" {
		flowiseClient = client.NewFlowiseClient(flowiseURL, flowiseChatflowID, flowiseAPIKey)
		log.Printf("Flowise enabled: %s/api/v1/prediction/%s", flowiseURL, flowiseChatflowID)
	} else {
		log.Println("Flowise not configured — natural chat via Gemini (ai-vision-service)")
	}

	// Owner Assistant — optional separate Flowise chatflow for owner numbers.
	var ownerFlowiseClient *client.FlowiseClient
	ownerChatflowID := os.Getenv("OWNER_CHATFLOW_ID")
	if flowiseURL != "" && ownerChatflowID != "" {
		ownerFlowiseClient = client.NewFlowiseClient(flowiseURL, ownerChatflowID, flowiseAPIKey)
		log.Printf("Owner Assistant enabled: chatflow %s", ownerChatflowID)
	}
	ownerPhones := map[string]bool{}
	for _, p := range strings.Split(os.Getenv("OWNER_PHONES"), ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			ownerPhones[p] = true
		}
	}

	// TRADE bot-integration client — optional, for owner supplier-offer uploads.
	var tradeClient *client.TradeClient
	tradeURL := os.Getenv("TRADE_URL")
	tradeBotAPIKey := os.Getenv("TRADE_BOT_API_KEY")
	if tradeURL != "" && tradeBotAPIKey != "" {
		tradeClient = client.NewTradeClient(tradeURL, tradeBotAPIKey)
		log.Printf("TRADE client enabled: %s", tradeURL)
	}

	cache := state.NewCustomerCache(store, pricingClient)

	wa, err := waclient.New(ctx, dataDir)
	if err != nil {
		log.Fatalf("failed to init whatsmeow client: %v", err)
	}

	r := router.New(wa.WA, store, cache, aiClient, flowiseClient, ownerFlowiseClient, ownerPhones, tradeClient, searchClient, searchURL)
	wa.SetEventHandler(r.HandleEvent)

	go func() {
		if err := wa.Connect(ctx); err != nil {
			log.Printf("whatsmeow connect error: %v", err)
		}
	}()

	http.HandleFunc("/simulate", r.HandleSimulate(qrToken))

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"service": name,
			"status":  "ok",
		})
	})

	http.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != qrToken {
			http.Error(w, "invalid token", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"connected":  wa.IsConnected(),
			"logged_in":  wa.IsLoggedIn(),
			"phone":      wa.PhoneNumber(),
			"qr_pending": wa.IsConnected() && !wa.IsLoggedIn(),
		})
	})

	http.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Query().Get("token") != qrToken {
			http.Error(w, "invalid token", http.StatusForbidden)
			return
		}
		if err := wa.Logout(r.Context()); err != nil {
			http.Error(w, "logout error: "+err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "logged_out"})
	})

	http.HandleFunc("/qr", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != qrToken {
			http.Error(w, "invalid token", http.StatusForbidden)
			return
		}
		http.ServeFile(w, r, wa.QRPath())
	})

	http.HandleFunc("/pair-phone", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") != qrToken {
			http.Error(w, "invalid token", http.StatusForbidden)
			return
		}
		phone := r.URL.Query().Get("phone")
		if phone == "" {
			http.Error(w, "missing ?phone= query param (format: 62812xxxxxxxx, tanpa + atau 0 di depan)", http.StatusBadRequest)
			return
		}
		code, err := wa.PairPhone(r.Context(), phone)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"pairing_code": code})
	})

	server := &http.Server{Addr: os.Getenv("BIND_ADDR") + ":" + port}
	go func() {
		log.Printf("%s listening on :%s", name, port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	wa.Disconnect()
	_ = server.Close()
}
