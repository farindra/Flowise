package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// ── Helpers ──────────────────────────────────────────────────────────────────

func broadcastProvider(dryRun bool) Provider {
	var p Provider = newWhatsmeowProvider()
	if dryRun {
		p = &dryRunProvider{inner: p}
	}
	return p
}

// editableStatuses are the states in which a campaign's definition may change.
// Once it starts, the audience is frozen — otherwise "I tweaked the filter"
// silently turns into "it re-sent to everyone".
func isEditable(status string) bool {
	switch status {
	case BStatusDraft, BStatusReady, BStatusScheduled, BStatusPaused:
		return true
	}
	return false
}

// ── Config endpoints ─────────────────────────────────────────────────────────

// GET /api/broadcasts/throttle-defaults
func handleBroadcastThrottleDefaults(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]any{
		"defaults": defaultThrottle,
		"limits": map[string]any{
			"hard_daily_cap":       hardDailyCap,
			"min_delay_floor_sec":  minDelayFloorSec,
			"max_recipients":       maxRecipients,
			"max_media_bytes":      maxMediaBytesCRM(),
		},
	})
}

// GET /api/broadcasts/senders
func handleBroadcastSenders(w http.ResponseWriter, r *http.Request) {
	senders, err := broadcastProvider(false).Senders(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	jsonOK(w, senders)
}

// GET /api/broadcasts/chat-sources — WA sessions + their chatflow, for the picker
func handleBroadcastChatSources(w http.ResponseWriter, r *http.Request) {
	rows, err := dbWASessionChatflows(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, rows)
}

// GET /api/customers/wilayah
func handleCustomerWilayah(w http.ResponseWriter, r *http.Request) {
	rows, err := dbWilayahCounts(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, rows)
}

// ── Opt-outs ─────────────────────────────────────────────────────────────────

// GET /api/broadcasts/optouts
func handleListOptOuts(w http.ResponseWriter, r *http.Request) {
	rows, err := dbListOptOuts(r.Context())
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, rows)
}

// POST /api/broadcasts/optouts — called by go-wa when a customer sends STOP
func handleCreateOptOut(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Phone  string `json:"phone"`
		Reason string `json:"reason"`
		Note   string `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonError(w, "JSON tidak valid", http.StatusBadRequest)
		return
	}
	phone := normPhone(in.Phone)
	if phone == "" {
		jsonError(w, "phone wajib diisi", http.StatusBadRequest)
		return
	}
	if in.Reason == "" {
		in.Reason = "stop_keyword"
	}
	if err := dbAddOptOut(r.Context(), phone, in.Reason, in.Note); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "ok", "phone": phone})
}

// DELETE /api/broadcasts/optouts/{phone}
func handleDeleteOptOut(w http.ResponseWriter, r *http.Request) {
	phone := normPhone(r.PathValue("phone"))
	if err := dbRemoveOptOut(r.Context(), phone); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

// ── Audience preview & file upload ───────────────────────────────────────────

type broadcastInput struct {
	Name             string         `json:"name"`
	Provider         string         `json:"provider"`
	SenderID         string         `json:"sender_id"`
	MessageText      string         `json:"message_text"`
	MediaPath        string         `json:"media_path"`
	MediaMime        string         `json:"media_mime"`
	MediaMode        string         `json:"media_mode"`
	Audience         AudienceSpec   `json:"audience"`
	Throttle         map[string]any `json:"throttle"`
	IncludeBlacklist bool           `json:"include_blacklist"`
	AllPhones        bool           `json:"all_phones"`
	DryRun           bool           `json:"dry_run"`
	ScheduledAt      *time.Time     `json:"scheduled_at"`
}

func (in *broadcastInput) toBroadcast() *Broadcast {
	b := &Broadcast{
		Name:             strings.TrimSpace(in.Name),
		Provider:         in.Provider,
		SenderID:         in.SenderID,
		MessageText:      in.MessageText,
		MediaPath:        in.MediaPath,
		MediaMime:        in.MediaMime,
		MediaMode:        in.MediaMode,
		Audience:         in.Audience,
		Throttle:         in.Throttle,
		IncludeBlacklist: in.IncludeBlacklist,
		AllPhones:        in.AllPhones,
		DryRun:           in.DryRun,
		ScheduledAt:      in.ScheduledAt,
	}
	if b.Provider == "" {
		b.Provider = "whatsmeow"
	}
	if b.MediaMode == "" {
		b.MediaMode = "caption"
	}
	if b.Throttle == nil {
		b.Throttle = map[string]any{}
	}
	return b
}

// POST /api/broadcasts/preview-audience
func handlePreviewAudience(w http.ResponseWriter, r *http.Request) {
	var in broadcastInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonError(w, "JSON tidak valid", http.StatusBadRequest)
		return
	}
	b := in.toBroadcast()
	b.ID = "00000000-0000-0000-0000-000000000000" // no campaign yet; cooldown excludes nothing
	thr := mergeThrottle(b.Throttle)

	res, err := resolveAudience(r.Context(), b, broadcastProvider(b.DryRun), thr)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	jsonOK(w, res)
}

// POST /api/broadcasts/audience-file — multipart .xlsx/.csv, parsed inline
func handleAudienceFile(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		jsonError(w, "form tidak valid: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "file wajib diisi", http.StatusBadRequest)
		return
	}
	defer file.Close()

	name := strings.ToLower(hdr.Filename)
	var rows []UploadRow
	switch {
	case strings.HasSuffix(name, ".csv"):
		rows, err = parseAudienceCSV(file)
	case strings.HasSuffix(name, ".xlsx"):
		rows, err = parseAudienceXLSX(file)
	default:
		jsonError(w, "format tidak didukung — gunakan .xlsx atau .csv", http.StatusBadRequest)
		return
	}
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(rows) > maxRecipients {
		jsonError(w, fmt.Sprintf("maksimal %d baris, file berisi %d", maxRecipients, len(rows)),
			http.StatusBadRequest)
		return
	}

	// Report the extra columns back so the composer can offer them as variables.
	extraCols := map[string]bool{}
	for _, row := range rows {
		for k := range row.Vars {
			extraCols[k] = true
		}
	}
	vars := []string{}
	for k := range extraCols {
		vars = append(vars, k)
	}

	jsonOK(w, map[string]any{
		"rows":      rows,
		"count":     len(rows),
		"variables": vars,
	})
}

// headerIndex maps a normalized header name to its column position, and reports
// which columns are "extra" (i.e. become template variables).
func headerIndex(headers []string) (nameCol, phoneCol, wilayahCol int, extras map[int]string) {
	nameCol, phoneCol, wilayahCol = -1, -1, -1
	extras = map[int]string{}
	for i, h := range headers {
		key := strings.ToLower(strings.TrimSpace(h))
		switch key {
		case "nama", "name":
			nameCol = i
		case "no wa", "no_wa", "nowa", "phone", "no telepon", "no hp", "telepon":
			phoneCol = i
		case "wilayah", "kota", "region":
			wilayahCol = i
		default:
			if key != "" {
				extras[i] = strings.ReplaceAll(key, " ", "_")
			}
		}
	}
	return
}

func buildUploadRows(headers []string, records [][]string) ([]UploadRow, error) {
	nameCol, phoneCol, wilayahCol, extras := headerIndex(headers)
	if phoneCol < 0 {
		return nil, fmt.Errorf("kolom 'No WA' tidak ditemukan di baris header")
	}
	at := func(rec []string, i int) string {
		if i < 0 || i >= len(rec) {
			return ""
		}
		return strings.TrimSpace(rec[i])
	}

	out := []UploadRow{}
	for _, rec := range records {
		// One cell may hold several comma-separated numbers.
		for _, phone := range splitPhones(at(rec, phoneCol)) {
			row := UploadRow{
				Name:    at(rec, nameCol),
				Phone:   phone,
				Wilayah: at(rec, wilayahCol),
				Vars:    map[string]string{},
			}
			for idx, varName := range extras {
				if v := at(rec, idx); v != "" {
					row.Vars[varName] = v
				}
			}
			out = append(out, row)
		}
	}
	return out, nil
}

func parseAudienceCSV(rd io.Reader) ([]UploadRow, error) {
	cr := csv.NewReader(rd)
	cr.FieldsPerRecord = -1
	records, err := cr.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("gagal baca CSV: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("file harus punya baris header dan minimal 1 baris data")
	}
	return buildUploadRows(records[0], records[1:])
}

func parseAudienceXLSX(rd io.Reader) ([]UploadRow, error) {
	f, err := excelize.OpenReader(rd)
	if err != nil {
		return nil, fmt.Errorf("gagal buka file Excel: %w", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("file Excel tidak punya sheet")
	}
	records, err := f.GetRows(sheets[0])
	if err != nil {
		return nil, fmt.Errorf("gagal baca sheet: %w", err)
	}
	if len(records) < 2 {
		return nil, fmt.Errorf("file harus punya baris header dan minimal 1 baris data")
	}
	return buildUploadRows(records[0], records[1:])
}

// GET /api/broadcasts/template
func handleBroadcastTemplate(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()
	const sheet = "Broadcast"
	idx, err := f.NewSheet(sheet)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	f.SetActiveSheet(idx)
	f.DeleteSheet("Sheet1")

	headers := []string{"Nama", "No WA", "Wilayah", "produk"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	_ = f.SetCellValue(sheet, "A2", "Budi Santoso")
	_ = f.SetCellValue(sheet, "B2", "081234567890")
	_ = f.SetCellValue(sheet, "C2", "Surabaya")
	_ = f.SetCellValue(sheet, "D2", "Bearing 6205")
	_ = f.SetColWidth(sheet, "A", "D", 22)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="template-broadcast.xlsx"`)
	if err := f.Write(w); err != nil {
		fmt.Printf("[broadcast] write template: %v\n", err)
	}
}

// ── Media ────────────────────────────────────────────────────────────────────

func maxMediaBytesCRM() int {
	return envInt("BROADCAST_MAX_MEDIA_BYTES", 5<<20)
}

var allowedBroadcastMimes = map[string]string{
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

// POST /api/broadcasts/media
func handleBroadcastMediaUpload(w http.ResponseWriter, r *http.Request) {
	limit := maxMediaBytesCRM()
	if err := r.ParseMultipartForm(int64(limit) + (1 << 20)); err != nil {
		jsonError(w, "form tidak valid: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, hdr, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "file wajib diisi", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		jsonError(w, "gagal baca file", http.StatusBadRequest)
		return
	}
	if len(data) > limit {
		jsonError(w, fmt.Sprintf("file terlalu besar (maks %d MB)", limit>>20), http.StatusBadRequest)
		return
	}

	mime := hdr.Header.Get("Content-Type")
	if mime == "" {
		mime = http.DetectContentType(data)
	}
	ext, ok := allowedBroadcastMimes[mime]
	if !ok {
		jsonError(w, "tipe file tidak didukung: "+mime+" (hanya jpeg, png, webp)", http.StatusBadRequest)
		return
	}

	dir := mediaDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		jsonError(w, "gagal siapkan folder media: "+err.Error(), http.StatusInternalServerError)
		return
	}
	fname := newJobID() + ext
	if err := os.WriteFile(filepath.Join(dir, fname), data, 0o644); err != nil {
		jsonError(w, "gagal simpan media: "+err.Error(), http.StatusInternalServerError)
		return
	}

	jsonOK(w, map[string]any{"media_path": fname, "media_mime": mime, "size": len(data)})
}

// GET /api/broadcasts/media/{name} — authenticated; filepath.Base guards traversal
func handleBroadcastMediaGet(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(r.PathValue("name"))
	path := filepath.Join(mediaDir(), name)
	data, err := os.ReadFile(path)
	if err != nil {
		jsonError(w, "media tidak ditemukan", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", http.DetectContentType(data))
	_, _ = w.Write(data)
}

// ── Campaign CRUD ────────────────────────────────────────────────────────────

// GET /api/broadcasts
func handleListBroadcasts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	status := r.URL.Query().Get("status")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 25
	}
	items, total, err := dbListBroadcasts(r.Context(), q, status, page, limit)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"items": items, "total": total})
}

// GET /api/broadcasts/{id}
func handleGetBroadcast(w http.ResponseWriter, r *http.Request) {
	b, err := dbGetBroadcast(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonError(w, "broadcast tidak ditemukan", http.StatusNotFound)
		return
	}
	// `throttle` stays exactly as stored (just the user's overrides) so the
	// edit form can be re-opened without silently discarding them.
	// `effective_throttle` is what will actually run once merged with the
	// current server defaults, for read-only display.
	resp := map[string]any{}
	raw, _ := json.Marshal(b)
	_ = json.Unmarshal(raw, &resp)
	resp["effective_throttle"] = throttleToMap(mergeThrottle(b.Throttle))
	jsonOK(w, resp)
}

// throttleToMap flattens a Throttle into a plain map for JSON responses.
func throttleToMap(t Throttle) map[string]any {
	raw, _ := json.Marshal(t)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}

func validateBroadcastInput(in *broadcastInput) error {
	if strings.TrimSpace(in.Name) == "" {
		return fmt.Errorf("nama campaign wajib diisi")
	}
	if strings.TrimSpace(in.MessageText) == "" && in.MediaPath == "" {
		return fmt.Errorf("pesan atau gambar wajib diisi")
	}
	if in.SenderID == "" {
		return fmt.Errorf("pengirim wajib dipilih")
	}
	if in.MediaMode != "" && in.MediaMode != "caption" && in.MediaMode != "separate" {
		return fmt.Errorf("mode media harus 'caption' atau 'separate'")
	}
	return validateThrottleOverrides(in.Throttle)
}

// POST /api/broadcasts
func handleCreateBroadcast(w http.ResponseWriter, r *http.Request) {
	var in broadcastInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonError(w, "JSON tidak valid", http.StatusBadRequest)
		return
	}
	if err := validateBroadcastInput(&in); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	b := in.toBroadcast()
	b.Status = BStatusDraft
	id, err := dbCreateBroadcast(r.Context(), b)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, map[string]string{"id": id})
}

// PUT /api/broadcasts/{id}
func handleUpdateBroadcast(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := dbGetBroadcast(r.Context(), id)
	if err != nil {
		jsonError(w, "broadcast tidak ditemukan", http.StatusNotFound)
		return
	}
	if !isEditable(existing.Status) {
		jsonError(w, "campaign sedang "+existing.Status+" — tidak bisa diubah", http.StatusConflict)
		return
	}
	var in broadcastInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonError(w, "JSON tidak valid", http.StatusBadRequest)
		return
	}
	if err := validateBroadcastInput(&in); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	b := in.toBroadcast()
	b.ID = id
	if err := dbUpdateBroadcast(r.Context(), b); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "updated"})
}

// DELETE /api/broadcasts/{id}
func handleDeleteBroadcast(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, err := dbGetBroadcast(r.Context(), id)
	if err != nil {
		jsonError(w, "broadcast tidak ditemukan", http.StatusNotFound)
		return
	}
	if b.Status == BStatusRunning {
		jsonError(w, "campaign sedang berjalan — hentikan dulu sebelum dihapus", http.StatusConflict)
		return
	}
	if err := dbDeleteBroadcast(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

// ── Audience build (async job, mirrors the Excel-import pattern) ──────────────

// POST /api/broadcasts/{id}/audience
func handleBuildAudience(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, err := dbGetBroadcast(r.Context(), id)
	if err != nil {
		jsonError(w, "broadcast tidak ditemukan", http.StatusNotFound)
		return
	}
	if !isEditable(b.Status) {
		jsonError(w, "campaign sedang "+b.Status+" — audiens tidak bisa dibangun ulang",
			http.StatusConflict)
		return
	}

	jobID := newJobID()
	job := &ImportJob{Status: "processing"}
	importJobs.Lock()
	importJobs.m[jobID] = job
	importJobs.Unlock()

	go func() {
		// Detached from the request: the client is already gone.
		ctx := context.Background()
		thr := mergeThrottle(b.Throttle)
		res, err := resolveAudience(ctx, b, broadcastProvider(b.DryRun), thr)
		job.mu.Lock()
		defer job.mu.Unlock()
		defer func() { job.Status = "done" }()
		if err != nil {
			job.Errors = append(job.Errors, map[string]any{"error": err.Error()})
			return
		}
		if err := dbReplaceRecipients(ctx, id, res.Recipients); err != nil {
			job.Errors = append(job.Errors, map[string]any{"error": err.Error()})
			return
		}
		job.Total = len(res.Recipients)
		job.Processed = len(res.Recipients)
		job.Created = res.Total
		_ = dbSetBroadcastStatus(ctx, id, BStatusReady)
	}()

	w.WriteHeader(http.StatusAccepted)
	jsonOK(w, map[string]any{"job_id": jobID})
}

// GET /api/broadcasts/{id}/audience/{jobId}
func handleBuildAudienceStatus(w http.ResponseWriter, r *http.Request) {
	importJobs.Lock()
	job := importJobs.m[r.PathValue("jobId")]
	importJobs.Unlock()
	if job == nil {
		jsonError(w, "job tidak ditemukan (mungkin sudah lama, atau go-crm sempat restart)",
			http.StatusNotFound)
		return
	}
	jsonOK(w, job.snapshot())
}

// GET /api/broadcasts/{id}/recipients
func handleListBroadcastRecipients(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit == 0 {
		limit = 50
	}
	items, total, err := dbListRecipients(r.Context(), r.PathValue("id"),
		r.URL.Query().Get("status"), r.URL.Query().Get("q"), page, limit)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"items": items, "total": total})
}

// GET /api/broadcasts/{id}/export
func handleExportBroadcastRecipients(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	items, _, err := dbListRecipients(r.Context(), id, "", "", 0, 0)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	f := excelize.NewFile()
	defer f.Close()
	const sheet = "Hasil"
	idx, err := f.NewSheet(sheet)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	f.SetActiveSheet(idx)
	f.DeleteSheet("Sheet1")

	headers := []string{"Nama", "No WA", "Wilayah", "Tier", "Sumber", "Status", "Alasan Dilewati",
		"Percobaan", "Error", "Message ID", "Waktu Kirim"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	for i, it := range items {
		row := i + 2
		sentAt := ""
		if it.SentAt != nil {
			sentAt = it.SentAt.Format("2006-01-02 15:04:05")
		}
		vals := []any{it.Name, it.Phone, it.Wilayah, it.Tier, it.Source, it.Status,
			it.SkipReason, it.Attempts, it.LastError, it.ProviderMsgID, sentAt}
		for c, v := range vals {
			cell, _ := excelize.CoordinatesToCellName(c+1, row)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}
	_ = f.SetColWidth(sheet, "A", "K", 18)

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="broadcast-%s.xlsx"`, id[:8]))
	if err := f.Write(w); err != nil {
		fmt.Printf("[broadcast] write export: %v\n", err)
	}
}

// ── Controls ─────────────────────────────────────────────────────────────────

// POST /api/broadcasts/{id}/test-send — bypasses the queue entirely
func handleBroadcastTestSend(w http.ResponseWriter, r *http.Request) {
	b, err := dbGetBroadcast(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonError(w, "broadcast tidak ditemukan", http.StatusNotFound)
		return
	}
	var in struct {
		Phone string `json:"phone"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonError(w, "JSON tidak valid", http.StatusBadRequest)
		return
	}
	phone := normPhone(in.Phone)
	if !validWAPhone(phone) {
		jsonError(w, "nomor tidak valid (contoh: 081234567890)", http.StatusBadRequest)
		return
	}

	// Sample variables so the tester sees the template rendered, not raw.
	vars := map[string]string{"nama": "Test", "wilayah": "Test", "tier": "normal"}
	if len(b.Audience.Upload) > 0 {
		for k, v := range b.Audience.Upload[0].Vars {
			vars[k] = v
		}
	}

	msg := OutboundMessage{
		Text:      renderTemplate(b.MessageText, vars),
		MediaMime: b.MediaMime,
		Separate:  b.MediaMode == "separate",
	}
	if b.MediaPath != "" {
		data, err := os.ReadFile(filepath.Join(mediaDir(), filepath.Base(b.MediaPath)))
		if err != nil {
			jsonError(w, "gagal baca media: "+err.Error(), http.StatusInternalServerError)
			return
		}
		msg.Media = data
		msg.MediaName = filepath.Base(b.MediaPath)
	}

	res, err := broadcastProvider(b.DryRun).Send(r.Context(), b.SenderID, phone, msg)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadGateway)
		return
	}
	jsonOK(w, map[string]any{"status": "sent", "message_id": res.ProviderMsgID, "preview": msg.Text})
}

// POST /api/broadcasts/{id}/start
func handleStartBroadcast(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, err := dbGetBroadcast(r.Context(), id)
	if err != nil {
		jsonError(w, "broadcast tidak ditemukan", http.StatusNotFound)
		return
	}
	switch b.Status {
	case BStatusRunning:
		jsonError(w, "campaign sudah berjalan", http.StatusConflict)
		return
	case BStatusDone, BStatusCancelled:
		jsonError(w, "campaign sudah selesai", http.StatusConflict)
		return
	}

	pending, err := dbPendingRecipientCount(r.Context(), id)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if pending == 0 {
		jsonError(w, "audiens kosong — bangun audiens dulu", http.StatusBadRequest)
		return
	}
	if err := broadcastProvider(b.DryRun).Ready(r.Context(), b.SenderID); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	// A future scheduled_at parks the campaign; the worker promotes it when due.
	if b.ScheduledAt != nil && b.ScheduledAt.After(time.Now()) {
		if err := dbSetBroadcastStatus(r.Context(), id, BStatusScheduled); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]any{"status": BStatusScheduled, "scheduled_at": b.ScheduledAt})
		return
	}
	if err := dbStartBroadcast(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"status": BStatusRunning, "pending": pending})
}

// POST /api/broadcasts/{id}/pause
func handlePauseBroadcast(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	status, err := dbBroadcastStatus(r.Context(), id)
	if err != nil {
		jsonError(w, "broadcast tidak ditemukan", http.StatusNotFound)
		return
	}
	if status != BStatusRunning && status != BStatusScheduled {
		jsonError(w, "campaign tidak sedang berjalan", http.StatusConflict)
		return
	}
	if err := dbSetBroadcastStatus(r.Context(), id, BStatusPaused); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": BStatusPaused})
}

// POST /api/broadcasts/{id}/resume
func handleResumeBroadcast(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	b, err := dbGetBroadcast(r.Context(), id)
	if err != nil {
		jsonError(w, "broadcast tidak ditemukan", http.StatusNotFound)
		return
	}
	if b.Status != BStatusPaused {
		jsonError(w, "campaign tidak sedang dipause", http.StatusConflict)
		return
	}
	if err := broadcastProvider(b.DryRun).Ready(r.Context(), b.SenderID); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := dbStartBroadcast(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": BStatusRunning})
}

// POST /api/broadcasts/{id}/cancel
func handleCancelBroadcast(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := dbBroadcastStatus(r.Context(), id); err != nil {
		jsonError(w, "broadcast tidak ditemukan", http.StatusNotFound)
		return
	}
	if err := dbSetBroadcastStatus(r.Context(), id, BStatusCancelled); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := dbCancelPendingRecipients(r.Context(), id); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": BStatusCancelled})
}

// POST /api/broadcasts/{id}/retry — {"scope":"failed"|"uncertain"}
func handleRetryBroadcast(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in struct {
		Scope string `json:"scope"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		jsonError(w, "JSON tidak valid", http.StatusBadRequest)
		return
	}
	n, err := dbRetryRecipients(r.Context(), id, in.Scope)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Retrying a finished campaign puts it back to ready so it can be started.
	if status, err := dbBroadcastStatus(r.Context(), id); err == nil && status == BStatusDone && n > 0 {
		_ = dbSetBroadcastStatus(r.Context(), id, BStatusReady)
	}
	jsonOK(w, map[string]any{"status": "ok", "requeued": n})
}
