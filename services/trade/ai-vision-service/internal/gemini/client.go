package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	apiKey      string
	model       string
	baseURL     string
	maxTokens   int
	temperature float64
	httpClient  *http.Client
}

func New(apiKey, model, baseURL string, maxTokens int, temperature float64) *Client {
	return &Client{
		apiKey:      apiKey,
		model:       model,
		baseURL:     strings.TrimSuffix(baseURL, "/"),
		maxTokens:   maxTokens,
		temperature: temperature,
		httpClient:  &http.Client{},
	}
}

type Part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *InlineData `json:"inlineData,omitempty"`
}

type InlineData struct {
	MimeType string `json:"mimeType"`
	Data     string `json:"data"`
}

type requestBody struct {
	Contents         []requestContent `json:"contents"`
	GenerationConfig genConfig        `json:"generationConfig"`
}

type requestContent struct {
	Parts []Part `json:"parts"`
}

type genConfig struct {
	Temperature     float64 `json:"temperature"`
	MaxOutputTokens int     `json:"maxOutputTokens"`
}

type responseBody struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
}

// GenerateContent calls the Gemini generateContent REST endpoint with the given
// parts (text and/or inline image data) and returns the response text.
func (c *Client) GenerateContent(ctx context.Context, parts []Part) (string, error) {
	reqBody := requestBody{
		Contents:         []requestContent{{Parts: parts}},
		GenerationConfig: genConfig{Temperature: c.temperature, MaxOutputTokens: c.maxTokens},
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	url := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, c.model, c.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("gemini API error: status %d: %s", resp.StatusCode, string(body))
	}

	var parsed responseBody
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("gemini API: failed to parse response: %w", err)
	}

	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini API: no response candidates")
	}

	return parsed.Candidates[0].Content.Parts[0].Text, nil
}

// IsOverloaded reports whether err looks like a Gemini 503/overloaded error,
// which the original bot retries with exponential backoff.
func IsOverloaded(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "503") || strings.Contains(strings.ToLower(msg), "overloaded")
}
