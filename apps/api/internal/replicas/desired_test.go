package replicas_test

import (
	"testing"

	"github.com/deploycore/deploy-core/apps/api/internal/replicas"
)

func TestDesiredFromRuntime(t *testing.T) {
	if got := replicas.DesiredFromRuntime(nil); got != 1 {
		t.Fatalf("nil => %d", got)
	}
	if got := replicas.DesiredFromRuntime(map[string]any{"desiredReplicas": float64(3)}); got != 3 {
		t.Fatalf("float => %d", got)
	}
	if got := replicas.DesiredFromRuntime(map[string]any{"desiredReplicas": 99}); got != replicas.MaxDesiredReplicas {
		t.Fatalf("clamp => %d", got)
	}
	if got := replicas.DesiredFromRuntime(map[string]any{"desiredReplicas": 0}); got != 1 {
		t.Fatalf("zero => %d", got)
	}
}

func TestMultiplyLimits(t *testing.T) {
	cpu, mem, disk := replicas.MultiplyLimits(500, 1<<20, 10, 3)
	if cpu != 1500 || mem != 3<<20 || disk != 30 {
		t.Fatalf("got cpu=%d mem=%d disk=%d", cpu, mem, disk)
	}
}

func TestContainerName(t *testing.T) {
	if got := replicas.ContainerName("api", 2, 1); got != "api-r2-1" {
		t.Fatalf("got %s", got)
	}
}

func TestValidateDesired(t *testing.T) {
	if err := replicas.ValidateDesired(0); err == nil {
		t.Fatal("expected error")
	}
	if err := replicas.ValidateDesired(2); err != nil {
		t.Fatal(err)
	}
}
