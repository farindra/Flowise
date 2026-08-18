package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"rsc.io/qr"

	_ "modernc.org/sqlite"
)

// botPhones is a process-wide registry of phone numbers that belong to our
// own WA bot sessions. It's used to break bot-to-bot reply loops: if a
// session's own greeting reaches another one of our bots (e.g. both are
// members of the same group, or one has the other saved as a contact), the
// receiving bot must not treat it as a real customer message.
var (
	botPhonesMu sync.RWMutex
	botPhones   = map[string]bool{}
)

func registerBotPhone(phone string) {
	if phone == "" {
		return
	}
	botPhonesMu.Lock()
	botPhones[phone] = true
	botPhonesMu.Unlock()
}

func isBotPhone(phone string) bool {
	botPhonesMu.RLock()
	defer botPhonesMu.RUnlock()
	return botPhones[phone]
}

// sessionRateLimiter is a per-chatId circuit breaker: if one sender crosses
// rateLimitMaxMessages within rateLimitWindow, further messages from them
// are dropped (no Flowise/LLM call) until the window rolls past. This caps
// the damage of ANY runaway loop — bot-to-bot, a misbehaving client, or a
// future bug — not just the specific pairing we found, since Flowise's own
// rate limiter can only key on source IP and all our gateway traffic shares
// one IP.
const (
	rateLimitWindow        = 5 * time.Minute
	rateLimitMaxMessages   = 25
	rateLimitAlertCooldown = 30 * time.Minute

	// pairingCodeValidity bounds how long the QR-refresh loop holds off
	// reconnecting after a phone-pairing code is issued. WhatsApp's own
	// pairing codes stay enterable for a few minutes; this is a safety
	// upper bound, not the real code lifetime.
	pairingCodeValidity = 3 * time.Minute
)

type sessionRateLimiter struct {
	mu      sync.Mutex
	hits    map[string][]time.Time
	alerted map[string]time.Time
}

func newSessionRateLimiter() *sessionRateLimiter {
	return &sessionRateLimiter{hits: map[string][]time.Time{}, alerted: map[string]time.Time{}}
}

// allow records a hit for phone and reports whether it's still under the
// limit, plus the current count in the window (for logging/alerting).
func (rl *sessionRateLimiter) allow(phone string) (ok bool, count int) {
	now := time.Now()
	cutoff := now.Add(-rateLimitWindow)
	rl.mu.Lock()
	defer rl.mu.Unlock()
	kept := rl.hits[phone][:0]
	for _, t := range rl.hits[phone] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	kept = append(kept, now)
	rl.hits[phone] = kept
	count = len(kept)
	return count <= rateLimitMaxMessages, count
}

// shouldAlert reports whether an admin alert for phone should fire now,
// throttled to at most one per rateLimitAlertCooldown.
func (rl *sessionRateLimiter) shouldAlert(phone string) bool {
	now := time.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if last, ok := rl.alerted[phone]; ok && now.Sub(last) < rateLimitAlertCooldown {
		return false
	}
	rl.alerted[phone] = now
	return true
}

type WASession struct {
	id            string
	name          string
	chatflowID    string
	humanContact  string
	allowPhones   map[string]bool
	disableUpload bool

	flowiseBaseURL string
	flowiseAPIKey  string
	timeout        time.Duration

	mu        sync.RWMutex
	waClient  *whatsmeow.Client
	container *sqlstore.Container

	currentQR []byte
	qrReady   chan struct{}
	phone     string

	// pairingInFlight is true while a phone-pairing code is outstanding and
	// unconfirmed. The QR-refresh loop must not reconnect the socket during
	// this window — see PairPhone and runQRFlow.
	pairingInFlight bool

	httpClient  *http.Client
	rateLimiter *sessionRateLimiter
}

func newWASession(r *SessionRecord, flowiseBaseURL, flowiseAPIKey, dataDir string, timeout time.Duration) (*WASession, error) {
	sessionDir := filepath.Join(dataDir, r.ID)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir: %w", err)
	}

	dbPath := filepath.Join(sessionDir, "device.db")
	dbLog := waLog.Stdout("DB", "ERROR", false)
	container, err := sqlstore.New(context.Background(), "sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)", dbLog)
	if err != nil {
		return nil, fmt.Errorf("open sqlstore: %w", err)
	}

	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("get device: %w", err)
	}

	clientLog := waLog.Stdout("WA", "ERROR", false)
	waClient := whatsmeow.NewClient(deviceStore, clientLog)

	phones := parsePhoneSet(r.AllowPhones)

	s := &WASession{
		id:             r.ID,
		name:           r.Name,
		chatflowID:     r.ChatflowID,
		humanContact:   r.HumanContact,
		allowPhones:    phones,
		disableUpload:  r.DisableUpload,
		flowiseBaseURL: flowiseBaseURL,
		flowiseAPIKey:  flowiseAPIKey,
		timeout:        timeout,
		waClient:       waClient,
		container:      container,
		qrReady:        make(chan struct{}),
		httpClient:     &http.Client{Timeout: timeout},
		rateLimiter:    newSessionRateLimiter(),
	}

	waClient.AddEventHandler(s.handleEvent)
	return s, nil
}

func (s *WASession) Connect(ctx context.Context) {
	if s.waClient.Store.ID != nil {
		if err := s.waClient.Connect(); err == nil {
			s.mu.Lock()
			s.phone = s.waClient.Store.ID.User
			s.mu.Unlock()
			registerBotPhone(s.phone)
			return
		}
		// Stored session expired/rejected — clear credentials and fall through to QR flow.
		s.waClient.Disconnect()
		if delErr := s.waClient.Store.Delete(ctx); delErr != nil {
			fmt.Printf("[%s] failed to delete stale store: %v\n", s.name, delErr)
		}
	}
	go s.runQRFlow(ctx)
}

func (s *WASession) runQRFlow(ctx context.Context) {
	store.SetOSInfo(s.name, [3]uint32{10, 0, 0})
	for {
		// Each outer iteration owns its own signal channel to avoid double-close
		// panic when multiple goroutines race through runQRFlow concurrently.
		localCh := make(chan struct{})
		s.mu.Lock()
		s.qrReady = localCh
		s.mu.Unlock()

		qrChan, err := s.waClient.GetQRChannel(ctx)
		if err != nil {
			return
		}
		if err := s.waClient.Connect(); err != nil {
			return
		}

		var localChClosed bool
		timedOut := false

		for evt := range qrChan {
			switch evt.Event {
			case whatsmeow.QRChannelEventCode:
				if code, err := qr.Encode(evt.Code, qr.L); err == nil {
					s.mu.Lock()
					s.currentQR = code.PNG()
					s.mu.Unlock()
				}
				if !localChClosed {
					close(localCh)
					localChClosed = true
				}
			case "success":
				s.mu.Lock()
				s.currentQR = nil
				if s.waClient.Store.ID != nil {
					s.phone = s.waClient.Store.ID.User
				}
				s.mu.Unlock()
				registerBotPhone(s.phone)
				return
			case "timeout":
				s.mu.Lock()
				s.currentQR = nil
				s.mu.Unlock()
				localChClosed = false
				timedOut = true
			}
		}

		if !timedOut {
			return
		}

		// A phone-pairing code may be outstanding — disconnecting now would
		// invalidate its ephemeral key (WA logs "pairing ref mismatch") and
		// force the user to retry, burning through WhatsApp's own
		// pairing-code rate limit. Wait for it to clear first.
		for {
			s.mu.RLock()
			inFlight := s.pairingInFlight
			s.mu.RUnlock()
			if !inFlight {
				break
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(1 * time.Second):
			}
		}

		s.waClient.Disconnect()
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (s *WASession) PairPhone(ctx context.Context, phone string) (string, error) {
	if s.waClient.IsLoggedIn() {
		return "", fmt.Errorf("session sudah terhubung, tidak perlu pair lagi")
	}

	pairCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
	defer cancel()

	// If websocket dropped, trigger reconnect once
	if !s.waClient.IsConnected() {
		s.waClient.Connect() //nolint
	}

	for !s.waClient.IsConnected() {
		select {
		case <-pairCtx.Done():
			return "", fmt.Errorf("websocket tidak tersambung — coba Refresh QR dulu")
		case <-time.After(300 * time.Millisecond):
		}
	}
	code, err := s.waClient.PairPhone(pairCtx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
	if err != nil {
		return "", err
	}

	s.mu.Lock()
	s.pairingInFlight = true
	s.mu.Unlock()
	time.AfterFunc(pairingCodeValidity, func() {
		s.mu.Lock()
		s.pairingInFlight = false
		s.mu.Unlock()
	})

	return code, nil
}

func (s *WASession) QR() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentQR
}

func (s *WASession) StatusInfo() map[string]any {
	s.mu.RLock()
	ph := s.phone
	qrAvail := s.currentQR != nil
	s.mu.RUnlock()

	connected := s.waClient.IsConnected()
	loggedIn := s.waClient.IsLoggedIn()
	statusStr := "offline"
	if connected && loggedIn {
		statusStr = "connected"
	} else if qrAvail {
		statusStr = "qr_pending"
	}

	return map[string]any{
		"id":        s.id,
		"name":      s.name,
		"status":    statusStr,
		"connected": connected,
		"logged_in": loggedIn,
		"phone":     ph,
		"qr_ready":  qrAvail,
	}
}

func (s *WASession) Logout(ctx context.Context) error {
	_ = s.waClient.Logout(ctx)
	s.waClient.Disconnect()
	s.mu.Lock()
	s.currentQR = nil
	s.qrReady = make(chan struct{})
	s.phone = ""
	s.mu.Unlock()
	return nil
}

func (s *WASession) Disconnect() {
	s.waClient.Disconnect()
}

// SendText sends a plain-text WA message to a phone number (format: 628xxx without + or spaces).
func (s *WASession) SendText(ctx context.Context, phone, text string) error {
	jid := types.NewJID(phone, types.DefaultUserServer)
	msg := &waE2E.Message{Conversation: proto.String(text)}
	_, err := s.waClient.SendMessage(ctx, jid, msg)
	return err
}

// handleEvent processes whatsmeow events
func (s *WASession) handleEvent(rawEvt interface{}) {
	switch evt := rawEvt.(type) {
	case *events.Message:
		if evt.Info.IsFromMe {
			return
		}
		go s.handleMessage(evt)
	case *events.Connected:
		s.mu.Lock()
		if s.waClient.Store.ID != nil {
			s.phone = s.waClient.Store.ID.User
		}
		s.mu.Unlock()
		registerBotPhone(s.phone)
		fmt.Printf("[%s] Connected: +%s\n", s.name, s.phone)
	case *events.Disconnected:
		fmt.Printf("[%s] Disconnected\n", s.name)
		// Clear QR so UI doesn't show stale code after disconnect
		if !s.waClient.IsLoggedIn() {
			s.mu.Lock()
			s.currentQR = nil
			s.mu.Unlock()
		}
	case *events.LoggedOut:
		fmt.Printf("[%s] LoggedOut — restarting QR flow\n", s.name)
		s.mu.Lock()
		s.phone = ""
		s.currentQR = nil
		s.qrReady = make(chan struct{})
		s.mu.Unlock()
		go func() {
			time.Sleep(3 * time.Second)
			s.Connect(context.Background())
		}()
	}
}

// normalizeWAPhone strips a leading "+" and rewrites a leading local "0" into
// the Indonesian country code, so callers can pass either form.
func normalizeWAPhone(phone string) string {
	phone = strings.TrimPrefix(strings.TrimSpace(phone), "+")
	if strings.HasPrefix(phone, "0") {
		phone = "62" + phone[1:]
	}
	return phone
}

// SendMessage sends a text message and returns the WhatsApp message ID.
func (s *WASession) SendMessage(ctx context.Context, phone, text string) (string, error) {
	jid, err := parseJID(normalizeWAPhone(phone))
	if err != nil {
		return "", fmt.Errorf("invalid phone: %w", err)
	}
	msg := &waE2E.Message{Conversation: proto.String(mdToWA(text))}
	resp, err := s.waClient.SendMessage(ctx, jid, msg)
	if err != nil {
		return "", err
	}
	return resp.ID, nil
}

const errCodeChars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func newErrorCode() string {
	b := make([]byte, 5)
	for i := range b {
		b[i] = errCodeChars[rand.Intn(len(errCodeChars))]
	}
	return string(b)
}

func parseJID(phone string) (types.JID, error) {
	return types.ParseJID(phone + "@s.whatsapp.net")
}

// reWAPhone matches an already-usable Indonesian WhatsApp number.
var reWAPhone = regexp.MustCompile(`^62[0-9]{8,13}$`)

// ResolveLIDs maps WhatsApp LIDs to real phone numbers using this session's
// local LID store. Identifiers that already look like phone numbers come back
// in `passthrough` rather than `resolved`, because callers building a broadcast
// audience must be able to tell the two apart — anything that merely looks
// numeric may be from another platform (e.g. a Telegram user ID that shares the
// chatflow) and should not be messaged by default.
func (s *WASession) ResolveLIDs(ctx context.Context, ids []string) (resolved, passthrough map[string]string, unresolved []string) {
	resolved = map[string]string{}
	passthrough = map[string]string{}
	unresolved = []string{}

	// A session that has never completed pairing has no LID store yet, so
	// every identifier is simply unresolvable here rather than an error.
	hasLIDStore := s.waClient != nil && s.waClient.Store != nil && s.waClient.Store.LIDs != nil

	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if hasLIDStore {
			jid := types.JID{User: id, Server: types.HiddenUserServer}
			if pn, err := s.waClient.Store.LIDs.GetPNForLID(ctx, jid); err == nil && pn.User != "" {
				resolved[id] = pn.User
				continue
			}
		}
		if reWAPhone.MatchString(id) {
			passthrough[id] = id
			continue
		}
		unresolved = append(unresolved, id)
	}
	return resolved, passthrough, unresolved
}

func (s *WASession) handleMessage(evt *events.Message) {
	text := extractText(evt.Message)
	imgMsg := evt.Message.GetImageMessage()
	if text == "" && imgMsg == nil {
		return
	}

	senderPhone := evt.Info.Sender.User

	// Loop guard: resolve LID senders to their real phone number and drop
	// the message if it's coming from one of our own bot numbers. Without
	// this, two of our bots that have each other as a contact (or share a
	// group) will auto-reply to each other's greeting forever, burning API
	// quota non-stop.
	resolvedPhone := senderPhone
	if evt.Info.Sender.Server == types.HiddenUserServer {
		if pn, err := s.waClient.Store.LIDs.GetPNForLID(context.Background(), evt.Info.Sender); err == nil && pn.User != "" {
			resolvedPhone = pn.User
		}
	}
	if isBotPhone(resolvedPhone) {
		fmt.Printf("[%s] loop guard: dropping message from own bot number %s (lid sender %s)\n", s.name, resolvedPhone, senderPhone)
		return
	}

	if ok, count := s.rateLimiter.allow(resolvedPhone); !ok {
		fmt.Printf("[%s] rate limit: %s sent %d msgs in %s, dropping\n", s.name, resolvedPhone, count, rateLimitWindow)
		if s.humanContact != "" && s.rateLimiter.shouldAlert(resolvedPhone) {
			go func() {
				alertCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				alertMsg := fmt.Sprintf("⚠️ [%s] Sesi %s kirim %d+ pesan dalam %s — auto-pause sementara (kemungkinan loop/spam). Cek manual kalau perlu.", s.name, resolvedPhone, count, rateLimitWindow)
				_ = s.SendText(alertCtx, s.humanContact, alertMsg)
			}()
		}
		return
	}

	if len(s.allowPhones) > 0 && !s.allowPhones[resolvedPhone] {
		return
	}

	// Broadcast opt-out. Handled before the Flowise call so an unsubscribe is
	// never answered by the sales agent, and recorded against resolvedPhone
	// because that is the number a broadcast would target.
	if isStopKeyword(text) {
		go postOptOut(resolvedPhone, s.name)
		stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer stopCancel()
		if _, err := s.SendMessage(stopCtx, resolvedPhone, optOutReply); err != nil {
			fmt.Printf("[%s] opt-out confirmation to %s failed: %v\n", s.name, resolvedPhone, err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	var uploads []map[string]any
	if imgMsg != nil && !s.disableUpload {
		if text == "" {
			text = imgMsg.GetCaption()
		}
		if data, err := s.waClient.Download(ctx, imgMsg); err != nil {
			fmt.Printf("[%s] image download error from %s: %v\n", s.name, senderPhone, err)
		} else {
			mime := imgMsg.GetMimetype()
			if mime == "" {
				mime = "image/jpeg"
			}
			uploads = []map[string]any{{
				"data": "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data),
				"type": "file",
				"name": "image." + strings.TrimPrefix(mime, "image/"),
				"mime": mime,
			}}
		}
	}

	// Use resolvedPhone (not the raw LID) as the Flowise chatId. The agent
	// prompt tries to read the customer's phone number straight from this
	// value to auto-run customer_tier_lookup — a LID like "219464406679598"
	// doesn't look like a phone number, so with the raw senderPhone the agent
	// always fell back to asking the customer for their number even though
	// go-wa already knew it.
	reply, err := s.callFlowise(ctx, text, resolvedPhone, uploads)
	if err != nil {
		code := newErrorCode()
		if ctx.Err() != nil {
			fmt.Printf("[%s] [%s] timeout (%s)\n", s.name, code, resolvedPhone)
			reply = fmt.Sprintf("🔴 Server sedang sibuk, coba lagi nanti. (kode: %s)", code)
		} else {
			fmt.Printf("[%s] [%s] error (%s): %v\n", s.name, code, senderPhone, err)
			reply = fmt.Sprintf("⚠️ Terjadi kesalahan, silakan coba kembali. (kode: %s)", code)
		}
		if s.humanContact != "" {
			reply += "\nAtau hubungi admin: " + s.humanContact
		}
	}
	if reply == "" {
		return
	}

	msg := &waE2E.Message{Conversation: proto.String(mdToWA(reply))}
	// Use a fresh context for send — the flowise ctx may already be expired on timeout.
	sendCtx, sendCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer sendCancel()
	if _, err := s.waClient.SendMessage(sendCtx, evt.Info.Chat, msg); err != nil {
		fmt.Printf("[%s] Send error: %v\n", s.name, err)
	}
}

func (s *WASession) callFlowise(ctx context.Context, question, sessionID string, uploads []map[string]any) (string, error) {
	url := s.flowiseBaseURL + "/api/v1/prediction/" + s.chatflowID
	payload := map[string]any{
		"question": question,
		"chatId":   sessionID,
	}
	if len(uploads) > 0 {
		payload["uploads"] = uploads
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if s.flowiseAPIKey != "" {
		req.Header.Set("Authorization", "Bearer "+s.flowiseAPIKey)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("flowise %d: %s", resp.StatusCode, string(data))
	}

	var result struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return string(data), nil
	}
	return result.Text, nil
}

var (
	waReBold    = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	waReItalic  = regexp.MustCompile(`(?m)(?:^|[^*])\*([^*\n]+)\*(?:[^*]|$)`)
	waReHeading = regexp.MustCompile(`(?m)^#{1,3}\s+(.+)$`)
	waReBullet  = regexp.MustCompile(`(?m)^\*[ \t]+`)
	waReLink    = regexp.MustCompile(`\[([^\]]+)\]\s*\(([^)]+)\)`)
	waReHRule   = regexp.MustCompile(`(?m)^---+$`)
)

// mdToWA converts Markdown to WhatsApp-compatible format.
// WhatsApp: *bold*, _italic_, ~strike~, `mono`
func mdToWA(s string) string {
	s = waReHeading.ReplaceAllString(s, "*$1*")
	s = waReBold.ReplaceAllString(s, "*$1*")
	s = waReBullet.ReplaceAllString(s, "• ")
	s = waReLink.ReplaceAllStringFunc(s, func(m string) string {
		parts := waReLink.FindStringSubmatch(m)
		if len(parts) < 3 {
			return m
		}
		text := strings.TrimSpace(parts[1])
		url := strings.TrimSpace(parts[2])
		if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
			return text + ": " + url
		}
		return text
	})
	s = waReHRule.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// extractText gets plain text from any message type
func extractText(msg *waE2E.Message) string {
	if msg == nil {
		return ""
	}
	if t := msg.GetConversation(); t != "" {
		return t
	}
	if ext := msg.GetExtendedTextMessage(); ext != nil {
		return ext.GetText()
	}
	return ""
}

func parsePhoneSet(s string) map[string]bool {
	m := map[string]bool{}
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			m[p] = true
		}
	}
	return m
}
