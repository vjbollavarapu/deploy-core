package executor

import (
	"context"

	"github.com/deploycore/deploy-core/apps/agent/internal/docker"
	"github.com/deploycore/deploy-core/apps/agent/internal/network"
	"github.com/deploycore/deploy-core/packages/protocol-go"
)

// --------------------------------------------------------------------------
// Network operation handlers
// --------------------------------------------------------------------------

type createNetworkPayload struct {
	Name            string            `json:"name"`
	ProjectID       string            `json:"projectId"`
	ProjectSlug     string            `json:"projectSlug"`
	EnvironmentID   string            `json:"environmentId"`
	EnvironmentSlug string            `json:"environmentSlug"`
	OrganizationID  string            `json:"organizationId"`
	NetworkType     string            `json:"networkType"` // "private", "proxy", "isolated"
	Driver          string            `json:"driver"`
	Internal        bool              `json:"internal"`
	Labels          map[string]string `json:"labels"`
	Options         map[string]string `json:"options"`
}

func createNetworkHandler(cli *docker.Client) Handler {
	mgr := network.NewManager(cli)

	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p createNetworkPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}

		meta := network.Metadata{
			OrganizationID:  p.OrganizationID,
			ProjectID:       p.ProjectID,
			ProjectSlug:     p.ProjectSlug,
			EnvironmentID:   p.EnvironmentID,
			EnvironmentSlug: p.EnvironmentSlug,
			NetworkType:     p.NetworkType,
		}

		// Case 1: DeployCore proxy network
		if p.Name == protocol.ProxyNetworkName || p.NetworkType == protocol.NetworkTypeProxy {
			detail, err := mgr.EnsureProxyNetwork(ctx)
			if err != nil {
				return ExecutionResult{}, wrapDockerErr(err)
			}
			return ExecutionResult{Output: map[string]any{
				"networkId": detail.ID,
				"name":      detail.Name,
				"proxy":     true,
			}}, nil
		}

		// Case 2: Project-scoped private network via slug/id
		if (p.ProjectSlug != "" || p.ProjectID != "") && (p.EnvironmentSlug != "" || p.EnvironmentID != "") && p.Name == "" {
			detail, err := mgr.EnsurePrivateNetwork(ctx, meta)
			if err != nil {
				return ExecutionResult{}, wrapDockerErr(err)
			}
			return ExecutionResult{Output: map[string]any{
				"networkId": detail.ID,
				"name":      detail.Name,
			}}, nil
		}

		// Case 3: Explicit name specified
		if p.Name == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "network name or project/environment identifiers required")
		}

		detail, err := mgr.EnsureNetwork(ctx, p.Name, meta, p.Internal)
		if err != nil {
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{Output: map[string]any{
			"networkId": detail.ID,
			"name":      detail.Name,
		}}, nil
	})
}

type removeNetworkPayload struct {
	NetworkID      string `json:"networkId"`
	Name           string `json:"name"`
	OrganizationID string `json:"organizationId"`
}

func removeNetworkHandler(cli *docker.Client) Handler {
	mgr := network.NewManager(cli)

	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p removeNetworkPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}

		target := p.NetworkID
		if target == "" {
			target = p.Name
		}
		if target == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "networkId or name is required")
		}

		// DeleteNetwork prevents accidental deletion if connected containers exist
		// and blocks modification of unmanaged networks
		if err := mgr.DeleteNetwork(ctx, target, p.OrganizationID); err != nil {
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{Output: map[string]any{
			"target":  target,
			"removed": true,
		}}, nil
	})
}

type inspectNetworkPayload struct {
	NetworkID string `json:"networkId"`
	Name      string `json:"name"`
}

func inspectNetworkHandler(cli *docker.Client) Handler {
	mgr := network.NewManager(cli)

	return HandlerFunc(func(ctx context.Context, payload map[string]any) (ExecutionResult, error) {
		var p inspectNetworkPayload
		if err := decodePayload(payload, &p); err != nil {
			return ExecutionResult{}, err
		}

		target := p.NetworkID
		if target == "" {
			target = p.Name
		}
		if target == "" {
			return ExecutionResult{}, Errorf(ErrCodeInvalidPayload, "networkId or name is required")
		}

		detail, ownership, err := mgr.InspectNetwork(ctx, target)
		if err != nil {
			return ExecutionResult{}, wrapDockerErr(err)
		}

		return ExecutionResult{Output: map[string]any{
			"networkId":           detail.ID,
			"name":                detail.Name,
			"driver":              detail.Driver,
			"scope":               detail.Scope,
			"internal":            detail.Internal,
			"ownership":           string(ownership),
			"connectedContainers": len(detail.Containers),
			"labels":              detail.Labels,
		}}, nil
	})
}
