package buildlogs

import (
	"regexp"
	"strings"
)

var (
	// dockerStepRE matches classic Docker engine build steps:
	// "Step 1/5 : FROM alpine:3.18"
	dockerStepRE = regexp.MustCompile(`^Step\s+([0-9]+/[0-9]+)\s*:\s*(.*)$`)

	// buildkitStageRE matches BuildKit multiplexed stage lines:
	// "#4 [internal] load build definition from Dockerfile"
	// "#7 [stage-0 2/4] RUN go build -o /bin/app"
	buildkitStageRE = regexp.MustCompile(`^#([0-9]+)\s+(\[[^\]]+\])\s*(.*)$`)

	// buildkitPhaseRE matches BuildKit global phases:
	// "#8 exporting to image"
	// "#9 naming to docker.io/library/myapp"
	buildkitPhaseRE = regexp.MustCompile(`^#([0-9]+)\s+([a-zA-Z0-9_\-\s]+)$`)
)

// StageDetector detects and tracks current build stages from log output.
type StageDetector struct {
	currentStage string
}

// NewStageDetector creates a new StageDetector.
func NewStageDetector() *StageDetector {
	return &StageDetector{}
}

// Current returns the most recently detected stage.
func (d *StageDetector) Current() string {
	return d.currentStage
}

// Detect checks if the line defines or advances a build stage.
// It returns the effective stage for the line and whether this line introduced a new stage.
func (d *StageDetector) Detect(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return d.currentStage, false
	}

	// 1. Check classic Docker Step
	if matches := dockerStepRE.FindStringSubmatch(trimmed); len(matches) == 3 {
		stepNum := matches[1]
		cmd := strings.TrimSpace(matches[2])
		d.currentStage = "Step " + stepNum + ": " + cmd
		return d.currentStage, true
	}

	// 2. Check BuildKit stage: #4 [internal] ...
	if matches := buildkitStageRE.FindStringSubmatch(trimmed); len(matches) == 4 {
		stageID := matches[1]
		stageName := matches[2]
		desc := strings.TrimSpace(matches[3])
		if desc != "" {
			d.currentStage = "#" + stageID + " " + stageName + " " + desc
		} else {
			d.currentStage = "#" + stageID + " " + stageName
		}
		return d.currentStage, true
	}

	// 3. Check BuildKit phase: #8 exporting to image
	if matches := buildkitPhaseRE.FindStringSubmatch(trimmed); len(matches) == 3 {
		stageID := matches[1]
		phase := strings.TrimSpace(matches[2])
		d.currentStage = "#" + stageID + " " + phase
		return d.currentStage, true
	}

	// 4. Check completion markers
	if strings.HasPrefix(trimmed, "Successfully built ") {
		d.currentStage = "Built"
		return d.currentStage, true
	}
	if strings.HasPrefix(trimmed, "Successfully tagged ") {
		d.currentStage = "Tagged"
		return d.currentStage, true
	}

	return d.currentStage, false
}
