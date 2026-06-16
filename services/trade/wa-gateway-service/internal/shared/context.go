package shared

import (
	"regexp"
	"strings"
	"time"
)

// ConversationContext is stored under the "conversationContext" key in the
// state store, mirroring the updatedContext object in
// messageHandler.updateConversationContext (line 5082).
type ConversationContext struct {
	LastIntent       string   `json:"lastIntent"`
	LastInteraction  int64    `json:"lastInteraction"`
	IntentHistory    []string `json:"intentHistory"`
	FrustrationLevel int      `json:"frustrationLevel"`
}

// InputContext is the result of AnalyzeInputContext, mirroring the return
// value of messageHandler.analyzeInputContext (line 5068).
type InputContext struct {
	Intent            string
	IsFrustrated      bool
	HasProductCode    bool
	HasProductKeyword bool
	IsGreeting        bool
	IsQuestion        bool
	Confidence        float64
}

var (
	productCodePatterns = []*regexp.Regexp{
		regexp.MustCompile(`^\d{4}\s*[a-zA-Z0-9]*$`),
		regexp.MustCompile(`\b[A-Za-z0-9]{3,}\.[A-Za-z]{2,}\b`),
		regexp.MustCompile(`^[A-Z]{2,}\s*\d{2,}$`),
		regexp.MustCompile(`^\d{2,}\s*[A-Z]{2,}$`),
	}

	productContextKeywords = []string{
		"bearing", "laher", "produk", "barang", "toyota", "honda", "motor",
		"ntn", "skf", "fag", "timken", "deep groove", "angular contact",
		"roller", "ball", "thrust", "needle", "spherical", "cylindrical",
		"tapered", "pillow block", "insert", "hub", "wheel", "automotive",
		"suzuki", "daihatsu", "mitsubishi", "isuzu", "hino", "koyo",
		"bushing", "seal", "oil",
	}

	greetingContextWords = []string{
		"halo", "hai", "hello", "hi", "selamat", "pagi", "siang", "sore", "malam",
		"assalamualaikum",
	}

	questionWords = []string{"apa", "siapa", "dimana", "kapan", "bagaimana", "berapa", "kenapa", "mengapa"}

	frustratedWords = []string{
		"muak", "kesal", "bodo", "bodoh", "bgt", "banget", "males", "malas",
		"capek", "cape", "bosen", "bosan", "gak usah", "ga usah", "skip",
		"lewati", "lanjut", "langsung", "ribet", "rumit", "lama", "lambat",
		"cepetan", "cepat", "buruk", "jelek", "anjing", "anjir", "anjay",
	}
)

// AnalyzeInputContext ports messageHandler.analyzeInputContext (line 5022).
// isRegistered and currentState are pre-fetched by the caller from the state store.
func AnalyzeInputContext(messageBody string, isRegistered bool, currentState string) InputContext {
	lower := strings.ToLower(messageBody)
	trimmed := strings.TrimSpace(messageBody)

	isFrustrated := false
	for _, w := range frustratedWords {
		if strings.Contains(lower, w) {
			isFrustrated = true
			break
		}
	}

	hasProductCode := false
	for _, re := range productCodePatterns {
		if re.MatchString(trimmed) {
			hasProductCode = true
			break
		}
	}

	hasProductKeyword := false
	for _, kw := range productContextKeywords {
		if strings.Contains(lower, kw) {
			hasProductKeyword = true
			break
		}
	}

	isGreet := false
	for _, w := range greetingContextWords {
		if strings.Contains(lower, w) {
			isGreet = true
			break
		}
	}

	isQuestion := strings.Contains(messageBody, "?")
	if !isQuestion {
		for _, w := range questionWords {
			if strings.Contains(lower, w) {
				isQuestion = true
				break
			}
		}
	}

	intent := "general"
	switch {
	case hasProductCode || hasProductKeyword:
		intent = "product_search"
	case isGreet:
		intent = "greeting"
	case isQuestion:
		intent = "question"
	case !isRegistered && currentState == "idle":
		intent = "registration_required"
	}

	return InputContext{
		Intent:            intent,
		IsFrustrated:      isFrustrated,
		HasProductCode:    hasProductCode,
		HasProductKeyword: hasProductKeyword,
		IsGreeting:        isGreet,
		IsQuestion:        isQuestion,
		Confidence:        CalculateIntentConfidence(messageBody, intent),
	}
}

// UpdateConversationContext ports messageHandler.updateConversationContext
// (line 5079). Pure: caller handles Get/Set on the state store.
// Returns the updated ConversationContext and whether skipRegistration should
// be set (frustrationLevel >= 2).
func UpdateConversationContext(current ConversationContext, input InputContext) (ConversationContext, bool) {
	history := current.IntentHistory
	if len(history) >= 5 {
		history = history[len(history)-4:]
	}
	history = append(history, input.Intent)

	frustration := current.FrustrationLevel
	if input.IsFrustrated {
		frustration++
	} else if frustration > 0 {
		frustration--
	}

	updated := ConversationContext{
		LastIntent:       input.Intent,
		LastInteraction:  time.Now().UnixMilli(),
		IntentHistory:    history,
		FrustrationLevel: frustration,
	}
	return updated, frustration >= 2
}

var (
	productCodeConfidenceRe = regexp.MustCompile(`^\d{2,}$`)
	productCodeAlphaRe      = regexp.MustCompile(`^\d{2,}[\s.-]?[A-Za-z0-9]*$`)
	greetingExactRe         = regexp.MustCompile(`(?i)^(halo|hai|hello|hi)$`)
	bearingLaherRe          = regexp.MustCompile(`(?i)bearing|laher`)
)

// CalculateIntentConfidence ports messageHandler.calculateIntentConfidence
// (line 5101).
func CalculateIntentConfidence(messageBody, intent string) float64 {
	trimmed := strings.TrimSpace(messageBody)
	switch intent {
	case "product_search":
		if productCodeConfidenceRe.MatchString(trimmed) || productCodeAlphaRe.MatchString(trimmed) {
			return 0.9
		}
		if bearingLaherRe.MatchString(messageBody) {
			return 0.8
		}
		return 0.6
	case "greeting":
		if greetingExactRe.MatchString(trimmed) {
			return 0.9
		}
		return 0.7
	default:
		return 0.5
	}
}
