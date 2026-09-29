package protocol

import (
	"fmt"
	"regexp"
	"strings"
)

// Standard platform network names
const (
	// ProxyNetworkName is the dedicated reverse-proxy Docker network (Traefik ingress).
	ProxyNetworkName = "deploycore-proxy"
)

// platformSlugRE matches a single DNS label used in platform network names.
// Length is capped at 40 so dc-<project>-<environment>-private stays within 63 characters.
var platformSlugRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)

// dnsAliasRE matches one DNS label. Application slugs used as dnsAlias must match.
var dnsAliasRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// FormatPrivateNetworkName returns the canonical project/environment network:
// dc-<projectSlug>-<environmentSlug>-private.
func FormatPrivateNetworkName(projectSlug, envSlug string) (string, error) {
	proj := strings.TrimSpace(projectSlug)
	env := strings.TrimSpace(envSlug)
	if proj == "" || !platformSlugRE.MatchString(proj) {
		return "", fmt.Errorf("invalid project slug %q", projectSlug)
	}
	if env == "" || !platformSlugRE.MatchString(env) {
		return "", fmt.Errorf("invalid environment slug %q", envSlug)
	}
	name := fmt.Sprintf("%s-%s-%s-%s", ContainerPrefix, proj, env, NetworkTypePrivate)
	if len(name) > 63 {
		return "", fmt.Errorf("private network name %q exceeds 63 characters", name)
	}
	return name, nil
}

// ValidDNSAlias reports whether alias is a single DNS label.
// It is the application slug attached only to the private network.
func ValidDNSAlias(alias string) bool {
	alias = strings.TrimSpace(alias)
	if alias == "" || strings.Contains(alias, ".") {
		return false
	}
	return dnsAliasRE.MatchString(alias)
}

// Network Label Keys
const (
	LabelNetworkType     = "deploycore.network.type"
	LabelNetworkName     = "deploycore.network.name"
	LabelProjectID       = "deploycore.project_id"
	LabelProjectSlug     = "deploycore.project_slug"
	LabelEnvironmentSlug = "deploycore.environment_slug"
)

// Network Types
const (
	NetworkTypePrivate  = "private"
	NetworkTypeProxy    = "proxy"
	NetworkTypeIsolated = "isolated"
)
