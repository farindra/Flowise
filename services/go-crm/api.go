package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func apiAuth(key string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		k := r.Header.Get("X-Internal-Key")
		if k == "" {
			k = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		}
		if key != "" && k != key {
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

// ── Leads ─────────────────────────────────────────────────────────────────────

func handleListLeads(w http.ResponseWriter, r *http.Request) {
	stage := r.URL.Query().Get("stage")
	leads, err := dbListLeads(r.Context(), stage)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if leads == nil {
		leads = []Lead{}
	}
	jsonOK(w, leads)
}

func handleGetLead(w http.ResponseWriter, r *http.Request) {
	lead, err := dbGetLead(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	jsonOK(w, lead)
}

func handleCreateLead(w http.ResponseWriter, r *http.Request) {
	var body Lead
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Phone == "" {
		jsonError(w, "phone required", http.StatusBadRequest)
		return
	}
	if body.Stage == "" {
		body.Stage = "new"
	}
	if body.Source == "" {
		body.Source = "wa"
	}
	id, err := dbCreateLead(r.Context(), &body)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Auto-schedule follow-up notifications D+1, D+3, D+7
	name := body.Name
	if name == "" {
		name = "Bapak/Ibu"
	}
	templates := []struct {
		days int
		msg  string
	}{
		{1, "Assalamu'alaikum " + name + ", terima kasih sudah menghubungi Al Azhar Memorial Garden. Ada pertanyaan mengenai kavling yang bisa kami bantu? Konsultan kami siap di WA: 085 888 555 200. Jazakallahu Khayran."},
		{3, "Assalamu'alaikum " + name + ", kami dari Al Azhar Memorial Garden kembali menyapa. Apakah Bapak/Ibu sudah sempat mempertimbangkan pilihan kavling? Kami bisa jadwalkan kunjungan ke lahan Karawang. Hubungi kami di 085 888 555 200."},
		{7, "Assalamu'alaikum " + name + ", semoga Bapak/Ibu dan keluarga sehat selalu. Al Azhar Memorial Garden — pemakaman muslim terpercaya — siap melayani kapan pun dibutuhkan. Info: 085 888 555 200. Jazakallahu Khayran."},
	}
	leadID := id
	for _, t := range templates {
		notif := &Notification{
			LeadID:         &leadID,
			Channel:        "wa",
			Type:           "lead_followup",
			RecipientPhone: body.Phone,
			Message:        t.msg,
			ScheduledAt:    time.Now().UTC().AddDate(0, 0, t.days),
		}
		_, _ = dbCreateNotif(r.Context(), notif)
	}

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, map[string]string{"id": id})
}

func handleUpdateLead(w http.ResponseWriter, r *http.Request) {
	existing, err := dbGetLead(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	var body struct {
		Name        string     `json:"name"`
		Stage       string     `json:"stage"`
		Score       *int       `json:"score"`
		Urgency     string     `json:"urgency"`
		BudgetRange string     `json:"budget_range"`
		Interest    string     `json:"interest"`
		Notes       string     `json:"notes"`
		AssignedTo  string     `json:"assigned_to"`
		FollowUpAt  *time.Time `json:"follow_up_at"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Name != "" {
		existing.Name = body.Name
	}
	if body.Stage != "" {
		existing.Stage = body.Stage
	}
	if body.Score != nil {
		existing.Score = *body.Score
	}
	if body.Urgency != "" {
		existing.Urgency = body.Urgency
	}
	if body.BudgetRange != "" {
		existing.BudgetRange = body.BudgetRange
	}
	if body.Interest != "" {
		existing.Interest = body.Interest
	}
	if body.Notes != "" {
		existing.Notes = body.Notes
	}
	if body.AssignedTo != "" {
		existing.AssignedTo = body.AssignedTo
	}
	if body.FollowUpAt != nil {
		existing.FollowUpAt = body.FollowUpAt
	}
	now := time.Now()
	existing.LastContactAt = &now

	if err := dbUpdateLead(r.Context(), existing); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "updated"})
}

// ── Kavlings ──────────────────────────────────────────────────────────────────

func handleListKavlings(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	kavlings, err := dbListKavlings(r.Context(), status)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if kavlings == nil {
		kavlings = []Kavling{}
	}
	jsonOK(w, kavlings)
}

func handleGetKavling(w http.ResponseWriter, r *http.Request) {
	kav, err := dbGetKavling(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	jsonOK(w, kav)
}

func handleUpdateKavling(w http.ResponseWriter, r *http.Request) {
	existing, err := dbGetKavling(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	var body struct {
		Status  string  `json:"status"`
		BuyerID *string `json:"buyer_id"`
		Notes   string  `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Status != "" {
		existing.Status = body.Status
	}
	if body.BuyerID != nil {
		existing.BuyerID = body.BuyerID
	}
	if body.Notes != "" {
		existing.Notes = body.Notes
	}
	if err := dbUpdateKavling(r.Context(), existing); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "updated"})
}

// ── Buyers ────────────────────────────────────────────────────────────────────

func handleListBuyers(w http.ResponseWriter, r *http.Request) {
	buyers, err := dbListBuyers(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if buyers == nil {
		buyers = []Buyer{}
	}
	jsonOK(w, buyers)
}

func handleGetBuyer(w http.ResponseWriter, r *http.Request) {
	buyer, err := dbGetBuyer(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	jsonOK(w, buyer)
}

func handleCreateBuyer(w http.ResponseWriter, r *http.Request) {
	var body Buyer
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Name == "" || body.Phone == "" {
		jsonError(w, "name and phone required", http.StatusBadRequest)
		return
	}
	id, err := dbCreateBuyer(r.Context(), &body)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, map[string]string{"id": id})
}

// ── Notifications ─────────────────────────────────────────────────────────────

func handleListPendingNotifs(w http.ResponseWriter, r *http.Request) {
	notifs, err := dbListPendingNotifs(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if notifs == nil {
		notifs = []Notification{}
	}
	jsonOK(w, notifs)
}

func handleCreateNotif(w http.ResponseWriter, r *http.Request) {
	var body Notification
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.RecipientPhone == "" || body.Message == "" || body.Type == "" {
		jsonError(w, "recipient_phone, message, type required", http.StatusBadRequest)
		return
	}
	if body.ScheduledAt.IsZero() {
		body.ScheduledAt = time.Now()
	}
	if body.Channel == "" {
		body.Channel = "wa"
	}
	id, err := dbCreateNotif(r.Context(), &body)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, map[string]string{"id": id})
}

func handleMarkNotifSent(w http.ResponseWriter, r *http.Request) {
	if err := dbMarkNotifSent(r.Context(), r.PathValue("id")); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "sent"})
}

// ── Stats ─────────────────────────────────────────────────────────────────────

func handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var totalLeads, newLeads, hotLeads, totalBuyers, availableKavlings, pendingNotifs int
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_leads`).Scan(&totalLeads)
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_leads WHERE stage='new'`).Scan(&newLeads)
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_leads WHERE urgency='high' AND stage NOT IN ('closed','lost')`).Scan(&hotLeads)
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_buyers`).Scan(&totalBuyers)
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_kavlings WHERE status='available'`).Scan(&availableKavlings)
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_notifications WHERE status='pending' AND scheduled_at <= NOW()`).Scan(&pendingNotifs)

	jsonOK(w, map[string]int{
		"total_leads":        totalLeads,
		"new_leads":          newLeads,
		"hot_leads":          hotLeads,
		"total_buyers":       totalBuyers,
		"available_kavlings": availableKavlings,
		"pending_notifs":     pendingNotifs,
	})
}
