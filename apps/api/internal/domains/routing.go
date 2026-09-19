package domains

import (
	"fmt"
	"regexp"
	"strings"
)

var hostnameRE = regexp.MustCompile(`(?i)^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}$|^localhost$|^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?)$`)

func NormalizeHostname(raw string) (string, error) {
	h := strings.ToLower(strings.TrimSpace(raw))
	h = strings.TrimSuffix(h, ".")
	if h == "" {
		return "", fmt.Errorf("hostname is required")
	}
	if len(h) > 253 {
		return "", fmt.Errorf("hostname too long")
	}
	if !hostnameRE.MatchString(h) {
		return "", fmt.Errorf("invalid hostname")
	}
	return h, nil
}

// BuildRoutingConfig generates desired Traefik labels for a domain assignment.
func BuildRoutingConfig(appSlug string, d Domain) RoutingConfig {
	safeApp := sanitizeLabel(appSlug)
	safeHost := sanitizeLabel(d.Hostname)
	router := fmt.Sprintf("%s-%s", safeApp, safeHost)
	service := safeApp

	labels := map[string]string{
		"traefik.enable": "true",
		fmt.Sprintf("traefik.http.routers.%s.rule", router):                      fmt.Sprintf("Host(`%s`)", d.Hostname),
		fmt.Sprintf("traefik.http.routers.%s.service", router):                   service,
		fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", service): fmt.Sprintf("%d", d.InternalPort),
	}

	if d.ForceHTTPS {
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", router)] = "websecure"
		labels[fmt.Sprintf("traefik.http.routers.%s.tls", router)] = "true"
		labels[fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", router)] = "letsencrypt"

		redir := router + "-http"
		labels[fmt.Sprintf("traefik.http.routers.%s.rule", redir)] = fmt.Sprintf("Host(`%s`)", d.Hostname)
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", redir)] = "web"
		labels[fmt.Sprintf("traefik.http.routers.%s.middlewares", redir)] = redir + "-redirect"
		labels[fmt.Sprintf("traefik.http.middlewares.%s-redirect.redirectscheme.scheme", redir)] = "https"
		labels[fmt.Sprintf("traefik.http.middlewares.%s-redirect.redirectscheme.permanent", redir)] = "true"
	} else {
		labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", router)] = "web"
	}

	if d.IsPrimary {
		labels["deploycore.domain.primary"] = "true"
	}
	labels["deploycore.domain.hostname"] = d.Hostname
	labels["deploycore.domain.id"] = d.ID.String()
	labels["deploycore.application.id"] = d.ApplicationID.String()
	labels["deploycore.environment.id"] = d.EnvironmentID.String()
	// Shared Traefik service name (= app slug) lets multiple replica containers
	// register as load-balanced backends when they carry the same labels.
	labels["deploycore.routing.mode"] = "replicas"

	return RoutingConfig{Provider: "traefik", Labels: labels}
}

// BuildReplicaRoutingLabels returns per-replica Traefik labels that join the
// application-level service for load balancing across healthy replicas.
func BuildReplicaRoutingLabels(appSlug string, d Domain, replicaIndex int) map[string]string {
	base := BuildRoutingConfig(appSlug, d).Labels
	base["deploycore.replica.index"] = fmt.Sprintf("%d", replicaIndex)
	base["deploycore.replica.role"] = "member"
	return base
}

func sanitizeLabel(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_' || r == '.':
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "app"
	}
	if len(out) > 48 {
		out = out[:48]
	}
	return out
}
