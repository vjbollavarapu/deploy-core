package network

import (
	"strings"
	"testing"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

func TestFormatPrivateNetworkName_StandardCases(t *testing.T) {
	tests := []struct {
		name        string
		projectSlug string
		envSlug     string
		expected    string
	}{
		{
			name:        "prompt example dc-daya-prod-private",
			projectSlug: "daya",
			envSlug:     "prod",
			expected:    "dc-daya-prod-private",
		},
		{
			name:        "hyphenated slugs",
			projectSlug: "my-service",
			envSlug:     "staging-1",
			expected:    "dc-my-service-staging-1-private",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FormatPrivateNetworkName(tc.projectSlug, tc.envSlug)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestFormatNetworkName_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		project string
		env     string
		scope   string
	}{
		{"empty project", "", "prod", "private"},
		{"empty env", "proj", "", "private"},
		{"empty scope", "proj", "prod", ""},
		{"uppercase project", "MyProject", "prod", "private"},
		{"traversal in slug", "../proj", "prod", "private"},
		{"spaces in slug", "my proj", "prod", "private"},
		{"length exceeding 63 chars", strings.Repeat("a", 35), strings.Repeat("b", 35), "private"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FormatNetworkName(tc.project, tc.env, tc.scope)
			if err == nil {
				t.Errorf("expected validation error for test %q, got nil", tc.name)
			}
		})
	}
}

func TestParseNetworkName(t *testing.T) {
	tests := []struct {
		input      string
		expectedP  string
		expectedE  string
		expectedS  string
		expectedOk bool
	}{
		{"dc-daya-prod-private", "daya", "prod", "private", true},
		{"dc-auth-service-staging-isolated", "auth-service", "staging", "isolated", true},
		{"deploycore-proxy", "", "", "", false}, // proxy has distinct dedicated format
		{"unmanaged-bridge", "", "", "", false},
		{"dc--prod-private", "", "", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			p, e, s, ok := ParseNetworkName(tc.input)
			if ok != tc.expectedOk {
				t.Fatalf("expected ok=%v, got %v", tc.expectedOk, ok)
			}
			if ok {
				if p != tc.expectedP || e != tc.expectedE || s != tc.expectedS {
					t.Errorf("expected (%s, %s, %s), got (%s, %s, %s)", tc.expectedP, tc.expectedE, tc.expectedS, p, e, s)
				}
			}
		})
	}
}

func TestIsPlatformNetworkName(t *testing.T) {
	if !IsPlatformNetworkName(protocol.ProxyNetworkName) {
		t.Errorf("expected deploycore-proxy to be recognized as platform network")
	}
	if !IsPlatformNetworkName("dc-daya-prod-private") {
		t.Errorf("expected dc-daya-prod-private to be recognized as platform network")
	}
	if IsPlatformNetworkName("bridge") {
		t.Errorf("expected default bridge to NOT be recognized as platform network")
	}
	if IsPlatformNetworkName("host") {
		t.Errorf("expected host to NOT be recognized as platform network")
	}
}
