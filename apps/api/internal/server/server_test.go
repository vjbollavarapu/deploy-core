package server_test

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
)

func TestHealthAndRequestID(t *testing.T) {
	t.Parallel()

	cfg := config.Config{
		HTTPAddr:           ":0",
		CORSAllowedOrigins: []string{"http://localhost:3000"},
		ReadHeaderTimeout:  0,
	}
	srv := server.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	srv.HTTPHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID")
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("expected secure headers")
	}

	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("body = %#v", body)
	}
}

func TestAPIVersionRoot(t *testing.T) {
	t.Parallel()

	cfg := config.Config{CORSAllowedOrigins: []string{"*"}}
	srv := server.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/", nil)
	rec := httptest.NewRecorder()
	srv.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestOpenAPIJSON(t *testing.T) {
	t.Parallel()

	cfg := config.Config{CORSAllowedOrigins: []string{"*"}}
	srv := server.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)

	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	rec := httptest.NewRecorder()
	srv.HTTPHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["openapi"] != "3.1.0" {
		t.Fatalf("openapi=%v", body["openapi"])
	}
}
