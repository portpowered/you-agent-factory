package codex_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	codex "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/codex"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
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
	effect := codex.NewCommandEffect(executionwire.AdaptPlatformCommandRunner(platformRunner), platformclock.Real{})
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
	effect := codex.NewCommandEffect(executionwire.AdaptPlatformCommandRunner(platformRunner), platformclock.Real{})
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

func TestCommandEffectForwardsWorkerArgsBeforeResumeAndPrompt(t *testing.T) {
	t.Parallel()

	platformRunner := testutil.NewProviderCommandRunner()
	effect := codex.NewCommandEffect(executionwire.AdaptPlatformCommandRunner(platformRunner), platformclock.Real{})
	if effect == nil {
		t.Fatal("NewCommandEffect() returned nil")
	}

	_, err := effect.Execute(context.Background(), execution.ContinuationRequest{
		ExecuteRequest: providers.ExecuteRequest{
			Provider:        providers.IDCodex,
			AttemptID:       "worker-args-dispatch",
			Model:           "gpt-5.6-luna",
			ReasoningEffort: "high",
			SkipPermissions: true,
			Args:            []string{"--config", "mcp_servers.playwright.enabled=false", " ", "--disable", "plugins"},
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
	want := []string{
		"exec",
		"--json",
		"--dangerously-bypass-approvals-and-sandbox",
		"--model", "gpt-5.6-luna",
		"--config", `model_reasoning_effort="high"`,
		"--config", "mcp_servers.playwright.enabled=false",
		"--disable", "plugins",
		"resume", "thread-previous",
		"-",
	}
	if got := platformRunner.LastRequest().Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("command args = %#v, want %#v - worker-authored args must reach codex exec before the resume subcommand", got, want)
	}
}

func TestCommandEffectRendersLunaXHighReasoningEffort(t *testing.T) {
	t.Parallel()

	platformRunner := testutil.NewProviderCommandRunner()
	effect := codex.NewCommandEffect(executionwire.AdaptPlatformCommandRunner(platformRunner), platformclock.Real{})
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

// The runner advances only the supplied clock: elapsed host time cannot satisfy
// this assertion, including on unsuccessful attempts.
func TestCommandEffectUsesInjectedRunnerAndClockOnTerminalPaths(t *testing.T) {
	t.Parallel()
	observerErr := errors.New("observer stopped")
	for _, tc := range []struct {
		name       string
		runErr     error
		observeErr error
		wantKind   providers.ExecuteFailureKind
	}{
		{name: "success"},
		{name: "timeout", runErr: context.DeadlineExceeded, wantKind: providers.ExecuteFailureKindTimeout},
		{name: "cancellation", runErr: context.Canceled, wantKind: providers.ExecuteFailureKindCanceled},
		{name: "observer failure", observeErr: observerErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			clock := platformclock.NewDeterministic(time.Unix(0, 0).UTC(), time.Millisecond)
			calls := 0
			runner := terminalCommandRunner(func(ctx context.Context, request providerservice.CommandRequest) (providerservice.CommandResult, error) {
				calls++
				if request.FactorySessionID != "owned-session" || request.Command != "codex" {
					t.Fatalf("command = %#v, want owned session and provider", request)
				}
				clock.SetTick(37)
				return providerservice.CommandResult{Stdout: []byte("delivered"), Stderr: []byte("private diagnostic")}, tc.runErr
			})
			effect := codex.NewCommandEffect(runner, clock)
			var observed strings.Builder
			result, err := effect.Execute(t.Context(), execution.ContinuationRequest{ExecuteRequest: providers.ExecuteRequest{
				Provider:    providers.IDCodex,
				UserMessage: "perform work",
				Correlation: providers.ExecuteCorrelation{FactorySessionID: "owned-session"},
			}}, func(chunk []byte) error {
				observed.Write(chunk)
				return tc.observeErr
			})
			if calls != 1 || result.DurationMillis != 37 || observed.String() != "delivered" {
				t.Fatalf("calls = %d, duration = %d, stdout = %q", calls, result.DurationMillis, observed.String())
			}
			if tc.wantKind != "" {
				var failure providers.ExecuteFailure
				if !errors.As(err, &failure) || failure.Kind != tc.wantKind {
					t.Fatalf("error = %v, want %s", err, tc.wantKind)
				}
			} else if !errors.Is(err, tc.observeErr) {
				t.Fatalf("error = %v, want %v", err, tc.observeErr)
			}
		})
	}
}

type terminalCommandRunner func(context.Context, providerservice.CommandRequest) (providerservice.CommandResult, error)

func (runner terminalCommandRunner) Run(ctx context.Context, request providerservice.CommandRequest) (providerservice.CommandResult, error) {
	return runner(ctx, request)
}
