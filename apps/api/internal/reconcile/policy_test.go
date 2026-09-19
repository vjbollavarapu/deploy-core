package reconcile_test

import (
	"testing"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/reconcile"
	"github.com/deploycore/deploy-core/apps/api/internal/replicas"
)

func TestBucketKeyStableWithinInterval(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 10, 0, time.UTC)
	a := reconcile.BucketKey(now, 30*time.Second)
	b := reconcile.BucketKey(now.Add(10*time.Second), 30*time.Second)
	if a != b {
		t.Fatalf("expected same bucket got %s vs %s", a, b)
	}
	c := reconcile.BucketKey(now.Add(40*time.Second), 30*time.Second)
	if a == c {
		t.Fatal("expected next bucket")
	}
}

func TestShouldRestartPolicies(t *testing.T) {
	failed := replicas.Replica{Status: replicas.StatusFailed}
	stoppedScale := replicas.Replica{Status: replicas.StatusStopped, LastError: "scale_down"}
	unhealthy := replicas.Replica{Status: replicas.StatusUnhealthy}
	exitOne := 1
	failedExit := replicas.Replica{Status: replicas.StatusStopped, ObservedExitCode: &exitOne}

	if !reconcile.ShouldRestartForTest("always", failed) {
		t.Fatal("always should restart failed")
	}
	if reconcile.ShouldRestartForTest("no", failed) {
		t.Fatal("no should not restart")
	}
	if reconcile.ShouldRestartForTest("unless-stopped", stoppedScale) {
		t.Fatal("unless-stopped should skip scale_down")
	}
	if !reconcile.ShouldRestartForTest("unless-stopped", unhealthy) {
		t.Fatal("unless-stopped should restart unhealthy")
	}
	if !reconcile.ShouldRestartForTest("on-failure", failedExit) {
		t.Fatal("on-failure should restart non-zero exit")
	}
	zero := 0
	cleanStop := replicas.Replica{Status: replicas.StatusStopped, ObservedExitCode: &zero}
	if reconcile.ShouldRestartForTest("on-failure", cleanStop) {
		t.Fatal("on-failure should not restart zero exit")
	}
}
