package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/deploycore/deploy-core/apps/api/pkg/validation"
	"github.com/google/uuid"
)

// PasswordResetNotifier delivers reset tokens out-of-band (email later).
type PasswordResetNotifier interface {
	NotifyPasswordReset(ctx context.Context, email, rawToken string) error
}

// LogPasswordResetNotifier logs reset tokens in non-production for local testing.
// Never log tokens in production deployments.
type LogPasswordResetNotifier struct {
	Log *slog.Logger
}

func (n LogPasswordResetNotifier) NotifyPasswordReset(ctx context.Context, email, rawToken string) error {
	n.Log.Info("password reset token issued",
		slog.String("email", email),
		slog.String("request_id", requestid.FromContext(ctx)),
		slog.String("token", rawToken),
	)
	return nil
}

// NopPasswordResetNotifier discards reset notifications.
type NopPasswordResetNotifier struct{}

func (NopPasswordResetNotifier) NotifyPasswordReset(context.Context, string, string) error {
	return nil
}

// AuditSink records security events without importing the audit package (avoids import cycles).
type AuditSink interface {
	WriteAuthEvent(ctx context.Context, actorUserID *uuid.UUID, action, resourceType, resourceID, ip, userAgent string, after map[string]any) error
}

// ServiceConfig holds auth token and TTL settings.
type ServiceConfig struct {
	AccessTokenSecret []byte
	AccessTokenTTL    time.Duration
	RefreshTokenTTL   time.Duration
	PasswordResetTTL  time.Duration
	MinPasswordLength int
}

// Service implements authentication use cases.
type Service struct {
	repo     Repository
	cfg      ServiceConfig
	log      *slog.Logger
	notifier PasswordResetNotifier
	audit    AuditSink
	now      func() time.Time
}

func NewService(repo Repository, cfg ServiceConfig, log *slog.Logger, notifier PasswordResetNotifier) *Service {
	if notifier == nil {
		notifier = NopPasswordResetNotifier{}
	}
	if cfg.MinPasswordLength <= 0 {
		cfg.MinPasswordLength = 8
	}
	if cfg.AccessTokenTTL <= 0 {
		cfg.AccessTokenTTL = 15 * time.Minute
	}
	if cfg.RefreshTokenTTL <= 0 {
		cfg.RefreshTokenTTL = 30 * 24 * time.Hour
	}
	if cfg.PasswordResetTTL <= 0 {
		cfg.PasswordResetTTL = time.Hour
	}
	return &Service{
		repo:     repo,
		cfg:      cfg,
		log:      log,
		notifier: notifier,
		now:      time.Now,
	}
}

func (s *Service) WithAudit(a AuditSink) *Service {
	s.audit = a
	return s
}

type RegisterInput struct {
	Email       string
	Password    string
	DisplayName string
	UserAgent   string
	IP          string
}

type LoginInput struct {
	Email     string
	Password  string
	UserAgent string
	IP        string
}

type AuthResult struct {
	User   User         `json:"user"`
	Tokens TokenPair    `json:"tokens"`
	MFA    MFAChallenge `json:"mfa"`
}

func (s *Service) Register(ctx context.Context, in RegisterInput) (AuthResult, error) {
	if err := validateCredentials(in.Email, in.Password, s.cfg.MinPasswordLength); err != nil {
		return AuthResult{}, err
	}
	display := strings.TrimSpace(in.DisplayName)
	if display == "" {
		display = strings.Split(normalizeEmail(in.Email), "@")[0]
	}
	hash, err := crypto.HashPassword(in.Password)
	if err != nil {
		return AuthResult{}, apierror.Internal("could not create account")
	}
	user, err := s.repo.CreateUser(ctx, in.Email, hash, display)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return AuthResult{}, apierror.Conflict("email already registered")
		}
		return AuthResult{}, err
	}
	result, err := s.issueForUser(ctx, user, in.UserAgent, in.IP, nil)
	if err != nil {
		return AuthResult{}, err
	}
	s.writeAudit(ctx, &user.ID, "auth.register", "user", user.ID.String(), in.IP, in.UserAgent, nil, map[string]any{
		"email": user.Email,
	})
	return result.AuthResult, nil
}

func (s *Service) Login(ctx context.Context, in LoginInput) (AuthResult, error) {
	if err := validateCredentials(in.Email, in.Password, s.cfg.MinPasswordLength); err != nil {
		return AuthResult{}, err
	}
	user, err := s.repo.GetUserByEmail(ctx, in.Email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			s.writeAudit(ctx, nil, "auth.login.failure", "user", "", in.IP, in.UserAgent, nil, map[string]any{
				"email":  normalizeEmail(in.Email),
				"reason": "not_found",
			})
			return AuthResult{}, apierror.Unauthorized("invalid email or password")
		}
		return AuthResult{}, err
	}
	if user.Status == UserStatusDisabled {
		s.writeAudit(ctx, &user.ID, "auth.login.denied", "user", user.ID.String(), in.IP, in.UserAgent, nil, map[string]any{
			"email":  user.Email,
			"reason": "disabled",
		})
		return AuthResult{}, apierror.Forbidden("account disabled")
	}
	if user.Status != UserStatusActive && user.Status != UserStatusPending {
		s.writeAudit(ctx, &user.ID, "auth.login.denied", "user", user.ID.String(), in.IP, in.UserAgent, nil, map[string]any{
			"email":  user.Email,
			"reason": "status_" + user.Status,
		})
		return AuthResult{}, apierror.Forbidden("account is not allowed to sign in")
	}
	ok, err := crypto.VerifyPassword(user.PasswordHash, in.Password)
	if err != nil || !ok {
		s.writeAudit(ctx, &user.ID, "auth.login.failure", "user", user.ID.String(), in.IP, in.UserAgent, nil, map[string]any{
			"email":  user.Email,
			"reason": "bad_password",
		})
		return AuthResult{}, apierror.Unauthorized("invalid email or password")
	}
	_ = s.repo.UpdateLastLogin(ctx, user.ID, s.now().UTC())
	user.LastLoginAt = ptrTime(s.now().UTC())
	result, err := s.issueForUser(ctx, user, in.UserAgent, in.IP, nil)
	if err != nil {
		return AuthResult{}, err
	}
	s.writeAudit(ctx, &user.ID, "auth.login.success", "user", user.ID.String(), in.IP, in.UserAgent, nil, map[string]any{
		"email": user.Email,
	})
	return result.AuthResult, nil
}

func (s *Service) Refresh(ctx context.Context, refreshToken, userAgent, ip string) (AuthResult, error) {
	if strings.TrimSpace(refreshToken) == "" {
		return AuthResult{}, apierror.Unauthorized("invalid refresh token")
	}
	hash := crypto.HashTokenSHA256(refreshToken)
	session, err := s.repo.GetSessionByRefreshHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return AuthResult{}, apierror.Unauthorized("invalid refresh token")
		}
		return AuthResult{}, err
	}
	now := s.now().UTC()
	if session.RevokedAt != nil {
		// Reuse of a rotated refresh token → kill the whole family.
		_ = s.repo.RevokeSessionFamily(ctx, session.FamilyID, now)
		return AuthResult{}, apierror.Unauthorized("invalid refresh token")
	}
	if session.ExpiresAt.Before(now) {
		_ = s.repo.RevokeSession(ctx, session.ID, now)
		return AuthResult{}, apierror.Unauthorized("invalid refresh token")
	}
	user, err := s.repo.GetUserByID(ctx, session.UserID)
	if err != nil {
		return AuthResult{}, apierror.Unauthorized("invalid refresh token")
	}
	if user.Status == UserStatusDisabled {
		_ = s.repo.RevokeSessionFamily(ctx, session.FamilyID, now)
		return AuthResult{}, apierror.Forbidden("account disabled")
	}
	// Atomic rotate: only one concurrent refresh wins.
	if _, err := s.repo.RevokeSessionAtomic(ctx, session.ID, now); err != nil {
		if errors.Is(err, ErrConflict) {
			_ = s.repo.RevokeSessionFamily(ctx, session.FamilyID, now)
			return AuthResult{}, apierror.Unauthorized("invalid refresh token")
		}
		return AuthResult{}, err
	}
	result, err := s.issueForUser(ctx, user, userAgent, ip, &session.FamilyID)
	if err != nil {
		return AuthResult{}, err
	}
	// Best-effort link; family revoke already protects on reuse.
	_ = s.repo.MarkSessionReplaced(ctx, session.ID, result.SessionID)
	return result.AuthResult, nil
}

func (s *Service) Logout(ctx context.Context, accessToken, refreshToken string) error {
	now := s.now().UTC()
	var actorID *uuid.UUID
	if strings.TrimSpace(accessToken) != "" {
		userID, sessionID, err := parseAccessToken(s.cfg.AccessTokenSecret, accessToken)
		if err == nil {
			_ = s.repo.RevokeSession(ctx, sessionID, now)
			actorID = &userID
		}
	}
	if strings.TrimSpace(refreshToken) != "" {
		hash := crypto.HashTokenSHA256(refreshToken)
		session, err := s.repo.GetSessionByRefreshHash(ctx, hash)
		if err == nil {
			_ = s.repo.RevokeSession(ctx, session.ID, now)
			if actorID == nil {
				id := session.UserID
				actorID = &id
			}
		}
	}
	rid := ""
	if actorID != nil {
		rid = actorID.String()
	}
	s.writeAudit(ctx, actorID, "auth.logout", "user", rid, "", "", nil, nil)
	return nil
}

func (s *Service) ForgotPassword(ctx context.Context, email string) error {
	email = normalizeEmail(email)
	if _, err := mail.ParseAddress(email); err != nil {
		return apierror.Validation("invalid email", map[string]any{
			"fields": validation.Errors{{Field: "email", Message: "must be a valid email"}},
		})
	}
	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil // no enumeration
		}
		return err
	}
	if user.Status == UserStatusDisabled {
		return nil
	}
	raw, err := crypto.RandomURLToken(32)
	if err != nil {
		return apierror.Internal("could not start password reset")
	}
	hash := crypto.HashTokenSHA256(raw)
	reqID := requestid.FromContext(ctx)
	var reqPtr *string
	if reqID != "" {
		reqPtr = &reqID
	}
	if _, err := s.repo.CreatePasswordResetToken(ctx, user.ID, hash, s.now().UTC().Add(s.cfg.PasswordResetTTL), reqPtr); err != nil {
		return err
	}
	if err := s.notifier.NotifyPasswordReset(ctx, user.Email, raw); err != nil {
		s.log.Error("password reset notify failed", slog.String("error", err.Error()))
	}
	s.writeAudit(ctx, &user.ID, "auth.password_reset.request", "user", user.ID.String(), "", "", nil, map[string]any{
		"email": user.Email,
	})
	return nil
}

func (s *Service) ResetPassword(ctx context.Context, rawToken, newPassword string) error {
	if strings.TrimSpace(rawToken) == "" {
		return apierror.Validation("invalid token", map[string]any{
			"fields": validation.Errors{{Field: "token", Message: "is required"}},
		})
	}
	var errs validation.Errors
	validation.RequiredString(&errs, "password", newPassword)
	if len(newPassword) < s.cfg.MinPasswordLength {
		errs.Add("password", "must be at least 8 characters")
	}
	if !errs.Empty() {
		return apierror.Validation("invalid password", errs.Details())
	}
	hash := crypto.HashTokenSHA256(rawToken)
	token, err := s.repo.GetPasswordResetByHash(ctx, hash)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.Unauthorized("invalid or expired reset token")
		}
		return err
	}
	now := s.now().UTC()
	if token.UsedAt != nil || token.ExpiresAt.Before(now) {
		return apierror.Unauthorized("invalid or expired reset token")
	}
	pwHash, err := crypto.HashPassword(newPassword)
	if err != nil {
		return apierror.Internal("could not reset password")
	}
	if err := s.repo.UpdatePassword(ctx, token.UserID, pwHash); err != nil {
		return err
	}
	if err := s.repo.MarkPasswordResetUsed(ctx, token.ID, now); err != nil {
		return err
	}
	_ = s.repo.RevokeAllUserSessions(ctx, token.UserID, now)
	s.writeAudit(ctx, &token.UserID, "auth.password_reset.complete", "user", token.UserID.String(), "", "", nil, nil)
	return nil
}

func (s *Service) Me(ctx context.Context, userID uuid.UUID) (User, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return User{}, apierror.Unauthorized("not authenticated")
		}
		return User{}, err
	}
	if user.Status == UserStatusDisabled {
		return User{}, apierror.Forbidden("account disabled")
	}
	return user.User, nil
}

// AuthenticateAccessToken validates a bearer access token and active session.
func (s *Service) AuthenticateAccessToken(ctx context.Context, raw string) (User, uuid.UUID, error) {
	userID, sessionID, err := parseAccessToken(s.cfg.AccessTokenSecret, raw)
	if err != nil {
		return User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	session, err := s.repo.GetSessionByID(ctx, sessionID)
	if err != nil || session.RevokedAt != nil || session.ExpiresAt.Before(s.now().UTC()) || session.UserID != userID {
		return User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return User{}, uuid.Nil, apierror.Unauthorized("not authenticated")
	}
	if user.Status == UserStatusDisabled {
		return User{}, uuid.Nil, apierror.Forbidden("account disabled")
	}
	return user.User, sessionID, nil
}

func (s *Service) issueForUser(ctx context.Context, user UserRecord, userAgent, ip string, familyID *uuid.UUID) (issuedAuth, error) {
	rawRefresh, err := crypto.RandomURLToken(32)
	if err != nil {
		return issuedAuth{}, apierror.Internal("could not create session")
	}
	refreshHash := crypto.HashTokenSHA256(rawRefresh)
	now := s.now().UTC()
	var ua, ipPtr *string
	if strings.TrimSpace(userAgent) != "" {
		ua = &userAgent
	}
	if strings.TrimSpace(ip) != "" {
		ipPtr = &ip
	}
	session, err := s.repo.CreateSession(ctx, user.ID, refreshHash, now.Add(s.cfg.RefreshTokenTTL), ua, ipPtr, familyID)
	if err != nil {
		return issuedAuth{}, err
	}
	access, accessExp, err := issueAccessToken(s.cfg.AccessTokenSecret, user.ID, session.ID, s.cfg.AccessTokenTTL, now)
	if err != nil {
		return issuedAuth{}, apierror.Internal("could not issue access token")
	}
	return issuedAuth{
		AuthResult: AuthResult{
			User: user.User,
			Tokens: TokenPair{
				AccessToken:      access,
				RefreshToken:     rawRefresh,
				AccessExpiresAt:  accessExp,
				RefreshExpiresAt: session.ExpiresAt,
				TokenType:        "Bearer",
			},
			MFA: MFAChallenge{
				Required: false,
				Methods:  nil,
			},
		},
		SessionID: session.ID,
	}, nil
}

type issuedAuth struct {
	AuthResult
	SessionID uuid.UUID
}

func validateCredentials(email, password string, minLen int) error {
	var errs validation.Errors
	email = strings.TrimSpace(email)
	validation.RequiredString(&errs, "email", email)
	if email != "" {
		if _, err := mail.ParseAddress(email); err != nil {
			errs.Add("email", "must be a valid email")
		}
	}
	validation.RequiredString(&errs, "password", password)
	if password != "" && len(password) < minLen {
		errs.Add("password", "must be at least 8 characters")
	}
	if !errs.Empty() {
		return apierror.Validation("invalid request", errs.Details())
	}
	return nil
}

func (s *Service) writeAudit(ctx context.Context, actorID *uuid.UUID, action, resourceType, resourceID, ip, ua string, before, after map[string]any) {
	if s.audit == nil {
		return
	}
	_ = before
	if err := s.audit.WriteAuthEvent(ctx, actorID, action, resourceType, resourceID, ip, ua, after); err != nil && s.log != nil {
		s.log.Error("audit write failed", slog.String("error", err.Error()), slog.String("action", action))
	}
}

func ptrTime(t time.Time) *time.Time { return &t }
