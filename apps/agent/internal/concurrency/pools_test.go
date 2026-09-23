package concurrency

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPoolManager_AcquireAndRelease(t *testing.T) {
	limits := Limits{
		MaxBuilds:       1,
		MaxDeployments:  2,
		MaxBackups:      1,
		MaxContainerOps: 5,
	}
	pm := NewPoolManager(limits)

	ctx := context.Background()

	// 1. Acquire single slot in MaxBuilds (limit 1)
	rel1, err := pm.Acquire(ctx, CategoryBuild)
	if err != nil {
		t.Fatalf("unexpected acquire error: %v", err)
	}
	if pm.ActiveCount(CategoryBuild) != 1 {
		t.Errorf("expected 1 active build worker, got %d", pm.ActiveCount(CategoryBuild))
	}

	// 2. Second acquire with timeout must fail
	timeoutCtx, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()

	_, err = pm.Acquire(timeoutCtx, CategoryBuild)
	if !errors.Is(err, ErrPoolSaturated) {
		t.Errorf("expected ErrPoolSaturated, got %v", err)
	}

	// 3. Release first slot and re-acquire
	rel1()
	if pm.ActiveCount(CategoryBuild) != 0 {
		t.Errorf("expected 0 active workers after release, got %d", pm.ActiveCount(CategoryBuild))
	}

	rel2, err := pm.Acquire(ctx, CategoryBuild)
	if err != nil {
		t.Fatalf("expected successful acquire after release: %v", err)
	}
	rel2()
}

func TestPoolManager_MultipleCategories(t *testing.T) {
	pm := NewPoolManager(Limits{
		MaxBuilds:      1,
		MaxDeployments: 2,
	})

	ctx := context.Background()
	relBuild, err := pm.Acquire(ctx, CategoryBuild)
	if err != nil {
		t.Fatal(err)
	}
	defer relBuild()

	// CategoryDeployment should still have free slots
	relDep1, err := pm.Acquire(ctx, CategoryDeployment)
	if err != nil {
		t.Fatal(err)
	}
	defer relDep1()

	relDep2, err := pm.Acquire(ctx, CategoryDeployment)
	if err != nil {
		t.Fatal(err)
	}
	defer relDep2()

	if pm.ActiveCount(CategoryDeployment) != 2 {
		t.Errorf("expected 2 active deployments, got %d", pm.ActiveCount(CategoryDeployment))
	}
}
