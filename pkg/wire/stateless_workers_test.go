package wire

import (
	"context"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"go.uber.org/zap"
)

func TestCanonicalStatelessWorkersExecuteBeforeRuntimeOpening(t *testing.T) {
	providersService, err := provideProvidersService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideProvidersService() error = %v", err)
	}
	modelsService, err := provideModelsService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideModelsService() error = %v", err)
	}
	service, err := provideStatelessWorkersService(
		providersService,
		modelsService,
		statelessCompositionCommandRunner{},
		platformfilesystem.Local{},
		platformclock.Real{},
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("provideStatelessWorkersService() error = %v", err)
	}

	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			DispatchID: "dispatch-canonical",
			AttemptID:  "attempt-canonical",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "script-worker",
			RunnerID:   "script",
			Command:    "canonical-script",
		},
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, want ACCEPTED", result.Outcome)
	}
	if len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "canonical-output" {
		t.Fatalf("output = %#v, want canonical-output", result.Output)
	}
}

type statelessCompositionCommandRunner struct{}

func (statelessCompositionCommandRunner) Run(
	context.Context,
	workers.CommandRequest,
) (workers.CommandResult, error) {
	return workers.CommandResult{Stdout: []byte("canonical-output")}, nil
}
