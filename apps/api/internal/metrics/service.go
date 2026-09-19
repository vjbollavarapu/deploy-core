package metrics

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/deploycore/deploy-core/apps/api/internal/agents"
	"github.com/deploycore/deploy-core/apps/api/internal/rbac"
	"github.com/deploycore/deploy-core/apps/api/pkg/apierror"
	"github.com/google/uuid"
)

type Service struct {
	repo Repository
	ts   TimeSeriesStore
	authz *rbac.Authorizer
	log  *slog.Logger
	now  func() time.Time
}

func NewService(repo Repository, ts TimeSeriesStore, authz *rbac.Authorizer, log *slog.Logger) *Service {
	if ts == nil {
		ts = NopTimeSeries{}
	}
	return &Service{repo: repo, ts: ts, authz: authz, log: log, now: time.Now}
}

func (s *Service) GetServer(ctx context.Context, actorID, serverID uuid.UUID) (ServerSnapshot, error) {
	orgID, err := s.repo.GetServerOrg(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ServerSnapshot{}, apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return ServerSnapshot{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ServerRead); err != nil {
		return ServerSnapshot{}, err
	}
	snap, err := s.repo.GetServerSnapshot(ctx, serverID)
	if err == nil {
		return snap, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return ServerSnapshot{}, err
	}
	// Fallback: latest heartbeat sample (bounded telemetry, not indefinite series).
	snap, err = s.repo.LatestHeartbeatAsServer(ctx, serverID)
	if errors.Is(err, ErrNotFound) {
		return ServerSnapshot{
			ServerID:       serverID,
			OrganizationID: orgID,
			RecordedAt:     s.now().UTC(),
			Source:         SourceHeartbeat,
			Payload:        map[string]any{},
			UpdatedAt:      s.now().UTC(),
		}, nil
	}
	return snap, err
}

func (s *Service) ListContainers(ctx context.Context, actorID, serverID uuid.UUID) ([]ContainerSnapshot, error) {
	orgID, err := s.repo.GetServerOrg(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return nil, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ServerRead); err != nil {
		return nil, err
	}
	return s.repo.ListContainerSnapshots(ctx, serverID)
}

func (s *Service) GetApplication(ctx context.Context, actorID, appID uuid.UUID) (ContainerSnapshot, error) {
	orgID, err := s.repo.GetApplicationOrg(ctx, appID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return ContainerSnapshot{}, apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
		}
		return ContainerSnapshot{}, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ApplicationRead); err != nil {
		return ContainerSnapshot{}, err
	}
	c, err := s.repo.GetContainerByApplication(ctx, appID)
	if errors.Is(err, ErrNotFound) {
		return ContainerSnapshot{}, apierror.NotFound("no container metrics for application")
	}
	return c, err
}

// QuerySeries exposes the time-series abstraction (memory / future Prometheus).
func (s *Service) QuerySeries(ctx context.Context, actorID, serverID uuid.UUID, q RangeQuery) ([]Series, error) {
	orgID, err := s.repo.GetServerOrg(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return nil, err
	}
	if err := s.authz.RequirePermission(ctx, actorID, orgID, rbac.ServerRead); err != nil {
		return nil, err
	}
	if q.Labels == nil {
		q.Labels = map[string]string{}
	}
	q.Labels["server_id"] = serverID.String()
	return s.ts.Query(ctx, q)
}

func (s *Service) IngestFromAgent(ctx context.Context, agent agents.Agent, in AgentIngestInput) error {
	if in.Server == nil && len(in.Containers) == 0 {
		return apierror.Validation("server or containers required", nil)
	}
	orgID := agent.OrganizationID
	serverID := agent.ServerID
	gotOrg, err := s.repo.GetServerOrg(ctx, serverID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return apierror.NotFoundCode(apierror.CodeServerNotFound, "server not found")
		}
		return err
	}
	if gotOrg != orgID {
		return apierror.Forbidden("server organization mismatch")
	}

	now := s.now().UTC()
	var samples []Sample

	if in.Server != nil {
		at := now
		if in.Server.RecordedAt != nil {
			at = in.Server.RecordedAt.UTC()
		}
		snap := ServerSnapshot{
			ServerID:         serverID,
			OrganizationID:   orgID,
			RecordedAt:       at,
			CPUPercent:       in.Server.CPUPercent,
			MemoryUsedBytes:  in.Server.MemoryUsedBytes,
			MemoryTotalBytes: in.Server.MemoryTotalBytes,
			DiskUsedBytes:    in.Server.DiskUsedBytes,
			DiskTotalBytes:   in.Server.DiskTotalBytes,
			Load1:            in.Server.Load1,
			Load5:            in.Server.Load5,
			Load15:           in.Server.Load15,
			UptimeSeconds:    in.Server.UptimeSeconds,
			NetworkRxBytes:   in.Server.NetworkRxBytes,
			NetworkTxBytes:   in.Server.NetworkTxBytes,
			ContainerCount:   in.Server.ContainerCount,
			Source:           SourceAgent,
			Payload:          in.Server.Payload,
		}
		if _, err := s.repo.UpsertServerSnapshot(ctx, snap); err != nil {
			return err
		}
		samples = append(samples, serverSamples(serverID, snap)...)
	}

	if len(in.Containers) > 200 {
		return apierror.Validation("too many containers in one batch", map[string]any{"max": 200})
	}
	for _, c := range in.Containers {
		cid := strings.TrimSpace(c.ContainerID)
		if cid == "" {
			return apierror.Validation("containerId is required", map[string]any{"field": "containerId"})
		}
		if c.ApplicationID != nil {
			appOrg, err := s.repo.GetApplicationOrg(ctx, *c.ApplicationID)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					return apierror.NotFoundCode(apierror.CodeApplicationNotFound, "application not found")
				}
				return err
			}
			if appOrg != orgID {
				return apierror.Forbidden("application is outside agent organization")
			}
		}
		at := now
		if c.RecordedAt != nil {
			at = c.RecordedAt.UTC()
		}
		status := strings.TrimSpace(c.Status)
		if status == "" {
			status = "unknown"
		}
		snap := ContainerSnapshot{
			OrganizationID:   orgID,
			ServerID:         serverID,
			ApplicationID:    c.ApplicationID,
			ContainerID:      cid,
			ContainerName:    strings.TrimSpace(c.ContainerName),
			RecordedAt:       at,
			CPUPercent:       c.CPUPercent,
			MemoryUsedBytes:  c.MemoryUsedBytes,
			MemoryLimitBytes: c.MemoryLimitBytes,
			NetworkRxBytes:   c.NetworkRxBytes,
			NetworkTxBytes:   c.NetworkTxBytes,
			RestartCount:     c.RestartCount,
			Status:           status,
			Payload:          c.Payload,
		}
		if _, err := s.repo.UpsertContainerSnapshot(ctx, snap); err != nil {
			return err
		}
		samples = append(samples, containerSamples(serverID, snap)...)
	}

	if len(samples) > 0 {
		_ = s.ts.Write(ctx, samples)
	}
	return nil
}

// UpsertFromHeartbeat keeps summary metrics warm from B7 heartbeats without
// treating heartbeats as unbounded time series storage.
func (s *Service) UpsertFromHeartbeat(ctx context.Context, orgID, serverID uuid.UUID, fields agents.HeartbeatMetricFields, at time.Time) error {
	snap := ServerSnapshot{
		ServerID:        serverID,
		OrganizationID:  orgID,
		RecordedAt:      at.UTC(),
		CPUPercent:      fields.CPUPercent,
		MemoryUsedBytes: fields.MemoryUsedBytes,
		DiskUsedBytes:   fields.DiskUsedBytes,
		Load1:           fields.Load1,
		ContainerCount:  fields.ContainerCount,
		UptimeSeconds:   fields.UptimeSeconds,
		Source:          SourceHeartbeat,
		Payload:         map[string]any{},
	}
	if _, err := s.repo.UpsertServerSnapshot(ctx, snap); err != nil {
		return err
	}
	_ = s.ts.Write(ctx, serverSamples(serverID, snap))
	return nil
}

func serverSamples(serverID uuid.UUID, snap ServerSnapshot) []Sample {
	labels := map[string]string{"server_id": serverID.String()}
	var out []Sample
	add := func(name string, v *float64) {
		if v == nil {
			return
		}
		out = append(out, Sample{Metric: name, Labels: labels, Timestamp: snap.RecordedAt, Value: *v})
	}
	addInt := func(name string, v *int64) {
		if v == nil {
			return
		}
		f := float64(*v)
		out = append(out, Sample{Metric: name, Labels: labels, Timestamp: snap.RecordedAt, Value: f})
	}
	add("server_cpu_percent", snap.CPUPercent)
	add("server_load1", snap.Load1)
	addInt("server_memory_used_bytes", snap.MemoryUsedBytes)
	addInt("server_disk_used_bytes", snap.DiskUsedBytes)
	addInt("server_network_rx_bytes", snap.NetworkRxBytes)
	addInt("server_network_tx_bytes", snap.NetworkTxBytes)
	addInt("server_uptime_seconds", snap.UptimeSeconds)
	if snap.ContainerCount != nil {
		f := float64(*snap.ContainerCount)
		out = append(out, Sample{Metric: "server_container_count", Labels: labels, Timestamp: snap.RecordedAt, Value: f})
	}
	return out
}

func containerSamples(serverID uuid.UUID, snap ContainerSnapshot) []Sample {
	labels := map[string]string{
		"server_id":    serverID.String(),
		"container_id": snap.ContainerID,
	}
	if snap.ApplicationID != nil {
		labels["application_id"] = snap.ApplicationID.String()
	}
	var out []Sample
	if snap.CPUPercent != nil {
		out = append(out, Sample{Metric: "container_cpu_percent", Labels: labels, Timestamp: snap.RecordedAt, Value: *snap.CPUPercent})
	}
	if snap.MemoryUsedBytes != nil {
		out = append(out, Sample{Metric: "container_memory_used_bytes", Labels: labels, Timestamp: snap.RecordedAt, Value: float64(*snap.MemoryUsedBytes)})
	}
	if snap.NetworkRxBytes != nil {
		out = append(out, Sample{Metric: "container_network_rx_bytes", Labels: labels, Timestamp: snap.RecordedAt, Value: float64(*snap.NetworkRxBytes)})
	}
	if snap.NetworkTxBytes != nil {
		out = append(out, Sample{Metric: "container_network_tx_bytes", Labels: labels, Timestamp: snap.RecordedAt, Value: float64(*snap.NetworkTxBytes)})
	}
	if snap.RestartCount != nil {
		out = append(out, Sample{Metric: "container_restart_count", Labels: labels, Timestamp: snap.RecordedAt, Value: float64(*snap.RestartCount)})
	}
	return out
}
