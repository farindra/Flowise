package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	"rsc.io/qr"

	_ "modernc.org/sqlite"
)

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

	httpClient *http.Client
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
		}
		return
	}
	go s.runQRFlow(ctx)
}

func (s *WASession) runQRFlow(ctx context.Context) {
	for {
		qrChan, err := s.waClient.GetQRChannel(ctx)
		if err != nil {
			return
		}
		if err := s.waClient.Connect(); err != nil {
			return
		}

		var qrReadyClosed bool
		timedOut := false

		for evt := range qrChan {
			switch evt.Event {
			case whatsmeow.QRChannelEventCode:
				if code, err := qr.Encode(evt.Code, qr.L); err == nil {
					s.mu.Lock()
					s.currentQR = code.PNG()
					s.mu.Unlock()
				}
				if !qrReadyClosed {
					close(s.qrReady)
					qrReadyClosed = true
				}
			case "success":
				s.mu.Lock()
				s.currentQR = nil
				if s.waClient.Store.ID != nil {
					s.phone = s.waClient.Store.ID.User
				}
				s.mu.Unlock()
				return
			case "timeout":
				s.mu.Lock()
				s.currentQR = nil
				s.qrReady = make(chan struct{})
				s.mu.Unlock()
				qrReadyClosed = false
				timedOut = true
			}
		}

		if !timedOut {
			return
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
	select {
	case <-s.qrReady:
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(60 * time.Second):
		return "", fmt.Errorf("timeout waiting for QR channel (start connect first)")
	}
	return s.waClient.PairPhone(ctx, phone, true, whatsmeow.PairClientChrome, "Chrome (Linux)")
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
		fmt.Printf("[%s] Connected: +%s\n", s.name, s.phone)
	case *events.Disconnected:
		fmt.Printf("[%s] Disconnected\n", s.name)
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

func (s *WASession) handleMessage(evt *events.Message) {
	text := extractText(evt.Message)
	if text == "" {
		return
	}

	senderPhone := evt.Info.Sender.User
	if len(s.allowPhones) > 0 && !s.allowPhones[senderPhone] {
		return
	}

	sessionID := senderPhone + "@" + s.id
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	reply, err := s.callFlowise(ctx, text, sessionID)
	if err != nil {
		fmt.Printf("[%s] Flowise error (%s): %v\n", s.name, senderPhone, err)
		if s.humanContact != "" {
			reply = fmt.Sprintf("Maaf, terjadi gangguan. Silakan hubungi %s", s.humanContact)
		} else {
			return
		}
	}
	if reply == "" {
		return
	}

	msg := &waE2E.Message{Conversation: proto.String(mdToWA(reply))}
	if _, err := s.waClient.SendMessage(ctx, evt.Info.Chat, msg); err != nil {
		fmt.Printf("[%s] Send error: %v\n", s.name, err)
	}
}

func (s *WASession) callFlowise(ctx context.Context, question, sessionID string) (string, error) {
	url := s.flowiseBaseURL + "/api/v1/prediction/" + s.chatflowID
	body, _ := json.Marshal(map[string]any{
		"question": question,
		"chatId":   sessionID,
	})

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
