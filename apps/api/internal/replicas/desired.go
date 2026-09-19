package replicas

import (
	"encoding/json"
	"fmt"
	"math"
)

// DesiredFromRuntime extracts desiredReplicas from runtime_config (default 1).
func DesiredFromRuntime(runtime map[string]any) int {
	if runtime == nil {
		return DefaultDesired
	}
	switch v := runtime["desiredReplicas"].(type) {
	case float64:
		return clampDesired(int(v))
	case int:
		return clampDesired(v)
	case int64:
		return clampDesired(int(v))
	case json.Number:
		n, err := v.Int64()
		if err != nil {
			return DefaultDesired
		}
		return clampDesired(int(n))
	default:
		return DefaultDesired
	}
}

// SetDesiredRuntime returns a copy of runtime with desiredReplicas set.
func SetDesiredRuntime(runtime map[string]any, desired int) map[string]any {
	out := map[string]any{}
	for k, v := range runtime {
		out[k] = v
	}
	out["desiredReplicas"] = clampDesired(desired)
	return out
}

func clampDesired(n int) int {
	if n < 1 {
		return DefaultDesired
	}
	if n > MaxDesiredReplicas {
		return MaxDesiredReplicas
	}
	return n
}

// ValidateDesired returns an error message when desired is out of range.
func ValidateDesired(n int) error {
	if n < 1 || n > MaxDesiredReplicas {
		return fmt.Errorf("desiredReplicas must be between 1 and %d", MaxDesiredReplicas)
	}
	return nil
}

// MultiplyLimits scales per-replica resource requests by desired count.
func MultiplyLimits(perReplicaCPU int, perReplicaMem, perReplicaDisk int64, desired int) (cpu int, mem, disk int64) {
	d := clampDesired(desired)
	cpu = perReplicaCPU * d
	mem = perReplicaMem * int64(d)
	disk = perReplicaDisk * int64(d)
	if perReplicaCPU > 0 && cpu/d != perReplicaCPU {
		cpu = math.MaxInt32
	}
	return
}

// ContainerName builds a deterministic container name for a replica slot.
func ContainerName(appSlug string, revisionNumber, index int) string {
	safe := appSlug
	if safe == "" {
		safe = "app"
	}
	return fmt.Sprintf("%s-r%d-%d", safe, revisionNumber, index)
}
