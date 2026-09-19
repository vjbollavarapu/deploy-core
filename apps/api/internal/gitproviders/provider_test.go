package gitproviders_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/gitproviders"
)

func TestGitHubVerifyAndParsePush(t *testing.T) {
	p := gitproviders.GitHubProvider{}
	secret := []byte("whsec-test")
	body := []byte(`{
		"ref":"refs/heads/main",
		"after":"abc123deadbeef",
		"deleted":false,
		"repository":{
			"full_name":"acme/api",
			"clone_url":"https://github.com/acme/api.git",
			"html_url":"https://github.com/acme/api"
		}
	}`)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	sig := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	hdr := http.Header{}
	hdr.Set("X-Hub-Signature-256", sig)
	hdr.Set("X-GitHub-Event", "push")
	hdr.Set("X-GitHub-Delivery", "deliv-1")

	if err := p.VerifyWebhook(hdr, body, secret); err != nil {
		t.Fatalf("verify: %v", err)
	}
	ev, err := p.ParsePushEvent(hdr, body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if ev.Branch != "main" || ev.RepositoryFullName != "acme/api" || ev.CommitSHA != "abc123deadbeef" {
		t.Fatalf("event=%#v", ev)
	}
}

func TestNormalizeRepoKey(t *testing.T) {
	cases := map[string]string{
		"https://github.com/Acme/API.git": "acme/api",
		"git@github.com:Acme/API.git":     "acme/api",
		"Acme/API":                        "acme/api",
	}
	for in, want := range cases {
		if got := gitproviders.NormalizeRepoKey(in); got != want {
			t.Fatalf("%s -> %s want %s", in, got, want)
		}
	}
}

func TestIgnoredNonPush(t *testing.T) {
	p := gitproviders.GitHubProvider{}
	hdr := http.Header{}
	hdr.Set("X-GitHub-Event", "ping")
	hdr.Set("X-GitHub-Delivery", "deliv-ping")
	_, err := p.ParsePushEvent(hdr, []byte(`{}`))
	if err == nil {
		t.Fatal("expected ignored")
	}
	_ = json.RawMessage{}
}
