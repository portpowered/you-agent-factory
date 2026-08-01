package codex_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	effects "github.com/portpowered/infinite-you/pkg/services/providers/internal/effects"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	codex "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
)

func TestCommandEffectCarriesAttemptCorrelationToProvidersEffectRunner(t *testing.T) {
	t.Parallel()

	runner := &recordingCommandRunner{}
	effect := codex.NewCommandEffect(runner)
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
	if runner.request.AttemptID != "mock-dispatch" ||
		runner.request.WorkerType != "mocked-worker" ||
		runner.request.WorkstationName != "mock-process" {
		t.Fatalf("Providers effect request = %#v, want Providers-owned correlation", runner.request)
	}
}

func TestCommandEffectPropagatesStreamingObserverFailure(t *testing.T) {
	t.Parallel()

	observerErr := errors.New("output observer failed")
	effect := codex.NewCommandEffect(streamingObserverErrorRunner{})
	_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
		Provider:    providers.IDCodex,
		AttemptID:   "observer-failure",
		UserMessage: "perform work",
	}, func([]byte) error { return observerErr })
	if !errors.Is(err, observerErr) {
		t.Fatalf("Execute() error = %v, want observer failure", err)
	}
}

func TestCommandEffectIsSafeForConcurrentAttempts(t *testing.T) {
	t.Parallel()

	runner := &concurrentCommandRunner{}
	effect := codex.NewCommandEffect(runner)
	if effect == nil {
		t.Fatal("NewCommandEffect() returned nil")
	}

	const attempts = 32
	var waitGroup sync.WaitGroup
	waitGroup.Add(attempts)
	errors := make(chan error, attempts)
	for range attempts {
		go func() {
			defer waitGroup.Done()
			_, err := effect.Execute(context.Background(), providers.ExecuteRequest{
				Provider:        providers.IDCodex,
				AttemptID:       "concurrent-attempt",
				UserMessage:     "perform work",
				WorkerType:      "mocked-worker",
				WorkstationName: "mock-process",
			}, func([]byte) error { return nil })
			if err != nil {
				errors <- err
			}
		}()
	}
	waitGroup.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("Execute() error = %v", err)
	}
	if got := runner.RequestCount(); got != attempts {
		t.Fatalf("runner request count = %d, want %d", got, attempts)
	}
}

func TestCommandEffectRejectsUnsupportedReasoningEffortBeforeDispatch(t *testing.T) {
	t.Parallel()

	platformRunner := testutil.NewProviderCommandRunner()
	effect := codex.NewCommandEffect(executionwire.AdaptPlatformCommandRunner(platformRunner))
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
	effect := codex.NewCommandEffect(executionwire.AdaptPlatformCommandRunner(platformRunner))
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

type recordingCommandRunner struct {
	request effects.CommandRequest
}

type streamingObserverErrorRunner struct{}

func (streamingObserverErrorRunner) Run(
	context.Context,
	effects.CommandRequest,
) (effects.CommandResult, error) {
	return effects.CommandResult{}, nil
}

func (runner streamingObserverErrorRunner) RunStreaming(
	_ context.Context,
	_ effects.CommandRequest,
	observer effects.OutputChunkObserver,
) (effects.CommandResult, error) {
	return effects.CommandResult{}, observer(effects.OutputStreamStdout, []byte("output"))
}

func (runner *recordingCommandRunner) Run(
	_ context.Context,
	request effects.CommandRequest,
) (effects.CommandResult, error) {
	runner.request = request
	return effects.CommandResult{}, nil
}

type concurrentCommandRunner struct {
	mu       sync.Mutex
	requests int
}

func (runner *concurrentCommandRunner) Run(
	_ context.Context,
	_ effects.CommandRequest,
) (effects.CommandResult, error) {
	runner.mu.Lock()
	runner.requests++
	runner.mu.Unlock()
	return effects.CommandResult{}, nil
}

func (runner *concurrentCommandRunner) RequestCount() int {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.requests
}
