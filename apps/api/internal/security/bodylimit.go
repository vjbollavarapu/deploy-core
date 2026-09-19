package security

import (
	"net/http"
)

// MaxBytesMiddleware rejects request bodies larger than limit bytes.
func MaxBytesMiddleware(limit int64) func(http.Handler) http.Handler {
	if limit <= 0 {
		limit = 1 << 20 // 1 MiB
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodHead &&
				r.Method != http.MethodOptions && r.Method != http.MethodTrace {
				r.Body = http.MaxBytesReader(w, r.Body, limit)
			}
			next.ServeHTTP(w, r)
		})
	}
}
