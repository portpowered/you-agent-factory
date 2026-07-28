package codex_test

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	codex "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	mockworker "github.com/portpowered/infinite-you/pkg/services/workers/services/testing"
)

func TestCommandEffectRoutesDispatchContextThroughMockWorkerRunner(t *testing.T) {
	t.Parallel()

	platformRunner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("live provider should not run")},
	)
	effect := codex.NewCommandEffect(&mockworker.MockWorkerCommandRunner{
		Config: &workers.MockWorkersConfig{
			MockWorkers: []workers.MockWorkerConfig{{
				WorkerName:      "mocked-worker",
				WorkstationName: "mock-process",
				RunType:         workers.MockWorkerRunTypeAccept,
			}},
		},
		Next: workerprocess.AdaptCommandRunner(platformRunner),
	})
	if effect == nil {
		t.Fatal("NewCommandEffect() returned nil")
	}

	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:        providers.IDCodex,
		AttemptID:       "mock-dispatch",
		UserMessage:     "perform work",
		WorkerType:      "mocked-worker",
		WorkstationName: "mock-process",
	}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if platformRunner.CallCount() != 0 {
		t.Fatalf("platform runner calls = %d, want mock intercept", platformRunner.CallCount())
	}
}
