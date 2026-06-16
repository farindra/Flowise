package vision

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"

	"ai-vision-service/internal/gemini"
	"ai-vision-service/internal/ratelimit"
)

// AnalyzeResult mirrors the object returned by aiService.analyzeImage().
type AnalyzeResult struct {
	Products    []string `json:"products"`
	Codes       []string `json:"codes"`
	Brands      []string `json:"brands,omitempty"`
	Confidence  float64  `json:"confidence"`
	Description string   `json:"description"`
	Error       string   `json:"error,omitempty"`
	Fallback    bool     `json:"fallback,omitempty"`
}

// ErrRateLimited mirrors the error thrown by analyzeImage() when the
// per-phone image rate limit (5/min) is exceeded.
var ErrRateLimited = errors.New("Terlalu banyak permintaan analisis gambar. Silakan coba lagi nanti.")

const (
	noDetectionMessage            = "Maaf, saya tidak dapat mendeteksi kode bearing dari gambar tersebut. Silakan kirim gambar yang lebih jelas atau ketik kode bearing yang Anda cari."
	overloadedFallbackDescription = "Maaf, layanan analisis gambar sedang mengalami gangguan. Silakan coba lagi nanti atau ketik kode produk yang Anda cari secara manual."
	genericErrorDescription       = "Gagal menganalisis gambar"
	busyDescription               = "Maaf, saat ini sistem sedang sibuk memproses antrian gambar. Mohon tunggu beberapa saat sebelum mengirim gambar lagi agar layanan tetap optimal."
	invalidImageDescription       = "Maaf, saya tidak dapat memproses media ini. Silakan kirim gambar produk yang jelas atau ketik nama/kode produk yang Anda cari."

	maxConcurrentImages = 5
	imageRateLimitMax   = 5
	maxRetries          = 3
)

// Analyzer wraps the Gemini client with the rate-limiting, caching and
// concurrency control behavior of aiService.analyzeImage().
type Analyzer struct {
	gemini  *gemini.Client
	cache   *ratelimit.Cache
	limiter *ratelimit.Limiter
	sem     chan struct{}
}

func NewAnalyzer(g *gemini.Client) *Analyzer {
	return &Analyzer{
		gemini:  g,
		cache:   ratelimit.NewCache(),
		limiter: ratelimit.NewLimiter(),
		sem:     make(chan struct{}, maxConcurrentImages),
	}
}

// AnalyzeImage ports aiService.analyzeImage(): validates input, checks the
// per-image cache and per-phone rate limit, calls Gemini Vision with retry
// on 503/overloaded, then parses and validates the resulting bearing codes.
func (a *Analyzer) AnalyzeImage(ctx context.Context, imageData, phoneNumber string) (*AnalyzeResult, error) {
	if phoneNumber == "" {
		phoneNumber = "unknown"
	}

	if len(imageData) < 100 {
		return &AnalyzeResult{
			Products:    []string{},
			Codes:       []string{},
			Confidence:  0,
			Description: invalidImageDescription,
		}, nil
	}

	hash := sha256.Sum256([]byte(imageData))
	cacheKey := "image_analysis_" + hex.EncodeToString(hash[:])

	if cached, ok := a.cache.Get(cacheKey); ok {
		result := cached.(AnalyzeResult)
		return &result, nil
	}

	if a.limiter.IsRateLimited("image_"+phoneNumber, imageRateLimitMax) {
		return nil, ErrRateLimited
	}

	select {
	case a.sem <- struct{}{}:
		defer func() { <-a.sem }()
	default:
		return &AnalyzeResult{
			Products:    []string{},
			Codes:       []string{},
			Confidence:  0,
			Description: busyDescription,
		}, nil
	}

	a.limiter.AddCall("image_" + phoneNumber)

	mimeType := detectMimeType(imageData)

	parts := []gemini.Part{
		{Text: analyzeImagePrompt},
		{InlineData: &gemini.InlineData{MimeType: mimeType, Data: imageData}},
	}

	content, fallback := a.generateWithRetry(ctx, parts)
	if fallback != nil {
		return fallback, nil
	}

	analysis := parseAnalysisResponse(content)

	// Enhanced: check description for codes Gemini mentioned but didn't put in JSON
	if analysis.Description != "" {
		codesFromDesc := extractProductCodesFromText(analysis.Description, false)
		if len(codesFromDesc) > 0 {
			analysis.Products = uniqueStrings(append(append([]string{}, analysis.Products...), codesFromDesc...))
		}
	}

	for i, code := range analysis.Products {
		analysis.Products[i] = cleanBearingCode(code)
	}
	analysis.Products = validateAndNormalizeBearingCodes(analysis.Products)

	if len(analysis.Codes) > 0 {
		analysis.Codes = validateAndNormalizeBearingCodes(analysis.Codes)
		analysis.Products = uniqueStrings(append(append([]string{}, analysis.Products...), analysis.Codes...))
	}

	a.cache.Set(cacheKey, *analysis, time.Hour)

	return analysis, nil
}

// generateWithRetry calls Gemini with a 45s timeout, retrying up to
// maxRetries times with exponential backoff (1s, 2s, 4s) on 503/overloaded
// errors. If retries are exhausted, or a non-overloaded error occurs, it
// returns a fallback AnalyzeResult matching the original error responses.
func (a *Analyzer) generateWithRetry(ctx context.Context, parts []gemini.Part) (string, *AnalyzeResult) {
	delay := time.Second

	for retry := 0; ; retry++ {
		cctx, cancel := context.WithTimeout(ctx, 45*time.Second)
		text, err := a.gemini.GenerateContent(cctx, parts)
		cancel()

		if err == nil {
			return text, nil
		}

		if gemini.IsOverloaded(err) {
			if retry < maxRetries {
				time.Sleep(delay)
				delay *= 2
				continue
			}
			return "", &AnalyzeResult{
				Products:    []string{},
				Codes:       []string{},
				Confidence:  0,
				Description: overloadedFallbackDescription,
				Error:       "Gemini API unavailable after retries",
				Fallback:    true,
			}
		}

		return "", &AnalyzeResult{
			Products:    []string{},
			Codes:       []string{},
			Confidence:  0,
			Description: genericErrorDescription,
		}
	}
}

var jsonFencePattern = regexp.MustCompile("(?s)```json\\s*(.*?)\\s*```")

type rawAnalysis struct {
	Products    []string `json:"products"`
	Codes       []string `json:"codes"`
	Brands      []string `json:"brands"`
	Confidence  float64  `json:"confidence"`
	Description string   `json:"description"`
}

// parseAnalysisResponse ports the JSON-parsing + fallback block in
// analyzeImage() (~lines 351-388).
func parseAnalysisResponse(content string) *AnalyzeResult {
	jsonContent := content
	if strings.Contains(content, "```json") {
		if m := jsonFencePattern.FindStringSubmatch(content); m != nil {
			jsonContent = m[1]
		}
	}

	var raw rawAnalysis
	if err := json.Unmarshal([]byte(jsonContent), &raw); err == nil {
		result := &AnalyzeResult{
			Products:    raw.Products,
			Codes:       raw.Codes,
			Brands:      raw.Brands,
			Confidence:  raw.Confidence,
			Description: raw.Description,
		}
		if result.Products == nil {
			result.Products = []string{}
		}
		if len(result.Products) == 0 {
			result.Description = noDetectionMessage
		}
		return result
	}

	products := extractProductCodesFromText(content, true)
	result := &AnalyzeResult{
		Products:    products,
		Codes:       []string{},
		Confidence:  0.5,
		Description: content,
	}
	if len(result.Products) == 0 {
		result.Description = noDetectionMessage
	}
	return result
}

var dataURLPrefixPattern = regexp.MustCompile(`^data:image/[a-z]+;base64,`)

// detectMimeType ports aiService.detectMimeType() (magic-byte sniffing on
// the base64 image data).
func detectMimeType(base64Data string) string {
	clean := dataURLPrefixPattern.ReplaceAllString(base64Data, "")

	firstBytes := clean
	if len(firstBytes) > 12 {
		firstBytes = firstBytes[:12]
	}

	if strings.HasPrefix(firstBytes, "/9j/") {
		return "image/jpeg"
	}
	if strings.HasPrefix(firstBytes, "iVBORw0KGgo") {
		return "image/png"
	}
	if strings.HasPrefix(firstBytes, "R0lGODlh") || strings.HasPrefix(firstBytes, "R0lGODdh") {
		return "image/gif"
	}
	if strings.HasPrefix(firstBytes, "UklGR") {
		return "image/webp"
	}

	if buf, err := base64.StdEncoding.DecodeString(clean); err == nil && len(buf) >= 12 {
		if string(buf[4:8]) == "ftyp" {
			brand := string(buf[8:12])
			if brand == "heic" || brand == "heix" || brand == "hevc" || brand == "hevx" {
				return "image/heic"
			}
		}
	}

	return "image/jpeg"
}

var (
	deepBallBearingPattern = regexp.MustCompile(`(?i)deep\s+ball\s+bearing\s+`)
	ballBearingPattern     = regexp.MustCompile(`(?i)ball\s+bearing\s+`)
	bearingWordPattern     = regexp.MustCompile(`(?i)bearing\s+`)
)

// cleanBearingCode ports the per-code cleanup in analyzeImage() that strips
// descriptive words like "Deep Ball Bearing " from a detected code.
func cleanBearingCode(code string) string {
	code = deepBallBearingPattern.ReplaceAllString(code, "")
	code = ballBearingPattern.ReplaceAllString(code, "")
	code = bearingWordPattern.ReplaceAllString(code, "")
	return strings.TrimSpace(code)
}

func uniqueStrings(items []string) []string {
	seen := make(map[string]bool, len(items))
	result := make([]string, 0, len(items))
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

var knownBrandsLower = []string{
	"nsk", "skf", "fag", "ntn", "timken", "koyo", "ina", "iko", "snr", "nmb",
	"gmb", "gwm", "gwt", "gwd", "gwz", "gws", "guis", "gum", "toyota", "honda",
	"mitsubishi", "suzuki", "daihatsu", "isuzu", "nissan", "hino",
}

var bearingCodePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^[1-9]\d{3,4}$`),
	regexp.MustCompile(`^[1-9]\d{3,4}[A-Za-z0-9\s/-]*$`),
	regexp.MustCompile(`(?i)^G[WMDTZSK]\d{1,3}[A-Z]?$`),
	regexp.MustCompile(`(?i)^G[WMDTZSK][-\s]?\d{1,3}[A-Z]?$`),
	regexp.MustCompile(`(?i)^GU[A-Z]{1,3}[-\s]?\d+$`),
	regexp.MustCompile(`^\d{3}[A-Za-z0-9]*$`),
	regexp.MustCompile(`^\d+[-\s]\d+$`),
	regexp.MustCompile(`^[A-Za-z0-9]+[-\s]\d+$`),
	regexp.MustCompile(`^\d+[-\s][A-Za-z0-9]+$`),
	regexp.MustCompile(`^\d{3,4}\s\d{3,4}\s\d{2,3}$`),
	regexp.MustCompile(`^[A-Za-z0-9]+/[A-Za-z0-9]+$`),
	regexp.MustCompile(`^[A-Za-z0-9\s]+\.[A-Za-z0-9]+$`),
	regexp.MustCompile(`^[A-Za-z]+\s+\d+`),
	regexp.MustCompile(`^\d{2,4}[A-Za-z]\d{2}$`),
	regexp.MustCompile(`^\d{2,4}[A-Za-z]{2,4}\d{2,4}$`),
	regexp.MustCompile(`^[A-Za-z]{2,4}\d{2,4}$`),
	regexp.MustCompile(`^\d{2,3}[A-Za-z]{2,5}\d{1,3}$`),
	regexp.MustCompile(`^[A-Za-z]{1,3}\d{4,6}$`),
	regexp.MustCompile(`^\d{2,6}[A-Za-z]{1,5}$`),
	regexp.MustCompile(`^MR-\d{6}$`),
	regexp.MustCompile(`^DG-\d{6}$`),
	regexp.MustCompile(`(?i)^(TK[ATDZ])\d{4}$`),
	regexp.MustCompile(`^0\d{1,5}$`),
	regexp.MustCompile(`^[A-Za-z]{1,4}\d{3,5}[A-Za-z0-9]*$`),
	regexp.MustCompile(`^[A-Za-z]{2,4}-\d{4,6}$`),
	regexp.MustCompile(`^\d{4,6}-[A-Za-z0-9]{1,5}$`),
	regexp.MustCompile(`^[A-Za-z0-9\-.]{3,15}$`),
}

var (
	pureDigitsPattern         = regexp.MustCompile(`^\d+$`)
	hasDigitPattern           = regexp.MustCompile(`\d`)
	datePattern               = regexp.MustCompile(`\b(\d{1,2}[-/]\d{1,2}[-/]\d{2,4})\b`)
	longDigitsPattern         = regexp.MustCompile(`\b\d{10,}\b`)
	flexibleKnownBrandPattern = regexp.MustCompile(`^[A-Za-z0-9\-.\s]{3,20}$`)
)

// validateAndNormalizeBearingCodes ports
// aiService.validateAndNormalizeBearingCodes(): dedupes, trims, and filters
// codes against a comprehensive set of bearing-code patterns.
func validateAndNormalizeBearingCodes(codes []string) []string {
	deduped := uniqueStrings(codes)

	result := make([]string, 0, len(deduped))
	for _, raw := range deduped {
		code := strings.TrimSpace(raw)

		if len(code) < 2 {
			continue
		}

		if pureDigitsPattern.MatchString(code) && len(code) < 3 {
			continue
		}

		if !hasDigitPattern.MatchString(code) {
			continue
		}

		if datePattern.MatchString(code) {
			continue
		}

		if longDigitsPattern.MatchString(code) && !strings.HasPrefix(code, "01") {
			continue
		}

		lowerCode := strings.ToLower(code)
		containsKnownBrand := false
		for _, brand := range knownBrandsLower {
			if strings.Contains(lowerCode, brand) {
				containsKnownBrand = true
				break
			}
		}

		matchesBearingPattern := false
		for _, p := range bearingCodePatterns {
			if p.MatchString(code) {
				matchesBearingPattern = true
				break
			}
		}

		if matchesBearingPattern {
			result = append(result, code)
			continue
		}

		if containsKnownBrand {
			if flexibleKnownBrandPattern.MatchString(code) {
				result = append(result, code)
			}
			continue
		}
	}

	return result
}

var (
	ocrDigitReplacements = []struct {
		pattern *regexp.Regexp
		repl    string
	}{
		{regexp.MustCompile(`[oO0]`), "0"},
		{regexp.MustCompile(`[iIl|]`), "1"},
		{regexp.MustCompile(`[sS]`), "5"},
		{regexp.MustCompile(`[zZ]`), "2"},
		{regexp.MustCompile(`[bB]`), "8"},
		{regexp.MustCompile(`[gG]`), "6"},
		{regexp.MustCompile(`[tT]`), "7"},
		{regexp.MustCompile(`[qQ]`), "9"},
	}
	smartQuotesPattern  = regexp.MustCompile(`[\x{2018}\x{2019}\x{201C}\x{201D}]`)
	nonCodeCharsPattern = regexp.MustCompile(`[^\w\s\-/.]`)

	extractBearingPattern           = regexp.MustCompile(`\b\d{3,5}(?:[-\s]?[A-Za-z0-9]+)?\b`)
	extractAlphaNumericPattern      = regexp.MustCompile(`\b[A-Za-z]{1,4}\d{3,5}(?:[-\s]?[A-Za-z0-9]+)?\b`)
	extractSpecialCodePattern       = regexp.MustCompile(`\b(TK[ATDZ])\s*([0-9]{3,5})\b`)
	extractBrandedBearingPattern    = regexp.MustCompile(`(?i)\b([A-Za-z]{1,5})(\d{3,5})(?:[A-Za-z0-9\-/]*)\b`)
	extractSpecialFormatPattern     = regexp.MustCompile(`\b([A-Za-z]{1,3})(\d{2,3})[-\s](\d{1,3})\b`)
	extractHandwrittenFormatPattern = regexp.MustCompile(`\b([A-Za-z]{1,3})[\s-](\d{2,4})[\s-](\d{1,3})\b`)

	bearingContextPattern = regexp.MustCompile(`(?i)\b(bearing|laher|bushing|seal|nsk|skf|fag|ntn|timken|koyo)\b`)
	bearingNumberPattern  = regexp.MustCompile(`\b[267]\d{3}\b`)
)

var extractKnownBrands = []string{"NSK", "SKF", "NTN", "FAG", "INA", "IKO", "NMB", "SNR", "TIMKEN", "KOYO"}
var extractSpecialBrands = []string{"NTN", "KOYO", "FAG"}

// extractProductCodesFromText ports aiService.extractProductCodesFromText():
// a regex-based fallback extractor used when Gemini's JSON response can't be
// parsed, or to scan a free-text description for additional codes.
func extractProductCodesFromText(text string, isRawOCR bool) []string {
	if text == "" {
		return []string{}
	}

	lines := strings.Split(text, "\n")
	var productCodes []string

	for _, line := range lines {
		if datePattern.MatchString(line) || longDigitsPattern.MatchString(line) {
			continue
		}

		normalizedLine := line
		if isRawOCR {
			for _, r := range ocrDigitReplacements {
				normalizedLine = r.pattern.ReplaceAllString(normalizedLine, r.repl)
			}
		}
		normalizedLine = smartQuotesPattern.ReplaceAllString(normalizedLine, "")
		normalizedLine = nonCodeCharsPattern.ReplaceAllString(normalizedLine, " ")

		// 1. Standard bearing codes
		matches := extractBearingPattern.FindAllString(normalizedLine, -1)
		if len(matches) > 0 {
			hasBearingContext := bearingContextPattern.MatchString(normalizedLine)
			for _, match := range matches {
				if hasBearingContext || bearingNumberPattern.MatchString(match) {
					productCodes = append(productCodes, strings.TrimSpace(match))
				}
			}
		}

		// 1.5 Alphanumeric codes (e.g. NU224)
		for _, match := range extractAlphaNumericPattern.FindAllString(normalizedLine, -1) {
			productCodes = append(productCodes, strings.TrimSpace(match))
		}

		// 2. Special codes like TKT 3101
		for _, m := range extractSpecialCodePattern.FindAllString(normalizedLine, -1) {
			productCodes = append(productCodes, strings.TrimSpace(m))
		}

		// 3. Branded bearing codes (e.g. NSK6203, SKF6305)
		for _, m := range extractBrandedBearingPattern.FindAllStringSubmatch(normalizedLine, -1) {
			full, brand, code := m[0], strings.ToUpper(m[1]), m[2]
			if contains(extractKnownBrands, brand) {
				suffixStart := len(brand) + len(code)
				suffix := ""
				if suffixStart <= len(full) {
					suffix = full[suffixStart:]
				}
				productCodes = append(productCodes, code+suffix)
				productCodes = append(productCodes, full)
			}
		}

		// 3.1 Special detection for NTN, KOYO, FAG without standard format
		for _, specialBrand := range extractSpecialBrands {
			if strings.Contains(normalizedLine, specialBrand) {
				for _, codeMatch := range extractBearingPattern.FindAllString(normalizedLine, -1) {
					productCodes = append(productCodes, codeMatch)
					productCodes = append(productCodes, codeMatch+"."+specialBrand)
				}
			}
		}

		// 4. Special format codes (e.g. LM11949-10)
		for _, m := range extractSpecialFormatPattern.FindAllStringSubmatch(normalizedLine, -1) {
			productCodes = append(productCodes, m[1]+m[2]+"-"+m[3])
		}

		// 5. Handwritten format codes with flexible spacing
		for _, m := range extractHandwrittenFormatPattern.FindAllStringSubmatch(normalizedLine, -1) {
			productCodes = append(productCodes, m[1]+m[2]+"-"+m[3])
		}
	}

	result := uniqueStrings(productCodes)
	return validateAndNormalizeBearingCodes(result)
}

func contains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
