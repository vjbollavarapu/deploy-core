package deployments_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
	"github.com/deploycore/deploy-core/apps/api/internal/platform/db"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func repoPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://localhost/deploycore_b33_deploy_repo_test?sslmode=disable"
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
	_, _ = pool.Exec(ctx, `
		DELETE FROM deployment_events;
		DELETE FROM jobs;
		DELETE FROM application_replicas;
		UPDATE deployments SET active_revision_id = NULL, target_revision_id = NULL;
		DELETE FROM revisions;
		DELETE FROM deployments;
		DELETE FROM application_configs;
		DELETE FROM applications;
		DELETE FROM environments;
		DELETE FROM projects;
		DELETE FROM servers;
		DELETE FROM organization_members;
		DELETE FROM organizations;
		DELETE FROM users;`)
	return pool
}

func seedOrgApp(t *testing.T, pool *pgxpool.Pool) (orgID, appID, envID, serverID, userID uuid.UUID) {
	t.Helper()
	ctx := context.Background()
	userID, orgID, projectID := uuid.New(), uuid.New(), uuid.New()
	envID, serverID, appID = uuid.New(), uuid.New(), uuid.New()
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, q, args...); err != nil {
			t.Fatalf("%v", err)
		}
	}
	mustExec(`INSERT INTO users (id, email, password_hash, display_name) VALUES ($1,$2,'x','U')`,
		userID, userID.String()+"@ex.com")
	mustExec(`INSERT INTO organizations (id, name, slug) VALUES ($1,'O',$2)`, orgID, "o-"+orgID.String()[:8])
	mustExec(`INSERT INTO projects (id, organization_id, name, slug) VALUES ($1,$2,'P','p')`, projectID, orgID)
	mustExec(`INSERT INTO environments (id, organization_id, project_id, name, slug) VALUES ($1,$2,$3,'E','e')`,
		envID, orgID, projectID)
	mustExec(`INSERT INTO servers (id, organization_id, name, provider, region, hostname, architecture, status)
		VALUES ($1,$2,'s','test','x','h','amd64','ONLINE')`, serverID, orgID)
	mustExec(`INSERT INTO applications (id, organization_id, project_id, environment_id, name, slug, type, target_server_id, status)
		VALUES ($1,$2,$3,$4,'a','a','API',$5,'ready')`, appID, orgID, projectID, envID, serverID)
	mustExec(`INSERT INTO application_configs (organization_id, application_id, version, source_type, image_reference)
		VALUES ($1,$2,1,'image','img:1')`, orgID, appID)
	return orgID, appID, envID, serverID, userID
}

func TestListIsolatesByOrganization(t *testing.T) {
	pool := repoPool(t)
	repo := deployments.NewPostgresRepository(pool)
	org1, app1, env1, srv1, user1 := seedOrgApp(t, pool)
	org2, app2, env2, srv2, user2 := seedOrgApp(t, pool)

	d1, err := repo.CreateQueued(context.Background(), org1, app1, env1, &srv1,
		deployments.TriggerManual, nil, nil, "iso-1", user1, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateQueued(context.Background(), org2, app2, env2, &srv2,
		deployments.TriggerManual, nil, nil, "iso-2", user2, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}

	items, total, err := repo.List(context.Background(), org1, nil, nil, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != d1.ID {
		t.Fatalf("org1 list=%#v total=%d", items, total)
	}
	for _, it := range items {
		if it.OrganizationID != org1 {
			t.Fatalf("leaked org %s", it.OrganizationID)
		}
	}
}

func TestTransitionInvalidDoesNotMutate(t *testing.T) {
	pool := repoPool(t)
	repo := deployments.NewPostgresRepository(pool)
	org, app, env, srv, user := seedOrgApp(t, pool)
	d, err := repo.CreateQueued(context.Background(), org, app, env, &srv,
		deployments.TriggerManual, nil, nil, "tx-1", user, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Transition(context.Background(), d.ID, deployments.StatusQueued, deployments.TransitionInput{
		ToStatus: deployments.StatusBuilding,
		Message:  "illegal jump",
	}, time.Now().UTC())
	if err != deployments.ErrInvalidTransition {
		t.Fatalf("want ErrInvalidTransition got %v", err)
	}
	got, err := repo.Get(context.Background(), d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != deployments.StatusQueued {
		t.Fatalf("status mutated to %s", got.Status)
	}
}

func TestTransitionOptimisticConflict(t *testing.T) {
	pool := repoPool(t)
	repo := deployments.NewPostgresRepository(pool)
	org, app, env, srv, user := seedOrgApp(t, pool)
	d, err := repo.CreateQueued(context.Background(), org, app, env, &srv,
		deployments.TriggerManual, nil, nil, "tx-2", user, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Transition(context.Background(), d.ID, deployments.StatusQueued, deployments.TransitionInput{
		ToStatus: deployments.StatusPreparing,
		Message:  "first",
	}, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Transition(context.Background(), d.ID, deployments.StatusQueued, deployments.TransitionInput{
		ToStatus: deployments.StatusPreparing,
		Message:  "stale",
	}, time.Now().UTC())
	if err != deployments.ErrConflict {
		t.Fatalf("want ErrConflict got %v", err)
	}
}

func TestIdempotencyKeyConstraint(t *testing.T) {
	pool := repoPool(t)
	repo := deployments.NewPostgresRepository(pool)
	org, app, env, srv, user := seedOrgApp(t, pool)
	key := "same-key"
	_, err := repo.CreateQueued(context.Background(), org, app, env, &srv,
		deployments.TriggerManual, &key, nil, "idemp-1", user, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.CreateQueued(context.Background(), org, app, env, &srv,
		deployments.TriggerManual, &key, nil, "idemp-2", user, time.Now().UTC())
	if err != deployments.ErrConflict {
		t.Fatalf("want conflict got %v", err)
	}
}

func TestRevisionNumberUniqueConstraint(t *testing.T) {
	pool := repoPool(t)
	org, app, env, srv, user := seedOrgApp(t, pool)
	_ = env
	_ = srv
	_ = user
	ctx := context.Background()
	_, err := pool.Exec(ctx, `
		INSERT INTO revisions (organization_id, application_id, revision_number, status, effective_config)
		VALUES ($1,$2,1,'CREATED','{}')`, org, app)
	if err != nil {
		t.Fatal(err)
	}
	_, err = pool.Exec(ctx, `
		INSERT INTO revisions (organization_id, application_id, revision_number, status, effective_config)
		VALUES ($1,$2,1,'CREATED','{}')`, org, app)
	if err == nil {
		t.Fatal("expected unique violation on revision_number")
	}
}
