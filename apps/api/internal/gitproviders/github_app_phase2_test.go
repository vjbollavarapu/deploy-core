package gitproviders

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestSignGitHubAppJWT(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	now := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	signed, err := signGitHubAppJWT("5150", pemBytes, now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(signed, "PRIVATE") || strings.Contains(string(pemBytes), signed) {
		t.Fatal("jwt contained private key material or the key contained the jwt")
	}
	parsed, err := jwt.ParseWithClaims(signed, &jwt.RegisteredClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodRS256 {
			t.Fatalf("alg=%v", token.Header["alg"])
		}
		return &key.PublicKey, nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("parse jwt: %v", err)
	}
	claims := parsed.Claims.(*jwt.RegisteredClaims)
	if claims.Issuer != "5150" {
		t.Fatalf("iss=%s", claims.Issuer)
	}
	lifetime := claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time)
	if lifetime <= 0 || lifetime > 10*time.Minute {
		t.Fatalf("lifetime=%s", lifetime)
	}
	if !claims.IssuedAt.Time.Before(now) {
		t.Fatal("iat was not skewed into the past")
	}
	if _, err := signGitHubAppJWT("5150", []byte("not-a-key-secret-marker"), now); err == nil || strings.Contains(err.Error(), "secret-marker") {
		t.Fatalf("invalid key err=%v", err)
	}
}

func TestGitHubClientPaginationAndTokenRequest(t *testing.T) {
	var sawJWT, sawToken bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		switch {
		case r.URL.Path == "/app/installations/42":
			if strings.Count(bearer, ".") != 2 {
				t.Errorf("installation request did not use a jwt")
			}
			sawJWT = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"id":42,"app_id":5150,"repository_selection":"all","suspended_at":null,"account":{"login":"octo","id":7,"type":"Organization"}}`))
		case r.URL.Path == "/app/installations/42/access_tokens":
			sawJWT = strings.Count(bearer, ".") == 2
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"token":"ghs_memory_only","expires_at":"2026-10-04T13:00:00Z"}`))
		case r.URL.Path == "/installation/repositories":
			if bearer != "ghs_memory_only" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			sawToken = true
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if page == 0 {
				page = 1
			}
			w.Header().Set("Content-Type", "application/json")
			if page == 1 {
				w.Header().Set("Link", `<`+srvURL(r, 2)+`>; rel="next"`)
				_, _ = w.Write([]byte(`{"total_count":2,"repositories":[{"id":1,"full_name":"octo/a","private":true,"default_branch":"main","clone_url":"https://github.com/octo/a.git","html_url":"https://github.com/octo/a","archived":false,"visibility":"private"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"total_count":2,"repositories":[{"id":2,"full_name":"octo/b","private":false,"default_branch":"trunk","clone_url":"https://user:ghs_should_not_stick@github.com/octo/b.git","html_url":"https://github.com/octo/b","archived":true,"visibility":"public"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	client := NewGitHubAppClient(srv.URL)
	inst, err := client.GetInstallation(context.Background(), "header.payload.sig", 42)
	if err != nil || inst.AccountLogin != "octo" || inst.Suspended {
		t.Fatalf("installation=%+v err=%v", inst, err)
	}
	token, _, err := client.CreateInstallationToken(context.Background(), "header.payload.sig", 42)
	if err != nil || token != "ghs_memory_only" {
		t.Fatalf("token err=%v", err)
	}
	repos, err := client.ListInstallationRepositories(context.Background(), token)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 2 || repos[0].FullName != "octo/a" || repos[1].FullName != "octo/b" || !repos[1].Archived || repos[1].Visibility != "public" {
		t.Fatalf("repos=%+v", repos)
	}
	if !sawJWT || !sawToken {
		t.Fatalf("jwt=%v token=%v", sawJWT, sawToken)
	}
	clone, err := credentialFreeHTTPS(repos[1].CloneURL)
	if err != nil || clone != "https://github.com/octo/b.git" || strings.Contains(clone, "ghs_should_not_stick") {
		t.Fatalf("clone=%s err=%v", clone, err)
	}
}

func TestGitHubClientFailedPageAndRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"token":"ghs_should_not_leak","message":"boom"}`))
			return
		}
		w.Header().Set("Link", `<`+srvURL(r, 2)+`>; rel="next"`)
		_, _ = w.Write([]byte(`{"total_count":2,"repositories":[{"id":1,"full_name":"octo/a","clone_url":"https://github.com/octo/a.git"}]}`))
	}))
	defer srv.Close()
	_, err := NewGitHubAppClient(srv.URL).ListInstallationRepositories(context.Background(), "ghs_memory_only")
	if err == nil || strings.Contains(err.Error(), "ghs_should_not_leak") || strings.Contains(err.Error(), "boom") {
		t.Fatalf("err=%v", err)
	}

	limited := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer limited.Close()
	_, err = NewGitHubAppClient(limited.URL).GetInstallation(context.Background(), "header.payload.sig", 9)
	var status *githubStatusError
	if err == nil || !errorsAs(err, &status) || !status.RateLimited {
		t.Fatalf("rate err=%v", err)
	}
}

func srvURL(r *http.Request, page int) string {
	return "http://" + r.Host + "/installation/repositories?per_page=100&page=" + strconv.Itoa(page)
}

func errorsAs(err error, target **githubStatusError) bool {
	if err == nil {
		return false
	}
	status, ok := err.(*githubStatusError)
	if !ok {
		return false
	}
	*target = status
	return true
}
