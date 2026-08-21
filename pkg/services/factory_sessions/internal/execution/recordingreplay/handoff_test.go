package recordingreplay

import (
	"context"
	"errors"
	"reflect"
	"testing"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
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

	resumeResult    fse.LifecycleControlResult
	resumeErr       error
	resumeIntResult fse.AsyncStartResult
	resumeIntErr    error
	dispatches      fse.ListDispatchesResult
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

func (s *handoffLiveOwner) ListDispatches(context.Context, string) (fse.ListDispatchesResult, error) {
	s.listCalls++
	return s.dispatches, nil
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
	if service.IsNonLiveReplay() {
		t.Fatal("successful resume handoff remained marked as non-live")
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
