package acceptance

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/config/defaultpaths"
	"github.com/portpowered/infinite-you/pkg/factory/packages/goal"
)

var quietLeakForbiddenMarkers = []string{
	"Factory initiated",
	"Dashboard URL",
	"Runtime log",
	"Opening dashboard",
	"Factory:",
	"Recording saved",
}

func TestInvalidGoal_UnknownNamedFactory_RejectsWithDocumentedError(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := session.Run(ctx,
		"run",
		"--named", "@you/missing",
		"--no-record",
		"invalid-goal-acceptance-prompt",
	)
	if err == nil {
		t.Fatalf("expected unknown named factory failure, got result=%#v", result)
	}
	if result.ExitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero for invalid goal")
	}

	combined := result.Stdout + result.Stderr
	for _, want := range []string{
		`resolve named factory "@you/missing"`,
		"not found",
		"project root",
		"global root",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("output = %q, want documented invalid-goal guidance %q", combined, want)
		}
	}
}

func TestInvalidGoal_QuietMode_SuppressesTerminalOnOperationalFailure(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := session.Run(ctx,
		"run",
		"--named", "@you/missing",
		"--no-record",
		"--quiet",
		"invalid-goal-quiet-acceptance-prompt",
	)
	if err == nil {
		t.Fatalf("expected unknown named factory failure, got result=%#v", result)
	}
	if result.ExitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero for invalid goal in quiet mode")
	}
	if result.Stdout != "" {
		t.Fatalf("stdout = %q, want empty quiet operational-failure terminal output", result.Stdout)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty quiet operational-failure terminal output", result.Stderr)
	}
	assertQuietLeakContractForbidden(t, result.Stdout+result.Stderr)
}

func TestInvalidGoal_InvalidTopology_RejectsWithDocumentedGraphReferenceError(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	factoryPath := writeInvalidGoalTopologyFactory(t, session.WorkDir)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := session.Run(ctx,
		"run",
		"--factory", factoryPath,
		"--no-record",
		"invalid-topology-acceptance-prompt",
	)
	if err == nil {
		t.Fatalf("expected invalid topology failure, got result=%#v", result)
	}
	if result.ExitCode == 0 {
		t.Fatalf("exit code = 0, want non-zero for invalid goal topology")
	}

	combined := result.Stdout + result.Stderr
	for _, want := range []string{
		"invalid graph references",
		"Blocking findings:",
		"you factory config validate",
	} {
		if !strings.Contains(combined, want) {
			t.Fatalf("output = %q, want documented invalid-topology guidance %q", combined, want)
		}
	}
}

func TestQuietMode_SuccessfulNamedGoal_SuppressesOperatorChatterAndPreservesPrimaryResult(t *testing.T) {
	if testing.Short() {
		t.Skip("slow built-CLI quiet mode named goal acceptance")
	}

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t).WithNoExternalServer(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	initResult, err := session.Run(ctx, "config", "init")
	session.RequireSuccess(t, "quiet-mode-config-init", initResult, err)

	configPath := defaultpaths.OperatorConfigPath(session.HomeDir)
	configBody := []byte(`{
  "defaults": {
    "workerModelProvider": "codex",
    "workerModel": "gpt-5-codex"
  }
}`)
	if writeErr := os.WriteFile(configPath, configBody, 0o600); writeErr != nil {
		t.Fatalf("WriteFile(%q): %v", configPath, writeErr)
	}

	mockWorkersPath := writePackagedGoalMockWorkersConfig(t)
	goalText := fmt.Sprintf("acceptance-quiet-mode-%d", time.Now().UnixNano())

	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--named", goal.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--quiet",
		mockWorkersPath,
		goalText,
	)

	result, err := session.Run(ctx, args...)
	session.RequireSuccess(t, "quiet-mode-named-goal", result, err)

	if got := result.Stdout; got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("stdout = %q, want authoritative primary result %q", got, packagedGoalMockWorkerAcceptedSummary)
	}
	if strings.Contains(result.Stdout, goalText) {
		t.Fatalf("stdout echoed submitted goal text %q", goalText)
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty stderr on successful quiet run", result.Stderr)
	}
	assertQuietLeakContractForbidden(t, result.Stdout+result.Stderr)
}

func assertQuietLeakContractForbidden(t *testing.T, output string) {
	t.Helper()

	for _, forbidden := range quietLeakForbiddenMarkers {
		if strings.Contains(output, forbidden) {
			t.Fatalf("output = %q, want no quiet-leak marker %q", output, forbidden)
		}
	}
}

func writeInvalidGoalTopologyFactory(t *testing.T, workDir string) string {
	t.Helper()

	factoryPath := filepath.Join(workDir, "factory.json")
	if err := os.WriteFile(factoryPath, []byte(invalidGoalTopologyFactoryJSON), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", factoryPath, err)
	}
	return factoryPath
}

const invalidGoalTopologyFactoryJSON = `{
  "name": "@you/goal",
  "workTypes": [{
    "name": "goal",
    "handlingBehavior": ["DEFAULT"],
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "plan", "type": "PROCESSING"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{"name": "goal-planner", "type": "AGENT_WORKER"}],
  "workstations": [{
    "name": "plan-goal",
    "type": "AGENT_RUN",
    "worker": "goal-planner",
    "inputs": [{"workType": "goal", "state": "init"}],
    "outputs": [{"workType": "goal", "state": "missing-plan-state"}],
    "onFailure": [{"workType": "goal", "state": "failed"}]
  }]
}`
