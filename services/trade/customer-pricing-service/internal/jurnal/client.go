package jurnal

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to the Jurnal.id partner API used by local-sync-system.js
// (fetchAllCustomers / fetchSinglePage / fetchWithRetry / fetchCustomerProfile).
type Client struct {
	httpClient     *http.Client
	apiKeyCust1    string
	apiKeyCust2    string
	profileURLTmpl string
	profileLimiter *rateLimiter
}

func New(apiKeyCust1, apiKeyCust2, profileURLTmpl string) *Client {
	return &Client{
		httpClient:     &http.Client{},
		apiKeyCust1:    apiKeyCust1,
		apiKeyCust2:    apiKeyCust2,
		profileURLTmpl: profileURLTmpl,
		// ~20 requests/minute, matching new RateLimiter(20) in local-sync-system.js
		profileLimiter: newRateLimiter(20),
	}
}

type apiConfig struct {
	apiKey  string
	company string
}

// FetchAllCustomers - ported verbatim from fetchAllCustomers: paginates each
// configured API key until an empty page is returned.
func (c *Client) FetchAllCustomers(ctx context.Context) ([]map[string]interface{}, error) {
	var all []map[string]interface{}

	configs := []apiConfig{
		{c.apiKeyCust1, "CUST1"},
		{c.apiKeyCust2, "CUST2"},
	}

	for _, cfg := range configs {
		if cfg.apiKey == "" {
			continue
		}

		page := 1
		for {
			pageData, err := c.fetchSinglePage(ctx, cfg.apiKey, cfg.company, page)
			if err != nil {
				break
			}
			if len(pageData) == 0 {
				break
			}
			all = append(all, pageData...)
			page++
		}
	}

	return all, nil
}

// fetchSinglePage - ported verbatim from fetchSinglePage.
//
// Node's https module sends the contact_index JSON unencoded in the query
// string and Jurnal's edge (Alibaba ESA) accepts it, but Go's net/http gets
// a 400 from the same edge for the unencoded form. Percent-encoding the
// value (url.Values.Encode) is accepted and Jurnal decodes it identically -
// confirmed to return the same contact_list payload.
func (c *Client) fetchSinglePage(ctx context.Context, apiKey, company string, page int) ([]map[string]interface{}, error) {
	q := url.Values{}
	q.Set("contact_index", fmt.Sprintf(`{"curr_page":%d,"selected_tab":1,"sort_asc":true,"show_archive":false}`, page))

	u := &url.URL{
		Scheme:   "https",
		Host:     "api.jurnal.id",
		Path:     "/partner/core/api/v1/contacts",
		RawQuery: q.Encode(),
	}

	data, err := c.fetchWithRetry(ctx, u.String(), apiKey)
	if err != nil {
		return nil, err
	}

	contactList, _ := data["contact_list"].(map[string]interface{})
	if contactList == nil {
		return nil, nil
	}
	contactData, _ := contactList["contact_data"].(map[string]interface{})
	if contactData == nil {
		return nil, nil
	}

	if customers, ok := contactData["customer"].([]interface{}); ok {
		return tagCompany(customers, company), nil
	}

	if personData, ok := contactData["person_data"].([]interface{}); ok {
		return tagCompany(personData, company), nil
	}

	return nil, nil
}

func tagCompany(items []interface{}, company string) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		m["company"] = company
		result = append(result, m)
	}
	return result
}

// fetchWithRetry - ported verbatim from fetchWithRetry: 3 attempts, 15s
// timeout per attempt, exponential backoff (2^attempt seconds).
func (c *Client) fetchWithRetry(ctx context.Context, rawURL, apiKey string) (map[string]interface{}, error) {
	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		data, err := c.doGet(ctx, rawURL, apiKey, 15*time.Second)
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

func (c *Client) doGet(ctx context.Context, rawURL, apiKey string, timeout time.Duration) (map[string]interface{}, error) {
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Ocean-Bearing-Bot/1.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	var data map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, err
	}
	return data, nil
}

// FetchCustomerProfile - ported from the active (2nd, overriding)
// fetchCustomerProfile definition in local-sync-system.js, trying
// API_KEY_CUST1 then API_KEY_CUST2 (per updateCustomersWithProvinceData),
// rate limited to ~20 requests/minute.
func (c *Client) FetchCustomerProfile(ctx context.Context, customerID int64) (map[string]interface{}, error) {
	for _, apiKey := range []string{c.apiKeyCust1, c.apiKeyCust2} {
		if apiKey == "" {
			continue
		}

		if err := c.profileLimiter.wait(ctx); err != nil {
			return nil, err
		}

		rawURL := strings.Replace(c.profileURLTmpl, "{id}", fmt.Sprintf("%d", customerID), 1)
		data, err := c.doGet(ctx, rawURL, apiKey, 15*time.Second)
		if err != nil {
			continue
		}

		if person, ok := data["person"].(map[string]interface{}); ok {
			return person, nil
		}
	}

	return nil, nil
}
