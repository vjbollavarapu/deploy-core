package applications

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

const (
	TypeWebService   = "WEB_SERVICE"
	TypeAPI          = "API"
	TypeWorker       = "WORKER"
	TypeScheduledJob = "SCHEDULED_JOB"
	TypeStaticSite   = "STATIC_SITE"
	TypeCompose      = "DOCKER_COMPOSE"
	TypeDockerImage  = "DOCKER_IMAGE"
)

const (
	SourceGit     = "git"
	SourceImage   = "image"
	SourceCompose = "compose"
	SourceUpload  = "upload"
)

type Application struct {
	ID              uuid.UUID
	OrganizationID  uuid.UUID
	ProjectID       uuid.UUID
	EnvironmentID   uuid.UUID
	Name            string
	Slug            string
	Type            string
	Status          string
	TargetServerID  *uuid.UUID
	PlacementPolicy map[string]any
	CreatedBy       *uuid.UUID
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Config          Config
}

type Config struct {
	ID                uuid.UUID
	Version           int
	SourceType        string
	RepositoryURL     *string
	GitBranch         *string
	DockerfilePath    *string
	BuildContext      *string
	ImageReference    *string
	InternalPort      *int
	Command           *string
	Entrypoint        *string
	CPULimitMillis    *int
	MemoryLimitBytes  *int64
	RestartPolicy     string
	HealthCheck       map[string]any
	RuntimeConfig     map[string]any
	AutoDeployEnabled bool
	GitConnectionID   *uuid.UUID
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

type CreateInput struct {
	OrganizationID  uuid.UUID
	ProjectID       uuid.UUID
	EnvironmentID   uuid.UUID
	Name            string
	Slug            string
	Type            string
	TargetServerID  *uuid.UUID
	PlacementPolicy map[string]any
	Config          ConfigInput
}

type ConfigInput struct {
	SourceType         string
	RepositoryURL      *string
	GitBranch          *string
	DockerfilePath     *string
	BuildContext       *string
	ImageReference     *string
	InternalPort       *int
	Command            *string
	Entrypoint         *string
	CPULimitMillis     *int
	MemoryLimitBytes   *int64
	RestartPolicy      string
	HealthCheck        map[string]any
	RuntimeConfig      map[string]any
	AutoDeployEnabled  *bool
	GitConnectionID    *uuid.UUID
	ClearGitConnection bool
}

type UpdateInput struct {
	Name            *string
	Slug            *string
	TargetServerID  *uuid.UUID
	ClearTarget     bool
	PlacementPolicy map[string]any
	Config          *ConfigInput
}

func mustJSON(v any) []byte {
	if v == nil {
		v = map[string]any{}
	}
	b, _ := json.Marshal(v)
	return b
}

func mapOrEmpty(m map[string]any) map[string]any {
	if m == nil {
		return map[string]any{}
	}
	return m
}
