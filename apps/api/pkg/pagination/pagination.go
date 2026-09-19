package pagination

import (
	"net/http"
	"strconv"
)

const (
	DefaultLimit = 20
	MaxLimit     = 100
)

// Params holds cursor-agnostic offset pagination inputs.
type Params struct {
	Limit  int
	Offset int
}

// Page is a generic paginated response envelope.
type Page[T any] struct {
	Items      []T    `json:"items"`
	Limit      int    `json:"limit"`
	Offset     int    `json:"offset"`
	TotalCount *int64 `json:"totalCount,omitempty"`
}

// FromRequest parses limit/offset query params with safe defaults.
func FromRequest(r *http.Request) Params {
	limit := DefaultLimit
	offset := 0

	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > MaxLimit {
		limit = MaxLimit
	}

	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	return Params{Limit: limit, Offset: offset}
}
