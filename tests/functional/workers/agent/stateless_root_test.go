package agent_test

import (
	"context"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot proves the
// standalone Workers root composes and executes without opening Factory Runtime
// or a Factory Session. The direct Execute boundary has no Process.Execute
// transport representation, so this is intentionally a public root test.
func TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot(t *testing.T) {
	t.Parallel()

	service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ScriptCommandRunner: functionalStatelessCommandRunner{},
	})
	if err != nil {
		t.Fatalf("root.BuildStatelessWorkers() error = %v", err)
	}

	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-functional",
			RuntimeID:        "runtime-functional",
			GenerationID:     "generation-functional",
			DispatchID:       "functional-stateless-dispatch",
			AttemptID:        "functional-stateless-attempt",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "script-worker",
			RunnerID:   "script",
			Command:    "functional-stateless-script",
		},
	})
	if err != nil {
		t.Fatalf("stateless Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted ||
		len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "functional-stateless-output" {
		t.Fatalf("stateless result = %#v, want accepted functional output", result)
	}

	if service, err := root.BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ProviderRegistrations: []providerswire.Registration{{
			Manifest:    providerswire.Manifest{ID: "codex"},
			Integration: providerswire.ProgressingExternalIntegration("codex", "unused"),
		}},
	}); service != nil || err == nil || !strings.Contains(err.Error(), "provider registry validation failed") {
		t.Fatalf("invalid stateless provider registration = (%#v, %v), want provider registry validation failure", service, err)
	}
}

type functionalStatelessCommandRunner struct{}

func (functionalStatelessCommandRunner) Run(
	context.Context,
	platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte("functional-stateless-output")}, nil
}
