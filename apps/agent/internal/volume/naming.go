package volume

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

var (
	// ErrInvalidVolumeName indicates the volume name is invalid or violates platform constraints.
	ErrInvalidVolumeName = errors.New("invalid platform volume name")
	// ErrInvalidSlug indicates a scope or component slug is invalid.
	ErrInvalidSlug = errors.New("invalid volume slug (must be lowercase alphanumeric + hyphens)")
)

// Regex patterns for volume naming
var (
	slugRE       = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]{0,38}[a-z0-9])?$`)
	volumeNameRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_\-\.]{0,63}$`)
)

// FormatVolumeName formats a platform-scoped volume name.
// Standard: dc-<scope>-<name>
func FormatVolumeName(scopeSlug, name string) (string, error) {
	scope := strings.TrimSpace(scopeSlug)
	if scope == "" || !slugRE.MatchString(scope) {
		return "", fmt.Errorf("%w: scope %q", ErrInvalidSlug, scopeSlug)
	}

	n := strings.TrimSpace(name)
	if n == "" || !slugRE.MatchString(n) {
		return "", fmt.Errorf("%w: name %q", ErrInvalidSlug, name)
	}

	formatted := fmt.Sprintf("%s-%s-%s", protocol.ContainerPrefix, scope, n)
	if len(formatted) > 64 {
		return "", fmt.Errorf("%w: %q exceeds 64 characters", ErrInvalidVolumeName, formatted)
	}
	return formatted, nil
}

// ValidateVolumeName checks if a volume name meets Docker and platform naming criteria.
func ValidateVolumeName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || !volumeNameRE.MatchString(trimmed) {
		return fmt.Errorf("%w: %q", ErrInvalidVolumeName, name)
	}
	return nil
}

// IsPlatformVolumeName returns true if the volume name follows the platform prefix standard (dc-*).
func IsPlatformVolumeName(name string) bool {
	trimmed := strings.TrimSpace(name)
	return strings.HasPrefix(trimmed, protocol.ContainerPrefix+"-")
}
