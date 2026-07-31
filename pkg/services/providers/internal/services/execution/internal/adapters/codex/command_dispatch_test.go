package codex_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	codex "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCommandEffectRoutesDispatchContextThroughMockWorkerRunner(t *testing.T) {
	t.Parallel()

	platformRunner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("live provider should not run")},
	)
	effect := codex.NewCommandEffect(&workers.MockWorkerCommandRunner{
		Config: &workers.MockWorkersConfig{
			MockWorkers: []workers.MockWorkerConfig{{
				WorkerName:      "mocked-worker",
				WorkstationName: "mock-process",
				RunType:         workers.MockWorkerRunTypeAccept,
			}},
		},
		Next: workers.AdaptCommandRunner(platformRunner),
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

func TestCommandEffectRejectsUnsupportedReasoningEffortBeforeDispatch(t *testing.T) {
	t.Parallel()

	platformRunner := testutil.NewProviderCommandRunner()
	effect := codex.NewCommandEffect(workers.AdaptCommandRunner(platformRunner))
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:        providers.IDCodex,
		AttemptID:       "invalid-effort-dispatch",
		ReasoningEffort: "extreme",
		UserMessage:     "perform work",
	}, func([]byte) error { return nil })
	var failure execution.AttemptFailure
	if !errors.As(err, &failure) ||
		failure.NativeError == nil ||
		!strings.Contains(failure.NativeError.Error(), `unsupported reasoning effort "extreme"`) {
		t.Fatalf("Execute() error = %v, want unsupported effort", err)
	}
	if got := platformRunner.Requests(); len(got) != 0 {
		t.Fatalf("runner requests = %#v, want none", got)
	}
}

func TestCommandEffectRendersLunaXHighReasoningEffort(t *testing.T) {
	t.Parallel()

	platformRunner := testutil.NewProviderCommandRunner()
	effect := codex.NewCommandEffect(workers.AdaptCommandRunner(platformRunner))
	if effect == nil {
		t.Fatal("NewCommandEffect() returned nil")
	}

	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:        providers.IDCodex,
		AttemptID:       "luna-xhigh-dispatch",
		Model:           "gpt-5.6-luna",
		ReasoningEffort: "xhigh",
		UserMessage:     "perform work",
	}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	request := platformRunner.LastRequest()
	want := []string{
		"exec",
		"--json",
		"--model", "gpt-5.6-luna",
		"--config", `model_reasoning_effort="xhigh"`,
		"-",
	}
	if !reflect.DeepEqual(request.Args, want) {
		t.Fatalf("command args = %#v, want %#v", request.Args, want)
	}
}
