package release_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/releasesmoke"
	"github.com/portpowered/infinite-you/pkg/testutil"
)

func TestReleaseSmokeHarness_RunsBuiltBinaryAgainstCanonicalFixture(t *testing.T) {
	t.Parallel()

	binaryPath := buildReleaseSmokeBinary(t)
	fixturePath := testutil.MustRepoPath(t, "tests/release/testdata/cli_smoke_factory")
	var renderedDashboardURL string
	result, err := releasesmoke.Run(context.Background(), releasesmoke.Config{
		BinaryPath:  binaryPath,
		FixturePath: fixturePath,
		Timeout:     20 * time.Second,
		RenderedDashboardVerify: func(_ context.Context, dashboardURL string) (releasesmoke.DashboardRenderEvidence, error) {
			renderedDashboardURL = dashboardURL
			return releasesmoke.DashboardRenderEvidence{
				AssetRequestPaths: []string{"/dashboard/ui/assets/index.js"},
				LiveRequestPaths:  []string{"/events"},
				StreamMessage:     "Factory event stream connected.",
				VisibleTexts:      []string{"Agent Factory", "Work totals", "step-one", "step-two"},
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("run release smoke harness: %v", err)
	}

	if result.CompletedWorkCount < 1 {
		t.Fatalf("completed work count = %d, want at least 1", result.CompletedWorkCount)
	}
	if len(result.ObservedEventTypes) < 3 {
		t.Fatalf("observed event types = %#v, want run/init/work prelude", result.ObservedEventTypes)
	}
	if result.BaseURL == "" || result.DashboardURL == "" {
		t.Fatalf("result URLs = %#v, want non-empty base and dashboard URLs", result)
	}
	if renderedDashboardURL != result.DashboardURL {
		t.Fatalf("rendered dashboard URL = %q, want %q", renderedDashboardURL, result.DashboardURL)
	}
	if result.DashboardRenderEvidence.StreamMessage != "Factory event stream connected." {
		t.Fatalf("stream message = %q, want connected evidence", result.DashboardRenderEvidence.StreamMessage)
	}
	if len(result.DashboardRenderEvidence.AssetRequestPaths) == 0 || len(result.DashboardRenderEvidence.LiveRequestPaths) == 0 {
		t.Fatalf("dashboard render evidence = %#v, want asset and live request paths", result.DashboardRenderEvidence)
	}
}

func TestReleaseSmokeHarness_FailingRenderedDashboardVerificationReturnsStructuredFailure(t *testing.T) {
	t.Parallel()

	binaryPath := buildReleaseSmokeBinary(t)
	fixturePath := testutil.MustRepoPath(t, "tests/release/testdata/cli_smoke_factory")
	renderErr := errors.New("forced rendered dashboard failure")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err := releasesmoke.Run(ctx, releasesmoke.Config{
		BinaryPath:  binaryPath,
		FixturePath: fixturePath,
		Timeout:     20 * time.Second,
		RenderedDashboardVerify: func(context.Context, string) (releasesmoke.DashboardRenderEvidence, error) {
			return releasesmoke.DashboardRenderEvidence{}, renderErr
		},
	})
	if err == nil {
		t.Fatal("run release smoke harness: expected verify_dashboard_render failure")
	}

	var failure *releasesmoke.Failure
	if !errors.As(err, &failure) {
		t.Fatalf("run release smoke harness: error type = %T, want *releasesmoke.Failure", err)
	}
	if failure.Phase != "verify_dashboard_render" {
		t.Fatalf("failure phase = %q, want verify_dashboard_render", failure.Phase)
	}
	if !strings.Contains(failure.Message, renderErr.Error()) {
		t.Fatalf("failure message = %q, want substring %q", failure.Message, renderErr.Error())
	}
	if failure.BaseURL == "" || failure.DashboardURL == "" || failure.WorkspacePath == "" {
		t.Fatalf("failure = %#v, want populated urls and workspace", failure)
	}
	if len(failure.ObservedEventTypes) < 3 {
		t.Fatalf("observed event types = %#v, want run/init/work prelude", failure.ObservedEventTypes)
	}
}

func TestGoInstallSmoke_InstallsCmdFactoryBinaryIntoCleanGOBIN(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoRoot(t)
	tempRoot := t.TempDir()
	installDir := filepath.Join(tempRoot, "bin")
	goCacheDir := filepath.Join(tempRoot, "gocache")
	for _, dir := range []string{installDir, goCacheDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create temp dir %q: %v", dir, err)
		}
	}

	install := exec.Command("go", "install", "./cmd/factory")
	install.Dir = repoRoot
	install.Env = append(os.Environ(),
		"GOBIN="+installDir,
		"GOCACHE="+goCacheDir,
	)
	if output, err := install.CombinedOutput(); err != nil {
		t.Fatalf("go install ./cmd/factory: %v\n%s", err, string(output))
	}

	binaryPath := filepath.Join(installDir, goInstallBinaryName())
	if _, err := os.Stat(binaryPath); err != nil {
		t.Fatalf("installed binary missing at %q: %v", binaryPath, err)
	}

	smoke := exec.Command(binaryPath, "docs", "config")
	smoke.Dir = filepath.Join(tempRoot, "outside-repo")
	if err := os.MkdirAll(smoke.Dir, 0o755); err != nil {
		t.Fatalf("create smoke working dir %q: %v", smoke.Dir, err)
	}
	output, err := smoke.CombinedOutput()
	if err != nil {
		t.Fatalf("run installed binary docs smoke: %v\n%s", err, string(output))
	}

	rendered := string(output)
	for _, want := range []string{"# Config", "factory.json", "workTypes"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("installed docs output missing %q:\n%s", want, rendered)
		}
	}
}

func buildReleaseSmokeBinary(t *testing.T) string {
	t.Helper()

	binaryPath := filepath.Join(t.TempDir(), releaseSmokeBinaryName())
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/factory")
	build.Dir = testutil.MustRepoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build release smoke binary: %v\n%s", err, string(output))
	}
	return binaryPath
}

func releaseSmokeBinaryName() string {
	binaryName := "agent-factory"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	return binaryName
}

func goInstallBinaryName() string {
	binaryName := "factory"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	return binaryName
}
