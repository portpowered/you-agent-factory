package service

import (
	"context"
	"errors"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

func TestNewRequiresExecutionAndStaysInert(t *testing.T) {
	t.Parallel()

	if service, err := New(nil); err == nil || service != nil {
		t.Fatalf("New(nil) = (%#v, %v), want deterministic dependency error", service, err)
	}

	stub := &executionStub{}
	service, err := New(stub)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if service == nil {
		t.Fatal("New returned nil service")
	}
	if stub.calls != 0 {
		t.Fatalf("construction invoked execution %d times, want 0", stub.calls)
	}
}

func TestServiceDelegatesDurableExecutionContract(t *testing.T) {
	t.Parallel()

	stub := &executionStub{replay: true}
	service, err := New(stub)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	started, err := service.StartAsync(context.Background(), factorysessions.StartRequest{RequestID: "request-1"})
	if err != nil || started.SessionID != "dur-sess-1" {
		t.Fatalf("StartAsync = (%#v, %v)", started, err)
	}
	read, err := service.GetSession(context.Background(), started.SessionID)
	if err != nil || read.SessionID != started.SessionID {
		t.Fatalf("GetSession = (%#v, %v)", read, err)
	}
	result, err := service.GetResult(context.Background(), started.SessionID, factorysessions.ResultRequest{})
	if err != nil || result.ResultStatus != factorysessions.ResultStatusFinal {
		t.Fatalf("GetResult = (%#v, %v)", result, err)
	}
	control, err := service.Pause(context.Background(), started.SessionID, factorysessions.ControlRequest{})
	if err != nil || control.Outcome != factorysessions.LifecycleControlOutcomeAccepted {
		t.Fatalf("Pause = (%#v, %v)", control, err)
	}
	events, err := service.ReadEvents(context.Background(), started.SessionID, factorysessions.EventReconnectRequest{})
	if err != nil || events.SessionID != started.SessionID {
		t.Fatalf("ReadEvents = (%#v, %v)", events, err)
	}
	if !service.IsNonLiveReplay() {
		t.Fatal("IsNonLiveReplay = false, want forwarded replay capability")
	}
	if err := service.RecordPetriTokenMutations(started.SessionID, []interfaces.TokenMutationRecord{{}}); err != nil {
		t.Fatalf("RecordPetriTokenMutations: %v", err)
	}
	if stub.calls != 6 {
		t.Fatalf("delegated calls = %d, want 6", stub.calls)
	}
}

type executionStub struct {
	durableexecution.Service
	calls  int
	replay bool
}

func (s *executionStub) StartAsync(context.Context, factorysessions.StartRequest) (factorysessions.AsyncStartResult, error) {
	s.calls++
	return factorysessions.AsyncStartResult{SessionID: "dur-sess-1"}, nil
}

func (s *executionStub) GetSession(context.Context, string) (factorysessions.SessionReadResult, error) {
	s.calls++
	return factorysessions.SessionReadResult{SessionID: "dur-sess-1"}, nil
}

func (s *executionStub) GetResult(context.Context, string, factorysessions.ResultRequest) (factorysessions.ResultReadResult, error) {
	s.calls++
	return factorysessions.ResultReadResult{SessionID: "dur-sess-1", ResultStatus: factorysessions.ResultStatusFinal}, nil
}

func (s *executionStub) Pause(context.Context, string, factorysessions.ControlRequest) (factorysessions.LifecycleControlResult, error) {
	s.calls++
	return factorysessions.LifecycleControlResult{Outcome: factorysessions.LifecycleControlOutcomeAccepted}, nil
}

func (s *executionStub) ReadEvents(context.Context, string, factorysessions.EventReconnectRequest) (factorysessions.EventReadResult, error) {
	s.calls++
	return factorysessions.EventReadResult{SessionID: "dur-sess-1"}, nil
}

func (s *executionStub) IsNonLiveReplay() bool { return s.replay }

func (s *executionStub) RecordPetriTokenMutations(string, []interfaces.TokenMutationRecord) error {
	s.calls++
	return nil
}

func TestRecordPetriTokenMutationsRejectsUnsupportedExecution(t *testing.T) {
	t.Parallel()

	service, err := New(&executionWithoutMutation{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	err = service.RecordPetriTokenMutations("dur-sess-1", nil)
	if err == nil || errors.Is(err, factorysessions.ErrExecutionServiceNotConfigured) {
		t.Fatalf("RecordPetriTokenMutations error = %v, want unsupported-capability error", err)
	}
}

func TestStartSyncWithEventConsumerForwardsOptionalCapability(t *testing.T) {
	var nilService *Service
	if _, err := nilService.StartSyncWithEventConsumer(context.Background(), factorysessions.StartRequest{}, nil); !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("nil StartSyncWithEventConsumer error = %v", err)
	}

	base := &executionStub{}
	eventAware := &eventConsumerExecutionStub{executionStub: base}
	service, err := New(eventAware)
	if err != nil {
		t.Fatalf("New(event-aware): %v", err)
	}
	consumer := factorysessions.FactoryEventConsumer(func([]interfaces.FactoryEvent) {})
	started, err := service.StartSyncWithEventConsumer(
		context.Background(), factorysessions.StartRequest{RequestID: "event-aware"}, consumer,
	)
	if err != nil || started.SessionID != "event-aware-session" || !eventAware.consumed {
		t.Fatalf("event-aware start = (%#v, %v), consumed = %t", started, err, eventAware.consumed)
	}

	fallback := &syncFallbackExecutionStub{executionStub: &executionStub{}}
	service, err = New(fallback)
	if err != nil {
		t.Fatalf("New(fallback): %v", err)
	}
	started, err = service.StartSyncWithEventConsumer(
		context.Background(), factorysessions.StartRequest{RequestID: "fallback"}, nil,
	)
	if err != nil || started.SessionID != "fallback-session" || fallback.syncCalls != 1 {
		t.Fatalf("fallback start = (%#v, %v), sync calls = %d", started, err, fallback.syncCalls)
	}
}

type eventConsumerExecutionStub struct {
	*executionStub
	consumed bool
}

func (s *eventConsumerExecutionStub) StartSyncWithEventConsumer(
	context.Context,
	factorysessions.StartRequest,
	factorysessions.FactoryEventConsumer,
) (factorysessions.SyncStartResult, error) {
	s.consumed = true
	return factorysessions.SyncStartResult{
		AsyncStartResult: factorysessions.AsyncStartResult{SessionID: "event-aware-session"},
	}, nil
}

type syncFallbackExecutionStub struct {
	*executionStub
	syncCalls int
}

func (s *syncFallbackExecutionStub) StartSync(
	context.Context,
	factorysessions.StartRequest,
) (factorysessions.SyncStartResult, error) {
	s.syncCalls++
	return factorysessions.SyncStartResult{
		AsyncStartResult: factorysessions.AsyncStartResult{SessionID: "fallback-session"},
	}, nil
}

type executionWithoutMutation struct {
	durableexecution.Service
}
