package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"strings"
	"time"
)

// whatsmeowProvider talks to the self-hosted go-wa gateway.
type whatsmeowProvider struct {
	baseURL string
	key     string
	client  *http.Client
}

func newWhatsmeowProvider() *whatsmeowProvider {
	return &whatsmeowProvider{
		baseURL: strings.TrimRight(envOr("WA_SERVICE_URL", "http://127.0.0.1:8082"), "/"),
		key:     envOr("WA_INTERNAL_KEY", "ob-wa-internal-2026"),
		// Generous: go-wa performs the WhatsApp round trip inline, and a media
		// send also uploads to WhatsApp's servers first.
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (p *whatsmeowProvider) Name() string { return "whatsmeow" }

func (p *whatsmeowProvider) Capabilities() Capabilities {
	return Capabilities{FreeText: true, Media: true, RequiresTemplate: false, ResolvesIDs: true}
}

func (p *whatsmeowProvider) do(req *http.Request) (int, []byte, error) {
	req.Header.Set("X-Internal-Key", p.key)
	resp, err := p.client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	return resp.StatusCode, body, nil
}

func (p *whatsmeowProvider) Senders(ctx context.Context) ([]SenderInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+"/api/sessions", nil)
	if err != nil {
		return nil, err
	}
	code, body, err := p.do(req)
	if err != nil {
		return nil, fmt.Errorf("go-wa tidak bisa dihubungi: %w", err)
	}
	if code != http.StatusOK {
		return nil, fmt.Errorf("go-wa HTTP %d: %s", code, string(body))
	}
	var raw []struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Phone  string `json:"phone"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	out := make([]SenderInfo, 0, len(raw))
	for _, s := range raw {
		out = append(out, SenderInfo{ID: s.ID, Label: s.Name, Phone: s.Phone, Status: s.Status})
	}
	return out, nil
}

func (p *whatsmeowProvider) Ready(ctx context.Context, sender string) error {
	if sender == "" {
		return fmt.Errorf("pengirim belum dipilih")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		p.baseURL+"/api/sessions/"+sender+"/status", nil)
	if err != nil {
		return err
	}
	code, body, err := p.do(req)
	if err != nil {
		return fmt.Errorf("go-wa tidak bisa dihubungi: %w", err)
	}
	if code != http.StatusOK {
		return fmt.Errorf("sesi WA tidak ditemukan (HTTP %d)", code)
	}
	var st struct {
		Status string `json:"status"`
		Phone  string `json:"phone"`
	}
	if err := json.Unmarshal(body, &st); err != nil {
		return err
	}
	if st.Status != "connected" {
		return fmt.Errorf("sesi WA tidak terhubung (status: %s)", st.Status)
	}
	return nil
}

// classify maps a go-wa response onto retry semantics.
func classify(code int, body string) *SendError {
	err := fmt.Errorf("go-wa HTTP %d: %s", code, body)
	switch {
	case code == http.StatusNotFound:
		// Session vanished — every subsequent recipient would fail the same
		// way, so stop the campaign instead of burning the queue.
		return &SendError{Err: err, Retryable: true, FatalForSender: true}
	case code == http.StatusBadRequest && strings.Contains(strings.ToLower(body), "invalid phone"):
		return &SendError{Err: err, Retryable: false}
	case code >= 500:
		return &SendError{Err: err, Retryable: true}
	case code >= 400:
		return &SendError{Err: err, Retryable: false}
	}
	return nil
}

func (p *whatsmeowProvider) Send(ctx context.Context, sender, phone string, msg OutboundMessage) (SendResult, error) {
	if len(msg.Media) > 0 {
		caption := msg.Text
		if msg.Separate {
			caption = ""
		}
		res, err := p.sendMedia(ctx, sender, phone, caption, msg)
		if err != nil {
			return res, err
		}
		if !msg.Separate || strings.TrimSpace(msg.Text) == "" {
			return res, nil
		}
		// Caption too long for one message: the image went first, the text
		// follows as its own message. The recipient counts once against the
		// daily cap but consumes two sends.
		return p.sendText(ctx, sender, phone, msg.Text)
	}
	return p.sendText(ctx, sender, phone, msg.Text)
}

func (p *whatsmeowProvider) sendText(ctx context.Context, sender, phone, text string) (SendResult, error) {
	body, _ := json.Marshal(map[string]string{"phone": phone, "message": text})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/api/sessions/"+sender+"/send", bytes.NewReader(body))
	if err != nil {
		return SendResult{}, &SendError{Err: err, Retryable: false}
	}
	req.Header.Set("Content-Type", "application/json")

	code, respBody, err := p.do(req)
	if err != nil {
		// Connection error or timeout: the message may in fact have been sent.
		// Retryable, and the claim bookkeeping caps how often we can try.
		return SendResult{}, &SendError{Err: err, Retryable: true}
	}
	if se := classify(code, string(respBody)); se != nil {
		return SendResult{}, se
	}
	var res struct {
		MessageID string `json:"message_id"`
	}
	_ = json.Unmarshal(respBody, &res)
	return SendResult{ProviderMsgID: res.MessageID, SentAt: time.Now()}, nil
}

func (p *whatsmeowProvider) sendMedia(ctx context.Context, sender, phone, caption string, msg OutboundMessage) (SendResult, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("phone", phone)
	_ = mw.WriteField("caption", caption)

	name := msg.MediaName
	if name == "" {
		name = "image"
	}

	// CreateFormFile would hard-code application/octet-stream for the part,
	// which the receiving side rejects. Declare the real mime instead.
	partHeader := make(textproto.MIMEHeader)
	partHeader.Set("Content-Disposition",
		fmt.Sprintf(`form-data; name="file"; filename=%q`, name))
	mediaMime := msg.MediaMime
	if mediaMime == "" {
		mediaMime = http.DetectContentType(msg.Media)
	}
	partHeader.Set("Content-Type", mediaMime)

	part, err := mw.CreatePart(partHeader)
	if err != nil {
		return SendResult{}, &SendError{Err: err, Retryable: false}
	}
	if _, err := part.Write(msg.Media); err != nil {
		return SendResult{}, &SendError{Err: err, Retryable: false}
	}
	if err := mw.Close(); err != nil {
		return SendResult{}, &SendError{Err: err, Retryable: false}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/api/sessions/"+sender+"/send-media", &buf)
	if err != nil {
		return SendResult{}, &SendError{Err: err, Retryable: false}
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())

	code, respBody, err := p.do(req)
	if err != nil {
		return SendResult{}, &SendError{Err: err, Retryable: true}
	}
	if se := classify(code, string(respBody)); se != nil {
		return SendResult{}, se
	}
	var res struct {
		MessageID string `json:"message_id"`
	}
	_ = json.Unmarshal(respBody, &res)
	return SendResult{ProviderMsgID: res.MessageID, SentAt: time.Now()}, nil
}

// ResolveIDs maps WhatsApp LIDs to phone numbers. Only `resolved` is returned —
// go-wa's `passthrough` bucket (identifiers that merely look like phone
// numbers) is deliberately dropped here, because that is where identifiers from
// other platforms sharing the same chatflow would leak in. Callers that want
// them ask for them explicitly via ResolveIDsWithPassthrough.
func (p *whatsmeowProvider) ResolveIDs(ctx context.Context, sender string, ids []string) (map[string]string, []string, error) {
	resolved, _, unresolved, err := p.ResolveIDsWithPassthrough(ctx, sender, ids)
	return resolved, unresolved, err
}

func (p *whatsmeowProvider) ResolveIDsWithPassthrough(ctx context.Context, sender string, ids []string) (resolved, passthrough map[string]string, unresolved []string, err error) {
	if len(ids) == 0 {
		return map[string]string{}, map[string]string{}, nil, nil
	}
	body, _ := json.Marshal(map[string][]string{"ids": ids})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/api/sessions/"+sender+"/resolve-lids", bytes.NewReader(body))
	if err != nil {
		return nil, nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	code, respBody, err := p.do(req)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("go-wa tidak bisa dihubungi: %w", err)
	}
	if code != http.StatusOK {
		return nil, nil, nil, fmt.Errorf("go-wa HTTP %d: %s", code, string(respBody))
	}
	var res struct {
		Resolved    map[string]string `json:"resolved"`
		Passthrough map[string]string `json:"passthrough"`
		Unresolved  []string          `json:"unresolved"`
	}
	if err := json.Unmarshal(respBody, &res); err != nil {
		return nil, nil, nil, err
	}
	if res.Resolved == nil {
		res.Resolved = map[string]string{}
	}
	if res.Passthrough == nil {
		res.Passthrough = map[string]string{}
	}
	return res.Resolved, res.Passthrough, res.Unresolved, nil
}
