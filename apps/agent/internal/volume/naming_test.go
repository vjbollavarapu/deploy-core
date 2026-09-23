package volume

import (
	"strings"
	"testing"
)

func TestFormatVolumeName(t *testing.T) {
	tests := []struct {
		name      string
		scope     string
		volName   string
		expected  string
		shouldErr bool
	}{
		{
			name:      "valid platform volume name",
			scope:     "dayaapi",
			volName:   "data",
			expected:  "dc-dayaapi-data",
			shouldErr: false,
		},
		{
			name:      "hyphenated scope and volume name",
			scope:     "web-frontend",
			volName:   "uploads-cache",
			expected:  "dc-web-frontend-uploads-cache",
			shouldErr: false,
		},
		{
			name:      "empty scope",
			scope:     "",
			volName:   "data",
			shouldErr: true,
		},
		{
			name:      "empty volume name",
			scope:     "app",
			volName:   "",
			shouldErr: true,
		},
		{
			name:      "uppercase scope",
			scope:     "MyApp",
			volName:   "data",
			shouldErr: true,
		},
		{
			name:      "length exceeding 64 chars",
			scope:     strings.Repeat("a", 35),
			volName:   strings.Repeat("b", 35),
			shouldErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FormatVolumeName(tc.scope, tc.volName)
			if tc.shouldErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tc.shouldErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tc.shouldErr && got != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, got)
			}
		})
	}
}

func TestValidateVolumeName(t *testing.T) {
	valid := []string{
		"dc-app-data",
		"my_named_volume.1",
		"custom-vol-123",
	}
	for _, v := range valid {
		if err := ValidateVolumeName(v); err != nil {
			t.Errorf("expected valid volume name %q, got error: %v", v, err)
		}
	}

	invalid := []string{
		"",
		" ",
		"-leading-dash",
	}
	for _, v := range invalid {
		if err := ValidateVolumeName(v); err == nil {
			t.Errorf("expected invalid volume name %q, got nil error", v)
		}
	}
}

func TestIsPlatformVolumeName(t *testing.T) {
	if !IsPlatformVolumeName("dc-daya-data") {
		t.Errorf("expected dc-daya-data to be recognized as platform volume")
	}
	if IsPlatformVolumeName("unmanaged-volume") {
		t.Errorf("expected unmanaged-volume to NOT be recognized as platform volume")
	}
}
