package gitproviders

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

var (
	ErrUnsupportedProvider = errors.New("unsupported git provider")
	ErrInvalidSignature    = errors.New("invalid webhook signature")
	ErrIgnoredEvent        = errors.New("ignored webhook event")
)

// Provider normalizes provider-specific webhook verification and parsing.
type Provider interface {
	Name() string
	VerifyWebhook(headers http.Header, body, secret []byte) error
	ParsePushEvent(headers http.Header, body []byte) (PushEvent, error)
}

func Lookup(name string) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case ProviderGitHub:
		return GitHubProvider{}, nil
	case ProviderGitLab:
		return stubProvider{name: ProviderGitLab}, nil
	case ProviderBitbucket:
		return stubProvider{name: ProviderBitbucket}, nil
	case ProviderGeneric:
		return stubProvider{name: ProviderGeneric}, nil
	default:
		return nil, ErrUnsupportedProvider
	}
}

type GitHubProvider struct{}

func (GitHubProvider) Name() string { return ProviderGitHub }

func (GitHubProvider) VerifyWebhook(headers http.Header, body, secret []byte) error {
	sig := strings.TrimSpace(headers.Get("X-Hub-Signature-256"))
	if sig == "" {
		return ErrInvalidSignature
	}
	const prefix = "sha256="
	if !strings.HasPrefix(sig, prefix) {
		return ErrInvalidSignature
	}
	want, err := hex.DecodeString(strings.TrimPrefix(sig, prefix))
	if err != nil || len(want) != sha256.Size {
		return ErrInvalidSignature
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(body)
	got := mac.Sum(nil)
	if !hmac.Equal(got, want) {
		return ErrInvalidSignature
	}
	return nil
}

func (p GitHubProvider) ParsePushEvent(headers http.Header, body []byte) (PushEvent, error) {
	eventType := strings.TrimSpace(headers.Get("X-GitHub-Event"))
	deliveryID := strings.TrimSpace(headers.Get("X-GitHub-Delivery"))
	if deliveryID == "" {
		return PushEvent{}, fmt.Errorf("missing X-GitHub-Delivery")
	}
	if eventType != "push" {
		return PushEvent{
			DeliveryID: deliveryID,
			EventType:  eventType,
		}, ErrIgnoredEvent
	}
	var raw struct {
		Deleted bool   `json:"deleted"`
		Ref     string `json:"ref"`
		After   string `json:"after"`
		Repo    struct {
			FullName string `json:"full_name"`
			CloneURL string `json:"clone_url"`
			HTMLURL  string `json:"html_url"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return PushEvent{}, fmt.Errorf("invalid push payload")
	}
	branch := strings.TrimPrefix(raw.Ref, "refs/heads/")
	if branch == raw.Ref {
		branch = strings.TrimPrefix(raw.Ref, "refs/tags/")
	}
	return PushEvent{
		DeliveryID:         deliveryID,
		EventType:          eventType,
		RepositoryFullName: raw.Repo.FullName,
		CloneURL:           raw.Repo.CloneURL,
		HTMLURL:            raw.Repo.HTMLURL,
		Branch:             branch,
		CommitSHA:          raw.After,
		Deleted:            raw.Deleted,
	}, nil
}

type stubProvider struct{ name string }

func (s stubProvider) Name() string { return s.name }

func (stubProvider) VerifyWebhook(http.Header, []byte, []byte) error {
	return fmt.Errorf("%w: webhook verification not implemented", ErrUnsupportedProvider)
}

func (stubProvider) ParsePushEvent(http.Header, []byte) (PushEvent, error) {
	return PushEvent{}, fmt.Errorf("%w: webhook parsing not implemented", ErrUnsupportedProvider)
}

func NormalizeRepoKey(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "git@")
	s = strings.ReplaceAll(s, ":", "/")
	if i := strings.Index(s, "github.com/"); i >= 0 {
		s = s[i+len("github.com/"):]
	}
	if i := strings.Index(s, "gitlab.com/"); i >= 0 {
		s = s[i+len("gitlab.com/"):]
	}
	parts := strings.Split(strings.Trim(s, "/"), "/")
	if len(parts) >= 2 {
		return parts[0] + "/" + parts[1]
	}
	return s
}

func BranchMatches(configured, eventBranch string) bool {
	a := strings.TrimSpace(configured)
	b := strings.TrimSpace(eventBranch)
	if a == "" || b == "" {
		return false
	}
	a = strings.TrimPrefix(a, "refs/heads/")
	b = strings.TrimPrefix(b, "refs/heads/")
	return strings.EqualFold(a, b)
}
