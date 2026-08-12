package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// isJurnalCustomer reports whether phone (already normalized, 62-prefixed)
// belongs to a known customer in the Jurnal OB and/or SBB accounting system.
// Used to tell apart "unregistered" (truly unknown contact) from "normal"
// (a real, already-known business customer who just hasn't been flagged
// VIP/blacklist) — see handleCustomerTier.
//
// Jurnal's own phone field is unnormalized free text (landlines, multiple
// numbers slash-separated, dashes, etc — confirmed by direct inspection of
// the synced data), so this searches Meilisearch for the number and then
// verifies the match by normalizing every candidate substring in the result,
// rather than trusting Meilisearch's fuzzy/typo-tolerant ranking alone.
func isJurnalCustomer(ctx context.Context, phone string) bool {
	base := strings.TrimRight(envOr("MEILI_URL", "http://127.0.0.1:7700"), "/")
	key := envOr("MEILI_KEY", "")
	if key == "" {
		return false // Jurnal check unavailable; caller falls back to unregistered
	}

	client := &http.Client{Timeout: 3 * time.Second}
	local := phone
	if strings.HasPrefix(phone, "62") {
		local = "0" + phone[2:]
	}

	for _, index := range []string{"customers", "sbb_customers"} {
		for _, q := range []string{phone, local} {
			if meiliHasMatchingPhone(ctx, client, base, key, index, q, phone) {
				return true
			}
		}
	}
	return false
}

var rePhoneSplit = regexp.MustCompile(`[/,;]+`)

func meiliHasMatchingPhone(ctx context.Context, client *http.Client, base, key, index, query, target string) bool {
	body, _ := json.Marshal(map[string]any{
		"q":                    query,
		"limit":                5,
		"attributesToRetrieve": []string{"phone"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		fmt.Sprintf("%s/indexes/%s/search", base, index), strings.NewReader(string(body)))
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false
	}

	var res struct {
		Hits []struct {
			Phone string `json:"phone"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return false
	}

	for _, hit := range res.Hits {
		for _, part := range rePhoneSplit.Split(hit.Phone, -1) {
			if normPhone(part) == target {
				return true
			}
		}
	}
	return false
}
