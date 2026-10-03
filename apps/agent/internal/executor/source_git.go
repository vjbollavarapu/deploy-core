package executor

import (
	"context"
	"errors"
	"net/url"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
)

// gitCloner acquires a Git repository without invoking a shell.
type gitCloner interface {
	Clone(ctx context.Context, dest, repositoryURL, branch string) (commit string, err error)
}

type goGitCloner struct{}

func (goGitCloner) Clone(ctx context.Context, dest, repositoryURL, branch string) (string, error) {
	repo, err := git.PlainCloneContext(ctx, dest, false, &git.CloneOptions{
		URL:           repositoryURL,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
		Depth:         1,
	})
	if err != nil {
		return "", redactRepository(err, repositoryURL)
	}
	head, err := repo.Head()
	if err != nil {
		return "", redactRepository(err, repositoryURL)
	}
	return head.Hash().String(), nil
}

func redactRepository(err error, repositoryURL string) error {
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
