package appcontainer

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/deploycore/deploy-core/packages/protocol-go"
)

var (
	// ErrMissingLabel indicates a required platform label is missing.
	ErrMissingLabel = errors.New("missing required platform label")
	// ErrNotManaged indicates the container does not have deploycore.managed=true.
	ErrNotManaged = errors.New("container is not managed by deploycore")
)

// Metadata represents the verified platform metadata associated with an application container.
type Metadata struct {
	OrganizationID string
	ApplicationID  string
	EnvironmentID  string
	DeploymentID   string
	RevisionID     string
	Instance       int
	AppShortID     string // application slug
	IsCandidate    bool   // true during candidate evaluation phase before promotion
}

// Validate checks that all required platform identity fields are populated.
func (m Metadata) Validate() error {
	var missing []string
	if strings.TrimSpace(m.OrganizationID) == "" {
		missing = append(missing, protocol.LabelOrganizationID)
	}
	if strings.TrimSpace(m.ApplicationID) == "" {
		missing = append(missing, protocol.LabelApplicationID)
	}
	if strings.TrimSpace(m.EnvironmentID) == "" {
		missing = append(missing, protocol.LabelEnvironmentID)
	}
	if strings.TrimSpace(m.DeploymentID) == "" {
		missing = append(missing, protocol.LabelDeploymentID)
	}
	if strings.TrimSpace(m.RevisionID) == "" {
		missing = append(missing, protocol.LabelRevisionID)
	}
	if m.Instance < 1 {
		missing = append(missing, protocol.LabelInstance+" (must be >= 1)")
	}

	if len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrMissingLabel, strings.Join(missing, ", "))
	}
	return nil
}

// Labels returns the canonical trusted labels map for this metadata.
func (m Metadata) Labels() map[string]string {
	labels := map[string]string{
		protocol.LabelManaged:        "true",
		protocol.LabelOrganizationID: strings.TrimSpace(m.OrganizationID),
		protocol.LabelApplicationID:  strings.TrimSpace(m.ApplicationID),
		protocol.LabelEnvironmentID:  strings.TrimSpace(m.EnvironmentID),
		protocol.LabelDeploymentID:   strings.TrimSpace(m.DeploymentID),
		protocol.LabelRevisionID:     strings.TrimSpace(m.RevisionID),
	}

	if m.Instance > 0 {
		labels[protocol.LabelInstance] = strconv.Itoa(m.Instance)
	}
	if strings.TrimSpace(m.AppShortID) != "" {
		labels[protocol.LabelApplicationSlug] = strings.TrimSpace(m.AppShortID)
	}
	if m.IsCandidate {
		labels[protocol.LabelCandidate] = "true"
	}

	return labels
}

// ExtractMetadata parses and verifies the trusted labels from a container's labels map.
func ExtractMetadata(labels map[string]string) (Metadata, error) {
	if labels == nil || labels[protocol.LabelManaged] != "true" {
		return Metadata{}, ErrNotManaged
	}

	instance := 0
	if instStr, ok := labels[protocol.LabelInstance]; ok && instStr != "" {
		if val, err := strconv.Atoi(instStr); err == nil && val >= 1 {
			instance = val
		}
	}

	m := Metadata{
		OrganizationID: labels[protocol.LabelOrganizationID],
		ApplicationID:  labels[protocol.LabelApplicationID],
		EnvironmentID:  labels[protocol.LabelEnvironmentID],
		DeploymentID:   labels[protocol.LabelDeploymentID],
		RevisionID:     labels[protocol.LabelRevisionID],
		Instance:       instance,
		AppShortID:     labels[protocol.LabelApplicationSlug],
		IsCandidate:    labels[protocol.LabelCandidate] == "true",
	}

	if err := m.Validate(); err != nil {
		return Metadata{}, err
	}

	return m, nil
}
