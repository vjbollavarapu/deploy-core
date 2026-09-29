package executor

import (
	"context"
	"strings"

	"github.com/deploycore/deploy-core/apps/agent/internal/transport"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

// applyRevisionRuntime loads the revision snapshot and merges it into container
// env. Returned values are plaintext and must only be used to sanitize errors.
// They must not be logged or copied into ExecutionResult.
func applyRevisionRuntime(ctx context.Context, tr transport.Client, revisionID string, base []string) (env []string, sensitive []string, err error) {
	revisionID = strings.TrimSpace(revisionID)
	if _, parseErr := uuid.Parse(revisionID); parseErr != nil {
		return append([]string(nil), base...), nil, nil
	}
	if tr == nil {
		return nil, nil, Errorf(ErrCodeInternalError, "transport required to load revision runtime")
	}
	boot, err := tr.FetchRevisionRuntime(ctx, revisionID)
	if err != nil {
		return nil, nil, Errorf(ErrCodeInternalError, "revision runtime bootstrap failed")
	}
	merged, values := mergeRuntimeEnv(base, boot.Env)
	return merged, values, nil
}

func mergeRuntimeEnv(base []string, extra []protocol.RuntimeVariable) ([]string, []string) {
	order := make([]string, 0, len(base)+len(extra))
	vals := make(map[string]string, len(base)+len(extra))
	add := func(key, value string) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		if _, ok := vals[key]; !ok {
			order = append(order, key)
		}
		vals[key] = value
	}
	for _, entry := range base {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		add(key, value)
	}
	sensitive := make([]string, 0, len(extra))
	for _, item := range extra {
		add(item.Name, item.Value)
		if item.Value != "" {
			sensitive = append(sensitive, item.Value)
		}
	}
	out := make([]string, 0, len(order))
	for _, key := range order {
		out = append(out, key+"="+vals[key])
	}
	return out, sensitive
}

func sanitizeRuntimeErr(err error, sensitive []string) error {
	if err == nil {
		return nil
	}
	msg := SanitizeMessage(err.Error(), sensitive...)
	if exec, ok := err.(*ExecutionError); ok {
		return &ExecutionError{Code: exec.Code, Message: msg}
	}
	return Errorf(ErrCodeDockerError, "%s", msg)
}
