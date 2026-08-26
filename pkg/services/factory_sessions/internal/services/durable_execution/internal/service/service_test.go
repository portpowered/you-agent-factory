package service

import (
	"context"
	"errors"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/workers"
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

func TestServiceForwardsOptionalRestorableStateProbe(t *testing.T) {
	t.Parallel()

	stub := &restorableStateExecutionStub{restorable: true}
	service, err := New(stub)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := service.HasRestorableState(context.Background(), "dur-sess-1")
	if err != nil || !got || stub.calls != 1 {
		t.Fatalf("HasRestorableState = (%v, %v), calls = %d", got, err, stub.calls)
	}
}

func TestServiceForwardsPersistenceBackedStateProbe(t *testing.T) {
	t.Parallel()

	stub := &restorableStateExecutionStub{restorable: true}
	service, err := New(stub)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, err := service.HasDurableState(context.Background(), "~default")
	if err != nil || !got {
		t.Fatalf("HasDurableState = (%v, %v), want true", got, err)
	}
}

func TestServicePreflightsLifecycleResumeOnlyForInterruptedSessions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		status        factorysessions.LifecycleStatus
		restorable    bool
		wantErr       error
		wantProbeCall int
	}{
		{
			name:          "interrupted and restorable",
			status:        factorysessions.LifecycleStatusInterrupted,
			restorable:    true,
			wantProbeCall: 1,
		},
		{
			name:          "interrupted without restorable state",
			status:        factorysessions.LifecycleStatusInterrupted,
			wantErr:       factorysessions.ErrDurableSessionNotFound,
			wantProbeCall: 1,
		},
		{
			name:       "running skips restorable probe",
			status:     factorysessions.LifecycleStatusRunning,
			restorable: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			stub := &restorableStateExecutionStub{
				restorable:      test.restorable,
				lifecycleStatus: test.status,
			}
			service, err := New(stub)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			got, err := service.Resume(
				context.Background(),
				"dur-sess-1",
				factorysessions.ControlRequest{RequestID: "resume-control-1"},
			)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("Resume error = %v, want %v", err, test.wantErr)
			}
			if test.wantErr == nil && got.SessionID != "dur-sess-1" {
				t.Fatalf("Resume = %#v, want forwarded session", got)
			}
			if stub.calls != test.wantProbeCall {
				t.Fatalf("probe calls = %d, want %d", stub.calls, test.wantProbeCall)
			}
			wantResumeCalls := 0
			if test.wantErr == nil {
				wantResumeCalls = 1
			}
			if stub.resumeControlCalls != wantResumeCalls {
				t.Fatalf("forwarded Resume calls = %d, want %d", stub.resumeControlCalls, wantResumeCalls)
			}
		})
	}
}

type restorableStateExecutionStub struct {
	durableexecution.Service
	restorable         bool
	lifecycleStatus    factorysessions.LifecycleStatus
	calls              int
	resumeControlCalls int
}

func (s *restorableStateExecutionStub) HasRestorableState(context.Context, string) (bool, error) {
	s.calls++
	return s.restorable, nil
}

func (s *restorableStateExecutionStub) HasDurableState(context.Context, string) (bool, error) {
	return s.restorable, nil
}

func (s *restorableStateExecutionStub) GetSession(context.Context, string) (factorysessions.SessionReadResult, error) {
	return factorysessions.SessionReadResult{
		SessionID: "dur-sess-1",
		Status:    s.lifecycleStatus,
	}, nil
}

func (s *restorableStateExecutionStub) Resume(
	context.Context,
	string,
	factorysessions.ControlRequest,
) (factorysessions.LifecycleControlResult, error) {
	s.resumeControlCalls++
	return factorysessions.LifecycleControlResult{
		SessionID: "dur-sess-1",
	}, nil
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

func TestServiceCloseForwardsOptionalExecutionOwner(t *testing.T) {
	t.Parallel()

	stub := &closeAwareExecutionStub{}
	service, err := New(stub)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := service.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if stub.closeCalls != 1 {
		t.Fatalf("execution close calls = %d, want 1", stub.closeCalls)
	}
}

type executionWithoutMutation struct {
	durableexecution.Service
}

type closeAwareExecutionStub struct {
	executionStub
	closeCalls int
}

func (s *closeAwareExecutionStub) Close() error {
	s.closeCalls++
	return nil
}

func TestServiceForwardsOptionalLiveChangeAndWorkerCapabilities(t *testing.T) {
	t.Parallel()

	stub := &executionCapabilitiesStub{}
	service, err := New(stub)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	service.SetWorkerInvoker(nil)
	service.SetWorkerExecution(nil, nil, "runtime-1", "generation-1", nil, nil, nil)
	service.SetWorkerProgressPublisher(func(workers.ProgressFragment) {})
	service.SetWorkerAttemptStarter(func(
		context.Context,
		workers.ExecuteRequest,
	) (func(context.Context, workers.ExecuteResult, error) error, error) {
		return func(context.Context, workers.ExecuteResult, error) error { return nil }, nil
	})
	cursor, err := service.SubscribeResponseEvents(
		context.Background(),
		"session-1",
		factorysessions.ResponseEventSubscriptionRequest{SessionID: "session-1", AfterSequence: 7},
	)
	if err != nil || cursor != stub.cursor {
		t.Fatalf("SubscribeResponseEvents = (%#v, %v), want delegated cursor", cursor, err)
	}
	applied, err := service.ApplyLiveChange(context.Background(), "session-1", factorysessions.LiveChangeRequest{RequestID: "request-1"})
	if err != nil || applied.RequestID != "request-1" {
		t.Fatalf("ApplyLiveChange = (%#v, %v)", applied, err)
	}
	recovered, err := service.RecoverLiveChange(context.Background(), "session-1", "request-1")
	if err != nil || recovered.RequestID != "request-1" {
		t.Fatalf("RecoverLiveChange = (%#v, %v)", recovered, err)
	}
	service.PublishWorkerProgress(workers.ProgressFragment{DispatchID: "dispatch-1", Kind: workers.ProgressFragmentKind})

	gotCalls := [...]int{
		stub.setCalls, stub.setExecutionCalls, stub.progressPublisherCalls,
		stub.attemptStarterCalls, stub.subscribeCalls, stub.applyCalls,
		stub.recoverCalls, stub.progressCalls,
	}
	if gotCalls != [...]int{1, 1, 1, 1, 1, 1, 1, 1} {
		t.Fatalf("optional capability calls = %#v, want one call each", gotCalls)
	}
}

func TestServiceOptionalCapabilitiesReturnUnavailableWhenUnsupported(t *testing.T) {
	t.Parallel()

	service, err := New(&executionWithoutMutation{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	service.SetWorkerInvoker(nil)
	service.SetWorkerExecution(nil, nil, "", "", nil, nil, nil)
	service.PublishWorkerProgress(workers.ProgressFragment{})
	if _, err := service.SubscribeResponseEvents(context.Background(), "session-1", factorysessions.ResponseEventSubscriptionRequest{}); !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("SubscribeResponseEvents error = %v, want ErrRuntimeNotAvailable", err)
	}
	if _, err := service.ApplyLiveChange(context.Background(), "session-1", factorysessions.LiveChangeRequest{}); !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("ApplyLiveChange error = %v, want ErrRuntimeNotAvailable", err)
	}
	if _, err := service.RecoverLiveChange(context.Background(), "session-1", "request-1"); !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("RecoverLiveChange error = %v, want ErrRuntimeNotAvailable", err)
	}

	var nilService *Service
	if nilService.IsNonLiveReplay() {
		t.Fatal("nil Service IsNonLiveReplay = true, want false")
	}
	if _, err := nilService.SubscribeResponseEvents(context.Background(), "session-1", factorysessions.ResponseEventSubscriptionRequest{}); !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("nil Service SubscribeResponseEvents error = %v, want ErrRuntimeNotAvailable", err)
	}
}

type executionCapabilitiesStub struct {
	durableexecution.Service
	cursor                 *factorysessions.ResponseEventCursor
	setCalls               int
	setExecutionCalls      int
	subscribeCalls         int
	applyCalls             int
	recoverCalls           int
	progressCalls          int
	progressPublisherCalls int
	attemptStarterCalls    int
}

func (s *executionCapabilitiesStub) SetWorkerInvoker(factoryruntime.Service) {
	s.setCalls++
}

func (s *executionCapabilitiesStub) SetWorkerExecution(
	interface {
		Execute(context.Context, workers.ExecuteRequest) (workers.ExecuteResult, error)
	},
	factoryruntime.ResourceCapacityLeaseAdmission,
	string,
	string,
	providers.Service,
	*workers.MockWorkersConfig,
	platformprocess.CommandRunner,
) {
	s.setExecutionCalls++
}

func (s *executionCapabilitiesStub) SubscribeResponseEvents(context.Context, string, factorysessions.ResponseEventSubscriptionRequest) (*factorysessions.ResponseEventCursor, error) {
	s.subscribeCalls++
	if s.cursor == nil {
		s.cursor = &factorysessions.ResponseEventCursor{}
	}
	return s.cursor, nil
}

func (s *executionCapabilitiesStub) ApplyLiveChange(_ context.Context, _ string, request factorysessions.LiveChangeRequest) (factorysessions.LiveChangeResult, error) {
	s.applyCalls++
	return factorysessions.LiveChangeResult{RequestID: request.RequestID}, nil
}

func (s *executionCapabilitiesStub) RecoverLiveChange(_ context.Context, _ string, requestID string) (factorysessions.LiveChangeResult, error) {
	s.recoverCalls++
	return factorysessions.LiveChangeResult{RequestID: requestID}, nil
}

func (s *executionCapabilitiesStub) PublishWorkerProgress(workers.ProgressFragment) {
	s.progressCalls++
}

func (s *executionCapabilitiesStub) SetWorkerProgressPublisher(workers.ProgressPublisher) {
	s.progressPublisherCalls++
}

func (s *executionCapabilitiesStub) SetWorkerAttemptStarter(func(
	context.Context,
	workers.ExecuteRequest,
) (func(context.Context, workers.ExecuteResult, error) error, error)) {
	s.attemptStarterCalls++
}
