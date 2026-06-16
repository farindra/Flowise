package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// AnalyzeResult mirrors ai-vision-service/internal/vision/analyze.go's
// AnalyzeResult (response of POST /analyze-image).
type AnalyzeResult struct {
	Products    []string `json:"products"`
	Codes       []string `json:"codes"`
	Brands      []string `json:"brands,omitempty"`
	Confidence  float64  `json:"confidence"`
	Description string   `json:"description"`
	Error       string   `json:"error,omitempty"`
	Fallback    bool     `json:"fallback,omitempty"`
}

// MultiProductResult mirrors ai-vision-service/internal/vision/multiproduct.go's
// MultiProductResult (response of POST /parse-multi-product).
type MultiProductResult struct {
	IsMultiProduct bool     `json:"isMultiProduct"`
	Products       []string `json:"products"`
	Confidence     float64  `json:"confidence"`
	Method         string   `json:"method"`
}

// MessageAnalysis mirrors ai-vision-service/internal/vision/message.go's
// MessageAnalysis (response of POST /analyze-message).
type MessageAnalysis struct {
	Keywords          []string `json:"keywords"`
	Intent            string   `json:"intent,omitempty"`
	Products          []string `json:"products,omitempty"`
	Quantity          int      `json:"quantity,omitempty"`
	ContainsProfanity bool     `json:"containsProfanity"`
	EnhancedQuery     string   `json:"enhancedQuery"`
	OriginalMessage   string   `json:"originalMessage"`
}

// AIVisionClient talks to ai-vision-service.
type AIVisionClient struct {
	baseURL string
	http    *http.Client
}

func NewAIVisionClient(baseURL string) *AIVisionClient {
	return &AIVisionClient{baseURL: strings.TrimSuffix(baseURL, "/"), http: &http.Client{}}
}

func (c *AIVisionClient) postJSON(ctx context.Context, path string, req, out interface{}) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var eb errorBody
		_ = json.NewDecoder(resp.Body).Decode(&eb)
		return fmt.Errorf("ai-vision: POST %s status %d: %s", path, resp.StatusCode, eb.Error)
	}

	return json.NewDecoder(resp.Body).Decode(out)
}

type analyzeImageRequest struct {
	Image       string `json:"image"`
	MimeType    string `json:"mimeType,omitempty"`
	PhoneNumber string `json:"phoneNumber"`
}

// AnalyzeImage calls POST /analyze-image, the port of aiService.analyzeImage().
func (c *AIVisionClient) AnalyzeImage(ctx context.Context, imageBase64, mimeType, phoneNumber string) (*AnalyzeResult, error) {
	var result AnalyzeResult
	req := analyzeImageRequest{Image: imageBase64, MimeType: mimeType, PhoneNumber: phoneNumber}
	if err := c.postJSON(ctx, "/analyze-image", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

type textPhoneRequest struct {
	Text        string `json:"text"`
	PhoneNumber string `json:"phoneNumber"`
}

// ParseMultiProduct calls POST /parse-multi-product, the port of
// aiService.parseMultiProductWithAI().
func (c *AIVisionClient) ParseMultiProduct(ctx context.Context, text, phoneNumber string) (*MultiProductResult, error) {
	var result MultiProductResult
	req := textPhoneRequest{Text: text, PhoneNumber: phoneNumber}
	if err := c.postJSON(ctx, "/parse-multi-product", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// AnalyzeMessage calls POST /analyze-message, the port of
// aiService.analyzeMessage().
func (c *AIVisionClient) AnalyzeMessage(ctx context.Context, message, phoneNumber string) (*MessageAnalysis, error) {
	var result MessageAnalysis
	req := textPhoneRequest{Text: message, PhoneNumber: phoneNumber}
	if err := c.postJSON(ctx, "/analyze-message", req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

type naturalChatRequest struct {
	Message      string   `json:"message"`
	PhoneNumber  string   `json:"phoneNumber"`
	CustomerName string   `json:"customerName,omitempty"`
	History      []string `json:"history,omitempty"`
	IsGreeting   bool     `json:"isGreeting"`
	IsFirstTime  bool     `json:"isFirstTime"`
}

type naturalChatResponse struct {
	Response string `json:"response"`
}

// GenerateNatural calls POST /generate-natural, porting
// aiService.generateNaturalGreeting() and aiService.generateNaturalResponse().
// It never returns an error — on failure the service returns a hardcoded fallback.
func (c *AIVisionClient) GenerateNatural(ctx context.Context, message, phoneNumber, customerName string, history []string, isGreeting, isFirstTime bool) string {
	var resp naturalChatResponse
	req := naturalChatRequest{
		Message:      message,
		PhoneNumber:  phoneNumber,
		CustomerName: customerName,
		History:      history,
		IsGreeting:   isGreeting,
		IsFirstTime:  isFirstTime,
	}
	if err := c.postJSON(ctx, "/generate-natural", req, &resp); err != nil {
		return ""
	}
	return resp.Response
}
