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
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ── Telegram types ────────────────────────────────────────────────────────────

type Update struct {
	UpdateID int      `json:"update_id"`
	Message  *Message `json:"message"`
}

type Message struct {
	MessageID int    `json:"message_id"`
	From      *User  `json:"from"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
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

type SendChatAction struct {
	ChatID int64  `json:"chat_id"`
	Action string `json:"action"`
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

// ── Config ────────────────────────────────────────────────────────────────────

var (
	botToken       = mustEnv("TELEGRAM_BOT_TOKEN")
	flowiseURL     = mustEnv("FLOWISE_ENDPOINT")
	webhookSecret  = os.Getenv("WEBHOOK_SECRET")
	port           = envOr("PORT", "8081")
	flowiseTimeout  = parseTimeoutSec(envOr("FLOWISE_TIMEOUT", "60"))
	waitMsgInterval = parseTimeoutSec(envOr("WAIT_MSG_INTERVAL", "6"))

	// Client terpisah: Telegram harus cepat, Flowise boleh lama
	tgClient      = &http.Client{Timeout: 10 * time.Second}
	flowiseClient = &http.Client{Timeout: flowiseTimeout + 5*time.Second}
)

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

// ── Main ──────────────────────────────────────────────────────────────────────

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", handleWebhook)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "OK")
	})

	log.Printf("go-telegram service listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}

// ── Webhook handler ───────────────────────────────────────────────────────────

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Verify Telegram secret token if configured
	if webhookSecret != "" {
		if r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != webhookSecret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1 MB limit
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

	// Ack Telegram immediately; process async
	w.WriteHeader(http.StatusOK)

	if update.Message == nil || strings.TrimSpace(update.Message.Text) == "" {
		return
	}

	go processMessage(update.Message)
}

// ── Message processing ────────────────────────────────────────────────────────

var waitingMessages = []string{
	"⏳ Mencari informasi ...",
	"⏳ Mohon ditunggu ...",
	"⏳ Masih mencari informasi ...",
	"⏳ ...",
	"⏳ Harap bersabar 🙂 masih proses ...",
	"⏳ Sedang diproses ...",
	"⏳ Sebentar lagi ...",
}

func processMessage(msg *Message) {
	chatID := msg.Chat.ID
	sessionID := strconv.FormatInt(chatID, 10)

	// Kirim pesan tunggu pertama langsung
	waitMsgID, err := sendAndGetID(chatID, waitingMessages[0])
	if err != nil {
		log.Printf("[telegram] failed to send wait message: %v", err)
	}

	// Goroutine: ganti teks tunggu tiap 3 detik sampai Flowise selesai
	done := make(chan struct{})
	if waitMsgID > 0 {
		go func() {
			for i := 1; ; i++ {
				select {
				case <-done:
					return
				case <-time.After(waitMsgInterval):
					editTextAsync(chatID, waitMsgID, waitingMessages[i%len(waitingMessages)])
				}
			}
		}()
	}

	ctx, cancel := context.WithTimeout(context.Background(), flowiseTimeout)
	defer cancel()

	answer, err := callFlowise(ctx, msg.Text, sessionID)
	close(done) // hentikan rotasi pesan tunggu

	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[flowise] timeout for session %s", sessionID)
			answer = "🔴 Maaf, server data kami sedang sangat sibuk, mohon coba kembali nanti."
		} else {
			log.Printf("[flowise] error for session %s: %v", sessionID, err)
			answer = "⚠️ Maaf, terjadi kesalahan. Silakan coba kembali."
		}
	}

	// Guard: pastikan answer tidak kosong sebelum dikirim
	answer = strings.TrimSpace(answer)
	if answer == "" {
		answer = "⚠️ Tidak ada respons dari sistem. Silakan coba kembali."
	}

	log.Printf("[reply] session %s: %.80s", sessionID, answer)

	const maxLen = 4096
	first, rest := answer, ""
	if len(answer) > maxLen {
		first = answer[:maxLen]
		rest = answer[maxLen:]
	}

	if waitMsgID > 0 {
		// Coba edit pesan tunggu; kalau gagal (misal message sudah expired), kirim baru
		if !editTextSafe(chatID, waitMsgID, first) {
			sendText(chatID, first)
		}
	} else {
		sendText(chatID, first)
	}
	if rest != "" {
		sendText(chatID, rest)
	}
}

// ── Flowise call ──────────────────────────────────────────────────────────────

func callFlowise(ctx context.Context, question, sessionID string) (string, error) {
	payload := FlowiseRequest{Question: question, SessionID: sessionID}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, flowiseURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := flowiseClient.Do(req)
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
		log.Printf("[flowise] empty text response, raw: %.200s", string(raw))
		return "", fmt.Errorf("flowise returned empty text")
	}
	return text, nil
}

// ── Telegram helpers ──────────────────────────────────────────────────────────

func telegramAPI(method string, payload any) error {
	_, err := telegramAPIWithResult(method, payload)
	return err
}

func telegramAPIWithResult(method string, payload any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", botToken, method)
	resp, err := tgClient.Post(url, "application/json", bytes.NewReader(body))
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


// mdToHTML konversi subset Markdown → HTML untuk Telegram
// Handles: [text](url), **bold**, *italic*, `code`, ```block```
var (
	reMdLink  = regexp.MustCompile(`\[([^\]]+)\]\s*\(([^)]+)\)`)
	reBold    = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	reBullet  = regexp.MustCompile(`(?m)^\*[ \t]+`)   // * di awal baris = bullet list
)

// mdToHTML konversi Markdown sederhana ke HTML Telegram
func mdToHTML(s string) string {
	// 1. Ganti bullet list * → • sebelum apapun (cegah false italic)
	s = reBullet.ReplaceAllString(s, "• ")

	// 2. HTML-escape seluruh teks — [, ], (, ) tidak ikut ter-escape
	s = html.EscapeString(s)

	// 3. Konversi [text](url) → <a href>
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
		return text // placeholder bukan URL → teks saja
	})

	// 4. Bold saja (italic dihapus — * untuk bullet list menyebabkan false match)
	s = reBold.ReplaceAllString(s, `<b>$1</b>`)

	return s
}

func sendText(chatID int64, text string) {
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
		if err := telegramAPI("sendMessage", SendMessage{
			ChatID:    chatID,
			Text:      chunk,
			ParseMode: "HTML",
		}); err != nil {
			log.Printf("[telegram] sendMessage error: %v", err)
			_ = telegramAPI("sendMessage", SendMessage{ChatID: chatID, Text: chunk})
		}
	}
}

func sendChatAction(chatID int64, action string) {
	if err := telegramAPI("sendChatAction", SendChatAction{ChatID: chatID, Action: action}); err != nil {
		log.Printf("[telegram] sendChatAction error: %v", err)
	}
}

func sendAndGetID(chatID int64, text string) (int, error) {
	raw, err := telegramAPIWithResult("sendMessage", SendMessage{ChatID: chatID, Text: text})
	if err != nil {
		return 0, err
	}
	var result TelegramSendResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return 0, err
	}
	return result.Result.MessageID, nil
}

// editTextSafe mengembalikan true jika berhasil, false jika gagal total (pakai sendText sebagai fallback)
func editTextSafe(chatID int64, messageID int, text string) bool {
	if text == "" {
		return false
	}
	text = mdToHTML(text)
	err := telegramAPI("editMessageText", EditMessageText{
		ChatID: chatID, MessageID: messageID, Text: text, ParseMode: "HTML",
	})
	if err != nil {
		log.Printf("[telegram] editMessageText error: %v — retrying without Markdown", err)
		err = telegramAPI("editMessageText", EditMessageText{
			ChatID: chatID, MessageID: messageID, Text: text,
		})
		if err != nil {
			log.Printf("[telegram] editMessageText retry failed: %v", err)
			return false
		}
	}
	return true
}

// editTextAsync dipakai goroutine rotating — tidak perlu fallback
func editTextAsync(chatID int64, messageID int, text string) {
	telegramAPI("editMessageText", EditMessageText{ //nolint
		ChatID: chatID, MessageID: messageID, Text: text,
	})
}
