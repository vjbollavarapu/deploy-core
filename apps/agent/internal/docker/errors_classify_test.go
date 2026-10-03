package docker

import (
	"errors"
	"strings"
	"testing"

	dockererr "github.com/docker/docker/errdefs"
)

func TestMapDockerErrorDockerfileIsNotDaemonUnavailable(t *testing.T) {
	mapped := mapDockerError(dockererr.System(errors.New("Cannot locate specified Dockerfile: Dockerfile")))
	agentErr, ok := mapped.(*AgentError)
	if !ok {
		t.Fatalf("got %T", mapped)
	}
	if agentErr.Code != ErrCodeDockerfileNotFound {
		t.Fatalf("code=%s", agentErr.Code)
	}
	if strings.Contains(agentErr.Error(), "DOCKER_DAEMON_UNAVAILABLE") {
		t.Fatalf("dockerfile error was labeled as the daemon: %s", agentErr.Error())
	}
	if !strings.Contains(agentErr.Message, "Cannot locate specified Dockerfile: Dockerfile") {
		t.Fatalf("message=%s", agentErr.Message)
	}

	readErr := mapDockerError(dockererr.System(errors.New("failed to read dockerfile: open Dockerfile: no such file")))
	if got := readErr.(*AgentError).Code; got != ErrCodeDockerfileNotFound {
		t.Fatalf("read dockerfile code=%s", got)
	}
}

func TestMapDockerErrorDaemonUnavailable(t *testing.T) {
	mapped := mapDockerError(dockererr.Unavailable(errors.New("Cannot connect to the Docker daemon at unix:///var/run/docker.sock: connection refused")))
	agentErr := mapped.(*AgentError)
	if agentErr.Code != ErrCodeDaemonUnavailable {
		t.Fatalf("code=%s", agentErr.Code)
	}
	if !strings.Contains(agentErr.Message, "connection refused") {
		t.Fatalf("message=%s", agentErr.Message)
	}

	other := mapDockerError(dockererr.System(errors.New("unexpected build failure")))
	if other.(*AgentError).Code == ErrCodeDaemonUnavailable {
		t.Fatal("unrelated system error was classified as the daemon")
	}
}
