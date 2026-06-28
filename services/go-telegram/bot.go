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
	"math/rand"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ── Error codes ───────────────────────────────────────────────────────────────

const errCodeChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newErrorCode() string {
	b := make([]byte, 5)
	for i := range b {
		b[i] = errCodeChars[rand.Intn(len(errCodeChars))]
	}
	return string(b)
}

// ── Telegram wire types ───────────────────────────────────────────────────────

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

type FlowiseRequest struct {
	Question string `json:"question"`
	ChatID   string `json:"chatId,omitempty"`
}

type FlowiseResponse struct {
	Text string `json:"text"`
}

// ── Bot ───────────────────────────────────────────────────────────────────────

type Bot struct {
	name          string
	token         string
	flowiseURL    string
	flowiseKey    string
	webhookSecret string
	humanContact  string
	ownerIDs      map[int64]bool
	disableUpload bool
	tgClient      *http.Client
	flowiseCli    *http.Client
	timeout       time.Duration
	waitInterval  time.Duration
}

func newBot(name, token, flowiseURL, flowiseKey, webhookSecret, humanContact string,
	ownerIDs map[int64]bool, disableUpload bool,
	timeout, waitInterval time.Duration) *Bot {
	return &Bot{
		name:          name,
		token:         token,
		flowiseURL:    flowiseURL,
		flowiseKey:    flowiseKey,
		webhookSecret: webhookSecret,
		humanContact:  humanContact,
		ownerIDs:      ownerIDs,
		disableUpload: disableUpload,
		tgClient:      &http.Client{Timeout: 30 * time.Second},
		flowiseCli:    &http.Client{Timeout: timeout + 5*time.Second},
		timeout:       timeout,
		waitInterval:  waitInterval,
	}
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
	if !b.disableUpload && update.Message.Document != nil && b.isExcelDoc(update.Message.Document) {
		go b.processDocumentMessage(update.Message)
		return
	}
	if strings.TrimSpace(update.Message.Text) == "" {
		return
	}
	go b.processMessage(update.Message)
}

func (b *Bot) processMessage(msg *Message) {
	chatID := msg.Chat.ID
	username := ""
	if msg.From != nil {
		username = msg.From.Username
		if username == "" {
			username = msg.From.FirstName
		}
	}
	sessionID := strconv.FormatInt(chatID, 10)

	if len(b.ownerIDs) > 0 && (msg.From == nil || !b.ownerIDs[msg.From.ID]) {
		b.sendText(chatID, "❌ Akses ditolak.")
		return
	}

	if !b.disableUpload {
		if p := takePendingUpload(chatID); p != nil {
			b.handlePendingUploadReply(chatID, msg.Text, p)
			return
		}
	}

	waitMsgID, err := b.sendAndGetID(chatID, waitingMessages[0])
	if err != nil {
		log.Printf("[%s] send wait msg error: %v", b.name, err)
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
		code := newErrorCode()
		if errors.Is(err, context.DeadlineExceeded) {
			log.Printf("[%s] [%s] timeout chat:%d user:%s", b.name, code, chatID, username)
			answer = fmt.Sprintf("🔴 Server sedang sibuk, coba lagi nanti. <i>(kode: <code>%s</code>)</i>", code)
		} else {
			log.Printf("[%s] [%s] error chat:%d user:%s: %v", b.name, code, chatID, username, err)
			answer = fmt.Sprintf("⚠️ Terjadi kesalahan, silakan coba kembali. <i>(kode: <code>%s</code>)</i>", code)
		}
		if b.humanContact != "" {
			answer += "\nAtau hubungi admin: " + b.humanContact
		}
		if waitMsgID > 0 {
			b.telegramAPI("editMessageText", EditMessageText{ //nolint
				ChatID: chatID, MessageID: waitMsgID, Text: answer, ParseMode: "HTML",
			})
		} else {
			b.telegramAPI("sendMessage", SendMessage{ChatID: chatID, Text: answer, ParseMode: "HTML"}) //nolint
		}
		return
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
	payload := FlowiseRequest{Question: question, ChatID: sessionID}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, b.flowiseURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if b.flowiseKey != "" {
		req.Header.Set("Authorization", "Bearer "+b.flowiseKey)
	}
	resp, err := b.flowiseCli.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("flowise %d: %s", resp.StatusCode, string(raw))
	}
	var fr FlowiseResponse
	if err := json.Unmarshal(raw, &fr); err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	text := strings.TrimSpace(fr.Text)
	if text == "" {
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

func (b *Bot) registerWebhook(webhookURL string) {
	type req struct {
		URL         string `json:"url"`
		SecretToken string `json:"secret_token,omitempty"`
	}
	payload := req{URL: webhookURL}
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
	body, _ := json.Marshal(payload)
	url := fmt.Sprintf("https://api.telegram.org/bot%s/%s", b.token, method)
	resp, err := b.tgClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("telegram %s: %w", method, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("telegram %s %d: %s", method, resp.StatusCode, string(raw))
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
			ChatID: chatID, Text: chunk, ParseMode: "HTML",
		}); err != nil {
			log.Printf("[%s] sendMessage: %v", b.name, err)
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
	text = mdToHTML(text)
	err := b.telegramAPI("editMessageText", EditMessageText{
		ChatID: chatID, MessageID: messageID, Text: text, ParseMode: "HTML",
	})
	if err != nil {
		err = b.telegramAPI("editMessageText", EditMessageText{
			ChatID: chatID, MessageID: messageID, Text: text,
		})
		if err != nil {
			log.Printf("[%s] editMessageText failed: %v", b.name, err)
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
