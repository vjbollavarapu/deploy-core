package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/deploycore/deploy-core/apps/api/internal/deployments"
	"github.com/deploycore/deploy-core/apps/api/internal/replicas"
	"github.com/deploycore/deploy-core/packages/protocol-go"
	"github.com/google/uuid"
)

const cutoverReason = "writable_volume_cutover"
const cutoverReleaseReason = "writable_volume_release"

// volumeMount is the immutable snapshot stored in revisions.effective_config.
type volumeMount struct {
	Name      string `json:"name"`
	MountPath string `json:"mountPath"`
	ReadOnly  bool   `json:"readOnly"`
}

func parseVolumeMounts(raw any) []volumeMount {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]volumeMount, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		mount := volumeMount{
			Name:      stringField(m, "name"),
			MountPath: stringField(m, "mountPath"),
			ReadOnly:  boolField(m["readOnly"]),
		}
		if mount.Name == "" || mount.MountPath == "" {
			continue
		}
		out = append(out, mount)
	}
	return out
}

func stringField(m map[string]any, key string) string {
	s, _ := m[key].(string)
	return s
}

func boolField(raw any) bool {
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		return v == "true"
	default:
		return false
	}
}

func writableMounts(mounts []volumeMount) bool {
	for _, m := range mounts {
		if !m.ReadOnly {
			return true
		}
	}
	return false
}

func agentVolumePayload(mounts []volumeMount) []map[string]any {
	if len(mounts) == 0 {
		return nil
	}
	out := make([]map[string]any, 0, len(mounts))
	for _, m := range mounts {
		out = append(out, map[string]any{
			"name":          m.Name,
			"containerPath": m.MountPath,
			"readOnly":      m.ReadOnly,
		})
	}
	return out
}

// validateWritableReplicas rejects more than one replica when a writable mount is present.
func validateWritableReplicas(writable bool, desired int) error {
	if writable && desired != 1 {
		return fmt.Errorf("writable application volume requires desiredReplicas 1")
	}
	return nil
}

// cutoverStopsFirst reports whether the previous container must stop before create.
func cutoverStopsFirst(writable bool) bool {
	return writable
}

func (o *Orchestrator) applicationVolumeMounts(ctx context.Context, appID uuid.UUID, serverID *uuid.UUID) ([]volumeMount, error) {
	if serverID == nil {
		return nil, nil
	}
	rows, err := o.pool.Query(ctx, `
		SELECT COALESCE(NULLIF(docker_name, ''), name), mount_path, labels
		FROM volumes
		WHERE attached_resource_type = 'application'
		  AND attached_resource_id = $1
		  AND server_id = $2
		  AND deleted_at IS NULL
		  AND state NOT IN ('DELETED', 'DELETING', 'FAILED')
		ORDER BY name`, appID, *serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []volumeMount
	for rows.Next() {
		var name, mountPath string
		var labels []byte
		if err := rows.Scan(&name, &mountPath, &labels); err != nil {
			return nil, err
		}
		if name == "" || mountPath == "" {
			continue
		}
		out = append(out, volumeMount{
			Name:      name,
			MountPath: mountPath,
			ReadOnly:  readOnlyLabel(labels),
		})
	}
	return out, rows.Err()
}

func readOnlyLabel(raw []byte) bool {
	if len(raw) == 0 {
		return false
	}
	var labels map[string]any
	if err := json.Unmarshal(raw, &labels); err != nil {
		return false
	}
	return boolField(labels["readOnly"])
}

func (o *Orchestrator) revisionVolumeMounts(ctx context.Context, revisionID *uuid.UUID) ([]volumeMount, error) {
	if revisionID == nil {
		return nil, nil
	}
	var raw []byte
	if err := o.pool.QueryRow(ctx, `SELECT effective_config FROM revisions WHERE id = $1`, *revisionID).Scan(&raw); err != nil {
		return nil, err
	}
	var cfg map[string]any
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
	}
	return parseVolumeMounts(cfg["volumeMounts"]), nil
}

func predecessorNames(list []replicas.Replica, previousRevision uuid.UUID) []string {
	var names []string
	for _, rep := range list {
		if rep.RevisionID == nil || *rep.RevisionID != previousRevision || rep.ContainerName == "" {
			continue
		}
		names = append(names, rep.ContainerName)
	}
	return names
}

// stopWritableHolders stops replica containers that already belong to a
// revision. The active revision's containers are marked for restore. Any other
// revision-backed container is stopped so it releases the volume. A slot with
// no revision id is only a planned name and is not a volume holder.
func (o *Orchestrator) stopWritableHolders(ctx context.Context, d deployments.Deployment, list []replicas.Replica, active *activeRev) error {
	for _, rep := range list {
		if rep.ContainerName == "" || rep.RevisionID == nil {
			continue
		}
		reason := cutoverReleaseReason
		payload := map[string]any{
			"containerName": rep.ContainerName,
			"replicaIndex":  rep.ReplicaIndex,
			"reason":        reason,
		}
		if active != nil && rep.RevisionID != nil && *rep.RevisionID == active.ID {
			reason = cutoverReason
			payload["reason"] = reason
			// issueOrSimulate rewrites payload revisionId to the deployment
			// target. Keep the predecessor on a key that rewrite does not touch.
			payload["predecessorRevisionId"] = rep.RevisionID.String()
		}
		if err := o.issueOrSimulate(ctx, d, protocol.OpStopContainer, payload); err != nil {
			return err
		}
	}
	return nil
}

func (o *Orchestrator) restartCutoverPredecessors(ctx context.Context, d deployments.Deployment) {
	if o.pool == nil {
		return
	}
	if d.TargetRevisionID != nil {
		rows, err := o.pool.Query(ctx, `
			SELECT container_name FROM application_replicas
			WHERE application_id = $1 AND revision_id = $2 AND container_name <> ''`,
			d.ApplicationID, *d.TargetRevisionID)
		if err != nil {
			return
		}
		var created []string
		for rows.Next() {
			var name string
			if rows.Scan(&name) == nil && name != "" {
				created = append(created, name)
			}
		}
		rows.Close()
		for _, name := range created {
			if err := o.issueOrSimulate(ctx, d, protocol.OpStopContainer, map[string]any{
				"containerName": name,
				"reason":        "writable_volume_candidate_failed",
			}); err != nil {
				// The candidate may still hold the volume. Starting the
				// predecessor now would mount it twice.
				return
			}
		}
	}
	preds := o.cutoverPredecessorNames(ctx, d)
	revID, ok := o.predecessorRevisionForRestore(ctx, d, preds)
	if !ok {
		return
	}
	for _, pred := range preds {
		slot, err := o.replicas.UpsertSlot(ctx, replicas.UpsertInput{
			OrganizationID: d.OrganizationID,
			ApplicationID:  d.ApplicationID,
			RevisionID:     &revID,
			ServerID:       d.ServerID,
			ReplicaIndex:   pred.ReplicaIndex,
			ContainerName:  pred.Name,
			Status:         replicas.StatusStopped,
		})
		if err != nil {
			continue
		}
		if err := o.issueOrSimulate(ctx, d, protocol.OpStartContainer, map[string]any{
			"containerName": pred.Name,
			"reason":        "writable_volume_restore",
		}); err != nil {
			continue
		}
		healthy := true
		_, _ = o.replicas.UpdateStatus(ctx, slot.ID, replicas.StatusRunning, &healthy, nil, nil, nil)
	}
}

// predecessorRevisionForRestore returns the still-active predecessor revision.
// The stop-command revisionId is not used: production issueOrSimulate replaces
// it with the failed candidate before the command is stored.
func (o *Orchestrator) predecessorRevisionForRestore(ctx context.Context, d deployments.Deployment, preds []cutoverPredecessor) (uuid.UUID, bool) {
	if active, err := o.getActiveRevision(ctx, d.ApplicationID); err == nil && active != nil && !sameTargetRevision(active.ID, d.TargetRevisionID) {
		return active.ID, true
	}
	for _, pred := range preds {
		if pred.RevisionID != uuid.Nil && !sameTargetRevision(pred.RevisionID, d.TargetRevisionID) {
			return pred.RevisionID, true
		}
	}
	return uuid.Nil, false
}

func sameTargetRevision(id uuid.UUID, target *uuid.UUID) bool {
	return target != nil && id == *target
}

type cutoverPredecessor struct {
	Name         string
	ReplicaIndex int
	RevisionID   uuid.UUID
}

func (o *Orchestrator) cutoverPredecessorNames(ctx context.Context, d deployments.Deployment) []cutoverPredecessor {
	if o.cfg.SimulateAgent {
		var out []cutoverPredecessor
		seen := map[string]bool{}
		for _, cmd := range o.simulated {
			if cmd.deploymentID != d.ID || cmd.op != protocol.OpStopContainer {
				continue
			}
			if cmd.payload["reason"] != cutoverReason {
				continue
			}
			pred, ok := cutoverPredecessorFromPayload(cmd.payload)
			if !ok || seen[pred.Name] {
				continue
			}
			seen[pred.Name] = true
			out = append(out, pred)
		}
		return out
	}
	rows, err := o.pool.Query(ctx, `
		SELECT payload->>'containerName', COALESCE(payload->>'replicaIndex', '0'), COALESCE(payload->>'predecessorRevisionId', '')
		FROM agent_commands
		WHERE correlation_id = $1
		  AND operation = 'STOP_CONTAINER'
		  AND payload->>'reason' = $2
		  AND COALESCE(payload->>'containerName', '') <> ''`,
		d.ID.String(), cutoverReason)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []cutoverPredecessor
	seen := map[string]bool{}
	for rows.Next() {
		var name, indexText, revisionText string
		if rows.Scan(&name, &indexText, &revisionText) != nil || name == "" || seen[name] {
			continue
		}
		seen[name] = true
		pred := cutoverPredecessor{Name: name}
		if n, err := strconv.Atoi(indexText); err == nil && n >= 0 {
			pred.ReplicaIndex = n
		}
		if id, err := uuid.Parse(revisionText); err == nil {
			pred.RevisionID = id
		}
		out = append(out, pred)
	}
	return out
}

func cutoverPredecessorFromPayload(payload map[string]any) (cutoverPredecessor, bool) {
	name, _ := payload["containerName"].(string)
	if name == "" {
		return cutoverPredecessor{}, false
	}
	pred := cutoverPredecessor{Name: name}
	switch v := payload["replicaIndex"].(type) {
	case int:
		pred.ReplicaIndex = v
	case int64:
		pred.ReplicaIndex = int(v)
	case float64:
		pred.ReplicaIndex = int(v)
	case string:
		if n, err := strconv.Atoi(v); err == nil {
			pred.ReplicaIndex = n
		}
	}
	if raw, _ := payload["predecessorRevisionId"].(string); raw != "" {
		if id, err := uuid.Parse(raw); err == nil {
			pred.RevisionID = id
		}
	}
	return pred, true
}
