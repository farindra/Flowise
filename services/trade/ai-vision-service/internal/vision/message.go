package vision

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"ai-vision-service/internal/gemini"
	"ai-vision-service/internal/ratelimit"
)

// MessageAnalysis mirrors the object returned by aiService.analyzeMessage().
type MessageAnalysis struct {
	Keywords          []string `json:"keywords"`
	Intent            string   `json:"intent,omitempty"`
	Products          []string `json:"products,omitempty"`
	Quantity          int      `json:"quantity,omitempty"`
	ContainsProfanity bool     `json:"containsProfanity"`
	EnhancedQuery     string   `json:"enhancedQuery"`
	OriginalMessage   string   `json:"originalMessage"`
}

const (
	messageRateLimitMax = 30
	messageTimeout      = 25 * time.Second
)

// MessageAnalyzer wraps the Gemini client for customer message analysis
// (intent classification, product/quantity extraction, profanity detection),
// porting aiService.analyzeMessage().
type MessageAnalyzer struct {
	gemini  *gemini.Client
	cache   *ratelimit.Cache
	limiter *ratelimit.Limiter
}

func NewMessageAnalyzer(g *gemini.Client) *MessageAnalyzer {
	return &MessageAnalyzer{
		gemini:  g,
		cache:   ratelimit.NewCache(),
		limiter: ratelimit.NewLimiter(),
	}
}

// AnalyzeMessage ports aiService.analyzeMessage(): checks a 30-minute result
// cache and a per-phone rate limit (30/min) before calling Gemini with a
// 25s timeout. Falls back to a minimal result (no intent/products/quantity)
// on empty input, rate limiting, or any Gemini error.
func (m *MessageAnalyzer) AnalyzeMessage(ctx context.Context, message, phoneNumber string) (*MessageAnalysis, error) {
	if phoneNumber == "" {
		phoneNumber = "unknown"
	}

	if message == "" {
		return &MessageAnalysis{
			Keywords:        []string{},
			EnhancedQuery:   "",
			OriginalMessage: "",
		}, nil
	}

	cacheKey := "message_analysis_" + messageAnalysisCacheKey(message)

	if cached, ok := m.cache.Get(cacheKey); ok {
		result := cached.(MessageAnalysis)
		return &result, nil
	}

	if m.limiter.IsRateLimited("message_"+phoneNumber, messageRateLimitMax) {
		return &MessageAnalysis{
			Keywords:        []string{},
			EnhancedQuery:   message,
			OriginalMessage: message,
		}, nil
	}

	m.limiter.AddCall("message_" + phoneNumber)

	prompt := fmt.Sprintf(messageAnalysisPromptTemplate, message)

	cctx, cancel := context.WithTimeout(ctx, messageTimeout)
	content, err := m.gemini.GenerateContent(cctx, []gemini.Part{{Text: prompt}})
	cancel()

	if err != nil || content == "" {
		return &MessageAnalysis{
			Keywords:        []string{},
			EnhancedQuery:   message,
			OriginalMessage: message,
		}, nil
	}

	analysis := parseMessageAnalysisResponse(content, message)
	m.cache.Set(cacheKey, *analysis, 30*time.Minute)

	return analysis, nil
}

// messageAnalysisCacheKey ports the cache-key derivation from
// aiService.analyzeMessage(): base64-encode the message, and if longer than
// 64 chars, take the first 32 + last 32 chars to avoid collisions while
// bounding key length.
func messageAnalysisCacheKey(message string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(message))
	if len(encoded) > 64 {
		return encoded[:32] + encoded[len(encoded)-32:]
	}
	return encoded
}

var messageJSONPattern = regexp.MustCompile(`(?s)\{.*\}`)

type rawMessageAnalysis struct {
	Keywords          []string `json:"keywords"`
	Intent            string   `json:"intent"`
	Products          []string `json:"products"`
	Quantity          int      `json:"quantity"`
	ContainsProfanity bool     `json:"containsProfanity"`
	EnhancedQuery     string   `json:"enhancedQuery"`
}

// parseMessageAnalysisResponse ports the JSON parsing + default-filling
// block of aiService.analyzeMessage() (~lines 1192-1223). On parse failure it
// returns the same minimal fallback shape as the rate-limit/error paths.
func parseMessageAnalysisResponse(content, message string) *MessageAnalysis {
	jsonStr := content
	if m := messageJSONPattern.FindString(content); m != "" {
		jsonStr = m
	}

	var raw rawMessageAnalysis
	if err := json.Unmarshal([]byte(jsonStr), &raw); err != nil {
		return &MessageAnalysis{
			Keywords:        []string{},
			EnhancedQuery:   message,
			OriginalMessage: message,
		}
	}

	keywords := raw.Keywords
	if keywords == nil {
		keywords = []string{}
	}

	intent := raw.Intent
	if intent == "" {
		intent = "general_search"
	}

	products := raw.Products
	if products == nil {
		products = []string{}
	}

	quantity := raw.Quantity
	if quantity == 0 {
		quantity = 1
	}

	enhancedQuery := raw.EnhancedQuery
	if enhancedQuery == "" {
		enhancedQuery = message
	}

	return &MessageAnalysis{
		Keywords:          keywords,
		Intent:            intent,
		Products:          products,
		Quantity:          quantity,
		ContainsProfanity: raw.ContainsProfanity,
		EnhancedQuery:     enhancedQuery,
		OriginalMessage:   message,
	}
}
