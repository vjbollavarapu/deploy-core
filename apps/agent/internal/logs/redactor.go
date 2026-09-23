package logs

import (
	"regexp"
	"sort"
	"strings"
)

// Redactor scrubs sensitive substrings from log messages without broad simplistic replacement.
type Redactor struct {
	values   []string
	mask     string
	patterns []*regexp.Regexp
}

// defaultSecretPatterns catches common credential leakage in build/pull output (R11).
var defaultSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(password|passwd|pwd)\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)(api[_-]?key|access[_-]?key|secret[_-]?key)\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)(authorization|token)\s*[:=]\s*\S+`),
	regexp.MustCompile(`(?i)bearer\s+[a-z0-9\-._~+/]+=*`),
}

// NewRedactor creates a new Redactor based on the provided policy.
func NewRedactor(policy *RedactionPolicy) *Redactor {
	mask := "[REDACTED]"
	var filtered []string
	if policy != nil {
		minLen := policy.MinLength
		if minLen <= 0 {
			minLen = 6
		}
		if policy.Mask != "" {
			mask = policy.Mask
		}
		seen := make(map[string]bool)
		for _, v := range policy.SensitiveValues {
			trimmed := strings.TrimSpace(v)
			if len(trimmed) >= minLen && !seen[trimmed] {
				filtered = append(filtered, trimmed)
				seen[trimmed] = true
			}
		}
		sort.Slice(filtered, func(i, j int) bool {
			return len(filtered[i]) > len(filtered[j])
		})
	}

	return &Redactor{
		values:   filtered,
		mask:     mask,
		patterns: defaultSecretPatterns,
	}
}

// Redact replaces configured sensitive values and common secret patterns.
func (r *Redactor) Redact(text string) string {
	if text == "" {
		return text
	}

	res := text
	for _, val := range r.values {
		res = strings.ReplaceAll(res, val, r.mask)
	}
	for _, pat := range r.patterns {
		res = pat.ReplaceAllString(res, r.mask)
	}
	return res
}
