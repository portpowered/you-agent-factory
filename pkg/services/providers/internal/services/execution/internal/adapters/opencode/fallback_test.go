package opencode_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	catalogwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/catalog/wire"
	opencode "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/opencode"
	executionwire "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
)

const (
	privatePrompt = "private prompt material"
	privateSecret = "private-secret-value"
)

func TestOpenCodeRootFallsBackOnceOnUnsupportedStructuredFormat(t *testing.T) {
	t.Parallel()

	rejection := []byte("error: unknown option '--format'\n")
	runner := &sequenceStreamingRunner{attempts: []streamingAttempt{
		{result: workerprocess.CommandResult{Stderr: rejection, ExitCode: 2}},
		{result: workerprocess.CommandResult{Stdout: []byte("fallback answer")}},
	}}
	root := newOpenCodeCommandRoot(t, runner)

	result, err := root.Execute(t.Context(), openCodeFallbackRequest())
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertSafeFallbackResult(t, runner.requests, result, rejection)
}

func TestOpenCodeRootCachesFinalOnlyAfterFallback(t *testing.T) {
	t.Parallel()

	rejection := []byte("error: unknown option '--format'\n")
	runner := &sequenceStreamingRunner{attempts: []streamingAttempt{
		{result: workerprocess.CommandResult{Stderr: rejection, ExitCode: 2}},
		{result: workerprocess.CommandResult{Stdout: []byte("fallback answer")}},
		{result: workerprocess.CommandResult{Stdout: []byte("cached answer")}},
	}}
	root := newOpenCodeCommandRoot(t, runner)

	first, err := root.Execute(t.Context(), openCodeFallbackRequest())
	if err != nil {
		t.Fatalf("first Execute() error = %v", err)
	}
	assertSafeFallbackResult(t, runner.requests[:2], first, rejection)

	second, err := root.Execute(t.Context(), openCodeFallbackRequest())
	if err != nil {
		t.Fatalf("second Execute() error = %v", err)
	}
	if second.Content != "cached answer" {
		t.Fatalf("second Content = %q", second.Content)
	}
	if len(runner.requests) != 3 || containsArgs(runner.requests[2].Args, "--format", "json") {
		t.Fatalf("cached requests = %#v", runner.requests)
	}
}

func TestOpenCodeStructuredFallbackRejectsUnsafeOutcomes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		attempt streamingAttempt
	}{
		{
			name: "provider activity",
			attempt: streamingAttempt{
				observations: []streamObservation{
					{stream: workerprocess.OutputStreamStdout, chunk: []byte(`{"type":"step_start","sessionID":"ses_1"}` + "\n")},
				},
				result: workerprocess.CommandResult{
					Stdout:   []byte(`{"type":"step_start","sessionID":"ses_1"}` + "\n"),
					Stderr:   []byte("unknown option --format"),
					ExitCode: 2,
				},
			},
		},
		{
			name: "ambiguous process rejection",
			attempt: streamingAttempt{
				result: workerprocess.CommandResult{
					Stderr:   []byte("format initialization failed"),
					ExitCode: 2,
				},
			},
		},
		{
			name: "malformed stdout",
			attempt: streamingAttempt{
				observations: []streamObservation{
					{stream: workerprocess.OutputStreamStdout, chunk: []byte("not-json")},
				},
				result: workerprocess.CommandResult{
					Stdout:   []byte("not-json"),
					Stderr:   []byte("unknown option --format"),
					ExitCode: 2,
				},
			},
		},
		{
			name: "explicit provider failure",
			attempt: streamingAttempt{
				observations: []streamObservation{
					{stream: workerprocess.OutputStreamStdout, chunk: []byte(`{"type":"error","error":{"name":"AuthError","data":{"status":401}}}`)},
				},
				result: workerprocess.CommandResult{
					Stdout:   []byte(`{"type":"error","error":{"name":"AuthError","data":{"status":401}}}`),
					Stderr:   []byte("unknown option --format"),
					ExitCode: 2,
				},
			},
		},
		{
			name: "cancellation",
			attempt: streamingAttempt{
				result: workerprocess.CommandResult{Stderr: []byte("unknown option --format"), ExitCode: 2},
				err:    context.Canceled,
			},
		},
		{
			name: "timeout",
			attempt: streamingAttempt{
				result: workerprocess.CommandResult{Stderr: []byte("unknown option --format"), ExitCode: 2},
				err:    context.DeadlineExceeded,
			},
		},
		{
			name: "ambiguous process error",
			attempt: streamingAttempt{
				result: workerprocess.CommandResult{Stderr: []byte("unknown option --format"), ExitCode: 2},
				err:    errors.New("process transport failed"),
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runner := &sequenceStreamingRunner{attempts: []streamingAttempt{tc.attempt}}
			root := newOpenCodeCommandRoot(t, runner)
			_, err := root.Execute(t.Context(), openCodeFallbackRequest())
			if len(runner.requests) != 1 {
				t.Fatalf("unsafe outcome retried: requests=%d", len(runner.requests))
			}
			if err == nil {
				t.Fatal("unsafe outcome succeeded")
			}
		})
	}
}

func TestOpenCodeRequiredStructuredStreamRejectsUnsupportedModeWithoutFallback(t *testing.T) {
	t.Parallel()

	rejection := []byte("unknown option: --format")
	runner := &sequenceStreamingRunner{attempts: []streamingAttempt{
		{result: workerprocess.CommandResult{Stderr: rejection, ExitCode: 2}},
	}}
	root := newOpenCodeCommandRootWithOptions(t, runner, opencode.RegistrationOptions{
		RequireStructuredStream: true,
	})

	_, err := root.Execute(t.Context(), providers.ExecuteRequest{
		Provider:    providers.IDOpenCode,
		AttemptID:   "attempt-required-structured",
		UserMessage: privatePrompt,
		EnvVars: map[string]string{
			"providers_require_structured_stream": "true",
		},
	})
	if err == nil {
		t.Fatal("required structured execution fell back")
	}
	if len(runner.requests) != 1 {
		t.Fatalf("required structured execution retried: requests=%d", len(runner.requests))
	}
}

func TestOpenCodeFallbackAttemptNeverRecurses(t *testing.T) {
	t.Parallel()

	rejection := []byte("unknown option: --format")
	runner := &sequenceStreamingRunner{attempts: []streamingAttempt{
		{result: workerprocess.CommandResult{Stderr: rejection, ExitCode: 2}},
		{result: workerprocess.CommandResult{Stderr: rejection, ExitCode: 2}},
	}}
	root := newOpenCodeCommandRoot(t, runner)

	_, err := root.Execute(t.Context(), openCodeFallbackRequest())
	if err == nil {
		t.Fatal("second attempt succeeded")
	}
	if len(runner.requests) != 2 {
		t.Fatalf("process launches = %d, want 2", len(runner.requests))
	}
}

func openCodeFallbackRequest() providers.ExecuteRequest {
	return providers.ExecuteRequest{
		Provider:    providers.IDOpenCode,
		AttemptID:   "attempt-fallback",
		UserMessage: privatePrompt,
		Model:       "openai/gpt-5",
	}
}

func newOpenCodeCommandRoot(t *testing.T, runner workers.CommandRunner) providers.Service {
	t.Helper()
	return newOpenCodeCommandRootWithOptions(t, runner, opencode.RegistrationOptions{})
}

func newOpenCodeCommandRootWithOptions(
	t *testing.T,
	runner workers.CommandRunner,
	options opencode.RegistrationOptions,
) providers.Service {
	t.Helper()
	catalog, err := catalogwire.NewService()
	if err != nil {
		t.Fatal(err)
	}
	effect := opencode.NewCommandEffect(runner)
	executionService, err := executionwire.NewService(
		catalog,
		opencode.NewRegistrationWithOptions(effect, opencode.ModeStructured, options),
	)
	if err != nil {
		t.Fatal(err)
	}
	root, err := providerservice.New(catalog, executionService)
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func assertSafeFallbackResult(
	t *testing.T,
	requests []workers.CommandRequest,
	result providers.ExecuteResult,
	rejection []byte,
) {
	t.Helper()
	if len(requests) != 2 ||
		!containsArgs(requests[0].Args, "--format", "json") ||
		containsArgs(requests[1].Args, "--format", "json") {
		t.Fatalf("requests = %#v", requests)
	}
	if countFinalOnlyLaunches(requests) != 1 {
		t.Fatalf("customer-work launches = %d, want 1", countFinalOnlyLaunches(requests))
	}
	if result.Content != "fallback answer" {
		t.Fatalf("Content = %q", result.Content)
	}
	assertSafeDegradationDiagnostic(t, result.Diagnostics.Progress, rejection)
}

func assertSafeDegradationDiagnostic(
	t *testing.T,
	progress []providers.ExecuteProgress,
	rejection []byte,
) {
	t.Helper()
	var diagnostic *providers.ExecuteProgress
	for index := range progress {
		if progress[index].Metadata["code"] == "structured_mode_degraded" {
			diagnostic = &progress[index]
			break
		}
	}
	if diagnostic == nil {
		t.Fatalf("diagnostics = %#v", progress)
	}
	message := diagnostic.Detail
	for _, forbidden := range []string{privatePrompt, privateSecret, "fallback answer", string(rejection)} {
		if strings.Contains(message, forbidden) {
			t.Fatalf("degradation diagnostic exposed %q: %#v", forbidden, diagnostic)
		}
	}
	for _, required := range []string{"OpenCode", "final_only", "unsupported_format"} {
		if !strings.Contains(message, required) {
			t.Fatalf("degradation diagnostic lacks %q: %#v", required, diagnostic)
		}
	}
}

func containsArgs(args []string, want ...string) bool {
	for index := 0; index+len(want) <= len(args); index++ {
		if reflect.DeepEqual(args[index:index+len(want)], want) {
			return true
		}
	}
	return false
}

func countFinalOnlyLaunches(requests []workers.CommandRequest) int {
	count := 0
	for _, request := range requests {
		if !containsArgs(request.Args, "--format", "json") {
			count++
		}
	}
	return count
}

type streamObservation struct {
	stream string
	chunk  []byte
}

type streamingAttempt struct {
	observations []streamObservation
	result       workerprocess.CommandResult
	err          error
}

type sequenceStreamingRunner struct {
	attempts []streamingAttempt
	requests []workers.CommandRequest
}

func (runner *sequenceStreamingRunner) Run(
	ctx context.Context,
	request workers.CommandRequest,
) (workers.CommandResult, error) {
	return runner.runAttempt(ctx, request, nil)
}

func (runner *sequenceStreamingRunner) RunStreaming(
	ctx context.Context,
	request workers.CommandRequest,
	observe workerprocess.OutputChunkObserver,
) (workers.CommandResult, error) {
	return runner.runAttempt(ctx, request, observe)
}

func (runner *sequenceStreamingRunner) runAttempt(
	ctx context.Context,
	request workers.CommandRequest,
	observe workerprocess.OutputChunkObserver,
) (workers.CommandResult, error) {
	runner.requests = append(runner.requests, request)
	if len(runner.attempts) == 0 {
		return workers.CommandResult{}, errors.New("unexpected process launch")
	}
	attempt := runner.attempts[0]
	runner.attempts = runner.attempts[1:]
	if observe != nil {
		for _, observation := range attempt.observations {
			observe(observation.stream, observation.chunk)
		}
		if len(attempt.observations) == 0 && len(attempt.result.Stdout) > 0 {
			observe(workerprocess.OutputStreamStdout, attempt.result.Stdout)
		}
	}
	return attempt.result, attempt.err
}

var _ workers.CommandRunner = (*sequenceStreamingRunner)(nil)
