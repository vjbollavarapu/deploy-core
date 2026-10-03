package executor

import (
	"context"
	"errors"
	"net/url"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	githttp "github.com/go-git/go-git/v5/plumbing/transport/http"
)

// cloneAuth is in-memory HTTPS basic authentication for one clone.
// Password is not written to disk and is not guaranteed to be zeroed.
type cloneAuth struct {
	Username string
	Password string
}

// gitCloner acquires a Git repository without invoking a shell.
type gitCloner interface {
	Clone(ctx context.Context, dest, repositoryURL, branch string, auth *cloneAuth) (commit string, err error)
}

type goGitCloner struct{}

func (goGitCloner) Clone(ctx context.Context, dest, repositoryURL, branch string, auth *cloneAuth) (string, error) {
	opts := &git.CloneOptions{
		URL:           repositoryURL,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Depth:         1,
	}
	var secret string
	if auth != nil {
		secret = auth.Password
		opts.Auth = &githttp.BasicAuth{Username: auth.Username, Password: auth.Password}
	}
	repo, err := git.PlainCloneContext(ctx, dest, false, opts)
	if err != nil {
		return "", redactRepository(err, repositoryURL, secret)
	}
	head, err := repo.Head()
	if err != nil {
		return "", redactRepository(err, repositoryURL, secret)
	}
	return head.Hash().String(), nil
}

func redactRepository(err error, repositoryURL string, secrets ...string) error {
	if err == nil {
		return nil
	}
	msg := SanitizeMessage(err.Error(), repositoryURL)
	if parsed, parseErr := url.Parse(repositoryURL); parseErr == nil && parsed.User != nil {
		msg = SanitizeMessage(msg, parsed.User.String())
		if password, ok := parsed.User.Password(); ok {
			msg = SanitizeMessage(msg, password)
		}
	}
	for _, secret := range secrets {
		msg = SanitizeMessage(msg, secret)
	}
	return errors.New(msg)
}

func publicRepositoryURL(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" {
		return ""
	}
	parsed.User = nil
	return parsed.String()
}
