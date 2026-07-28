package provider

import (
	"context"
	"io/fs"
	"reflect"
	"testing"

	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	agyadapter "github.com/portpowered/infinite-you/pkg/services/workers/provider/agy"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers/agypty"
)

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
