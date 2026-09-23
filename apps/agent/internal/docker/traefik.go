package docker

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

var (
	// hostSanitizeRE strips non-alphanumeric chars for safe router naming
	hostSanitizeRE = regexp.MustCompile(`[^a-z0-9\-]+`)
)

// GenerateTraefikLabels produces Traefik v2/v3 reverse-proxy labels from a typed TraefikConfig.
// This is the single trusted source of Traefik label generation — callers cannot inject raw labels.
func GenerateTraefikLabels(tc *TraefikConfig) map[string]string {
	if tc == nil || !tc.Enabled {
		return nil
	}

	svc := tc.ServiceName
	labels := make(map[string]string)

	// 1. Core Enable & Load Balancer Port
	labels["traefik.enable"] = "true"
	labels[fmt.Sprintf("traefik.http.services.%s.loadbalancer.server.port", svc)] = fmt.Sprintf("%d", tc.Port)

	// 2. Explicit Network Control
	// Ensures Traefik routes to containers via the dedicated proxy network (deploycore-proxy)
	proxyNet := tc.TraefikNetwork
	if proxyNet == "" {
		proxyNet = protocol.ProxyNetworkName
	}
	labels["traefik.docker.network"] = proxyNet

	// 3. WebSocket-Compatible Routing
	if tc.EnableWebSocket {
		labels[fmt.Sprintf("traefik.http.services.%s.loadbalancer.passhostheader", svc)] = "true"
		labels[fmt.Sprintf("traefik.http.services.%s.loadbalancer.responseforwarding.flushinterval", svc)] = "100ms"
	}

	// 4. Normalize domains list
	domains := tc.Domains
	if len(domains) == 0 && tc.Host != "" {
		domains = []DomainConfig{
			{
				Hostname:   tc.Host,
				IsPrimary:  true,
				ForceHTTPS: tc.TLS,
				PathPrefix: tc.PathPrefix,
			},
		}
	}

	if len(domains) == 0 {
		return labels
	}

	// 5. Identify Primary Domain
	var primaryDomain DomainConfig
	primaryFound := false
	for _, d := range domains {
		if d.IsPrimary {
			primaryDomain = d
			primaryFound = true
			break
		}
	}
	if !primaryFound {
		primaryDomain = domains[0]
		primaryDomain.IsPrimary = true
	}

	// 6. Generate Routers and Middlewares for each domain
	for i, d := range domains {
		cleanHost := sanitizeHost(d.Hostname)
		if cleanHost == "" {
			continue
		}

		isPrimary := d.IsPrimary || (!primaryFound && i == 0)
		routerBase := svc
		if len(domains) > 1 {
			routerBase = fmt.Sprintf("%s-%s", svc, sanitizeRouterSlug(cleanHost))
		}

		// Host rule with optional path prefix (strictly escaped)
		rule := fmt.Sprintf("Host(`%s`)", cleanHost)
		if d.PathPrefix != "" {
			rule = fmt.Sprintf("Host(`%s`) && PathPrefix(`%s`)", cleanHost, d.PathPrefix)
		}

		// Handle Redirect-To-Primary Policy for secondary domains
		if d.RedirectToPrimary && !isPrimary && primaryDomain.Hostname != "" {
			redirRouter := fmt.Sprintf("%s-to-primary", routerBase)
			redirMW := fmt.Sprintf("%s-redir", redirRouter)

			labels[fmt.Sprintf("traefik.http.routers.%s.rule", redirRouter)] = rule
			labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", redirRouter)] = "web,websecure"
			labels[fmt.Sprintf("traefik.http.routers.%s.middlewares", redirRouter)] = redirMW
			labels[fmt.Sprintf("traefik.http.routers.%s.service", redirRouter)] = svc

			// Redirect regex middleware: rewrite host to primary hostname
			primaryClean := sanitizeHost(primaryDomain.Hostname)
			labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.regex", redirMW)] = fmt.Sprintf("^https?://%s/(.*)", regexp.QuoteMeta(cleanHost))
			labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.replacement", redirMW)] = fmt.Sprintf("https://%s/${1}", primaryClean)
			labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectregex.permanent", redirMW)] = "true"
			continue
		}

		// Combine domain-specific and service-level middlewares (preserving order, deduplicated)
		var allMiddlewares []string
		seenMW := make(map[string]struct{})
		for _, mw := range append(append([]string{}, tc.Middlewares...), d.Middlewares...) {
			mw = strings.TrimSpace(mw)
			if mw == "" {
				continue
			}
			if _, exists := seenMW[mw]; !exists {
				seenMW[mw] = struct{}{}
				allMiddlewares = append(allMiddlewares, mw)
			}
		}

		// HTTPS / TLS handling
		forceHTTPS := d.ForceHTTPS || tc.TLS
		if forceHTTPS {
			certResolver := d.CertResolver
			if certResolver == "" {
				certResolver = "letsencrypt"
			}

			// Secure Router on websecure (443)
			labels[fmt.Sprintf("traefik.http.routers.%s.rule", routerBase)] = rule
			labels[fmt.Sprintf("traefik.http.routers.%s.service", routerBase)] = svc
			labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", routerBase)] = "websecure"
			labels[fmt.Sprintf("traefik.http.routers.%s.tls", routerBase)] = "true"
			labels[fmt.Sprintf("traefik.http.routers.%s.tls.certresolver", routerBase)] = certResolver

			if len(allMiddlewares) > 0 {
				labels[fmt.Sprintf("traefik.http.routers.%s.middlewares", routerBase)] = strings.Join(allMiddlewares, ",")
			}

			// Plain HTTP Router on web (80) with HTTP → HTTPS redirect via
			// docker-provider middleware labels (no external @file dependency).
			httpRouter := fmt.Sprintf("%s-http", routerBase)
			httpRedirectMW := fmt.Sprintf("%s-https-redirect", routerBase)

			labels[fmt.Sprintf("traefik.http.routers.%s.rule", httpRouter)] = rule
			labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", httpRouter)] = "web"
			labels[fmt.Sprintf("traefik.http.routers.%s.middlewares", httpRouter)] = httpRedirectMW
			labels[fmt.Sprintf("traefik.http.routers.%s.service", httpRouter)] = svc
			labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectscheme.scheme", httpRedirectMW)] = "https"
			labels[fmt.Sprintf("traefik.http.middlewares.%s.redirectscheme.permanent", httpRedirectMW)] = "true"
		} else {
			// Plain HTTP only router on web (80)
			labels[fmt.Sprintf("traefik.http.routers.%s.rule", routerBase)] = rule
			labels[fmt.Sprintf("traefik.http.routers.%s.service", routerBase)] = svc
			labels[fmt.Sprintf("traefik.http.routers.%s.entrypoints", routerBase)] = "web"

			if len(allMiddlewares) > 0 {
				labels[fmt.Sprintf("traefik.http.routers.%s.middlewares", routerBase)] = strings.Join(allMiddlewares, ",")
			}
		}

		if isPrimary {
			labels["deploycore.domain.primary"] = "true"
			labels["deploycore.domain.hostname"] = cleanHost
		}
	}

	return labels
}

// sanitizeHost strips backticks, spaces, and normalizes hostname to lowercase.
func sanitizeHost(host string) string {
	clean := strings.ToLower(strings.TrimSpace(host))
	return strings.ReplaceAll(clean, "`", "")
}

// sanitizeRouterSlug converts a hostname into a DNS-safe router slug.
func sanitizeRouterSlug(host string) string {
	slug := strings.ReplaceAll(host, ".", "-")
	slug = hostSanitizeRE.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 32 {
		slug = slug[:32]
	}
	return slug
}

// mergeLabels merges multiple label maps in order of increasing precedence.
// Later maps override earlier maps.
func mergeLabels(maps ...map[string]string) map[string]string {
	total := 0
	for _, m := range maps {
		total += len(m)
	}
	out := make(map[string]string, total)
	for _, m := range maps {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
}
