package security

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"unicode"
)

var containerPathRE = regexp.MustCompile(`^(/[A-Za-z0-9._-]+)+$`)

// ValidateContainerMountPath requires an absolute container path without ".." segments.
func ValidateContainerMountPath(raw string) error {
	p := strings.TrimSpace(raw)
	if p == "" {
		return fmt.Errorf("mountPath is required")
	}
	if len(p) > 512 {
		return fmt.Errorf("mountPath too long")
	}
	if !strings.HasPrefix(p, "/") {
		return fmt.Errorf("mountPath must be absolute")
	}
	if strings.Contains(p, "..") || path.Clean(p) != p {
		return fmt.Errorf("mountPath must not contain relative segments")
	}
	if !containerPathRE.MatchString(p) {
		return fmt.Errorf("mountPath contains invalid characters")
	}
	return nil
}

// SanitizeLogField strips control characters and truncates for safe structured logs.
func SanitizeLogField(s string, maxLen int) string {
	if maxLen <= 0 {
		maxLen = 256
	}
	var b strings.Builder
	b.Grow(min(len(s), maxLen))
	for _, r := range s {
		if r == '\r' || r == '\n' || r == '\t' {
			b.WriteByte(' ')
			continue
		}
		if unicode.IsControl(r) {
			continue
		}
		b.WriteRune(r)
		if b.Len() >= maxLen {
			break
		}
	}
	return b.String()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
