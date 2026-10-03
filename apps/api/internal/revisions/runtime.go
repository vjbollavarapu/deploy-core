package revisions

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/deploycore/deploy-core/apps/api/internal/agents"
	"github.com/deploycore/deploy-core/apps/api/internal/secrets"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/google/uuid"
)

// RuntimeVariable is one container environment entry. Value is plaintext and
// must not be written to logs, events, audit records, or command payloads.
type RuntimeVariable struct {
	Name  string
	Value string
}

// RuntimeConfig is the agent-only view of a revision snapshot.
type RuntimeConfig struct {
	RevisionID uuid.UUID
	Env        []RuntimeVariable
}

// BootstrapForAgent returns snapshotted variables and the exact secret versions
// referenced by the revision. It does not substitute a newer secret version.
func (s *Service) BootstrapForAgent(ctx context.Context, agent agents.Agent, revisionID uuid.UUID) (RuntimeConfig, error) {
	rc, err := s.repo.GetRuntimeContext(ctx, revisionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return RuntimeConfig{}, apierror.NotFoundCode(apierror.CodeRevisionNotFound, "revision not found")
		}
		return RuntimeConfig{}, err
	}
	if rc.OrganizationID != agent.OrganizationID {
		return RuntimeConfig{}, apierror.Forbidden("revision is outside agent scope")
	}
	if rc.ServerID == nil || *rc.ServerID != agent.ServerID {
		return RuntimeConfig{}, apierror.Forbidden("revision is outside agent scope")
	}

	env, err := variablesFromSnapshot(rc.VariableSnapshot)
	if err != nil {
		return RuntimeConfig{}, apierror.Validation(err.Error(), nil)
	}
	secretsEnv, err := s.secretsFromSnapshot(ctx, rc)
	if err != nil {
		return RuntimeConfig{}, err
	}
	// Secret values override a variable of the same name. The snapshot is authoritative.
	env = overlayEnv(env, secretsEnv)
	return RuntimeConfig{RevisionID: rc.ID, Env: env}, nil
}

// SourceAuth is the agent-only Git HTTPS credential for a revision.
// Scheme none means the revision has no git connection. Password must not be logged.
type SourceAuth struct {
	RevisionID uuid.UUID
	Scheme     string
	Username   string
	Password   string
}

// SourceAuthForAgent resolves the git connection pinned on the revision snapshot.
// It does not accept a client-supplied connection id.
func (s *Service) SourceAuthForAgent(ctx context.Context, agent agents.Agent, revisionID uuid.UUID) (SourceAuth, error) {
	rc, err := s.repo.GetRuntimeContext(ctx, revisionID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return SourceAuth{}, apierror.NotFoundCode(apierror.CodeRevisionNotFound, "revision not found")
		}
		return SourceAuth{}, err
	}
	if rc.OrganizationID != agent.OrganizationID {
		return SourceAuth{}, apierror.Forbidden("revision is outside agent scope")
	}
	if rc.ServerID == nil || *rc.ServerID != agent.ServerID {
		return SourceAuth{}, apierror.Forbidden("revision is outside agent scope")
	}
	raw, _ := rc.EffectiveConfig["gitConnectionId"].(string)
	connectionID := strings.TrimSpace(raw)
	if connectionID == "" {
		return SourceAuth{RevisionID: rc.ID, Scheme: "none"}, nil
	}
	parsed, err := uuid.Parse(connectionID)
	if err != nil {
		return SourceAuth{}, apierror.Validation("revision git connection is invalid", nil)
	}
	if s.gitCreds == nil {
		return SourceAuth{}, apierror.Internal("git source authentication is not configured")
	}
	username, token, err := s.gitCreds.ResolveCloneCredential(ctx, rc.OrganizationID, parsed)
	if err != nil {
		return SourceAuth{}, err
	}
	return SourceAuth{
		RevisionID: rc.ID,
		Scheme:     "basic",
		Username:   username,
		Password:   token,
	}, nil
}

func (s *Service) secretsFromSnapshot(ctx context.Context, rc RuntimeContext) ([]RuntimeVariable, error) {
	if len(rc.SecretRefs) == 0 {
		return nil, nil
	}
	if s.secrets == nil || len(s.platformKey) == 0 {
		return nil, apierror.Internal("revision runtime secrets are not configured")
	}
	out := make([]RuntimeVariable, 0, len(rc.SecretRefs))
	for _, item := range rc.SecretRefs {
		name, scope, version, err := parseSecretRef(item)
		if err != nil {
			return nil, apierror.Validation(err.Error(), nil)
		}
		projectID, environmentID, applicationID := scopeIDs(scope, rc)
		rec, err := s.secrets.GetByVersion(ctx, rc.OrganizationID, scope, name, version, projectID, environmentID, applicationID)
		if err != nil {
			if errors.Is(err, secrets.ErrNotFound) {
				return nil, apierror.NotFoundCode(apierror.CodeSecretNotFound, "snapshotted secret version not found")
			}
			return nil, err
		}
		plain, err := crypto.Open(s.platformKey, crypto.Envelope{
			KeyID:      rec.KeyID,
			Nonce:      rec.Nonce,
			Ciphertext: rec.Ciphertext,
		})
		if err != nil {
			return nil, apierror.Internal("could not decrypt snapshotted secret")
		}
		out = append(out, RuntimeVariable{Name: name, Value: string(plain)})
	}
	return out, nil
}

func scopeIDs(scope string, rc RuntimeContext) (projectID, environmentID, applicationID *uuid.UUID) {
	switch scope {
	case secrets.ScopeProject:
		projectID = &rc.ProjectID
	case secrets.ScopeEnvironment:
		projectID = &rc.ProjectID
		environmentID = &rc.EnvironmentID
	case secrets.ScopeApplication:
		projectID = &rc.ProjectID
		environmentID = &rc.EnvironmentID
		applicationID = &rc.ApplicationID
	}
	return projectID, environmentID, applicationID
}

func variablesFromSnapshot(snapshot map[string]any) ([]RuntimeVariable, error) {
	if len(snapshot) == 0 {
		return nil, nil
	}
	out := make([]RuntimeVariable, 0, len(snapshot))
	for key, raw := range snapshot {
		name := strings.TrimSpace(key)
		if name == "" {
			return nil, fmt.Errorf("variable snapshot contains an empty key")
		}
		value, err := snapshotValue(raw)
		if err != nil {
			return nil, fmt.Errorf("variable %s: %w", name, err)
		}
		out = append(out, RuntimeVariable{Name: name, Value: value})
	}
	return out, nil
}

func snapshotValue(raw any) (string, error) {
	switch v := raw.(type) {
	case string:
		return v, nil
	case map[string]any:
		val, ok := v["value"]
		if !ok {
			return "", fmt.Errorf("missing value")
		}
		s, ok := val.(string)
		if !ok {
			return "", fmt.Errorf("value must be a string")
		}
		return s, nil
	default:
		return "", fmt.Errorf("unsupported snapshot shape")
	}
}

func parseSecretRef(item any) (name, scope string, version int, err error) {
	m, ok := item.(map[string]any)
	if !ok {
		return "", "", 0, fmt.Errorf("secret ref must be an object")
	}
	name, _ = m["name"].(string)
	name = strings.TrimSpace(name)
	ref, ok := m["ref"].(map[string]any)
	if !ok {
		return "", "", 0, fmt.Errorf("secret ref %q is missing ref", name)
	}
	scope, _ = ref["scope"].(string)
	scope = strings.ToUpper(strings.TrimSpace(scope))
	version, err = versionFromJSON(ref["version"])
	if err != nil {
		return "", "", 0, fmt.Errorf("secret ref %q: %w", name, err)
	}
	if name == "" || scope == "" || version < 1 {
		return "", "", 0, fmt.Errorf("secret ref requires name, scope, and version")
	}
	switch scope {
	case secrets.ScopeOrganization, secrets.ScopeProject, secrets.ScopeEnvironment, secrets.ScopeApplication:
	default:
		return "", "", 0, fmt.Errorf("secret ref %q has invalid scope", name)
	}
	return name, scope, version, nil
}

func versionFromJSON(raw any) (int, error) {
	switch v := raw.(type) {
	case float64:
		if v != float64(int(v)) {
			return 0, fmt.Errorf("version must be an integer")
		}
		return int(v), nil
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, fmt.Errorf("version must be an integer")
		}
		return n, nil
	default:
		return 0, fmt.Errorf("version must be an integer")
	}
}

func overlayEnv(base, over []RuntimeVariable) []RuntimeVariable {
	if len(over) == 0 {
		return base
	}
	index := make(map[string]int, len(base))
	out := append([]RuntimeVariable(nil), base...)
	for i, item := range out {
		index[item.Name] = i
	}
	for _, item := range over {
		if i, ok := index[item.Name]; ok {
			out[i] = item
			continue
		}
		index[item.Name] = len(out)
		out = append(out, item)
	}
	return out
}
