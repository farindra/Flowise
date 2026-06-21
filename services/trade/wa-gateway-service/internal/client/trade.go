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

func (c *TradeClient) BaseURL() string { return c.baseURL }
func (c *TradeClient) APIKey() string  { return c.apiKey }

// TradeDataPreview is the response from /owner/preview-trade-data.
type TradeDataPreview struct {
	TotalRows       int      `json:"total_rows"`
	ValidRows       int      `json:"valid_rows"`
	InvalidRows     int      `json:"invalid_rows"`
	ColumnsFound    []string `json:"columns_found"`
	MissingRequired []string `json:"missing_required"`
	HasPrice        bool     `json:"has_price"`
	Errors          []struct {
		Row    int      `json:"row"`
		Errors []string `json:"errors"`
	} `json:"errors"`
}

// TradeDataImportResult is the response from /owner/import-trade-data.
type TradeDataImportResult struct {
	ImportID    string `json:"import_id"`
	Status      string `json:"status"`
	TotalRows   int    `json:"total_rows"`
	ValidRows   int    `json:"valid_rows"`
	InvalidRows int    `json:"invalid_rows"`
}

func (c *TradeClient) postMultipart(ctx context.Context, path string, fileData []byte, filename string) ([]byte, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, &buf)
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
		msg := string(raw)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, fmt.Errorf("trade api %d: %s", resp.StatusCode, msg)
	}
	return raw, nil
}

// PreviewTradeData runs a dry-run validation on an Excel/CSV file.
func (c *TradeClient) PreviewTradeData(ctx context.Context, fileData []byte, filename string) (*TradeDataPreview, error) {
	raw, err := c.postMultipart(ctx, "/api/v1/bot-integration/owner/preview-trade-data", fileData, filename)
	if err != nil {
		return nil, err
	}
	var result TradeDataPreview
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

// ImportTradeData imports an Excel/CSV file into TRADE trade data.
func (c *TradeClient) ImportTradeData(ctx context.Context, fileData []byte, filename string) (*TradeDataImportResult, error) {
	raw, err := c.postMultipart(ctx, "/api/v1/bot-integration/owner/import-trade-data", fileData, filename)
	if err != nil {
		return nil, err
	}
	var result TradeDataImportResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

// UnrecordedItemsPreview is the response from /owner/preview-unrecorded-items.
type UnrecordedItemsPreview struct {
	TotalRows   int      `json:"total_rows"`
	ValidRows   int      `json:"valid_rows"`
	InvalidRows int      `json:"invalid_rows"`
	ColumnsFound []string `json:"columns_found"`
	HasNameCol  bool     `json:"has_name_col"`
	Errors      []struct {
		Row    int      `json:"row"`
		Errors []string `json:"errors"`
	} `json:"errors"`
	Sample []map[string]string `json:"sample"`
}

// UnrecordedItemsImportResult is the response from /owner/import-unrecorded-items.
type UnrecordedItemsImportResult struct {
	Status      string `json:"status"`
	Created     int    `json:"created"`
	TotalRows   int    `json:"total_rows"`
	InvalidRows int    `json:"invalid_rows"`
	WeekLabel   string `json:"week_label"`
	Source      string `json:"source"`
	Message     string `json:"message"`
}

// PreviewUnrecordedItems runs a dry-run validation on a Permintaan Barang file.
func (c *TradeClient) PreviewUnrecordedItems(ctx context.Context, fileData []byte, filename string) (*UnrecordedItemsPreview, error) {
	raw, err := c.postMultipart(ctx, "/api/v1/bot-integration/owner/preview-unrecorded-items", fileData, filename)
	if err != nil {
		return nil, err
	}
	var result UnrecordedItemsPreview
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
}

// ImportUnrecordedItems imports Permintaan Barang file into TRADE + triggers mapping.
func (c *TradeClient) ImportUnrecordedItems(ctx context.Context, fileData []byte, filename, source string) (*UnrecordedItemsImportResult, error) {
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, err := w.CreateFormFile("file", filepath.Base(filename))
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(fileData); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	_ = w.WriteField("source", source)
	w.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/bot-integration/owner/import-unrecorded-items", &buf)
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
		msg := string(raw)
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return nil, fmt.Errorf("trade api %d: %s", resp.StatusCode, msg)
	}
	var result UnrecordedItemsImportResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return &result, nil
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
