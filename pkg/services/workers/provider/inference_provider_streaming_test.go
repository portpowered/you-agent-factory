package provider

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	agyadapter "github.com/portpowered/infinite-you/pkg/services/workers/provider/agy"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
	opencodeadapter "github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter/opencode"
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

func TestScriptWrapProvider_Infer_ClaudeCompletionPublisherPreservesFinalResponse(t *testing.T) {
	skipConductorRoutedNativeProviderTest(t)
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
