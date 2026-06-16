package vision

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"ai-vision-service/internal/gemini"
	"ai-vision-service/internal/ratelimit"
)

// MultiProductResult mirrors the object returned by
// aiService.parseMultiProductWithAI() / productService.parseMultiProductWithAI().
type MultiProductResult struct {
	IsMultiProduct bool     `json:"isMultiProduct"`
	Products       []string `json:"products"`
	Confidence     float64  `json:"confidence"`
	Method         string   `json:"method"`
}

const (
	multiProductRateLimitMax = 20
	multiProductTimeout      = 30 * time.Second
)

// Parser wraps the Gemini client for AI-based multi-product parsing, with a
// manual regex-based fallback ported from productService.js.
type Parser struct {
	gemini  *gemini.Client
	cache   *ratelimit.Cache
	limiter *ratelimit.Limiter
}

func NewParser(g *gemini.Client) *Parser {
	return &Parser{
		gemini:  g,
		cache:   ratelimit.NewCache(),
		limiter: ratelimit.NewLimiter(),
	}
}

// ParseMultiProductWithAI ports aiService.parseMultiProductWithAI(): tries
// Gemini first (cached, rate-limited), falling back to manual parsing
// (parseMultiProductInput/isMultiProductSearch) on rate limit, error, or an
// invalid/empty AI response.
func (p *Parser) ParseMultiProductWithAI(ctx context.Context, text, phoneNumber string) *MultiProductResult {
	if phoneNumber == "" {
		phoneNumber = "unknown"
	}

	if text == "" {
		return &MultiProductResult{IsMultiProduct: false, Products: []string{""}, Confidence: 0, Method: "fallback"}
	}

	cacheKey := "multi_product_" + multiProductCacheKey(text)

	if cached, ok := p.cache.Get(cacheKey); ok {
		result := cached.(MultiProductResult)
		return &result
	}

	if p.limiter.IsRateLimited("multiproduct_"+phoneNumber, multiProductRateLimitMax) {
		return manualFallback(text, 0.5)
	}

	p.limiter.AddCall("multiproduct_" + phoneNumber)

	prompt := fmt.Sprintf(multiProductPromptTemplate, text)

	cctx, cancel := context.WithTimeout(ctx, multiProductTimeout)
	content, err := p.gemini.GenerateContent(cctx, []gemini.Part{{Text: prompt}})
	cancel()

	if err == nil {
		if result, ok := parseMultiProductResponse(content); ok {
			p.cache.Set(cacheKey, *result, time.Hour)
			return result
		}
	}

	return manualFallback(text, 0.7)
}

func multiProductCacheKey(text string) string {
	encoded := base64.StdEncoding.EncodeToString([]byte(text))
	if len(encoded) > 32 {
		return encoded[:32]
	}
	return encoded
}

var jsonFenceStripPattern = regexp.MustCompile("```json\n?|\n?```")

type rawMultiProduct struct {
	IsMultiProduct *bool    `json:"isMultiProduct"`
	Products       []string `json:"products"`
	Confidence     float64  `json:"confidence"`
}

// parseMultiProductResponse ports the JSON parsing + validation block of
// aiService.parseMultiProductWithAI() (~lines 1342-1373). It returns ok=false
// if the response is missing, malformed, or has an empty products array,
// signalling the caller to fall back to manual parsing.
func parseMultiProductResponse(content string) (*MultiProductResult, bool) {
	if content == "" {
		return nil, false
	}

	clean := strings.TrimSpace(jsonFenceStripPattern.ReplaceAllString(content, ""))

	var raw rawMultiProduct
	if err := json.Unmarshal([]byte(clean), &raw); err != nil {
		return nil, false
	}

	if raw.IsMultiProduct == nil || raw.Products == nil {
		return nil, false
	}

	products := make([]string, 0, len(raw.Products))
	for _, prod := range raw.Products {
		if strings.TrimSpace(prod) != "" {
			products = append(products, prod)
		}
	}

	if len(products) == 0 {
		return nil, false
	}

	confidence := raw.Confidence
	if confidence == 0 {
		confidence = 0.8
	}
	if confidence < 0 {
		confidence = 0
	}
	if confidence > 1 {
		confidence = 1
	}

	return &MultiProductResult{
		IsMultiProduct: *raw.IsMultiProduct,
		Products:       products,
		Confidence:     confidence,
		Method:         "ai_parsing",
	}, true
}

func manualFallback(text string, confidence float64) *MultiProductResult {
	return &MultiProductResult{
		IsMultiProduct: isMultiProductSearch(text),
		Products:       parseMultiProductInput(text),
		Confidence:     confidence,
		Method:         "manual_fallback",
	}
}

// ---------------------------------------------------------------------
// Manual parsing fallback, ported from productService.js
// parseMultiProductInput() / isMultiProductSearch().
// ---------------------------------------------------------------------

var (
	multiProductIndicators = []*regexp.Regexp{
		regexp.MustCompile(`,\s*\w+`),
		regexp.MustCompile(`\.\s*\w+.*\d`),
		regexp.MustCompile(`[•\-*]\s*\w+.*[•\-*]\s*\w+`),
		regexp.MustCompile(`(?m)^\d+[.)]\s*\w+.*^\d+[.)]\s*\w+`),
		regexp.MustCompile(`\n.*\w+.*\n.*\w+`),
		regexp.MustCompile(`(?i)\b(dan|sama|dengan)\b.*\d`),
	}

	productBrandPrefixPattern = regexp.MustCompile(`(?i)^(SKF|FAG|KOYO|TIMKEN|NSK|NTN|INA|GMB|CT|WTP)`)
	productBrandWordPattern   = regexp.MustCompile(`(?i)\b(SKF|FAG|KOYO|TIMKEN|NSK|NTN|INA|GMB|CT|WTP)\b`)
)

// isMultiProductSearch ports productService.isMultiProductSearch().
func isMultiProductSearch(text string) bool {
	if text == "" {
		return false
	}

	for _, pattern := range multiProductIndicators {
		if pattern.MatchString(text) {
			return true
		}
	}

	if strings.Contains(text, ".") {
		var dotParts []string
		for _, part := range strings.Split(text, ".") {
			if strings.TrimSpace(part) != "" {
				dotParts = append(dotParts, part)
			}
		}
		if len(dotParts) >= 2 {
			productLikeCount := 0
			for _, part := range dotParts {
				if hasDigitPattern.MatchString(part) || productBrandPrefixPattern.MatchString(part) {
					productLikeCount++
				}
			}
			if productLikeCount >= 2 {
				return true
			}
		}
	}

	return false
}

var (
	removeWordsPattern = regexp.MustCompile(`(?i)\b(selamat|siang|pagi|sore|malam|halo|hai|tolong|bisa|minta|carikan|cari|harga|berapa|brp|ada|stock|stok|ready|tersedia|info|informasi|dong|apa|bearing|yang|dengan|kode|nama|ini|itu)\b`)
	greetingPattern    = regexp.MustCompile(`(?i)^[^\d]*\b(selamat|siang|pagi|sore|malam|halo|hai|tolong|bisa|minta|carikan|cari|bearing)\b[^\d]*`)

	numberedListPattern = regexp.MustCompile(`(?m)^\d+[.)]\s+`)
	bulletPointPattern  = regexp.MustCompile(`(?m)^[•\-*]\s+`)

	multiSpaceTabPattern = regexp.MustCompile(`[ \t]+`)
	newlineSpacePattern  = regexp.MustCompile(`\n\s+`)

	bulletOrNumberCheckPattern = regexp.MustCompile(`(?m)[•\-*]\s+|^\d+[).]\s`)

	bulletPrefixPattern  = regexp.MustCompile(`^[-•*]\s*`)
	numberPrefixPattern  = regexp.MustCompile(`^\d+[.)]\s*`)
	onlyPunctPattern     = regexp.MustCompile(`^[:\-*•.)]+$`)
	skipWordPattern      = regexp.MustCompile(`(?i)^(atau|kode|nama|ini|itu|bearing|dengan|yang|dong|tolong|carikan)$`)
	connectorOnlyPattern = regexp.MustCompile(`(?i)^(atau\s*[:;]?|dan\s*[:;]?)$`)

	trailingNumPattern        = regexp.MustCompile(`\s+\d{1,2}$`)
	trailingNumCapturePattern = regexp.MustCompile(`\s+(\d{1,2})$`)

	danSamaWordPattern = regexp.MustCompile(`(?i)\b(dan|sama)\b`)
	danWordPattern     = regexp.MustCompile(`(?i)\bdan\b`)
	jugaWordPattern    = regexp.MustCompile(`(?i)\b(juga|jg)\b`)
	threeDigitsPattern = regexp.MustCompile(`\d{3,}`)

	productCodePattern         = regexp.MustCompile(`\b\d{3,}(?:\.\d+)?[A-Za-z\-]*\b`)
	threeDigitWordPattern      = regexp.MustCompile(`\b\d{3,}\b`)
	multiSpaceOrNewlinePattern = regexp.MustCompile(`\n|\s{2,}`)
	whitespacePattern          = regexp.MustCompile(`\s+`)

	trailingQuestionExclaimPattern = regexp.MustCompile(`[?!]{2,}$`)
	trailingSemicolonCommaPattern  = regexp.MustCompile(`[;,]+$`)
	digitDotEndPattern             = regexp.MustCompile(`\d\.$`)
	trailingDotsPattern            = regexp.MustCompile(`\.+$`)
)

// parseMultiProductInput ports productService.parseMultiProductInput()
// (~lines 67-313): splits a free-text product query into individual product
// queries using a series of heuristics (bullet/numbered lists, commas, dots,
// "dan"/"sama" connectors, space-separated codes).
func parseMultiProductInput(text string) []string {
	if text == "" {
		return []string{}
	}

	if !isMultiProductSearch(text) {
		return []string{strings.TrimSpace(text)}
	}

	cleanText := strings.TrimSpace(text)

	hasNumberedList := numberedListPattern.MatchString(cleanText)
	hasBulletPoints := bulletPointPattern.MatchString(cleanText)

	if !hasNumberedList && !hasBulletPoints {
		cleanText = removeWordsPattern.ReplaceAllString(cleanText, " ")
	} else {
		cleanText = greetingPattern.ReplaceAllString(cleanText, "")
	}

	cleanText = multiSpaceTabPattern.ReplaceAllString(cleanText, " ")
	cleanText = newlineSpacePattern.ReplaceAllString(cleanText, "\n")
	cleanText = strings.TrimSpace(cleanText)

	var products []string

	switch {
	case hasNumberedList || hasBulletPoints || bulletOrNumberCheckPattern.MatchString(cleanText):
		products = parseBulletOrNumbered(cleanText)
	case strings.Contains(cleanText, ","):
		products = parseCommaSeparated(cleanText)
	case strings.Contains(cleanText, ".") && !strings.HasSuffix(cleanText, "."):
		products = parseDotSeparated(cleanText)
	case danSamaWordPattern.MatchString(cleanText):
		products = parseConnectingWords(cleanText)
	default:
		products = parseSpaceSeparated(cleanText)
	}

	return cleanProducts(products)
}

func parseBulletOrNumbered(cleanText string) []string {
	var products []string

	for _, line := range strings.Split(cleanText, "\n") {
		cleaned := strings.TrimSpace(line)

		cleaned = bulletPrefixPattern.ReplaceAllString(cleaned, "")
		cleaned = numberPrefixPattern.ReplaceAllString(cleaned, "")

		if cleaned == "" || len(cleaned) <= 1 || onlyPunctPattern.MatchString(cleaned) {
			continue
		}

		if skipWordPattern.MatchString(cleaned) {
			continue
		}

		if connectorOnlyPattern.MatchString(cleaned) {
			continue
		}

		if trailingNumPattern.MatchString(cleaned) {
			if m := trailingNumCapturePattern.FindStringSubmatch(cleaned); m != nil {
				trailingNum := m[1]
				withoutTrailing := strings.TrimSpace(trailingNumPattern.ReplaceAllString(cleaned, ""))

				num := 0
				for _, c := range trailingNum {
					num = num*10 + int(c-'0')
				}

				if withoutTrailing != "" && (len(withoutTrailing) > 2 || hasDigitPattern.MatchString(withoutTrailing)) && num <= 20 {
					cleaned = withoutTrailing
				}
			}
		}

		if cleaned != "" && (strings.Contains(cleaned, ",") || danSamaWordPattern.MatchString(cleaned)) {
			var parts []string
			if strings.Contains(cleaned, ",") {
				for _, part := range strings.Split(cleaned, ",") {
					part = strings.TrimSpace(part)
					if part != "" {
						parts = append(parts, part)
					}
				}
			} else {
				parts = []string{cleaned}
			}

			for _, part := range parts {
				if danSamaWordPattern.MatchString(part) {
					for _, connectorPart := range danSamaWordPattern.Split(part, -1) {
						trimmedPart := strings.TrimSpace(connectorPart)
						trimmedPart = strings.TrimSpace(jugaWordPattern.ReplaceAllString(trimmedPart, ""))

						if trimmedPart != "" && len(trimmedPart) > 1 {
							if threeDigitsPattern.MatchString(trimmedPart) || productBrandWordPattern.MatchString(trimmedPart) {
								products = append(products, trimmedPart)
							}
						}
					}
				} else if part != "" && len(part) > 1 {
					products = append(products, part)
				}
			}
		} else if cleaned != "" && len(cleaned) > 1 {
			products = append(products, cleaned)
		}
	}

	return products
}

func parseCommaSeparated(cleanText string) []string {
	var products []string
	for _, part := range strings.Split(cleanText, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			products = append(products, part)
		}
	}

	if len(products) > 0 {
		lastProduct := products[len(products)-1]
		if danWordPattern.MatchString(lastProduct) {
			danParts := danWordPattern.Split(lastProduct, -1)
			products[len(products)-1] = strings.TrimSpace(danParts[0])
			if len(danParts) > 1 && strings.TrimSpace(danParts[1]) != "" {
				products = append(products, strings.TrimSpace(danParts[1]))
			}
		}
	}

	return products
}

func parseDotSeparated(cleanText string) []string {
	var dotParts []string
	for _, part := range strings.Split(cleanText, ".") {
		if strings.TrimSpace(part) != "" {
			dotParts = append(dotParts, part)
		}
	}

	if len(dotParts) >= 2 {
		productLikeCount := 0
		for _, part := range dotParts {
			if hasDigitPattern.MatchString(part) || productBrandPrefixPattern.MatchString(part) {
				productLikeCount++
			}
		}
		if productLikeCount >= 2 {
			var products []string
			for _, part := range dotParts {
				part = strings.TrimSpace(part)
				if part != "" {
					products = append(products, part)
				}
			}
			return products
		}
	}

	return []string{cleanText}
}

func parseConnectingWords(cleanText string) []string {
	productPatterns := productCodePattern.FindAllString(cleanText, -1)
	if len(productPatterns) > 1 {
		return productPatterns
	}

	var products []string
	for _, part := range danSamaWordPattern.Split(cleanText, -1) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if hasDigitPattern.MatchString(part) {
			products = append(products, part)
		}
	}
	return products
}

func parseSpaceSeparated(cleanText string) []string {
	numericPatterns := productCodePattern.FindAllString(cleanText, -1)
	if len(numericPatterns) > 1 {
		return numericPatterns
	}

	if strings.Contains(cleanText, "\n") || multiSpaceOrNewlinePattern.MatchString(cleanText) {
		var products []string
		for _, part := range multiSpaceOrNewlinePattern.Split(cleanText, -1) {
			part = strings.TrimSpace(part)
			if part != "" && len(part) > 1 {
				products = append(products, part)
			}
		}
		return products
	}

	var words []string
	for _, w := range whitespacePattern.Split(cleanText, -1) {
		if w != "" {
			words = append(words, w)
		}
	}

	if len(words) > 1 {
		var productLikeWords []string
		for _, word := range words {
			if hasDigitPattern.MatchString(word) || productBrandPrefixPattern.MatchString(word) {
				productLikeWords = append(productLikeWords, word)
			}
		}

		if len(productLikeWords) > 1 {
			return productLikeWords
		} else if len(productLikeWords) == 1 {
			allNumbers := threeDigitWordPattern.FindAllString(cleanText, -1)
			if len(allNumbers) > 1 {
				return allNumbers
			}
			return []string{cleanText}
		}
		return []string{cleanText}
	}

	return []string{cleanText}
}

func cleanProducts(products []string) []string {
	result := make([]string, 0, len(products))
	for _, product := range products {
		product = strings.TrimSpace(product)
		if product == "" {
			continue
		}

		cleaned := trailingQuestionExclaimPattern.ReplaceAllString(product, "")
		cleaned = whitespacePattern.ReplaceAllString(cleaned, " ")
		cleaned = strings.TrimSpace(cleaned)

		cleaned = trailingSemicolonCommaPattern.ReplaceAllString(cleaned, "")
		cleaned = strings.TrimSpace(cleaned)

		if strings.HasSuffix(cleaned, ".") && !digitDotEndPattern.MatchString(cleaned) {
			cleaned = trailingDotsPattern.ReplaceAllString(cleaned, "")
		}

		if len(cleaned) > 1 {
			result = append(result, cleaned)
		}
	}
	return result
}
