package placement_test

import (
	"errors"
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/placement"
	"github.com/google/uuid"
)

func TestSelectManualForcedServer(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	cands := []placement.Candidate{
		{ID: a, Status: "OFFLINE", CPUTotalMillis: 2000, MemoryTotalBytes: 4 << 30},
		{ID: b, Status: "ONLINE", CPUTotalMillis: 4000, MemoryTotalBytes: 8 << 30},
	}
	res, err := placement.Select(cands, placement.Request{
		CPUMillis:    500,
		MemoryBytes:  1 << 30,
		ForcedServer: &a,
		Policy:       placement.Policy{Mode: placement.ModeManual},
	})
	if err != nil {
		t.Fatalf("manual placement: %v", err)
	}
	if res.ServerID != a {
		t.Fatalf("got %s want %s", res.ServerID, a)
	}
}

func TestSelectSchedulerLeastLoaded(t *testing.T) {
	light := uuid.New()
	heavy := uuid.New()
	cands := []placement.Candidate{
		{
			ID: heavy, Status: "ONLINE",
			CPUTotalMillis: 4000, CPUAllocatedMillis: 3000,
			MemoryTotalBytes: 8 << 30, MemoryAllocatedBytes: 6 << 30,
		},
		{
			ID: light, Status: "ONLINE",
			CPUTotalMillis: 4000, CPUAllocatedMillis: 500,
			MemoryTotalBytes: 8 << 30, MemoryAllocatedBytes: 1 << 30,
		},
	}
	res, err := placement.Select(cands, placement.Request{
		CPUMillis:   500,
		MemoryBytes: 1 << 30,
		Policy:      placement.Policy{Mode: placement.ModeScheduler, Scoring: "least_loaded"},
	})
	if err != nil {
		t.Fatalf("scheduler: %v", err)
	}
	if res.ServerID != light {
		t.Fatalf("expected light server, got %s", res.ServerID)
	}
}

func TestSelectFiltersMaintenanceLabelsCapacity(t *testing.T) {
	ok := uuid.New()
	cands := []placement.Candidate{
		{ID: uuid.New(), Status: "ONLINE", MaintenanceMode: true, CPUTotalMillis: 4000, MemoryTotalBytes: 8 << 30, Labels: map[string]string{"tier": "prod"}},
		{ID: uuid.New(), Status: "ONLINE", CPUTotalMillis: 4000, MemoryTotalBytes: 8 << 30, Labels: map[string]string{"tier": "dev"}},
		{ID: uuid.New(), Status: "ONLINE", CPUTotalMillis: 100, MemoryTotalBytes: 8 << 30, Labels: map[string]string{"tier": "prod"}},
		{ID: ok, Status: "ONLINE", CPUTotalMillis: 4000, MemoryTotalBytes: 8 << 30, Labels: map[string]string{"tier": "prod"}},
	}
	res, err := placement.Select(cands, placement.Request{
		CPUMillis: 1000,
		Policy: placement.Policy{
			Mode:   placement.ModeScheduler,
			Labels: map[string]string{"tier": "prod"},
		},
	})
	if err != nil {
		t.Fatalf("select: %v", err)
	}
	if res.ServerID != ok {
		t.Fatalf("got %s want %s", res.ServerID, ok)
	}
}

func TestSelectInsufficientResources(t *testing.T) {
	_, err := placement.Select(nil, placement.Request{
		CPUMillis: 100,
		Policy:    placement.Policy{Mode: placement.ModeScheduler},
	})
	if !errors.Is(err, placement.ErrInsufficientResources) {
		t.Fatalf("want ErrInsufficientResources, got %v", err)
	}

	id := uuid.New()
	_, err = placement.Select([]placement.Candidate{{
		ID: id, Status: "ONLINE", CPUTotalMillis: 100, CPUAllocatedMillis: 80,
	}}, placement.Request{
		CPUMillis:    50,
		ForcedServer: &id,
	})
	if !errors.Is(err, placement.ErrInsufficientResources) {
		t.Fatalf("forced oversubscribed: want ErrInsufficientResources, got %v", err)
	}
}

func TestCoresToMillis(t *testing.T) {
	n := 2
	if got := placement.CoresToMillis(&n); got != 2000 {
		t.Fatalf("got %d", got)
	}
	if placement.CoresToMillis(nil) != 0 {
		t.Fatal("nil cores")
	}
}
