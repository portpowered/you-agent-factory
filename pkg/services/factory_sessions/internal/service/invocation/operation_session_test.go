package invocation

import (
	"context"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/models"
	"go.uber.org/zap"
)

type recordingInvocationSessions struct {
	factorysessions.Service
	onStart func()
	start   factorysessions.SessionStartRequest
	invoke  factorysessions.SessionInvokeRequest
	control factorysessions.SessionControlRequest
}

func (s *recordingInvocationSessions) Start(_ context.Context, request factorysessions.SessionStartRequest) (factorysessions.SessionStartResult, error) {
	s.start = request
	if s.onStart != nil {
		s.onStart()
	}
	return factorysessions.SessionStartResult{SessionID: request.SessionID}, nil
}

func (*recordingInvocationSessions) GetFactorySession(context.Context, string) (factorysessions.SessionProjection, error) {
	return factorysessions.SessionProjection{}, factorysessions.ErrSessionNotFound
}

func (s *recordingInvocationSessions) Invoke(_ context.Context, request factorysessions.SessionInvokeRequest) (factorysessions.InvocationResult, error) {
	s.invoke = request
	return factorysessions.InvocationResult{Status: factorysessions.InvocationTerminalStatusCompleted}, nil
}

func (s *recordingInvocationSessions) Control(_ context.Context, request factorysessions.SessionControlRequest) (factorysessions.SessionControlResult, error) {
	s.control = request
	return factorysessions.SessionControlResult{Closed: true}, nil
}

func (*recordingInvocationSessions) InvokeModelForSession(context.Context, string, string, models.Request) (models.Result, error) {
	return models.Result{}, nil
}

func (*recordingInvocationSessions) ModelsScopeForSession(context.Context, string) (models.RuntimeScopeRef, error) {
	return models.RuntimeScopeRef{}, nil
}

func TestInvokeFactoryUsesCanonicalSessionCommands(t *testing.T) {
	sessions := &recordingInvocationSessions{}
	op := &operation{
		sessions:      sessions,
		artifactRoots: func(string) factoryruntime.RuntimeArtifactRoots { return factoryruntime.RuntimeArtifactRoots{} },
		presentations: invocationPresentationOwnerStub{},
		logger:        zap.NewNop(),
	}
	requestID := "request-1"
	timeoutMillis := int64(2500)
	target := roles.InvocationTarget{FactorySessionID: "session-1", FactoryDir: "factory", RunnerID: "opencode"}
	outcome, err := op.InvokeFactory(context.Background(), target, factorysessions.InvocationRequest{
		RequestID: &requestID, TimeoutMillis: &timeoutMillis,
	})
	if err != nil {
		t.Fatalf("InvokeFactory() error = %v", err)
	}
	if !sessions.start.ActivationOnly || sessions.start.SessionID != "session-1" || sessions.start.RuntimeSelection.Workers.RunnerID != "opencode" {
		t.Fatalf("Start request = %#v, want one activation-only session", sessions.start)
	}
	if sessions.invoke.SessionID != "session-1" || sessions.invoke.Correlation.RequestID != requestID || sessions.invoke.Wait.TimeoutMillis != timeoutMillis {
		t.Fatalf("Invoke request = %#v, want session-keyed invocation values", sessions.invoke)
	}
	if sessions.control.SessionID != "session-1" || sessions.control.Operation != factorysessions.SessionControlClose {
		t.Fatalf("Control request = %#v, want session close", sessions.control)
	}
	if outcome.Result.Status != factorydefinitions.InvocationTerminalStatusCompleted {
		t.Fatalf("outcome status = %q, want completed", outcome.Result.Status)
	}
}

func TestInvokeFactoryCallerSnapshotSurvivesOpeningMutation(t *testing.T) {
	t.Parallel()
	caller := &workersessions.CallerIdentity{WorkerSessionID: "exact-caller", Token: "planted-before-opening"}
	sessions := &recordingInvocationSessions{onStart: func() { caller.WorkerSessionID = "changed-caller"; caller.Token = "changed-token" }}
	op := &operation{sessions: sessions, artifactRoots: func(string) factoryruntime.RuntimeArtifactRoots { return factoryruntime.RuntimeArtifactRoots{} }, presentations: invocationPresentationOwnerStub{}, logger: zap.NewNop()}
	_, err := op.InvokeFactory(t.Context(), roles.InvocationTarget{FactorySessionID: "selected", FactoryDir: "/project"}, factorysessions.InvocationRequest{Caller: caller})
	if err != nil {
		t.Fatal(err)
	}
	got := sessions.invoke.Caller
	if got == nil || got == caller || got.WorkerSessionID != "exact-caller" || got.Token != "planted-before-opening" {
		t.Fatal("opening mutated invocation caller authority")
	}
	if sessions.control.SessionID != "selected" || sessions.control.Operation != factorysessions.SessionControlClose {
		t.Fatal("caller forwarding changed session cleanup")
	}
}
