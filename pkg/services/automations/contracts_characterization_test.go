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
