package wire_test

import (
	"context"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecutionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution/wire"
)

func TestNewServiceConstructsInertCapability(t *testing.T) {
	t.Parallel()

	stub := &executionSpy{}
	service, err := durableexecutionwire.NewService(stub)
	if err != nil || service == nil {
		t.Fatalf("NewService = (%v, %v)", service, err)
	}
	if stub.calls != 0 {
		t.Fatalf("construction invoked execution %d times, want 0", stub.calls)
	}
}

func TestNewServiceRejectsMissingExecutionCollaborator(t *testing.T) {
	t.Parallel()

	service, err := durableexecutionwire.NewService(nil)
	if err == nil || service != nil {
		t.Fatalf("NewService(nil) = (%v, %v), want deterministic dependency error", service, err)
	}
}

func TestNewServiceDefersRuntimeEffectsUntilInvocation(t *testing.T) {
	t.Parallel()

	stub := &executionSpy{}
	service, err := durableexecutionwire.NewService(stub)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	if _, err := service.StartAsync(context.Background(), factorysessions.StartRequest{RequestID: "req-1"}); err != nil {
		t.Fatalf("StartAsync: %v", err)
	}
	if _, err := service.GetSession(context.Background(), "sess-1"); err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if stub.calls != 2 {
		t.Fatalf("runtime calls = %d, want 2 after start and inspect", stub.calls)
	}
}

type executionSpy struct {
	factorysessions.ExecutionService
	calls int
}

func (s *executionSpy) StartAsync(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	s.calls++
	return factorysessions.AsyncStartResult{SessionID: "sess-1"}, nil
}

func (s *executionSpy) GetSession(context.Context, string) (factorysessions.SessionReadResult, error) {
	s.calls++
	return factorysessions.SessionReadResult{SessionID: "sess-1"}, nil
}
