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

// ── Telegram types ─────────────────────────────────────────────────────────────

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
	if update.Message == nil || strings.TrimSpace(update.Message.Text) == "" {
		return
	}
	go b.processMessage(update.Message)
}

func (b *Bot) processMessage(msg *Message) {
	chatID := msg.Chat.ID
	sessionID := strconv.FormatInt(chatID, 10)

	// Guard: if ownerIDs is non-empty, deny users not in the list.
	if len(b.ownerIDs) > 0 && (msg.From == nil || !b.ownerIDs[msg.From.ID]) {
		b.sendText(chatID, "❌ Akses ditolak. Bot ini hanya untuk owner Ocean Bearings.")
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
