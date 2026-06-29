package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func pickSalesmanName(ctx context.Context) string {
	s, err := dbPickSalesman(ctx)
	if err != nil || s == nil {
		return ""
	}
	return s.Name
}

func scoreFromUrgency(urgency string) int {
	switch urgency {
	case "high":
		return 80
	case "medium":
		return 50
	default:
		return 20
	}
}

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

// scheduleDripNotifs — jadwalkan drip D+1/3/5/7 + retargeting D+30 untuk lead
func scheduleDripNotifs(ctx context.Context, leadID, phone, name string) {
	if name == "" {
		name = "Bapak/Ibu"
	}
	drip := []struct {
		days int
		typ  string
		msg  string
	}{
		{1, "drip_d1", "Assalamu'alaikum " + name + " 🌿\n\nTerima kasih sudah menghubungi *Al Azhar Memorial Garden (AAMG)* — pemakaman Muslim No.1 di Indonesia.\n\nAAMG berdiri di atas lahan 80 hektar di Karawang, dilengkapi:\n✅ Arah kiblat bersertifikat Kemenag RI\n✅ Ustaz dan petugas 24 jam\n✅ ISO 9001 manajemen pemakaman\n✅ Akses tol Karawang Barat hanya 5 menit\n\nBila ada pertanyaan, konsultan kami siap membantu di 085 888 555 200.\n\nJazakallahu Khayran 🤲"},
		{3, "drip_d3", "Assalamu'alaikum " + name + " 🌿\n\nKetenangan pikiran tidak ternilai harganya.\n\nKeluarga yang telah memiliki kavling di AAMG menceritakan: _\"Alhamdulillah, saat musibah datang kami tidak perlu bingung mencari tempat. Semua sudah disiapkan.\"_\n\n📍 Pilihan kavling kami:\n• *Keluarga* — 2 liang, cocok untuk pasangan\n• *Premium* — 4 liang + taman\n• *VIP Garden* — lokasi pilihan, view terbaik\n\nIngin tahu lebih lanjut? Balas pesan ini atau hubungi 085 888 555 200 🙏"},
		{5, "drip_d5", "Assalamu'alaikum " + name + " 🌿\n\nBerikut perbandingan tipe kavling AAMG yang sering ditanyakan:\n\n| Tipe | Liang | Luas | Harga mulai |\n|------|-------|------|-------------|\n| Standar | 1 | 1×2m | Rp 25 jt |\n| Keluarga | 2 | 2×2m | Rp 45 jt |\n| Premium | 4 | 3×3m | Rp 85 jt |\n| VIP Garden | 4+ | 4×4m | Rp 150 jt |\n\n💡 Harga kavling cenderung naik setiap tahun — semakin cepat memiliki, semakin hemat.\n\nPerlu simulasi cicilan? Hubungi kami di 085 888 555 200 📞"},
		{7, "drip_d7", "Assalamu'alaikum " + name + " 🌿\n\nIni adalah pesan terakhir dari seri perkenalan kami. Kami harap informasi yang dibagikan bermanfaat.\n\n🎁 *Penawaran Khusus:* Konsultasi GRATIS dengan konsultan AAMG — kami bisa visit ke rumah Bapak/Ibu atau kunjungi langsung lahan kami di Karawang.\n\n📞 Hubungi: 085 888 555 200\n🌐 Atau balas pesan ini kapan saja\n\n_\"Dan siapkanlah untuk menghadapi-Nya\"_ — semoga Allah memudahkan setiap urusan kita. Aamiin 🤲\n\n*Tim AAMG*"},
		{30, "retargeting", "Assalamu'alaikum " + name + " 🌿\n\nSudah sebulan berlalu sejak kami pertama berkenalan. Kami mendoakan Bapak/Ibu dan keluarga selalu dalam lindungan Allah SWT.\n\nJika suatu saat membutuhkan informasi mengenai *kavling pemakaman Muslim*, kami selalu siap membantu — tanpa tekanan.\n\n📞 *Al Azhar Memorial Garden*: 085 888 555 200\n\nJazakallahu Khayran 🤲"},
	}
	for _, t := range drip {
		_, _ = dbCreateNotif(ctx, &Notification{
			LeadID:         &leadID,
			Channel:        "wa",
			Type:           t.typ,
			RecipientPhone: phone,
			Message:        t.msg,
			ScheduledAt:    time.Now().UTC().AddDate(0, 0, t.days),
		})
	}
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
	// auto-score berdasarkan urgensi
	if body.Score == 0 {
		body.Score = scoreFromUrgency(body.Urgency)
	}
	// assign salesman: pilih yang paling sedikit leads bulan ini
	if body.AssignedTo == "" {
		body.AssignedTo = pickSalesmanName(r.Context())
	}

	id, err := dbCreateLead(r.Context(), &body)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	scheduleDripNotifs(r.Context(), id, body.Phone, body.Name)

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

// ── Salesmen ─────────────────────────────────────────────────────────────────

func handleListSalesmen(w http.ResponseWriter, r *http.Request) {
	salesmen, err := dbListSalesmen(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if salesmen == nil {
		salesmen = []Salesman{}
	}
	// attach stats per salesman
	for i := range salesmen {
		month, total, won := dbSalesmanStats(r.Context(), salesmen[i].ID)
		salesmen[i].LeadsThisMonth = month
		salesmen[i].LeadsTotal = total
		salesmen[i].LeadsWon = won
		if total > 0 {
			salesmen[i].ConversionRate = float64(won) / float64(total) * 100
		}
		if salesmen[i].CommissionType == "percentage" {
			// estimasi komisi bulan ini: asumsi avg kavling Rp 50jt
			salesmen[i].EstCommission = float64(won) * 50_000_000 * salesmen[i].CommissionRate / 100
		} else {
			salesmen[i].EstCommission = float64(won) * salesmen[i].CommissionRate
		}
	}
	jsonOK(w, salesmen)
}

func handleGetSalesman(w http.ResponseWriter, r *http.Request) {
	s, err := dbGetSalesman(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	month, total, won := dbSalesmanStats(r.Context(), s.ID)
	s.LeadsThisMonth = month
	s.LeadsTotal = total
	s.LeadsWon = won
	if total > 0 {
		s.ConversionRate = float64(won) / float64(total) * 100
	}
	jsonOK(w, s)
}

func handleCreateSalesman(w http.ResponseWriter, r *http.Request) {
	var body Salesman
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}
	if body.Status == "" {
		body.Status = "active"
	}
	if body.CommissionType == "" {
		body.CommissionType = "percentage"
	}
	id, err := dbCreateSalesman(r.Context(), &body)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, map[string]string{"id": id})
}

func handleUpdateSalesman(w http.ResponseWriter, r *http.Request) {
	existing, err := dbGetSalesman(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	var body Salesman
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Name != "" {
		existing.Name = body.Name
	}
	if body.Phone != "" {
		existing.Phone = body.Phone
	}
	if body.TelegramID != "" {
		existing.TelegramID = body.TelegramID
	}
	existing.TelegramChatID = body.TelegramChatID
	if body.Email != "" {
		existing.Email = body.Email
	}
	existing.Area = body.Area
	existing.Notes = body.Notes
	if body.CommissionType != "" {
		existing.CommissionType = body.CommissionType
	}
	existing.CommissionRate = body.CommissionRate
	if body.TargetMonthly > 0 {
		existing.TargetMonthly = body.TargetMonthly
	}
	if body.Status != "" {
		existing.Status = body.Status
	}
	if err := dbUpdateSalesman(r.Context(), existing); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "updated"})
}

func handleDeleteSalesman(w http.ResponseWriter, r *http.Request) {
	if err := dbDeleteSalesman(r.Context(), r.PathValue("id")); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

// handleUnrespondedLeads — leads masih di stage "new" lebih dari N hari (default 7)
func handleUnrespondedLeads(w http.ResponseWriter, r *http.Request) {
	days := 7
	leads, err := dbListLeads(r.Context(), "new")
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	var stale []Lead
	for _, l := range leads {
		if l.CreatedAt.Before(cutoff) {
			stale = append(stale, l)
		}
	}
	if stale == nil {
		stale = []Lead{}
	}
	jsonOK(w, stale)
}

// ── Stats ─────────────────────────────────────────────────────────────────────

func handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var totalLeads, hotLeads, totalBuyers, availableKavlings, pendingNotifs int
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_leads`).Scan(&totalLeads)
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_leads WHERE urgency='high' AND stage NOT IN ('won','lost')`).Scan(&hotLeads)
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_buyers`).Scan(&totalBuyers)
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_kavlings WHERE status='available'`).Scan(&availableKavlings)
	pool.QueryRow(ctx, `SELECT COUNT(*) FROM crm_notifications WHERE status='pending' AND scheduled_at <= NOW()`).Scan(&pendingNotifs)

	// per-stage breakdown
	rows, _ := pool.Query(ctx, `SELECT stage, COUNT(*) FROM crm_leads GROUP BY stage`)
	byStage := map[string]int{}
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var stage string
			var cnt int
			rows.Scan(&stage, &cnt)
			byStage[stage] = cnt
		}
	}

	jsonOK(w, map[string]any{
		"total_leads":        totalLeads,
		"hot_leads":          hotLeads,
		"total_buyers":       totalBuyers,
		"available_kavlings": availableKavlings,
		"pending_notifs":     pendingNotifs,
		"by_stage":           byStage,
	})
}

// ── Campaigns ─────────────────────────────────────────────────────────────────

func handleCheckSlug(w http.ResponseWriter, r *http.Request) {
	slug := r.URL.Query().Get("slug")
	excludeID := r.URL.Query().Get("exclude_id")
	if slug == "" {
		jsonError(w, "slug required", http.StatusBadRequest)
		return
	}
	exists, err := dbSlugExists(r.Context(), slug, excludeID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]bool{"exists": exists})
}

func handleListCampaigns(w http.ResponseWriter, r *http.Request) {
	list, err := dbListCampaigns(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if list == nil {
		list = []Campaign{}
	}
	jsonOK(w, list)
}

func handleGetCampaign(w http.ResponseWriter, r *http.Request) {
	c, err := dbGetCampaign(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	jsonOK(w, c)
}

func handleCreateCampaign(w http.ResponseWriter, r *http.Request) {
	var body Campaign
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Name == "" {
		jsonError(w, "name required", http.StatusBadRequest)
		return
	}
	if body.Slug == "" {
		body.Slug = slugify(body.Name)
	}
	if body.Status == "" {
		body.Status = "active"
	}
	if body.RedirectType == "" {
		body.RedirectType = "wa"
	}
	if body.Pixels == nil {
		body.Pixels = []Pixel{}
	}
	if body.ProductIDs == nil {
		body.ProductIDs = []string{}
	}
	id, err := dbCreateCampaign(r.Context(), &body)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, map[string]string{"id": id, "slug": body.Slug})
}

func handleUpdateCampaign(w http.ResponseWriter, r *http.Request) {
	existing, err := dbGetCampaign(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	var body Campaign
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Name != "" {
		existing.Name = body.Name
	}
	if body.Slug != "" {
		existing.Slug = body.Slug
	}
	existing.Description = body.Description
	existing.FormNote = body.FormNote
	existing.CustomScript = body.CustomScript
	existing.CustomHTML = body.CustomHTML
	existing.RedirectURL = body.RedirectURL
	if body.Status != "" {
		existing.Status = body.Status
	}
	if body.RedirectType != "" {
		existing.RedirectType = body.RedirectType
	}
	if body.Pixels != nil {
		existing.Pixels = body.Pixels
	}
	if body.ProductIDs != nil {
		existing.ProductIDs = body.ProductIDs
	}
	if err := dbUpdateCampaign(r.Context(), existing); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "updated"})
}

func handleDeleteCampaign(w http.ResponseWriter, r *http.Request) {
	if err := dbDeleteCampaign(r.Context(), r.PathValue("id")); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

// handlePublicGetCampaign — public endpoint, tanpa auth
func handlePublicGetCampaign(w http.ResponseWriter, r *http.Request) {
	c, err := dbGetCampaignBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		jsonError(w, "campaign not found", http.StatusNotFound)
		return
	}
	jsonOK(w, c)
}

// handlePublicSubmitCampaign — public form submit dari landing page
func handlePublicSubmitCampaign(w http.ResponseWriter, r *http.Request) {
	c, err := dbGetCampaignBySlug(r.Context(), r.PathValue("slug"))
	if err != nil {
		jsonError(w, "campaign not found", http.StatusNotFound)
		return
	}

	var body struct {
		Name     string `json:"name"`
		Phone    string `json:"phone"`
		Email    string `json:"email"`
		Interest string `json:"interest"`
		Notes    string `json:"notes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid json", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Name) == "" || strings.TrimSpace(body.Phone) == "" {
		jsonError(w, "name and phone required", http.StatusBadRequest)
		return
	}

	salesman := pickSalesmanName(r.Context())

	lead := &Lead{
		Name:       strings.TrimSpace(body.Name),
		Phone:      strings.TrimSpace(body.Phone),
		Email:      strings.TrimSpace(body.Email),
		Source:     "landing_page",
		Stage:      "new",
		Score:      50,
		Urgency:    "medium",
		Interest:   strings.TrimSpace(body.Interest),
		Notes:      strings.TrimSpace(body.Notes),
		AssignedTo: salesman,
		CampaignID: c.ID,
	}

	leadID, err := dbCreateLead(r.Context(), lead)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	dbIncrCampaignLeads(r.Context(), c.ID)
	scheduleDripNotifs(r.Context(), leadID, lead.Phone, lead.Name)

	// ambil phone salesman untuk WA redirect
	salesmanPhone := ""
	if sm, err := dbPickSalesman(r.Context()); err == nil && sm != nil {
		if sm.Name == salesman {
			salesmanPhone = sm.Phone
		}
	}
	if salesmanPhone == "" {
		// fallback: cari by name
		if list, err := dbListSalesmen(r.Context()); err == nil {
			for _, s := range list {
				if s.Name == salesman {
					salesmanPhone = s.Phone
					break
				}
			}
		}
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	jsonOK(w, map[string]string{
		"status":          "ok",
		"lead_id":         leadID,
		"assigned_to":     salesman,
		"salesman_phone":  salesmanPhone,
		"redirect_type":   c.RedirectType,
		"redirect_url":    c.RedirectURL,
	})
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == ' ' || r == '-' || r == '_' {
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
