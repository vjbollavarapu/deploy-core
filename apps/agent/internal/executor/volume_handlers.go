package executor

import (
	"context"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/volume"
)

// --------------------------------------------------------------------------
// Volume operation handlers
// --------------------------------------------------------------------------

type createVolumePayload struct {
	Name           string            `json:"name"`
	VolumeID       string            `json:"volumeId"`
	OrganizationID string            `json:"organizationId"`
	ProjectID      string            `json:"projectId"`
	EnvironmentID  string            `json:"environmentId"`
	ApplicationID  string            `json:"applicationId"`
	Critical       string            `json:"critical"`
	Driver         string            `json:"driver"`
	DriverOpts     map[string]string `json:"driverOpts"`
	Labels         map[string]string `json:"labels"`
}

func createVolumeHandler(cli *docker.Client) Handler {
	mgr := volume.NewManager(cli)

	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p createVolumePayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}
		if p.Name == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "volume name is required")
		}

		meta := volume.Metadata{
			OrganizationID: p.OrganizationID,
			VolumeID:       p.VolumeID,
			VolumeName:     p.Name,
			ProjectID:      p.ProjectID,
			EnvironmentID:  p.EnvironmentID,
			ApplicationID:  p.ApplicationID,
			Critical:       p.Critical,
		}

		detail, err := mgr.EnsureVolume(ctx, p.Name, meta, p.Driver, p.DriverOpts)
		if err != nil {
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{Output: map[string]any{
			"name":       detail.Name,
			"driver":     detail.Driver,
			"mountpoint": detail.Mountpoint,
			"labels":     detail.Labels,
		}}, nil
	})
}

type removeVolumePayload struct {
	Name           string `json:"name"`
	Force          bool   `json:"force"`
	OrganizationID string `json:"organizationId"`
}

func removeVolumeHandler(cli *docker.Client) Handler {
	mgr := volume.NewManager(cli)

	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p removeVolumePayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}
		if p.Name == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "volume name is required")
		}

		// DeleteVolume blocks if attached containers exist, unless force is authorized
		if err := mgr.DeleteVolume(ctx, p.Name, p.Force, p.OrganizationID); err != nil {
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{Output: map[string]any{
			"name":    p.Name,
			"removed": true,
			"forced":  p.Force,
		}}, nil
	})
}

type attachVolumePayload struct {
	Name        string `json:"name"`
	VolumeName  string `json:"volumeName"`
	ContainerID string `json:"containerId"`
	MountPath   string `json:"mountPath"`
	ReadOnly    bool   `json:"readOnly"`
}

func attachVolumeHandler(cli *docker.Client) Handler {
	mgr := volume.NewManager(cli)

	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p attachVolumePayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}

		volName := p.Name
		if volName == "" {
			volName = p.VolumeName
		}
		if volName == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "volume name is required")
		}

		res, err := mgr.AttachVolume(ctx, volName, p.ContainerID, p.MountPath, p.ReadOnly)
		if err != nil {
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{Output: res}, nil
	})
}

type detachVolumePayload struct {
	Name        string `json:"name"`
	VolumeName  string `json:"volumeName"`
	ContainerID string `json:"containerId"`
}

func detachVolumeHandler(cli *docker.Client) Handler {
	mgr := volume.NewManager(cli)

	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p detachVolumePayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}

		volName := p.Name
		if volName == "" {
			volName = p.VolumeName
		}
		if volName == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "volume name is required")
		}

		res, err := mgr.DetachVolume(ctx, volName, p.ContainerID)
		if err != nil {
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{Output: res}, nil
	})
}

type inspectVolumePayload struct {
	Name       string `json:"name"`
	VolumeName string `json:"volumeName"`
}

func inspectVolumeHandler(cli *docker.Client) Handler {
	mgr := volume.NewManager(cli)

	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p inspectVolumePayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}

		volName := p.Name
		if volName == "" {
			volName = p.VolumeName
		}
		if volName == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "volume name is required")
		}

		inspection, err := mgr.InspectVolume(ctx, volName)
		if err != nil {
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{Output: map[string]any{
			"name":               inspection.Detail.Name,
			"driver":             inspection.Detail.Driver,
			"mountpoint":         inspection.Detail.Mountpoint,
			"scope":              inspection.Detail.Scope,
			"labels":             inspection.Detail.Labels,
			"ownership":          string(inspection.Ownership),
			"sizeBytes":          inspection.SizeBytes,
			"attachedContainers": inspection.AttachedContainers,
		}}, nil
	})
}
