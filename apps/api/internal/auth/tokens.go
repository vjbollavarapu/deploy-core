package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const accessTokenIssuer = "deploycore-api"

type accessClaims struct {
	SessionID string `json:"sid"`
	jwt.RegisteredClaims
}

func issueAccessToken(secret []byte, userID, sessionID uuid.UUID, ttl time.Duration, now time.Time) (string, time.Time, error) {
	expires := now.Add(ttl)
	claims := accessClaims{
		SessionID: sessionID.String(),
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID.String(),
			Issuer:    accessTokenIssuer,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expires),
			ID:        uuid.NewString(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(secret)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, expires, nil
}

func parseAccessToken(secret []byte, raw string) (userID, sessionID uuid.UUID, err error) {
	parsed, err := jwt.ParseWithClaims(raw, &accessClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return secret, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(accessTokenIssuer))
	if err != nil {
		return uuid.Nil, uuid.Nil, err
	}
	claims, ok := parsed.Claims.(*accessClaims)
	if !ok || !parsed.Valid {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid token claims")
	}
	userID, err = uuid.Parse(claims.Subject)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid subject")
	}
	sessionID, err = uuid.Parse(claims.SessionID)
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("invalid session id")
	}
	return userID, sessionID, nil
}
