package server

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/security"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
)

func requestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if strings.TrimSpace(id) == "" {
			id = requestid.New()
		}
		ctx := requestid.WithContext(r.Context(), id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func loggingMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			log.Info("request",
				slog.String("request_id", requestid.FromContext(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", security.SanitizeLogField(r.URL.Path, 512)),
				slog.Int("status", rw.status),
				slog.Duration("duration", time.Since(start)),
				slog.String("remote_addr", security.SanitizeLogField(r.RemoteAddr, 128)),
				slog.String("user_agent", security.SanitizeLogField(r.UserAgent(), 256)),
			)
		})
	}
}

func recoverMiddleware(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered",
						slog.Any("panic", rec),
						slog.String("request_id", requestid.FromContext(r.Context())),
						slog.String("stack", string(debug.Stack())),
					)
					apierror.WriteJSON(w, requestid.FromContext(r.Context()), apierror.Internal("internal server error"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

func secureHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func corsMiddleware(allowed []string) func(http.Handler) http.Handler {
	allowAll := len(allowed) == 1 && allowed[0] == "*"
	allowedSet := map[string]struct{}{}
	for _, o := range allowed {
		allowedSet[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && (allowAll || containsOrigin(allowedSet, origin)) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Request-ID")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func containsOrigin(set map[string]struct{}, origin string) bool {
	_, ok := set[origin]
	return ok
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
