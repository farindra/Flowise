package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"time"
)

// TradeClient calls the TRADE bot-integration API.
type TradeClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewTradeClient(baseURL, apiKey string) *TradeClient {
	return &TradeClient{
		baseURL: baseURL,
		apiKey:  apiKey,
		http:    &http.Client{Timeout: 120 * time.Second},
	}
}

type SupplierOfferUploadResult struct {
	UploadID     string `json:"upload_id"`
	Status       string `json:"status"`
	Message      string `json:"message"`
	SupplierName string `json:"supplier_name"`
	Currency     string `json:"currency"`
}

// UploadSupplierOffer sends an Excel file to TRADE for background processing.
// supplierName and currency are optional (can be empty → TRADE auto-detects).
func (c *TradeClient) UploadSupplierOffer(ctx context.Context, fileData []byte, filename, supplierName, currency string) (*SupplierOfferUploadResult, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	part, err := w.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	if supplierName != "" {
		_ = w.WriteField("supplier_name", supplierName)
	}
	if currency != "" {
		_ = w.WriteField("currency", currency)
	} else {
		_ = w.WriteField("currency", "USD")
	}
	w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/bot-integration/owner/upload-supplier-offer", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("X-API-Key", c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trade api %d: %s", resp.StatusCode, string(raw))
	}

	var result SupplierOfferUploadResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}
