package protocol

import "testing"

func TestSanitizeApplicationSlug(t *testing.T) {
	if got := SanitizeApplicationSlug(" Redis_Cache.API "); got != "rediscacheapi" {
		t.Fatalf("got %q", got)
	}
	if got := SanitizeApplicationSlug(""); got != "app" {
		t.Fatalf("empty slug = %q", got)
	}
	long := SanitizeApplicationSlug("abcdefghijklmnopqrstuvwxyz0123456789")
	if long != "abcdefghijklmnopqrstuvwxyz0123" || len(long) != 30 {
		t.Fatalf("truncated slug = %q", long)
	}
}

func TestPlatformContainerName(t *testing.T) {
	got, err := PlatformContainerName("redis", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "dc-redis-r1-1" {
		t.Fatalf("got %q", got)
	}
	second, err := PlatformContainerName("redis", 2, 0)
	if err != nil || second != "dc-redis-r2-1" {
		t.Fatalf("second = %q err=%v", second, err)
	}
	if _, err := PlatformContainerName("redis", 0, 0); err == nil {
		t.Fatal("revision 0 was accepted")
	}
	if _, err := PlatformContainerName("redis", 1, -1); err == nil {
		t.Fatal("negative replica index was accepted")
	}
}
