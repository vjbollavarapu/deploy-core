package stats

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// DockerClient represents the Docker subset required for container stats.
type DockerClient interface {
	InspectContainer(ctx context.Context, id string) (docker.ContainerDetail, error)
	ContainerStats(ctx context.Context, id string) (docker.ContainerStatsSnapshot, error)
	ListContainers(ctx context.Context, all bool) ([]docker.ContainerSummary, error)
}

// Collector collects point-in-time resource statistics for containers.
type Collector struct {
	cli DockerClient
}

// NewCollector constructs a Collector.
func NewCollector(cli DockerClient) *Collector {
	return &Collector{cli: cli}
}

// GetContainerStats collects an efficient one-shot resource snapshot for a single container.
func (c *Collector) GetContainerStats(ctx context.Context, id string) (*ContainerStats, error) {
	detail, err := c.cli.InspectContainer(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to inspect container: %w", err)
	}

	snap, err := c.cli.ContainerStats(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get container stats: %w", err)
	}

	appID := ""
	revID := ""
	if detail.Labels != nil {
		appID = detail.Labels[protocol.LabelApplicationID]
		revID = detail.Labels[protocol.LabelRevisionID]
	}

	status := detail.State.Status
	if status == "" {
		if detail.State.Running {
			status = "running"
		} else {
			status = "stopped"
		}
	}

	return &ContainerStats{
		ContainerID:   detail.ID,
		ContainerName: detail.Name,
		ApplicationID: appID,
		RevisionID:    revID,
		Status:        status,
		CPUPercent:    snap.CPUPercent,
		MemoryUsage:   snap.MemoryUsage,
		MemoryLimit:   snap.MemoryLimit,
		NetworkRx:     snap.NetworkRx,
		NetworkTx:     snap.NetworkTx,
		BlockRead:     snap.BlockRead,
		BlockWrite:    snap.BlockWrite,
		PidsCurrent:   snap.PidsCurrent,
		RestartCount:  detail.RestartCount,
		Timestamp:     time.Now().UTC(),
	}, nil
}

// CollectAllManaged queries one-shot stats for all managed containers using bounded concurrency.
func (c *Collector) CollectAllManaged(ctx context.Context) ([]ContainerStats, error) {
	containers, err := c.cli.ListContainers(ctx, true)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	// Filter strictly managed containers
	var managedIDs []string
	for _, cnt := range containers {
		if cnt.Labels != nil && cnt.Labels[protocol.LabelManaged] == "true" {
			managedIDs = append(managedIDs, cnt.ID)
		}
	}

	if len(managedIDs) == 0 {
		return []ContainerStats{}, nil
	}

	// Bounded worker pool (max 5 concurrent inspect/stats calls)
	concurrency := 5
	if len(managedIDs) < concurrency {
		concurrency = len(managedIDs)
	}

	idChan := make(chan string, len(managedIDs))
	for _, id := range managedIDs {
		idChan <- id
	}
	close(idChan)

	var mu sync.Mutex
	results := make([]ContainerStats, 0, len(managedIDs))

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range idChan {
				if ctx.Err() != nil {
					return
				}
				stat, err := c.GetContainerStats(ctx, id)
				if err == nil && stat != nil {
					mu.Lock()
					results = append(results, *stat)
					mu.Unlock()
				}
			}
		}()
	}

	wg.Wait()

	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	return results, nil
}
