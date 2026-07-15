package pi_test

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/work"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter/testkit"
	"github.com/portpowered/infinite-you/pkg/workers/provider/pi"
	"github.com/portpowered/infinite-you/pkg/workers/provider/structured"
)

const (
	privateConformancePrompt = "private Pi prompt must not escape"
	privateConformanceToken  = "sk-pi-fixture-secret"
)

func TestAdapterFullStreamConformance(t *testing.T) {
	t.Parallel()

	testkit.RunFullStream(t, testkit.FullStreamFixture{
		NewAdapter: func() adapter.Adapter { return pi.NewAdapter() },
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch: work.WorkDispatch{DispatchID: "dispatch-conformance"},
			Model:    "anthropic/claude-sonnet-4", UserMessage: privateConformancePrompt,
		},
		ContentAndTools: piObservations(
			`{"type":"agent_start"}`,
			`{"type":"message_update","message":{"id":"msg-conformance","role":"assistant","content":[]},"assistantMessageEvent":{"type":"text_delta","delta":"Hello ","contentIndex":0}}`,
			`{"type":"message_update","message":{"id":"msg-conformance","role":"assistant","content":[]},"assistantMessageEvent":{"type":"text_delta","delta":"world","contentIndex":0}}`,
			`{"type":"message_end","message":{"id":"msg-conformance","role":"assistant","content":[{"type":"text","text":"Hello world"}],"stopReason":"stop"}}`,
			`{"type":"tool_execution_start","toolCallId":"call-conformance","toolName":"weather","args":{"city":"Oslo"}}`,
			`{"type":"tool_execution_update","toolCallId":"call-conformance","partialResult":{"temperature":12}}`,
			`{"type":"tool_execution_end","toolCallId":"call-conformance","toolName":"weather","result":{"temperature":12}}`,
		),
		RetryableFailure: piObservations(
			`{"type":"session","id":"pi-session-conformance"}`,
			`{"type":"auto_retry_start","attempt":2,"retryDelayMs":2000,"errorStatus":429}`,
		),
		UnsafeAndRecovering: piObservations(
			`{"type":`+privateConformancePrompt+`,"token":"`+privateConformanceToken+`"}`,
			`{"type":"future_shape","prompt":"`+privateConformancePrompt+`","token":"`+privateConformanceToken+`"}`,
			`{"type":"message_end","message":{"id":"msg-conformance","role":"assistant","content":[{"type":"text","text":"Hello world"}],"stopReason":"stop"}}`,
		),
		UnterminatedFinal: []adapter.Observation{{Stream: adapter.OutputStreamStdout, Chunk: []byte(
			`{"type":"message_end","message":{"id":"msg-conformance","role":"assistant","content":[{"type":"text","text":"Hello world"}],"stopReason":"stop"}}`,
		)}},
		FinalResult: workerprocess.CommandResult{Stdout: []byte(strings.Join([]string{
			`{"type":"session","id":"pi-session-conformance"}`,
			`{"type":"message_end","message":{"id":"msg-conformance","role":"assistant","content":[{"type":"text","text":"Hello world"}],"stopReason":"stop"}}`,
		}, "\n") + "\n")},
		Expected: testkit.FullStreamExpected{
			Capabilities: adapter.Capabilities{
				NativeStreaming: true, MessageDeltas: true, MessageSnapshots: true, ReasoningSummaries: true,
				ToolLifecycle: true, ToolOutputDeltas: true, StableItemIDs: true,
			},
			ProviderSession: workerexecution.ProviderSessionMetadata{Provider: "pi", Kind: "session_id", ID: "pi-session-conformance"},
			MessageItemID:   "msg-conformance",
			ToolItemID:      "call-conformance", ToolCallID: "call-conformance",
			MessageDeltas: []string{"Hello ", "world"}, FinalContent: "Hello world",
			ToolOutputDelta: `{"temperature":12}`, RetryAfter: 2,
		},
		ForbiddenDiagnostic: []string{privateConformancePrompt, privateConformanceToken},
	})
}

func piObservations(records ...string) []adapter.Observation {
	observations := make([]adapter.Observation, 0, len(records))
	for _, record := range records {
		observations = append(observations, adapter.Observation{Stream: adapter.OutputStreamStdout, Chunk: []byte(record + "\n")})
	}
	return observations
}

func TestAdapterBuildCommandRequestsNonInteractiveJSONMode(t *testing.T) {
	t.Parallel()

	built, err := pi.NewAdapter().BuildCommand(context.Background(), adapter.CommandContext{
		Request: workerexecution.ProviderInferenceRequest{
			ModelProvider: string(modelprovider.Pi),
			Model:         "anthropic/claude-sonnet-4",
			SessionID:     "pi-session-1",
			UserMessage:   "inspect the workspace",
		},
	})
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	want := []string{
		"--print", "--mode", "json", "--approve",
		"--model", "anthropic/claude-sonnet-4", "--session", "pi-session-1",
		"inspect the workspace",
	}
	if !reflect.DeepEqual(built.Request.Args, want) {
		t.Fatalf("args = %#v, want %#v", built.Request.Args, want)
	}
	if built.Request.Command != "pi" {
		t.Fatalf("command = %q, want pi", built.Request.Command)
	}
}

func TestAdapterBuildCommandWiresAuthThroughEnvironmentNotArgv(t *testing.T) {
	t.Parallel()

	built, err := pi.NewAdapter().BuildCommand(context.Background(), adapter.CommandContext{
		Request: workerexecution.ProviderInferenceRequest{
			Dispatch:         work.WorkDispatch{DispatchID: "dispatch-auth"},
			ModelProvider:    string(modelprovider.Pi),
			SystemPrompt:     "safe system prompt",
			UserMessage:      "review the workspace",
			WorkingDirectory: "workspace",
			EnvVars: map[string]string{
				"ANTHROPIC_API_KEY": privateConformanceToken,
				"PI_TEST_BOUNDARY":  "value",
			},
		},
	})
	if err != nil {
		t.Fatalf("BuildCommand() error = %v", err)
	}
	for _, arg := range built.Request.Args {
		if strings.Contains(arg, privateConformanceToken) {
			t.Fatalf("secret leaked into argv: %#v", built.Request.Args)
		}
	}
	if !slices.Contains(built.Request.Env, "ANTHROPIC_API_KEY="+privateConformanceToken) {
		t.Fatalf("auth env missing from command env: %#v", built.Request.Env)
	}
	if !slices.Contains(built.Request.Env, "PI_TEST_BOUNDARY=value") {
		t.Fatalf("explicit env missing from command env: %#v", built.Request.Env)
	}
	if !slices.Contains(built.Request.Env, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("automation env missing from command env: %#v", built.Request.Env)
	}
	if built.Request.WorkDir != "workspace" {
		t.Fatalf("workdir = %q, want workspace", built.Request.WorkDir)
	}
}

func TestAdapterReportsPiStreamingCapabilities(t *testing.T) {
	t.Parallel()

	providerAdapter := pi.NewAdapter()
	if providerAdapter.Identity() != "pi" {
		t.Fatalf("Identity() = %q", providerAdapter.Identity())
	}
	reported, err := providerAdapter.Capabilities(context.Background(), adapter.CapabilityContext{})
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	capabilities := reported.Capabilities
	if !capabilities.NativeStreaming || !capabilities.MessageDeltas || !capabilities.MessageSnapshots ||
		!capabilities.ToolLifecycle || !capabilities.StableItemIDs || capabilities.FinalOnly {
		t.Fatalf("Capabilities() = %#v", capabilities)
	}
}

func TestRegistryResolvesPiAdapterWithoutFallback(t *testing.T) {
	t.Parallel()

	registry, err := adapter.NewRegistry(pi.NewAdapter())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	got, err := registry.Lookup(" PI ")
	if err != nil {
		t.Fatalf("Lookup() error = %v", err)
	}
	if got.Identity() != "pi" {
		t.Fatalf("Lookup() identity = %q", got.Identity())
	}
}

func TestSelectableInvocationIntegrationReturnsAuthoritativeTerminalResult(t *testing.T) {
	t.Parallel()

	req := workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-pi-production"},
		ModelProvider: string(modelprovider.Pi),
		Model:         "anthropic/claude-sonnet-4",
		SessionID:     "pi-session-production",
		UserMessage:   privateConformancePrompt,
		EnvVars:       map[string]string{"ANTHROPIC_API_KEY": privateConformanceToken},
	}
	streamRunner := &recordingRunner{result: workerprovider.CommandResult{Stdout: []byte(structuredPiOutput())}}
	var published []workerprovider.InferenceProgressFragment
	streamProvider := workerprovider.NewScriptWrapProvider(
		workerprovider.WithProviderCommandRunner(streamRunner),
		workerprovider.WithInferenceProgressPublisher(func(fragment workerprovider.InferenceProgressFragment) {
			published = append(published, fragment)
		}),
		workerprovider.WithResponseStreamExecutor(structured.NewExecutor()),
	)
	streamResponse, err := streamProvider.Infer(context.Background(), req)
	if err != nil {
		t.Fatalf("response-stream Infer: %v", err)
	}
	if streamResponse.Content != "authoritative answer" {
		t.Fatalf("response content = %q", streamResponse.Content)
	}
	if streamResponse.ProviderSession == nil || streamResponse.ProviderSession.ID != "pi-session-production" {
		t.Fatalf("provider session = %#v", streamResponse.ProviderSession)
	}
	if !slices.Contains(streamRunner.request.Args, "--mode") || !slices.Contains(streamRunner.request.Args, "json") {
		t.Fatalf("args = %#v", streamRunner.request.Args)
	}
	for _, arg := range streamRunner.request.Args {
		if strings.Contains(arg, privateConformanceToken) {
			t.Fatalf("secret leaked into argv: %#v", streamRunner.request.Args)
		}
	}
	if !slices.Contains(streamRunner.request.Env, "ANTHROPIC_API_KEY="+privateConformanceToken) {
		t.Fatalf("auth env missing: %#v", streamRunner.request.Env)
	}
	if !publishedDraft(published, responseevents.KindMessage, responseevents.PhaseCompleted, "msg-production") {
		t.Fatalf("published fragments = %#v", published)
	}
}

func TestAdapterParseFinalIsIndependentFromDecoderObservations(t *testing.T) {
	t.Parallel()

	result, err := pi.NewAdapter().ParseFinal(context.Background(), adapter.FinalParseContext{
		CommandResult: workerprocess.CommandResult{Stdout: []byte(structuredPiOutput())},
		FlushReason:   adapter.FlushReasonCompleted,
	})
	if err != nil {
		t.Fatalf("ParseFinal() error = %v", err)
	}
	if result.Response.Content != "authoritative answer" || result.Response.ProviderSession == nil ||
		result.Response.ProviderSession.ID != "pi-session-production" {
		t.Fatalf("ParseFinal() = %#v", result.Response)
	}
}

func TestAdapterClassifyFailureFromTerminalAssistantMessage(t *testing.T) {
	t.Parallel()

	failure := pi.NewAdapter().ClassifyFailure(context.Background(), adapter.FailureContext{
		CommandResult: workerprocess.CommandResult{
			ExitCode: 1,
			Stdout: []byte(strings.Join([]string{
				`{"type":"session","id":"pi-session-production"}`,
				`{"type":"message_end","message":{"role":"assistant","content":[],"stopReason":"error","errorMessage":"rate limited"}}`,
			}, "\n") + "\n"),
		},
	})
	if failure.Failure == nil || failure.Failure.Message != "rate limited" {
		t.Fatalf("failure = %#v", failure)
	}
}

func TestAdapterClassifyFailureReturnsEmptyForSuccessfulInvocation(t *testing.T) {
	t.Parallel()

	failure := pi.NewAdapter().ClassifyFailure(context.Background(), adapter.FailureContext{})
	if failure.Failure != nil {
		t.Fatalf("failure = %#v, want empty result", failure)
	}
}

func TestAdapterClassifyFailureMapsExecutionOutcomes(t *testing.T) {
	t.Parallel()

	adapterInstance := pi.NewAdapter()
	cases := []struct {
		name    string
		input   adapter.FailureContext
		message string
	}{
		{
			name:    "canceled",
			input:   adapter.FailureContext{CommandError: context.Canceled},
			message: "Pi execution was canceled.",
		},
		{
			name:    "flush canceled",
			input:   adapter.FailureContext{FlushReason: adapter.FlushReasonCanceled},
			message: "Pi execution was canceled.",
		},
		{
			name:    "timeout",
			input:   adapter.FailureContext{CommandError: context.DeadlineExceeded},
			message: "Pi execution timed out.",
		},
		{
			name:    "exit 124",
			input:   adapter.FailureContext{CommandResult: workerprocess.CommandResult{ExitCode: 124}},
			message: "Pi execution timed out.",
		},
		{
			name:    "command failure",
			input:   adapter.FailureContext{CommandResult: workerprocess.CommandResult{ExitCode: 2}},
			message: "Pi invocation failed.",
		},
		{
			name:    "decode failure",
			input:   adapter.FailureContext{DecodeError: errors.New("decode failed")},
			message: "Pi did not produce a valid completed response.",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			failure := adapterInstance.ClassifyFailure(context.Background(), tc.input)
			if failure.Failure == nil || failure.Failure.Message != tc.message {
				t.Fatalf("failure = %#v, want message %q", failure, tc.message)
			}
		})
	}
}

func TestAdapterClassifyFailureMapsRetryFromStdout(t *testing.T) {
	t.Parallel()

	failure := pi.NewAdapter().ClassifyFailure(context.Background(), adapter.FailureContext{
		CommandResult: workerprocess.CommandResult{
			ExitCode: 1,
			Stdout: []byte(strings.Join([]string{
				`{"type":"session","id":"pi-session-retry"}`,
				`{"type":"auto_retry_start","attempt":2,"retryDelayMs":2000,"errorStatus":429}`,
			}, "\n") + "\n"),
		},
	})
	if failure.Failure == nil || !failure.Failure.Retry.Retryable || failure.Failure.Message != "Pi reported a retryable provider API failure." {
		t.Fatalf("failure = %#v", failure)
	}
	if failure.Failure.ProviderSession == nil || failure.Failure.ProviderSession.ID != "pi-session-retry" {
		t.Fatalf("provider session = %#v", failure.Failure.ProviderSession)
	}
}

func TestAdapterObserveStderrEmitsIgnoredDiagnostic(t *testing.T) {
	t.Parallel()

	decoder, err := pi.NewAdapter().NewDecoder(context.Background(), adapter.DecoderContext{DispatchID: "dispatch-stderr"})
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	result, err := decoder.Observe(context.Background(), adapter.Observation{
		Stream: adapter.OutputStreamStderr,
		Chunk:  []byte("private stderr must not escape"),
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "pi_stderr_ignored" {
		t.Fatalf("diagnostics = %#v", result.Diagnostics)
	}
	if strings.Contains(result.Diagnostics[0].Message, "private stderr must not escape") {
		t.Fatalf("stderr leaked into diagnostic: %#v", result.Diagnostics[0])
	}
}

func structuredPiOutput() string {
	return strings.Join([]string{
		`{"type":"session","id":"pi-session-production","version":3,"cwd":"/repo"}`,
		`{"type":"agent_start"}`,
		`{"type":"turn_start"}`,
		`{"type":"message_start","message":{"id":"msg-production","role":"assistant","content":[]}}`,
		`{"type":"message_update","message":{"id":"msg-production","role":"assistant","content":[]},"assistantMessageEvent":{"type":"text_delta","delta":"authoritative answer","contentIndex":0}}`,
		`{"type":"message_end","message":{"id":"msg-production","role":"assistant","content":[{"type":"text","text":"authoritative answer"}],"stopReason":"stop"}}`,
		`{"type":"turn_end","message":{"id":"msg-production","role":"assistant","content":[{"type":"text","text":"authoritative answer"}]},"toolResults":[]}`,
		`{"type":"agent_end","messages":[{"id":"msg-production","role":"assistant","content":[{"type":"text","text":"authoritative answer"}],"stopReason":"stop"}]}`,
	}, "\n") + "\n"
}

func publishedDraft(fragments []workerprovider.InferenceProgressFragment, kind responseevents.Kind, phase responseevents.Phase, itemID string) bool {
	for _, fragment := range fragments {
		draft, ok := fragment.CanonicalDraft.(responseevents.Draft)
		if !ok || draft.Kind != kind || draft.Phase != phase || draft.ItemID != itemID {
			continue
		}
		return true
	}
	return false
}

type recordingRunner struct {
	request workerprovider.CommandRequest
	result  workerprovider.CommandResult
	calls   int
	err     error
}

func (r *recordingRunner) Run(_ context.Context, req workerprovider.CommandRequest) (workerprovider.CommandResult, error) {
	r.calls++
	r.request = req
	return r.result, r.err
}

var _ interface {
	Run(context.Context, workerprovider.CommandRequest) (workerprovider.CommandResult, error)
} = (*recordingRunner)(nil)

func TestAdapterDecodeDoesNotLeakPrivatePromptIntoDiagnostics(t *testing.T) {
	t.Parallel()

	decoder, err := pi.NewAdapter().NewDecoder(context.Background(), adapter.DecoderContext{DispatchID: "dispatch-safe"})
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	result, err := decoder.Observe(context.Background(), adapter.Observation{
		Stream: adapter.OutputStreamStdout,
		Chunk:  []byte(`{"type":` + privateConformancePrompt + `,"token":"` + privateConformanceToken + `"}` + "\n"),
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	for _, diagnostic := range result.Diagnostics {
		if strings.Contains(diagnostic.Message, privateConformancePrompt) || strings.Contains(diagnostic.Message, privateConformanceToken) {
			t.Fatalf("diagnostic leaked private data: %#v", diagnostic)
		}
	}
}

func TestAdapterDecodeMalformedRecordContinues(t *testing.T) {
	t.Parallel()

	decoder, err := pi.NewAdapter().NewDecoder(context.Background(), adapter.DecoderContext{DispatchID: "dispatch-recover"})
	if err != nil {
		t.Fatalf("NewDecoder() error = %v", err)
	}
	result, err := decoder.Observe(context.Background(), adapter.Observation{
		Stream: adapter.OutputStreamStdout,
		Chunk:  []byte("not-json\n" + structuredPiOutput()),
	})
	if err != nil {
		t.Fatalf("Observe() error = %v", err)
	}
	if len(result.Diagnostics) == 0 {
		t.Fatal("expected malformed record diagnostic")
	}
	final, err := pi.NewAdapter().ParseFinal(context.Background(), adapter.FinalParseContext{
		CommandResult: workerprocess.CommandResult{Stdout: []byte(structuredPiOutput())},
	})
	if err != nil {
		t.Fatalf("ParseFinal() after malformed record = %v", err)
	}
	if final.Response.Content != "authoritative answer" {
		t.Fatalf("final content = %q", final.Response.Content)
	}
}

func TestAdapterSelectableInvocationFailureIsNormalized(t *testing.T) {
	t.Parallel()

	provider := workerprovider.NewScriptWrapProvider(
		workerprovider.WithProviderCommandRunner(&recordingRunner{
			result: workerprovider.CommandResult{ExitCode: 1, Stdout: []byte(`{"type":"message_end","message":{"role":"assistant","stopReason":"error","errorMessage":"provider unavailable"}}` + "\n")},
		}),
		workerprovider.WithInferenceProgressPublisher(func(workerprovider.InferenceProgressFragment) {}),
		workerprovider.WithResponseStreamExecutor(structured.NewExecutor()),
	)
	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-pi-failure"},
		ModelProvider: string(modelprovider.Pi),
		UserMessage:   "private prompt",
	})
	if err == nil {
		t.Fatal("expected normalized failure")
	}
	var providerErr *workerprovider.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Message != "provider unavailable" {
		t.Fatalf("provider error = %v", err)
	}
}
