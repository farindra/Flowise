package main

import (
	"encoding/json"
	"net/http"
	"strings"
)

// apiAuth middleware: validates X-Internal-Key header.
func apiAuth(key string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		k := r.Header.Get("X-Internal-Key")
		if k == "" {
			k = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if k != key {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

// GET /api/bots — list all bots (token masked).
func handleListBots(mgr *BotManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		records, err := dbListBots(r.Context())
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		type row struct {
			ID            string `json:"id"`
			Name          string `json:"name"`
			TokenMasked   string `json:"token_masked"`
			ChatflowID    string `json:"chatflow_id"`
			AllowUserIDs  string `json:"allow_user_ids"`
			DisableUpload bool   `json:"disable_upload"`
			HumanContact  string `json:"human_contact"`
			Active        bool   `json:"active"`
			WebhookURL    string `json:"webhook_url"`
			CreatedAt     string `json:"created_at"`
		}
		var out []row
		for _, rec := range records {
			masked := maskToken(rec.Token)
			wh := ""
			if mgr.webhookBase != "" {
				wh = mgr.webhookBase + "/webhook/" + rec.ID
			}
			out = append(out, row{
				ID:            rec.ID,
				Name:          rec.Name,
				TokenMasked:   masked,
				ChatflowID:    rec.ChatflowID,
				AllowUserIDs:  rec.AllowUserIDs,
				DisableUpload: rec.DisableUpload,
				HumanContact:  rec.HumanContact,
				Active:        rec.Active,
				WebhookURL:    wh,
				CreatedAt:     rec.CreatedAt.Format("2006-01-02T15:04:05Z"),
			})
		}
		if out == nil {
			out = []row{}
		}
		jsonOK(w, out)
	}
}

// POST /api/bots — create a new bot.
func handleCreateBot(mgr *BotManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name          string `json:"name"`
			Token         string `json:"token"`
			ChatflowID    string `json:"chatflow_id"`
			AllowUserIDs  string `json:"allow_user_ids"`
			DisableUpload bool   `json:"disable_upload"`
			HumanContact  string `json:"human_contact"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, "invalid json", http.StatusBadRequest)
			return
		}
		if body.Name == "" || body.Token == "" || body.ChatflowID == "" {
			jsonError(w, "name, token, chatflow_id required", http.StatusBadRequest)
			return
		}
		exists, err := dbTokenExists(r.Context(), body.Token)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if exists {
			jsonError(w, "token already registered", http.StatusConflict)
			return
		}
		rec := &BotRecord{
			Name:          body.Name,
			Token:         body.Token,
			ChatflowID:    body.ChatflowID,
			AllowUserIDs:  body.AllowUserIDs,
			DisableUpload: body.DisableUpload,
			HumanContact:  body.HumanContact,
			Active:        true,
		}
		id, err := mgr.Add(r.Context(), rec)
		if err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		jsonOK(w, map[string]string{"id": id})
	}
}

// PUT /api/bots/:id — update bot config.
func handleUpdateBot(mgr *BotManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		var body struct {
			Name          string `json:"name"`
			ChatflowID    string `json:"chatflow_id"`
			AllowUserIDs  string `json:"allow_user_ids"`
			DisableUpload bool   `json:"disable_upload"`
			HumanContact  string `json:"human_contact"`
			Active        *bool  `json:"active"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, "invalid json", http.StatusBadRequest)
			return
		}
		existing, err := dbGetBot(r.Context(), id)
		if err != nil {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		if body.Name != "" {
			existing.Name = body.Name
		}
		if body.ChatflowID != "" {
			existing.ChatflowID = body.ChatflowID
		}
		existing.AllowUserIDs = body.AllowUserIDs
		existing.DisableUpload = body.DisableUpload
		existing.HumanContact = body.HumanContact
		if body.Active != nil {
			existing.Active = *body.Active
		}
		if err := mgr.Update(r.Context(), existing); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]string{"status": "updated"})
	}
}

// DELETE /api/bots/:id — remove bot.
func handleDeleteBot(mgr *BotManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := mgr.Remove(r.Context(), id); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})
	}
}

// POST /api/bots/:id/register-webhook — force re-register webhook.
func handleRegisterWebhook(mgr *BotManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		mgr.mu.RLock()
		bot, ok := mgr.bots[id]
		mgr.mu.RUnlock()
		if !ok {
			jsonError(w, "bot not found or inactive", http.StatusNotFound)
			return
		}
		webhookURL := mgr.webhookBase + "/webhook/" + id
		go bot.registerWebhook(webhookURL)
		jsonOK(w, map[string]string{"status": "registering", "webhook_url": webhookURL})
	}
}

func maskToken(token string) string {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) == 2 && len(parts[1]) > 6 {
		return parts[0] + ":***" + parts[1][len(parts[1])-4:]
	}
	if len(token) > 8 {
		return token[:4] + "***" + token[len(token)-4:]
	}
	return "***"
}
