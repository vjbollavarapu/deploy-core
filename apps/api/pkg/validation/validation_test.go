package validation_test

import (
	"testing"

	"github.com/deploycore/deploy-core/apps/api/pkg/validation"
)

func TestRequiredAndMaxLen(t *testing.T) {
	t.Parallel()

	var errs validation.Errors
	validation.RequiredString(&errs, "name", "  ")
	validation.MaxLen(&errs, "name", "abcdefghij", 5)
	if errs.Empty() {
		t.Fatal("expected errors")
	}
	details := errs.Details()
	fields, ok := details["fields"].(validation.Errors)
	if !ok || len(fields) != 2 {
		t.Fatalf("details = %#v", details)
	}
}
