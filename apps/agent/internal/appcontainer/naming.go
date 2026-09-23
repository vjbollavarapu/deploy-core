package appcontainer

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

var (
	// ErrInvalidContainerName indicates the container name does not follow platform standards.
	ErrInvalidContainerName = errors.New("invalid platform container name")
	// ErrInvalidAppShortID indicates the application slug/short ID is invalid.
	ErrInvalidAppShortID = errors.New("invalid application short id")
	// ErrInvalidRevision indicates the revision identifier is invalid.
	ErrInvalidRevision = errors.New("invalid revision identifier")
	// ErrInvalidInstance indicates the instance/replica index is invalid.
	ErrInvalidInstance = errors.New("invalid instance index (must be >= 1)")
)

// Regex patterns for platform container naming
var (
	// appSlugRE ensures the short-id is DNS-safe lowercase alphanumeric and hyphens.
	appSlugRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]{0,38}[a-z0-9])?$`)

	// revRE matches valid revision identifiers (numeric or alphanumeric slug/commit hash).
	revRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_\-]{0,31}$`)

	// platformNameRE parses dc-<application-short-id>-r<revision>-<instance>
	// Example: dc-dayaapi-r49-1
	platformNameRE = regexp.MustCompile(`^` + protocol.ContainerPrefix + `-([a-z0-9](?:[a-z0-9\-]*[a-z0-9])?)-r([a-zA-Z0-9_\-]+)-([0-9]+)$`)
)

// ParsedName contains the components extracted from a platform container name.
type ParsedName struct {
	RawName    string
	AppShortID string
	Revision   string
	Instance   int
}

// FormatName builds a deterministic platform container name.
// Standard: dc-<application-short-id>-r<revision>-<instance>
// Example: dc-dayaapi-r49-1
func FormatName(appShortID string, revision string, instance int) (string, error) {
	slug := strings.TrimSpace(appShortID)
	if slug == "" || !appSlugRE.MatchString(slug) {
		return "", fmt.Errorf("%w: %q (must be lowercase alphanumeric + hyphens, 1-40 chars)", ErrInvalidAppShortID, appShortID)
	}

	cleanRev := strings.TrimSpace(revision)
	// Strip optional 'rev-', 'rev', or 'r' prefixes if caller provided one
	lowerRev := strings.ToLower(cleanRev)
	if strings.HasPrefix(lowerRev, "rev-") && len(cleanRev) > 4 {
		cleanRev = cleanRev[4:]
	} else if strings.HasPrefix(lowerRev, "rev") && len(cleanRev) > 3 {
		cleanRev = cleanRev[3:]
	} else if (strings.HasPrefix(cleanRev, "r") || strings.HasPrefix(cleanRev, "R")) && len(cleanRev) > 1 {
		cleanRev = cleanRev[1:]
	}

	if cleanRev == "" || !revRE.MatchString(cleanRev) {
		return "", fmt.Errorf("%w: %q (must be alphanumeric, 1-32 chars)", ErrInvalidRevision, revision)
	}

	if instance < 1 {
		return "", fmt.Errorf("%w: got %d", ErrInvalidInstance, instance)
	}

	name := fmt.Sprintf("%s-%s-r%s-%d", protocol.ContainerPrefix, slug, cleanRev, instance)

	// Docker container names max length is 63 chars (DNS label standard)
	if len(name) > 63 {
		return "", fmt.Errorf("%w: generated name %q exceeds 63 characters", ErrInvalidContainerName, name)
	}

	return name, nil
}

// ParseName extracts components from a container name.
// Accepts optional leading slash as returned by Docker inspect/list (e.g. "/dc-dayaapi-r49-1").
func ParseName(name string) (ParsedName, error) {
	trimmed := strings.TrimPrefix(strings.TrimSpace(name), "/")
	matches := platformNameRE.FindStringSubmatch(trimmed)
	if len(matches) != 4 {
		return ParsedName{}, fmt.Errorf("%w: %q (expected format dc-<app>-r<rev>-<instance>)", ErrInvalidContainerName, name)
	}

	instance, err := strconv.Atoi(matches[3])
	if err != nil || instance < 1 {
		return ParsedName{}, fmt.Errorf("%w: invalid instance %q in name %q", ErrInvalidInstance, matches[3], name)
	}

	return ParsedName{
		RawName:    trimmed,
		AppShortID: matches[1],
		Revision:   matches[2],
		Instance:   instance,
	}, nil
}

// ValidateName checks whether a container name strictly conforms to the platform naming standard.
func ValidateName(name string) error {
	_, err := ParseName(name)
	return err
}

// IsPlatformName checks if the name matches the naming standard pattern without returning errors.
func IsPlatformName(name string) bool {
	return ValidateName(name) == nil
}
