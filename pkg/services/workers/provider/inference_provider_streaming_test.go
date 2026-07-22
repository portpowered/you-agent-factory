package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerdiagnostics "github.com/portpowered/infinite-you/pkg/services/workers/diagnostics"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	agyadapter "github.com/portpowered/infinite-you/pkg/services/workers/provider/agy"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	opencodeadapter "github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter/opencode"
	cursorpkg "github.com/portpowered/infinite-you/pkg/services/workers/provider/cursor"
)

func TestScriptWrapProvider_OpenCodeNegotiatedAdapterPublishesProductionStream(t *testing.T) {
	t.Parallel()
	privatePrompt := "private production prompt"
	stdout, err := os.ReadFile("adapter/opencode/testdata/structured-success.jsonl")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	runner := &recordingProviderExec{result: CommandResult{Stdout: stdout}}
	var published []InferenceProgressFragment
	provider := NewScriptWrapProviderWithDependencies(false, nil, runner, openCodeResolverForTest(t, opencodeadapter.ModeStructured), func(fragment InferenceProgressFragment) {
		published = append(published, fragment)
	}, nil, "", nil, nil)

	response, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderOpenCode), UserMessage: privatePrompt,
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-opencode-production"},
	})
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	if response.Content != "Hello world" || response.ProviderSession == nil || response.ProviderSession.ID != "ses_open_42" {
		t.Fatalf("response = %#v", response)
	}
	if runner.request.Command != "opencode" || !reflect.DeepEqual(runner.request.Args, []string{"run", "--format", "json", privatePrompt}) {
		t.Fatalf("production command = %#v", runner.request)
	}
	if len(published) < 2 || published[0].Metadata["selected_mode"] != "structured" || published[0].Metadata["fidelity"] != "normalized" {
		t.Fatalf("capability publication = %#v", published)
	}
	assertPublishedOpenCodeDraft(t, published, factorysessions.ResponseEventKindMessage, factorysessions.ResponseEventPhaseCompleted)
	assertPublishedOpenCodeDraft(t, published, factorysessions.ResponseEventKindTool, factorysessions.ResponseEventPhaseCompleted)
	assertPublishedOpenCodeDraft(t, published, factorysessions.ResponseEventKindUsage, factorysessions.ResponseEventPhaseUpdated)
	for _, fragment := range published {
		if strings.Contains(fragment.Payload, "private prompt") || strings.Contains(fragment.Payload, "PRIVATE.md") || strings.Contains(fragment.Payload, "private result") {
			t.Fatalf("published sensitive provider data: %#v", fragment)
		}
	}
}

func TestScriptWrapProvider_OpenCodeProductionProgressRunnerUsesCanonicalAdapterStream(t *testing.T) {
	t.Parallel()
	stdout, err := os.ReadFile("adapter/opencode/testdata/structured-success.jsonl")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	executable := writeProviderOutputFixture(t, filepath.Join(t.TempDir(), "opencode"), stdout, nil, 0)
	var rawPublished []InferenceProgressFragment
	progressRunner := NewInferenceProgressPublishingCommandRunnerWithRunner(testProviderExecRunner(t), func(fragment InferenceProgressFragment) {
		rawPublished = append(rawPublished, fragment)
	}, nil)
	var canonicalPublished []InferenceProgressFragment
	provider := NewScriptWrapProviderWithDependencies(false, nil, progressRunner, openCodeResolverForExecutable(t, opencodeadapter.ModeStructured, executable), func(fragment InferenceProgressFragment) {
		canonicalPublished = append(canonicalPublished, fragment)
	}, nil, "", nil, nil)

	response, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderOpenCode), UserMessage: "private production prompt",
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-opencode-production-runner"},
	})
	if err != nil {
		t.Fatalf("Infer() error = %v", err)
	}
	if response.Content != "Hello world" {
		t.Fatalf("response = %#v", response)
	}
	if len(rawPublished) != 0 {
		t.Fatalf("legacy publisher received raw OpenCode output: %#v", rawPublished)
	}
	assertPublishedOpenCodeDraft(t, canonicalPublished, factorysessions.ResponseEventKindMessage, factorysessions.ResponseEventPhaseCompleted)
}

func TestScriptWrapProvider_OpenCodePublishesSafeProductionFallback(t *testing.T) {
	t.Parallel()
	privatePrompt := "private fallback prompt"
	rejection := "private rejection: unknown option '--format'"
	runner := &sequenceProviderRunner{results: []CommandResult{
		{Stderr: []byte(rejection), ExitCode: 2},
		{Stdout: []byte("fallback answer")},
	}}
	var published []InferenceProgressFragment
	provider := NewScriptWrapProviderWithDependencies(false, nil, runner, openCodeResolverForTest(t, opencodeadapter.ModeStructured), func(fragment InferenceProgressFragment) { published = append(published, fragment) }, nil, "", nil, nil)

	response, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderOpenCode), UserMessage: privatePrompt,
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-opencode-fallback"},
	})
	if err != nil || response.Content != "fallback answer" {
		t.Fatalf("Infer() = %#v, %v", response, err)
	}
	if len(runner.requests) != 2 ||
		!reflect.DeepEqual(runner.requests[0].Args, []string{"run", "--format", "json", privatePrompt}) ||
		!reflect.DeepEqual(runner.requests[1].Args, []string{"run", privatePrompt}) {
		t.Fatalf("fallback requests = %#v", runner.requests)
	}
	var degraded *InferenceProgressFragment
	for index := range published {
		if published[index].Metadata["selected_mode"] == "final_only" && published[index].Metadata["downgrade_reason"] == "unsupported_format" {
			degraded = &published[index]
		}
	}
	if degraded == nil || !strings.Contains(degraded.Payload, "structured_mode_degraded") && degraded.ExternalEventType != "structured_mode_degraded" {
		t.Fatalf("degradation publication = %#v", published)
	}
	if strings.Contains(degraded.Payload, rejection) || strings.Contains(degraded.Payload, privatePrompt) {
		t.Fatalf("degradation exposed private input: %#v", degraded)
	}
}

func TestScriptWrapProvider_OpenCodeRejectsUnsupportedRequiredCapabilitiesBeforeExecution(t *testing.T) {
	t.Parallel()
	for _, capability := range []workerexecution.RunnerOptionalCapability{
		workerexecution.RunnerOptionalCapabilityImageInput,
		workerexecution.RunnerOptionalCapabilityWorktree,
	} {
		t.Run(string(capability), func(t *testing.T) {
			runner := &recordingProviderExec{result: CommandResult{Stdout: []byte("must not execute")}}
			provider := NewScriptWrapProviderWithDependencies(false, nil, runner, openCodeResolverForTest(t, opencodeadapter.ModeStructured), nil, nil, "", nil, nil)

			_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
				ModelProvider: string(modelprovider.ProviderOpenCode), UserMessage: "private prompt",
				RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{capability},
			})
			assertOpenCodePermanentBadRequest(t, err)
			if runner.calls != 0 {
				t.Fatalf("runner calls = %d, want 0", runner.calls)
			}
		})
	}
}

func TestScriptWrapProvider_OpenCodeRejectsRequiredStructuredOutputWhenKnownFinalOnly(t *testing.T) {
	t.Parallel()
	runner := &recordingProviderExec{result: CommandResult{Stdout: []byte("must not execute")}}
	var published []InferenceProgressFragment
	provider := NewScriptWrapProviderWithDependencies(false, nil, runner, openCodeResolverForTest(t, opencodeadapter.ModeFinalOnly), func(fragment InferenceProgressFragment) { published = append(published, fragment) }, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderOpenCode), UserMessage: "private prompt",
		Dispatch:                     work.WorkDispatch{DispatchID: "dispatch-required-final-only"},
		RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{workerexecution.RunnerOptionalCapabilityStructuredOutput},
	})
	assertOpenCodePermanentBadRequest(t, err)
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
	if len(published) < 2 || published[0].Metadata["selected_mode"] != "final_only" || published[len(published)-1].Kind != FailedFragmentKind {
		t.Fatalf("published capability and failure = %#v", published)
	}
}

func TestScriptWrapProvider_OpenCodeRequiredStructuredOutputProhibitsStaleFallback(t *testing.T) {
	t.Parallel()
	resolver := openCodeResolverForTest(t, opencodeadapter.ModeStructured)
	runner := &sequenceProviderRunner{results: []CommandResult{{
		Stderr: []byte("unknown option '--format'"), ExitCode: 2,
	}}}
	provider := NewScriptWrapProviderWithDependencies(false, nil, runner, resolver, nil, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderOpenCode), UserMessage: "private prompt",
		RequiredOptionalCapabilities: []workerexecution.RunnerOptionalCapability{workerexecution.RunnerOptionalCapabilityStructuredOutput},
	})
	assertOpenCodePermanentBadRequest(t, err)
	if len(runner.requests) != 1 || !reflect.DeepEqual(runner.requests[0].Args, []string{"run", "--format", "json", "private prompt"}) {
		t.Fatalf("runner requests = %#v, want one structured attempt", runner.requests)
	}
	decision, resolveErr := resolver.Resolve(context.Background(), string(modelprovider.ProviderOpenCode))
	if resolveErr != nil || decision.Mode != opencodeadapter.ModeStructured {
		t.Fatalf("cached decision = %#v, %v; required stream must not downgrade", decision, resolveErr)
	}
}

func assertOpenCodePermanentBadRequest(t *testing.T, err error) {
	t.Helper()
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Type != workerexecution.WorkFailureTypePermanentBadRequest {
		t.Fatalf("error = %T %v, want permanent bad request", err, err)
	}
}

type sequenceProviderRunner struct {
	results  []CommandResult
	requests []CommandRequest
}

func (r *sequenceProviderRunner) Run(_ context.Context, request CommandRequest) (CommandResult, error) {
	r.requests = append(r.requests, request)
	if len(r.results) == 0 {
		return CommandResult{}, errors.New("unexpected provider invocation")
	}
	result := r.results[0]
	r.results = r.results[1:]
	return result, nil
}

func assertPublishedOpenCodeDraft(t *testing.T, fragments []InferenceProgressFragment, kind factorysessions.ResponseEventKind, phase factorysessions.ResponseEventPhase) {
	t.Helper()
	for _, fragment := range fragments {
		draft, ok := fragment.CanonicalDraft.(factorysessions.ResponseEventDraft)
		if ok && draft.Kind == kind && draft.Phase == phase {
			return
		}
	}
	t.Fatalf("missing canonical draft %s/%s: %#v", kind, phase, fragments)
}

func TestScriptWrapProvider_CursorDiagnosticsUseInjectedDispatchLogger(t *testing.T) {
	t.Parallel()
	var injectedOutput bytes.Buffer
	var unrelatedOutput bytes.Buffer
	encoderConfig := zap.NewProductionEncoderConfig()
	newCore := func(output *bytes.Buffer) zapcore.Core {
		return zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), zapcore.AddSync(output), zapcore.DebugLevel)
	}
	unrelatedUndo := zap.ReplaceGlobals(zap.New(newCore(&unrelatedOutput)))
	t.Cleanup(unrelatedUndo)

	base := zap.New(newCore(&injectedOutput)).With(
		zap.String("runtime_instance_id", "runtime-cursor-1"),
		zap.String("session_id", "factory-session-cursor-1"),
	)
	request := workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCursor), Model: "cursor-model", WorkerType: "agent",
		Dispatch: work.WorkDispatch{DispatchID: "dispatch-cursor-1", WorkerType: "agent", WorkstationName: "implementation"},
	}

	for _, tc := range []struct {
		name      string
		result    CommandResult
		wantError bool
		wantID    string
	}{
		{name: "success", result: CommandResult{Stdout: []byte(`{"type":"result","subtype":"success","is_error":false,"result":"done","session_id":"cursor-success-session"}` + "\n")}, wantID: "cursor-success-session"},
		{name: "failure", result: CommandResult{ExitCode: 1, Stderr: []byte("private cursor failure output"), Stdout: []byte(`{"type":"result","subtype":"timeout","is_error":true,"result":"Request timed out","session_id":"cursor-failure-session"}` + "\n")}, wantError: true, wantID: "cursor-failure-session"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			injectedOutput.Reset()
			unrelatedOutput.Reset()
			provider := NewScriptWrapProviderWithDependencies(false, logging.NewZapLogger(base, false), &recordingProviderExec{result: tc.result}, nil, nil, nil, "", nil, nil)
			_, err := provider.Infer(context.Background(), request)
			if (err != nil) != tc.wantError {
				t.Fatalf("Infer error = %v, wantError %v", err, tc.wantError)
			}
			if unrelatedOutput.Len() != 0 {
				t.Fatalf("Cursor diagnostics leaked to unrelated global sink: %s", unrelatedOutput.String())
			}
			record := cursorTerminalLogRecord(t, injectedOutput.String(), tc.wantError)
			for key, want := range map[string]any{"runtime_instance_id": "runtime-cursor-1", "session_id": "factory-session-cursor-1", "dispatch_id": "dispatch-cursor-1", "provider": "cursor", "provider_session_id": tc.wantID} {
				if record[key] != want {
					t.Fatalf("%s = %#v, want %#v; record = %#v", key, record[key], want, record)
				}
			}
			if strings.Contains(injectedOutput.String(), "private cursor failure output") {
				t.Fatalf("Cursor diagnostics leaked command output: %s", injectedOutput.String())
			}
		})
	}
}

func cursorTerminalLogRecord(t *testing.T, logs string, failure bool) map[string]any {
	t.Helper()
	wantMessage := "inferencer: request completed"
	if failure {
		wantMessage = "provider failure normalized"
	}
	for _, line := range strings.Split(strings.TrimSpace(logs), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("decode Cursor diagnostic: %v", err)
		}
		if record["msg"] == wantMessage {
			return record
		}
	}
	t.Fatalf("terminal Cursor diagnostic %q absent: %s", wantMessage, logs)
	return nil
}

func TestScriptWrapProvider_Infer_CursorErrorFlaggedSuccessPublishesOnlyCanonicalFailure(t *testing.T) {
	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, string(modelprovider.ProviderCursor))
	writeProviderOutputFixture(t, scriptPath, []byte("{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":true,\"result\":\"Request timed out\",\"session_id\":\"cursor-session-error\"}\n"), nil, 0)
	t.Setenv("PATH", scriptDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	var publishedMu sync.Mutex
	var published []InferenceProgressFragment
	publish := func(fragment InferenceProgressFragment) {
		publishedMu.Lock()
		published = append(published, fragment)
		publishedMu.Unlock()
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil, NewInferenceProgressPublishingCommandRunnerWithRunner(testProviderExecRunner(t), publish, nil), nil, publish, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch:           work.WorkDispatch{DispatchID: "dispatch-cursor-error-flagged-success"},
		ModelProvider:      string(modelprovider.ProviderCursor),
		UserMessage:        "private prompt",
		ProcessEnvironment: os.Environ(),
	})
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("error = %T, want *ProviderError", err)
	}
	if providerErr.Type != workerexecution.WorkFailureTypeTimeout || providerErr.Message != "Request timed out" {
		t.Fatalf("provider error = %#v, want canonical timeout", providerErr)
	}

	publishedMu.Lock()
	defer publishedMu.Unlock()
	var failed *InferenceProgressFragment
	for i := range published {
		if published[i].Kind == ResponseFragmentKind {
			t.Fatalf("published fragments = %#v, error result must not emit a response", published)
		}
		if published[i].Kind == FailedFragmentKind {
			failed = &published[i]
		}
	}
	if failed == nil || failed.Payload != providerErr.Message {
		t.Fatalf("published fragments = %#v, want canonical failed marker", published)
	}
	if failed.ProviderSessionRef == nil || failed.ProviderSessionRef.ID != "cursor-session-error" {
		t.Fatalf("failed provider session = %#v, want cursor-session-error", failed.ProviderSessionRef)
	}
}

func TestScriptWrapProvider_Infer_CursorZeroExitTerminalFailureCarriesCanonicalResultOnce(t *testing.T) {
	t.Parallel()
	stdout := []byte(
		"{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"cursor-initial-session\"}\n" +
			"{\"type\":\"result\",\"subtype\":\"timeout\",\"is_error\":true,\"result\":\"Cursor terminal request timed out\",\"session_id\":\"cursor-final-session\"}\n",
	)
	var published []InferenceProgressFragment
	provider := NewScriptWrapProviderWithDependencies(false, nil, &recordingProviderExec{result: CommandResult{
		Stdout: stdout,
		Stderr: []byte("unrelated authentication failed"),
	}}, nil, func(fragment InferenceProgressFragment) {
		published = append(published, fragment)
	}, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-cursor-zero-exit-failure"},
		ModelProvider: string(modelprovider.ProviderCursor),
		UserMessage:   "private prompt",
	})
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("error = %T, want *ProviderError", err)
	}
	if providerErr.Type != workerexecution.WorkFailureTypeTimeout || providerErr.Message != "Cursor terminal request timed out" {
		t.Fatalf("provider error = %#v, want canonical terminal timeout", providerErr)
	}
	if providerErr.ProviderSession == nil || providerErr.ProviderSession.ID != "cursor-final-session" {
		t.Fatalf("provider session = %#v, want cursor-final-session", providerErr.ProviderSession)
	}
	if len(published) != 1 || published[0].Kind != FailedFragmentKind || published[0].Payload != providerErr.Message {
		t.Fatalf("published fragments = %#v, want one canonical failed marker", published)
	}
	if published[0].ProviderSessionRef == nil || published[0].ProviderSessionRef.ID != providerErr.ProviderSession.ID {
		t.Fatalf("published provider session = %#v, want final provider error session %#v", published[0].ProviderSessionRef, providerErr.ProviderSession)
	}
}

func TestScriptWrapProvider_Infer_CursorMalformedStructuredOutputDoesNotPublishPromptText(t *testing.T) {
	t.Parallel()
	privatePrompt := "deploy production using the customer launch phrase"
	stdout := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"` + privatePrompt + `"}]}`)
	var published []InferenceProgressFragment
	provider := NewScriptWrapProviderWithDependencies(false, nil, &recordingProviderExec{result: CommandResult{Stdout: stdout, ExitCode: 1}}, nil, func(fragment InferenceProgressFragment) {
		published = append(published, fragment)
	}, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-cursor-malformed-structured"},
		ModelProvider: string(modelprovider.ProviderCursor),
		UserMessage:   privatePrompt,
	})
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("error = %T, want *ProviderError", err)
	}
	if strings.Contains(providerErr.Message, privatePrompt) {
		t.Fatalf("provider message = %q, must not surface malformed assistant content", providerErr.Message)
	}
	if len(published) != 1 || published[0].Kind != FailedFragmentKind || published[0].Payload != providerErr.Message {
		t.Fatalf("published fragments = %#v, want one canonical failure marker", published)
	}
	if strings.Contains(published[0].Payload, privatePrompt) {
		t.Fatalf("failure fragment = %q, must not surface malformed assistant content", published[0].Payload)
	}
}

func TestScriptWrapProvider_Infer_CursorParsesStreamJSONResult(t *testing.T) {
	t.Parallel()
	stdout := []byte(
		"{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"cursor-session-abc\"}\n" +
			"{\"type\":\"assistant\",\"timestamp_ms\":1,\"message\":{\"role\":\"assistant\",\"content\":[{\"type\":\"text\",\"text\":\"Parsed \"}]},\"session_id\":\"cursor-session-abc\"}\n" +
			"{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"Parsed assistant answer.\",\"session_id\":\"cursor-session-abc\"}\n",
	)
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: stdout},
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

	resp, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCursor),
		Model:         "gpt-5",
		UserMessage:   "run the tests",
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if resp.Content != "Parsed assistant answer." {
		t.Fatalf("content = %q, want parsed result text", resp.Content)
	}
	if resp.Content == string(stdout) {
		t.Fatal("content must not be raw JSON stdout")
	}
	if resp.ProviderSession == nil {
		t.Fatal("expected provider session metadata")
	}
	if resp.ProviderSession.Provider != "cursor" {
		t.Fatalf("provider = %q, want cursor", resp.ProviderSession.Provider)
	}
	if resp.ProviderSession.ID != "cursor-session-abc" {
		t.Fatalf("session id = %q, want cursor-session-abc", resp.ProviderSession.ID)
	}
	if resp.Diagnostics == nil || resp.Diagnostics.Command == nil {
		t.Fatal("expected command diagnostics on success")
	}
	if string(resp.Diagnostics.Command.Stdout) != string(stdout) {
		t.Fatal("command diagnostics should retain raw stdout for observability")
	}
}

func TestScriptWrapProvider_Infer_CursorPublishesTerminalCompletionMarker(t *testing.T) {
	t.Parallel()
	stdout := cursorpkg.SuccessStdoutJSON("Parsed assistant answer.", "cursor-session-abc")
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: stdout},
	}
	var published []InferenceProgressFragment
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, func(fragment InferenceProgressFragment) {
		published = append(published, fragment)
	}, nil, "", nil, nil)

	resp, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-cursor-success"},
		ModelProvider: string(modelprovider.ProviderCursor),
		Model:         "gpt-5",
		UserMessage:   "run the tests",
	})
	if err != nil {
		t.Fatalf("Infer returned error: %v", err)
	}
	if resp.ProviderSession == nil {
		t.Fatal("expected provider session metadata")
	}
	if len(published) != 1 {
		t.Fatalf("published fragments = %#v, want one completion marker", published)
	}
	if published[0].Kind != CompletedFragmentKind {
		t.Fatalf("published kind = %q, want %q", published[0].Kind, CompletedFragmentKind)
	}
	if published[0].DispatchID != "dispatch-cursor-success" {
		t.Fatalf("dispatch id = %q, want dispatch-cursor-success", published[0].DispatchID)
	}
	if published[0].ProviderSessionRef == nil || published[0].ProviderSessionRef.ID != "cursor-session-abc" {
		t.Fatalf("provider session ref = %#v, want cursor-session-abc", published[0].ProviderSessionRef)
	}
}

func TestScriptWrapProvider_Infer_CursorCompletionPublisherPreservesFinalResponse(t *testing.T) {
	t.Parallel()
	stdout := cursorpkg.SuccessStdoutJSON("Parsed assistant answer.", "cursor-session-abc")
	req := workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-cursor-success"},
		ModelProvider: string(modelprovider.ProviderCursor),
		Model:         "gpt-5",
		UserMessage:   "run the tests",
	}

	withoutPublisher := NewScriptWrapProviderWithDependencies(false, nil, &recordingProviderExec{
		result: CommandResult{Stdout: stdout},
	}, nil, nil, nil, "", nil, nil)

	want, err := withoutPublisher.Infer(context.Background(), req)
	if err != nil {
		t.Fatalf("Infer without publisher returned error: %v", err)
	}

	var published []InferenceProgressFragment
	withPublisher := NewScriptWrapProviderWithDependencies(false, nil, &recordingProviderExec{
		result: CommandResult{Stdout: stdout},
	}, nil, func(fragment InferenceProgressFragment) {
		published = append(published, fragment)
	}, nil, "", nil, nil)

	got, err := withPublisher.Infer(context.Background(), req)
	if err != nil {
		t.Fatalf("Infer with publisher returned error: %v", err)
	}

	assertEquivalentInferenceResponse(t, got, want)
	if len(published) != 1 || published[0].Kind != CompletedFragmentKind {
		t.Fatalf("published fragments = %#v, want one completion marker", published)
	}
}

func TestScriptWrapProvider_Infer_CursorMalformedJSONReturnsProviderError(t *testing.T) {
	t.Parallel()
	privateMalformedContent := "private malformed Cursor content"
	stdout := []byte(`{"type":"assistant","message":"` + privateMalformedContent)
	stderr := []byte("cursor stderr detail")
	fakeExec := &recordingProviderExec{
		result: CommandResult{Stdout: stdout, Stderr: stderr},
	}
	var published []InferenceProgressFragment
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, func(fragment InferenceProgressFragment) {
		published = append(published, fragment)
	}, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-cursor-malformed-json"},
		ModelProvider: string(modelprovider.ProviderCursor),
		UserMessage:   "run the tests",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Type != workerexecution.WorkFailureTypeUnknown {
		t.Fatalf("error type = %q, want unknown", providerErr.Type)
	}
	if providerErr.Message != string(stderr) || strings.Contains(providerErr.Message, privateMalformedContent) {
		t.Fatalf("provider message = %q, want safe unknown stderr result", providerErr.Message)
	}
	if providerErr.Cause == nil {
		t.Fatal("parse failure cause = nil, want original JSON parse cause")
	}
	if len(published) != 1 || published[0].Kind != FailedFragmentKind {
		t.Fatalf("published fragments = %#v, want one failed marker", published)
	}
	if published[0].Payload != providerErr.Message || strings.Contains(published[0].Payload, privateMalformedContent) {
		t.Fatalf("failure fragment = %#v, want canonical safe unknown result", published[0])
	}
	if providerErr.Diagnostics == nil || providerErr.Diagnostics.Command == nil {
		t.Fatal("expected command diagnostics on parse failure")
	}
	if got := providerErr.Diagnostics.Command.Stdout; got != string(stdout) {
		t.Fatalf("command stdout = %q, want full stdout for worker-internal diagnostics", got)
	}
	if got := providerErr.Diagnostics.Command.Stderr; got != string(stderr) {
		t.Fatalf("command stderr = %q, want full stderr for worker-internal diagnostics", got)
	}
	assertCursorFailureExcerpts(t, providerErr.Diagnostics, string(stdout), string(stderr))
	assertSafeCursorFailureExcerpts(t, providerErr.Diagnostics)
}

func TestScriptWrapProvider_Infer_CursorParseFailureUsesStderrParserResult(t *testing.T) {
	t.Parallel()
	stdout := []byte(`{"type":"result"`)
	stderr := []byte("Cursor authentication failed; sign in again")
	provider := NewScriptWrapProviderWithDependencies(false, nil, &recordingProviderExec{
		result: CommandResult{Stdout: stdout, Stderr: stderr},
	}, nil, nil, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCursor),
		UserMessage:   "private prompt",
	})
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("error = %T, want *ProviderError", err)
	}
	if providerErr.Type != workerexecution.WorkFailureTypeAuthFailure || providerErr.Message != string(stderr) {
		t.Fatalf("provider error = %#v, want canonical stderr authentication result", providerErr)
	}
	if providerErr.Cause == nil {
		t.Fatal("parse failure cause = nil, want original JSON parse cause")
	}
}

func TestScriptWrapProvider_Infer_CursorExitFailurePreservesBoundedDiagnosticsExcerpts(t *testing.T) {
	t.Parallel()
	stdout := []byte("partial json output")
	stderr := []byte("noise before\nERROR: unexpected status 500 from cursor upstream")
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: 1,
		},
	}
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, nil, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		ModelProvider: string(modelprovider.ProviderCursor),
		UserMessage:   "run the tests",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}
	providerErr, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError, got %T", err)
	}
	if providerErr.Type != workerexecution.WorkFailureTypeInternalServerError {
		t.Fatalf("error type = %q, want internal_server_error", providerErr.Type)
	}
	if providerErr.Message != "ERROR: unexpected status 500 from cursor upstream" {
		t.Fatalf("error message = %q", providerErr.Message)
	}
	assertCursorFailureExcerpts(t, providerErr.Diagnostics, string(stdout), string(stderr))
	assertSafeCursorFailureExcerpts(t, providerErr.Diagnostics)
}

func TestScriptWrapProvider_Infer_CursorExitFailurePublishesTerminalFailureMarker(t *testing.T) {
	t.Parallel()
	stdout := []byte("partial json output")
	stderr := []byte("noise before\nERROR: unexpected status 500 from cursor upstream")
	fakeExec := &recordingProviderExec{
		result: CommandResult{
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: 1,
		},
	}
	var published []InferenceProgressFragment
	provider := NewScriptWrapProviderWithDependencies(false, nil, fakeExec, nil, func(fragment InferenceProgressFragment) {
		published = append(published, fragment)
	}, nil, "", nil, nil)

	_, err := provider.Infer(context.Background(), workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-cursor-failure"},
		ModelProvider: string(modelprovider.ProviderCursor),
		UserMessage:   "run the tests",
	})
	if err == nil {
		t.Fatal("expected Infer to fail")
	}
	if len(published) != 1 {
		t.Fatalf("published fragments = %#v, want one failure marker", published)
	}
	if published[0].Kind != FailedFragmentKind {
		t.Fatalf("published kind = %q, want %q", published[0].Kind, FailedFragmentKind)
	}
	if published[0].DispatchID != "dispatch-cursor-failure" {
		t.Fatalf("dispatch id = %q, want dispatch-cursor-failure", published[0].DispatchID)
	}
	if published[0].Payload != "ERROR: unexpected status 500 from cursor upstream" {
		t.Fatalf("failure payload = %q, want normalized provider error message", published[0].Payload)
	}
}

func TestScriptWrapProvider_Infer_CursorFailurePublisherPreservesProviderError(t *testing.T) {
	t.Parallel()
	stdout := []byte("partial json output")
	stderr := []byte("noise before\nERROR: unexpected status 500 from cursor upstream")
	req := workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-cursor-failure"},
		ModelProvider: string(modelprovider.ProviderCursor),
		UserMessage:   "run the tests",
	}

	withoutPublisher := NewScriptWrapProviderWithDependencies(false, nil, &recordingProviderExec{
		result: CommandResult{
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: 1,
		},
	}, nil, nil, nil, "", nil, nil)

	_, err := withoutPublisher.Infer(context.Background(), req)
	if err == nil {
		t.Fatal("expected Infer without publisher to fail")
	}
	want, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError without publisher, got %T", err)
	}

	var published []InferenceProgressFragment
	withPublisher := NewScriptWrapProviderWithDependencies(false, nil, &recordingProviderExec{
		result: CommandResult{
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: 1,
		},
	}, nil, func(fragment InferenceProgressFragment) {
		published = append(published, fragment)
	}, nil, "", nil, nil)

	_, err = withPublisher.Infer(context.Background(), req)
	if err == nil {
		t.Fatal("expected Infer with publisher to fail")
	}
	got, ok := err.(*ProviderError)
	if !ok {
		t.Fatalf("expected ProviderError with publisher, got %T", err)
	}

	assertEquivalentProviderError(t, got, want)
	if len(published) != 1 || published[0].Kind != FailedFragmentKind {
		t.Fatalf("published fragments = %#v, want one failure marker", published)
	}
}

func TestScriptWrapProvider_Infer_ClaudeCompletionPublisherPreservesFinalResponse(t *testing.T) {
	t.Parallel()
	req := workerexecution.ProviderInferenceRequest{
		Dispatch:      work.WorkDispatch{DispatchID: "dispatch-claude-success"},
		ModelProvider: string(modelprovider.ProviderClaude),
		Model:         "claude-sonnet-4-5-20250514",
		SessionID:     "claude-session-123",
		UserMessage:   "fix it",
	}

	withoutPublisher := NewScriptWrapProviderWithDependencies(false, nil, &recordingProviderExec{
		result: CommandResult{Stdout: []byte("claude output")},
	}, nil, nil, nil, "", nil, nil)

	want, err := withoutPublisher.Infer(context.Background(), req)
	if err != nil {
		t.Fatalf("Infer without publisher returned error: %v", err)
	}

	var published []InferenceProgressFragment
	withPublisher := NewScriptWrapProviderWithDependencies(false, nil, &recordingProviderExec{
		result: CommandResult{Stdout: []byte("claude output")},
	}, nil, func(fragment InferenceProgressFragment) {
		published = append(published, fragment)
	}, nil, "", nil, nil)

	got, err := withPublisher.Infer(context.Background(), req)
	if err != nil {
		t.Fatalf("Infer with publisher returned error: %v", err)
	}

	assertEquivalentInferenceResponse(t, got, want)
	if len(published) != 1 || published[0].Kind != CompletedFragmentKind {
		t.Fatalf("published fragments = %#v, want one completion marker", published)
	}
	if published[0].ProviderSessionRef == nil || published[0].ProviderSessionRef.ID != "claude-session-123" {
		t.Fatalf("provider session ref = %#v, want claude-session-123", published[0].ProviderSessionRef)
	}
}

func assertCursorFailureExcerpts(t *testing.T, diagnostics *workerexecution.WorkDiagnostics, wantStdout, wantStderr string) {
	t.Helper()
	if diagnostics == nil || diagnostics.Provider == nil {
		t.Fatal("expected provider diagnostics with failure excerpts")
	}
	metadata := diagnostics.Provider.ResponseMetadata
	if got := metadata[cursorpkg.ResponseMetadataStdoutExcerpt]; got != wantStdout {
		t.Fatalf("stdout excerpt = %q, want %q", got, wantStdout)
	}
	if got := metadata[cursorpkg.ResponseMetadataStderrExcerpt]; got != wantStderr {
		t.Fatalf("stderr excerpt = %q, want %q", got, wantStderr)
	}
}

func assertSafeCursorFailureExcerpts(t *testing.T, diagnostics *workerexecution.WorkDiagnostics) {
	t.Helper()
	safe := workerdiagnostics.SafeWorkDiagnosticsFromWorkDiagnostics(diagnostics)
	if safe == nil || safe.Provider == nil {
		t.Fatal("expected safe provider diagnostics")
	}
	if safe.Provider.ResponseMetadata[cursorpkg.ResponseMetadataStdoutExcerpt] == "" {
		t.Fatal("expected safe stdout excerpt")
	}
	if safe.Provider.ResponseMetadata[cursorpkg.ResponseMetadataStderrExcerpt] == "" {
		t.Fatal("expected safe stderr excerpt")
	}
	if safe.Provider.ResponseMetadata["raw_body"] != "" {
		t.Fatal("safe diagnostics must not include unsafe metadata keys")
	}
}

func assertEquivalentInferenceResponse(t *testing.T, got, want workerexecution.InferenceResponse) {
	t.Helper()
	if got.Content != want.Content {
		t.Fatalf("content = %q, want %q", got.Content, want.Content)
	}
	if !reflect.DeepEqual(got.ProviderSession, want.ProviderSession) {
		t.Fatalf("provider session = %#v, want %#v", got.ProviderSession, want.ProviderSession)
	}
	assertEquivalentWorkDiagnostics(t, got.Diagnostics, want.Diagnostics)
}

func assertEquivalentProviderError(t *testing.T, got, want *ProviderError) {
	t.Helper()
	if got.Type != want.Type {
		t.Fatalf("error type = %q, want %q", got.Type, want.Type)
	}
	if got.Message != want.Message {
		t.Fatalf("error message = %q, want %q", got.Message, want.Message)
	}
	if !reflect.DeepEqual(got.ProviderSession, want.ProviderSession) {
		t.Fatalf("provider session = %#v, want %#v", got.ProviderSession, want.ProviderSession)
	}
	assertEquivalentWorkDiagnostics(t, got.Diagnostics, want.Diagnostics)
}

func assertEquivalentWorkDiagnostics(t *testing.T, got, want *workerexecution.WorkDiagnostics) {
	t.Helper()
	if (got == nil) != (want == nil) {
		t.Fatalf("diagnostics presence = %#v, want %#v", got, want)
	}
	if got == nil {
		return
	}
	if !reflect.DeepEqual(got.Provider, want.Provider) {
		t.Fatalf("provider diagnostics = %#v, want %#v", got.Provider, want.Provider)
	}
	if !reflect.DeepEqual(got.Metadata, want.Metadata) {
		t.Fatalf("diagnostics metadata = %#v, want %#v", got.Metadata, want.Metadata)
	}
	if !reflect.DeepEqual(got.RenderedPrompt, want.RenderedPrompt) {
		t.Fatalf("rendered prompt diagnostics = %#v, want %#v", got.RenderedPrompt, want.RenderedPrompt)
	}
	if !reflect.DeepEqual(got.Panic, want.Panic) {
		t.Fatalf("panic diagnostics = %#v, want %#v", got.Panic, want.Panic)
	}
	assertEquivalentCommandDiagnostic(t, got.Command, want.Command)
}

func assertEquivalentCommandDiagnostic(t *testing.T, got, want *workerexecution.CommandDiagnostic) {
	t.Helper()
	if (got == nil) != (want == nil) {
		t.Fatalf("command diagnostics presence = %#v, want %#v", got, want)
	}
	if got == nil {
		return
	}
	if got.Command != want.Command ||
		!reflect.DeepEqual(got.Args, want.Args) ||
		got.Stdin != want.Stdin ||
		!reflect.DeepEqual(got.Env, want.Env) ||
		got.Stdout != want.Stdout ||
		got.Stderr != want.Stderr ||
		got.ExitCode != want.ExitCode ||
		got.TimedOut != want.TimedOut ||
		got.WorkingDir != want.WorkingDir {
		t.Fatalf("command diagnostics = %#v, want %#v", got, want)
	}
}

func TestScriptWrapProviderExecuteAgyTimeoutWithPartialDoesNotReturnSuccessOrCompletedRun(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &agyInferenceStubAllocator{result: agypty.SessionResult{
		ExitCode: 124, TimedOut: true, CleanedText: "partial answer before timeout",
	}}
	var published []InferenceProgressFragment
	provider := NewScriptWrapProviderWithDependencies(false, nil, nil, nil, func(fragment InferenceProgressFragment) {
		published = append(published, fragment)
	}, nil, factoryRoot, mock, nil)
	provider.agyExecutableDependencies = missingAgyExecutableDependencies()

	_, err := provider.Execute(context.Background(), workerexecution.RunnerExecutionRequest{
		Dispatch:         work.WorkDispatch{DispatchID: "dispatch-agy-timeout"},
		ModelProvider:    string(modelprovider.ProviderAgy),
		WorkingDirectory: ".",
		UserMessage:      "plan the goal",
	})
	if err == nil {
		t.Fatal("Execute() error = nil, want timeout failure")
	}
	for _, fragment := range published {
		if fragment.Kind == CompletedFragmentKind {
			t.Fatalf("published completed fragment on timeout: %#v", published)
		}
		if fragment.Kind == FailedFragmentKind && !fragment.CanonicalEventAlreadyPublished {
			t.Fatalf("published duplicate legacy failure after canonical timeout drafts: %#v", published)
		}
	}
	if !agyTimeoutPartialDraftPublished(published) {
		t.Fatalf("published fragments = %#v, want partial timeout canonical draft", published)
	}
}

func TestScriptWrapProviderExecuteAgyUsesPTYAdapterPath(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &agyInferenceStubAllocator{result: agypty.SessionResult{ExitCode: 0, CleanedText: "final answer"}}
	provider := NewScriptWrapProviderWithDependencies(false, nil, nil, nil, nil, nil, factoryRoot, mock, nil)
	provider.agyExecutableDependencies = missingAgyExecutableDependencies()

	response, err := provider.Execute(context.Background(), workerexecution.RunnerExecutionRequest{
		Dispatch:         work.WorkDispatch{DispatchID: "dispatch-agy-cli"},
		ModelProvider:    string(modelprovider.ProviderAgy),
		WorkingDirectory: ".",
		UserMessage:      "plan the goal",
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if response.Content != "final answer" {
		t.Fatalf("content = %q, want final answer", response.Content)
	}
	if len(mock.sessions) != 1 {
		t.Fatalf("pty sessions = %d, want 1", len(mock.sessions))
	}
	if err := agypty.ValidateArgv(mock.sessions[0].launch.Argv); err != nil {
		t.Fatalf("ValidateArgv() error = %v", err)
	}
}

type missingAgyExecutableEffects struct{}

func (missingAgyExecutableEffects) LookPath(string) (string, error)  { return "", fs.ErrNotExist }
func (missingAgyExecutableEffects) Stat(string) (fs.FileInfo, error) { return nil, fs.ErrNotExist }

func missingAgyExecutableDependencies() agyadapter.ExecutableDependencies {
	effects := missingAgyExecutableEffects{}
	return agyadapter.ExecutableDependencies{Locator: effects, Inspector: effects}
}
