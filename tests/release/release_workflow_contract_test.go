package release_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/agent-factory/pkg/testutil"
	"gopkg.in/yaml.v3"
)

type workflowConfig struct {
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Name     string           `yaml:"name"`
	RunsOn   string           `yaml:"runs-on"`
	Strategy workflowStrategy `yaml:"strategy"`
	Steps    []workflowStep   `yaml:"steps"`
}

type workflowStrategy struct {
	Matrix workflowMatrix `yaml:"matrix"`
}

type workflowMatrix struct {
	Include []workflowInclude `yaml:"include"`
}

type workflowInclude struct {
	OSID   string `yaml:"os_id"`
	Runner string `yaml:"runner"`
}

type workflowStep struct {
	Name string            `yaml:"name"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	Env  map[string]string `yaml:"env"`
	With map[string]string `yaml:"with"`
}

func TestReleaseWorkflowContract_PublishesHostedInstallerAndHomebrewAssets(t *testing.T) {
	t.Parallel()

	workflow := loadReleaseWorkflow(t)
	publishJob, ok := workflow.Jobs["publish-release"]
	if !ok {
		t.Fatal("release workflow missing publish-release job")
	}

	publishStep := findWorkflowStep(t, publishJob.Steps, "Publish GitHub release assets from the tagged commit")
	if publishStep.Env["GITHUB_TOKEN"] != "${{ secrets.GITHUB_TOKEN }}" {
		t.Fatalf("publish step GITHUB_TOKEN = %q, want secrets.GITHUB_TOKEN", publishStep.Env["GITHUB_TOKEN"])
	}
	if publishStep.Env["HOMEBREW_TAP_GITHUB_TOKEN"] != "${{ secrets.HOMEBREW_TAP_GITHUB_TOKEN }}" {
		t.Fatalf("publish step HOMEBREW_TAP_GITHUB_TOKEN = %q, want secrets.HOMEBREW_TAP_GITHUB_TOKEN", publishStep.Env["HOMEBREW_TAP_GITHUB_TOKEN"])
	}

	uploadStep := findWorkflowStep(t, publishJob.Steps, "Upload hosted install.sh release asset")
	if !strings.Contains(uploadStep.Run, "gh release upload") || !strings.Contains(uploadStep.Run, "install.sh") {
		t.Fatalf("upload hosted installer step run = %q, want gh release upload for install.sh", uploadStep.Run)
	}
}

func TestReleaseWorkflowContract_VerifiesHomebrewInstallerAndGoInstallSurfacesSeparately(t *testing.T) {
	t.Parallel()

	workflow := loadReleaseWorkflow(t)

	homebrewJob, ok := workflow.Jobs["verify-homebrew-cask"]
	if !ok {
		t.Fatal("release workflow missing verify-homebrew-cask job")
	}
	if homebrewJob.RunsOn != "macos-latest" {
		t.Fatalf("verify-homebrew-cask runs-on = %q, want macos-latest", homebrewJob.RunsOn)
	}
	tapCheckout := findWorkflowStep(t, homebrewJob.Steps, "Check out Homebrew tap")
	if tapCheckout.With["repository"] != "portpowered/cask" {
		t.Fatalf("tap checkout repository = %q, want portpowered/cask", tapCheckout.With["repository"])
	}
	installCask := findWorkflowStep(t, homebrewJob.Steps, "Install published Homebrew cask from tap")
	if !strings.Contains(installCask.Run, "brew install --cask") || !strings.Contains(installCask.Run, "tap/Casks/agent-factory.rb") {
		t.Fatalf("homebrew install step run = %q, want brew install of published cask file", installCask.Run)
	}
	verifyHomebrew := findWorkflowStep(t, homebrewJob.Steps, "Verify installed Homebrew binary starts successfully")
	if !strings.Contains(verifyHomebrew.Run, "agent-factory --help") {
		t.Fatalf("homebrew verification step run = %q, want agent-factory --help", verifyHomebrew.Run)
	}

	installerJob, ok := workflow.Jobs["smoke-hosted-installer"]
	if !ok {
		t.Fatal("release workflow missing smoke-hosted-installer job")
	}
	if len(installerJob.Strategy.Matrix.Include) != 2 {
		t.Fatalf("smoke-hosted-installer matrix include count = %d, want 2", len(installerJob.Strategy.Matrix.Include))
	}
	if !hasInstallerRunner(installerJob.Strategy.Matrix.Include, "linux", "ubuntu-latest") {
		t.Fatalf("smoke-hosted-installer matrix = %#v, want linux ubuntu runner", installerJob.Strategy.Matrix.Include)
	}
	if !hasInstallerRunner(installerJob.Strategy.Matrix.Include, "macos", "macos-latest") {
		t.Fatalf("smoke-hosted-installer matrix = %#v, want macos runner", installerJob.Strategy.Matrix.Include)
	}
	smokeInstaller := findWorkflowStep(t, installerJob.Steps, "Smoke hosted install.sh against the published release")
	if !strings.Contains(smokeInstaller.Run, "scripts/release/smoke-install.sh") {
		t.Fatalf("installer smoke step run = %q, want smoke-install helper", smokeInstaller.Run)
	}
	if !strings.Contains(smokeInstaller.Run, "/releases/download/${{ needs.resolve-release-tag.outputs.release_tag }}/install.sh") {
		t.Fatalf("installer smoke step run = %q, want hosted release install.sh URL", smokeInstaller.Run)
	}

	goInstallJob, ok := workflow.Jobs["verify-go-install"]
	if !ok {
		t.Fatal("release workflow missing verify-go-install job")
	}
	smokeGoInstall := findWorkflowStep(t, goInstallJob.Steps, "Smoke go install verification contract")
	if !strings.Contains(smokeGoInstall.Run, "go test ./tests/release -run TestGoInstallSmoke_") {
		t.Fatalf("go install verification run = %q, want focused go install release test", smokeGoInstall.Run)
	}
}

func loadReleaseWorkflow(t *testing.T) workflowConfig {
	t.Helper()

	workflowPath := filepath.Join(testutil.MustRepoRoot(t), ".github", "workflows", "release.yml")
	contents, err := os.ReadFile(workflowPath)
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}

	var workflow workflowConfig
	if err := yaml.Unmarshal(contents, &workflow); err != nil {
		t.Fatalf("unmarshal release workflow: %v", err)
	}
	return workflow
}

func findWorkflowStep(t *testing.T, steps []workflowStep, name string) workflowStep {
	t.Helper()

	for _, step := range steps {
		if step.Name == name {
			return step
		}
	}
	t.Fatalf("workflow step %q not found", name)
	return workflowStep{}
}

func hasInstallerRunner(include []workflowInclude, osID string, runner string) bool {
	for _, entry := range include {
		if entry.OSID == osID && entry.Runner == runner {
			return true
		}
	}
	return false
}
