package validation

import (
	"fmt"
	"strings"
)

// FieldError is a single field validation failure.
type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Errors aggregates field errors for API details.
type Errors []FieldError

func (e Errors) Empty() bool { return len(e) == 0 }

func (e *Errors) Add(field, message string) {
	*e = append(*e, FieldError{Field: field, Message: message})
}

func (e Errors) Details() map[string]any {
	return map[string]any{"fields": e}
}

// RequiredString rejects blank trimmed strings.
func RequiredString(errs *Errors, field, value string) {
	if strings.TrimSpace(value) == "" {
		errs.Add(field, "is required")
	}
}

// MaxLen rejects values longer than max.
func MaxLen(errs *Errors, field, value string, max int) {
	if len(value) > max {
		errs.Add(field, fmt.Sprintf("must be at most %d characters", max))
	}
}
