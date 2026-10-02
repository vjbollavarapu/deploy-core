package protocol

import (
	"fmt"
	"regexp"
	"strings"
)

// applicationSlugSafeRE matches the Agent deployment slug sanitizer:
// lowercase letters, digits, and hyphens are kept; everything else is removed.
var applicationSlugSafeRE = regexp.MustCompile(`[^a-z0-9\-]+`)

// SanitizeApplicationSlug applies the platform container-name slug rules.
// Empty input becomes "app". The result is truncated to 30 characters.
func SanitizeApplicationSlug(slug string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	slug = applicationSlugSafeRE.ReplaceAllString(slug, "")
	if len(slug) > 30 {
		slug = slug[:30]
	}
	if slug == "" {
		return "app"
	}
	return slug
}

// PlatformContainerName is the Docker name for one replica of one revision.
// replicaIndex is 0-based. The instance token is replicaIndex+1.
// revisionNumber is the durable numeric revision, never a revision UUID.
func PlatformContainerName(slug string, revisionNumber, replicaIndex int) (string, error) {
	if revisionNumber < 1 {
		return "", fmt.Errorf("revision number must be a positive integer")
	}
	if replicaIndex < 0 {
		return "", fmt.Errorf("replica index must be >= 0")
	}
	safe := SanitizeApplicationSlug(slug)
	instance := replicaIndex + 1
	return fmt.Sprintf("%s-%s-r%d-%d", ContainerPrefix, safe, revisionNumber, instance), nil
}
