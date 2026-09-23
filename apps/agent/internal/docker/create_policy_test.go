package docker

import (
	"strings"
	"testing"
	"time"
)

func TestValidateCreateRequest_Valid(t *testing.T) {
	req := &CreateContainerRequest{
		Name:          "web-app-123",
		Image:         "ghcr.io/org/repo:v1.2.3",
		Entrypoint:    []string{"/bin/app"},
		Command:       []string{"serve", "--port", "8080"},
		Env:           []string{"APP_ENV=production", "PORT=8080", "SECRET_KEY=supersecret"},
		CPUMillis:     2000,
		MemoryBytes:   512 * 1024 * 1024,
		RestartPolicy: RestartUnlessStopped,
		InternalPorts: []PortMapping{
			{ContainerPort: 8080, Protocol: "tcp"},
		},
		Networks: []string{"app-net"},
		Volumes: []VolumeMount{
			{VolumeName: "app-data", MountPath: "/data", ReadOnly: false},
		},
		PlatformLabels: map[string]string{
			"deploycore.app_id": "app-123",
		},
		HealthCheck: &HealthCheckConfig{
			Test:        []string{"CMD-SHELL", "curl -f http://localhost:8080/health || exit 1"},
			Interval:    15 * time.Second,
			Timeout:     5 * time.Second,
			StartPeriod: 10 * time.Second,
			Retries:     3,
		},
		ReadOnlyRootFS: true,
	}

	if err := validateCreateRequest(req); err != nil {
		t.Fatalf("expected valid request, got error: %v", err)
	}
}

func TestValidateCreateRequest_NameViolations(t *testing.T) {
	tests := []struct {
		name      string
		cname     string
		expectErr string
	}{
		{"uppercase", "MyContainer", "name: must be lowercase alphanumeric"},
		{"spaces", "my container", "name: must be lowercase alphanumeric"},
		{"leading hyphen", "-mycontainer", "name: must be lowercase alphanumeric"},
		{"trailing hyphen", "mycontainer-", "name: must be lowercase alphanumeric"},
		{"special symbols", "my_container!", "name: must be lowercase alphanumeric"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &CreateContainerRequest{
				Name:  tc.cname,
				Image: "alpine:latest",
			}
			err := validateCreateRequest(req)
			if err == nil {
				t.Fatalf("expected error for container name %q, got nil", tc.cname)
			}
			if !strings.Contains(err.Error(), tc.expectErr) {
				t.Errorf("expected error containing %q, got %v", tc.expectErr, err)
			}
		})
	}
}

func TestValidateCreateRequest_ImageViolations(t *testing.T) {
	tests := []struct {
		name      string
		image     string
		expectErr string
	}{
		{"empty image", "", "image: required"},
		{"invalid characters", "alpine; rm -rf /", "image: invalid reference format"},
		{"spaces", "my image:latest", "image: invalid reference format"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &CreateContainerRequest{
				Name:  "my-app",
				Image: tc.image,
			}
			err := validateCreateRequest(req)
			if err == nil {
				t.Fatalf("expected error for image %q, got nil", tc.image)
			}
			if !strings.Contains(err.Error(), tc.expectErr) {
				t.Errorf("expected error containing %q, got %v", tc.expectErr, err)
			}
		})
	}
}

func TestValidateCreateRequest_EnvViolations(t *testing.T) {
	tests := []struct {
		name      string
		env       []string
		expectErr string
	}{
		{"no equals", []string{"INVALID_ENV"}, "must be KEY=VALUE format"},
		{"invalid key name", []string{"123_INVALID=val"}, "invalid key"},
		{"special char in key", []string{"BAD-KEY=val"}, "invalid key"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &CreateContainerRequest{
				Name:  "my-app",
				Image: "alpine:latest",
				Env:   tc.env,
			}
			err := validateCreateRequest(req)
			if err == nil {
				t.Fatalf("expected error for env %v, got nil", tc.env)
			}
			if !strings.Contains(err.Error(), tc.expectErr) {
				t.Errorf("expected error containing %q, got %v", tc.expectErr, err)
			}
		})
	}
}

func TestValidateCreateRequest_PortViolations(t *testing.T) {
	tests := []struct {
		name      string
		ports     []PortMapping
		expectErr string
	}{
		{"zero container port", []PortMapping{{ContainerPort: 0}}, "containerPort is required"},
		{"invalid protocol", []PortMapping{{ContainerPort: 8080, Protocol: "http"}}, "protocol must be 'tcp' or 'udp'"},
		{"privileged host port", []PortMapping{{ContainerPort: 8080, HostPort: 80}}, "privileged port"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &CreateContainerRequest{
				Name:          "my-app",
				Image:         "alpine:latest",
				InternalPorts: tc.ports,
			}
			err := validateCreateRequest(req)
			if err == nil {
				t.Fatalf("expected error for ports %v, got nil", tc.ports)
			}
			if !strings.Contains(err.Error(), tc.expectErr) {
				t.Errorf("expected error containing %q, got %v", tc.expectErr, err)
			}
		})
	}
}

func TestValidateCreateRequest_ResourceLimitViolations(t *testing.T) {
	tests := []struct {
		name        string
		cpuMillis   int64
		memoryBytes int64
		expectErr   string
	}{
		{"negative cpu", -1, 0, "cpuMillis: must be >= 0"},
		{"cpu exceeds max", 129_000, 0, "cpuMillis: exceeds maximum allowed"},
		{"negative memory", 0, -1, "memoryBytes: must be >= 0"},
		{"memory below minimum", 0, 1024, "memoryBytes: minimum is 4 MiB"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &CreateContainerRequest{
				Name:        "my-app",
				Image:       "alpine:latest",
				CPUMillis:   tc.cpuMillis,
				MemoryBytes: tc.memoryBytes,
			}
			err := validateCreateRequest(req)
			if err == nil {
				t.Fatalf("expected error for limits cpu=%d mem=%d, got nil", tc.cpuMillis, tc.memoryBytes)
			}
			if !strings.Contains(err.Error(), tc.expectErr) {
				t.Errorf("expected error containing %q, got %v", tc.expectErr, err)
			}
		})
	}
}

func TestValidateCreateRequest_RestartPolicy(t *testing.T) {
	req := &CreateContainerRequest{
		Name:          "my-app",
		Image:         "alpine:latest",
		RestartPolicy: "invalid-policy",
	}
	err := validateCreateRequest(req)
	if err == nil || !strings.Contains(err.Error(), "unknown value") {
		t.Fatalf("expected unknown restart policy error, got %v", err)
	}
}

func TestValidateCreateRequest_VolumeSecurity(t *testing.T) {
	tests := []struct {
		name      string
		volumes   []VolumeMount
		expectErr string
	}{
		{"empty volume name", []VolumeMount{{VolumeName: "", MountPath: "/data"}}, "volumeName is required"},
		{"empty mount path", []VolumeMount{{VolumeName: "data", MountPath: ""}}, "mountPath is required"},
		{"relative mount path", []VolumeMount{{VolumeName: "data", MountPath: "relative/path"}}, "mountPath must be an absolute path"},
		{"docker socket mount", []VolumeMount{{VolumeName: "/var/run/docker.sock", MountPath: "/var/run/docker.sock"}}, "Docker socket / raw host path mounts are blocked"},
		{"alternate docker sock", []VolumeMount{{VolumeName: "/run/docker.sock", MountPath: "/docker.sock"}}, "Docker socket / raw host path mounts are blocked"},
		{"raw root host mount", []VolumeMount{{VolumeName: "/etc", MountPath: "/etc"}}, "Docker socket / raw host path mounts are blocked"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &CreateContainerRequest{
				Name:    "my-app",
				Image:   "alpine:latest",
				Volumes: tc.volumes,
			}
			err := validateCreateRequest(req)
			if err == nil {
				t.Fatalf("expected error for volume %v, got nil", tc.volumes)
			}
			if !strings.Contains(err.Error(), tc.expectErr) {
				t.Errorf("expected error containing %q, got %v", tc.expectErr, err)
			}
		})
	}
}

func TestValidateCreateRequest_BlockedHostNetwork(t *testing.T) {
	req := &CreateContainerRequest{
		Name:     "my-app",
		Image:    "alpine:latest",
		Networks: []string{"host"},
	}
	err := validateCreateRequest(req)
	if err == nil || !strings.Contains(err.Error(), "host network is blocked by default security policy") {
		t.Fatalf("expected host network blocked error, got %v", err)
	}
}

func TestValidateCreateRequest_DeniedLabels(t *testing.T) {
	denied := []string{
		"io.kubernetes.pod.name",
		"com.docker.swarm.service.name",
		"com.docker.stack.namespace",
		"com.docker.compose.project",
		"org.opencontainers.image.revision",
		"deploycore.managed",
		"deploycore.organization_id",
		"traefik.http.routers.spoof.rule",
	}

	for _, label := range denied {
		t.Run(label, func(t *testing.T) {
			req := &CreateContainerRequest{
				Name:  "my-app",
				Image: "alpine:latest",
				Labels: map[string]string{
					label: "value",
				},
			}
			err := validateCreateRequest(req)
			if err == nil || !strings.Contains(err.Error(), "is not allowed") {
				t.Fatalf("expected denied label error for %q, got %v", label, err)
			}
		})
	}
}

func TestValidateCreateRequest_PrivilegedPolicyRestrictions(t *testing.T) {
	tests := []struct {
		name      string
		policy    *PrivilegedPolicy
		expectErr string
	}{
		{
			name:      "privileged mode blocked",
			policy:    &PrivilegedPolicy{AllowPrivileged: true},
			expectErr: "privileged mode requires explicit admin grant",
		},
		{
			name:      "host pid blocked",
			policy:    &PrivilegedPolicy{AllowHostPID: true},
			expectErr: "host PID namespace requires explicit admin grant",
		},
		{
			name:      "host ipc blocked",
			policy:    &PrivilegedPolicy{AllowHostIPC: true},
			expectErr: "host IPC namespace requires explicit admin grant",
		},
		{
			name:      "device passthrough blocked",
			policy:    &PrivilegedPolicy{AllowDevicePassthrough: true},
			expectErr: "device passthrough requires explicit admin grant",
		},
		{
			name: "unauthorized capability",
			policy: &PrivilegedPolicy{
				AddCapabilities: []string{"SYS_ADMIN"},
			},
			expectErr: "capability \"SYS_ADMIN\" is not in the permitted add-capability list",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &CreateContainerRequest{
				Name:   "my-app",
				Image:  "alpine:latest",
				Policy: tc.policy,
			}
			err := validateCreateRequest(req)
			if err == nil {
				t.Fatalf("expected error for policy %v, got nil", tc.policy)
			}
			if !strings.Contains(err.Error(), tc.expectErr) {
				t.Errorf("expected error containing %q, got %v", tc.expectErr, err)
			}
		})
	}
}

func TestValidateCreateRequest_AllowedCapability(t *testing.T) {
	req := &CreateContainerRequest{
		Name:  "my-app",
		Image: "alpine:latest",
		Policy: &PrivilegedPolicy{
			AddCapabilities: []string{"NET_BIND_SERVICE", "CHOWN"},
		},
	}
	if err := validateCreateRequest(req); err != nil {
		t.Fatalf("expected permitted capabilities to pass validation, got: %v", err)
	}
}

func TestTraefikLabelGeneration(t *testing.T) {
	cfg := &TraefikConfig{
		Enabled:     true,
		Host:        "app.deploycore.dev",
		Port:        8080,
		TLS:         true,
		PathPrefix:  "/api",
		Middlewares: []string{"compress", "auth-mw"},
		ServiceName: "app-service",
	}

	labels := GenerateTraefikLabels(cfg)

	expected := map[string]string{
		"traefik.enable":                                                               "true",
		"traefik.http.routers.app-service.rule":                                        "Host(`app.deploycore.dev`) && PathPrefix(`/api`)",
		"traefik.http.routers.app-service.entrypoints":                                 "websecure",
		"traefik.http.routers.app-service-http.rule":                                   "Host(`app.deploycore.dev`) && PathPrefix(`/api`)",
		"traefik.http.routers.app-service-http.entrypoints":                            "web",
		"traefik.http.routers.app-service-http.middlewares":                            "app-service-https-redirect",
		"traefik.http.middlewares.app-service-https-redirect.redirectscheme.scheme":    "https",
		"traefik.http.middlewares.app-service-https-redirect.redirectscheme.permanent": "true",
		"traefik.http.routers.app-service.tls":                                         "true",
		"traefik.http.routers.app-service.tls.certresolver":                            "letsencrypt",
		"traefik.http.routers.app-service.middlewares":                                 "compress,auth-mw",
		"traefik.http.services.app-service.loadbalancer.server.port":                   "8080",
	}

	for k, v := range expected {
		got, ok := labels[k]
		if !ok {
			t.Errorf("missing expected label %q", k)
		} else if got != v {
			t.Errorf("label %q: expected %q, got %q", k, v, got)
		}
	}
}

func TestTraefikLabelGeneration_NilOrDisabled(t *testing.T) {
	if len(GenerateTraefikLabels(nil)) != 0 {
		t.Error("expected empty labels for nil TraefikConfig")
	}
	if len(GenerateTraefikLabels(&TraefikConfig{Enabled: false})) != 0 {
		t.Error("expected empty labels for disabled TraefikConfig")
	}
}

func TestValidateCreateRequest_VolumeNameFormat(t *testing.T) {
	tests := []struct {
		name      string
		volName   string
		expectErr string
	}{
		{"spaces in volume name", "my vol", "invalid volume name format"},
		{"shell injection in volume name", "vol; rm -rf /", "invalid volume name format"},
		{"leading dot in volume name", ".myvol", "invalid volume name format"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &CreateContainerRequest{
				Name:    "my-app",
				Image:   "alpine:latest",
				Volumes: []VolumeMount{{VolumeName: tc.volName, MountPath: "/data"}},
			}
			err := validateCreateRequest(req)
			if err == nil {
				t.Fatalf("expected error for volume name %q, got nil", tc.volName)
			}
			if !strings.Contains(err.Error(), tc.expectErr) {
				t.Errorf("expected error containing %q, got %v", tc.expectErr, err)
			}
		})
	}
}

func TestValidateCreateRequest_NetworkNameFormat(t *testing.T) {
	tests := []struct {
		name      string
		netName   string
		expectErr string
	}{
		{"spaces in network name", "my net", "invalid network name/ID format"},
		{"shell injection in network name", "net; rm -rf /", "invalid network name/ID format"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &CreateContainerRequest{
				Name:     "my-app",
				Image:    "alpine:latest",
				Networks: []string{tc.netName},
			}
			err := validateCreateRequest(req)
			if err == nil {
				t.Fatalf("expected error for network name %q, got nil", tc.netName)
			}
			if !strings.Contains(err.Error(), tc.expectErr) {
				t.Errorf("expected error containing %q, got %v", tc.expectErr, err)
			}
		})
	}
}

func TestValidateCreateRequest_PrivilegedPolicyHooks_DisabledByDefault(t *testing.T) {
	hooks := []struct {
		name      string
		policy    *PrivilegedPolicy
		expectErr string
	}{
		{"AllowPrivileged", &PrivilegedPolicy{AllowPrivileged: true}, "privileged mode requires explicit admin grant"},
		{"AllowHostPID", &PrivilegedPolicy{AllowHostPID: true}, "host PID namespace requires explicit admin grant"},
		{"AllowHostIPC", &PrivilegedPolicy{AllowHostIPC: true}, "host IPC namespace requires explicit admin grant"},
		{"AllowHostNetwork", &PrivilegedPolicy{AllowHostNetwork: true}, "host network requires explicit admin grant"},
		{"AllowDockerSocket", &PrivilegedPolicy{AllowDockerSocket: true}, "Docker socket mount requires explicit admin grant"},
		{"AllowArbitraryHostPaths", &PrivilegedPolicy{AllowArbitraryHostPaths: true}, "raw host path mounts require explicit admin grant"},
		{"AllowDevicePassthrough", &PrivilegedPolicy{AllowDevicePassthrough: true}, "device passthrough requires explicit admin grant"},
	}

	for _, h := range hooks {
		t.Run(h.name, func(t *testing.T) {
			req := &CreateContainerRequest{
				Name:   "my-app",
				Image:  "alpine:latest",
				Policy: h.policy,
			}
			err := validateCreateRequest(req)
			if err == nil {
				t.Fatalf("expected error for hook %s, got nil", h.name)
			}
			if !strings.Contains(err.Error(), h.expectErr) {
				t.Errorf("expected error containing %q, got %v", h.expectErr, err)
			}
		})
	}
}

func TestValidateCreateRequest_ReadOnlyRootFS(t *testing.T) {
	req := &CreateContainerRequest{
		Name:           "my-app",
		Image:          "alpine:latest",
		ReadOnlyRootFS: true,
	}
	if err := validateCreateRequest(req); err != nil {
		t.Fatalf("expected valid request with ReadOnlyRootFS, got: %v", err)
	}
}
