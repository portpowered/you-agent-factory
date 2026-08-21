package recordingreplay

import (
	"context"
	"errors"
	"reflect"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	durableexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/services/durable_execution"
)

type handoffLiveOwner struct {
	fse.Service

	restorable     bool
	probeErr       error
	probeCalls     int
	resumeCalls    int
	resumeIntCalls int
	startCalls     int
	listCalls      int
	queryCalls     int
	queryRequest   fse.DispatchQueryRequest

	resumeResult    fse.LifecycleControlResult
	resumeErr       error
	pauseCalls      int
	pauseResult     fse.LifecycleControlResult
	resumeIntResult fse.AsyncStartResult
	resumeIntErr    error
	dispatches      fse.ListDispatchesResult
	queryResult     fse.ListDispatchesResult
	queryErr        error
	responseCalls   int
	responseRequest factorysessions.ResponseEventSubscriptionRequest
	responseCursor  *factorysessions.ResponseEventCursor
	responseErr     error
}

func (s *handoffLiveOwner) HasRestorableState(context.Context, string) (bool, error) {
	s.probeCalls++
	return s.restorable, s.probeErr
}

func (s *handoffLiveOwner) StartAsync(context.Context, fse.StartRequest) (fse.AsyncStartResult, error) {
	s.startCalls++
	return fse.AsyncStartResult{}, nil
}

func (s *handoffLiveOwner) StartSync(context.Context, fse.StartRequest) (fse.SyncStartResult, error) {
	s.startCalls++
	return fse.SyncStartResult{}, nil
}

func (s *handoffLiveOwner) ResumeInterruptedSession(
	context.Context,
	string,
	fse.ResumeSessionRequest,
) (fse.AsyncStartResult, error) {
	s.resumeIntCalls++
	return s.resumeIntResult, s.resumeIntErr
}

func (s *handoffLiveOwner) Resume(context.Context, string, fse.ControlRequest) (fse.LifecycleControlResult, error) {
	s.resumeCalls++
	return s.resumeResult, s.resumeErr
}

func (s *handoffLiveOwner) Pause(context.Context, string, fse.ControlRequest) (fse.LifecycleControlResult, error) {
	s.pauseCalls++
	return s.pauseResult, nil
}

func (s *handoffLiveOwner) ListDispatches(context.Context, string) (fse.ListDispatchesResult, error) {
	s.listCalls++
	return s.dispatches, nil
}

func (s *handoffLiveOwner) QueryDispatches(_ context.Context, request fse.DispatchQueryRequest) (fse.ListDispatchesResult, error) {
	s.queryCalls++
	s.queryRequest = request
	return s.queryResult, s.queryErr
}

func (s *handoffLiveOwner) SubscribeResponseEvents(
	_ context.Context,
	_ string,
	request factorysessions.ResponseEventSubscriptionRequest,
) (*factorysessions.ResponseEventCursor, error) {
	s.responseCalls++
	s.responseRequest = request
	return s.responseCursor, s.responseErr
}

var _ fse.Service = (*handoffLiveOwner)(nil)

func TestServiceResumeInterruptedSessionDelegatesOnlyAfterLiveProbe(t *testing.T) {
	projection, err := ReplayRecording(buildLifecycleRecording(t, "INTERRUPTED", false))
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	const sessionID = "dur-sess-lifecycle-recording"
	want := fse.AsyncStartResult{SessionID: sessionID, Status: "RESUMING", OrchestratorKind: "JAVASCRIPT"}
	live := &handoffLiveOwner{restorable: true, resumeIntResult: want}
	service := NewService(projection, live)

	got, err := service.ResumeInterruptedSession(context.Background(), sessionID, fse.ResumeSessionRequest{RequestID: "resume-1"})
	if err != nil {
		t.Fatalf("ResumeInterruptedSession error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ResumeInterruptedSession result = %#v, want %#v", got, want)
	}
	if live.probeCalls != 1 || live.resumeIntCalls != 1 {
		t.Fatalf("live calls = probe:%d resumeInterrupted:%d, want 1:1", live.probeCalls, live.resumeIntCalls)
	}
	if !service.IsNonLiveReplay() {
		t.Fatal("successful resume handoff lost replay routing marker")
	}
}

func TestServiceResumeUsesLiveResultAndMakesLiveOwnerAuthoritative(t *testing.T) {
	projection, err := ReplayRecording(buildLifecycleRecording(t, "PAUSED", false))
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	const sessionID = "dur-sess-lifecycle-recording"
	want := fse.LifecycleControlResult{
		SessionID: sessionID,
		Operation: fse.LifecycleControlResume,
		Outcome:   fse.LifecycleControlOutcomeAccepted,
		Status:    fse.LifecycleStatusRunning,
	}
	live := &handoffLiveOwner{
		restorable:   true,
		resumeResult: want,
		dispatches: fse.ListDispatchesResult{
			SessionID:  sessionID,
			Dispatches: []fse.DispatchSummary{{ID: "live-dispatch", Status: fse.DispatchStatusRunning}},
		},
		queryResult: fse.ListDispatchesResult{
			SessionID:  sessionID,
			Dispatches: []fse.DispatchSummary{{ID: "live-dispatch", Status: fse.DispatchStatusRunning}},
		},
	}
	service := NewService(projection, live)

	got, err := service.Resume(context.Background(), sessionID, fse.ControlRequest{RequestID: "resume-2"})
	if err != nil {
		t.Fatalf("Resume error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Resume result = %#v, want %#v", got, want)
	}
	dispatches, err := service.ListDispatches(context.Background(), sessionID)
	if err != nil {
		t.Fatalf("ListDispatches after handoff error = %v", err)
	}
	if len(dispatches.Dispatches) != 1 || dispatches.Dispatches[0].ID != "live-dispatch" || live.listCalls != 1 {
		t.Fatalf("ListDispatches after handoff = %#v, live calls = %d", dispatches, live.listCalls)
	}
	queried, err := service.QueryDispatches(context.Background(), fse.DispatchQueryRequest{
		SessionID: sessionID,
		Filters:   fse.DispatchFilters{Status: fse.DispatchStatusRunning},
	})
	if err != nil {
		t.Fatalf("QueryDispatches after handoff error = %v", err)
	}
	if !reflect.DeepEqual(queried, live.queryResult) || live.queryCalls != 1 || live.queryRequest.Filters.Status != fse.DispatchStatusRunning {
		t.Fatalf("QueryDispatches after handoff = %#v, calls = %d request = %#v", queried, live.queryCalls, live.queryRequest)
	}
	if _, err := service.QueryDispatches(context.Background(), fse.DispatchQueryRequest{SessionID: "missing"}); !errors.Is(err, fse.ErrSessionNotFound) {
		t.Fatalf("unknown QueryDispatches error = %v, want ErrSessionNotFound", err)
	}
	if live.queryCalls != 1 {
		t.Fatalf("unknown QueryDispatches reached live owner: calls = %d, want 1", live.queryCalls)
	}
}

func TestServiceSubscribeResponseEventsRoutesOnlyAfterHandoff(t *testing.T) {
	projection, err := ReplayRecording(buildLifecycleRecording(t, "PAUSED", false))
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	sessionID := projection.Session.SessionID
	request := factorysessions.ResponseEventSubscriptionRequest{
		SessionID: sessionID, AfterSequence: 7, DispatchID: "dispatch-1",
	}
	cursor := &factorysessions.ResponseEventCursor{}
	live := &handoffLiveOwner{restorable: true, responseCursor: cursor}
	service := NewService(projection, live)

	if _, err := service.SubscribeResponseEvents(context.Background(), sessionID, request); !errors.Is(err, ErrNonLiveReplay) {
		t.Fatalf("historical SubscribeResponseEvents error = %v, want ErrNonLiveReplay", err)
	}
	if live.responseCalls != 0 {
		t.Fatalf("historical response subscription calls = %d, want 0", live.responseCalls)
	}
	if _, err := service.SubscribeResponseEvents(context.Background(), "missing", request); !errors.Is(err, fse.ErrSessionNotFound) {
		t.Fatalf("unknown SubscribeResponseEvents error = %v, want ErrSessionNotFound", err)
	}
	if _, err := service.Resume(context.Background(), sessionID, fse.ControlRequest{}); err != nil {
		t.Fatalf("Resume error = %v", err)
	}
	got, err := service.SubscribeResponseEvents(context.Background(), sessionID, request)
	if err != nil {
		t.Fatalf("live SubscribeResponseEvents error = %v", err)
	}
	if got != cursor || live.responseCalls != 1 || !reflect.DeepEqual(live.responseRequest, request) {
		t.Fatalf("live response subscription = (%p, %d, %#v), want (%p, 1, %#v)", got, live.responseCalls, live.responseRequest, cursor, request)
	}

	withoutSubscriber := &handoffLiveOwnerWithoutResponseEvents{restorable: true}
	service = NewService(projection, withoutSubscriber)
	if _, err := service.Resume(context.Background(), sessionID, fse.ControlRequest{}); err != nil {
		t.Fatalf("Resume without response subscriber error = %v", err)
	}
	if _, err := service.SubscribeResponseEvents(context.Background(), sessionID, request); !errors.Is(err, factorysessions.ErrRuntimeNotAvailable) {
		t.Fatalf("unsupported response subscription error = %v, want ErrRuntimeNotAvailable", err)
	}
}

func TestServiceResumeFailureDoesNotTakeOwnership(t *testing.T) {
	projection, err := ReplayRecording(buildLifecycleRecording(t, "INTERRUPTED", false))
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	live := &handoffLiveOwner{
		restorable: true,
		resumeIntErr: &fse.ResumeError{
			Outcome:   fse.ResumeOutcomeCorruptedPersistence,
			SessionID: projection.Session.SessionID,
			Message:   "checkpoint state is invalid",
		},
	}
	service := NewService(projection, live)

	_, err = service.ResumeInterruptedSession(context.Background(), projection.Session.SessionID, fse.ResumeSessionRequest{RequestID: "resume-3"})
	var resumeErr *fse.ResumeError
	if !errors.As(err, &resumeErr) || resumeErr.Outcome != fse.ResumeOutcomeCorruptedPersistence {
		t.Fatalf("ResumeInterruptedSession error = %v, want typed persistence error", err)
	}
	if !service.IsNonLiveReplay() {
		t.Fatal("failed resume handoff made live owner authoritative")
	}
	if _, err := service.Pause(context.Background(), projection.Session.SessionID, fse.ControlRequest{}); !errors.Is(err, ErrNonLiveReplay) {
		t.Fatalf("Pause after failed handoff error = %v, want ErrNonLiveReplay", err)
	}
}

func TestServiceResumeProbeClassifiesOwnerFailuresAndRetainsHandoff(t *testing.T) {
	projection, err := ReplayRecording(buildLifecycleRecording(t, "INTERRUPTED", false))
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	sessionID := projection.Session.SessionID

	t.Run("typed probe failure is forwarded", func(t *testing.T) {
		testTypedProbeFailure(t, projection, sessionID)
	})
	t.Run("missing persisted state remains non-live", func(t *testing.T) {
		testMissingPersistedState(t, projection, sessionID)
	})
	t.Run("owner without probe remains non-live", func(t *testing.T) {
		testOwnerWithoutProbe(t, projection, sessionID)
	})
	t.Run("successful handoff retains owner for later operations", func(t *testing.T) {
		testSuccessfulHandoffRetainsOwner(t, projection, sessionID)
	})

	var nilService *Service
	if owner, handedOff := nilService.handedOffOwner(); owner != nil || handedOff {
		t.Fatalf("nil handedOffOwner() = (%v, %t), want (nil, false)", owner, handedOff)
	}
	nilService.markHandedOff()
}

func testTypedProbeFailure(t *testing.T, projection RecordingReplayProjection, sessionID string) {
	want := &fse.ResumeError{
		Outcome:   fse.ResumeOutcomeCorruptedPersistence,
		SessionID: sessionID,
		Message:   "checkpoint state is unavailable",
	}
	service := NewService(projection, &handoffLiveOwner{probeErr: want})
	_, err := service.Resume(context.Background(), sessionID, fse.ControlRequest{})
	var got *fse.ResumeError
	if !errors.As(err, &got) || got != want {
		t.Fatalf("Resume probe error = %T %#v, want forwarded %#v", err, err, want)
	}
	if !service.IsNonLiveReplay() {
		t.Fatal("probe failure made replay live")
	}
}

func testMissingPersistedState(t *testing.T, projection RecordingReplayProjection, sessionID string) {
	service := NewService(projection, &handoffLiveOwner{probeErr: fse.ErrSessionNotFound})
	_, err := service.ResumeInterruptedSession(context.Background(), sessionID, fse.ResumeSessionRequest{})
	if !errors.Is(err, ErrNonLiveReplay) {
		t.Fatalf("ResumeInterruptedSession error = %v, want ErrNonLiveReplay", err)
	}
}

func testOwnerWithoutProbe(t *testing.T, projection RecordingReplayProjection, sessionID string) {
	service := NewService(projection, &handoffOwnerWithoutProbe{})
	_, err := service.ResumeInterruptedSession(context.Background(), sessionID, fse.ResumeSessionRequest{})
	if !errors.Is(err, ErrNonLiveReplay) {
		t.Fatalf("ResumeInterruptedSession error = %v, want ErrNonLiveReplay", err)
	}
}

func testSuccessfulHandoffRetainsOwner(t *testing.T, projection RecordingReplayProjection, sessionID string) {
	live := &handoffLiveOwner{
		restorable:   true,
		resumeResult: fse.LifecycleControlResult{SessionID: sessionID},
		pauseResult:  fse.LifecycleControlResult{SessionID: sessionID},
	}
	service := NewService(projection, live)
	if _, err := service.Pause(context.Background(), sessionID, fse.ControlRequest{}); !errors.Is(err, ErrNonLiveReplay) {
		t.Fatalf("Pause before handoff error = %v, want ErrNonLiveReplay", err)
	}
	if _, err := service.Pause(context.Background(), "missing", fse.ControlRequest{}); !errors.Is(err, fse.ErrSessionNotFound) {
		t.Fatalf("Pause for unknown session error = %v, want ErrSessionNotFound", err)
	}
	if _, err := service.Resume(context.Background(), sessionID, fse.ControlRequest{}); err != nil {
		t.Fatalf("first Resume error = %v", err)
	}
	if _, err := service.Resume(context.Background(), sessionID, fse.ControlRequest{}); err != nil {
		t.Fatalf("second Resume error = %v", err)
	}
	result, err := controlplane.PauseDurableFactorySession(
		context.Background(),
		&replayControlPlaneHost{execution: service},
		sessionID,
		fse.ControlRequest{},
	)
	if err != nil {
		t.Fatalf("control-plane Pause after handoff error = %v", err)
	}
	if result.SessionID != sessionID {
		t.Fatalf("control-plane Pause result session = %q, want %q", result.SessionID, sessionID)
	}
	if live.probeCalls != 1 || live.resumeCalls != 2 || live.pauseCalls != 1 {
		t.Fatalf("live calls = probe:%d resume:%d pause:%d, want 1:2:1", live.probeCalls, live.resumeCalls, live.pauseCalls)
	}
	if !service.IsNonLiveReplay() {
		t.Fatal("successful handoff lost replay routing marker")
	}
}

type replayControlPlaneHost struct {
	execution durableexecution.Service
}

func (h *replayControlPlaneHost) DurableExecution() durableexecution.Service {
	return h.execution
}

type handoffOwnerWithoutProbe struct{ fse.Service }

type handoffLiveOwnerWithoutResponseEvents struct {
	fse.Service
	restorable bool
}

func (s *handoffLiveOwnerWithoutResponseEvents) HasRestorableState(context.Context, string) (bool, error) {
	return s.restorable, nil
}

func (s *handoffLiveOwnerWithoutResponseEvents) Resume(context.Context, string, fse.ControlRequest) (fse.LifecycleControlResult, error) {
	return fse.LifecycleControlResult{}, nil
}

func TestServiceWallsEightNonResumeOperationsBeforeHandoff(t *testing.T) {
	projection, err := ReplayRecording(buildLifecycleRecording(t, "INTERRUPTED", false))
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	sessionID := projection.Session.SessionID
	live := &handoffLiveOwner{restorable: true}
	service := NewService(projection, live)
	ctx := context.Background()

	operations := []struct {
		name string
		run  func() error
	}{
		{name: "start async", run: func() error { _, err := service.StartAsync(ctx, fse.StartRequest{}); return err }},
		{name: "start sync", run: func() error { _, err := service.StartSync(ctx, fse.StartRequest{}); return err }},
		{name: "pause", run: func() error { _, err := service.Pause(ctx, sessionID, fse.ControlRequest{}); return err }},
		{name: "cancel", run: func() error { _, err := service.Cancel(ctx, sessionID, fse.ControlRequest{}); return err }},
		{name: "terminate", run: func() error { _, err := service.Terminate(ctx, sessionID, fse.ControlRequest{}); return err }},
		{name: "approve", run: func() error { _, err := service.Approve(ctx, sessionID, fse.ApproveRequest{}); return err }},
		{name: "retry dispatch", run: func() error { _, err := service.RetryDispatch(ctx, sessionID, fse.RetryDispatchRequest{}); return err }},
		{name: "interrupt dispatch", run: func() error {
			_, err := service.InterruptDispatch(ctx, sessionID, fse.InterruptDispatchRequest{})
			return err
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.run(); !errors.Is(err, ErrNonLiveReplay) {
				t.Fatalf("error = %v, want ErrNonLiveReplay", err)
			}
		})
	}
	if live.probeCalls != 0 || live.startCalls != 0 || live.resumeCalls != 0 || live.resumeIntCalls != 0 {
		t.Fatalf("live owner was consulted before handoff: probe:%d start:%d resume:%d resumeInterrupted:%d", live.probeCalls, live.startCalls, live.resumeCalls, live.resumeIntCalls)
	}
}

func TestServiceWallsAllOperationsWhenPublicCheckpointIsNotRestorable(t *testing.T) {
	projection, err := ReplayRecording(buildLifecycleRecording(t, "PAUSED", false))
	if err != nil {
		t.Fatalf("ReplayRecording: %v", err)
	}
	sessionID := projection.Session.SessionID
	live := &handoffLiveOwner{}
	service := NewService(projection, live)
	ctx := context.Background()
	operations := []struct {
		name string
		run  func() error
	}{
		{name: "start async", run: func() error { _, err := service.StartAsync(ctx, fse.StartRequest{}); return err }},
		{name: "start sync", run: func() error { _, err := service.StartSync(ctx, fse.StartRequest{}); return err }},
		{name: "resume interrupted", run: func() error {
			_, err := service.ResumeInterruptedSession(ctx, sessionID, fse.ResumeSessionRequest{})
			return err
		}},
		{name: "pause", run: func() error { _, err := service.Pause(ctx, sessionID, fse.ControlRequest{}); return err }},
		{name: "resume", run: func() error { _, err := service.Resume(ctx, sessionID, fse.ControlRequest{}); return err }},
		{name: "cancel", run: func() error { _, err := service.Cancel(ctx, sessionID, fse.ControlRequest{}); return err }},
		{name: "terminate", run: func() error { _, err := service.Terminate(ctx, sessionID, fse.ControlRequest{}); return err }},
		{name: "approve", run: func() error { _, err := service.Approve(ctx, sessionID, fse.ApproveRequest{}); return err }},
		{name: "retry dispatch", run: func() error { _, err := service.RetryDispatch(ctx, sessionID, fse.RetryDispatchRequest{}); return err }},
		{name: "interrupt dispatch", run: func() error {
			_, err := service.InterruptDispatch(ctx, sessionID, fse.InterruptDispatchRequest{})
			return err
		}},
	}
	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			if err := operation.run(); !errors.Is(err, ErrNonLiveReplay) {
				t.Fatalf("error = %v, want ErrNonLiveReplay", err)
			}
		})
	}
	if live.probeCalls != 2 || live.startCalls != 0 || live.resumeCalls != 0 || live.resumeIntCalls != 0 {
		t.Fatalf("live owner calls = probe:%d start:%d resume:%d resumeInterrupted:%d", live.probeCalls, live.startCalls, live.resumeCalls, live.resumeIntCalls)
	}
}
