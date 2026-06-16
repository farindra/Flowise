package handler

import (
	"encoding/json"
	"log"
	"net/http"

	"ai-vision-service/internal/vision"
)

type Handler struct {
	analyzer        *vision.Analyzer
	parser          *vision.Parser
	messageAnalyzer *vision.MessageAnalyzer
	chatter         *vision.Chatter
	name            string
}

func New(analyzer *vision.Analyzer, parser *vision.Parser, messageAnalyzer *vision.MessageAnalyzer, chatter *vision.Chatter, serviceName string) *Handler {
	return &Handler{analyzer: analyzer, parser: parser, messageAnalyzer: messageAnalyzer, chatter: chatter, name: serviceName}
}

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"service": h.name,
		"status":  "ok",
	})
}

type analyzeImageRequest struct {
	Image       string `json:"image"`
	MimeType    string `json:"mimeType,omitempty"`
	PhoneNumber string `json:"phoneNumber"`
}

// AnalyzeImage handles POST /analyze-image, the port of
// aiService.analyzeImage().
func (h *Handler) AnalyzeImage(w http.ResponseWriter, r *http.Request) {
	var req analyzeImageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	result, err := h.analyzer.AnalyzeImage(r.Context(), req.Image, req.PhoneNumber)
	if err != nil {
		if err == vision.ErrRateLimited {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": err.Error()})
			return
		}
		log.Printf("analyze-image error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

type parseMultiProductRequest struct {
	Text        string `json:"text"`
	PhoneNumber string `json:"phoneNumber"`
}

// ParseMultiProduct handles POST /parse-multi-product, the port of
// aiService.parseMultiProductWithAI().
func (h *Handler) ParseMultiProduct(w http.ResponseWriter, r *http.Request) {
	var req parseMultiProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	result := h.parser.ParseMultiProductWithAI(r.Context(), req.Text, req.PhoneNumber)
	writeJSON(w, http.StatusOK, result)
}

type analyzeMessageRequest struct {
	Text        string `json:"text"`
	PhoneNumber string `json:"phoneNumber"`
}

// AnalyzeMessage handles POST /analyze-message, the port of
// aiService.analyzeMessage().
func (h *Handler) AnalyzeMessage(w http.ResponseWriter, r *http.Request) {
	var req analyzeMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	result, err := h.messageAnalyzer.AnalyzeMessage(r.Context(), req.Text, req.PhoneNumber)
	if err != nil {
		log.Printf("analyze-message error: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GenerateNatural handles POST /generate-natural, porting
// aiService.generateNaturalGreeting() and aiService.generateNaturalResponse().
func (h *Handler) GenerateNatural(w http.ResponseWriter, r *http.Request) {
	var req vision.NaturalChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	response := h.chatter.GenerateNatural(r.Context(), &req)
	writeJSON(w, http.StatusOK, vision.NaturalChatResponse{Response: response})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(body)
}
