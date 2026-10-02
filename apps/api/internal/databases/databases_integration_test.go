package databases_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/auth"
	"github.com/deploycore/deploy-core/apps/api/internal/config"
	"github.com/deploycore/deploy-core/apps/api/internal/databases"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/internal/server"
	"github.com/deploycore/deploy-core/apps/api/pkg/crypto"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b22_test?sslmode=disable"
	}
	ctx := context.Background()
	pool, err := db.NewPool(ctx, url)
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	t.Cleanup(pool.Close)
	if err := db.NewMigrator(pool).Up(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := rbac.EnsureSeeded(ctx, pool); err != nil {
		t.Fatalf("rbac seed: %v", err)
	}
	_, _ = pool.Exec(ctx, `
		DELETE FROM managed_databases;
		DELETE FROM agent_commands;
		DELETE FROM application_configs;
		DELETE FROM applications;
		DELETE FROM environments;
		DELETE FROM projects;
		DELETE FROM server_heartbeats;
		DELETE FROM server_agents;
		DELETE FROM servers;
		DELETE FROM organization_invitation_roles;
		DELETE FROM organization_invitations;
		DELETE FROM member_roles;
		DELETE FROM organization_members;
		TRUNCATE audit_logs;
		DELETE FROM password_reset_tokens;
		DELETE FROM sessions;
		DELETE FROM organizations;
		DELETE FROM users;`)
	return pool
}

func testServer(t *testing.T, pool *pgxpool.Pool) *server.Server {
	t.Helper()
	key, _ := crypto.NormalizePlatformKey([]byte("test-platform-key-for-b22-databases"))
	cfg := config.Config{
		Env:                   "test",
		AuthTokenSecret:       "test-secret-0123456789abcdef01234567",
		AccessTokenTTL:        time.Minute,
		RefreshTokenTTL:       time.Hour,
		AuthRateLimitPerMin:   1000,
		CORSAllowedOrigins:    []string{"*"},
		AuthMinPasswordLength: 8,
		SecretsPlatformKey:    key,
		SecretsKeyID:          "platform:v1",
		AgentRegistrationTTL:  15 * time.Minute,
		AgentHeartbeatRetain:  50,
		JobWorkerEnabled:      false,
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return server.New(cfg, log, pool)
}

func TestManagedDatabaseProvisionFlow(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)

	ownerTok := register(t, srv, "owner-b22-"+uuid.NewString()+"@example.com", "password123", "Owner")
	orgID := createOrg(t, srv, ownerTok, "B22 Org", "b22-org-"+uuid.NewString()[:8])
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	serverID := createServer(t, srv, ownerTok, orgID)
	agentCred := registerAgent(t, srv, ownerTok, serverID)

	cpu := 500
	mem := int64(512 * 1024 * 1024)
	create := doJSON(t, srv, http.MethodPost, "/api/v1/databases", mustJSON(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"serverId": serverID, "name": "Primary PG", "engine": "postgresql", "engineVersion": "16",
		"databaseName": "appdb", "username": "appuser", "password": "supersecret1",
		"cpuMillis": cpu, "memoryBytes": mem,
		"backupPolicy": map[string]any{"enabled": true, "retentionDays": 14},
	}), ownerTok)
	if create.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Database struct {
			ID                 string  `json:"id"`
			Status             string  `json:"status"`
			StorageVolumeName  string  `json:"storageVolumeName"`
			VolumeProtected    bool    `json:"volumeProtected"`
			HasCredential      bool    `json:"hasCredential"`
			ProvisionCommandID *string `json:"provisionCommandId"`
		} `json:"database"`
	}
	decode(t, create, &created)
	if created.Database.Status != databases.StatusProvisioning {
		t.Fatalf("status=%s", created.Database.Status)
	}
	if !created.Database.VolumeProtected || !created.Database.HasCredential {
		t.Fatalf("db=%#v", created.Database)
	}
	if created.Database.ProvisionCommandID == nil {
		t.Fatal("expected provisionCommandId")
	}

	// Password must not appear in agent command payload.
	var payload []byte
	if err := pool.QueryRow(context.Background(), `
		SELECT payload FROM agent_commands WHERE id = $1`, *created.Database.ProvisionCommandID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(payload, []byte("supersecret1")) {
		t.Fatalf("password leaked into command payload: %s", payload)
	}
	var commandPayload map[string]any
	if err := json.Unmarshal(payload, &commandPayload); err != nil {
		t.Fatal(err)
	}
	var projectSlug, envSlug string
	if err := pool.QueryRow(context.Background(), `
		SELECT p.slug, e.slug
		FROM environments e
		JOIN projects p ON p.id = e.project_id
		WHERE e.id = $1`, envID).Scan(&projectSlug, &envSlug); err != nil {
		t.Fatal(err)
	}
	wantNetwork, err := protocol.FormatPrivateNetworkName(projectSlug, envSlug)
	if err != nil {
		t.Fatal(err)
	}
	if commandPayload["networkName"] != wantNetwork {
		t.Fatalf("networkName=%v want %s", commandPayload["networkName"], wantNetwork)
	}
	if commandPayload["dnsAlias"] != "db-primary-pg" {
		t.Fatalf("dnsAlias=%v", commandPayload["dnsAlias"])
	}
	for _, key := range []string{"projectId", "projectSlug", "environmentId", "environmentSlug", "organizationId"} {
		if stringsTrim(commandPayload[key]) == "" {
			t.Fatalf("payload missing %s: %s", key, payload)
		}
	}
	if _, ok := commandPayload["password"]; ok {
		t.Fatal("password key present in provision payload")
	}

	boot := doJSON(t, srv, http.MethodGet, "/api/v1/agents/databases/"+created.Database.ID+"/bootstrap", nil, agentCred)
	if boot.Code != http.StatusOK {
		t.Fatalf("bootstrap status=%d body=%s", boot.Code, boot.Body.String())
	}
	var bootBody struct {
		Password          string `json:"password"`
		StorageVolumeName string `json:"storageVolumeName"`
	}
	decode(t, boot, &bootBody)
	if bootBody.Password != "supersecret1" {
		t.Fatalf("password=%q", bootBody.Password)
	}

	// Complete provision via agent command status.
	statusBody := mustJSON(map[string]any{
		"status": "completed",
		"result": map[string]any{"containerRuntimeId": "ctr-pg-1"},
	})
	done := doJSON(t, srv, http.MethodPost, "/api/v1/agents/commands/"+*created.Database.ProvisionCommandID+"/status", statusBody, agentCred)
	if done.Code != http.StatusOK {
		t.Fatalf("command status=%d body=%s", done.Code, done.Body.String())
	}

	get := doJSON(t, srv, http.MethodGet, "/api/v1/databases/"+created.Database.ID, nil, ownerTok)
	if get.Code != http.StatusOK {
		t.Fatalf("get status=%d body=%s", get.Code, get.Body.String())
	}
	var got struct {
		Database struct {
			Status             string  `json:"status"`
			ContainerRuntimeID *string `json:"containerRuntimeId"`
			PrivateHost        string  `json:"privateHost"`
			Port               int     `json:"port"`
			DatabaseName       string  `json:"databaseName"`
			Username           string  `json:"username"`
			Password           string  `json:"password"`
		} `json:"database"`
	}
	decode(t, get, &got)
	if got.Database.Status != databases.StatusRunning {
		t.Fatalf("status=%s", got.Database.Status)
	}
	if got.Database.PrivateHost != "db-primary-pg" || got.Database.Port != 5432 {
		t.Fatalf("connection metadata host=%s port=%d", got.Database.PrivateHost, got.Database.Port)
	}
	if got.Database.DatabaseName != "appdb" || got.Database.Username != "appuser" {
		t.Fatalf("identity name=%s user=%s", got.Database.DatabaseName, got.Database.Username)
	}
	if got.Database.Password != "" || bytes.Contains(get.Body.Bytes(), []byte("supersecret1")) {
		t.Fatal("detail response exposed the password")
	}
	if got.Database.ContainerRuntimeID == nil || *got.Database.ContainerRuntimeID != "ctr-pg-1" {
		t.Fatalf("runtime=%v", got.Database.ContainerRuntimeID)
	}

	list := doJSON(t, srv, http.MethodGet, "/api/v1/databases?organizationId="+orgID, nil, ownerTok)
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	if bytes.Contains(list.Body.Bytes(), []byte("supersecret1")) {
		t.Fatal("list response exposed the password")
	}
	if !bytes.Contains(list.Body.Bytes(), []byte(`"privateHost":"db-primary-pg"`)) && !bytes.Contains(list.Body.Bytes(), []byte(`"privateHost": "db-primary-pg"`)) {
		t.Fatalf("list missing privateHost: %s", list.Body.String())
	}
	if !bytes.Contains(list.Body.Bytes(), []byte(`"port":5432`)) && !bytes.Contains(list.Body.Bytes(), []byte(`"port": 5432`)) {
		t.Fatalf("list missing port: %s", list.Body.String())
	}

	clash := doJSON(t, srv, http.MethodPost, "/api/v1/databases", mustJSON(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"serverId": serverID, "name": "primary-pg", "engine": "postgresql",
		"databaseName": "otherdb", "username": "otheruser", "password": "supersecret1",
	}), ownerTok)
	if clash.Code != http.StatusConflict {
		t.Fatalf("alias collision status=%d body=%s", clash.Code, clash.Body.String())
	}

	if _, err := pool.Exec(context.Background(), `
		INSERT INTO applications (organization_id, project_id, environment_id, name, slug, type, status)
		VALUES ($1, $2, $3, 'App', 'db-billing', 'API', 'draft')`, orgID, projectID, envID); err != nil {
		t.Fatal(err)
	}
	appClash := doJSON(t, srv, http.MethodPost, "/api/v1/databases", mustJSON(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"serverId": serverID, "name": "billing", "engine": "postgresql",
		"databaseName": "billingdb", "username": "billinguser", "password": "supersecret1",
	}), ownerTok)
	if appClash.Code != http.StatusConflict {
		t.Fatalf("application alias collision status=%d body=%s", appClash.Code, appClash.Body.String())
	}

	reveal := doJSON(t, srv, http.MethodPost, "/api/v1/databases/"+created.Database.ID+"/credentials/reveal", nil, ownerTok)
	if reveal.Code != http.StatusOK {
		t.Fatalf("reveal status=%d body=%s", reveal.Code, reveal.Body.String())
	}

	del := doJSON(t, srv, http.MethodDelete, "/api/v1/databases/"+created.Database.ID, nil, ownerTok)
	if del.Code != http.StatusOK {
		t.Fatalf("delete status=%d body=%s", del.Code, del.Body.String())
	}
	var delBody struct {
		RuntimeStopped bool   `json:"runtimeStopped"`
		VolumeDeleted  bool   `json:"volumeDeleted"`
		Message        string `json:"message"`
	}
	decode(t, del, &delBody)
	if delBody.RuntimeStopped || delBody.VolumeDeleted {
		t.Fatalf("delete must not claim runtime/volume destruction: %+v", delBody)
	}
	// Volume name row soft-deleted; protected volume must not be auto-removed (no volume table yet).
	var vol string
	var status string
	err = pool.QueryRow(context.Background(), `
		SELECT storage_volume_name, status FROM managed_databases WHERE id = $1`, created.Database.ID).
		Scan(&vol, &status)
	if err != nil {
		t.Fatal(err)
	}
	if status != databases.StatusDeleted || vol == "" {
		t.Fatalf("after delete status=%s vol=%s", status, vol)
	}

	_ = protocol.OpProvisionDatabase
}

func TestManagedDatabaseProvisionRetry(t *testing.T) {
	pool := testPool(t)
	srv := testServer(t, pool)
	ctx := context.Background()

	ownerTok := register(t, srv, "owner-retry-"+uuid.NewString()+"@example.com", "password123", "Owner")
	viewerEmail := "viewer-retry-" + uuid.NewString() + "@example.com"
	viewerTok := register(t, srv, viewerEmail, "password123", "Viewer")
	outsiderTok := register(t, srv, "outsider-retry-"+uuid.NewString()+"@example.com", "password123", "Outsider")
	orgID := createOrg(t, srv, ownerTok, "Retry Org", "retry-org-"+uuid.NewString()[:8])
	_ = createOrg(t, srv, outsiderTok, "Other Org", "other-org-"+uuid.NewString()[:8])
	inviteDatabaseViewer(t, srv, ownerTok, viewerTok, orgID, viewerEmail)
	projectID := createProject(t, srv, ownerTok, orgID)
	envID := createEnvironment(t, srv, ownerTok, projectID)
	serverID := createServer(t, srv, ownerTok, orgID)
	agentCred := registerAgent(t, srv, ownerTok, serverID)

	const password = "retry-secret-1"
	created := createManagedDatabase(t, srv, ownerTok, orgID, projectID, envID, serverID, "Retry PG", "retrydb", "retryuser", password)
	failDatabaseCommand(t, srv, agentCred, created.commandID)

	var credentialBefore []byte
	var volumeBefore string
	var protected bool
	var attached string
	err := pool.QueryRow(ctx, `
		SELECT d.credential_ciphertext, v.id::text, v.protected, v.attached_resource_id::text
		FROM managed_databases d
		JOIN volumes v ON v.attached_resource_type = 'database'
		  AND v.attached_resource_id = d.id
		  AND v.deleted_at IS NULL
		WHERE d.id = $1`, created.id).Scan(&credentialBefore, &volumeBefore, &protected, &attached)
	if err != nil {
		t.Fatal(err)
	}
	if !protected || attached != created.id || len(credentialBefore) == 0 {
		t.Fatalf("volume=%s protected=%v attached=%s", volumeBefore, protected, attached)
	}

	denied := doJSON(t, srv, http.MethodPost, "/api/v1/databases/"+created.id+"/retry", nil, viewerTok)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("viewer retry status=%d body=%s", denied.Code, denied.Body.String())
	}
	outsider := doJSON(t, srv, http.MethodPost, "/api/v1/databases/"+created.id+"/retry", nil, outsiderTok)
	if outsider.Code != http.StatusForbidden {
		t.Fatalf("outsider retry status=%d body=%s", outsider.Code, outsider.Body.String())
	}
	if countProvisionCommands(t, pool, created.id) != 1 {
		t.Fatal("denied retries must not issue a command")
	}

	retry := doJSON(t, srv, http.MethodPost, "/api/v1/databases/"+created.id+"/retry", nil, ownerTok)
	if retry.Code != http.StatusOK {
		t.Fatalf("retry status=%d body=%s", retry.Code, retry.Body.String())
	}
	var retried struct {
		Database struct {
			ID                 string  `json:"id"`
			Status             string  `json:"status"`
			StorageVolumeName  string  `json:"storageVolumeName"`
			VolumeProtected    bool    `json:"volumeProtected"`
			ProvisionCommandID *string `json:"provisionCommandId"`
			LastError          string  `json:"lastError"`
			ContainerRuntimeID *string `json:"containerRuntimeId"`
			PrivateHost        string  `json:"privateHost"`
		} `json:"database"`
	}
	decode(t, retry, &retried)
	if retried.Database.ID != created.id {
		t.Fatalf("id changed from %s to %s", created.id, retried.Database.ID)
	}
	if retried.Database.Status != databases.StatusProvisioning {
		t.Fatalf("status=%s", retried.Database.Status)
	}
	if retried.Database.ProvisionCommandID == nil || *retried.Database.ProvisionCommandID == created.commandID {
		t.Fatalf("command=%v previous=%s", retried.Database.ProvisionCommandID, created.commandID)
	}
	if retried.Database.LastError != "" || retried.Database.ContainerRuntimeID != nil {
		t.Fatalf("error=%q runtime=%v", retried.Database.LastError, retried.Database.ContainerRuntimeID)
	}
	if !retried.Database.VolumeProtected || retried.Database.StorageVolumeName != created.volume || retried.Database.PrivateHost != "db-retry-pg" {
		t.Fatalf("db=%+v", retried.Database)
	}
	if bytes.Contains(retry.Body.Bytes(), []byte(password)) {
		t.Fatal("retry response exposed the password")
	}

	var payload []byte
	var oldStatus string
	if err := pool.QueryRow(ctx, `SELECT payload FROM agent_commands WHERE id = $1`, *retried.Database.ProvisionCommandID).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT status FROM agent_commands WHERE id = $1`, created.commandID).Scan(&oldStatus); err != nil {
		t.Fatal(err)
	}
	if oldStatus != "failed" {
		t.Fatalf("previous command status=%s", oldStatus)
	}
	if bytes.Contains(payload, []byte(password)) {
		t.Fatalf("password leaked into retry payload: %s", payload)
	}
	var commandPayload map[string]any
	if err := json.Unmarshal(payload, &commandPayload); err != nil {
		t.Fatal(err)
	}
	if _, ok := commandPayload["password"]; ok {
		t.Fatal("password key present in retry payload")
	}
	var projectSlug, envSlug string
	if err := pool.QueryRow(ctx, `
		SELECT p.slug, e.slug FROM environments e
		JOIN projects p ON p.id = e.project_id WHERE e.id = $1`, envID).Scan(&projectSlug, &envSlug); err != nil {
		t.Fatal(err)
	}
	wantNetwork, err := protocol.FormatPrivateNetworkName(projectSlug, envSlug)
	if err != nil {
		t.Fatal(err)
	}
	if commandPayload["networkName"] != wantNetwork || commandPayload["dnsAlias"] != "db-retry-pg" {
		t.Fatalf("network=%v alias=%v", commandPayload["networkName"], commandPayload["dnsAlias"])
	}
	for _, key := range []string{"projectId", "projectSlug", "environmentId", "environmentSlug", "organizationId", "databaseId", "storageVolumeName"} {
		if stringsTrim(commandPayload[key]) == "" {
			t.Fatalf("payload missing %s: %s", key, payload)
		}
	}
	if commandPayload["databaseId"] != created.id || commandPayload["storageVolumeName"] != created.volume {
		t.Fatalf("payload identity changed: %s", payload)
	}

	var credentialAfter []byte
	var volumeAfter string
	var rows int
	err = pool.QueryRow(ctx, `
		SELECT d.credential_ciphertext, v.id::text,
		       (SELECT COUNT(*) FROM managed_databases WHERE id = $1)
		FROM managed_databases d
		JOIN volumes v ON v.attached_resource_type = 'database'
		  AND v.attached_resource_id = d.id
		  AND v.deleted_at IS NULL
		  AND v.protected = TRUE
		WHERE d.id = $1`, created.id).Scan(&credentialAfter, &volumeAfter, &rows)
	if err != nil {
		t.Fatal(err)
	}
	if rows != 1 || volumeAfter != volumeBefore || !bytes.Equal(credentialBefore, credentialAfter) {
		t.Fatalf("rows=%d volume %s→%s credential preserved=%v", rows, volumeBefore, volumeAfter, bytes.Equal(credentialBefore, credentialAfter))
	}

	var beforeMeta, afterMeta []byte
	if err := pool.QueryRow(ctx, `
		SELECT before_metadata, after_metadata FROM audit_logs
		WHERE action = 'database.provision.retry' AND resource_id = $1`, created.id).Scan(&beforeMeta, &afterMeta); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(beforeMeta, []byte(password)) || bytes.Contains(afterMeta, []byte(password)) || bytes.Contains(bytes.ToLower(afterMeta), []byte("password")) {
		t.Fatalf("audit leaked credential material: before=%s after=%s", beforeMeta, afterMeta)
	}
	if !bytes.Contains(afterMeta, []byte(*retried.Database.ProvisionCommandID)) || !bytes.Contains(beforeMeta, []byte(created.commandID)) {
		t.Fatalf("audit missing command ids: before=%s after=%s", beforeMeta, afterMeta)
	}

	again := doJSON(t, srv, http.MethodPost, "/api/v1/databases/"+created.id+"/retry", nil, ownerTok)
	if again.Code != http.StatusConflict {
		t.Fatalf("provisioning retry status=%d body=%s", again.Code, again.Body.String())
	}
	for _, status := range []string{databases.StatusPending, databases.StatusRunning} {
		if _, err := pool.Exec(ctx, `
			UPDATE managed_databases
			SET status = $2, container_runtime_id = NULL, last_error = 'held'
			WHERE id = $1`, created.id, status); err != nil {
			t.Fatal(err)
		}
		rec := doJSON(t, srv, http.MethodPost, "/api/v1/databases/"+created.id+"/retry", nil, ownerTok)
		if rec.Code != http.StatusConflict {
			t.Fatalf("status %s retry=%d body=%s", status, rec.Code, rec.Body.String())
		}
	}
	if _, err := pool.Exec(ctx, `
		UPDATE managed_databases
		SET status = 'FAILED', container_runtime_id = 'ctr-already', last_error = 'has runtime'
		WHERE id = $1`, created.id); err != nil {
		t.Fatal(err)
	}
	withRuntime := doJSON(t, srv, http.MethodPost, "/api/v1/databases/"+created.id+"/retry", nil, ownerTok)
	if withRuntime.Code != http.StatusConflict {
		t.Fatalf("runtime retry status=%d body=%s", withRuntime.Code, withRuntime.Body.String())
	}
	commandsBeforeRace := countProvisionCommands(t, pool, created.id)
	if _, err := pool.Exec(ctx, `
		UPDATE managed_databases
		SET status = 'FAILED', container_runtime_id = NULL, last_error = 'race'
		WHERE id = $1`, created.id); err != nil {
		t.Fatal(err)
	}

	const racers = 8
	codes := make([]int, racers)
	var wg sync.WaitGroup
	wg.Add(racers)
	for i := 0; i < racers; i++ {
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/v1/databases/"+created.id+"/retry", nil)
			req.Header.Set("Authorization", "Bearer "+ownerTok)
			rec := httptest.NewRecorder()
			srv.HTTPHandler().ServeHTTP(rec, req)
			codes[i] = rec.Code
		}(i)
	}
	wg.Wait()
	okCount := 0
	for _, code := range codes {
		switch code {
		case http.StatusOK:
			okCount++
		case http.StatusConflict:
		default:
			t.Fatalf("concurrent retry codes=%v", codes)
		}
	}
	if okCount != 1 {
		t.Fatalf("concurrent successes=%d codes=%v", okCount, codes)
	}
	if got := countProvisionCommands(t, pool, created.id); got != commandsBeforeRace+1 {
		t.Fatalf("provision commands=%d want %d", got, commandsBeforeRace+1)
	}

	otherProject := createProject(t, srv, ownerTok, orgID)
	moved := createManagedDatabase(t, srv, ownerTok, orgID, projectID, envID, serverID, "Moved PG", "moveddb", "moveduser", password)
	failDatabaseCommand(t, srv, agentCred, moved.commandID)
	if _, err := pool.Exec(ctx, `UPDATE managed_databases SET project_id = $2 WHERE id = $1`, moved.id, otherProject); err != nil {
		t.Fatal(err)
	}
	beforeMove := countProvisionCommands(t, pool, moved.id)
	mismatch := doJSON(t, srv, http.MethodPost, "/api/v1/databases/"+moved.id+"/retry", nil, ownerTok)
	if mismatch.Code != http.StatusConflict {
		t.Fatalf("mismatch retry status=%d body=%s", mismatch.Code, mismatch.Body.String())
	}
	if countProvisionCommands(t, pool, moved.id) != beforeMove {
		t.Fatal("mismatched placement issued a command")
	}

	otherServer := createServer(t, srv, ownerTok, orgID)
	otherAgent := registerAgent(t, srv, ownerTok, otherServer)
	gone := createManagedDatabase(t, srv, ownerTok, orgID, projectID, envID, otherServer, "Gone Server PG", "gonedb", "goneuser", password)
	failDatabaseCommand(t, srv, otherAgent, gone.commandID)
	if _, err := pool.Exec(ctx, `UPDATE servers SET deleted_at = NOW() WHERE id = $1`, otherServer); err != nil {
		t.Fatal(err)
	}
	missingServer := doJSON(t, srv, http.MethodPost, "/api/v1/databases/"+gone.id+"/retry", nil, ownerTok)
	if missingServer.Code != http.StatusNotFound {
		t.Fatalf("missing server retry status=%d body=%s", missingServer.Code, missingServer.Body.String())
	}

	broken := createManagedDatabase(t, srv, ownerTok, orgID, projectID, envID, serverID, "Broken Alias PG", "brokendb", "brokenuser", password)
	failDatabaseCommand(t, srv, agentCred, broken.commandID)
	if _, err := pool.Exec(ctx, `UPDATE managed_databases SET dns_alias = 'NOT_A_LABEL' WHERE id = $1`, broken.id); err != nil {
		t.Fatal(err)
	}
	beforeBroken := countProvisionCommands(t, pool, broken.id)
	brokenRetry := doJSON(t, srv, http.MethodPost, "/api/v1/databases/"+broken.id+"/retry", nil, ownerTok)
	if brokenRetry.Code != http.StatusInternalServerError {
		t.Fatalf("invalid alias retry status=%d body=%s", brokenRetry.Code, brokenRetry.Body.String())
	}
	var brokenStatus string
	if err := pool.QueryRow(ctx, `SELECT status FROM managed_databases WHERE id = $1`, broken.id).Scan(&brokenStatus); err != nil {
		t.Fatal(err)
	}
	if brokenStatus != databases.StatusFailed || countProvisionCommands(t, pool, broken.id) != beforeBroken {
		t.Fatalf("status=%s commands=%d want FAILED and %d", brokenStatus, countProvisionCommands(t, pool, broken.id), beforeBroken)
	}
}

type createdDatabase struct {
	id        string
	commandID string
	volume    string
}

func createManagedDatabase(t *testing.T, srv *server.Server, token, orgID, projectID, envID, serverID, name, dbName, username, password string) createdDatabase {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/databases", mustJSON(map[string]any{
		"organizationId": orgID, "projectId": projectID, "environmentId": envID,
		"serverId": serverID, "name": name, "engine": "postgresql", "engineVersion": "16",
		"databaseName": dbName, "username": username, "password": password,
	}), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create %s status=%d body=%s", name, rec.Code, rec.Body.String())
	}
	var created struct {
		Database struct {
			ID                 string  `json:"id"`
			StorageVolumeName  string  `json:"storageVolumeName"`
			ProvisionCommandID *string `json:"provisionCommandId"`
		} `json:"database"`
	}
	decode(t, rec, &created)
	if created.Database.ProvisionCommandID == nil {
		t.Fatal("expected provision command")
	}
	return createdDatabase{id: created.Database.ID, commandID: *created.Database.ProvisionCommandID, volume: created.Database.StorageVolumeName}
}

func failDatabaseCommand(t *testing.T, srv *server.Server, agentCred, commandID string) {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/agents/commands/"+commandID+"/status", mustJSON(map[string]any{
		"status":       "failed",
		"errorMessage": "image pull failed",
	}), agentCred)
	if rec.Code != http.StatusOK {
		t.Fatalf("fail command status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func countProvisionCommands(t *testing.T, pool *pgxpool.Pool, databaseID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM agent_commands
		WHERE operation = 'PROVISION_DATABASE' AND payload->>'databaseId' = $1`, databaseID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func inviteDatabaseViewer(t *testing.T, srv *server.Server, ownerTok, viewerTok, orgID, email string) {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/organizations/"+orgID+"/invitations", mustJSON(map[string]any{
		"email": email, "roleKeys": []string{"viewer"},
	}), ownerTok)
	if rec.Code != http.StatusCreated {
		t.Fatalf("invite status=%d body=%s", rec.Code, rec.Body.String())
	}
	var inv struct {
		Invitation struct {
			Token string `json:"token"`
		} `json:"invitation"`
	}
	decode(t, rec, &inv)
	acc := doJSON(t, srv, http.MethodPost, "/api/v1/invitations/accept", mustJSON(map[string]string{"token": inv.Invitation.Token}), viewerTok)
	if acc.Code != http.StatusOK {
		t.Fatalf("accept status=%d body=%s", acc.Code, acc.Body.String())
	}
}

func stringsTrim(v any) string {
	s, _ := v.(string)
	return s
}

func createServer(t *testing.T, srv *server.Server, token, orgID string) string {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/servers", mustJSON(map[string]any{
		"organizationId": orgID, "name": "db-host-" + uuid.NewString()[:8], "provider": "hetzner",
		"region": "fsn1", "hostname": "db-host-" + uuid.NewString()[:8] + ".local", "architecture": "amd64",
	}), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("server status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Server struct {
			ID string `json:"id"`
		} `json:"server"`
	}
	decode(t, rec, &out)
	return out.Server.ID
}

func registerAgent(t *testing.T, srv *server.Server, ownerTok, serverID string) string {
	t.Helper()
	tokRec := doJSON(t, srv, http.MethodPost, "/api/v1/servers/"+serverID+"/registration-token", nil, ownerTok)
	if tokRec.Code != http.StatusCreated {
		t.Fatalf("reg token status=%d body=%s", tokRec.Code, tokRec.Body.String())
	}
	var tokOut struct {
		RegistrationToken struct {
			Token string `json:"token"`
		} `json:"registrationToken"`
	}
	decode(t, tokRec, &tokOut)
	body, _ := json.Marshal(map[string]any{"registrationToken": tokOut.RegistrationToken.Token, "agentVersion": "1.0.0"})
	reg := doJSON(t, srv, http.MethodPost, "/api/v1/agents/register", body, "")
	if reg.Code != http.StatusCreated {
		t.Fatalf("register agent status=%d body=%s", reg.Code, reg.Body.String())
	}
	var out struct {
		Agent struct {
			Credential string `json:"credential"`
		} `json:"agent"`
	}
	decode(t, reg, &out)
	return out.Agent.Credential
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func register(t *testing.T, srv *server.Server, email, password, name string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password, "displayName": name})
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/auth/register", body, "")
	if rec.Code != http.StatusCreated {
		t.Fatalf("register %s status=%d body=%s", email, rec.Code, rec.Body.String())
	}
	var out struct {
		Tokens auth.TokenPair `json:"tokens"`
	}
	decode(t, rec, &out)
	return out.Tokens.AccessToken
}

func createOrg(t *testing.T, srv *server.Server, token, name, slug string) string {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/organizations", mustJSON(map[string]string{"name": name, "slug": slug}), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create org status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Organization struct {
			ID string `json:"id"`
		} `json:"organization"`
	}
	decode(t, rec, &out)
	return out.Organization.ID
}

func createProject(t *testing.T, srv *server.Server, token, orgID string) string {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/projects",
		mustJSON(map[string]string{"organizationId": orgID, "name": "Platform", "slug": "platform-" + uuid.NewString()[:8]}), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create project status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	decode(t, rec, &out)
	return out.Project.ID
}

func createEnvironment(t *testing.T, srv *server.Server, token, projectID string) string {
	t.Helper()
	rec := doJSON(t, srv, http.MethodPost, "/api/v1/projects/"+projectID+"/environments",
		mustJSON(map[string]string{"name": "Production", "slug": "production", "kind": "production"}), token)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create env status=%d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Environment struct {
			ID string `json:"id"`
		} `json:"environment"`
	}
	decode(t, rec, &out)
	return out.Environment.ID
}

func doJSON(t *testing.T, srv *server.Server, method, path string, body []byte, token string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	srv.HTTPHandler().ServeHTTP(rec, req)
	return rec
}

func decode(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode: %v body=%s", err, rec.Body.String())
	}
}
