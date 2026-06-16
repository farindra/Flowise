package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"ai-vision-service/internal/gemini"
	"ai-vision-service/internal/handler"
	"ai-vision-service/internal/vision"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8103"
	}
	name := os.Getenv("SERVICE_NAME")
	if name == "" {
		name = "ai-vision-service"
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	model := os.Getenv("GEMINI_MODEL")
	baseURL := os.Getenv("GEMINI_BASE_URL")

	maxTokens, err := strconv.Atoi(os.Getenv("GEMINI_MAX_TOKENS"))
	if err != nil {
		maxTokens = 5000
	}

	temperature, err := strconv.ParseFloat(os.Getenv("GEMINI_TEMPERATURE"), 64)
	if err != nil {
		temperature = 0.7
	}

	g := gemini.New(apiKey, model, baseURL, maxTokens, temperature)
	analyzer := vision.NewAnalyzer(g)
	parser := vision.NewParser(g)
	messageAnalyzer := vision.NewMessageAnalyzer(g)
	chatter := vision.NewChatter(g)

	h := handler.New(analyzer, parser, messageAnalyzer, chatter, name)

	http.HandleFunc("/health", h.Health)
	http.HandleFunc("/analyze-image", h.AnalyzeImage)
	http.HandleFunc("/parse-multi-product", h.ParseMultiProduct)
	http.HandleFunc("/analyze-message", h.AnalyzeMessage)
	http.HandleFunc("/generate-natural", h.GenerateNatural)

	log.Printf("%s listening on :%s", name, port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
