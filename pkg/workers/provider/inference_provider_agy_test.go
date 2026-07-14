package provider

import (
	"context"
	"testing"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
)

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
