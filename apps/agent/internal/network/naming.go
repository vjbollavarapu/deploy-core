package network

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

var (
	// ErrInvalidNetworkName indicates the network name violates platform standards.
	ErrInvalidNetworkName = errors.New("invalid platform network name")
	// ErrInvalidSlug indicates a project, environment, or scope slug is invalid.
	ErrInvalidSlug = errors.New("invalid slug identifier (must be lowercase alphanumeric + hyphens)")
)

// Regex patterns for network naming
var (
	slugRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]{0,38}[a-z0-9])?$`)

	// networkNameRE parses dc-<project>-<environment>-<scope>
	networkNameRE = regexp.MustCompile(`^` + protocol.ContainerPrefix + `-([a-z0-9](?:[a-z0-9\-]*[a-z0-9])?)-([a-z0-9](?:[a-z0-9\-]*[a-z0-9])?)-([a-z0-9](?:[a-z0-9\-]*[a-z0-9])?)$`)
)

// FormatPrivateNetworkName formats standard private application network names:
// Example: dc-<project>-<environment>-private
func FormatPrivateNetworkName(projectSlug, envSlug string) (string, error) {
	return FormatNetworkName(projectSlug, envSlug, protocol.NetworkTypePrivate)
}

// FormatNetworkName builds a scoped platform network name.
// Standard: dc-<project>-<environment>-<scope>
func FormatNetworkName(projectSlug, envSlug, scope string) (string, error) {
	proj := strings.TrimSpace(projectSlug)
	if proj == "" || !slugRE.MatchString(proj) {
		return "", fmt.Errorf("%w: project %q", ErrInvalidSlug, projectSlug)
	}

	env := strings.TrimSpace(envSlug)
	if env == "" || !slugRE.MatchString(env) {
		return "", fmt.Errorf("%w: environment %q", ErrInvalidSlug, envSlug)
	}

	s := strings.TrimSpace(scope)
	if s == "" || !slugRE.MatchString(s) {
		return "", fmt.Errorf("%w: scope %q", ErrInvalidSlug, scope)
	}

	name := fmt.Sprintf("%s-%s-%s-%s", protocol.ContainerPrefix, proj, env, s)
	if len(name) > 63 {
		return "", fmt.Errorf("%w: name %q exceeds 63 characters", ErrInvalidNetworkName, name)
	}

	return name, nil
}

// ParseNetworkName extracts project, environment, and scope from a platform network name.
func ParseNetworkName(name string) (project, env, scope string, ok bool) {
	trimmed := strings.TrimSpace(name)
	matches := networkNameRE.FindStringSubmatch(trimmed)
	if len(matches) != 4 {
		return "", "", "", false
	}
	return matches[1], matches[2], matches[3], true
}

// IsPlatformNetworkName returns true if the network name matches the platform standard
// or is the dedicated proxy network (deploycore-proxy).
func IsPlatformNetworkName(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == protocol.ProxyNetworkName {
		return true
	}
	return networkNameRE.MatchString(trimmed)
}
