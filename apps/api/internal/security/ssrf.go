package security

import (
	"fmt"
	"net"
	"net/url"
	"strings"
	"sync/atomic"
)

// AllowPrivateOutboundHosts skips private/loopback destination checks when true.
// Intended for integration tests that deliver to httptest on localhost.
var AllowPrivateOutboundHosts atomic.Bool

// lookupIP resolves hostnames; overridden in unit tests.
var lookupIP = net.LookupIP

// ValidateOutboundURL rejects non-http(s) schemes and private/metadata destinations (SSRF).
func ValidateOutboundURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "https" && scheme != "http" {
		return fmt.Errorf("url must be http(s)")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("url host is required")
	}
	if AllowPrivateOutboundHosts.Load() {
		return nil
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return fmt.Errorf("url host is not allowed")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return fmt.Errorf("url host is not allowed")
		}
		return nil
	}
	// Hostname: resolve and reject if any answer is private/metadata.
	addrs, err := lookupIP(host)
	if err != nil {
		return fmt.Errorf("url host could not be resolved")
	}
	if len(addrs) == 0 {
		return fmt.Errorf("url host could not be resolved")
	}
	for _, ip := range addrs {
		if isBlockedIP(ip) {
			return fmt.Errorf("url host resolves to a private or metadata address")
		}
	}
	return nil
}

func isBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	// AWS/GCP/Azure metadata and similar.
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		// Carrier-grade NAT 100.64.0.0/10
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
	}
	return false
}
