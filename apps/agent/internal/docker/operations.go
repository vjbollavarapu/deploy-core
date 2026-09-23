package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/mount"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
	dockervolume "github.com/docker/docker/api/types/volume"
	"github.com/docker/docker/pkg/stdcopy"
	nat "github.com/docker/go-connections/nat"
)

const (
	defaultOpTimeout    = 30 * time.Second
	defaultPullTimeout  = 5 * time.Minute
	defaultBuildTimeout = 15 * time.Minute
	defaultLogsTimeout  = 0 // streaming — caller controls context
)

// withTimeout returns a new context with the given deadline, falling back to
// the parent if it already has an earlier deadline.
func withTimeout(parent context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, d)
}

// --------------------------------------------------------------------------
// Ping / Info / Version
// --------------------------------------------------------------------------

// Ping checks the Docker daemon is up and returns stable metadata.
func (c *Client) Ping(ctx context.Context) (PingResult, error) {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	p, err := c.cli.Ping(ctx)
	if err != nil {
		return PingResult{}, mapDockerError(err)
	}
	return PingResult{
		APIVersion:     p.APIVersion,
		OSType:         p.OSType,
		Experimental:   p.Experimental,
		BuilderVersion: string(p.BuilderVersion),
	}, nil
}

// Info returns high-level daemon information.
func (c *Client) Info(ctx context.Context) (InfoResult, error) {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	info, err := c.cli.Info(ctx)
	if err != nil {
		return InfoResult{}, mapDockerError(err)
	}
	return InfoResult{
		ServerVersion: info.ServerVersion,
		OS:            info.OperatingSystem,
		Architecture:  info.Architecture,
		CPUs:          info.NCPU,
		MemoryBytes:   info.MemTotal,
	}, nil
}

// Version returns the Docker daemon version.
func (c *Client) Version(ctx context.Context) (VersionResult, error) {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	v, err := c.cli.ServerVersion(ctx)
	if err != nil {
		return VersionResult{}, mapDockerError(err)
	}
	return VersionResult{
		Version:       v.Version,
		APIVersion:    v.APIVersion,
		GoVersion:     v.GoVersion,
		GitCommit:     v.GitCommit,
		OS:            v.Os,
		Architecture:  v.Arch,
		KernelVersion: v.KernelVersion,
	}, nil
}

// --------------------------------------------------------------------------
// Containers
// --------------------------------------------------------------------------

// ListContainers lists containers. If all is true, stopped containers are
// included.
func (c *Client) ListContainers(ctx context.Context, all bool) ([]ContainerSummary, error) {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	list, err := c.cli.ContainerList(ctx, types.ContainerListOptions{All: all})
	if err != nil {
		return nil, mapDockerError(err)
	}

	out := make([]ContainerSummary, 0, len(list))
	for _, c := range list {
		ports := make([]PortBinding, 0, len(c.Ports))
		for _, p := range c.Ports {
			ports = append(ports, PortBinding{
				IP:          p.IP,
				PrivatePort: p.PrivatePort,
				PublicPort:  p.PublicPort,
				Type:        p.Type,
			})
		}
		out = append(out, ContainerSummary{
			ID:      c.ID,
			Names:   c.Names,
			Image:   c.Image,
			ImageID: c.ImageID,
			Command: c.Command,
			State:   c.State,
			Status:  c.Status,
			Created: time.Unix(c.Created, 0).UTC(),
			Labels:  c.Labels,
			Ports:   ports,
		})
	}
	return out, nil
}

// InspectContainer returns full detail for a container by ID or name.
func (c *Client) InspectContainer(ctx context.Context, id string) (ContainerDetail, error) {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	info, err := c.cli.ContainerInspect(ctx, id)
	if err != nil {
		return ContainerDetail{}, mapDockerError(err)
	}

	created, _ := time.Parse(time.RFC3339Nano, info.Created)

	var startedAt, finishedAt *time.Time
	if info.State != nil {
		if t, err := time.Parse(time.RFC3339Nano, info.State.StartedAt); err == nil && !t.IsZero() {
			startedAt = &t
		}
		if t, err := time.Parse(time.RFC3339Nano, info.State.FinishedAt); err == nil && !t.IsZero() {
			finishedAt = &t
		}
	}

	mounts := make([]MountPoint, 0, len(info.Mounts))
	for _, m := range info.Mounts {
		mounts = append(mounts, MountPoint{
			Type:        string(m.Type),
			Source:      m.Source,
			Destination: m.Destination,
			Mode:        m.Mode,
			RW:          m.RW,
		})
	}

	st := ContainerState{}
	if info.State != nil {
		var health *ContainerHealth
		if info.State.Health != nil {
			health = &ContainerHealth{
				Status:        info.State.Health.Status,
				FailingStreak: info.State.Health.FailingStreak,
			}
		}
		st = ContainerState{
			Status:     info.State.Status,
			Running:    info.State.Running,
			Paused:     info.State.Paused,
			Restarting: info.State.Restarting,
			Dead:       info.State.Dead,
			Pid:        info.State.Pid,
			ExitCode:   info.State.ExitCode,
			Error:      info.State.Error,
			Health:     health,
		}
	}

	netMode := ""
	if info.HostConfig != nil {
		netMode = string(info.HostConfig.NetworkMode)
	}

	ipAddress := ""
	networks := make(map[string]string)
	if info.NetworkSettings != nil {
		ipAddress = info.NetworkSettings.IPAddress
		for netName, ep := range info.NetworkSettings.Networks {
			if ep != nil && ep.IPAddress != "" {
				networks[netName] = ep.IPAddress
				if ipAddress == "" {
					ipAddress = ep.IPAddress
				}
			}
		}
	}

	return ContainerDetail{
		ID:           info.ID,
		Name:         strings.TrimPrefix(info.Name, "/"),
		Image:        info.Config.Image,
		ImageID:      info.Image,
		State:        st,
		Created:      created,
		StartedAt:    startedAt,
		FinishedAt:   finishedAt,
		RestartCount: info.RestartCount,
		Labels:       info.Config.Labels,
		Env:          info.Config.Env,
		Mounts:       mounts,
		NetworkMode:  netMode,
		IPAddress:    ipAddress,
		Networks:     networks,
	}, nil
}

// CreateContainer validates and creates a container according to the full security policy.
// It returns a ValidationError for input problems, and a wrapped AgentError for Docker failures.
func (c *Client) CreateContainer(ctx context.Context, req CreateContainerRequest) (CreateContainerResult, error) {
	// Validate all inputs and enforce security policy before touching Docker.
	if err := validateCreateRequest(&req); err != nil {
		return CreateContainerResult{}, err
	}

	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	// --- Container config ---
	containerCfg := &container.Config{
		Image:      req.Image,
		Entrypoint: req.Entrypoint,
		Cmd:        req.Command,
		Labels:     mergeLabels(req.Labels, req.PlatformLabels, GenerateTraefikLabels(req.Traefik)),
	}

	// Env is passed through as-is; values are opaque and must not be logged.
	containerCfg.Env = req.Env

	// Exposed ports (internal only; no host binding from this config)
	exposedPorts := nat.PortSet{}
	portBindings := nat.PortMap{}
	for _, p := range req.InternalPorts {
		proto := p.Protocol
		if proto == "" {
			proto = "tcp"
		}
		natPort := nat.Port(fmt.Sprintf("%d/%s", p.ContainerPort, proto))
		exposedPorts[natPort] = struct{}{}
		if p.HostPort > 0 || p.HostIP != "" {
			hostIP := p.HostIP
			if hostIP == "" {
				hostIP = "0.0.0.0"
			}
			portBindings[natPort] = []nat.PortBinding{{
				HostIP:   hostIP,
				HostPort: fmt.Sprintf("%d", p.HostPort),
			}}
		}
	}
	containerCfg.ExposedPorts = exposedPorts

	// Health check
	if req.HealthCheck != nil && len(req.HealthCheck.Test) > 0 {
		hc := req.HealthCheck
		dockerHC := &container.HealthConfig{Test: hc.Test}
		if hc.Interval > 0 {
			dockerHC.Interval = hc.Interval
		}
		if hc.Timeout > 0 {
			dockerHC.Timeout = hc.Timeout
		}
		if hc.StartPeriod > 0 {
			dockerHC.StartPeriod = hc.StartPeriod
		}
		if hc.Retries > 0 {
			dockerHC.Retries = hc.Retries
		}
		containerCfg.Healthcheck = dockerHC
	}

	// --- Restart policy ---
	rp := container.RestartPolicy{}
	switch req.RestartPolicy {
	case RestartAlways:
		rp.Name = "always"
	case RestartOnFailure:
		rp.Name = "on-failure"
	case RestartUnlessStopped:
		rp.Name = "unless-stopped"
	default:
		rp.Name = "no"
	}

	// --- Volume mounts (named volumes only) ---
	mounts := make([]mount.Mount, 0, len(req.Volumes))
	for _, v := range req.Volumes {
		m := mount.Mount{
			Type:     mount.TypeVolume,
			Source:   v.VolumeName,
			Target:   v.MountPath,
			ReadOnly: v.ReadOnly,
		}
		mounts = append(mounts, m)
	}

	// --- Resource limits ---
	// CPUMillis → Docker nano-CPUs: 1000 millis = 1e9 nano-CPUs
	var nanoCPUs int64
	if req.CPUMillis > 0 {
		nanoCPUs = req.CPUMillis * 1_000_000 // millis → nanos
	}

	// --- Security / capability config ---
	policy := req.Policy
	capDrop := effectiveDropCaps(policy)
	var capAdd []string
	if policy != nil {
		capAdd = policy.AddCapabilities
	}

	hostCfg := &container.HostConfig{
		PortBindings:   portBindings,
		RestartPolicy:  rp,
		Mounts:         mounts,
		ReadonlyRootfs: req.ReadOnlyRootFS,
		Resources: container.Resources{
			Memory:   req.MemoryBytes,
			NanoCPUs: nanoCPUs,
		},
		CapAdd:  capAdd,
		CapDrop: capDrop,
		// Security defaults — all blocked unless policy explicitly overrides.
		// Note: AllowPrivileged / HostPID / HostIPC hooks exist in policy but are
		// intentionally NOT wired to Privileged/PidMode/IpcMode here — they
		// always return an error in validateSecurityPolicy for now.
		Privileged: false,
		PidMode:    "",
		IpcMode:    "",
	}

	// --- Network config ---
	// First network is set in HostConfig.NetworkMode; additional networks are
	// connected after container creation.
	networkCfg := &network.NetworkingConfig{}
	if len(req.Networks) > 0 {
		hostCfg.NetworkMode = container.NetworkMode(req.Networks[0])
		if len(req.Networks) > 1 {
			eps := make(map[string]*network.EndpointSettings, len(req.Networks)-1)
			for _, n := range req.Networks[1:] {
				eps[n] = &network.EndpointSettings{}
			}
			networkCfg.EndpointsConfig = eps
		}
	}

	resp, err := c.cli.ContainerCreate(ctx, containerCfg, hostCfg, networkCfg, nil, req.Name)
	if err != nil {
		return CreateContainerResult{}, mapDockerError(err)
	}
	return CreateContainerResult{ID: resp.ID, Warnings: resp.Warnings}, nil
}

// StartContainer starts a container by ID.
func (c *Client) StartContainer(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	err := c.cli.ContainerStart(ctx, id, types.ContainerStartOptions{})
	return mapDockerError(err)
}

// StopContainer stops a running container. timeout is the grace period
// before the daemon sends SIGKILL.
func (c *Client) StopContainer(ctx context.Context, id string, timeout time.Duration) error {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout+timeout)
	defer cancel()

	sec := int(timeout.Seconds())
	err := c.cli.ContainerStop(ctx, id, container.StopOptions{Timeout: &sec})
	return mapDockerError(err)
}

// RestartContainer restarts a container.
func (c *Client) RestartContainer(ctx context.Context, id string, timeout time.Duration) error {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout+timeout)
	defer cancel()

	sec := int(timeout.Seconds())
	err := c.cli.ContainerRestart(ctx, id, container.StopOptions{Timeout: &sec})
	return mapDockerError(err)
}

// RemoveContainer removes a container. force removes it even if running.
func (c *Client) RemoveContainer(ctx context.Context, id string, force bool) error {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	err := c.cli.ContainerRemove(ctx, id, types.ContainerRemoveOptions{
		Force:         force,
		RemoveVolumes: false,
	})
	return mapDockerError(err)
}

// ExecContainer executes a command inside a running container and waits for completion.
func (c *Client) ExecContainer(ctx context.Context, id string, cmd []string) (ExecResult, error) {
	if len(cmd) == 0 {
		return ExecResult{}, errors.New("exec command cannot be empty")
	}

	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	execCfg := types.ExecConfig{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := c.cli.ContainerExecCreate(ctx, id, execCfg)
	if err != nil {
		return ExecResult{}, mapDockerError(err)
	}

	attachResp, err := c.cli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{})
	if err != nil {
		return ExecResult{}, mapDockerError(err)
	}
	defer attachResp.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	_, _ = stdcopy.StdCopy(&stdoutBuf, &stderrBuf, attachResp.Reader)

	inspectResp, err := c.cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return ExecResult{}, mapDockerError(err)
	}

	return ExecResult{
		ExitCode: inspectResp.ExitCode,
		Stdout:   stdoutBuf.String(),
		Stderr:   stderrBuf.String(),
	}, nil
}

// ExecContainerWithIO executes a command inside a running container with custom env and streaming stdin/stdout/stderr.
func (c *Client) ExecContainerWithIO(ctx context.Context, id string, cmd []string, env []string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
	if len(cmd) == 0 {
		return -1, errors.New("exec command cannot be empty")
	}

	execCfg := types.ExecConfig{
		Cmd:          cmd,
		Env:          env,
		AttachStdin:  stdin != nil,
		AttachStdout: true,
		AttachStderr: true,
	}

	execResp, err := c.cli.ContainerExecCreate(ctx, id, execCfg)
	if err != nil {
		return -1, mapDockerError(err)
	}

	attachResp, err := c.cli.ContainerExecAttach(ctx, execResp.ID, types.ExecStartCheck{})
	if err != nil {
		return -1, mapDockerError(err)
	}
	defer attachResp.Close()

	if stdin != nil {
		go func() {
			defer attachResp.CloseWrite()
			_, _ = io.Copy(attachResp.Conn, stdin)
		}()
	}

	outW := stdout
	if outW == nil {
		outW = io.Discard
	}
	errW := stderr
	if errW == nil {
		errW = io.Discard
	}

	_, copyErr := stdcopy.StdCopy(outW, errW, attachResp.Reader)
	if copyErr != nil && ctx.Err() != nil {
		return -1, ctx.Err()
	}

	inspectResp, err := c.cli.ContainerExecInspect(ctx, execResp.ID)
	if err != nil {
		return -1, mapDockerError(err)
	}

	return inspectResp.ExitCode, nil
}

// --------------------------------------------------------------------------
// Images
// --------------------------------------------------------------------------

// ListImages returns all local images.
func (c *Client) ListImages(ctx context.Context, all bool) ([]ImageSummary, error) {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	list, err := c.cli.ImageList(ctx, types.ImageListOptions{All: all})
	if err != nil {
		return nil, mapDockerError(err)
	}

	out := make([]ImageSummary, 0, len(list))
	for _, img := range list {
		out = append(out, ImageSummary{
			ID:          img.ID,
			ParentID:    img.ParentID,
			RepoTags:    img.RepoTags,
			RepoDigests: img.RepoDigests,
			Created:     time.Unix(img.Created, 0).UTC(),
			Size:        img.Size,
			Labels:      img.Labels,
		})
	}
	return out, nil
}

// InspectImage returns full detail for an image by ID or name.
func (c *Client) InspectImage(ctx context.Context, id string) (ImageDetail, error) {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	img, _, err := c.cli.ImageInspectWithRaw(ctx, id)
	if err != nil {
		return ImageDetail{}, mapDockerError(err)
	}

	created, _ := time.Parse(time.RFC3339, img.Created)

	return ImageDetail{
		ID:           img.ID,
		RepoTags:     img.RepoTags,
		RepoDigests:  img.RepoDigests,
		Parent:       img.Parent,
		Created:      created,
		Size:         img.Size,
		Architecture: img.Architecture,
		OS:           img.Os,
	}, nil
}

// PullImageWithOptions pulls an image from a public or authenticated registry,
// streams progress events to the optional ProgressFn, and returns a structured result.
// Credentials passed in opts.RegistryAuth are zeroed out immediately after encoding
// to ensure passwords are not retained in memory.
func (c *Client) PullImageWithOptions(ctx context.Context, opts PullImageOptions) (PullImageResult, error) {
	if strings.TrimSpace(opts.Ref) == "" {
		return PullImageResult{}, &AgentError{Code: ErrCodeImagePullFailed, Message: "image reference cannot be empty"}
	}

	pullOptions := types.ImagePullOptions{}
	if opts.RegistryAuth != nil {
		authCfg := registry.AuthConfig{
			Username:      opts.RegistryAuth.Username,
			Password:      opts.RegistryAuth.Password,
			ServerAddress: opts.RegistryAuth.ServerAddress,
			IdentityToken: opts.RegistryAuth.IdentityToken,
			RegistryToken: opts.RegistryAuth.RegistryToken,
		}
		authStr, err := registry.EncodeAuthConfig(authCfg)
		// Immediately scrub credentials from memory
		opts.RegistryAuth.Zero()
		authCfg.Password = ""
		authCfg.IdentityToken = ""
		authCfg.RegistryToken = ""
		if err != nil {
			return PullImageResult{}, &AgentError{Code: ErrCodeImagePullFailed, Message: fmt.Sprintf("failed to encode registry auth: %v", err)}
		}
		pullOptions.RegistryAuth = authStr
	}

	start := time.Now()
	pullCtx, cancel := withTimeout(ctx, defaultPullTimeout)
	defer cancel()

	reader, err := c.cli.ImagePull(pullCtx, opts.Ref, pullOptions)
	if err != nil {
		return PullImageResult{}, &AgentError{Code: ErrCodeImagePullFailed, Message: err.Error()}
	}
	defer reader.Close()

	var streamDigest string
	var streamStatus string
	decoder := json.NewDecoder(reader)

	for {
		var msg struct {
			Status         string `json:"status"`
			ID             string `json:"id"`
			Progress       string `json:"progress"`
			ProgressDetail struct {
				Current int64 `json:"current"`
				Total   int64 `json:"total"`
			} `json:"progressDetail"`
			Error        string `json:"error"`
			ErrorMessage string `json:"errorMessage"`
		}

		if err := decoder.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if pullCtx.Err() != nil {
				return PullImageResult{}, mapDockerError(pullCtx.Err())
			}
			break
		}

		if msg.Error != "" || msg.ErrorMessage != "" {
			errMsg := msg.ErrorMessage
			if errMsg == "" {
				errMsg = msg.Error
			}
			return PullImageResult{}, &AgentError{Code: ErrCodeImagePullFailed, Message: errMsg}
		}

		if strings.HasPrefix(msg.Status, "Digest: ") {
			streamDigest = strings.TrimSpace(strings.TrimPrefix(msg.Status, "Digest: "))
		}
		if msg.Status != "" {
			streamStatus = msg.Status
		}

		if opts.ProgressFn != nil {
			opts.ProgressFn(PullProgressEvent{
				ID:        msg.ID,
				Status:    msg.Status,
				Progress:  msg.Progress,
				Current:   msg.ProgressDetail.Current,
				Total:     msg.ProgressDetail.Total,
				Timestamp: time.Now().UTC(),
			})
		}
	}

	duration := time.Since(start)

	// Inspect image to retrieve canonical ImageID and Size
	var imageID string
	var size int64
	if inspect, err := c.InspectImage(pullCtx, opts.Ref); err == nil {
		imageID = inspect.ID
		size = inspect.Size
		if streamDigest == "" && len(inspect.RepoDigests) > 0 {
			for _, rd := range inspect.RepoDigests {
				parts := strings.Split(rd, "@")
				if len(parts) == 2 {
					streamDigest = parts[1]
					break
				}
			}
		}
	}

	status := "pulled"
	if strings.Contains(streamStatus, "Image is up to date") {
		status = "already_up_to_date"
	}

	return PullImageResult{
		Digest:         streamDigest,
		ImageID:        imageID,
		Size:           size,
		PullDuration:   duration,
		PullDurationMs: duration.Milliseconds(),
		Status:         status,
	}, nil
}

// PullImage pulls an image from a registry and optionally writes progress to out.
// Returns when the pull completes or ctx is cancelled.
func (c *Client) PullImage(ctx context.Context, ref string, out io.Writer) error {
	var progressFn PullProgressFunc
	if out != nil {
		progressFn = func(ev PullProgressEvent) {
			_, _ = fmt.Fprintf(out, "%s %s %s\n", ev.ID, ev.Status, ev.Progress)
		}
	}
	_, err := c.PullImageWithOptions(ctx, PullImageOptions{
		Ref:        ref,
		ProgressFn: progressFn,
	})
	return err
}

// BuildImage builds a Docker image from a tar context stream and options.
// It parses the build stream, invokes ProgressFn for streaming output,
// handles cancellation, and returns canonical build metadata.
func (c *Client) BuildImage(ctx context.Context, opts BuildImageOptions) (BuildImageResult, error) {
	if opts.Context == nil {
		return BuildImageResult{}, &AgentError{Code: ErrCodeImagePullFailed, Message: "build context is required"}
	}

	dockerfile := opts.Dockerfile
	if dockerfile == "" {
		dockerfile = "Dockerfile"
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultBuildTimeout
	}
	buildCtx, cancel := withTimeout(ctx, timeout)
	defer cancel()

	buildOpts := types.ImageBuildOptions{
		Dockerfile:  dockerfile,
		Tags:        opts.Tags,
		BuildArgs:   opts.BuildArgs,
		Target:      opts.Target,
		Platform:    opts.Platform,
		NoCache:     opts.NoCache,
		PullParent:  opts.PullParent,
		Remove:      true,
		ForceRemove: true,
	}

	start := time.Now()
	resp, err := c.cli.ImageBuild(buildCtx, opts.Context, buildOpts)
	if err != nil {
		return BuildImageResult{}, mapDockerError(err)
	}
	defer resp.Body.Close()

	var imageID string
	decoder := json.NewDecoder(resp.Body)

	for {
		var msg struct {
			Stream       string `json:"stream"`
			Error        string `json:"error"`
			ErrorMessage string `json:"errorMessage"`
			Aux          struct {
				ID string `json:"ID"`
			} `json:"aux"`
		}

		if err := decoder.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			if buildCtx.Err() != nil {
				return BuildImageResult{}, mapDockerError(buildCtx.Err())
			}
			break
		}

		if msg.Error != "" || msg.ErrorMessage != "" {
			errMsg := msg.ErrorMessage
			if errMsg == "" {
				errMsg = msg.Error
			}
			return BuildImageResult{}, &AgentError{Code: ErrCodeImagePullFailed, Message: errMsg}
		}

		if msg.Aux.ID != "" {
			imageID = msg.Aux.ID
		}

		if opts.ProgressFn != nil {
			opts.ProgressFn(BuildProgressEvent{
				Stream:    msg.Stream,
				AuxID:     msg.Aux.ID,
				Error:     msg.Error,
				Timestamp: time.Now().UTC(),
			})
		}
	}

	duration := time.Since(start)

	var size int64
	var digest string
	if imageID != "" {
		if inspect, err := c.InspectImage(buildCtx, imageID); err == nil {
			size = inspect.Size
			if len(inspect.RepoDigests) > 0 {
				parts := strings.Split(inspect.RepoDigests[0], "@")
				if len(parts) == 2 {
					digest = parts[1]
				}
			}
		}
	}

	return BuildImageResult{
		ImageID:         imageID,
		Digest:          digest,
		Size:            size,
		BuildDuration:   duration,
		BuildDurationMs: duration.Milliseconds(),
		Tags:            opts.Tags,
		Status:          "built",
	}, nil
}

// RemoveImage removes an image. If force is true removes it even if tagged by
// running containers.
func (c *Client) RemoveImage(ctx context.Context, id string, force bool) error {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	_, err := c.cli.ImageRemove(ctx, id, types.ImageRemoveOptions{Force: force})
	return mapDockerError(err)
}

// --------------------------------------------------------------------------
// Networks
// --------------------------------------------------------------------------

// ListNetworks returns all Docker networks.
func (c *Client) ListNetworks(ctx context.Context) ([]NetworkSummary, error) {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	list, err := c.cli.NetworkList(ctx, types.NetworkListOptions{})
	if err != nil {
		return nil, mapDockerError(err)
	}

	out := make([]NetworkSummary, 0, len(list))
	for _, n := range list {
		out = append(out, NetworkSummary{
			ID:     n.ID,
			Name:   n.Name,
			Driver: n.Driver,
			Scope:  n.Scope,
			Labels: n.Labels,
		})
	}
	return out, nil
}

// CreateNetwork creates a new Docker network.
func (c *Client) CreateNetwork(ctx context.Context, req CreateNetworkRequest) (string, error) {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	resp, err := c.cli.NetworkCreate(ctx, req.Name, types.NetworkCreate{
		Driver:   req.Driver,
		Internal: req.Internal,
		Labels:   req.Labels,
		Options:  req.Options,
	})
	if err != nil {
		return "", mapDockerError(err)
	}
	if resp.Warning != "" {
		// Non-fatal; log at call site if needed.
		_ = resp.Warning
	}
	return resp.ID, nil
}

// RemoveNetwork removes a Docker network by ID or name.
func (c *Client) RemoveNetwork(ctx context.Context, id string) error {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	return mapDockerError(c.cli.NetworkRemove(ctx, id))
}

// InspectNetwork inspects a Docker network by ID or name.
func (c *Client) InspectNetwork(ctx context.Context, idOrName string) (NetworkDetail, error) {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	nr, err := c.cli.NetworkInspect(ctx, idOrName, types.NetworkInspectOptions{})
	if err != nil {
		return NetworkDetail{}, mapDockerError(err)
	}

	endpoints := make(map[string]NetworkEndpoint, len(nr.Containers))
	for containerID, ep := range nr.Containers {
		endpoints[containerID] = NetworkEndpoint{
			Name:        ep.Name,
			EndpointID:  ep.EndpointID,
			MacAddress:  ep.MacAddress,
			IPv4Address: ep.IPv4Address,
			IPv6Address: ep.IPv6Address,
		}
	}

	return NetworkDetail{
		ID:         nr.ID,
		Name:       nr.Name,
		Driver:     nr.Driver,
		Scope:      nr.Scope,
		Internal:   nr.Internal,
		Attachable: nr.Attachable,
		Labels:     nr.Labels,
		Containers: endpoints,
		Options:    nr.Options,
	}, nil
}

// ConnectNetwork connects a container to a Docker network.
// If the container is already connected, it returns nil (idempotent).
func (c *Client) ConnectNetwork(ctx context.Context, networkID string, containerID string) error {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	err := c.cli.NetworkConnect(ctx, networkID, containerID, nil)
	if err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "already exists in network") || strings.Contains(errStr, "already attached") {
			return nil
		}
		return mapDockerError(err)
	}
	return nil
}

// DisconnectNetwork disconnects a container from a Docker network.
// If the container is not connected, it returns nil (idempotent).
func (c *Client) DisconnectNetwork(ctx context.Context, networkID string, containerID string, force bool) error {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	err := c.cli.NetworkDisconnect(ctx, networkID, containerID, force)
	if err != nil {
		errStr := strings.ToLower(err.Error())
		if strings.Contains(errStr, "is not connected to network") || strings.Contains(errStr, "not in network") {
			return nil
		}
		return mapDockerError(err)
	}
	return nil
}

// --------------------------------------------------------------------------
// Volumes
// --------------------------------------------------------------------------

// ListVolumes returns all Docker volumes.
func (c *Client) ListVolumes(ctx context.Context) ([]VolumeSummary, error) {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	resp, err := c.cli.VolumeList(ctx, dockervolume.ListOptions{})
	if err != nil {
		return nil, mapDockerError(err)
	}

	out := make([]VolumeSummary, 0, len(resp.Volumes))
	for _, v := range resp.Volumes {
		out = append(out, VolumeSummary{
			Name:       v.Name,
			Driver:     v.Driver,
			Mountpoint: v.Mountpoint,
			Labels:     v.Labels,
			Scope:      v.Scope,
		})
	}
	return out, nil
}

// CreateVolume creates a new Docker volume.
func (c *Client) CreateVolume(ctx context.Context, req CreateVolumeRequest) (VolumeSummary, error) {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	v, err := c.cli.VolumeCreate(ctx, dockervolume.CreateOptions{
		Name:       req.Name,
		Driver:     req.Driver,
		DriverOpts: req.DriverOpts,
		Labels:     req.Labels,
	})
	if err != nil {
		return VolumeSummary{}, mapDockerError(err)
	}
	return VolumeSummary{
		Name:       v.Name,
		Driver:     v.Driver,
		Mountpoint: v.Mountpoint,
		Labels:     v.Labels,
		Scope:      v.Scope,
	}, nil
}

// RemoveVolume removes a Docker volume by name.
func (c *Client) RemoveVolume(ctx context.Context, name string, force bool) error {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	return mapDockerError(c.cli.VolumeRemove(ctx, name, force))
}

// InspectVolume inspects a Docker volume by name and retrieves usage data where available.
func (c *Client) InspectVolume(ctx context.Context, name string) (VolumeDetail, error) {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	v, err := c.cli.VolumeInspect(ctx, name)
	if err != nil {
		return VolumeDetail{}, mapDockerError(err)
	}

	var usage *VolumeUsage
	if v.UsageData != nil {
		usage = &VolumeUsage{
			SizeBytes: v.UsageData.Size,
			RefCount:  v.UsageData.RefCount,
		}
	}

	return VolumeDetail{
		Name:       v.Name,
		Driver:     v.Driver,
		Mountpoint: v.Mountpoint,
		CreatedAt:  v.CreatedAt,
		Labels:     v.Labels,
		Scope:      v.Scope,
		Options:    v.Options,
		Status:     v.Status,
		Usage:      usage,
	}, nil
}

// --------------------------------------------------------------------------
// Streaming: Logs, Stats, Events
// --------------------------------------------------------------------------

// StreamLogs streams container logs to the provided writer until ctx is
// cancelled or the stream closes. The caller is responsible for closing out.
func (c *Client) StreamLogs(ctx context.Context, id string, opts LogOptions, out io.Writer) error {
	logOpts := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     opts.Follow,
		Since:      opts.Since,
		Until:      opts.Until,
		Timestamps: opts.Timestamps,
		Details:    false,
	}
	if opts.Tail != "" {
		logOpts.Tail = opts.Tail
	} else {
		logOpts.Tail = "100"
	}

	reader, err := c.cli.ContainerLogs(ctx, id, logOpts)
	if err != nil {
		return mapDockerError(err)
	}
	defer reader.Close()

	if _, err := io.Copy(out, reader); err != nil && ctx.Err() == nil {
		return mapDockerError(err)
	}
	return nil
}

// OpenLogStream opens an io.ReadCloser streaming container logs from Docker.
// The caller must ensure reader.Close() is called when done.
func (c *Client) OpenLogStream(ctx context.Context, id string, opts LogOptions) (io.ReadCloser, error) {
	logOpts := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Follow:     opts.Follow,
		Since:      opts.Since,
		Until:      opts.Until,
		Timestamps: opts.Timestamps,
		Details:    false,
	}
	if opts.Tail != "" {
		logOpts.Tail = opts.Tail
	} else {
		logOpts.Tail = "100"
	}

	reader, err := c.cli.ContainerLogs(ctx, id, logOpts)
	if err != nil {
		return nil, mapDockerError(err)
	}
	return reader, nil
}

// ContainerStats returns a single point-in-time stats snapshot for a container.
// It performs one-shot (non-streaming) stats collection.
func (c *Client) ContainerStats(ctx context.Context, id string) (ContainerStatsSnapshot, error) {
	ctx, cancel := withTimeout(ctx, defaultOpTimeout)
	defer cancel()

	resp, err := c.cli.ContainerStats(ctx, id, false)
	if err != nil {
		return ContainerStatsSnapshot{}, mapDockerError(err)
	}
	defer resp.Body.Close()

	var raw types.StatsJSON
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return ContainerStatsSnapshot{}, &AgentError{Code: ErrCodeUnknown, Message: "failed to decode stats: " + err.Error()}
	}

	cpuPercent := calcCPUPercent(&raw)

	var rxBytes, txBytes int64
	for _, ns := range raw.Networks {
		rxBytes += int64(ns.RxBytes)
		txBytes += int64(ns.TxBytes)
	}

	var blkRead, blkWrite int64
	for _, bs := range raw.BlkioStats.IoServiceBytesRecursive {
		switch bs.Op {
		case "Read":
			blkRead += int64(bs.Value)
		case "Write":
			blkWrite += int64(bs.Value)
		}
	}

	return ContainerStatsSnapshot{
		ID:          raw.ID,
		CPUPercent:  cpuPercent,
		MemoryUsage: int64(raw.MemoryStats.Usage),
		MemoryLimit: int64(raw.MemoryStats.Limit),
		NetworkRx:   rxBytes,
		NetworkTx:   txBytes,
		BlockRead:   blkRead,
		BlockWrite:  blkWrite,
		PidsCurrent: int64(raw.PidsStats.Current),
	}, nil
}

// calcCPUPercent calculates the CPU usage percent from raw Docker stats.
func calcCPUPercent(stats *types.StatsJSON) float64 {
	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) -
		float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage) -
		float64(stats.PreCPUStats.SystemUsage)
	cpuCount := float64(stats.CPUStats.OnlineCPUs)
	if cpuCount == 0 {
		cpuCount = float64(len(stats.CPUStats.CPUUsage.PercpuUsage))
	}
	if systemDelta <= 0 || cpuDelta < 0 {
		return 0
	}
	return (cpuDelta / systemDelta) * cpuCount * 100.0
}

// DockerEvents streams Docker daemon events to the provided channel until ctx
// is cancelled. The channel is closed when the function returns. The caller
// must drain the channel promptly to avoid blocking the event loop.
func (c *Client) DockerEvents(ctx context.Context, eventCh chan<- Event, errCh chan<- error) {
	f := filters.NewArgs()
	msgCh, errsCh := c.cli.Events(ctx, types.EventsOptions{Filters: f})

	go func() {
		defer close(eventCh)
		for {
			select {
			case <-ctx.Done():
				return
			case err, ok := <-errsCh:
				if !ok {
					return
				}
				if ctx.Err() != nil {
					return
				}
				select {
				case errCh <- mapDockerError(err):
				default:
				}
				return
			case msg, ok := <-msgCh:
				if !ok {
					return
				}
				evt := Event{
					Type:    string(msg.Type),
					Action:  msg.Action,
					ActorID: msg.Actor.ID,
					Time:    time.Unix(msg.Time, 0).UTC(),
					Scope:   msg.Scope,
					Attrs:   msg.Actor.Attributes,
				}
				select {
				case eventCh <- evt:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
}
