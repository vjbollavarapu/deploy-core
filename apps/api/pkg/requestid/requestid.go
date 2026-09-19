package requestid

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

type ctxKey struct{}

// FromContext returns the request ID if present.
func FromContext(ctx context.Context) string {
	if v, ok := ctx.Value(ctxKey{}).(string); ok {
		return v
	}
	return ""
}

// WithContext stores the request ID on the context.
func WithContext(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKey{}, id)
}

// New generates a 16-byte hex request ID.
func New() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "00000000000000000000000000000000"
	}
	return hex.EncodeToString(b[:])
}
