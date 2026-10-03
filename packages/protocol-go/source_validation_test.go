package protocol

import "testing"

func TestBuildImagePayloadGitFetchAndBuild(t *testing.T) {
	fetch := BuildImagePayload{
		Phase:           BuildPhaseFetchSource,
		DeploymentID:    "dep-1",
		ApplicationID:   "app-1",
		RevisionID:      "rev-1",
		RepositoryURL:   "https://github.com/vjbollavarapu/modulyn",
		GitBranch:       "main",
		GitConnectionID: "11111111-1111-4111-8111-111111111111",
	}
	if err := fetch.Validate(); err != nil {
		t.Fatal(err)
	}
	build := BuildImagePayload{
		Phase:          BuildPhaseBuild,
		DeploymentID:   "dep-1",
		RevisionID:     "rev-1",
		DockerfilePath: "Dockerfile.prod",
		ContextPath:    "apps/backend",
	}
	if err := build.Validate(); err != nil {
		t.Fatal(err)
	}
	defaults := BuildImagePayload{
		Phase:          BuildPhaseBuild,
		DeploymentID:   "dep-1",
		DockerfilePath: "Dockerfile",
		ContextPath:    ".",
	}
	if err := defaults.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildImagePayloadRejectsUnsafeSource(t *testing.T) {
	cases := []BuildImagePayload{
		{Phase: BuildPhaseFetchSource, DeploymentID: "dep-1", RepositoryURL: "file:///tmp/repo", GitBranch: "main"},
		{Phase: BuildPhaseFetchSource, DeploymentID: "dep-1", RepositoryURL: "ssh://git@github.com/acme/app.git", GitBranch: "main"},
		{Phase: BuildPhaseFetchSource, DeploymentID: "dep-1", RepositoryURL: "https://user:token@github.com/acme/app", GitBranch: "main"},
		{Phase: BuildPhaseFetchSource, DeploymentID: "dep-1", RepositoryURL: "https://github.com/acme/app", GitBranch: "main\nrm -rf"},
		{Phase: BuildPhaseBuild, DeploymentID: "dep-1", DockerfilePath: "/etc/passwd", ContextPath: "."},
		{Phase: BuildPhaseBuild, DeploymentID: "dep-1", DockerfilePath: "../../Dockerfile", ContextPath: "."},
		{Phase: BuildPhaseBuild, DeploymentID: "dep-1", DockerfilePath: "Dockerfile", ContextPath: "/etc"},
		{Phase: BuildPhaseBuild, DeploymentID: "dep-1", DockerfilePath: "Dockerfile", ContextPath: "../../../../etc"},
		{Phase: BuildPhaseBuild, DeploymentID: "dep-1", DockerfilePath: "Docker\nfile", ContextPath: "."},
		{Phase: BuildPhaseFetchSource, DeploymentID: "dep-1", RepositoryURL: "https://github.com/acme/app", GitBranch: "main", GitConnectionID: "ghp_not_a_connection"},
		{Phase: BuildPhaseBuild, DeploymentID: "dep-1", DockerfilePath: "Dockerfile", ContextPath: ".", GitConnectionID: "11111111-1111-4111-8111-111111111111"},
	}
	for _, payload := range cases {
		if err := payload.Validate(); err == nil {
			t.Fatalf("expected rejection for %+v", payload)
		}
	}
}
