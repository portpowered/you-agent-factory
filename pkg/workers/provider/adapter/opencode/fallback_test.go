package opencode_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/work"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter/opencode"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter/testkit"
)

func TestOpenCodeFinalOnlyAdapterConformance(t *testing.T) {
	t.Parallel()
	decision := opencode.Decision{Installation: installation(), Version: "0.9.0", Mode: opencode.ModeFinalOnly}
	newAdapter := func() adapter.Adapter {
		providerAdapter, err := opencode.NewNegotiatedAdapter(decision, nil)
		if err != nil {
			t.Fatalf("NewNegotiatedAdapter() error = %v", err)
		}
		return providerAdapter
	}
	testkit.RunFinalOnly(t, testkit.FinalOnlyFixture{
		NewAdapter: newAdapter,
		Request: workerexecution.ProviderInferenceRequest{
			Model: "openai/gpt-5", UserMessage: privatePrompt,
		},
		Success: workerprocess.CommandResult{Stdout: []byte("Complete response\n")},
		Failures: []testkit.FinalOnlyFailureCase{
			{Name: "empty final output is normalized", Result: workerprocess.CommandResult{}},
			{Name: "invalid final output is normalized", Result: workerprocess.CommandResult{Stdout: []byte{0xff}}},
		},
		Expected:            testkit.FinalOnlyExpected{Content: "Complete response"},
		ForbiddenDiagnostic: []string{privatePrompt, privateSecret},
	})
}

func TestKnownUnsupportedOpenCodeExecutesCustomerWorkOnce(t *testing.T) {
	t.Parallel()
	providerAdapter := mustNegotiatedAdapter(t, opencode.Decision{
		Installation: installation(), Version: "0.9.0", Mode: opencode.ModeFinalOnly,
	}, nil)
	runner := &sequenceStreamingRunner{attempts: []streamingAttempt{{
		observations: []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: []byte("final answer")}},
		result:       workerprocess.CommandResult{Stdout: []byte("final answer")},
	}}}
	result, err := executeOpenCode(t, providerAdapter, runner)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Response.Content != "final answer" || !result.Capabilities.FinalOnly || len(result.Drafts) != 3 {
		t.Fatalf("result = %#v", result)
	}
	if len(runner.requests) != 1 || containsArgs(runner.requests[0].Args, "--format", "json") {
		t.Fatalf("requests = %#v", runner.requests)
	}
	assertOnlyFinalSnapshotActivity(t, result.Drafts)
}

func TestStaleStructuredCapabilityFallsBackOnceAndCachesDowngrade(t *testing.T) {
	t.Parallel()
	discoverer := &fakeDiscoverer{decision: opencode.Decision{
		Version: "opencode 1.2.3 " + privatePrompt + " " + privateSecret, Mode: opencode.ModeStructured,
	}}
	resolver := newResolver(t, &fakeIdentifier{installation: installation()}, discoverer)
	providerAdapter := mustNegotiatedAdapter(t, resolve(t, resolver), resolver)
	rejection := []byte("error: unknown option '--format'\n")
	runner := &sequenceStreamingRunner{attempts: []streamingAttempt{
		{
			observations: []adapter.Observation{{Stream: adapter.OutputStreamStderr, Chunk: rejection}},
			result:       workerprocess.CommandResult{Stderr: rejection, ExitCode: 2},
		},
		{
			observations: []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: []byte("fallback answer")}},
			result:       workerprocess.CommandResult{Stdout: []byte("fallback answer")},
		},
	}}
	result, err := executeOpenCode(t, providerAdapter, runner)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	assertSafeFallbackResult(t, runner.requests, result, rejection)
	assertOnlyFinalSnapshotActivity(t, result.Drafts)

	cached := resolve(t, resolver)
	assertCachedDowngrade(t, cached, discoverer)
	nextRunner := &sequenceStreamingRunner{attempts: []streamingAttempt{{result: workerprocess.CommandResult{Stdout: []byte("next answer")}}}}
	next := mustNegotiatedAdapter(t, cached, resolver)
	if _, err := executeOpenCode(t, next, nextRunner); err != nil {
		t.Fatalf("cached final-only Execute() error = %v", err)
	}
	if len(nextRunner.requests) != 1 || containsArgs(nextRunner.requests[0].Args, "--format", "json") {
		t.Fatalf("cached requests = %#v", nextRunner.requests)
	}
}

func assertSafeFallbackResult(t *testing.T, requests []workerprocess.CommandRequest, result adapter.ExecuteResult, rejection []byte) {
	t.Helper()
	if len(requests) != 2 || !containsArgs(requests[0].Args, "--format", "json") || containsArgs(requests[1].Args, "--format", "json") {
		t.Fatalf("requests = %#v", requests)
	}
	if countFinalOnlyLaunches(requests) != 1 {
		t.Fatalf("customer-work launches = %d, want 1", countFinalOnlyLaunches(requests))
	}
	if result.Response.Content != "fallback answer" || !result.Capabilities.FinalOnly || len(result.CapabilityUpdates) != 1 {
		t.Fatalf("fallback result = %#v", result)
	}
	update := result.CapabilityUpdates[0]
	if update.Provider != "opencode" || !update.Capabilities.FinalOnly || update.Capabilities.NativeStreaming {
		t.Fatalf("capability update = %#v", update)
	}
	assertSafeDegradationDiagnostic(t, result.Diagnostics, rejection)
}

func assertSafeDegradationDiagnostic(t *testing.T, diagnostics []adapter.Diagnostic, rejection []byte) {
	t.Helper()
	if len(diagnostics) != 1 || diagnostics[0].Code != "structured_mode_degraded" {
		t.Fatalf("diagnostics = %#v", diagnostics)
	}
	message := diagnostics[0].Message
	for _, forbidden := range []string{privatePrompt, privateSecret, "fallback answer", string(rejection)} {
		if strings.Contains(message, forbidden) {
			t.Fatalf("degradation diagnostic exposed %q: %#v", forbidden, diagnostics[0])
		}
	}
	for _, required := range []string{"OpenCode", "final_only", "unsupported_format", "1.2.3"} {
		if !strings.Contains(message, required) {
			t.Fatalf("degradation diagnostic lacks %q: %#v", required, diagnostics[0])
		}
	}
}

func assertCachedDowngrade(t *testing.T, cached opencode.Decision, discoverer *fakeDiscoverer) {
	t.Helper()
	if cached.Mode != opencode.ModeFinalOnly || discoverer.calls.Load() != 1 {
		t.Fatalf("cached decision = %#v, discovery calls = %d", cached, discoverer.calls.Load())
	}
}

func TestStructuredFallbackRejectsUnsafeOutcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		attempt streamingAttempt
	}{
		{
			name: "provider activity",
			attempt: streamingAttempt{
				observations: []adapter.Observation{
					{Stream: adapter.OutputStreamStdout, Chunk: []byte(`{"type":"step_start","sessionID":"ses_1"}` + "\n")},
					{Stream: adapter.OutputStreamStderr, Chunk: []byte("unknown option --format")},
				},
				result: workerprocess.CommandResult{Stderr: []byte("unknown option --format"), ExitCode: 2},
			},
		},
		{
			name: "ambiguous process rejection",
			attempt: streamingAttempt{
				observations: []adapter.Observation{{Stream: adapter.OutputStreamStderr, Chunk: []byte("format initialization failed")}},
				result:       workerprocess.CommandResult{Stderr: []byte("format initialization failed"), ExitCode: 2},
			},
		},
		{
			name: "malformed stdout",
			attempt: streamingAttempt{
				observations: []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: []byte("not-json")}},
				result:       workerprocess.CommandResult{Stdout: []byte("not-json"), Stderr: []byte("unknown option --format"), ExitCode: 2},
			},
		},
		{
			name: "explicit provider failure",
			attempt: streamingAttempt{
				observations: []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: []byte(`{"type":"error","error":{"name":"AuthError","data":{"status":401}}}`)}},
				result:       workerprocess.CommandResult{Stdout: []byte(`{"type":"error","error":{"name":"AuthError","data":{"status":401}}}`), Stderr: []byte("unknown option --format"), ExitCode: 2},
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
			resolver := newResolver(t, &fakeIdentifier{installation: installation()}, &fakeDiscoverer{
				decision: opencode.Decision{Version: "1.2.3", Mode: opencode.ModeStructured},
			})
			providerAdapter := mustNegotiatedAdapter(t, resolve(t, resolver), resolver)
			runner := &sequenceStreamingRunner{attempts: []streamingAttempt{tc.attempt}}
			result, _ := executeOpenCode(t, providerAdapter, runner)
			if len(runner.requests) != 1 || len(result.CapabilityUpdates) != 0 {
				t.Fatalf("unsafe outcome retried: requests=%d updates=%#v", len(runner.requests), result.CapabilityUpdates)
			}
			if cached := resolve(t, resolver); cached.Mode != opencode.ModeStructured {
				t.Fatalf("unsafe outcome changed cache: %#v", cached)
			}
		})
	}
}

func TestFallbackFailureUsesDirectFinalOnlyTerminalSemantics(t *testing.T) {
	testCases := []struct {
		name   string
		result workerprocess.CommandResult
		want   workerexecution.WorkFailureType
	}{
		{name: "authentication", result: workerprocess.CommandResult{ExitCode: 1, Stderr: []byte(`{"type":"error","error":{"name":"ProviderAuthError"}}`)}, want: workerexecution.WorkFailureTypeAuthFailure},
		{name: "bad request", result: workerprocess.CommandResult{ExitCode: 1, Stderr: []byte(`{"type":"error","error":{"name":"APIError","data":{"status":400}}}`)}, want: workerexecution.WorkFailureTypePermanentBadRequest},
		{name: "throttle", result: workerprocess.CommandResult{ExitCode: 1, Stderr: []byte("Error: rate limit exceeded")}, want: workerexecution.WorkFailureTypeThrottled},
		{name: "timeout", result: workerprocess.CommandResult{ExitCode: 124}, want: workerexecution.WorkFailureTypeTimeout},
		{name: "server", result: workerprocess.CommandResult{ExitCode: 1, Stderr: []byte("API Error: internal server error")}, want: workerexecution.WorkFailureTypeInternalServerError},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			resolver := newResolver(t, &fakeIdentifier{installation: installation()}, &fakeDiscoverer{
				decision: opencode.Decision{Version: "1.2.3", Mode: opencode.ModeStructured},
			})
			providerAdapter := mustNegotiatedAdapter(t, resolve(t, resolver), resolver)
			rejection := []byte("unknown flag: --format")
			fallbackResult, _ := executeOpenCode(t, providerAdapter, &sequenceStreamingRunner{attempts: []streamingAttempt{
				{result: workerprocess.CommandResult{Stderr: rejection, ExitCode: 2}},
				{result: tc.result},
			}})
			direct := mustNegotiatedAdapter(t, opencode.Decision{Installation: installation(), Version: "0.9.0", Mode: opencode.ModeFinalOnly}, nil)
			directResult, _ := executeOpenCode(t, direct, &sequenceStreamingRunner{attempts: []streamingAttempt{{result: tc.result}}})
			assertTerminalParity(t, fallbackResult.Failure, directResult.Failure, tc.want)
		})
	}
}

func assertTerminalParity(t *testing.T, fallback, direct *adapter.FailureFacts, want workerexecution.WorkFailureType) {
	t.Helper()
	if fallback == nil || direct == nil || direct.Type != want {
		t.Fatalf("failures = %#v / %#v, want %s", fallback, direct, want)
	}
	if fallback.Family != direct.Family || fallback.Type != direct.Type || fallback.Retry != direct.Retry || fallback.Message != direct.Message {
		t.Fatalf("terminal semantics differ: %#v / %#v", fallback, direct)
	}
}

func TestRequiredStructuredOutputRejectsUnsupportedModeWithoutFallback(t *testing.T) {
	t.Parallel()
	resolver := newResolver(t, &fakeIdentifier{installation: installation()}, &fakeDiscoverer{
		decision: opencode.Decision{Version: "1.2.3", Mode: opencode.ModeStructured},
	})
	decision := resolve(t, resolver)
	providerAdapter, err := opencode.NewNegotiatedAdapterForRequest(decision, resolver, workerexecution.ProviderInferenceRequest{
		RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{workerexecution.RunnerOptionalCapabilityStructuredOutput},
	})
	if err != nil {
		t.Fatalf("NewNegotiatedAdapterForRequest() error = %v", err)
	}
	rejection := []byte("unknown option: --format")
	runner := &sequenceStreamingRunner{attempts: []streamingAttempt{{
		result: workerprocess.CommandResult{Stderr: rejection, ExitCode: 2},
	}}}

	result, executeErr := executeOpenCode(t, providerAdapter, runner)
	if executeErr == nil || result.Failure == nil || result.Failure.Type != workerexecution.WorkFailureTypePermanentBadRequest {
		t.Fatalf("result = %#v, error = %v; want permanent bad request", result, executeErr)
	}
	if len(runner.requests) != 1 || len(result.CapabilityUpdates) != 0 {
		t.Fatalf("required structured execution fell back: requests=%d updates=%#v", len(runner.requests), result.CapabilityUpdates)
	}
	if cached := resolve(t, resolver); cached.Mode != opencode.ModeStructured {
		t.Fatalf("required structured rejection changed cache: %#v", cached)
	}
}

func TestFallbackAttemptNeverRecurses(t *testing.T) {
	t.Parallel()
	resolver := newResolver(t, &fakeIdentifier{installation: installation()}, &fakeDiscoverer{
		decision: opencode.Decision{Version: "1.2.3", Mode: opencode.ModeStructured},
	})
	providerAdapter := mustNegotiatedAdapter(t, resolve(t, resolver), resolver)
	rejection := []byte("unknown option: --format")
	runner := &sequenceStreamingRunner{attempts: []streamingAttempt{
		{result: workerprocess.CommandResult{Stderr: rejection, ExitCode: 2}},
		{result: workerprocess.CommandResult{Stderr: rejection, ExitCode: 2}},
	}}
	result, err := executeOpenCode(t, providerAdapter, runner)
	if err == nil || result.Failure == nil {
		t.Fatalf("second attempt result = %#v, error = %v", result, err)
	}
	if len(runner.requests) != 2 || len(runner.attempts) != 0 {
		t.Fatalf("process launches = %d, remaining fixtures = %d", len(runner.requests), len(runner.attempts))
	}
}

func executeOpenCode(t *testing.T, providerAdapter adapter.Adapter, runner adapter.StreamingCommandRunner) (adapter.ExecuteResult, error) {
	t.Helper()
	registry, err := adapter.NewRegistry(providerAdapter)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	return adapter.Execute(context.Background(), registry, runner, adapter.ExecuteInput{
		Provider: providerAdapter.Identity(),
		Command: adapter.CommandContext{Request: workerexecution.ProviderInferenceRequest{
			Dispatch: work.WorkDispatch{DispatchID: "dispatch-fallback"},
			Model:    "openai/gpt-5", UserMessage: privatePrompt,
		}},
		Decoder: adapter.DecoderContext{RunID: "run-fallback", DispatchID: "dispatch-fallback"},
	})
}

func mustNegotiatedAdapter(t *testing.T, decision opencode.Decision, resolver *opencode.Resolver) *opencode.NegotiatedAdapter {
	t.Helper()
	providerAdapter, err := opencode.NewNegotiatedAdapter(decision, resolver)
	if err != nil {
		t.Fatalf("NewNegotiatedAdapter() error = %v", err)
	}
	return providerAdapter
}

func assertOnlyFinalSnapshotActivity(t *testing.T, drafts []responseevents.Draft) {
	t.Helper()
	for _, draft := range drafts {
		if draft.Kind == responseevents.KindTool || draft.Kind == responseevents.KindReasoning || draft.Phase == responseevents.PhaseDelta {
			t.Fatalf("final-only execution fabricated activity: %#v", drafts)
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

func countFinalOnlyLaunches(requests []workerprocess.CommandRequest) int {
	count := 0
	for _, request := range requests {
		if !containsArgs(request.Args, "--format", "json") {
			count++
		}
	}
	return count
}

type streamingAttempt struct {
	observations []adapter.Observation
	result       workerprocess.CommandResult
	err          error
}

type sequenceStreamingRunner struct {
	attempts []streamingAttempt
	requests []workerprocess.CommandRequest
}

func (r *sequenceStreamingRunner) Run(ctx context.Context, request workerprocess.CommandRequest, observe func(adapter.Observation) error) (workerprocess.CommandResult, error) {
	r.requests = append(r.requests, request)
	if len(r.attempts) == 0 {
		return workerprocess.CommandResult{}, errors.New("unexpected process launch")
	}
	attempt := r.attempts[0]
	r.attempts = r.attempts[1:]
	for _, observation := range attempt.observations {
		if err := observe(observation); err != nil {
			return attempt.result, err
		}
	}
	return attempt.result, attempt.err
}

var _ adapter.StreamingCommandRunner = (*sequenceStreamingRunner)(nil)
