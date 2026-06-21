package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ── Telegram types ─────────────────────────────────────────────────────────────

type Update struct {
	UpdateID int      `json:"update_id"`
	Message  *Message `json:"message"`
}

type Document struct {
	FileID   string `json:"file_id"`
	FileName string `json:"file_name"`
	MimeType string `json:"mime_type"`
	FileSize int64  `json:"file_size"`
}

type GetFileResponse struct {
	OK     bool   `json:"ok"`
	Result struct {
		FilePath string `json:"file_path"`
	} `json:"result"`
}

type Message struct {
	MessageID int       `json:"message_id"`
	From      *User     `json:"from"`
	Chat      Chat      `json:"chat"`
	Text      string    `json:"text"`
	Document  *Document `json:"document"`
}

type User struct {
	ID        int64  `json:"id"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

type Chat struct {
	ID   int64  `json:"id"`
	Type string `json:"type"`
}

type SendMessage struct {
	ChatID    int64  `json:"chat_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

type EditMessageText struct {
	ChatID    int64  `json:"chat_id"`
	MessageID int    `json:"message_id"`
	Text      string `json:"text"`
	ParseMode string `json:"parse_mode,omitempty"`
}

type TelegramSendResult struct {
	OK     bool    `json:"ok"`
	Result Message `json:"result"`
}

// ── Flowise types ─────────────────────────────────────────────────────────────

type FlowiseRequest struct {
	Question  string `json:"question"`
	SessionID string `json:"sessionId,omitempty"`
}

type FlowiseResponse struct {
	Text string `json:"text"`
}

// ── Bot ───────────────────────────────────────────────────────────────────────
// Bot encapsulates one Telegram bot: one token, one Flowise endpoint, and an
// optional allow-list of Telegram user IDs (non-empty = deny non-members).

type Bot struct {
	name          string
	token         string
	flowiseURL    string
	ownerIDs      map[int64]bool
	webhookSecret string
	tgClient      *http.Client
	flowiseCli    *http.Client
	timeout       time.Duration
	waitInterval  time.Duration
}

var waitingMessages = []string{
	"⏳ Mencari informasi ...",
	"⏳ Mohon ditunggu ...",
	"⏳ Masih mencari informasi ...",
	"⏳ ...",
	"⏳ Harap bersabar 🙂 masih proses ...",
	"⏳ Sedang diproses ...",
	"⏳ Sebentar lagi ...",
}

func (b *Bot) handler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if b.webhookSecret != "" {
		if r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != b.webhookSecret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var update Update
	if err := json.Unmarshal(body, &update); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	if update.Message == nil {
		return
	}
	// Owner Excel upload
	if update.Message.Document != nil && b.isExcelDoc(update.Message.Document) {
		go b.processDocumentMessage(update.Message)
		return
	}
	if strings.TrimSpace(update.Message.Text) == "" {
		return
	}
	go b.processMessage(update.Message)
}

// detectFileIntent guesses file purpose from the filename.
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

// intentLabel returns a human-readable label for a detected intent.
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

// parseFileIntent parses the owner's reply to the "file ini untuk apa?" prompt.
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

// parseSupplierReply tries to extract supplier name and currency from a reply
// like "SUPPLIER: SANKO, CURRENCY: USD" or just "skip".
func parseSupplierReply(text string) (supplierName, currency string, ok bool) {
	t := strings.TrimSpace(text)
	lower := strings.ToLower(t)
	if lower == "skip" || lower == "auto" || lower == "-" {
		return "", "USD", true
	}
	// Match "SUPPLIER: X, CURRENCY: Y" in any order/case
	reSup := regexp.MustCompile(`(?i)supplier\s*:\s*([^,\n]+)`)
	reCur := regexp.MustCompile(`(?i)currency\s*:\s*([A-Za-z]{3})`)
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

func (b *Bot) processMessage(msg *Message) {
	chatID := msg.Chat.ID
	sessionID := strconv.FormatInt(chatID, 10)

	// Guard: if ownerIDs is non-empty, deny users not in the list.
	if len(b.ownerIDs) > 0 && (msg.From == nil || !b.ownerIDs[msg.From.ID]) {
		b.sendText(chatID, "❌ Akses ditolak. Bot ini hanya untuk owner Ocean Bearings.")
		return
	}

	// Check if this is a reply to a pending Excel upload prompt
	if p := takePendingUpload(chatID); p != nil {
		if time.Since(p.At) > 10*time.Minute {
			b.sendText(chatID, "⏰ Upload expired. Kirim ulang file Excel-nya.")
			return
		}
		switch p.Step {
		case "intent":
			intent := parseFileIntent(msg.Text)
			switch intent {
			case "supplier":
				p.Step = "supplier_info"
				setPendingUpload(chatID, p)
				b.sendText(chatID, "✅ Penawaran Supplier\\.\n\nBalas dengan format:\n`SUPPLIER: <nama supplier>, CURRENCY: <USD/IDR/SGD/JPY/EUR>`\n\nContoh: `SUPPLIER: SANKO, CURRENCY: USD`\n\nAtau ketik `skip` untuk auto\\-detect dari file\\.")
			case "trade":
				go b.doTradeDataPreview(chatID, p)
			case "permintaan":
				b.sendText(chatID, "🚧 Fitur import *Permintaan Barang* sedang dalam pengembangan\\. Coming soon\\!")
			default:
				setPendingUpload(chatID, p)
				b.sendText(chatID, "❓ Pilihan tidak dikenali\\. Balas:\n*1* \\- Penawaran Supplier\n*2* \\- Data Perdagangan\n*3* \\- Permintaan Barang")
			}
		case "supplier_info":
			supplierName, currency, ok := parseSupplierReply(msg.Text)
			if !ok {
				setPendingUpload(chatID, p)
				b.sendText(chatID, "❓ Format tidak dikenali\\. Balas dengan:\n`SUPPLIER: <nama>, CURRENCY: <USD/IDR/JPY/dll>`\n\nAtau ketik `skip` untuk auto\\-detect\\.")
				return
			}
			go b.doUploadSupplierOffer(chatID, p, supplierName, currency)
		case "trade_confirm":
			lower := strings.ToLower(strings.TrimSpace(msg.Text))
			if lower == "ya" || lower == "yes" || lower == "lanjut" || lower == "ok" || lower == "import" {
				go b.doTradeDataImport(chatID, p)
			} else if lower == "batal" || lower == "cancel" || lower == "tidak" || lower == "no" {
				b.sendText(chatID, "❌ Import dibatalkan\\.")
			} else {
				setPendingUpload(chatID, p)
				b.sendText(chatID, "❓ Balas *ya* untuk import atau *batal* untuk membatalkan\\.")
			}
		}
		return
	}

	waitMsgID, err := b.sendAndGetID(chatID, waitingMessages[0])
	if err != nil {
		log.Printf("[%s] failed to send wait message: %v", b.name, err)
	}

	done := make(chan struct{})
	if waitMsgID > 0 {
		go func() {
			for i := 1; ; i++ {
				select {
				case <-done:
					return
				case <-time.After(b.waitInterval):
					b.editTextAsync(chatID, waitMsgID, waitingMessages[i%len(waitingMessages)])
				}
			}
		}()
	}

	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()

	answer, err := b.callFlowise(ctx, msg.Text, sessionID)
	close(done)

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[%s] timeout for session %s", b.name, sessionID)
			answer = "🔴 Maaf, server data kami sedang sangat sibuk, mohon coba kembali nanti."
		} else {
			log.Printf("[%s] error for session %s: %v", b.name, sessionID, err)
			answer = "⚠️ Maaf, terjadi kesalahan. Silakan coba kembali."
		}
	}

	answer = strings.TrimSpace(answer)
	if answer == "" {
		answer = "⚠️ Tidak ada respons dari sistem. Silakan coba kembali."
	}

	log.Printf("[%s] reply session %s: %.80s", b.name, sessionID, answer)

	const maxLen = 4096
	first, rest := answer, ""
	if len(answer) > maxLen {
		first = answer[:maxLen]
		rest = answer[maxLen:]
	}

	if waitMsgID > 0 {
		if !b.editTextSafe(chatID, waitMsgID, first) {
			b.sendText(chatID, first)
		}
	} else {
		b.sendText(chatID, first)
	}
	if rest != "" {
		b.sendText(chatID, rest)
	}
}

func (b *Bot) callFlowise(ctx context.Context, question, sessionID string) (string, error) {
	payload := FlowiseRequest{Question: question, SessionID: sessionID}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.flowiseURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := b.flowiseCli.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("flowise returned %d: %s", resp.StatusCode, string(raw))
	}
	var fr FlowiseResponse
	if err := json.Unmarshal(raw, &fr); err != nil {
		return "", fmt.Errorf("parse response: %w (body: %s)", err, string(raw))
	}
	text := strings.TrimSpace(fr.Text)
	if text == "" {
		log.Printf("[%s] empty text response, raw: %.200s", b.name, string(raw))
		return "", fmt.Errorf("flowise returned empty text")
	}
	return text, nil
}

func (b *Bot) isExcelDoc(doc *Document) bool {
	if doc == nil {
		return false
	}
	return doc.MimeType == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
		doc.MimeType == "application/vnd.ms-excel" ||
		strings.HasSuffix(strings.ToLower(doc.FileName), ".xlsx") ||
		strings.HasSuffix(strings.ToLower(doc.FileName), ".xls")
}

// pendingUpload holds a downloaded Excel file waiting for user confirmation.
// Step: "intent" → file purpose; "supplier_info" → name+currency; "trade_confirm" → ya/batal after preview.
type pendingUpload struct {
	FileData    []byte
	FileName    string
	At          time.Time
	Step        string // "intent", "supplier_info", "trade_confirm"
	PreviewText string // formatted preview message (for trade confirm step)
}

// pendingUploads maps chatID → pending upload (max ~5 min TTL).
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

// processDocumentMessage handles Excel uploads from owner — downloads from Telegram,
// asks for supplier name + currency, then uploads to TRADE.
func (b *Bot) processDocumentMessage(msg *Message) {
	chatID := msg.Chat.ID
	doc := msg.Document

	if len(b.ownerIDs) > 0 && (msg.From == nil || !b.ownerIDs[msg.From.ID]) {
		b.sendText(chatID, "❌ Hanya owner yang bisa upload penawaran supplier.")
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

	// 1. getFile → get download path
	type getFileReq struct {
		FileID string `json:"file_id"`
	}
	raw, err := b.telegramAPIWithResult("getFile", getFileReq{FileID: doc.FileID})
	if err != nil {
		log.Printf("[%s] getFile error: %v", b.name, err)
		b.sendText(chatID, "❌ Gagal mengambil file dari Telegram.")
		return
	}
	var gfResp GetFileResponse
	if err := json.Unmarshal(raw, &gfResp); err != nil || gfResp.Result.FilePath == "" {
		b.sendText(chatID, "❌ Respons getFile tidak valid.")
		return
	}

	// 2. Download file bytes
	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.token, gfResp.Result.FilePath)
	dlReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	dlResp, err := b.tgClient.Do(dlReq)
	if err != nil {
		b.sendText(chatID, "❌ Gagal mengunduh file dari Telegram.")
		return
	}
	defer dlResp.Body.Close()
	fileData, _ := io.ReadAll(dlResp.Body)

	fname := doc.FileName
	if fname == "" {
		fname = "penawaran-supplier.xlsx"
	}

	// 3. Save pending and ask intent
	setPendingUpload(chatID, &pendingUpload{FileData: fileData, FileName: fname, At: time.Now(), Step: "intent"})

	hint := detectFileIntent(fname)
	var prompt string
	if hint != "" {
		prompt = fmt.Sprintf(
			"📄 File *%s* (%.1f KB) terdeteksi sebagai *%s*.\n\nBalas untuk konfirmasi:\n*1* \\- Penawaran Supplier\n*2* \\- Data Perdagangan\n*3* \\- Permintaan Barang",
			fname, float64(len(fileData))/1024, intentLabel(hint),
		)
	} else {
		prompt = fmt.Sprintf(
			"📄 File *%s* (%.1f KB) diterima\\.\n\nFile ini untuk apa?\n\n*1* \\- Penawaran Supplier\n*2* \\- Data Perdagangan\n*3* \\- Permintaan Barang",
			fname, float64(len(fileData))/1024,
		)
	}
	b.sendText(chatID, prompt)
}

// tradePreviewResponse mirrors the TRADE preview endpoint response.
type tradePreviewResponse struct {
	TotalRows       int      `json:"total_rows"`
	ValidRows       int      `json:"valid_rows"`
	InvalidRows     int      `json:"invalid_rows"`
	ColumnsFound    []string `json:"columns_found"`
	MissingRequired []string `json:"missing_required"`
	HasPrice        bool     `json:"has_price"`
	Errors          []struct {
		Row    int      `json:"row"`
		Errors []string `json:"errors"`
	} `json:"errors"`
	Filename   string `json:"filename"`
	CsvSizeKB  float64 `json:"csv_size_kb"`
}

// doTradeDataPreview sends the file to TRADE preview endpoint and shows summary to owner.
func (b *Bot) doTradeDataPreview(chatID int64, p *pendingUpload) {
	tradeURL := os.Getenv("TRADE_URL")
	tradeBotKey := os.Getenv("TRADE_BOT_API_KEY")

	b.sendText(chatID, "⏳ Memvalidasi file data perdagangan\\.\\.\\.")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", p.FileName)
	_, _ = part.Write(p.FileData)
	mw.Close()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		tradeURL+"/api/v1/bot-integration/owner/preview-trade-data", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-API-Key", tradeBotKey)

	resp, err := (&http.Client{Timeout: 60 * time.Second}).Do(req)
	if err != nil {
		b.sendText(chatID, "❌ Gagal koneksi ke TRADE: "+escapeMarkdown(err.Error()))
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		errMsg := string(raw)
		if len(errMsg) > 300 {
			errMsg = errMsg[:300]
		}
		b.sendText(chatID, fmt.Sprintf("❌ TRADE error %d: %s", resp.StatusCode, escapeMarkdown(errMsg)))
		return
	}

	var pr tradePreviewResponse
	json.Unmarshal(raw, &pr)

	// Build preview message
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📊 *Preview Data Perdagangan*\n\n"))
	sb.WriteString(fmt.Sprintf("📄 File: %s\n", escapeMarkdown(p.FileName)))
	sb.WriteString(fmt.Sprintf("📝 Total baris: *%d*\n", pr.TotalRows))
	sb.WriteString(fmt.Sprintf("✅ Valid: *%d*\n", pr.ValidRows))
	if pr.InvalidRows > 0 {
		sb.WriteString(fmt.Sprintf("❌ Invalid: *%d*\n", pr.InvalidRows))
	}

	if len(pr.MissingRequired) > 0 {
		sb.WriteString(fmt.Sprintf("\n⚠️ *Kolom wajib tidak ditemukan:*\n"))
		for _, c := range pr.MissingRequired {
			sb.WriteString(fmt.Sprintf("• `%s`\n", c))
		}
	} else {
		sb.WriteString("\n✅ Semua kolom wajib ditemukan\n")
		if !pr.HasPrice {
			sb.WriteString("⚠️ Kolom harga \\(`price_per_unit`/`trade_amount`\\) tidak ditemukan\n")
		}
	}

	if len(pr.Errors) > 0 {
		sb.WriteString(fmt.Sprintf("\n⚠️ *Contoh baris bermasalah \\(%d\\):*\n", pr.InvalidRows))
		for _, e := range pr.Errors {
			if len(pr.Errors) > 3 && e.Row > pr.Errors[2].Row {
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
		sb.WriteString("\n❌ File tidak dapat diimport\\. Perbaiki kolom yang hilang lalu kirim ulang\\.")
	}

	b.sendText(chatID, sb.String())
}

// doTradeDataImport sends the file to TRADE import endpoint after owner confirms.
func (b *Bot) doTradeDataImport(chatID int64, p *pendingUpload) {
	tradeURL := os.Getenv("TRADE_URL")
	tradeBotKey := os.Getenv("TRADE_BOT_API_KEY")

	b.sendText(chatID, "⏳ Mengimport data perdagangan ke TRADE\\.\\.\\.")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", p.FileName)
	_, _ = part.Write(p.FileData)
	mw.Close()

	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		tradeURL+"/api/v1/bot-integration/owner/import-trade-data", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-API-Key", tradeBotKey)

	resp, err := (&http.Client{Timeout: 120 * time.Second}).Do(req)
	if err != nil {
		b.sendText(chatID, "❌ Gagal koneksi ke TRADE: "+escapeMarkdown(err.Error()))
		return
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		errMsg := string(raw)
		if len(errMsg) > 300 {
			errMsg = errMsg[:300]
		}
		b.sendText(chatID, fmt.Sprintf("❌ TRADE error %d: %s", resp.StatusCode, escapeMarkdown(errMsg)))
		return
	}

	var result struct {
		TotalRows   int    `json:"total_rows"`
		ValidRows   int    `json:"valid_rows"`
		InvalidRows int    `json:"invalid_rows"`
		Message     string `json:"message"`
	}
	json.Unmarshal(raw, &result)

	b.sendText(chatID, fmt.Sprintf(
		"✅ *Import Data Perdagangan berhasil dimulai\\!*\n\n"+
			"📄 File: %s\n"+
			"📝 Total baris: *%d*\n"+
			"✅ Diproses: *%d*\n"+
			"❌ Dilewati: *%d*\n\n"+
			"Cek hasil di TRADE → *Data Perdagangan* dalam beberapa menit\\.",
		escapeMarkdown(p.FileName), result.TotalRows, result.ValidRows, result.InvalidRows,
	))
}

// escapeMarkdown escapes special MarkdownV2 characters.
func escapeMarkdown(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]",
		"(", "\\(", ")", "\\)", "~", "\\~", "`", "\\`",
		">", "\\>", "#", "\\#", "+", "\\+", "-", "\\-",
		"=", "\\=", "|", "\\|", "{", "\\{", "}", "\\}",
		".", "\\.", "!", "\\!",
	)
	return replacer.Replace(s)
}

// doUploadSupplierOffer uploads the pending Excel file to TRADE with given supplier+currency.
func (b *Bot) doUploadSupplierOffer(chatID int64, p *pendingUpload, supplierName, currency string) {
	tradeURL := os.Getenv("TRADE_URL")
	tradeBotKey := os.Getenv("TRADE_BOT_API_KEY")

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	b.sendText(chatID, fmt.Sprintf("⏳ Mengupload *%s* ke TRADE untuk diproses...", p.FileName))

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, _ := mw.CreateFormFile("file", p.FileName)
	_, _ = part.Write(p.FileData)
	if supplierName != "" {
		_ = mw.WriteField("supplier_name", supplierName)
	}
	if currency == "" {
		currency = "USD"
	}
	_ = mw.WriteField("currency", currency)
	mw.Close()

	tradeReq, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		tradeURL+"/api/v1/bot-integration/owner/upload-supplier-offer", &buf)
	tradeReq.Header.Set("Content-Type", mw.FormDataContentType())
	tradeReq.Header.Set("X-API-Key", tradeBotKey)

	tradeClient := &http.Client{Timeout: 90 * time.Second}
	tradeResp, err := tradeClient.Do(tradeReq)
	if err != nil {
		b.sendText(chatID, "❌ Gagal upload ke TRADE: "+err.Error())
		return
	}
	defer tradeResp.Body.Close()
	tradeRaw, _ := io.ReadAll(tradeResp.Body)

	if tradeResp.StatusCode != http.StatusOK {
		errMsg := string(tradeRaw)
		if len(errMsg) > 300 {
			errMsg = errMsg[:300]
		}
		b.sendText(chatID, fmt.Sprintf("❌ TRADE error %d: %s", tradeResp.StatusCode, errMsg))
		return
	}

	var result struct {
		UploadID string `json:"upload_id"`
	}
	json.Unmarshal(tradeRaw, &result)

	reply := fmt.Sprintf(
		"✅ *File penawaran supplier diterima!*\n\n"+
			"📄 File: %s\n"+
			"🏢 Supplier: %s\n"+
			"💱 Currency: %s\n"+
			"🔑 Upload ID: `%s`\n\n"+
			"Proses auto-mapping produk sedang berjalan di background.\n"+
			"Cek hasil di TRADE → *Penawaran Supplier* dalam beberapa menit.",
		p.FileName,
		func() string {
			if supplierName != "" {
				return supplierName
			}
			return "_(auto-detect dari file)_"
		}(),
		currency,
		result.UploadID,
	)
	b.sendText(chatID, reply)
}

// registerWebhook calls Telegram's setWebhook API so Telegram sends updates to
// this service. Runs in a goroutine on startup when WEBHOOK_BASE_URL is set.
func (b *Bot) registerWebhook(webhookURL string) {
	type setWebhookReq struct {
		URL         string `json:"url"`
		SecretToken string `json:"secret_token,omitempty"`
	}
	payload := setWebhookReq{URL: webhookURL}
	if b.webhookSecret != "" {
		payload.SecretToken = b.webhookSecret
	}
	if err := b.telegramAPI("setWebhook", payload); err != nil {
		log.Printf("[%s] setWebhook error: %v", b.name, err)
	} else {
		log.Printf("[%s] webhook registered: %s", b.name, webhookURL)
	}
}

// ── Telegram helpers ──────────────────────────────────────────────────────────

func (b *Bot) telegramAPI(method string, payload any) error {
	_, err := b.telegramAPIWithResult(method, payload)
	return err
}

func (b *Bot) telegramAPIWithResult(method string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", b.token, method)
	resp, err := b.tgClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("telegram %s: %w", method, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram %s returned %d: %s", method, resp.StatusCode, string(raw))
	}
	return raw, nil
}

var (
	reMdLink = regexp.MustCompile(`\[([^\]]+)\]\s*\(([^)]+)\)`)
	reBold   = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	reBullet = regexp.MustCompile(`(?m)^\*[ \t]+`)
)

func mdToHTML(s string) string {
	s = reBullet.ReplaceAllString(s, "• ")
	s = html.EscapeString(s)
	s = reMdLink.ReplaceAllStringFunc(s, func(m string) string {
		parts := reMdLink.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		text := strings.TrimSpace(parts[1])
		target := strings.TrimSpace(parts[2])
		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			return fmt.Sprintf(`<a href="%s">%s</a>`, target, text)
		}
		return text
	})
	s = reBold.ReplaceAllString(s, `<b>$1</b>`)
	return s
}

func (b *Bot) sendText(chatID int64, text string) {
	text = mdToHTML(text)
	const maxLen = 4096
	for len(text) > 0 {
		chunk := text
		if len(chunk) > maxLen {
			chunk = text[:maxLen]
			text = text[maxLen:]
		} else {
			text = ""
		}
		if err := b.telegramAPI("sendMessage", SendMessage{
			ChatID:    chatID,
			Text:      chunk,
			ParseMode: "HTML",
		}); err != nil {
			log.Printf("[%s] sendMessage error: %v", b.name, err)
			_ = b.telegramAPI("sendMessage", SendMessage{ChatID: chatID, Text: chunk})
		}
	}
}

func (b *Bot) sendAndGetID(chatID int64, text string) (int, error) {
	raw, err := b.telegramAPIWithResult("sendMessage", SendMessage{ChatID: chatID, Text: text})
	if err != nil {
		return 0, err
	}
	var result TelegramSendResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return 0, err
	}
	return result.Result.MessageID, nil
}

func (b *Bot) editTextSafe(chatID int64, messageID int, text string) bool {
	if text == "" {
		return false
	}
	text = mdToHTML(text)
	err := b.telegramAPI("editMessageText", EditMessageText{
		ChatID: chatID, MessageID: messageID, Text: text, ParseMode: "HTML",
	})
	if err != nil {
		log.Printf("[%s] editMessageText error: %v — retrying plain", b.name, err)
		err = b.telegramAPI("editMessageText", EditMessageText{
			ChatID: chatID, MessageID: messageID, Text: text,
		})
		if err != nil {
			log.Printf("[%s] editMessageText retry failed: %v", b.name, err)
			return false
		}
	}
	return true
}

func (b *Bot) editTextAsync(chatID int64, messageID int, text string) {
	b.telegramAPI("editMessageText", EditMessageText{ //nolint
		ChatID: chatID, MessageID: messageID, Text: text,
	})
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func parseIDSet(s string) map[int64]bool {
	set := map[int64]bool{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		var id int64
		if _, err := fmt.Sscanf(part, "%d", &id); err == nil {
			set[id] = true
		}
	}
	return set
}

func parseTimeoutSec(s string) time.Duration {
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 60 * time.Second
	}
	return time.Duration(n) * time.Second
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env var: %s", key)
	}
	return v
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func newBot(name, token, flowiseURL, webhookSecret string, ownerIDs map[int64]bool, timeout, waitInterval time.Duration) *Bot {
	return &Bot{
		name:          name,
		token:         token,
		flowiseURL:    flowiseURL,
		ownerIDs:      ownerIDs,
		webhookSecret: webhookSecret,
		tgClient:      &http.Client{Timeout: 10 * time.Second},
		flowiseCli:    &http.Client{Timeout: timeout + 5*time.Second},
		timeout:       timeout,
		waitInterval:  waitInterval,
	}
}

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	port := envOr("PORT", "8081")
	// WEBHOOK_BASE_URL: public URL prefix for webhook registration, e.g.
	// https://agentic.oceanbearings.co.id/telegram
	baseURL := os.Getenv("WEBHOOK_BASE_URL")
	timeout := parseTimeoutSec(envOr("FLOWISE_TIMEOUT", "60"))
	waitInterval := parseTimeoutSec(envOr("WAIT_MSG_INTERVAL", "6"))
	webhookSecret := os.Getenv("WEBHOOK_SECRET")

	// Customer bot — required
	customerBot := newBot(
		"customer",
		mustEnv("TELEGRAM_BOT_TOKEN"),
		mustEnv("FLOWISE_ENDPOINT"),
		webhookSecret,
		nil, // no ID restriction — allow all users
		timeout,
		waitInterval,
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", customerBot.handler)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	})

	if baseURL != "" {
		go customerBot.registerWebhook(baseURL + "/webhook")
	}

	// Owner bot — optional, enabled when OWNER_BOT_TOKEN + OWNER_FLOWISE_ENDPOINT are set.
	// Messages from users not listed in OWNER_TELEGRAM_IDS are denied automatically.
	ownerBotToken := os.Getenv("OWNER_BOT_TOKEN")
	ownerFlowiseURL := os.Getenv("OWNER_FLOWISE_ENDPOINT")
	if ownerBotToken != "" && ownerFlowiseURL != "" {
		ownerBot := newBot(
			"owner",
			ownerBotToken,
			ownerFlowiseURL,
			webhookSecret,
			parseIDSet(os.Getenv("OWNER_TELEGRAM_IDS")),
			timeout,
			waitInterval,
		)
		mux.HandleFunc("/webhook/owner", ownerBot.handler)
		log.Printf("Owner bot enabled on /webhook/owner")
		if baseURL != "" {
			go ownerBot.registerWebhook(baseURL + "/webhook/owner")
		} else {
			log.Println("WEBHOOK_BASE_URL not set — owner webhook not auto-registered, set manually")
		}
	} else {
		log.Println("Owner bot not configured (OWNER_BOT_TOKEN or OWNER_FLOWISE_ENDPOINT missing)")
	}

	log.Printf("go-telegram service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
