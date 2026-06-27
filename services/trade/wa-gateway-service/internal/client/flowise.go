package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// FlowiseClient calls a Flowise chatflow endpoint to generate natural
// conversation responses. It replaces ai-vision-service's /generate-natural
// (Gemini) for greeting and free-form chat. All business logic (product
// search, checkout, cart) stays in wa-gateway-service — Flowise only handles
// open-ended conversation.
type FlowiseClient struct {
	baseURL    string
	chatflowID string
	apiKey     string // optional — for Flowise instances with auth enabled
	http       *http.Client
}

// NewFlowiseClient creates a FlowiseClient. apiKey may be empty if the
// Flowise instance does not require authentication.
func NewFlowiseClient(baseURL, chatflowID, apiKey string) *FlowiseClient {
	return &FlowiseClient{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		chatflowID: chatflowID,
		apiKey:     apiKey,
		http:       &http.Client{Timeout: 90 * time.Second},
	}
}

// flowiseMessage is one turn in Flowise's history format.
type flowiseMessage struct {
	Role    string `json:"role"`    // "userMessage" or "apiMessage"
	Content string `json:"content"`
}

type flowiseRequest struct {
	Question  string           `json:"question"`
	SessionID string           `json:"sessionId,omitempty"`
	History   []flowiseMessage `json:"history,omitempty"`
}

type flowiseResponse struct {
	Text string `json:"text"`
}

// GenerateNatural sends a message to Flowise and returns the response text.
// It has the same signature as AIVisionClient.GenerateNatural so callers can
// swap implementations without change.
// Returns "" on any error so callers can fall back to Gemini.
func (c *FlowiseClient) GenerateNatural(ctx context.Context, message, phoneNumber, customerName string, history []string, isGreeting, isFirstTime bool) string {
	q := message
	if isGreeting && customerName != "" {
		q = fmt.Sprintf("[greeting from %s] %s", customerName, message)
	}

	req := flowiseRequest{
		Question:  q,
		SessionID: phoneNumber,
		History:   convertHistory(history),
	}

	body, err := json.Marshal(req)
	if err != nil {
		return ""
	}

	url := fmt.Sprintf("%s/api/v1/prediction/%s", c.baseURL, c.chatflowID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ""
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var out flowiseResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ""
	}
	return strings.TrimSpace(out.Text)
}

// AskDirect sends a raw question to this client's chatflow, returns the text
// response. No history, no decoration — used by owner assistant commands.
func (c *FlowiseClient) AskDirect(ctx context.Context, question, sessionID string) string {
	type req struct {
		Question  string `json:"question"`
		SessionID string `json:"sessionId,omitempty"`
	}
	body, err := json.Marshal(req{Question: question, SessionID: sessionID})
	if err != nil {
		return ""
	}
	url := fmt.Sprintf("%s/api/v1/prediction/%s", c.baseURL, c.chatflowID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return ""
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	var out flowiseResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return ""
	}
	return strings.TrimSpace(out.Text)
}

// convertHistory converts wa-gateway's []string history (alternating
// user/assistant entries) to Flowise's [{role, content}] format.
func convertHistory(history []string) []flowiseMessage {
	if len(history) == 0 {
		return nil
	}
	msgs := make([]flowiseMessage, 0, len(history))
	for i, h := range history {
		role := "userMessage"
		if i%2 == 1 {
			role = "apiMessage"
		}
		msgs = append(msgs, flowiseMessage{Role: role, Content: h})
	}
	return msgs
}
