package auth

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
)

// Handler exposes auth HTTP endpoints.
type Handler struct {
	svc     *Service
	limiter *RateLimiter
}

func NewHandler(svc *Service, limiter *RateLimiter) *Handler {
	if limiter == nil {
		limiter = NewRateLimiter(30, time.Minute)
	}
	return &Handler{svc: svc, limiter: limiter}
}

// Mount registers auth routes on mux (paths relative to /api/v1).
func (h *Handler) Mount(mux *http.ServeMux) {
	mux.Handle("POST /auth/register", h.limit(http.HandlerFunc(h.Register)))
	mux.Handle("POST /auth/login", h.limit(http.HandlerFunc(h.Login)))
	mux.Handle("POST /auth/refresh", h.limit(http.HandlerFunc(h.Refresh)))
	mux.Handle("POST /auth/logout", h.limit(http.HandlerFunc(h.Logout)))
	mux.Handle("POST /auth/forgot-password", h.limit(http.HandlerFunc(h.ForgotPassword)))
	mux.Handle("POST /auth/reset-password", h.limit(http.HandlerFunc(h.ResetPassword)))
	mux.Handle("GET /auth/me", h.RequireAuth(http.HandlerFunc(h.Me)))
}

func (h *Handler) limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := clientIP(r) + "|" + r.URL.Path
		if !h.limiter.Allow(key, time.Now().UTC()) {
			apierror.WriteJSON(w, requestid.FromContext(r.Context()), apierror.New(
				http.StatusTooManyRequests,
				apierror.CodeRateLimited,
				"too many authentication attempts",
			))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAuth validates the bearer access token and session.
func (h *Handler) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := bearerToken(r)
		if raw == "" {
			apierror.WriteJSON(w, requestid.FromContext(r.Context()), apierror.Unauthorized("not authenticated"))
			return
		}
		user, sessionID, err := h.svc.AuthenticateAccessToken(r.Context(), raw)
		if err != nil {
			writeErr(w, r, err)
			return
		}
		ctx := WithUser(r.Context(), user)
		ctx = WithSessionID(ctx, sessionID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

type registerRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type logoutRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type forgotRequest struct {
	Email string `json:"email"`
}

type resetRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"newPassword"`
}

type userResponse struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	DisplayName string  `json:"displayName"`
	Status      string  `json:"status"`
	MFAEnabled  bool    `json:"mfaEnabled"`
	LastLoginAt *string `json:"lastLoginAt,omitempty"`
	CreatedAt   string  `json:"createdAt"`
}

type authResponse struct {
	User   userResponse `json:"user"`
	Tokens TokenPair    `json:"tokens"`
	MFA    MFAChallenge `json:"mfa"`
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	result, err := h.svc.Register(r.Context(), RegisterInput{
		Email:       req.Email,
		Password:    req.Password,
		DisplayName: req.DisplayName,
		UserAgent:   r.UserAgent(),
		IP:          clientIP(r),
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, toAuthResponse(result))
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	result, err := h.svc.Login(r.Context(), LoginInput{
		Email:     req.Email,
		Password:  req.Password,
		UserAgent: r.UserAgent(),
		IP:        clientIP(r),
	})
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAuthResponse(result))
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	result, err := h.svc.Refresh(r.Context(), req.RefreshToken, r.UserAgent(), clientIP(r))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, toAuthResponse(result))
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	_ = decodeJSONOptional(r, &req)
	if err := h.svc.Logout(r.Context(), bearerToken(r), req.RefreshToken); err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req forgotRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	if err := h.svc.ForgotPassword(r.Context(), req.Email); err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"message": "if an account exists for that email, a reset link will be sent",
	})
}

func (h *Handler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	var req resetRequest
	if err := decodeJSON(r, &req); err != nil {
		writeErr(w, r, err)
		return
	}
	if err := h.svc.ResetPassword(r.Context(), req.Token, req.NewPassword); err != nil {
		writeErr(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	user, ok := UserFromContext(r.Context())
	if !ok {
		apierror.WriteJSON(w, requestid.FromContext(r.Context()), apierror.Unauthorized("not authenticated"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": toUserResponse(user)})
}

func toAuthResponse(result AuthResult) authResponse {
	return authResponse{
		User:   toUserResponse(result.User),
		Tokens: result.Tokens,
		MFA:    result.MFA,
	}
}

func toUserResponse(u User) userResponse {
	out := userResponse{
		ID:          u.ID.String(),
		Email:       u.Email,
		DisplayName: u.DisplayName,
		Status:      u.Status,
		MFAEnabled:  u.MFAEnabled,
		CreatedAt:   u.CreatedAt.UTC().Format(time.RFC3339Nano),
	}
	if u.LastLoginAt != nil {
		s := u.LastLoginAt.UTC().Format(time.RFC3339Nano)
		out.LastLoginAt = &s
	}
	return out
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return apierror.Validation("invalid JSON body", nil)
	}
	return nil
}

func decodeJSONOptional(r *http.Request, dst any) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(dst); err != nil {
		return nil
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	apierror.WriteError(w, requestid.FromContext(r.Context()), err)
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(h, prefix))
}

func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
