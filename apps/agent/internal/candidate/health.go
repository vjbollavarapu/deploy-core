package candidate

import (
	"context"
	"errors"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/health"
)

// DockerClient defines the subset of Docker daemon operations required for health evaluation.
type DockerClient interface {
	InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error)
	ExecContainer(ctx context.Context, id string, cmd []string) (docker.ExecResult, error)
}

// EvaluateHealth checks the readiness of a candidate container according to policy.
func EvaluateHealth(ctx context.Context, client DockerClient, containerID string, ipAddress string, policy HealthPolicy) error {
	cfg := health.Config{
		ProbeType:        health.ProbeType(policy.Type),
		InitialDelay:     policy.InitialDelay,
		Interval:         policy.Interval,
		Timeout:          policy.Timeout,
		FailureThreshold: policy.FailureThreshold,
		SuccessThreshold: policy.SuccessThreshold,
		HTTPPath:         policy.HTTPPath,
		HTTPPort:         policy.HTTPPort,
		HTTPScheme:       policy.HTTPScheme,
		ExpectedStatus:   policy.ExpectedStatus,
		TCPPort:          policy.TCPPort,
		Command:          policy.Command,
	}

	res, err := health.Execute(ctx, client, containerID, ipAddress, cfg)
	if err != nil {
		return err
	}
	if !res.Healthy {
		return errors.New(res.Summary)
	}
	return nil
}
