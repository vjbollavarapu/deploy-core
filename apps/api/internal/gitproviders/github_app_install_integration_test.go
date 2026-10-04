package gitproviders_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
)

const (
	installTokenMarker = "ghs_test_installation_token_marker"
	cloneUserMarker    = "ghs_clone_userinfo_marker"
	testWebhookMarker  = "phase2-webhook-marker-9f3a"
)

func TestGitHubAppBeginInstall(t *testing.T) {
	pool := testPool(t)
	plain := testServer(t, pool)
	ownerTok := register(t, plain, "owner-begin-"+uuid.NewString()+"@example.com", "password123", "Owner")
	viewerEmail := "viewer-begin-" + uuid.NewString() + "@example.com"
	viewerTok := register(t, plain, viewerEmail, "password123", "Viewer")
	orgID := createOrg(t, plain, ownerTok, "Begin Org", "beg-"+uuid.NewString()[:8])
	inviteViewer(t, plain, ownerTok, viewerTok, orgID, viewerEmail)

	unauth := doJSON(t, plain, http.MethodPost, "/api/v1/integrations/github/installations", []byte(`{"organizationId":"`+orgID+`"}`), "")
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d body=%s", unauth.Code, unauth.Body.String())
	}
	unconfigured := doJSON(t, plain, http.MethodPost, "/api/v1/integrations/github/installations", []byte(`{"organizationId":"`+orgID+`"}`), ownerTok)
	if unconfigured.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured status=%d body=%s", unconfigured.Code, unconfigured.Body.String())
	}

	var logs bytes.Buffer
	fake := newFakeGitHub(t)
	app := loadPhase2App(t)
	srv := phase2Server(t, pool, fake.URL(), &logs, app.cfg)
	forbidden := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations", []byte(`{"organizationId":"`+orgID+`"}`), viewerTok)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("viewer status=%d body=%s", forbidden.Code, forbidden.Body.String())
	}

	first := beginInstall(t, srv, ownerTok, orgID)
	second := beginInstall(t, srv, ownerTok, orgID)
	if first.State == second.State || len(first.State) < 43 || strings.Contains(first.URL, ownerTok) {
		t.Fatalf("states were not opaque high-entropy values")
	}
	if strings.Contains(first.URL, testWebhookMarker) || strings.Contains(first.URL, "PRIVATE KEY") || strings.Contains(first.Raw, app.keyMarker) {
		t.Fatal("begin response exposed GitHub App secret material")
	}
	parsed, err := url.Parse(first.URL)
	if err != nil || parsed.Host != "github.com" || parsed.Path != "/apps/deploycore/installations/new" || parsed.Query().Get("state") != first.State || len(parsed.Query()) != 1 {
		t.Fatalf("installation url=%s", first.URL)
	}
	var hash string
	var stateUser, stateOrg string
	if err := pool.QueryRow(context.Background(), `
		SELECT state_hash, user_id::text, organization_id::text
		FROM github_app_install_states
		WHERE state_hash = $1`, sha256Hex(first.State)).Scan(&hash, &stateUser, &stateOrg); err != nil {
		t.Fatal(err)
	}
	if hash == first.State || stateOrg != orgID {
		t.Fatalf("state was stored raw or bound to the wrong organization")
	}
	var rawCount int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM github_app_install_states
		WHERE state_hash = $1 OR state_hash LIKE '%' || $1 || '%'`, first.State).Scan(&rawCount); err != nil {
		t.Fatal(err)
	}
	if rawCount != 0 {
		t.Fatal("raw installation state was stored")
	}
	if strings.Contains(logs.String(), first.State) || strings.Contains(logs.String(), app.keyMarker) || strings.Contains(logs.String(), testWebhookMarker) {
		t.Fatal("logs contained installation state or GitHub App secrets")
	}
}

func TestGitHubAppInstallStateRejection(t *testing.T) {
	pool := testPool(t)
	fake := newFakeGitHub(t)
	app := loadPhase2App(t)
	srv := phase2Server(t, pool, fake.URL(), io.Discard, app.cfg)
	ownerTok := register(t, srv, "owner-state-"+uuid.NewString()+"@example.com", "password123", "Owner")
	otherTok := register(t, srv, "other-state-"+uuid.NewString()+"@example.com", "password123", "Other")
	orgID := createOrg(t, srv, ownerTok, "State Org", "st-"+uuid.NewString()[:8])
	otherOrg := createOrg(t, srv, otherTok, "Other Org", "ot-"+uuid.NewString()[:8])

	begun := beginInstall(t, srv, ownerTok, orgID)
	expiredBody := completeBody(4242, "install", begun.State)
	if _, err := pool.Exec(context.Background(), `
		UPDATE github_app_install_states SET expires_at = NOW() - INTERVAL '1 minute' WHERE state_hash = $1`, sha256Hex(begun.State)); err != nil {
		t.Fatal(err)
	}
	expired := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations/complete", expiredBody, ownerTok)
	if expired.Code != http.StatusBadRequest || !strings.Contains(expired.Body.String(), "expired") {
		t.Fatalf("expired status=%d body=%s", expired.Code, expired.Body.String())
	}
	assertStateOpen(t, pool, begun.State)
	if fake.calls() != 0 {
		t.Fatalf("expired state called GitHub %d times", fake.calls())
	}

	replay := beginInstall(t, srv, ownerTok, orgID)
	ok := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations/complete", completeBody(4242, "install", replay.State), ownerTok)
	if ok.Code != http.StatusCreated {
		t.Fatalf("complete status=%d body=%s", ok.Code, ok.Body.String())
	}
	callsAfterSuccess := fake.calls()
	again := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations/complete", completeBody(4242, "install", replay.State), ownerTok)
	if again.Code != http.StatusConflict || !strings.Contains(again.Body.String(), "already been used") {
		t.Fatalf("replay status=%d body=%s", again.Code, again.Body.String())
	}

	wrongUserState := beginInstall(t, srv, ownerTok, orgID)
	wrongUser := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations/complete", completeBody(4242, "install", wrongUserState.State), otherTok)
	if wrongUser.Code != http.StatusForbidden || !strings.Contains(wrongUser.Body.String(), "another user") {
		t.Fatalf("wrong user status=%d body=%s", wrongUser.Code, wrongUser.Body.String())
	}
	assertStateOpen(t, pool, wrongUserState.State)

	wrongOrgState := beginInstall(t, srv, ownerTok, orgID)
	if _, err := pool.Exec(context.Background(), `
		UPDATE github_app_install_states SET organization_id = $2 WHERE state_hash = $1`, sha256Hex(wrongOrgState.State), otherOrg); err != nil {
		t.Fatal(err)
	}
	wrongOrg := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations/complete", completeBody(4242, "install", wrongOrgState.State), ownerTok)
	if wrongOrg.Code != http.StatusForbidden {
		t.Fatalf("wrong org status=%d body=%s", wrongOrg.Code, wrongOrg.Body.String())
	}
	assertStateOpen(t, pool, wrongOrgState.State)

	badAction := beginInstall(t, srv, ownerTok, orgID)
	bad := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations/complete", completeBody(4242, "request", badAction.State), ownerTok)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("setup action status=%d body=%s", bad.Code, bad.Body.String())
	}
	assertStateOpen(t, pool, badAction.State)
	if fake.calls() != callsAfterSuccess {
		t.Fatalf("rejected states called GitHub after a successful install: before=%d after=%d", callsAfterSuccess, fake.calls())
	}
}

func TestGitHubAppInstallStateConsumedAtomically(t *testing.T) {
	pool := testPool(t)
	fake := newFakeGitHub(t)
	fake.pageSize = 100
	fake.repos = []map[string]any{repoJSON(101, "verified-octo/api", false, false, "public", "https://github.com/verified-octo/api.git")}
	app := loadPhase2App(t)
	srv := phase2Server(t, pool, fake.URL(), io.Discard, app.cfg)
	ownerTok := register(t, srv, "owner-race-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "Race Org", "rc-"+uuid.NewString()[:8])
	begun := beginInstall(t, srv, ownerTok, orgID)
	body := completeBody(4242, "install", begun.State)

	var wg sync.WaitGroup
	codes := make([]int, 2)
	wg.Add(2)
	for i := range codes {
		go func(i int) {
			defer wg.Done()
			rec := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations/complete", body, ownerTok)
			codes[i] = rec.Code
		}(i)
	}
	wg.Wait()
	created, replayed := 0, 0
	for _, code := range codes {
		switch code {
		case http.StatusCreated:
			created++
		case http.StatusConflict:
			replayed++
		}
	}
	if created != 1 || replayed != 1 {
		t.Fatalf("atomic consume codes=%v", codes)
	}
	var live int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM git_connections WHERE installation_id = 4242 AND deleted_at IS NULL`).Scan(&live); err != nil || live != 1 {
		t.Fatalf("live connections=%d err=%v", live, err)
	}
}

func TestGitHubAppSuspendedInstallationIsNotSynced(t *testing.T) {
	pool := testPool(t)
	fake := newFakeGitHub(t)
	fake.suspended = true
	app := loadPhase2App(t)
	srv := phase2Server(t, pool, fake.URL(), io.Discard, app.cfg)
	ownerTok := register(t, srv, "owner-susp-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "Suspended Org", "su-"+uuid.NewString()[:8])
	begun := beginInstall(t, srv, ownerTok, orgID)
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations/complete", completeBody(4242, "install", begun.State), ownerTok)
	if rec.Code != http.StatusConflict || strings.Contains(rec.Body.String(), installTokenMarker) {
		t.Fatalf("suspended status=%d body=%s", rec.Code, rec.Body.String())
	}
	var status string
	var synced *time.Time
	if err := pool.QueryRow(context.Background(), `
		SELECT status, last_sync_at FROM git_connections WHERE installation_id = 4242 AND deleted_at IS NULL`).Scan(&status, &synced); err != nil {
		t.Fatal(err)
	}
	if status != "disabled" || synced != nil {
		t.Fatalf("status=%s synced=%v", status, synced)
	}
	if fake.calls() != 1 {
		t.Fatalf("suspended install minted a token or listed repositories: calls=%d", fake.calls())
	}
}

func TestGitHubAppInstallationSync(t *testing.T) {
	pool := testPool(t)
	var logs bytes.Buffer
	fake := newFakeGitHub(t)
	fake.pageSize = 1
	fake.repos = []map[string]any{
		repoJSON(101, "verified-octo/api", true, true, "private", "https://x-access-token:"+cloneUserMarker+"@github.com/verified-octo/api.git"),
		repoJSON(102, "verified-octo/web", false, false, "public", "https://github.com/verified-octo/web.git"),
	}
	app := loadPhase2App(t)
	srv := phase2Server(t, pool, fake.URL(), &logs, app.cfg)
	ownerTok := register(t, srv, "owner-sync-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "Sync Org", "sy-"+uuid.NewString()[:8])

	override := map[string]any{"installationId": 4242, "setupAction": "install", "state": "ignored", "accountLogin": "browser-attacker", "accountId": "1", "organizationId": orgID}
	rawOverride, _ := json.Marshal(override)
	rejected := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations/complete", rawOverride, ownerTok)
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("browser metadata status=%d body=%s", rejected.Code, rejected.Body.String())
	}

	begun := beginInstall(t, srv, ownerTok, orgID)
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations/complete", completeBody(4242, "install", begun.State), ownerTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("complete status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, installTokenMarker) || strings.Contains(body, cloneUserMarker) || strings.Contains(body, testWebhookMarker) || strings.Contains(body, "PRIVATE KEY") || strings.Contains(body, app.keyMarker) {
		t.Fatal("complete response exposed credential material")
	}
	var created struct {
		Connection struct {
			ID                  string  `json:"id"`
			Provider            string  `json:"provider"`
			AuthMode            string  `json:"authMode"`
			AccountLogin        string  `json:"accountLogin"`
			AccountType         string  `json:"accountType"`
			RepositorySelection string  `json:"repositorySelection"`
			Status              string  `json:"status"`
			InstallationID      int64   `json:"installationId"`
			LastSyncAt          *string `json:"lastSyncAt"`
		} `json:"connection"`
		RepositoryCount int `json:"repositoryCount"`
	}
	decode(t, rec, &created)
	if created.Connection.Provider != "github" || created.Connection.AuthMode != "github_app" || created.Connection.AccountLogin != "verified-octo" || created.Connection.AccountType != "Organization" || created.Connection.RepositorySelection != "selected" || created.Connection.Status != "active" || created.Connection.InstallationID != 4242 || created.RepositoryCount != 2 || created.Connection.LastSyncAt == nil {
		t.Fatalf("connection=%+v count=%d", created.Connection, created.RepositoryCount)
	}
	assertCredentialColumnsNull(t, pool, created.Connection.ID)
	assertNoSecretMaterial(t, pool, logs.String(), app.keyMarker)

	repos := listRepos(t, srv, ownerTok, created.Connection.ID)
	if len(repos) != 2 {
		t.Fatalf("repos=%d", len(repos))
	}
	byName := map[string]repoItem{}
	for _, repo := range repos {
		byName[repo.FullName] = repo
		if strings.Contains(repo.CloneURL, "@") || strings.Contains(repo.CloneURL, cloneUserMarker) || !strings.HasPrefix(repo.CloneURL, "https://") {
			t.Fatalf("clone url %s", repo.CloneURL)
		}
	}
	apiRepo := byName["verified-octo/api"]
	if apiRepo.ExternalID != "101" || apiRepo.Metadata["private"] != true || apiRepo.Metadata["archived"] != true || apiRepo.Metadata["visibility"] != "private" {
		t.Fatalf("api repo=%+v", apiRepo)
	}
	if byName["verified-octo/web"].ExternalID != "102" || byName["verified-octo/web"].Metadata["visibility"] != "public" {
		t.Fatalf("web repo=%+v", byName["verified-octo/web"])
	}
	assertGitHubJWT(t, fake.jwts(), app.publicKey)

	fake.repos = []map[string]any{
		repoJSON(101, "verified-octo/api-renamed", true, false, "private", "https://github.com/verified-octo/api-renamed.git"),
	}
	updatedState := beginInstall(t, srv, ownerTok, orgID)
	updated := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations/complete", completeBody(4242, "update", updatedState.State), ownerTok)
	if updated.Code != http.StatusCreated {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	var updatedOut struct {
		Connection struct {
			ID string `json:"id"`
		} `json:"connection"`
		RepositoryCount int `json:"repositoryCount"`
	}
	decode(t, updated, &updatedOut)
	if updatedOut.Connection.ID != created.Connection.ID || updatedOut.RepositoryCount != 1 {
		t.Fatalf("update created a duplicate connection: %+v", updatedOut)
	}
	renamed := listRepos(t, srv, ownerTok, created.Connection.ID)
	if len(renamed) != 1 || renamed[0].ExternalID != "101" || renamed[0].FullName != "verified-octo/api-renamed" {
		t.Fatalf("rename repos=%+v", renamed)
	}

	var live int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM git_connections WHERE installation_id = 4242 AND deleted_at IS NULL`).Scan(&live); err != nil || live != 1 {
		t.Fatalf("live=%d err=%v", live, err)
	}
}

func TestGitHubAppInstallationOwnershipAndManualSync(t *testing.T) {
	pool := testPool(t)
	fake := newFakeGitHub(t)
	fake.pageSize = 100
	fake.repos = []map[string]any{repoJSON(101, "verified-octo/api", false, false, "public", "https://github.com/verified-octo/api.git")}
	app := loadPhase2App(t)
	srv := phase2Server(t, pool, fake.URL(), io.Discard, app.cfg)
	ownerTok := register(t, srv, "owner-own-"+uuid.NewString()+"@example.com", "password123", "Owner")
	otherTok := register(t, srv, "other-own-"+uuid.NewString()+"@example.com", "password123", "Other")
	orgID := createOrg(t, srv, ownerTok, "Owner Org", "ow-"+uuid.NewString()[:8])
	otherOrg := createOrg(t, srv, otherTok, "Claim Org", "cl-"+uuid.NewString()[:8])
	first := beginInstall(t, srv, ownerTok, orgID)
	created := completeInstall(t, srv, ownerTok, 4242, "install", first.State)

	second := beginInstall(t, srv, otherTok, otherOrg)
	claim := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations/complete", completeBody(4242, "install", second.State), otherTok)
	if claim.Code != http.StatusConflict || !strings.Contains(claim.Body.String(), "another organization") {
		t.Fatalf("claim status=%d body=%s", claim.Code, claim.Body.String())
	}
	var owner string
	if err := pool.QueryRow(context.Background(), `
		SELECT organization_id::text FROM git_connections WHERE id = $1 AND deleted_at IS NULL`, created.ID).Scan(&owner); err != nil || owner != orgID {
		t.Fatalf("owner=%s err=%v", owner, err)
	}

	del := doJSON(t, srv, http.MethodDelete, "/api/v1/integrations/git/connections/"+created.ID, nil, ownerTok)
	if del.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", del.Code)
	}
	reinstall := beginInstall(t, srv, otherTok, otherOrg)
	recreated := completeInstall(t, srv, otherTok, 4242, "install", reinstall.State)
	if recreated.ID == created.ID {
		t.Fatal("soft-deleted installation was revived in place")
	}

	patSrv := testServer(t, pool)
	patTok := register(t, patSrv, "pat-own-"+uuid.NewString()+"@example.com", "password123", "Pat")
	patOrg := createOrg(t, patSrv, patTok, "Pat Org", "pt-"+uuid.NewString()[:8])
	patBody, _ := json.Marshal(map[string]any{
		"organizationId": patOrg, "provider": "github", "accountLogin": "pat-user",
		"displayName": "PAT", "accessToken": "ghp_backward_compatible",
	})
	patRec := doJSON(t, patSrv, http.MethodPost, "/api/v1/integrations/git/connections", patBody, patTok)
	if patRec.Code != http.StatusCreated {
		t.Fatalf("pat create status=%d body=%s", patRec.Code, patRec.Body.String())
	}
	var patOut struct {
		Connection struct {
			ID       string `json:"id"`
			AuthMode string `json:"authMode"`
		} `json:"connection"`
	}
	decode(t, patRec, &patOut)
	syncBody, _ := json.Marshal(map[string]any{"repositories": []map[string]any{{
		"externalId": "7", "fullName": "pat-user/kept", "defaultBranch": "main",
		"cloneUrl": "https://github.com/pat-user/kept.git", "htmlUrl": "https://github.com/pat-user/kept",
	}}})
	synced := doJSON(t, patSrv, http.MethodPost, "/api/v1/integrations/git/connections/"+patOut.Connection.ID+"/sync", syncBody, patTok)
	if synced.Code != http.StatusOK || patOut.Connection.AuthMode != "pat" {
		t.Fatalf("pat sync status=%d auth=%s body=%s", synced.Code, patOut.Connection.AuthMode, synced.Body.String())
	}
	patRepos := listRepos(t, patSrv, patTok, patOut.Connection.ID)
	if len(patRepos) != 1 || patRepos[0].FullName != "pat-user/kept" {
		t.Fatalf("pat repos=%+v", patRepos)
	}

	previous := listRepos(t, srv, otherTok, recreated.ID)
	var previousSync string
	if err := pool.QueryRow(context.Background(), `SELECT last_sync_at::text FROM git_connections WHERE id = $1`, recreated.ID).Scan(&previousSync); err != nil {
		t.Fatal(err)
	}
	fake.failPage = 2
	fake.pageSize = 1
	fake.repos = []map[string]any{
		repoJSON(301, "evil/partial", false, false, "public", "https://github.com/evil/partial.git"),
		repoJSON(302, "evil/second", false, false, "public", "https://github.com/evil/second.git"),
	}
	evilBody, _ := json.Marshal(map[string]any{"repositories": []map[string]any{{
		"externalId": "999", "fullName": "evil/injected", "defaultBranch": "main",
		"cloneUrl": "https://github.com/evil/injected.git", "htmlUrl": "https://github.com/evil/injected",
	}}})
	failed := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/git/connections/"+recreated.ID+"/sync", evilBody, otherTok)
	if failed.Code < 400 || strings.Contains(failed.Body.String(), installTokenMarker) {
		t.Fatalf("failed sync status=%d body=%s", failed.Code, failed.Body.String())
	}
	var status, syncAt string
	if err := pool.QueryRow(context.Background(), `
		SELECT status, last_sync_at::text FROM git_connections WHERE id = $1`, recreated.ID).Scan(&status, &syncAt); err != nil {
		t.Fatal(err)
	}
	if status != "error" || syncAt != previousSync {
		t.Fatalf("status=%s sync=%s previous=%s", status, syncAt, previousSync)
	}
	after := listRepos(t, srv, otherTok, recreated.ID)
	if len(after) != len(previous) || after[0].FullName != previous[0].FullName {
		t.Fatalf("failed sync changed repos from %+v to %+v", previous, after)
	}
	for _, repo := range after {
		if repo.FullName == "evil/injected" || repo.FullName == "evil/partial" {
			t.Fatalf("client or partial page was stored: %+v", after)
		}
	}

	fresh := newFakeGitHub(t)
	fresh.installCode = http.StatusNotFound
	freshSrv := phase2Server(t, pool, fresh.URL(), io.Discard, app.cfg)
	missingState := beginInstall(t, freshSrv, ownerTok, orgID)
	missing := doJSON(t, freshSrv, http.MethodPost, "/api/v1/integrations/github/installations/complete", completeBody(777, "install", missingState.State), ownerTok)
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing installation status=%d body=%s", missing.Code, missing.Body.String())
	}
	var connections int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM git_connections WHERE installation_id = 777`).Scan(&connections); err != nil || connections != 0 {
		t.Fatalf("connections=%d err=%v", connections, err)
	}

	fresh.installCode = http.StatusUnauthorized
	unauthState := beginInstall(t, freshSrv, ownerTok, orgID)
	unauth := doJSON(t, freshSrv, http.MethodPost, "/api/v1/integrations/github/installations/complete", completeBody(778, "install", unauthState.State), ownerTok)
	if unauth.Code != http.StatusBadGateway || strings.Contains(unauth.Body.String(), installTokenMarker) {
		t.Fatalf("github unauthorized status=%d body=%s", unauth.Code, unauth.Body.String())
	}

	fresh.installCode = http.StatusTooManyRequests
	limitedState := beginInstall(t, freshSrv, ownerTok, orgID)
	limited := doJSON(t, freshSrv, http.MethodPost, "/api/v1/integrations/github/installations/complete", completeBody(779, "install", limitedState.State), ownerTok)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("rate limit status=%d body=%s", limited.Code, limited.Body.String())
	}

	fresh.installCode = 0
	fresh.repos = []map[string]any{
		repoJSON(1, "verified-octo/same", false, false, "public", "https://github.com/verified-octo/same.git"),
		repoJSON(2, "verified-octo/same", false, false, "public", "https://github.com/verified-octo/same.git"),
	}
	conflictState := beginInstall(t, freshSrv, ownerTok, orgID)
	conflicted := doJSON(t, freshSrv, http.MethodPost, "/api/v1/integrations/github/installations/complete", completeBody(780, "install", conflictState.State), ownerTok)
	if conflicted.Code != http.StatusServiceUnavailable {
		t.Fatalf("reconcile status=%d body=%s", conflicted.Code, conflicted.Body.String())
	}
	var reconcileStatus string
	var reconcileSync *time.Time
	if err := pool.QueryRow(context.Background(), `
		SELECT status, last_sync_at FROM git_connections WHERE installation_id = 780 AND deleted_at IS NULL`,
	).Scan(&reconcileStatus, &reconcileSync); err != nil {
		t.Fatal(err)
	}
	if reconcileStatus != "error" || reconcileSync != nil {
		t.Fatalf("reconcile status=%s sync=%v", reconcileStatus, reconcileSync)
	}
	var stored int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM git_repositories r
		JOIN git_connections c ON c.id = r.connection_id
		WHERE c.installation_id = 780`).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("stored=%d err=%v", stored, err)
	}
}

type installBegin struct {
	URL   string
	State string
	Raw   string
}

type createdInstall struct {
	ID string
}

type repoItem struct {
	ExternalID string         `json:"externalId"`
	FullName   string         `json:"fullName"`
	CloneURL   string         `json:"cloneUrl"`
	Metadata   map[string]any `json:"metadata"`
}

type phase2App struct {
	cfg       config.Config
	keyMarker string
	publicKey any
}

func beginInstall(t *testing.T, srv *server.Server, token, orgID string) installBegin {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations", []byte(`{"organizationId":"`+orgID+`"}`), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("begin status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		InstallationURL string `json:"installationUrl"`
	}
	decode(t, rec, &out)
	parsed, err := url.Parse(out.InstallationURL)
	if err != nil {
		t.Fatal(err)
	}
	return installBegin{URL: out.InstallationURL, State: parsed.Query().Get("state"), Raw: rec.Body.String()}
}

func completeInstall(t *testing.T, srv *server.Server, token string, installationID int64, action, state string) createdInstall {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/integrations/github/installations/complete", completeBody(installationID, action, state), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("complete status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Connection struct {
			ID string `json:"id"`
		} `json:"connection"`
	}
	decode(t, rec, &out)
	return createdInstall{ID: out.Connection.ID}
}

func completeBody(installationID int64, action, state string) []byte {
	body, _ := json.Marshal(map[string]any{"installationId": installationID, "setupAction": action, "state": state})
	return body
}

func listRepos(t *testing.T, srv *server.Server, token, connectionID string) []repoItem {
	t.Helper()
	rec := doJSON(t, srv, http.MethodGet, "/api/v1/integrations/git/connections/"+connectionID+"/repositories", nil, token)
	if rec.Code != http.StatusOK {
		t.Fatalf("list repos status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Items []repoItem `json:"items"`
	}
	decode(t, rec, &out)
	return out.Items
}

func assertStateOpen(t *testing.T, pool *pgxpool.Pool, state string) {
	t.Helper()
	var consumed *time.Time
	if err := pool.QueryRow(context.Background(), `
		SELECT consumed_at FROM github_app_install_states WHERE state_hash = $1`, sha256Hex(state)).Scan(&consumed); err != nil {
		t.Fatal(err)
	}
	if consumed != nil {
		t.Fatal("installation state was consumed")
	}
}

func assertCredentialColumnsNull(t *testing.T, pool *pgxpool.Pool, connectionID string) {
	t.Helper()
	var cipher, nonce, keyID *string
	var authMode string
	if err := pool.QueryRow(context.Background(), `
		SELECT credential_ciphertext::text, credential_nonce::text, credential_key_id, auth_mode
		FROM git_connections WHERE id = $1`, connectionID).Scan(&cipher, &nonce, &keyID, &authMode); err != nil {
		t.Fatal(err)
	}
	if cipher != nil || nonce != nil || keyID != nil || authMode != "github_app" {
		t.Fatalf("credentials cipher=%v nonce=%v key=%v mode=%s", cipher, nonce, keyID, authMode)
	}
}

func assertNoSecretMaterial(t *testing.T, pool *pgxpool.Pool, logs, keyMarker string) {
	t.Helper()
	var auditText, repoText string
	if err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(string_agg(COALESCE(after_metadata::text, '') || COALESCE(before_metadata::text, ''), ','), '')
		FROM audit_logs`).Scan(&auditText); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(context.Background(), `
		SELECT COALESCE(string_agg(clone_url || html_url || metadata::text, ','), '')
		FROM git_repositories`).Scan(&repoText); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{installTokenMarker, cloneUserMarker, testWebhookMarker, "PRIVATE KEY", keyMarker} {
		if strings.Contains(auditText, secret) || strings.Contains(repoText, secret) || strings.Contains(logs, secret) {
			t.Fatalf("secret material %q leaked", secret)
		}
	}
}

func assertGitHubJWT(t *testing.T, tokens []string, publicKey any) {
	t.Helper()
	if len(tokens) == 0 {
		t.Fatal("GitHub was not called with an app jwt")
	}
	parsed, err := jwt.ParseWithClaims(tokens[0], &jwt.RegisteredClaims{}, func(token *jwt.Token) (any, error) {
		if token.Method != jwt.SigningMethodRS256 {
			t.Fatalf("alg=%v", token.Header["alg"])
		}
		return publicKey, nil
	})
	if err != nil || !parsed.Valid {
		t.Fatalf("jwt: %v", err)
	}
	claims := parsed.Claims.(*jwt.RegisteredClaims)
	if claims.Issuer != "5150" || claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time) > 10*time.Minute {
		t.Fatalf("claims iss=%s lifetime=%s", claims.Issuer, claims.ExpiresAt.Time.Sub(claims.IssuedAt.Time))
	}
}

func loadPhase2App(t *testing.T) phase2App {
	t.Helper()
	keyFile := writeGitHubAppKey(t)
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_URL", "postgres://localhost/deploycore?sslmode=disable")
	t.Setenv("AUTH_TOKEN_SECRET", "test-secret-0123456789abcdef01234567")
	t.Setenv("SECRETS_PLATFORM_KEY", "")
	t.Setenv("GITHUB_APP_ID", "5150")
	t.Setenv("GITHUB_APP_SLUG", "deploycore")
	t.Setenv("GITHUB_APP_PRIVATE_KEY_FILE", keyFile.path)
	t.Setenv("GITHUB_APP_WEBHOOK_SECRET", testWebhookMarker)
	t.Setenv("GITHUB_APP_SETUP_BASE_URL", "https://deploycore.example.com")
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	pemBytes, err := os.ReadFile(keyFile.path)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(pemBytes)
	private, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	return phase2App{cfg: loaded, keyMarker: keyFile.marker, publicKey: &private.PublicKey}
}

func phase2Server(t *testing.T, pool *pgxpool.Pool, gitHubBase string, logs io.Writer, loaded config.Config) *server.Server {
	t.Helper()
	platformKey, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b16-gitproviders"))
	return server.New(config.Config{
		Env:                   "test",
		AuthTokenSecret:       "test-secret-0123456789abcdef01234567",
		AccessTokenTTL:        time.Minute,
		RefreshTokenTTL:       time.Hour,
		AuthRateLimitPerMin:   1000,
		CORSAllowedOrigins:    []string{"*"},
		AuthMinPasswordLength: 8,
		SecretsPlatformKey:    platformKey,
		SecretsKeyID:          "platform:v1",
		GitHubApp:             loaded.GitHubApp,
		GitHubAPIBaseURL:      gitHubBase,
	}, slog.New(slog.NewTextHandler(logs, nil)), pool)
}

func repoJSON(id int, fullName string, private, archived bool, visibility, cloneURL string) map[string]any {
	return map[string]any{
		"id": id, "full_name": fullName, "private": private, "archived": archived,
		"visibility": visibility, "default_branch": "main", "clone_url": cloneURL,
		"html_url": "https://github.com/" + fullName,
	}
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

type fakeGitHub struct {
	mu          sync.Mutex
	server      *httptest.Server
	appID       int64
	account     string
	accountID   int64
	accountType string
	selection   string
	repos       []map[string]any
	pageSize    int
	failPage    int
	installCode int
	suspended   bool
	token       string
	seenJWT     []string
	requests    int
}

func newFakeGitHub(t *testing.T) *fakeGitHub {
	t.Helper()
	f := &fakeGitHub{
		appID: 5150, account: "verified-octo", accountID: 77, accountType: "Organization",
		selection: "selected", pageSize: 100, token: installTokenMarker,
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeGitHub) URL() string { return f.server.URL }

func (f *fakeGitHub) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests
}

func (f *fakeGitHub) jwts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.seenJWT))
	copy(out, f.seenJWT)
	return out
}

func (f *fakeGitHub) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests++
	bearer := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if strings.HasPrefix(r.URL.Path, "/app/") && strings.Count(bearer, ".") == 2 {
		f.seenJWT = append(f.seenJWT, bearer)
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/app/installations/") && !strings.HasSuffix(r.URL.Path, "/access_tokens"):
		if f.installCode != 0 {
			w.WriteHeader(f.installCode)
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/app/installations/")
		suspended := "null"
		if f.suspended {
			suspended = `"2026-10-04T12:00:00Z"`
		}
		_, _ = w.Write([]byte(`{"id":` + id + `,"app_id":` + strconv.FormatInt(f.appID, 10) + `,"repository_selection":"` + f.selection + `","suspended_at":` + suspended + `,"account":{"login":"` + f.account + `","id":` + strconv.FormatInt(f.accountID, 10) + `,"type":"` + f.accountType + `"}}`))
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/access_tokens"):
		_, _ = w.Write([]byte(`{"token":"` + f.token + `","expires_at":"2026-10-04T13:00:00Z"}`))
	case r.URL.Path == "/installation/repositories":
		if bearer != f.token {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			page = 1
		}
		if f.failPage != 0 && page == f.failPage {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		size := f.pageSize
		if size <= 0 {
			size = len(f.repos)
		}
		start := (page - 1) * size
		if start > len(f.repos) {
			start = len(f.repos)
		}
		end := start + size
		if end > len(f.repos) {
			end = len(f.repos)
		}
		if end < len(f.repos) {
			w.Header().Set("Link", `<http://`+r.Host+`/installation/repositories?per_page=100&page=`+strconv.Itoa(page+1)+`>; rel="next"`)
		}
		payload := map[string]any{"total_count": len(f.repos), "repositories": f.repos[start:end]}
		raw, _ := json.Marshal(payload)
		_, _ = w.Write(raw)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}
