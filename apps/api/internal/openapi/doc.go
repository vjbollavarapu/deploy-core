package openapi

import (
	_ "embed"
	"encoding/json"
	"net/http"
)

//go:embed openapi.json
var SpecJSON []byte

// Document is the parsed OpenAPI 3.1 document (subset used for validation).
type Document struct {
	OpenAPI string         `json:"openapi"`
	Info    map[string]any `json:"info"`
	Paths   map[string]any `json:"paths"`
}

// Parse loads the embedded OpenAPI document.
func Parse() (Document, error) {
	var doc Document
	if err := json.Unmarshal(SpecJSON, &doc); err != nil {
		return Document{}, err
	}
	return doc, nil
}

// Handler serves GET /openapi.json.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=60")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(SpecJSON)
	})
}
