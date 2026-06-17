package main

import (
	"log"
	"net/http"
	"os"

	"product-search-service/internal/handler"
	"product-search-service/internal/meili"
	"product-search-service/internal/search"
)

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	port := getenv("PORT", "8101")
	name := getenv("SERVICE_NAME", "product-search-service")

	meiliURL := getenv("MEILI_URL", "http://meilisearch:7700")
	meiliAPIKey := os.Getenv("MEILI_API_KEY")

	meiliClient := meili.New(meiliURL, meiliAPIKey)
	searchService := search.New(meiliClient)
	h := handler.New(searchService, name)

	mux := http.NewServeMux()
	mux.HandleFunc("/health", h.Health)
	mux.HandleFunc("/search", h.Search)
	mux.HandleFunc("/export/csv", h.ExportCSV)

	log.Printf("%s listening on :%s", name, port)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
