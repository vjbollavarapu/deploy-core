package gitproviders

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	defaultGitHubAPIBase = "https://api.github.com"
	githubAPIAccept      = "application/vnd.github+json"
	githubAPIUserAgent   = "deploycore"
	githubRepoPageSize   = 100
	githubRepoPageCap    = 100
	githubBodyLimit      = 8 << 20
)

// GitHubInstallation is the verified GitHub App installation record.
type GitHubInstallation struct {
	ID                  int64
	AppID               int64
	AccountLogin        string
	AccountID           int64
	AccountType         string
	RepositorySelection string
	Suspended           bool
}

// GitHubRepository is repository metadata returned by GitHub for an installation.
type GitHubRepository struct {
	ID            int64
	FullName      string
	Private       bool
	DefaultBranch string
	CloneURL      string
	HTMLURL       string
	Archived      bool
	Visibility    string
}

// GitHubAppAPI is the GitHub HTTP surface used by installation and repository sync.
type GitHubAppAPI interface {
	GetInstallation(ctx context.Context, appJWT string, installationID int64) (GitHubInstallation, error)
	CreateInstallationToken(ctx context.Context, appJWT string, installationID int64) (string, time.Time, error)
	ListInstallationRepositories(ctx context.Context, installationToken string) ([]GitHubRepository, error)
}

type githubStatusError struct {
	Status      int
	RateLimited bool
}

func (e *githubStatusError) Error() string {
	if e == nil {
		return "github status"
	}
	if e.RateLimited {
		return "github rate limited"
	}
	return fmt.Sprintf("github status %d", e.Status)
}

type httpGitHubAppClient struct {
	base   string
	client *http.Client
}

// NewGitHubAppClient talks to the GitHub API. An empty base URL uses api.github.com.
func NewGitHubAppClient(baseURL string) GitHubAppAPI {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = defaultGitHubAPIBase
	}
	return &httpGitHubAppClient{
		base:   base,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *httpGitHubAppClient) GetInstallation(ctx context.Context, appJWT string, installationID int64) (GitHubInstallation, error) {
	body, err := c.do(ctx, http.MethodGet, "/app/installations/"+strconv.FormatInt(installationID, 10), appJWT, nil)
	if err != nil {
		return GitHubInstallation{}, err
	}
	var payload struct {
		ID                  int64  `json:"id"`
		AppID               int64  `json:"app_id"`
		RepositorySelection string `json:"repository_selection"`
		SuspendedAt         any    `json:"suspended_at"`
		Account             struct {
			Login string `json:"login"`
			ID    int64  `json:"id"`
			Type  string `json:"type"`
		} `json:"account"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return GitHubInstallation{}, errors.New("github installation response was not valid")
	}
	if payload.ID == 0 || payload.Account.ID == 0 || strings.TrimSpace(payload.Account.Login) == "" {
		return GitHubInstallation{}, errors.New("github installation response was incomplete")
	}
	return GitHubInstallation{
		ID:                  payload.ID,
		AppID:               payload.AppID,
		AccountLogin:        strings.TrimSpace(payload.Account.Login),
		AccountID:           payload.Account.ID,
		AccountType:         strings.TrimSpace(payload.Account.Type),
		RepositorySelection: strings.TrimSpace(payload.RepositorySelection),
		Suspended:           payload.SuspendedAt != nil,
	}, nil
}

func (c *httpGitHubAppClient) CreateInstallationToken(ctx context.Context, appJWT string, installationID int64) (string, time.Time, error) {
	body, err := c.do(ctx, http.MethodPost, "/app/installations/"+strconv.FormatInt(installationID, 10)+"/access_tokens", appJWT, map[string]any{})
	if err != nil {
		return "", time.Time{}, err
	}
	var payload struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Token == "" {
		return "", time.Time{}, errors.New("github installation token response was not valid")
	}
	return payload.Token, payload.ExpiresAt, nil
}

func (c *httpGitHubAppClient) ListInstallationRepositories(ctx context.Context, installationToken string) ([]GitHubRepository, error) {
	var (
		out   []GitHubRepository
		next  = "/installation/repositories?per_page=" + strconv.Itoa(githubRepoPageSize) + "&page=1"
		pages int
	)
	for next != "" {
		pages++
		if pages > githubRepoPageCap {
			return nil, errors.New("github repository pagination exceeded the page cap")
		}
		body, link, err := c.doWithLink(ctx, http.MethodGet, next, installationToken, nil)
		if err != nil {
			return nil, err
		}
		var payload struct {
			TotalCount   int `json:"total_count"`
			Repositories []struct {
				ID            int64  `json:"id"`
				FullName      string `json:"full_name"`
				Private       bool   `json:"private"`
				DefaultBranch string `json:"default_branch"`
				CloneURL      string `json:"clone_url"`
				HTMLURL       string `json:"html_url"`
				Archived      bool   `json:"archived"`
				Visibility    string `json:"visibility"`
			} `json:"repositories"`
		}
		if err := json.Unmarshal(body, &payload); err != nil {
			return nil, errors.New("github repository response was not valid")
		}
		for _, repo := range payload.Repositories {
			out = append(out, GitHubRepository{
				ID:            repo.ID,
				FullName:      strings.TrimSpace(repo.FullName),
				Private:       repo.Private,
				DefaultBranch: strings.TrimSpace(repo.DefaultBranch),
				CloneURL:      strings.TrimSpace(repo.CloneURL),
				HTMLURL:       strings.TrimSpace(repo.HTMLURL),
				Archived:      repo.Archived,
				Visibility:    strings.TrimSpace(repo.Visibility),
			})
		}
		next = ""
		if rel := linkRel(link, "next"); rel != "" {
			next = rel
		} else if payload.TotalCount > len(out) && len(payload.Repositories) == githubRepoPageSize {
			next = "/installation/repositories?per_page=" + strconv.Itoa(githubRepoPageSize) + "&page=" + strconv.Itoa(pages+1)
		}
	}
	return out, nil
}

func (c *httpGitHubAppClient) do(ctx context.Context, method, path, bearer string, payload any) ([]byte, error) {
	body, _, err := c.doWithLink(ctx, method, path, bearer, payload)
	return body, err
}

func (c *httpGitHubAppClient) doWithLink(ctx context.Context, method, path, bearer string, payload any) ([]byte, string, error) {
	endpoint, err := c.resolve(path)
	if err != nil {
		return nil, "", err
	}
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, "", err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", githubAPIAccept)
	req.Header.Set("User-Agent", githubAPIUserAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, "", errors.New("github api request failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, githubBodyLimit))
		return nil, "", &githubStatusError{
			Status:      resp.StatusCode,
			RateLimited: resp.StatusCode == http.StatusTooManyRequests || (resp.StatusCode == http.StatusForbidden && resp.Header.Get("X-RateLimit-Remaining") == "0"),
		}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, githubBodyLimit+1))
	if err != nil {
		return nil, "", errors.New("github api response could not be read")
	}
	if len(raw) > githubBodyLimit {
		return nil, "", errors.New("github api response was too large")
	}
	return raw, resp.Header.Get("Link"), nil
}

func (c *httpGitHubAppClient) resolve(path string) (string, error) {
	if strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://") {
		u, err := url.Parse(path)
		if err != nil {
			return "", errors.New("github pagination link was invalid")
		}
		base, err := url.Parse(c.base)
		if err != nil {
			return "", errors.New("github api base url was invalid")
		}
		if !strings.EqualFold(u.Scheme, base.Scheme) || !strings.EqualFold(u.Host, base.Host) {
			return "", errors.New("github pagination link left the api host")
		}
		return u.String(), nil
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return c.base + path, nil
}

func linkRel(header, rel string) string {
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if !strings.Contains(part, `rel="`+rel+`"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start >= 0 && end > start+1 {
			return part[start+1 : end]
		}
	}
	return ""
}
