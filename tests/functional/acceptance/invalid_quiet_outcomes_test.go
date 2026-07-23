package acceptance

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	runcli "github.com/portpowered/infinite-you/pkg/transports/cli/run"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

func TestInvalidGoal_OutputModesExitNonZero(t *testing.T) {
	t.Parallel()

	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	session := harness.NewSession(t)

	for _, tc := range []struct {
		name string
		args []string
	}{
		{name: "default", args: []string{"run", "--named", "@you/missing", "--no-record", "invalid-goal-prompt"}},
		{name: "quiet", args: []string{"run", "--named", "@you/missing", "--no-record", "--quiet", "invalid-goal-prompt"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			result, err := session.Run(ctx, tc.args...)
			if err == nil || result.ExitCode == 0 {
				t.Fatalf("invalid goal result = %#v, error = %v; want non-zero process exit", result, err)
			}
		})
	}
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

	if result.Stdout != "" {
		t.Fatalf("stdout = %q, want empty on pre-terminal failure", result.Stdout)
	}
	response := decodeInvalidInvocationErrorResponse(t, result.Stderr)
	for _, want := range []string{
		"invalid graph references",
		"blocking validation targets",
	} {
		if !strings.Contains(response.Message, want) {
			t.Fatalf("ErrorResponse message = %q, want invalid-topology detail %q", response.Message, want)
		}
	}
}

func decodeInvalidInvocationErrorResponse(t *testing.T, stderr string) factoryapi.ErrorResponse {
	t.Helper()
	var response factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(stderr), &response); err != nil {
		t.Fatalf("stderr is not one ErrorResponse: %v\nstderr:\n%s", err, stderr)
	}
	if response.Code != factoryapi.ErrorResponseCode(runcli.InvocationErrorCodeFailed) ||
		response.Family != factoryapi.ErrorFamilyInternalServerError || response.Message == "" {
		t.Fatalf("ErrorResponse = %#v", response)
	}
	return response
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
