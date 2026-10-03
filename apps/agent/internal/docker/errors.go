package docker

import (
	"context"
	"errors"
	"strings"

	dockererr "github.com/docker/docker/errdefs"
)

// mapDockerError converts raw Docker SDK errors into stable AgentError codes.
// This prevents raw daemon responses from leaking to callers.
func mapDockerError(err error) error {
	if err == nil {
		return nil
	}

	msg := err.Error()

	switch {
	case dockerfileNotFoundMessage(msg):
		return &AgentError{Code: ErrCodeDockerfileNotFound, Message: msg}
	case dockererr.IsNotFound(err):
		return &AgentError{Code: ErrCodeNotFound, Message: msg}
	case dockererr.IsConflict(err):
		return &AgentError{Code: ErrCodeConflict, Message: msg}
	case dockererr.IsUnauthorized(err) || dockererr.IsForbidden(err):
		return &AgentError{Code: ErrCodePermission, Message: msg}
	case dockererr.IsUnavailable(err) || daemonUnavailableMessage(msg):
		return &AgentError{Code: ErrCodeDaemonUnavailable, Message: msg}
	case dockererr.IsSystem(err):
		return &AgentError{Code: ErrCodeUnknown, Message: msg}
	case errors.Is(err, context.DeadlineExceeded) || strings.Contains(msg, "context deadline exceeded"):
		return &AgentError{Code: ErrCodeTimeout, Message: msg}
	default:
		return &AgentError{Code: ErrCodeUnknown, Message: msg}
	}
}

func dockerfileNotFoundMessage(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "cannot locate specified dockerfile") ||
		strings.Contains(lower, "failed to read dockerfile") ||
		strings.Contains(lower, "dockerfile not found")
}

func daemonUnavailableMessage(msg string) bool {
	lower := strings.ToLower(msg)
	return strings.Contains(lower, "cannot connect to the docker daemon") ||
		strings.Contains(lower, "is the docker daemon running") ||
		strings.Contains(lower, "connection refused")
}
