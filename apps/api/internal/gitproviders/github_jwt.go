package gitproviders

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	githubJWTIssuedAtSkew = 60 * time.Second
	githubJWTLifetime     = 8 * time.Minute
)

// signGitHubAppJWT builds a short-lived RS256 JWT for GitHub App authentication.
// The token is returned to the caller and must not be logged or stored.
func signGitHubAppJWT(appID string, privateKeyPEM []byte, now time.Time) (string, error) {
	if appID == "" || len(privateKeyPEM) == 0 {
		return "", errors.New("github app identity is incomplete")
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM(privateKeyPEM)
	if err != nil {
		return "", errors.New("github app private key is invalid")
	}
	issuedAt := now.Add(-githubJWTIssuedAtSkew)
	claims := jwt.RegisteredClaims{
		Issuer:    appID,
		IssuedAt:  jwt.NewNumericDate(issuedAt),
		ExpiresAt: jwt.NewNumericDate(issuedAt.Add(githubJWTLifetime)),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	signed, err := token.SignedString(key)
	if err != nil {
		return "", errors.New("github app jwt could not be signed")
	}
	return signed, nil
}
