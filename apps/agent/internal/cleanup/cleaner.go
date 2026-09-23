package cleanup

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// DockerClient represents the subset of Docker calls required for cleanup.
type DockerClient interface {
	ListContainers(ctx context.Context, all bool) ([]docker.ContainerSummary, error)
	RemoveContainer(ctx context.Context, id string, force bool) error
	ListImages(ctx context.Context, all bool) ([]docker.ImageSummary, error)
	RemoveImage(ctx context.Context, id string, force bool) error
}

// WorkspacePruner represents the workspace cleanup interface.
type WorkspacePruner interface {
	Prune(maxAge time.Duration) (int, error)
}

// Cleaner coordinates safe, platform-scoped host cleanup.
type Cleaner struct {
	cli   DockerClient
	wsMgr WorkspacePruner
	log   *slog.Logger
}

// NewCleaner constructs a Cleaner.
func NewCleaner(cli DockerClient, wsMgr WorkspacePruner, log *slog.Logger) *Cleaner {
	if log == nil {
		log = slog.Default()
	}
	return &Cleaner{cli: cli, wsMgr: wsMgr, log: log}
}

// Cleanup executes a safe platform cleanup according to the supplied policy.
// Invariant: Never indiscriminately runs destructive global Docker prune.
// Invariant: Never removes unmanaged resources, active revision images, database volumes, or backup volumes.
func (c *Cleaner) Cleanup(ctx context.Context, p Policy) (*Report, error) {
	report := &Report{
		Timestamp: time.Now().UTC(),
	}

	activeRevSet := make(map[string]bool, len(p.ActiveRevisionIDs))
	for _, id := range p.ActiveRevisionIDs {
		activeRevSet[id] = true
	}

	activeImgSet := make(map[string]bool, len(p.ActiveImageIDs))
	for _, id := range p.ActiveImageIDs {
		activeImgSet[id] = true
	}

	// 1. Prune expired workspaces
	if c.wsMgr != nil && p.WorkspaceMaxAge > 0 {
		count, err := c.wsMgr.Prune(p.WorkspaceMaxAge)
		if err != nil {
			c.log.Warn("workspace prune error", slog.String("error", err.Error()))
			report.Errors = append(report.Errors, "workspace prune: "+err.Error())
		} else {
			report.PrunedWorkspaces = count
		}
	}

	if c.cli == nil {
		return report, nil
	}

	// 2. Fetch all containers to identify candidates and referenced images
	containers, err := c.cli.ListContainers(ctx, true)
	if err != nil {
		report.Errors = append(report.Errors, "failed to list containers: "+err.Error())
		return report, fmt.Errorf("failed to list containers: %w", err)
	}

	referencedImages := make(map[string]bool)
	for _, cnt := range containers {
		if cnt.ImageID != "" {
			referencedImages[cnt.ImageID] = true
		}
		if cnt.Image != "" {
			referencedImages[cnt.Image] = true
		}
	}

	// 3. Prune stopped superseded managed containers
	if p.PruneStoppedContainers {
		for _, cnt := range containers {
			if !isEligibleContainerForRemoval(cnt, activeRevSet) {
				continue
			}

			c.log.Info("pruning stopped superseded container", slog.String("id", cnt.ID), slog.Any("names", cnt.Names))
			if err := c.cli.RemoveContainer(ctx, cnt.ID, false); err != nil {
				c.log.Warn("failed to remove stopped container", slog.String("id", cnt.ID), slog.String("error", err.Error()))
				report.Errors = append(report.Errors, fmt.Sprintf("remove container %s: %s", cnt.ID, err.Error()))
			} else {
				report.RemovedContainers = append(report.RemovedContainers, cnt.ID)
			}
		}
	}

	// 4. Prune expired unused managed images
	if p.PruneUnusedManagedImages {
		images, err := c.cli.ListImages(ctx, false)
		if err != nil {
			c.log.Warn("failed to list images", slog.String("error", err.Error()))
			report.Errors = append(report.Errors, "failed to list images: "+err.Error())
		} else {
			for _, img := range images {
				if !isEligibleImageForRemoval(img, referencedImages, activeImgSet) {
					continue
				}

				c.log.Info("pruning unreferenced managed image", slog.String("id", img.ID), slog.Any("repoTags", img.RepoTags))
				if err := c.cli.RemoveImage(ctx, img.ID, false); err != nil {
					c.log.Warn("failed to remove image", slog.String("id", img.ID), slog.String("error", err.Error()))
					report.Errors = append(report.Errors, fmt.Sprintf("remove image %s: %s", img.ID, err.Error()))
				} else {
					report.PrunedImages = append(report.PrunedImages, img.ID)
					report.ReclaimedBytes += img.Size
				}
			}
		}
	}

	return report, nil
}

// isEligibleContainerForRemoval tests if a container is safely eligible for cleanup.
func isEligibleContainerForRemoval(c docker.ContainerSummary, activeRevisions map[string]bool) bool {
	// Rule: Must have deploycore.managed == "true"
	if c.Labels == nil || c.Labels[protocol.LabelManaged] != "true" {
		return false
	}

	// Rule: Must NOT be protected (e.g. databases, stateful resources)
	if c.Labels[protocol.LabelProtected] == "true" || c.Labels["deploycore.service_type"] == "database" {
		return false
	}

	// Rule: Must be stopped (exited or dead)
	state := strings.ToLower(c.State)
	if state != "exited" && state != "dead" {
		return false
	}

	// Rule: Must NOT be currently active revision
	if c.Labels["deploycore.active"] == "true" {
		return false
	}
	revID := c.Labels[protocol.LabelRevisionID]
	if revID != "" && activeRevisions[revID] {
		return false
	}

	return true
}

// isEligibleImageForRemoval tests if an image is safely eligible for cleanup.
func isEligibleImageForRemoval(img docker.ImageSummary, referencedImages map[string]bool, activeImages map[string]bool) bool {
	// Rule: Must carry deploycore.managed == "true" (or have been created by platform)
	if img.Labels == nil || img.Labels[protocol.LabelManaged] != "true" {
		return false
	}

	// Rule: Must NOT be referenced by ANY container (running or stopped)
	if referencedImages[img.ID] {
		return false
	}
	for _, tag := range img.RepoTags {
		if referencedImages[tag] {
			return false
		}
	}

	// Rule: Must NOT be in active images set
	if activeImages[img.ID] {
		return false
	}
	for _, tag := range img.RepoTags {
		if activeImages[tag] {
			return false
		}
	}

	return true
}
