package databases

import (
	"errors"
	"strings"
	"unicode"

	"github.com/deploycore/deploy-core/apps/api/internal/projects"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

const postgresPort = 5432

// DNSAlias builds the immutable private hostname for a managed database.
// The slug uses the same sanitizer as project and environment slugs, prefixed
// so it does not occupy an application slug on the environment network.
func DNSAlias(displayName string) (string, error) {
	if !containsLetterOrDigit(displayName) {
		return "", errors.New("database name cannot form a DNS alias")
	}
	slug := projects.Slugify(displayName)
	if len(slug) > 60 {
		slug = strings.Trim(slug[:60], "-")
	}
	alias := "db-" + slug
	if !protocol.ValidDNSAlias(alias) {
		return "", errors.New("database name cannot form a DNS alias")
	}
	return alias, nil
}

func containsLetterOrDigit(s string) bool {
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
