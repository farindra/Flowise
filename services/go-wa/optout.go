package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// stopKeywords are the exact messages that unsubscribe a number from
// broadcasts. Matching is exact (case-insensitive, trimmed) rather than
// substring so a normal sentence like "stop dulu ya, saya pikir-pikir" is not
// mistaken for an opt-out.
var stopKeywords = map[string]bool{
	"stop":        true,
	"berhenti":    true,
	"unsub":       true,
	"unsubscribe": true,
	"stop promo":  true,
}

const optOutReply = "Baik, nomor Anda sudah kami keluarkan dari daftar broadcast. Terima kasih 🙏"

func isStopKeyword(text string) bool {
	return stopKeywords[strings.ToLower(strings.TrimSpace(text))]
}

// postOptOut records the opt-out in go-crm. Best-effort: a failure here is
// logged, never surfaced to the customer, because the confirmation reply has
// already been promised by the caller.
func postOptOut(phone, note string) {
	base := envOr("CRM_SERVICE_URL", "")
	key := envOr("CRM_INTERNAL_KEY", "")
	if base == "" {
		fmt.Printf("[optout] CRM_SERVICE_URL not set, cannot record opt-out for %s\n", phone)
		return
	}

	body, _ := json.Marshal(map[string]string{
		"phone":  phone,
		"reason": "stop_keyword",
		"note":   note,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(base, "/")+"/api/broadcasts/optouts", bytes.NewReader(body))
	if err != nil {
		fmt.Printf("[optout] build request for %s: %v\n", phone, err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("X-Internal-Key", key)
	}

	resp, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		fmt.Printf("[optout] record %s: %v\n", phone, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		fmt.Printf("[optout] record %s: HTTP %d\n", phone, resp.StatusCode)
		return
	}
	fmt.Printf("[optout] %s opted out via %s\n", phone, note)
}
