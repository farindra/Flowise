package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xuri/excelize/v2"
)

func newJobID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

const customerSheet = "Customers"
const maxImportRows = 20000
const importBatchSize = 200

var customerHeaders = []string{"Nama", "No WA", "Wilayah", "Flag (normal/vip/blacklist)", "Persen (%)", "Catatan"}

func writeCustomerHeaderRow(f *excelize.File) {
	for i, h := range customerHeaders {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(customerSheet, cell, h)
	}
	style, _ := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	f.SetRowStyle(customerSheet, 1, 1, style)
	f.SetColWidth(customerSheet, "A", "A", 28)
	f.SetColWidth(customerSheet, "B", "B", 30)
	f.SetColWidth(customerSheet, "C", "C", 18)
	f.SetColWidth(customerSheet, "D", "D", 22)
	f.SetColWidth(customerSheet, "E", "E", 12)
	f.SetColWidth(customerSheet, "F", "F", 30)
}

// handleCustomerTemplate — downloadable .xlsx with headers + one example row.
func handleCustomerTemplate(w http.ResponseWriter, r *http.Request) {
	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", customerSheet)
	writeCustomerHeaderRow(f)
	f.SetSheetRow(customerSheet, "A2", &[]any{
		"Jitu Bearing (JB SBY)", "6285233639611, 085233639611", "Surabaya", "vip", 2.5, "Contoh baris — boleh dihapus",
	})

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="template-customers.xlsx"`)
	if err := f.Write(w); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleCustomerExport — downloadable .xlsx of all current customers
// (respects ?q= the same way the list endpoint does).
func handleCustomerExport(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	customers, _, err := dbListCustomers(r.Context(), q, "", 0, 0)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	f := excelize.NewFile()
	defer f.Close()
	f.SetSheetName("Sheet1", customerSheet)
	writeCustomerHeaderRow(f)
	for i, c := range customers {
		row := i + 2
		f.SetSheetRow(customerSheet, fmt.Sprintf("A%d", row), &[]any{
			c.Name, strings.Join(c.Phone, ", "), c.Wilayah, c.Tier, adjToPct(c.Tier, c.Adj), c.Notes,
		})
	}

	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="customers.xlsx"`)
	if err := f.Write(w); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
	}
}

// adjToPct undoes the sign convention (vip=negative, blacklist=positive) so
// the exported sheet shows a plain, always-positive percentage — matching
// what the template/import expect back.
func adjToPct(tier string, adj float64) float64 {
	if adj < 0 {
		return -adj
	}
	return adj
}

func pctToAdj(tier string, pct float64) float64 {
	pct = adjToPct(tier, pct) // normalize to positive regardless of how it was typed
	switch tier {
	case "vip":
		return -pct
	case "blacklist":
		return pct
	default:
		return 0
	}
}

// ── Background import jobs ──────────────────────────────────────────────────
//
// Import runs as a background job instead of inline in the request: a large
// sheet processed synchronously risks HTTP client/gateway timeouts and gives
// no progress feedback. The job lives in memory only (this is a single-
// process service) — if go-crm restarts mid-import, the job is lost and the
// upload must be retried; acceptable for an admin bulk-edit tool.

type ImportJob struct {
	mu        sync.Mutex
	Status    string `json:"status"` // "processing" | "done"
	Total     int    `json:"total"`
	Processed int    `json:"processed"`
	Created   int    `json:"created"`
	Updated   int    `json:"updated"`
	Errors    []map[string]any `json:"errors"`
}

func (j *ImportJob) snapshot() map[string]any {
	j.mu.Lock()
	defer j.mu.Unlock()
	errs := j.Errors
	if errs == nil {
		errs = []map[string]any{}
	}
	return map[string]any{
		"status":    j.Status,
		"total":     j.Total,
		"processed": j.Processed,
		"created":   j.Created,
		"updated":   j.Updated,
		"errors":    errs,
	}
}

var importJobs = struct {
	sync.Mutex
	m map[string]*ImportJob
}{m: map[string]*ImportJob{}}

type importRow struct {
	rowNum int
	name   string
	phones []string
	wilayah string
	tier    string
	adj     float64
	notes   string
}

func parseImportRow(rowNum int, row []string) *importRow {
	get := func(idx int) string {
		if idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
		return ""
	}
	name := get(0)
	if name == "" {
		return nil // blank trailing row
	}
	tier := strings.ToLower(get(3))
	if tier != "vip" && tier != "blacklist" && tier != "normal" {
		// "normal" (adj forced to 0) must be typed explicitly in the sheet to
		// count — a blank/unrecognized cell defaults to unregistered (gets
		// the markup), not silently exempt from it.
		tier = TierUnregistered
	}
	pct, _ := strconv.ParseFloat(strings.TrimSuffix(get(4), "%"), 64)
	return &importRow{
		rowNum:  rowNum,
		name:    name,
		phones:  splitPhones(get(1)),
		wilayah: get(2),
		tier:    tier,
		adj:     pctToAdj(tier, pct),
		notes:   get(5),
	}
}

// runImportJob processes rows in batches: one query resolves matches for an
// entire batch (instead of one SELECT per row), then writes happen
// sequentially — no goroutine fan-out, so no race window between two rows
// that reference the same phone number.
// targetRowKey mirrors the grouping key built in runImportJob so a single
// row's resolved target can be looked back up in the targetRows map.
func targetRowKey(m []CustomerMatch, r *importRow) string {
	if len(m) == 1 {
		return "id:" + m[0].ID
	}
	if len(m) == 0 && len(r.phones) > 0 {
		return "new:" + strings.Join(r.phones, ",")
	}
	return ""
}

func runImportJob(job *ImportJob, rows []*importRow) {
	ctx := context.Background()
	for start := 0; start < len(rows); start += importBatchSize {
		end := start + importBatchSize
		if end > len(rows) {
			end = len(rows)
		}
		batch := rows[start:end]

		phonesByIdx := map[int][]string{}
		for i, r := range batch {
			if len(r.phones) > 0 {
				phonesByIdx[i] = r.phones
			}
		}
		matches, err := dbBatchFindMatches(ctx, phonesByIdx)

		// Detect two rows *within this same file* that would resolve to the
		// same target — either both matching the same existing customer, or
		// (with no existing match) both trying to create a customer with the
		// same phone. Batch-resolving matches up front means a naive
		// sequential apply would silently let the later row overwrite the
		// earlier one with no error; group by resolved target instead and
		// flag every row in a group of 2+ as a conflict.
		targetRows := map[string][]int{}
		if err == nil {
			for i, r := range batch {
				if key := targetRowKey(matches[i], r); key != "" {
					targetRows[key] = append(targetRows[key], i)
				}
				// len(matches[i]) > 1 is left ungrouped — that's already
				// handled as a conflict by dbUpsertCustomerWithMatches.
			}
		}

		job.mu.Lock()
		for i, r := range batch {
			if err != nil {
				job.Errors = append(job.Errors, map[string]any{"row": r.rowNum, "name": r.name, "error": "lookup batch gagal: " + err.Error()})
				job.Processed++
				continue
			}
			if key := targetRowKey(matches[i], r); key != "" && len(targetRows[key]) > 1 {
				otherRows := make([]int, 0, len(targetRows[key])-1)
				for _, ri := range targetRows[key] {
					if ri != i {
						otherRows = append(otherRows, batch[ri].rowNum)
					}
				}
				job.Errors = append(job.Errors, map[string]any{
					"row": r.rowNum, "name": r.name,
					"error": fmt.Sprintf("nomor HP sama dengan baris %v di file ini — gabungkan jadi 1 baris sebelum import ulang", otherRows),
				})
				job.Processed++
				continue
			}
			c := &Customer{Name: r.name, Phone: r.phones, Wilayah: r.wilayah, Tier: r.tier, Adj: r.adj, Notes: r.notes}
			wasCreated, upsertErr := dbUpsertCustomerWithMatches(ctx, c, matches[i])
			if upsertErr != nil {
				job.Errors = append(job.Errors, map[string]any{"row": r.rowNum, "name": r.name, "error": upsertErr.Error()})
			} else if wasCreated {
				job.Created++
			} else {
				job.Updated++
			}
			job.Processed++
		}
		job.mu.Unlock()
	}

	job.mu.Lock()
	job.Status = "done"
	job.mu.Unlock()
}

// handleCustomerImport — sync import: multipart file upload (.xlsx). Each
// row is matched against existing customers by phone-number overlap; a
// match gets updated in place, otherwise a new customer is created.
// Processing happens in a background job — this handler just validates the
// file and returns a job id to poll.
func handleCustomerImport(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		jsonError(w, "invalid upload: "+err.Error(), http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		jsonError(w, "field 'file' required", http.StatusBadRequest)
		return
	}
	defer file.Close()

	f, err := excelize.OpenReader(file)
	if err != nil {
		jsonError(w, "file bukan .xlsx yang valid: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer f.Close()

	sheet := f.GetSheetList()[0]
	rawRows, err := f.GetRows(sheet)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if len(rawRows) > 0 {
		rawRows = rawRows[1:] // drop header
	}
	if len(rawRows) > maxImportRows {
		jsonError(w, fmt.Sprintf("maksimal %d baris per import (file ini %d baris) — pecah jadi beberapa file", maxImportRows, len(rawRows)), http.StatusBadRequest)
		return
	}

	var rows []*importRow
	for i, raw := range rawRows {
		if parsed := parseImportRow(i+2, raw); parsed != nil { // +2: 1-indexed + header row
			rows = append(rows, parsed)
		}
	}

	job := &ImportJob{Status: "processing", Total: len(rows)}
	jobID := newJobID()
	importJobs.Lock()
	importJobs.m[jobID] = job
	importJobs.Unlock()

	go runImportJob(job, rows)

	w.WriteHeader(http.StatusAccepted)
	jsonOK(w, map[string]any{"job_id": jobID})
}

// handleCustomerImportStatus — poll the progress/result of a background import job.
func handleCustomerImportStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("jobId")
	importJobs.Lock()
	job, ok := importJobs.m[jobID]
	importJobs.Unlock()
	if !ok {
		jsonError(w, "job tidak ditemukan (mungkin sudah lama, atau go-crm sempat restart)", http.StatusNotFound)
		return
	}
	jsonOK(w, job.snapshot())
}

// pruneOldImportJobs — called periodically so finished jobs don't accumulate
// in memory forever on a long-running process.
func pruneOldImportJobs() {
	importJobs.Lock()
	defer importJobs.Unlock()
	for id, job := range importJobs.m {
		job.mu.Lock()
		done := job.Status == "done"
		job.mu.Unlock()
		if done {
			delete(importJobs.m, id)
		}
	}
}

func startImportJobJanitor() {
	go func() {
		ticker := time.NewTicker(1 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			pruneOldImportJobs()
		}
	}()
}
