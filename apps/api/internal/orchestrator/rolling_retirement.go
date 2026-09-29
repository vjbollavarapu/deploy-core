package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const rollingPredecessorEvent = "rolling predecessors captured"

// rollingPredecessor is the container occupying a replica slot before that
// slot is overwritten. It is stored before UpsertSlot and is the only
// identity used to retire the previous container.
type rollingPredecessor struct {
	ContainerName string `json:"containerName"`
	RevisionID    string `json:"revisionId,omitempty"`
	ReplicaIndex  int    `json:"replicaIndex"`
}

func (o *Orchestrator) captureRollingPredecessors(ctx context.Context, d deployments.Deployment, desired int) error {
	if o.pool == nil {
		return nil
	}
	if _, captured, err := o.loadRollingPredecessors(ctx, d.ID); err != nil {
		return err
	} else if captured {
		return nil
	}
	list, err := o.replicas.ListByApplication(ctx, d.ApplicationID)
	if err != nil {
		return err
	}
	preds := make([]rollingPredecessor, 0)
	for _, rep := range list {
		if rep.ReplicaIndex < 0 || rep.ReplicaIndex >= desired || rep.ContainerName == "" {
			continue
		}
		if d.TargetRevisionID != nil && rep.RevisionID != nil && *rep.RevisionID == *d.TargetRevisionID {
			continue
		}
		item := rollingPredecessor{
			ContainerName: rep.ContainerName,
			ReplicaIndex:  rep.ReplicaIndex,
		}
		if rep.RevisionID != nil {
			item.RevisionID = rep.RevisionID.String()
		}
		preds = append(preds, item)
	}
	body, err := json.Marshal(map[string]any{"predecessors": preds})
	if err != nil {
		return err
	}
	_, err = o.pool.Exec(ctx, `
		INSERT INTO deployment_events (
			organization_id, deployment_id, from_status, to_status, message, metadata, request_id
		) VALUES ($1, $2, $3, $3, $4, $5, $6)`,
		d.OrganizationID, d.ID, d.Status, rollingPredecessorEvent, body, d.RequestID)
	return err
}

func (o *Orchestrator) loadRollingPredecessors(ctx context.Context, deploymentID uuid.UUID) ([]rollingPredecessor, bool, error) {
	var raw []byte
	err := o.pool.QueryRow(ctx, `
		SELECT metadata FROM deployment_events
		WHERE deployment_id = $1 AND message = $2
		ORDER BY created_at ASC, id ASC
		LIMIT 1`, deploymentID, rollingPredecessorEvent).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var meta struct {
		Predecessors []rollingPredecessor `json:"predecessors"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &meta); err != nil {
			return nil, false, err
		}
	}
	return meta.Predecessors, true, nil
}

// retireRollingPredecessors stops each captured predecessor by container name.
// The Agent rejects STOP_CONTAINER when both containerId and containerName are empty.
func (o *Orchestrator) retireRollingPredecessors(ctx context.Context, d deployments.Deployment) error {
	if o.pool == nil {
		return nil
	}
	preds, _, err := o.loadRollingPredecessors(ctx, d.ID)
	if err != nil {
		return err
	}
	for _, pred := range preds {
		if pred.ContainerName == "" {
			continue
		}
		payload := map[string]any{
			"deploymentId":  d.ID.String(),
			"containerName": pred.ContainerName,
			"replicaIndex":  pred.ReplicaIndex,
			"reason":        "retire_previous",
			"drainRouting":  true,
		}
		if pred.RevisionID != "" {
			payload["predecessorRevisionId"] = pred.RevisionID
		}
		if err := o.issueOrSimulate(ctx, d, protocol.OpStopContainer, payload); err != nil {
			return fmt.Errorf("retire previous container %s: %w", pred.ContainerName, err)
		}
	}
	return nil
}
