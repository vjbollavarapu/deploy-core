package registries

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

var (
	ErrUnsupportedProvider = errors.New("unsupported registry provider")
	ErrInvalidRegistryURL  = errors.New("invalid registry url")
)

// Provider normalizes provider-specific defaults and validation.
// Pull/push execution is performed by agents using resolved credentials.
type Provider interface {
	Name() string
	DefaultURL() string
	NormalizeURL(raw string) (string, error)
	ValidateCredentials(creds *Credentials) error
}

func Lookup(name string) (Provider, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case ProviderGHCR:
		return ghcrProvider{}, nil
	case ProviderDockerHub:
		return dockerHubProvider{}, nil
	case ProviderOCI:
		return ociProvider{}, nil
	case ProviderGCP:
		return stubProvider{name: ProviderGCP, defaultURL: ""}, nil
	case ProviderECR:
		return stubProvider{name: ProviderECR, defaultURL: ""}, nil
	case ProviderACR:
		return stubProvider{name: ProviderACR, defaultURL: ""}, nil
	default:
		return nil, ErrUnsupportedProvider
	}
}

func SupportedInitially(provider string) bool {
	switch provider {
	case ProviderGHCR, ProviderDockerHub, ProviderOCI:
		return true
	default:
		return false
	}
}

type ghcrProvider struct{}

func (ghcrProvider) Name() string      { return ProviderGHCR }
func (ghcrProvider) DefaultURL() string { return "ghcr.io" }

func (p ghcrProvider) NormalizeURL(raw string) (string, error) {
	return normalizeHost(raw, p.DefaultURL())
}

func (ghcrProvider) ValidateCredentials(creds *Credentials) error {
	if creds == nil {
		return nil
	}
	if strings.TrimSpace(creds.Token) == "" && strings.TrimSpace(creds.Password) == "" {
		return fmt.Errorf("ghcr credentials require token or password")
	}
	return nil
}

type dockerHubProvider struct{}

func (dockerHubProvider) Name() string      { return ProviderDockerHub }
func (dockerHubProvider) DefaultURL() string { return "docker.io" }

func (p dockerHubProvider) NormalizeURL(raw string) (string, error) {
	return normalizeHost(raw, p.DefaultURL())
}

func (dockerHubProvider) ValidateCredentials(creds *Credentials) error {
	if creds == nil {
		return nil
	}
	if strings.TrimSpace(creds.Username) == "" {
		return fmt.Errorf("dockerhub credentials require username")
	}
	if strings.TrimSpace(creds.Token) == "" && strings.TrimSpace(creds.Password) == "" {
		return fmt.Errorf("dockerhub credentials require token or password")
	}
	return nil
}

type ociProvider struct{}

func (ociProvider) Name() string      { return ProviderOCI }
func (ociProvider) DefaultURL() string { return "" }

func (ociProvider) NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("%w: registryUrl is required for oci", ErrInvalidRegistryURL)
	}
	return normalizeHost(raw, "")
}

func (ociProvider) ValidateCredentials(creds *Credentials) error {
	return nil
}

type stubProvider struct {
	name       string
	defaultURL string
}

func (s stubProvider) Name() string      { return s.name }
func (s stubProvider) DefaultURL() string { return s.defaultURL }

func (s stubProvider) NormalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if s.defaultURL != "" {
			return s.defaultURL, nil
		}
		return "", fmt.Errorf("%w: registryUrl is required for %s", ErrInvalidRegistryURL, s.name)
	}
	return normalizeHost(raw, s.defaultURL)
}

func (stubProvider) ValidateCredentials(*Credentials) error { return nil }

func normalizeHost(raw, fallback string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if fallback == "" {
			return "", ErrInvalidRegistryURL
		}
		return fallback, nil
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", ErrInvalidRegistryURL
	}
	host := strings.ToLower(u.Host)
	if u.Path != "" && u.Path != "/" {
		host = host + strings.TrimSuffix(u.Path, "/")
	}
	return host, nil
}
