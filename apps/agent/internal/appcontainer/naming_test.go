package appcontainer

import (
	"strings"
	"testing"
)

func TestFormatName_StandardCases(t *testing.T) {
	tests := []struct {
		name       string
		appShortID string
		revision   string
		instance   int
		expected   string
	}{
		{
			name:       "prompt example dc-dayaapi-r49-1",
			appShortID: "dayaapi",
			revision:   "49",
			instance:   1,
			expected:   "dc-dayaapi-r49-1",
		},
		{
			name:       "already prefixed with r",
			appShortID: "dayaapi",
			revision:   "r49",
			instance:   1,
			expected:   "dc-dayaapi-r49-1",
		},
		{
			name:       "hyphenated slug",
			appShortID: "web-frontend",
			revision:   "r12",
			instance:   2,
			expected:   "dc-web-frontend-r12-2",
		},
		{
			name:       "alphanumeric commit hash revision",
			appShortID: "api",
			revision:   "a1b2c3d",
			instance:   1,
			expected:   "dc-api-ra1b2c3d-1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FormatName(tc.appShortID, tc.revision, tc.instance)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestFormatName_ValidationErrors(t *testing.T) {
	tests := []struct {
		name       string
		appShortID string
		revision   string
		instance   int
	}{
		{"empty slug", "", "49", 1},
		{"uppercase slug", "DayaApi", "49", 1},
		{"traversal slug", "../escape", "49", 1},
		{"space in slug", "daya api", "49", 1},
		{"empty revision", "api", "", 1},
		{"invalid chars in revision", "api", "49; rm -rf", 1},
		{"zero instance", "api", "49", 0},
		{"negative instance", "api", "49", -1},
		{
			"slug too long exceeding 63 chars",
			strings.Repeat("a", 55),
			"r100",
			1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := FormatName(tc.appShortID, tc.revision, tc.instance)
			if err == nil {
				t.Errorf("expected validation error for test case %q, got nil", tc.name)
			}
		})
	}
}

func TestParseName_Success(t *testing.T) {
	tests := []struct {
		input      string
		appShortID string
		revision   string
		instance   int
	}{
		{"dc-dayaapi-r49-1", "dayaapi", "49", 1},
		{"/dc-dayaapi-r49-1", "dayaapi", "49", 1}, // docker inspect format
		{"dc-web-service-r102-3", "web-service", "102", 3},
		{"dc-api-rcommit123-1", "api", "commit123", 1},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			parsed, err := ParseName(tc.input)
			if err != nil {
				t.Fatalf("unexpected error parsing %q: %v", tc.input, err)
			}
			if parsed.AppShortID != tc.appShortID {
				t.Errorf("appShortID: expected %q, got %q", tc.appShortID, parsed.AppShortID)
			}
			if parsed.Revision != tc.revision {
				t.Errorf("revision: expected %q, got %q", tc.revision, parsed.Revision)
			}
			if parsed.Instance != tc.instance {
				t.Errorf("instance: expected %d, got %d", tc.instance, parsed.Instance)
			}
		})
	}
}

func TestParseName_Failures(t *testing.T) {
	invalidNames := []string{
		"unmanaged-container",
		"docker-dayaapi-r49-1",
		"dc--r49-1",
		"dc-dayaapi-49-1",  // missing -r
		"dc-dayaapi-r49-0", // zero instance
		"dc-dayaapi-r49-",
		"",
	}

	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			_, err := ParseName(name)
			if err == nil {
				t.Errorf("expected error parsing invalid name %q, got nil", name)
			}
			if IsPlatformName(name) {
				t.Errorf("expected IsPlatformName(%q) to be false", name)
			}
		})
	}
}
