package codex_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	codex "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
)

func TestCommandEffectPreservesDispatchContextForProviderRunner(t *testing.T) {
	t.Parallel()

	runner := &recordingProviderCommandRunner{}
	effect := codex.NewCommandEffect(runner, platformclock.Real{})
	if effect == nil {
		t.Fatal("NewCommandEffect() returned nil")
	}

	_, err := effect.Execute(context.Background(), execution.ContinuationRequest{ExecuteRequest: providers.ExecuteRequest{
		Provider:        providers.IDCodex,
		AttemptID:       "mock-dispatch",
		UserMessage:     "perform work",
		WorkerType:      "mocked-worker",
		WorkstationName: "mock-process",
		Correlation: providers.ExecuteCorrelation{
			FactorySessionID: "factory-session-1",
			RequestID:        "request-1",
			TraceID:          "trace-1",
		},
	}}, func([]byte) error { return nil })
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if runner.calls != 1 {
		t.Fatalf("provider runner calls = %d, want 1", runner.calls)
	}
	if runner.request.AttemptID != "mock-dispatch" ||
		runner.request.FactorySessionID != "factory-session-1" ||
		runner.request.WorkerType != "mocked-worker" ||
		runner.request.WorkstationName != "mock-process" ||
		runner.request.Execution.RequestID != "request-1" ||
		runner.request.Execution.TraceID != "trace-1" {
		t.Fatalf("provider request metadata = %#v", runner.request)
	}
}

type recordingProviderCommandRunner struct {
	request providerservice.CommandRequest
	calls   int
}

func (runner *recordingProviderCommandRunner) Run(
	_ context.Context,
	request providerservice.CommandRequest,
) (providerservice.CommandResult, error) {
	runner.request = request
	runner.calls++
	return providerservice.CommandResult{}, nil
}

func TestCommandEffectRejectsUnsupportedReasoningEffortBeforeDispatch(t *testing.T) {
	t.Parallel()

	platformRunner := testutil.NewProviderCommandRunner()
	effect := codex.NewCommandEffect(platformRunner, platformclock.Real{})
	_, err := effect.Execute(context.Background(), execution.ContinuationRequest{ExecuteRequest: providers.ExecuteRequest{
		Provider:        providers.IDCodex,
		AttemptID:       "invalid-effort-dispatch",
		ReasoningEffort: "extreme",
		UserMessage:     "perform work",
	}}, func([]byte) error { return nil })
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

func TestCommandEffectRendersResumeSessionBeforeFreshSessionFlags(t *testing.T) {
	t.Parallel()

	platformRunner := testutil.NewProviderCommandRunner()
	effect := codex.NewCommandEffect(platformRunner, platformclock.Real{})
	if effect == nil {
		t.Fatal("NewCommandEffect() returned nil")
	}

	_, err := effect.Execute(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{
			Provider:        providers.IDCodex,
			AttemptID:       "resume-dispatch",
			Model:           "gpt-5.6-luna",
			ReasoningEffort: "xhigh",
			UserMessage:     "continue the prior turn",
		},
		ResumeSession: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "thread-previous",
		},
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
		"resume", "thread-previous",
		"-",
	}
	if !reflect.DeepEqual(request.Args, want) {
		t.Fatalf("command args = %#v, want %#v - a continued attempt must resume the exact referenced session instead of starting a fresh one", request.Args, want)
	}
}

func TestCommandEffectRendersLunaXHighReasoningEffort(t *testing.T) {
	t.Parallel()

	platformRunner := testutil.NewProviderCommandRunner()
	effect := codex.NewCommandEffect(platformRunner, platformclock.Real{})
	if effect == nil {
		t.Fatal("NewCommandEffect() returned nil")
	}

	_, err := effect.Execute(context.Background(), execution.ContinuationRequest{ExecuteRequest: providers.ExecuteRequest{
		Provider:        providers.IDCodex,
		AttemptID:       "luna-xhigh-dispatch",
		Model:           "gpt-5.6-luna",
		ReasoningEffort: "xhigh",
		UserMessage:     "perform work",
	}}, func([]byte) error { return nil })
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
