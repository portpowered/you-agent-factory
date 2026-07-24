package automations_test

import (
	"context"
	"errors"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

// fakeRootService is a peer-shaped Automations Service that depends only on
// Automations root contracts. It proves the singular root seam is implementable
// without Automations implementation packages or cron/poller/watcher types.
type fakeRootService struct {
	ready               bool
	conflictOnReconcile bool
	sources             map[string]string
	instances           map[string]fakeInstance
}

type fakeInstance struct {
	automationID string
	status       string
	cursor       string
	checkpoint   string
}

func (f *fakeRootService) ensureSources() {
	if f.sources == nil {
		f.sources = make(map[string]string)
	}
}

func (f *fakeRootService) ensureInstances() {
	if f.instances == nil {
		f.instances = make(map[string]fakeInstance)
	}
}

func (f *fakeRootService) Ready(context.Context, automations.ReadyRequest) (automations.ReadyResult, error) {
	if f == nil || !f.ready {
		return automations.ReadyResult{}, &automations.Error{
			Op:   "Ready",
			Code: automations.ErrorCodeNotReady,
			Err:  automations.ErrNotReady,
		}
	}
	return automations.ReadyResult{Ready: true}, nil
}

func (f *fakeRootService) Reconcile(_ context.Context, req automations.ReconcileRequest) (automations.ReconcileResult, error) {
	if f == nil || !f.ready {
		return automations.ReconcileResult{}, &automations.Error{
			Op:   "Reconcile",
			Code: automations.ErrorCodeNotReady,
			Err:  automations.ErrNotReady,
		}
	}
	if len(req.Desired) == 0 {
		return automations.ReconcileResult{}, &automations.Error{
			Op:   "Reconcile",
			Code: automations.ErrorCodeInvalid,
			Err:  automations.ErrInvalidRequest,
		}
	}
	for _, spec := range req.Desired {
		if spec.AutomationID == "" {
			return automations.ReconcileResult{}, &automations.Error{
				Op:   "Reconcile",
				Code: automations.ErrorCodeInvalid,
				Err:  automations.ErrInvalidRequest,
			}
		}
	}
	if f.conflictOnReconcile {
		return automations.ReconcileResult{}, &automations.Error{
			Op:   "Reconcile",
			Code: automations.ErrorCodeConflict,
			Err:  automations.ErrConflict,
		}
	}

	outcomes := make([]automations.ConvergenceOutcome, 0, len(req.Desired))
	observedByAutomation := make(map[string]automations.ObservedInstance, len(req.Observed))
	for _, observed := range req.Observed {
		observedByAutomation[observed.AutomationID] = observed
	}
	for _, spec := range req.Desired {
		outcome := automations.ConvergenceOutcome{
			AutomationID: spec.AutomationID,
			Action:       automations.ConvergenceActionCreated,
			Status:       automations.InstanceStatusReady,
		}
		if observed, ok := observedByAutomation[spec.AutomationID]; ok {
			outcome.InstanceID = observed.InstanceID
			outcome.Action = automations.ConvergenceActionUpdated
			outcome.Status = observed.Status
			if observed.Status == "" {
				outcome.Status = automations.InstanceStatusReady
			}
		} else {
			outcome.InstanceID = "instance:" + spec.AutomationID
		}
		outcomes = append(outcomes, outcome)
	}
	return automations.ReconcileResult{Outcomes: outcomes}, nil
}

func (f *fakeRootService) StartSource(_ context.Context, req automations.StartSourceRequest) (automations.StartSourceResult, error) {
	if f == nil || !f.ready {
		return automations.StartSourceResult{}, &automations.Error{
			Op:   "StartSource",
			Code: automations.ErrorCodeNotReady,
			Err:  automations.ErrNotReady,
		}
	}
	if req.SourceID == "" {
		return automations.StartSourceResult{}, &automations.Error{
			Op:   "StartSource",
			Code: automations.ErrorCodeInvalid,
			Err:  automations.ErrInvalidRequest,
		}
	}
	f.ensureSources()
	if status, ok := f.sources[req.SourceID]; ok && status != automations.InstanceStatusStopped {
		return automations.StartSourceResult{}, &automations.Error{
			Op:   "StartSource",
			Code: automations.ErrorCodeInvalid,
			Err:  automations.ErrInvalidRequest,
		}
	}
	f.sources[req.SourceID] = automations.InstanceStatusRunning
	return automations.StartSourceResult{
		Handle: automations.SourceHandle{ID: req.SourceID},
		Status: automations.InstanceStatusRunning,
	}, nil
}

func (f *fakeRootService) StopSource(_ context.Context, req automations.StopSourceRequest) (automations.StopSourceResult, error) {
	if f == nil || !f.ready {
		return automations.StopSourceResult{}, &automations.Error{
			Op:   "StopSource",
			Code: automations.ErrorCodeNotReady,
			Err:  automations.ErrNotReady,
		}
	}
	if req.Handle.ID == "" {
		return automations.StopSourceResult{}, &automations.Error{
			Op:   "StopSource",
			Code: automations.ErrorCodeInvalid,
			Err:  automations.ErrInvalidRequest,
		}
	}
	f.ensureSources()
	status, ok := f.sources[req.Handle.ID]
	if !ok {
		return automations.StopSourceResult{}, &automations.Error{
			Op:   "StopSource",
			Code: automations.ErrorCodeNotFound,
			Err:  automations.ErrNotFound,
		}
	}
	if status == automations.InstanceStatusStopped {
		return automations.StopSourceResult{}, &automations.Error{
			Op:   "StopSource",
			Code: automations.ErrorCodeConflict,
			Err:  automations.ErrConflict,
		}
	}
	f.sources[req.Handle.ID] = automations.InstanceStatusStopped
	return automations.StopSourceResult{
		Handle: req.Handle,
		Status: automations.InstanceStatusStopped,
	}, nil
}

func (f *fakeRootService) WaitSource(_ context.Context, req automations.WaitSourceRequest) (automations.WaitSourceResult, error) {
	if f == nil || !f.ready {
		return automations.WaitSourceResult{}, &automations.Error{
			Op:   "WaitSource",
			Code: automations.ErrorCodeNotReady,
			Err:  automations.ErrNotReady,
		}
	}
	if req.Handle.ID == "" {
		return automations.WaitSourceResult{}, &automations.Error{
			Op:   "WaitSource",
			Code: automations.ErrorCodeInvalid,
			Err:  automations.ErrInvalidRequest,
		}
	}
	f.ensureSources()
	status, ok := f.sources[req.Handle.ID]
	if !ok {
		return automations.WaitSourceResult{}, &automations.Error{
			Op:   "WaitSource",
			Code: automations.ErrorCodeNotFound,
			Err:  automations.ErrNotFound,
		}
	}
	if status != automations.InstanceStatusStopped {
		return automations.WaitSourceResult{}, &automations.Error{
			Op:   "WaitSource",
			Code: automations.ErrorCodeConflict,
			Err:  automations.ErrConflict,
		}
	}
	return automations.WaitSourceResult{
		Handle: req.Handle,
		Status: automations.InstanceStatusStopped,
	}, nil
}

func (f *fakeRootService) SourceStatus(_ context.Context, req automations.SourceStatusRequest) (automations.SourceStatusResult, error) {
	if f == nil || !f.ready {
		return automations.SourceStatusResult{}, &automations.Error{
			Op:   "SourceStatus",
			Code: automations.ErrorCodeNotReady,
			Err:  automations.ErrNotReady,
		}
	}
	if req.Handle.ID == "" {
		return automations.SourceStatusResult{}, &automations.Error{
			Op:   "SourceStatus",
			Code: automations.ErrorCodeInvalid,
			Err:  automations.ErrInvalidRequest,
		}
	}
	f.ensureSources()
	status, ok := f.sources[req.Handle.ID]
	if !ok {
		return automations.SourceStatusResult{}, &automations.Error{
			Op:   "SourceStatus",
			Code: automations.ErrorCodeNotFound,
			Err:  automations.ErrNotFound,
		}
	}
	return automations.SourceStatusResult{
		Handle: req.Handle,
		Status: status,
	}, nil
}

func (f *fakeRootService) GetStatus(_ context.Context, req automations.GetStatusRequest) (automations.GetStatusResult, error) {
	if f == nil || !f.ready {
		return automations.GetStatusResult{}, &automations.Error{
			Op:   "GetStatus",
			Code: automations.ErrorCodeNotReady,
			Err:  automations.ErrNotReady,
		}
	}
	if req.InstanceID == "" {
		return automations.GetStatusResult{}, &automations.Error{
			Op:   "GetStatus",
			Code: automations.ErrorCodeInvalid,
			Err:  automations.ErrInvalidRequest,
		}
	}
	f.ensureInstances()
	instance, ok := f.instances[req.InstanceID]
	if !ok {
		return automations.GetStatusResult{}, &automations.Error{
			Op:   "GetStatus",
			Code: automations.ErrorCodeNotFound,
			Err:  automations.ErrNotFound,
		}
	}
	return automations.GetStatusResult{
		AutomationID: instance.automationID,
		InstanceID:   req.InstanceID,
		Status:       instance.status,
	}, nil
}

func (f *fakeRootService) GetCursor(_ context.Context, req automations.GetCursorRequest) (automations.GetCursorResult, error) {
	if f == nil || !f.ready {
		return automations.GetCursorResult{}, &automations.Error{
			Op:   "GetCursor",
			Code: automations.ErrorCodeNotReady,
			Err:  automations.ErrNotReady,
		}
	}
	if req.InstanceID == "" {
		return automations.GetCursorResult{}, &automations.Error{
			Op:   "GetCursor",
			Code: automations.ErrorCodeInvalid,
			Err:  automations.ErrInvalidRequest,
		}
	}
	f.ensureInstances()
	instance, ok := f.instances[req.InstanceID]
	if !ok {
		return automations.GetCursorResult{}, &automations.Error{
			Op:   "GetCursor",
			Code: automations.ErrorCodeNotFound,
			Err:  automations.ErrNotFound,
		}
	}
	if req.ExpectedCursor != "" && req.ExpectedCursor != instance.cursor {
		return automations.GetCursorResult{}, &automations.Error{
			Op:   "GetCursor",
			Code: automations.ErrorCodeConflict,
			Err:  automations.ErrConflict,
		}
	}
	return automations.GetCursorResult{
		AutomationID: instance.automationID,
		InstanceID:   req.InstanceID,
		Cursor:       instance.cursor,
		Checkpoint:   instance.checkpoint,
	}, nil
}

func TestServiceRootSeam_FakeImplementsPlainContracts(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{ready: true}
	result, err := svc.Ready(context.Background(), automations.ReadyRequest{})
	if err != nil {
		t.Fatalf("Ready() unexpected error: %v", err)
	}
	if !result.Ready {
		t.Fatalf("Ready() result.Ready = false, want true")
	}
}

func TestServiceRootSeam_FakeTypedNotReady(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{ready: false}
	_, err := svc.Ready(context.Background(), automations.ReadyRequest{})
	if err == nil {
		t.Fatal("Ready() error = nil, want typed not-ready error")
	}
	var typed *automations.Error
	if !errors.As(err, &typed) {
		t.Fatalf("Ready() error type = %T, want *automations.Error", err)
	}
	if typed.Code != automations.ErrorCodeNotReady {
		t.Fatalf("Ready() error code = %q, want %q", typed.Code, automations.ErrorCodeNotReady)
	}
	if !errors.Is(err, automations.ErrNotReady) {
		t.Fatalf("Ready() error = %v, want errors.Is ErrNotReady", err)
	}
}

func TestServiceReconcile_FakeSuccessDetachedOutcomes(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{ready: true}
	result, err := svc.Reconcile(context.Background(), automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{
			{AutomationID: "auto-a", Kind: "schedule", Enabled: true},
		},
		Observed: []automations.ObservedInstance{
			{AutomationID: "auto-a", InstanceID: "inst-a", Status: automations.InstanceStatusRunning},
		},
	})
	if err != nil {
		t.Fatalf("Reconcile() unexpected error: %v", err)
	}
	if len(result.Outcomes) != 1 {
		t.Fatalf("Reconcile() outcomes len = %d, want 1", len(result.Outcomes))
	}
	outcome := result.Outcomes[0]
	if outcome.AutomationID != "auto-a" {
		t.Fatalf("Reconcile() outcome.AutomationID = %q, want %q", outcome.AutomationID, "auto-a")
	}
	if outcome.InstanceID != "inst-a" {
		t.Fatalf("Reconcile() outcome.InstanceID = %q, want %q", outcome.InstanceID, "inst-a")
	}
	if outcome.Action != automations.ConvergenceActionUpdated {
		t.Fatalf("Reconcile() outcome.Action = %q, want %q", outcome.Action, automations.ConvergenceActionUpdated)
	}
	if outcome.Status != automations.InstanceStatusRunning {
		t.Fatalf("Reconcile() outcome.Status = %q, want %q", outcome.Status, automations.InstanceStatusRunning)
	}
}

func TestServiceReconcile_FakeTypedInvalidDesiredSpecs(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{ready: true}
	_, err := svc.Reconcile(context.Background(), automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{{AutomationID: "", Kind: "schedule", Enabled: true}},
	})
	if err == nil {
		t.Fatal("Reconcile() error = nil, want typed invalid-request error")
	}
	var typed *automations.Error
	if !errors.As(err, &typed) {
		t.Fatalf("Reconcile() error type = %T, want *automations.Error", err)
	}
	if typed.Code != automations.ErrorCodeInvalid {
		t.Fatalf("Reconcile() error code = %q, want %q", typed.Code, automations.ErrorCodeInvalid)
	}
	if !errors.Is(err, automations.ErrInvalidRequest) {
		t.Fatalf("Reconcile() error = %v, want errors.Is ErrInvalidRequest", err)
	}
}

func TestServiceReconcile_FakeTypedConflict(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{ready: true, conflictOnReconcile: true}
	_, err := svc.Reconcile(context.Background(), automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{
			{AutomationID: "auto-a", Kind: "schedule", Enabled: true},
		},
	})
	if err == nil {
		t.Fatal("Reconcile() error = nil, want typed conflict error")
	}
	var typed *automations.Error
	if !errors.As(err, &typed) {
		t.Fatalf("Reconcile() error type = %T, want *automations.Error", err)
	}
	if typed.Code != automations.ErrorCodeConflict {
		t.Fatalf("Reconcile() error code = %q, want %q", typed.Code, automations.ErrorCodeConflict)
	}
	if !errors.Is(err, automations.ErrConflict) {
		t.Fatalf("Reconcile() error = %v, want errors.Is ErrConflict", err)
	}
}

func TestServiceSourceLifecycle_FakeSuccessStartStatusStopWait(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{ready: true}
	started, err := svc.StartSource(context.Background(), automations.StartSourceRequest{
		SourceID: "source-a",
		Kind:     "schedule",
	})
	if err != nil {
		t.Fatalf("StartSource() unexpected error: %v", err)
	}
	if started.Handle.ID == "" {
		t.Fatal("StartSource() handle.ID is empty")
	}
	if started.Status != automations.InstanceStatusRunning {
		t.Fatalf("StartSource() status = %q, want %q", started.Status, automations.InstanceStatusRunning)
	}

	status, err := svc.SourceStatus(context.Background(), automations.SourceStatusRequest{
		Handle: started.Handle,
	})
	if err != nil {
		t.Fatalf("SourceStatus() unexpected error: %v", err)
	}
	if status.Status != automations.InstanceStatusRunning && status.Status != automations.InstanceStatusReady {
		t.Fatalf("SourceStatus() status = %q, want running or ready", status.Status)
	}

	stopped, err := svc.StopSource(context.Background(), automations.StopSourceRequest{
		Handle: started.Handle,
	})
	if err != nil {
		t.Fatalf("StopSource() unexpected error: %v", err)
	}
	if stopped.Status != automations.InstanceStatusStopped {
		t.Fatalf("StopSource() status = %q, want %q", stopped.Status, automations.InstanceStatusStopped)
	}

	waited, err := svc.WaitSource(context.Background(), automations.WaitSourceRequest{
		Handle: started.Handle,
	})
	if err != nil {
		t.Fatalf("WaitSource() unexpected error: %v", err)
	}
	if waited.Status != automations.InstanceStatusStopped {
		t.Fatalf("WaitSource() status = %q, want terminal %q", waited.Status, automations.InstanceStatusStopped)
	}
}

func TestServiceSourceLifecycle_FakeTypedMissingSource(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{ready: true}
	_, err := svc.SourceStatus(context.Background(), automations.SourceStatusRequest{
		Handle: automations.SourceHandle{ID: "missing-source"},
	})
	if err == nil {
		t.Fatal("SourceStatus() error = nil, want typed not-found error")
	}
	var typed *automations.Error
	if !errors.As(err, &typed) {
		t.Fatalf("SourceStatus() error type = %T, want *automations.Error", err)
	}
	if typed.Code != automations.ErrorCodeNotFound {
		t.Fatalf("SourceStatus() error code = %q, want %q", typed.Code, automations.ErrorCodeNotFound)
	}
	if !errors.Is(err, automations.ErrNotFound) {
		t.Fatalf("SourceStatus() error = %v, want errors.Is ErrNotFound", err)
	}
}

func TestServiceSourceLifecycle_FakeTypedInvalidLifecycleTransition(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{ready: true}
	started, err := svc.StartSource(context.Background(), automations.StartSourceRequest{
		SourceID: "source-b",
		Kind:     "schedule",
	})
	if err != nil {
		t.Fatalf("StartSource() unexpected error: %v", err)
	}
	_, err = svc.StartSource(context.Background(), automations.StartSourceRequest{
		SourceID: started.Handle.ID,
		Kind:     "schedule",
	})
	if err == nil {
		t.Fatal("StartSource() error = nil, want typed invalid-transition error")
	}
	var typed *automations.Error
	if !errors.As(err, &typed) {
		t.Fatalf("StartSource() error type = %T, want *automations.Error", err)
	}
	if typed.Code != automations.ErrorCodeInvalid {
		t.Fatalf("StartSource() error code = %q, want %q", typed.Code, automations.ErrorCodeInvalid)
	}
	if !errors.Is(err, automations.ErrInvalidRequest) {
		t.Fatalf("StartSource() error = %v, want errors.Is ErrInvalidRequest", err)
	}
}

func TestServiceSourceLifecycle_FakeTypedAlreadyStopped(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{ready: true}
	started, err := svc.StartSource(context.Background(), automations.StartSourceRequest{
		SourceID: "source-c",
		Kind:     "schedule",
	})
	if err != nil {
		t.Fatalf("StartSource() unexpected error: %v", err)
	}
	if _, err := svc.StopSource(context.Background(), automations.StopSourceRequest{
		Handle: started.Handle,
	}); err != nil {
		t.Fatalf("StopSource() unexpected error: %v", err)
	}
	_, err = svc.StopSource(context.Background(), automations.StopSourceRequest{
		Handle: started.Handle,
	})
	if err == nil {
		t.Fatal("StopSource() error = nil, want typed already-stopped conflict")
	}
	var typed *automations.Error
	if !errors.As(err, &typed) {
		t.Fatalf("StopSource() error type = %T, want *automations.Error", err)
	}
	if typed.Code != automations.ErrorCodeConflict {
		t.Fatalf("StopSource() error code = %q, want %q", typed.Code, automations.ErrorCodeConflict)
	}
	if !errors.Is(err, automations.ErrConflict) {
		t.Fatalf("StopSource() error = %v, want errors.Is ErrConflict", err)
	}
}

func TestServiceCursorStatus_FakeSuccessDetachedValues(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{
		ready: true,
		instances: map[string]fakeInstance{
			"inst-a": {
				automationID: "auto-a",
				status:       automations.InstanceStatusRunning,
				cursor:       "cursor-42",
				checkpoint:   "checkpoint-7",
			},
		},
	}

	status, err := svc.GetStatus(context.Background(), automations.GetStatusRequest{
		InstanceID: "inst-a",
	})
	if err != nil {
		t.Fatalf("GetStatus() unexpected error: %v", err)
	}
	if status.AutomationID != "auto-a" {
		t.Fatalf("GetStatus() AutomationID = %q, want %q", status.AutomationID, "auto-a")
	}
	if status.InstanceID != "inst-a" {
		t.Fatalf("GetStatus() InstanceID = %q, want %q", status.InstanceID, "inst-a")
	}
	if status.Status != automations.InstanceStatusRunning {
		t.Fatalf("GetStatus() Status = %q, want %q", status.Status, automations.InstanceStatusRunning)
	}

	cursor, err := svc.GetCursor(context.Background(), automations.GetCursorRequest{
		InstanceID: "inst-a",
	})
	if err != nil {
		t.Fatalf("GetCursor() unexpected error: %v", err)
	}
	if cursor.AutomationID != "auto-a" {
		t.Fatalf("GetCursor() AutomationID = %q, want %q", cursor.AutomationID, "auto-a")
	}
	if cursor.InstanceID != "inst-a" {
		t.Fatalf("GetCursor() InstanceID = %q, want %q", cursor.InstanceID, "inst-a")
	}
	if cursor.Cursor != "cursor-42" {
		t.Fatalf("GetCursor() Cursor = %q, want %q", cursor.Cursor, "cursor-42")
	}
	if cursor.Checkpoint != "checkpoint-7" {
		t.Fatalf("GetCursor() Checkpoint = %q, want %q", cursor.Checkpoint, "checkpoint-7")
	}
}

func TestServiceCursorStatus_FakeTypedMissingInstance(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{ready: true}
	_, err := svc.GetStatus(context.Background(), automations.GetStatusRequest{
		InstanceID: "missing-instance",
	})
	if err == nil {
		t.Fatal("GetStatus() error = nil, want typed not-found error")
	}
	var typed *automations.Error
	if !errors.As(err, &typed) {
		t.Fatalf("GetStatus() error type = %T, want *automations.Error", err)
	}
	if typed.Code != automations.ErrorCodeNotFound {
		t.Fatalf("GetStatus() error code = %q, want %q", typed.Code, automations.ErrorCodeNotFound)
	}
	if !errors.Is(err, automations.ErrNotFound) {
		t.Fatalf("GetStatus() error = %v, want errors.Is ErrNotFound", err)
	}
}

func TestServiceCursorStatus_FakeTypedInvalidStaleCursor(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{
		ready: true,
		instances: map[string]fakeInstance{
			"inst-b": {
				automationID: "auto-b",
				status:       automations.InstanceStatusReady,
				cursor:       "cursor-current",
				checkpoint:   "checkpoint-1",
			},
		},
	}

	_, err := svc.GetCursor(context.Background(), automations.GetCursorRequest{
		InstanceID:     "",
		ExpectedCursor: "cursor-current",
	})
	if err == nil {
		t.Fatal("GetCursor() error = nil, want typed invalid-request error")
	}
	var invalid *automations.Error
	if !errors.As(err, &invalid) {
		t.Fatalf("GetCursor() error type = %T, want *automations.Error", err)
	}
	if invalid.Code != automations.ErrorCodeInvalid {
		t.Fatalf("GetCursor() error code = %q, want %q", invalid.Code, automations.ErrorCodeInvalid)
	}
	if !errors.Is(err, automations.ErrInvalidRequest) {
		t.Fatalf("GetCursor() error = %v, want errors.Is ErrInvalidRequest", err)
	}

	_, err = svc.GetCursor(context.Background(), automations.GetCursorRequest{
		InstanceID:     "inst-b",
		ExpectedCursor: "cursor-stale",
	})
	if err == nil {
		t.Fatal("GetCursor() error = nil, want typed stale-cursor conflict")
	}
	var stale *automations.Error
	if !errors.As(err, &stale) {
		t.Fatalf("GetCursor() error type = %T, want *automations.Error", err)
	}
	if stale.Code != automations.ErrorCodeConflict {
		t.Fatalf("GetCursor() error code = %q, want %q", stale.Code, automations.ErrorCodeConflict)
	}
	if !errors.Is(err, automations.ErrConflict) {
		t.Fatalf("GetCursor() error = %v, want errors.Is ErrConflict", err)
	}
}
