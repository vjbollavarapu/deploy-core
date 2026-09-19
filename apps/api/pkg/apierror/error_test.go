package apierror_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
)

func TestWriteJSONEnvelope(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	apierror.WriteJSON(rec, "req-123", apierror.Validation("invalid", map[string]any{
		"fields": []string{"name"},
	}))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}

	var body struct {
		Error apierror.Error `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != apierror.CodeValidationError {
		t.Fatalf("code = %s", body.Error.Code)
	}
	if body.Error.RequestID != "req-123" {
		t.Fatalf("requestId = %s", body.Error.RequestID)
	}
	if body.Error.Message != "invalid" {
		t.Fatalf("message = %s", body.Error.Message)
	}
}

func TestWriteErrorDoesNotLeakInternal(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	apierror.WriteError(rec, "req-9", errors.New("pq: relation secret does not exist"))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	got := rec.Body.String()
	if strings.Contains(got, "pq:") || strings.Contains(got, "secret") {
		t.Fatalf("leaked internal error: %s", got)
	}

	rec = httptest.NewRecorder()
	apierror.WriteError(rec, "req-9", apierror.NotFoundCode(apierror.CodeOrganizationNotFound, "organization not found"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	var body struct {
		Error apierror.Error `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Error.Code != apierror.CodeOrganizationNotFound {
		t.Fatalf("code = %s", body.Error.Code)
	}
}
