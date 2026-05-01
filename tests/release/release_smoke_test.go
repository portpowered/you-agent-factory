package release_test

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/internal/releasesmoke"
	"github.com/portpowered/agent-factory/pkg/testutil"
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

func buildReleaseSmokeBinary(t *testing.T) string {
	t.Helper()

	binaryName := "agent-factory"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}

	binaryPath := filepath.Join(t.TempDir(), binaryName)
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/factory")
	build.Dir = testutil.MustRepoRoot(t)
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build release smoke binary: %v\n%s", err, string(output))
	}
	return binaryPath
}
