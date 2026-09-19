package openapi_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/openapi"
)

func TestSpecParsesAndCoversCorePaths(t *testing.T) {
	doc, err := openapi.Parse()
	if err != nil {
		t.Fatal(err)
	}
	if doc.OpenAPI != "3.1.0" {
		t.Fatalf("openapi version = %q", doc.OpenAPI)
	}
	required := []string{
		"/health",
		"/ready",
		"/openapi.json",
		"/api/v1/auth/login",
		"/api/v1/auth/refresh",
		"/api/v1/organizations",
		"/api/v1/projects",
		"/api/v1/servers",
		"/api/v1/applications",
		"/api/v1/applications/{applicationId}/deployments",
		"/api/v1/applications/{applicationId}/replicas/scale",
		"/api/v1/deployments/{deploymentId}",
		"/api/v1/agents/register",
		"/api/v1/audit-logs",
		"/api/v1/integrations/webhooks",
	}
	for _, p := range required {
		if _, ok := doc.Paths[p]; !ok {
			t.Fatalf("missing path %s", p)
		}
	}
	if len(doc.Paths) < 90 {
		t.Fatalf("expected broad path coverage, got %d", len(doc.Paths))
	}

	var raw map[string]any
	if err := json.Unmarshal(openapi.SpecJSON, &raw); err != nil {
		t.Fatal(err)
	}
	components, _ := raw["components"].(map[string]any)
	schemas, _ := components["schemas"].(map[string]any)
	for _, name := range []string{
		"ErrorEnvelope", "ErrorCode", "Permission", "DeploymentStatus",
		"ServerStatus", "ApplicationType", "AuthResult", "CreateApplicationRequest",
	} {
		if _, ok := schemas[name]; !ok {
			t.Fatalf("missing schema %s", name)
		}
	}
}

func TestHandlerServesJSON(t *testing.T) {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
	openapi.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type=%q", ct)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["openapi"] != "3.1.0" {
		t.Fatalf("body openapi=%v", body["openapi"])
	}
}
