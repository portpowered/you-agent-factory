package provider

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/interfaces/responseevents"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
)

func TestScriptWrapProviderExecuteAgyTimeoutWithPartialDoesNotReturnSuccessOrCompletedRun(t *testing.T) {
	t.Parallel()

	factoryRoot := t.TempDir()
	mock := &agyInferenceStubAllocator{result: agypty.SessionResult{
		ExitCode: 124, TimedOut: true, CleanedText: "partial answer before timeout",
	}}
	var published []InferenceProgressFragment
	provider := NewScriptWrapProvider(
		WithAgyFactoryRoot(factoryRoot),
		WithAgyPTYAllocator(mock),
		WithInferenceProgressPublisher(func(fragment InferenceProgressFragment) {
			published = append(published, fragment)
		}),
	)
	_, err := provider.Execute(context.Background(), interfaces.RunnerExecutionRequest{
		Dispatch:         interfaces.WorkDispatch{DispatchID: "dispatch-agy-timeout"},
		ModelProvider:    string(interfaces.ModelProviderAgy),
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
	provider := NewScriptWrapProvider(
		WithAgyFactoryRoot(factoryRoot),
		WithAgyPTYAllocator(mock),
	)
	response, err := provider.Execute(context.Background(), interfaces.RunnerExecutionRequest{
		Dispatch:         interfaces.WorkDispatch{DispatchID: "dispatch-agy-cli"},
		ModelProvider:    string(interfaces.ModelProviderAgy),
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

func agyTimeoutPartialDraftPublished(published []InferenceProgressFragment) bool {
	for _, fragment := range published {
		draft, ok := fragment.CanonicalDraft.(responseevents.Draft)
		if !ok || draft.Kind != responseevents.KindMessage || draft.Phase != responseevents.PhaseCompleted {
			continue
		}
		var payload responseevents.MessagePayload
		if err := json.Unmarshal(draft.Payload, &payload); err != nil || !payload.Partial {
			continue
		}
		if len(payload.ContentBlocks) == 1 && payload.ContentBlocks[0].Text == "partial answer before timeout" {
			return true
		}
	}
	return false
}

type agyInferenceStubSession struct {
	launch agypty.ProcessLaunch
	result agypty.SessionResult
}

func (s *agyInferenceStubSession) Run(context.Context) (agypty.SessionResult, error) {
	return s.result, nil
}

func (s *agyInferenceStubSession) Close() error { return nil }

type agyInferenceStubAllocator struct {
	sessions []*agyInferenceStubSession
	result   agypty.SessionResult
}

func (a *agyInferenceStubAllocator) Allocate(_ context.Context, launch agypty.ProcessLaunch, _ agypty.SessionConfig) (agypty.PTYSession, error) {
	session := &agyInferenceStubSession{launch: launch, result: a.result}
	a.sessions = append(a.sessions, session)
	return session, nil
}
