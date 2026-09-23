package organizations

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/deploycore/deploy-core/apps/api/internal/audit"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/deploycore/deploy-core/apps/api/pkg/requestid"
	"github.com/deploycore/deploy-core/apps/api/pkg/validation"
	"github.com/google/uuid"
)

// InvitationNotifier delivers invite tokens out-of-band.
type InvitationNotifier interface {
	NotifyInvitation(ctx context.Context, email, orgName, rawToken string) error
}

type NopInvitationNotifier struct{}

func (NopInvitationNotifier) NotifyInvitation(context.Context, string, string, string) error {
	return nil
}

type LogInvitationNotifier struct {
	Log *slog.Logger
}

func (n LogInvitationNotifier) NotifyInvitation(ctx context.Context, email, orgName, rawToken string) error {
	n.Log.Info("organization invitation issued",
		slog.String("email", email),
		slog.String("organization", orgName),
		slog.String("request_id", requestid.FromContext(ctx)),
		slog.String("token", rawToken),
	)
	return nil
}

type Service struct {
	repo      Repository
	authz     *rbac.Authorizer
	audit     *audit.Writer
	log       *slog.Logger
	notifier  InvitationNotifier
	now       func() time.Time
	inviteTTL time.Duration
}

func NewService(repo Repository, authz *rbac.Authorizer, auditWriter *audit.Writer, log *slog.Logger, notifier InvitationNotifier) *Service {
	if notifier == nil {
		notifier = NopInvitationNotifier{}
	}
	return &Service{
		repo:      repo,
		authz:     authz,
		audit:     auditWriter,
		log:       log,
		notifier:  notifier,
		now:       time.Now,
		inviteTTL: 7 * 24 * time.Hour,
	}
}

type AuditMeta struct {
	IP        string
	UserAgent string
}

func (s *Service) Create(ctx context.Context, actorID uuid.UUID, name, slug string, meta AuditMeta) (Organization, error) {
	name = strings.TrimSpace(name)
	var errs validation.Errors
	validation.RequiredString(&errs, "name", name)
	validation.MaxLen(&errs, "name", name, 120)
	slug = strings.TrimSpace(slug)
	if slug == "" {
		slug = slugify(name)
	}
	if err := validateSlug(slug); err != nil {
		errs.Add("slug", err.Error())
	}
	if !errs.Empty() {
		return Organization{}, apierror.Validation("invalid organization", errs.Details())
	}

	org, err := s.repo.CreateOrganization(ctx, name, slug, actorID)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Organization{}, apierror.Conflict("organization slug already exists")
		}
		return Organization{}, err
	}

	ownerRoleID, err := s.repo.GetSystemRoleID(ctx, rbac.RoleOwner)
	if err != nil {
		return Organization{}, apierror.Internal("owner role not seeded")
	}
	now := s.now().UTC()
	memberID, err := s.repo.CreateMember(ctx, org.ID, actorID, "active", nil, &now)
	if err != nil {
		return Organization{}, err
	}
	if err := s.repo.ReplaceMemberRoles(ctx, memberID, []uuid.UUID{ownerRoleID}); err != nil {
		return Organization{}, err
	}

	s.writeAudit(ctx, &org.ID, &actorID, "organization.create", "organization", org.ID.String(), meta, nil, map[string]any{
		"name": org.Name, "slug": org.Slug,
	})
	return org, nil
}

func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID, limit, offset int) ([]Organization, int64, error) {
	return s.repo.ListOrganizationsForUser(ctx, userID, limit, offset)
}

func (s *Service) Get(ctx context.Context, actorID, orgID uuid.UUID) (Organization, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.OrganizationRead); err != nil {
		return Organization{}, err
	}
	org, err := s.repo.GetOrganization(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Organization{}, apierror.NotFoundCode(apierror.CodeOrganizationNotFound, "organization not found")
		}
		return Organization{}, err
	}
	return org, nil
}

func (s *Service) Update(ctx context.Context, actorID, orgID uuid.UUID, name, slug *string, meta AuditMeta) (Organization, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.OrganizationUpdate); err != nil {
		return Organization{}, err
	}
	before, err := s.repo.GetOrganization(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Organization{}, apierror.NotFoundCode(apierror.CodeOrganizationNotFound, "organization not found")
		}
		return Organization{}, err
	}

	var errs validation.Errors
	var namePtr, slugPtr *string
	if name != nil {
		n := strings.TrimSpace(*name)
		validation.RequiredString(&errs, "name", n)
		validation.MaxLen(&errs, "name", n, 120)
		namePtr = &n
	}
	if slug != nil {
		sl := strings.TrimSpace(*slug)
		if err := validateSlug(sl); err != nil {
			errs.Add("slug", err.Error())
		}
		slugPtr = &sl
	}
	if !errs.Empty() {
		return Organization{}, apierror.Validation("invalid organization", errs.Details())
	}

	org, err := s.repo.UpdateOrganization(ctx, orgID, namePtr, slugPtr)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Organization{}, apierror.Conflict("organization slug already exists")
		}
		if errors.Is(err, ErrNotFound) {
			return Organization{}, apierror.NotFoundCode(apierror.CodeOrganizationNotFound, "organization not found")
		}
		return Organization{}, err
	}
	s.writeAudit(ctx, &orgID, &actorID, "organization.update", "organization", orgID.String(), meta,
		map[string]any{"name": before.Name, "slug": before.Slug},
		map[string]any{"name": org.Name, "slug": org.Slug},
	)
	return org, nil
}

func (s *Service) ListMembers(ctx context.Context, actorID, orgID uuid.UUID, limit, offset int) ([]Member, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.MemberRead); err != nil {
		return nil, 0, err
	}
	if _, err := s.repo.GetOrganization(ctx, orgID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, 0, apierror.NotFoundCode(apierror.CodeOrganizationNotFound, "organization not found")
		}
		return nil, 0, err
	}
	return s.repo.ListMembers(ctx, orgID, limit, offset)
}

func (s *Service) Invite(ctx context.Context, actorID, orgID uuid.UUID, email string, roleKeys []string, meta AuditMeta) (Invitation, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.MemberInvite); err != nil {
		return Invitation{}, err
	}
	org, err := s.repo.GetOrganization(ctx, orgID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Invitation{}, apierror.NotFoundCode(apierror.CodeOrganizationNotFound, "organization not found")
		}
		return Invitation{}, err
	}

	email = strings.ToLower(strings.TrimSpace(email))
	var errs validation.Errors
	validation.RequiredString(&errs, "email", email)
	if len(roleKeys) == 0 {
		errs.Add("roleKeys", "must include at least one role")
	}
	if !errs.Empty() {
		return Invitation{}, apierror.Validation("invalid invitation", errs.Details())
	}

	roles, err := s.resolveRoles(ctx, roleKeys)
	if err != nil {
		return Invitation{}, err
	}
	roleIDs := make([]uuid.UUID, len(roles))
	for i, role := range roles {
		roleIDs[i] = role.ID
	}

	if userID, findErr := s.repo.FindUserIDByEmail(ctx, email); findErr == nil {
		if mem, mErr := s.repo.GetMemberByUser(ctx, orgID, userID); mErr == nil && mem.Status == "active" {
			return Invitation{}, apierror.Conflict("user is already a member")
		} else if mErr == nil && mem.Status == "invited" {
			if err := s.repo.ReplaceMemberRoles(ctx, mem.ID, roleIDs); err != nil {
				return Invitation{}, err
			}
		} else if errors.Is(mErr, ErrNotFound) {
			invitedBy := actorID
			if _, err := s.repo.CreateMember(ctx, orgID, userID, "invited", &invitedBy, nil); err != nil && !errors.Is(err, ErrConflict) {
				return Invitation{}, err
			}
			mem, err := s.repo.GetMemberByUser(ctx, orgID, userID)
			if err != nil {
				return Invitation{}, err
			}
			if err := s.repo.ReplaceMemberRoles(ctx, mem.ID, roleIDs); err != nil {
				return Invitation{}, err
			}
		} else if mErr != nil {
			return Invitation{}, mErr
		}
	} else if !errors.Is(findErr, ErrNotFound) {
		return Invitation{}, findErr
	}

	_ = s.repo.RevokeOpenInvitationsForEmail(ctx, orgID, email, s.now().UTC())

	raw, err := crypto.RandomURLToken(32)
	if err != nil {
		return Invitation{}, apierror.Internal("could not create invitation")
	}
	hash := crypto.HashTokenSHA256(raw)
	invitedBy := actorID
	inv, err := s.repo.CreateInvitation(ctx, orgID, email, &invitedBy, hash, s.now().UTC().Add(s.inviteTTL), roleIDs)
	if err != nil {
		if errors.Is(err, ErrConflict) {
			return Invitation{}, apierror.Conflict("invitation already pending")
		}
		return Invitation{}, err
	}
	inv.RawToken = raw
	_ = s.notifier.NotifyInvitation(ctx, email, org.Name, raw)

	s.writeAudit(ctx, &orgID, &actorID, "organization.invite", "invitation", inv.ID.String(), meta, nil, map[string]any{
		"email": email, "roles": roleKeys,
	})
	return inv, nil
}

func (s *Service) ListInvitations(ctx context.Context, actorID, orgID uuid.UUID, limit, offset int) ([]Invitation, int64, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.MemberRead); err != nil {
		return nil, 0, err
	}
	if _, err := s.repo.GetOrganization(ctx, orgID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, 0, apierror.NotFoundCode(apierror.CodeOrganizationNotFound, "organization not found")
		}
		return nil, 0, err
	}
	return s.repo.ListInvitations(ctx, orgID, limit, offset)
}

func (s *Service) RevokeInvitation(ctx context.Context, actorID, orgID, invitationID uuid.UUID, meta AuditMeta) error {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.MemberInvite); err != nil {
		return err
	}
	if _, err := s.repo.GetOrganization(ctx, orgID); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFoundCode(apierror.CodeOrganizationNotFound, "organization not found")
		}
		return err
	}
	now := s.now().UTC()
	if err := s.repo.RevokeInvitation(ctx, orgID, invitationID, now); err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFound("invitation not found or already closed")
		}
		return err
	}
	s.writeAudit(ctx, &orgID, &actorID, "organization.invitation.revoke", "invitation", invitationID.String(), meta, nil, nil)
	return nil
}

func (s *Service) AcceptInvitation(ctx context.Context, actorID uuid.UUID, actorEmail, rawToken string, meta AuditMeta) (Member, error) {
	if strings.TrimSpace(rawToken) == "" {
		return Member{}, apierror.Validation("invalid token", map[string]any{
			"fields": validation.Errors{{Field: "token", Message: "is required"}},
		})
	}
	inv, err := s.repo.GetInvitationByTokenHash(ctx, crypto.HashTokenSHA256(rawToken))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Member{}, apierror.Unauthorized("invalid or expired invitation")
		}
		return Member{}, err
	}
	now := s.now().UTC()
	if inv.AcceptedAt != nil || inv.RevokedAt != nil || inv.ExpiresAt.Before(now) {
		return Member{}, apierror.Unauthorized("invalid or expired invitation")
	}
	if !strings.EqualFold(inv.Email, actorEmail) {
		return Member{}, apierror.Forbidden("invitation email does not match authenticated user")
	}

	roleIDs := make([]uuid.UUID, len(inv.Roles))
	for i, role := range inv.Roles {
		roleIDs[i] = role.ID
	}

	mem, err := s.repo.GetMemberByUser(ctx, inv.OrganizationID, actorID)
	if errors.Is(err, ErrNotFound) {
		invitedBy := inv.InvitedBy
		memberID, cErr := s.repo.CreateMember(ctx, inv.OrganizationID, actorID, "active", invitedBy, &now)
		if cErr != nil {
			return Member{}, cErr
		}
		if err := s.repo.ReplaceMemberRoles(ctx, memberID, roleIDs); err != nil {
			return Member{}, err
		}
	} else if err != nil {
		return Member{}, err
	} else {
		if err := s.repo.SetMemberStatus(ctx, mem.ID, "active", &now); err != nil {
			return Member{}, err
		}
		if err := s.repo.ReplaceMemberRoles(ctx, mem.ID, roleIDs); err != nil {
			return Member{}, err
		}
	}

	if err := s.repo.MarkInvitationAccepted(ctx, inv.ID, now); err != nil {
		return Member{}, err
	}
	mem, err = s.repo.GetMemberByUser(ctx, inv.OrganizationID, actorID)
	if err != nil {
		return Member{}, err
	}
	s.writeAudit(ctx, &inv.OrganizationID, &actorID, "organization.invitation.accept", "invitation", inv.ID.String(), meta, nil, map[string]any{
		"memberId": mem.ID.String(),
	})
	return mem, nil
}

func (s *Service) UpdateMemberRoles(ctx context.Context, actorID, orgID, memberID uuid.UUID, roleKeys []string, meta AuditMeta) (Member, error) {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.MemberUpdate); err != nil {
		return Member{}, err
	}
	if len(roleKeys) == 0 {
		return Member{}, apierror.Validation("invalid roles", map[string]any{
			"fields": validation.Errors{{Field: "roleKeys", Message: "must include at least one role"}},
		})
	}
	roles, err := s.resolveRoles(ctx, roleKeys)
	if err != nil {
		return Member{}, err
	}
	member, err := s.repo.GetMember(ctx, orgID, memberID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Member{}, apierror.NotFound("member not found")
		}
		return Member{}, err
	}

	wasOwner, err := s.repo.MemberHasRole(ctx, member.ID, rbac.RoleOwner)
	if err != nil {
		return Member{}, err
	}
	willBeOwner := false
	roleIDs := make([]uuid.UUID, len(roles))
	for i, role := range roles {
		roleIDs[i] = role.ID
		if role.Key == rbac.RoleOwner {
			willBeOwner = true
		}
	}
	if wasOwner && !willBeOwner && member.Status == "active" {
		owners, err := s.repo.CountActiveOwners(ctx, orgID)
		if err != nil {
			return Member{}, err
		}
		if owners <= 1 {
			return Member{}, apierror.Conflict("cannot remove the last organization owner")
		}
	}

	beforeKeys := roleKeysOf(member.Roles)
	if err := s.repo.ReplaceMemberRoles(ctx, member.ID, roleIDs); err != nil {
		return Member{}, err
	}
	updated, err := s.repo.GetMember(ctx, orgID, memberID)
	if err != nil {
		return Member{}, err
	}
	s.writeAudit(ctx, &orgID, &actorID, "organization.member.update_roles", "member", memberID.String(), meta,
		map[string]any{"roles": beforeKeys},
		map[string]any{"roles": roleKeysOf(updated.Roles)},
	)
	return updated, nil
}

func (s *Service) RemoveMember(ctx context.Context, actorID, orgID, memberID uuid.UUID, meta AuditMeta) error {
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.MemberRemove); err != nil {
		return err
	}
	member, err := s.repo.GetMember(ctx, orgID, memberID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFound("member not found")
		}
		return err
	}
	isOwner, err := s.repo.MemberHasRole(ctx, member.ID, rbac.RoleOwner)
	if err != nil {
		return err
	}
	if isOwner && member.Status == "active" {
		owners, err := s.repo.CountActiveOwners(ctx, orgID)
		if err != nil {
			return err
		}
		if owners <= 1 {
			return apierror.Conflict("cannot remove the last organization owner")
		}
	}
	if err := s.repo.DeleteMember(ctx, orgID, memberID); err != nil {
		return err
	}
	s.writeAudit(ctx, &orgID, &actorID, "organization.member.remove", "member", memberID.String(), meta,
		map[string]any{"userId": member.UserID.String(), "email": member.Email}, nil,
	)
	return nil
}

func (s *Service) resolveRoles(ctx context.Context, keys []string) ([]Role, error) {
	normalized := make([]string, 0, len(keys))
	seen := map[string]struct{}{}
	for _, k := range keys {
		k = strings.ToLower(strings.TrimSpace(k))
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		normalized = append(normalized, k)
	}
	roles, err := s.repo.GetSystemRolesByKeys(ctx, normalized)
	if err != nil {
		return nil, err
	}
	if len(roles) != len(normalized) {
		return nil, apierror.Validation("unknown role", map[string]any{
			"fields": validation.Errors{{Field: "roleKeys", Message: "contains unknown role key"}},
		})
	}
	return roles, nil
}

func (s *Service) writeAudit(ctx context.Context, orgID, actorID *uuid.UUID, action, resourceType, resourceID string, meta AuditMeta, before, after map[string]any) {
	if s.audit == nil {
		return
	}
	if err := s.audit.Write(ctx, audit.Entry{
		OrganizationID: orgID,
		ActorUserID:    actorID,
		Action:         action,
		ResourceType:   resourceType,
		ResourceID:     resourceID,
		RequestID:      requestid.FromContext(ctx),
		IPAddress:      meta.IP,
		UserAgent:      meta.UserAgent,
		Before:         before,
		After:          after,
	}); err != nil {
		s.log.Error("audit write failed", slog.String("error", err.Error()), slog.String("action", action))
	}
}

func roleKeysOf(roles []Role) []string {
	out := make([]string, 0, len(roles))
	for _, r := range roles {
		out = append(out, r.Key)
	}
	return out
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func validateSlug(slug string) error {
	if slug == "" {
		return errors.New("is required")
	}
	if !slugPattern.MatchString(slug) {
		return errors.New("must be lowercase alphanumeric with optional hyphens")
	}
	return nil
}

func slugify(name string) string {
	var b strings.Builder
	lastHyphen := false
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastHyphen = false
			continue
		}
		if (r == ' ' || r == '-' || r == '_') && !lastHyphen && b.Len() > 0 {
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "org"
	}
	if len(out) > 63 {
		out = strings.Trim(out[:63], "-")
	}
	return out
}
