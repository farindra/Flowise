package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

// ── Pending upload state ──────────────────────────────────────────────────────

type pendingUpload struct {
	FileData []byte
	FileName string
	At       time.Time
	Step     string // "intent" | "supplier_info" | "trade_confirm" | "permintaan_confirm"
}

var (
	pendingUploadsMu sync.Mutex
	pendingUploads   = map[int64]*pendingUpload{}
)

func setPendingUpload(chatID int64, p *pendingUpload) {
	pendingUploadsMu.Lock()
	pendingUploads[chatID] = p
	pendingUploadsMu.Unlock()
}

func takePendingUpload(chatID int64) *pendingUpload {
	pendingUploadsMu.Lock()
	defer pendingUploadsMu.Unlock()
	p := pendingUploads[chatID]
	delete(pendingUploads, chatID)
	return p
}

// ── Intent helpers ────────────────────────────────────────────────────────────

func detectFileIntent(fname string) string {
	lower := strings.ToLower(fname)
	for _, k := range []string{"quote", "penawaran", "harga", "price", "offer", "supplier"} {
		if strings.Contains(lower, k) {
			return "supplier"
		}
	}
	for _, k := range []string{"trade", "perdagangan", "transaksi", "sales", "penjualan"} {
		if strings.Contains(lower, k) {
			return "trade"
		}
	}
	for _, k := range []string{"request", "permintaan", "indent", "po ", "purchase"} {
		if strings.Contains(lower, k) {
			return "permintaan"
		}
	}
	return ""
}

func intentLabel(intent string) string {
	switch intent {
	case "supplier":
		return "Penawaran Supplier"
	case "trade":
		return "Data Perdagangan"
	case "permintaan":
		return "Permintaan Barang"
	}
	return ""
}

func parseFileIntent(reply string) string {
	lower := strings.TrimSpace(strings.ToLower(reply))
	for _, k := range []string{"1", "penawaran", "supplier", "harga", "price", "quote", "offer"} {
		if lower == k || strings.Contains(lower, k) {
			return "supplier"
		}
	}
	for _, k := range []string{"2", "perdagangan", "trade", "transaksi", "sales"} {
		if lower == k || strings.Contains(lower, k) {
			return "trade"
		}
	}
	for _, k := range []string{"3", "permintaan", "request", "indent", "purchase"} {
		if lower == k || strings.Contains(lower, k) {
			return "permintaan"
		}
	}
	return ""
}

var (
	reSup = regexp.MustCompile(`(?i)supplier\s*:\s*([^,\n]+)`)
	reCur = regexp.MustCompile(`(?i)currency\s*:\s*([A-Za-z]{3})`)
)

func parseSupplierReply(text string) (supplierName, currency string, ok bool) {
	t := strings.TrimSpace(text)
	lower := strings.ToLower(t)
	if lower == "skip" || lower == "auto" || lower == "-" {
		return "", "USD", true
	}
	mSup := reSup.FindStringSubmatch(t)
	mCur := reCur.FindStringSubmatch(t)
	if mSup == nil && mCur == nil {
		return "", "", false
	}
	if mSup != nil {
		supplierName = strings.TrimSpace(mSup[1])
	}
	if mCur != nil {
		currency = strings.ToUpper(strings.TrimSpace(mCur[1]))
	} else {
		currency = "USD"
	}
	return supplierName, currency, true
}

func escapeMarkdown(s string) string {
	r := strings.NewReplacer(
		"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]",
		"(", "\\(", ")", "\\)", "~", "\\~", "`", "\\`",
		">", "\\>", "#", "\\#", "+", "\\+", "-", "\\-",
		"=", "\\=", "|", "\\|", "{", "\\{", "}", "\\}",
		".", "\\.", "!", "\\!",
	)
	return r.Replace(s)
}

// ── Pending upload reply dispatcher ──────────────────────────────────────────

func (b *Bot) handlePendingUploadReply(chatID int64, text string, p *pendingUpload) {
	if time.Since(p.At) > 10*time.Minute {
		b.sendText(chatID, "⏰ Upload expired. Kirim ulang file Excel-nya.")
		return
	}
	switch p.Step {
	case "intent":
		intent := parseFileIntent(text)
		switch intent {
		case "supplier":
			p.Step = "supplier_info"
			setPendingUpload(chatID, p)
			b.sendText(chatID, "✅ Penawaran Supplier\\.\n\nBalas dengan format:\n`SUPPLIER: <nama supplier>, CURRENCY: <USD/IDR/SGD/JPY/EUR>`\n\nAtau ketik `skip` untuk auto\\-detect dari file\\.")
		case "trade":
			go b.doTradeDataPreview(chatID, p)
		case "permintaan":
			go b.doUnrecordedItemsPreview(chatID, p)
		default:
			setPendingUpload(chatID, p)
			b.sendText(chatID, "❓ Pilihan tidak dikenali\\. Balas:\n*1* \\- Penawaran Supplier\n*2* \\- Data Perdagangan\n*3* \\- Permintaan Barang")
		}
	case "supplier_info":
		supplierName, currency, ok := parseSupplierReply(text)
		if !ok {
			setPendingUpload(chatID, p)
			b.sendText(chatID, "❓ Format tidak dikenali\\. Balas: `SUPPLIER: <nama>, CURRENCY: <USD/IDR/JPY/dll>` atau `skip`\\.")
			return
		}
		go b.doUploadSupplierOffer(chatID, p, supplierName, currency)
	case "trade_confirm":
		lower := strings.ToLower(strings.TrimSpace(text))
		if lower == "ya" || lower == "yes" || lower == "lanjut" || lower == "ok" || lower == "import" {
			go b.doTradeDataImport(chatID, p)
		} else if lower == "batal" || lower == "cancel" || lower == "tidak" || lower == "no" {
			b.sendText(chatID, "❌ Import dibatalkan\\.")
		} else {
			setPendingUpload(chatID, p)
			b.sendText(chatID, "❓ Balas *ya* untuk import atau *batal* untuk membatalkan\\.")
		}
	case "permintaan_confirm":
		lower := strings.ToLower(strings.TrimSpace(text))
		if lower == "ya" || lower == "yes" || lower == "lanjut" || lower == "ok" || lower == "import" {
			go b.doUnrecordedItemsImport(chatID, p)
		} else if lower == "batal" || lower == "cancel" || lower == "tidak" || lower == "no" {
			b.sendText(chatID, "❌ Import dibatalkan\\.")
		} else {
			setPendingUpload(chatID, p)
			b.sendText(chatID, "❓ Balas *ya* untuk import atau *batal* untuk membatalkan\\.")
		}
	}
}

// ── Document message handler ──────────────────────────────────────────────────

func (b *Bot) processDocumentMessage(msg *Message) {
	chatID := msg.Chat.ID
	doc := msg.Document

	if len(b.ownerIDs) > 0 && (msg.From == nil || !b.ownerIDs[msg.From.ID]) {
		b.sendText(chatID, "❌ Hanya owner yang bisa upload file.")
		return
	}

	tradeURL := os.Getenv("TRADE_URL")
	tradeBotKey := os.Getenv("TRADE_BOT_API_KEY")
	if tradeURL == "" || tradeBotKey == "" {
		b.sendText(chatID, "⚠️ TRADE_URL atau TRADE_BOT_API_KEY belum dikonfigurasi.")
		return
	}

	b.sendText(chatID, "⏳ Mengunduh file dari Telegram...")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	raw, err := b.telegramAPIWithResult("getFile", map[string]string{"file_id": doc.FileID})
	if err != nil {
		b.sendText(chatID, "❌ Gagal mengambil file dari Telegram.")
		return
	}
	var gfResp GetFileResponse
	if err := jsonUnmarshal(raw, &gfResp); err != nil || gfResp.Result.FilePath == "" {
		b.sendText(chatID, "❌ Respons getFile tidak valid.")
		return
	}

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.token, gfResp.Result.FilePath)
	dlReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	dlResp, err := b.tgClient.Do(dlReq)
	if err != nil {
		b.sendText(chatID, "❌ Gagal mengunduh file.")
		return
	}
	defer dlResp.Body.Close()
	fileData, _ := io.ReadAll(dlResp.Body)

	fname := doc.FileName
	if fname == "" {
		fname = "upload.xlsx"
	}

	setPendingUpload(chatID, &pendingUpload{FileData: fileData, FileName: fname, At: time.Now(), Step: "intent"})

	hint := detectFileIntent(fname)
	var prompt string
	if hint != "" {
		prompt = fmt.Sprintf("📄 File *%s* (%.1f KB) terdeteksi sebagai *%s*.\n\nBalas untuk konfirmasi:\n*1* \\- Penawaran Supplier\n*2* \\- Data Perdagangan\n*3* \\- Permintaan Barang",
			fname, float64(len(fileData))/1024, intentLabel(hint))
	} else {
		prompt = fmt.Sprintf("📄 File *%s* (%.1f KB) diterima\\.\n\nFile ini untuk apa?\n*1* \\- Penawaran Supplier\n*2* \\- Data Perdagangan\n*3* \\- Permintaan Barang",
			fname, float64(len(fileData))/1024)
	}
	b.sendText(chatID, prompt)
}

// ── TRADE upload helpers ──────────────────────────────────────────────────────

func tradePost(ctx context.Context, url, apiKey string, buf *bytes.Buffer, contentType string) ([]byte, int, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, buf)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("X-API-Key", apiKey)
	resp, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return raw, resp.StatusCode, nil
}

type tradePreviewResponse struct {
	TotalRows       int      `json:"total_rows"`
	ValidRows       int      `json:"valid_rows"`
	InvalidRows     int      `json:"invalid_rows"`
	MissingRequired []string `json:"missing_required"`
	HasPrice        bool     `json:"has_price"`
	Errors          []struct {
		Row    int      `json:"row"`
		Errors []string `json:"errors"`
	} `json:"errors"`
}

func (b *Bot) doTradeDataPreview(chatID int64, p *pendingUpload) {
	tradeURL := os.Getenv("TRADE_URL")
	tradeBotKey := os.Getenv("TRADE_BOT_API_KEY")
	b.sendText(chatID, "⏳ Memvalidasi file data perdagangan\\.\\.\\.")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", p.FileName)
	part.Write(p.FileData)
	mw.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	raw, status, err := tradePost(ctx, tradeURL+"/api/v1/bot-integration/owner/preview-trade-data", tradeBotKey, &buf, mw.FormDataContentType())
	if err != nil {
		b.sendText(chatID, "❌ Gagal koneksi ke TRADE: "+escapeMarkdown(err.Error()))
		return
	}
	if status != http.StatusOK {
		b.sendText(chatID, fmt.Sprintf("❌ TRADE error %d: %s", status, escapeMarkdown(truncate(string(raw), 300))))
		return
	}

	var pr tradePreviewResponse
	jsonUnmarshal(raw, &pr)

	var sb strings.Builder
	sb.WriteString("📊 *Preview Data Perdagangan*\n\n")
	sb.WriteString(fmt.Sprintf("📄 File: %s\n📝 Total: *%d* | ✅ Valid: *%d*", escapeMarkdown(p.FileName), pr.TotalRows, pr.ValidRows))
	if pr.InvalidRows > 0 {
		sb.WriteString(fmt.Sprintf(" | ❌ Invalid: *%d*", pr.InvalidRows))
	}
	sb.WriteString("\n")
	if len(pr.MissingRequired) > 0 {
		sb.WriteString("\n⚠️ *Kolom wajib tidak ditemukan:*\n")
		for _, c := range pr.MissingRequired {
			sb.WriteString("• `" + c + "`\n")
		}
	} else {
		sb.WriteString("\n✅ Semua kolom wajib ditemukan\n")
		if !pr.HasPrice {
			sb.WriteString("⚠️ Kolom harga tidak ditemukan\n")
		}
	}
	if len(pr.Errors) > 0 {
		sb.WriteString(fmt.Sprintf("\n⚠️ Contoh baris bermasalah:\n"))
		for i, e := range pr.Errors {
			if i >= 3 {
				break
			}
			sb.WriteString(fmt.Sprintf("• Baris %d: %s\n", e.Row, escapeMarkdown(strings.Join(e.Errors, ", "))))
		}
	}
	canImport := len(pr.MissingRequired) == 0 && pr.HasPrice && pr.ValidRows > 0
	if canImport {
		sb.WriteString(fmt.Sprintf("\nBalas *ya* untuk import *%d baris* ke TRADE, atau *batal*\\.", pr.ValidRows))
		p.Step = "trade_confirm"
		setPendingUpload(chatID, p)
	} else {
		sb.WriteString("\n❌ File tidak dapat diimport\\. Perbaiki dulu lalu kirim ulang\\.")
	}
	b.sendText(chatID, sb.String())
}

func (b *Bot) doTradeDataImport(chatID int64, p *pendingUpload) {
	tradeURL := os.Getenv("TRADE_URL")
	tradeBotKey := os.Getenv("TRADE_BOT_API_KEY")
	b.sendText(chatID, "⏳ Mengimport data perdagangan ke TRADE\\.\\.\\.")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", p.FileName)
	part.Write(p.FileData)
	mw.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	raw, status, err := tradePost(ctx, tradeURL+"/api/v1/bot-integration/owner/import-trade-data", tradeBotKey, &buf, mw.FormDataContentType())
	if err != nil {
		b.sendText(chatID, "❌ Gagal koneksi ke TRADE: "+escapeMarkdown(err.Error()))
		return
	}
	if status != http.StatusOK {
		b.sendText(chatID, fmt.Sprintf("❌ TRADE error %d: %s", status, escapeMarkdown(truncate(string(raw), 300))))
		return
	}
	var result struct {
		TotalRows   int `json:"total_rows"`
		ValidRows   int `json:"valid_rows"`
		InvalidRows int `json:"invalid_rows"`
	}
	jsonUnmarshal(raw, &result)
	b.sendText(chatID, fmt.Sprintf("✅ *Import Data Perdagangan berhasil\\!*\n\n📄 File: %s\n📝 Total: *%d* | ✅ Diproses: *%d* | ❌ Dilewati: *%d*\n\nCek di TRADE → *Data Perdagangan*\\.",
		escapeMarkdown(p.FileName), result.TotalRows, result.ValidRows, result.InvalidRows))
}

type unrecordedPreviewResponse struct {
	TotalRows   int  `json:"total_rows"`
	ValidRows   int  `json:"valid_rows"`
	InvalidRows int  `json:"invalid_rows"`
	HasNameCol  bool `json:"has_name_col"`
	Errors      []struct {
		Row    int      `json:"row"`
		Errors []string `json:"errors"`
	} `json:"errors"`
	Sample []map[string]string `json:"sample"`
}

func (b *Bot) doUnrecordedItemsPreview(chatID int64, p *pendingUpload) {
	tradeURL := os.Getenv("TRADE_URL")
	tradeBotKey := os.Getenv("TRADE_BOT_API_KEY")
	b.sendText(chatID, "⏳ Memvalidasi file permintaan barang\\.\\.\\.")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", p.FileName)
	part.Write(p.FileData)
	mw.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	raw, status, err := tradePost(ctx, tradeURL+"/api/v1/bot-integration/owner/preview-unrecorded-items", tradeBotKey, &buf, mw.FormDataContentType())
	if err != nil {
		b.sendText(chatID, "❌ Gagal koneksi ke TRADE: "+escapeMarkdown(err.Error()))
		return
	}
	if status != http.StatusOK {
		b.sendText(chatID, fmt.Sprintf("❌ TRADE error %d: %s", status, escapeMarkdown(truncate(string(raw), 300))))
		return
	}
	var pr unrecordedPreviewResponse
	jsonUnmarshal(raw, &pr)

	var sb strings.Builder
	sb.WriteString("📋 *Preview Permintaan Barang*\n\n")
	sb.WriteString(fmt.Sprintf("📄 File: %s\n📝 Total: *%d* | ✅ Valid: *%d*\n", escapeMarkdown(p.FileName), pr.TotalRows, pr.ValidRows))
	if !pr.HasNameCol {
		sb.WriteString("\n⚠️ Kolom nama barang tidak ditemukan\\.\n")
	} else if len(pr.Sample) > 0 {
		sb.WriteString("\n*Contoh item:*\n")
		for _, s := range pr.Sample {
			line := "• " + escapeMarkdown(s["nama_barang"])
			if s["merk"] != "" {
				line += " \\(" + escapeMarkdown(s["merk"]) + "\\)"
			}
			sb.WriteString(line + "\n")
		}
	}
	if pr.HasNameCol && pr.ValidRows > 0 {
		sb.WriteString(fmt.Sprintf("\nBalas *ya* untuk tambah *%d item* ke Permintaan Barang, atau *batal*\\.", pr.ValidRows))
		p.Step = "permintaan_confirm"
		setPendingUpload(chatID, p)
	} else {
		sb.WriteString("\n❌ File tidak dapat diimport\\.")
	}
	b.sendText(chatID, sb.String())
}

func (b *Bot) doUnrecordedItemsImport(chatID int64, p *pendingUpload) {
	tradeURL := os.Getenv("TRADE_URL")
	tradeBotKey := os.Getenv("TRADE_BOT_API_KEY")
	b.sendText(chatID, "⏳ Mengimport permintaan barang ke TRADE\\.\\.\\.")

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", p.FileName)
	part.Write(p.FileData)
	mw.WriteField("source", "telegram-bot")
	mw.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	raw, status, err := tradePost(ctx, tradeURL+"/api/v1/bot-integration/owner/import-unrecorded-items", tradeBotKey, &buf, mw.FormDataContentType())
	if err != nil {
		b.sendText(chatID, "❌ Gagal koneksi ke TRADE: "+escapeMarkdown(err.Error()))
		return
	}
	if status != http.StatusOK {
		b.sendText(chatID, fmt.Sprintf("❌ TRADE error %d: %s", status, escapeMarkdown(truncate(string(raw), 300))))
		return
	}
	var result struct {
		Created     int    `json:"created"`
		InvalidRows int    `json:"invalid_rows"`
		WeekLabel   string `json:"week_label"`
	}
	jsonUnmarshal(raw, &result)
	b.sendText(chatID, fmt.Sprintf("✅ *Permintaan Barang berhasil diimport\\!*\n\n📄 File: %s\n📝 Item: *%d* | ❌ Dilewati: *%d*\n📅 Minggu: %s\n\nCek di TRADE → *Permintaan Barang*\\.",
		escapeMarkdown(p.FileName), result.Created, result.InvalidRows, escapeMarkdown(result.WeekLabel)))
}

func (b *Bot) doUploadSupplierOffer(chatID int64, p *pendingUpload, supplierName, currency string) {
	tradeURL := os.Getenv("TRADE_URL")
	tradeBotKey := os.Getenv("TRADE_BOT_API_KEY")
	b.sendText(chatID, fmt.Sprintf("⏳ Mengupload *%s* ke TRADE\\.\\.\\.", escapeMarkdown(p.FileName)))

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", p.FileName)
	part.Write(p.FileData)
	if supplierName != "" {
		mw.WriteField("supplier_name", supplierName)
	}
	if currency == "" {
		currency = "USD"
	}
	mw.WriteField("currency", currency)
	mw.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	raw, status, err := tradePost(ctx, tradeURL+"/api/v1/bot-integration/owner/upload-supplier-offer", tradeBotKey, &buf, mw.FormDataContentType())
	if err != nil {
		b.sendText(chatID, "❌ Gagal upload ke TRADE: "+escapeMarkdown(err.Error()))
		return
	}
	if status != http.StatusOK {
		b.sendText(chatID, fmt.Sprintf("❌ TRADE error %d: %s", status, escapeMarkdown(truncate(string(raw), 300))))
		return
	}
	var result struct {
		UploadID string `json:"upload_id"`
	}
	jsonUnmarshal(raw, &result)

	sup := supplierName
	if sup == "" {
		sup = "_(auto-detect)_"
	}
	b.sendText(chatID, fmt.Sprintf("✅ *File penawaran supplier diterima\\!*\n\n📄 File: %s\n🏢 Supplier: %s\n💱 Currency: %s\n🔑 Upload ID: `%s`\n\nCek di TRADE → *Penawaran Supplier* dalam beberapa menit\\.",
		escapeMarkdown(p.FileName), escapeMarkdown(sup), currency, result.UploadID))
}

// ── Utils ─────────────────────────────────────────────────────────────────────

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
