package docker

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
)

// TestCreateContainer_HostConfigBaseline proves nil and empty policies reach
// the Docker create body with CapDrop ALL, the curated CapAdd baseline, and
// Privileged left false. No live daemon is required.
func TestCreateContainer_HostConfigBaseline(t *testing.T) {
	cases := []struct {
		name   string
		policy *PrivilegedPolicy
	}{
		{name: "nil policy", policy: nil},
		{name: "empty policy", policy: &PrivilegedPolicy{}},
		{
			name: "explicit safe additions",
			policy: &PrivilegedPolicy{
				AddCapabilities: []string{"SETUID", "CAP_SETUID", "CHOWN"},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hostCfg := captureCreateHostConfig(t, CreateContainerRequest{
				Name:   "app",
				Image:  "redis:7-alpine",
				Policy: tc.policy,
			})
			if hostCfg.Privileged {
				t.Fatal("Privileged = true")
			}
			if len(hostCfg.SecurityOpt) != 0 {
				t.Fatalf("SecurityOpt = %v", hostCfg.SecurityOpt)
			}
			if hostCfg.PidMode != "" || hostCfg.IpcMode != "" {
				t.Fatalf("PidMode=%q IpcMode=%q", hostCfg.PidMode, hostCfg.IpcMode)
			}
			if strings.Join(hostCfg.CapDrop, ",") != "ALL" {
				t.Fatalf("CapDrop = %v", []string(hostCfg.CapDrop))
			}
			want := []string{"CHOWN", "DAC_OVERRIDE", "SETUID", "SETGID", "NET_BIND_SERVICE", "SETPCAP"}
			if strings.Join(hostCfg.CapAdd, ",") != strings.Join(want, ",") {
				t.Fatalf("CapAdd = %v, want %v", []string(hostCfg.CapAdd), want)
			}
			for _, forbidden := range []string{"SYS_ADMIN", "ALL", "NET_RAW", "MKNOD", "NET_ADMIN"} {
				if containsCap(hostCfg.CapAdd, forbidden) {
					t.Fatalf("CapAdd contains %s", forbidden)
				}
			}
		})
	}
}

func TestCreateContainer_RejectsForbiddenCapabilityBeforeDocker(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	cli, err := client.NewClientWithOpts(client.WithHost(srv.URL), client.WithVersion("1.43"))
	if err != nil {
		t.Fatal(err)
	}
	defer cli.Close()

	_, err = (&Client{cli: cli}).CreateContainer(context.Background(), CreateContainerRequest{
		Name:  "app",
		Image: "alpine:latest",
		Policy: &PrivilegedPolicy{
			AddCapabilities: []string{"SYS_ADMIN"},
		},
	})
	if err == nil {
		t.Fatal("expected SYS_ADMIN to be rejected")
	}
	if called {
		t.Fatal("Docker create was called for a forbidden capability")
	}
}

func captureCreateHostConfig(t *testing.T, req CreateContainerRequest) container.HostConfig {
	t.Helper()
	var hostCfg container.HostConfig
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.Contains(r.URL.Path, "/containers/create") {
			http.NotFound(w, r)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var parsed struct {
			HostConfig container.HostConfig `json:"HostConfig"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			t.Errorf("decode body: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		hostCfg = parsed.HostConfig
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"Id":"cnt-test","Warnings":null}`))
	}))
	t.Cleanup(srv.Close)

	cli, err := client.NewClientWithOpts(client.WithHost(srv.URL), client.WithVersion("1.43"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cli.Close() })

	if _, err := (&Client{cli: cli}).CreateContainer(context.Background(), req); err != nil {
		t.Fatalf("CreateContainer: %v", err)
	}
	return hostCfg
}
