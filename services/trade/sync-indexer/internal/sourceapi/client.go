// Package sourceapi fetches the raw product list from the
// api.oceanbearings.co.id "PRODUK_API_URL"/"PRODUK_API_URL2" endpoints.
package sourceapi

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"time"

	"sync-indexer/internal/transform"
)

type Client struct {
	httpClient *http.Client
}

func New() *Client {
	return &Client{httpClient: &http.Client{}}
}

// FetchProducts - ported verbatim from fetchWithRetry in
// local-sync-system.js: 3 attempts, 15s timeout per attempt, exponential
// backoff (2^attempt seconds) between attempts.
func (c *Client) FetchProducts(ctx context.Context, url string) ([]transform.RawProduct, error) {
	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		data, err := c.doGet(ctx, url, 15*time.Second)
		if err == nil {
			return data, nil
		}
		lastErr = err

		if attempt == maxRetries {
			break
		}

		wait := time.Duration(math.Pow(2, float64(attempt))) * time.Second
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

func (c *Client) doGet(ctx context.Context, url string, timeout time.Duration) ([]transform.RawProduct, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Ocean-Bearing-Bot/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	var data []transform.RawProduct
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}
