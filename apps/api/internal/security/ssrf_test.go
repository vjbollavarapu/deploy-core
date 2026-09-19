package security

import (
	"net"
	"testing"
)

func TestValidateOutboundURL(t *testing.T) {
	AllowPrivateOutboundHosts.Store(false)
	prev := lookupIP
	lookupIP = func(host string) ([]net.IP, error) {
		switch host {
		case "example.com":
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		case "evil.internal":
			return []net.IP{net.ParseIP("10.0.0.5")}, nil
		default:
			return nil, &net.DNSError{Err: "no such host", Name: host, IsNotFound: true}
		}
	}
	t.Cleanup(func() { lookupIP = prev })

	cases := []struct {
		url string
		ok  bool
	}{
		{"https://example.com/hook", true},
		{"http://example.com/hook", true},
		{"https://8.8.8.8/hook", true},
		{"ftp://example.com", false},
		{"https://localhost/x", false},
		{"https://127.0.0.1/x", false},
		{"http://169.254.169.254/latest/meta-data", false},
		{"https://10.0.0.5/hook", false},
		{"https://192.168.1.1/hook", false},
		{"https://evil.internal/hook", false},
		{"", false},
	}
	for _, tc := range cases {
		err := ValidateOutboundURL(tc.url)
		if tc.ok && err != nil {
			t.Fatalf("%s: unexpected err %v", tc.url, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("%s: expected error", tc.url)
		}
	}
}

func TestValidateContainerMountPath(t *testing.T) {
	if err := ValidateContainerMountPath("/var/lib/data"); err != nil {
		t.Fatal(err)
	}
	bads := []string{"", "relative", "/var/../etc", "/var/lib/../data", "//x", "/var/lib/data;rm"}
	for _, b := range bads {
		if err := ValidateContainerMountPath(b); err == nil {
			t.Fatalf("expected error for %q", b)
		}
	}
}

func TestSanitizeLogField(t *testing.T) {
	if SanitizeLogField("hello\nworld", 100) != "hello world" {
		t.Fatalf("got %q", SanitizeLogField("hello\nworld", 100))
	}
	got := SanitizeLogField("a\nb\rc\x00d", 10)
	if got != "a b cd" {
		t.Fatalf("got %q", got)
	}
}
