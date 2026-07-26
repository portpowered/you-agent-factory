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
	sources             map[string]automations.SourceObservation
	instances           map[string]fakeInstance
	admittedRequests    map[string]automations.GeneratedWorkRequest
	workRequestOutcomes []automations.GeneratedWorkRequestOutcome
	emissionCount       int
}

type fakeInstance struct {
	automationID string
	status       automations.ObservedLifecycleState
	cursor       automations.Cursor
	checkpoint   string
}

func (f *fakeRootService) ensureSources() {
	if f.sources == nil {
		f.sources = make(map[string]automations.SourceObservation)
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

func (f *fakeRootService) Reconcile(ctx context.Context, req automations.ReconcileRequest) (automations.ReconcileResult, error) {
	if f == nil || !f.ready {
		return automations.ReconcileResult{}, &automations.Error{
			Op:   "Reconcile",
			Code: automations.ErrorCodeNotReady,
			Err:  automations.ErrNotReady,
		}
	}
	if err := ctx.Err(); err != nil {
		return cancelledReconcileResult(req), fakeError(
			"Reconcile", automations.ErrorCodeCancelled, err,
		)
	}
	if len(req.Desired) == 0 {
		return automations.ReconcileResult{}, &automations.Error{
			Op:   "Reconcile",
			Code: automations.ErrorCodeInvalid,
			Err:  automations.ErrInvalidRequest,
		}
	}
	if !validDesiredSpecs(req.Desired) {
		return automations.ReconcileResult{}, &automations.Error{
			Op:   "Reconcile",
			Code: automations.ErrorCodeInvalid,
			Err:  automations.ErrInvalidRequest,
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
	observedBySource := make(map[string]automations.ObservedInstance, len(req.Observed))
	for _, observed := range req.Observed {
		observedBySource[observed.AutomationID+"\x00"+observed.SourceID] = observed
	}
	for _, spec := range req.Desired {
		observed, exists := observedBySource[spec.AutomationID+"\x00"+spec.SourceID]
		outcomes = append(outcomes, fakeConvergenceOutcome(spec, observed, exists))
	}
	return automations.ReconcileResult{
		Outcomes:              outcomes,
		GeneratedWorkRequests: cloneWorkRequestOutcomes(f.workRequestOutcomes),
	}, nil
}

func validDesiredSpecs(specs []automations.DesiredSpec) bool {
	for _, spec := range specs {
		if spec.AutomationID == "" || spec.SourceID == "" {
			return false
		}
		if spec.State != automations.DesiredLifecycleRunning &&
			spec.State != automations.DesiredLifecycleStopped {
			return false
		}
	}
	return true
}

func fakeConvergenceOutcome(
	spec automations.DesiredSpec,
	observed automations.ObservedInstance,
	exists bool,
) automations.ConvergenceOutcome {
	outcome := automations.ConvergenceOutcome{
		AutomationID: spec.AutomationID,
		SourceID:     spec.SourceID,
		InstanceID:   "instance:" + spec.AutomationID + ":" + spec.SourceID,
		Action:       automations.ConvergenceActionCreated,
		Desired:      spec.State,
		Observed:     automations.ObservedLifecyclePending,
		Convergence:  automations.ConvergenceStatusProgressing,
	}
	if !exists {
		return outcome
	}

	outcome.InstanceID = observed.InstanceID
	outcome.Observed = observed.State
	outcome.Action = automations.ConvergenceActionUpdated
	switch {
	case observed.State == automations.ObservedLifecycleFailed:
		outcome.Convergence = automations.ConvergenceStatusFailed
	case desiredMatchesObserved(spec.State, observed.State):
		outcome.Action = automations.ConvergenceActionUnchanged
		outcome.Convergence = automations.ConvergenceStatusConverged
	}
	return outcome
}

func desiredMatchesObserved(
	desired automations.DesiredLifecycleState,
	observed automations.ObservedLifecycleState,
) bool {
	return desired == automations.DesiredLifecycleRunning &&
		observed == automations.ObservedLifecycleRunning ||
		desired == automations.DesiredLifecycleStopped &&
			observed == automations.ObservedLifecycleStopped
}

func (f *fakeRootService) StartSource(
	ctx context.Context,
	req automations.StartSourceRequest,
) (automations.StartSourceResult, error) {
	if f == nil || !f.ready {
		return automations.StartSourceResult{}, fakeError(
			"StartSource", automations.ErrorCodeNotReady, automations.ErrNotReady,
		)
	}
	if err := ctx.Err(); err != nil {
		return automations.StartSourceResult{
			Outcome: cancelledLifecycleOutcome(
				automations.DesiredLifecycleRunning,
				cancelledObservation(req.Identity, nil),
			),
		}, fakeError("StartSource", automations.ErrorCodeCancelled, err)
	}
	if !validSourceIdentity(req.Identity) || req.Kind == "" || !validResume(req) {
		return automations.StartSourceResult{}, fakeError(
			"StartSource", automations.ErrorCodeInvalid, automations.ErrInvalidRequest,
		)
	}
	f.ensureSources()
	key := sourceKey(req.Identity)
	observation, exists := f.sources[key]
	if !exists && req.Resume != nil {
		observation = *req.Resume
		exists = true
	}
	if exists && observation.State == automations.ObservedLifecycleRunning {
		f.sources[key] = observation
		return automations.StartSourceResult{
			Outcome: lifecycleOutcome(
				automations.DesiredLifecycleRunning,
				observation,
				automations.ConvergenceStatusConverged,
				true,
			),
		}, nil
	}
	if !exists {
		observation = automations.SourceObservation{
			Identity:   req.Identity,
			InstanceID: "instance:" + req.Identity.AutomationID + ":" + req.Identity.SourceID,
		}
	}
	observation.State = automations.ObservedLifecycleStarting
	f.sources[key] = observation
	return automations.StartSourceResult{
		Outcome: lifecycleOutcome(
			automations.DesiredLifecycleRunning,
			observation,
			automations.ConvergenceStatusProgressing,
			false,
		),
	}, nil
}

func (f *fakeRootService) StopSource(
	ctx context.Context,
	req automations.StopSourceRequest,
) (automations.StopSourceResult, error) {
	if f == nil || !f.ready {
		return automations.StopSourceResult{}, fakeError(
			"StopSource", automations.ErrorCodeNotReady, automations.ErrNotReady,
		)
	}
	if err := ctx.Err(); err != nil {
		return automations.StopSourceResult{
			Outcome: cancelledLifecycleOutcome(
				automations.DesiredLifecycleStopped,
				cancelledObservation(req.Identity, f.sourceObservation(req.Identity)),
			),
		}, fakeError("StopSource", automations.ErrorCodeCancelled, err)
	}
	if !validSourceIdentity(req.Identity) {
		return automations.StopSourceResult{}, fakeError(
			"StopSource", automations.ErrorCodeInvalid, automations.ErrInvalidRequest,
		)
	}
	f.ensureSources()
	key := sourceKey(req.Identity)
	observation, ok := f.sources[key]
	if !ok {
		return automations.StopSourceResult{}, fakeError(
			"StopSource", automations.ErrorCodeNotFound, automations.ErrNotFound,
		)
	}
	if observation.State == automations.ObservedLifecycleStopped {
		return automations.StopSourceResult{
			Outcome: lifecycleOutcome(
				automations.DesiredLifecycleStopped,
				observation,
				automations.ConvergenceStatusConverged,
				true,
			),
		}, nil
	}
	observation.State = automations.ObservedLifecycleStopping
	f.sources[key] = observation
	return automations.StopSourceResult{
		Outcome: lifecycleOutcome(
			automations.DesiredLifecycleStopped,
			observation,
			automations.ConvergenceStatusProgressing,
			false,
		),
	}, nil
}

func (f *fakeRootService) WaitSource(
	ctx context.Context,
	req automations.WaitSourceRequest,
) (automations.WaitSourceResult, error) {
	if f == nil || !f.ready {
		return automations.WaitSourceResult{}, fakeError(
			"WaitSource", automations.ErrorCodeNotReady, automations.ErrNotReady,
		)
	}
	if err := ctx.Err(); err != nil {
		return automations.WaitSourceResult{
			Outcome: cancelledLifecycleOutcome(
				req.Desired,
				cancelledObservation(req.Identity, f.sourceObservation(req.Identity)),
			),
		}, fakeError("WaitSource", automations.ErrorCodeCancelled, err)
	}
	if !validSourceIdentity(req.Identity) || !validDesiredState(req.Desired) {
		return automations.WaitSourceResult{}, fakeError(
			"WaitSource", automations.ErrorCodeInvalid, automations.ErrInvalidRequest,
		)
	}
	f.ensureSources()
	key := sourceKey(req.Identity)
	observation, ok := f.sources[key]
	if !ok {
		return automations.WaitSourceResult{}, fakeError(
			"WaitSource", automations.ErrorCodeNotFound, automations.ErrNotFound,
		)
	}
	observation = advanceObservation(observation)
	f.sources[key] = observation
	convergence := automations.ConvergenceStatusProgressing
	if desiredMatchesObserved(req.Desired, observation.State) {
		convergence = automations.ConvergenceStatusConverged
	}
	return automations.WaitSourceResult{
		Outcome: lifecycleOutcome(req.Desired, observation, convergence, false),
	}, nil
}

func (f *fakeRootService) SourceStatus(
	_ context.Context,
	req automations.SourceStatusRequest,
) (automations.SourceStatusResult, error) {
	if f == nil || !f.ready {
		return automations.SourceStatusResult{}, fakeError(
			"SourceStatus", automations.ErrorCodeNotReady, automations.ErrNotReady,
		)
	}
	if !validSourceIdentity(req.Identity) {
		return automations.SourceStatusResult{}, fakeError(
			"SourceStatus", automations.ErrorCodeInvalid, automations.ErrInvalidRequest,
		)
	}
	f.ensureSources()
	key := sourceKey(req.Identity)
	observation, ok := f.sources[key]
	if !ok {
		return automations.SourceStatusResult{}, fakeError(
			"SourceStatus", automations.ErrorCodeNotFound, automations.ErrNotFound,
		)
	}
	observation = advanceObservation(observation)
	f.sources[key] = observation
	return automations.SourceStatusResult{Observation: observation}, nil
}

func validSourceIdentity(identity automations.SourceIdentity) bool {
	return identity.AutomationID != "" && identity.SourceID != ""
}

func validResume(req automations.StartSourceRequest) bool {
	return req.Resume == nil || req.Resume.Identity == req.Identity
}

func validDesiredState(state automations.DesiredLifecycleState) bool {
	return state == automations.DesiredLifecycleRunning ||
		state == automations.DesiredLifecycleStopped
}

func sourceKey(identity automations.SourceIdentity) string {
	return identity.AutomationID + "\x00" + identity.SourceID
}

func advanceObservation(observation automations.SourceObservation) automations.SourceObservation {
	switch observation.State {
	case automations.ObservedLifecycleStarting:
		observation.State = automations.ObservedLifecycleRunning
	case automations.ObservedLifecycleStopping:
		observation.State = automations.ObservedLifecycleStopped
	}
	return observation
}

func lifecycleOutcome(
	desired automations.DesiredLifecycleState,
	observation automations.SourceObservation,
	convergence automations.ConvergenceStatus,
	idempotent bool,
) automations.LifecycleOutcome {
	return automations.LifecycleOutcome{
		Desired:     desired,
		Observation: observation,
		Convergence: convergence,
		Idempotent:  idempotent,
	}
}

func cancelledReconcileResult(req automations.ReconcileRequest) automations.ReconcileResult {
	outcomes := make([]automations.ConvergenceOutcome, 0, len(req.Desired))
	for _, desired := range req.Desired {
		outcomes = append(outcomes, automations.ConvergenceOutcome{
			AutomationID: desired.AutomationID,
			SourceID:     desired.SourceID,
			InstanceID:   "instance:" + desired.AutomationID + ":" + desired.SourceID,
			Desired:      desired.State,
			Observed:     automations.ObservedLifecycleCancelled,
			Convergence:  automations.ConvergenceStatusCancelled,
		})
	}
	return automations.ReconcileResult{Outcomes: outcomes}
}

func (f *fakeRootService) sourceObservation(
	identity automations.SourceIdentity,
) *automations.SourceObservation {
	if f == nil || f.sources == nil {
		return nil
	}
	observation, ok := f.sources[sourceKey(identity)]
	if !ok {
		return nil
	}
	return &observation
}

func cancelledObservation(
	identity automations.SourceIdentity,
	current *automations.SourceObservation,
) automations.SourceObservation {
	observation := automations.SourceObservation{
		Identity:   identity,
		InstanceID: "instance:" + identity.AutomationID + ":" + identity.SourceID,
	}
	if current != nil {
		observation = *current
	}
	observation.State = automations.ObservedLifecycleCancelled
	return observation
}

func cancelledLifecycleOutcome(
	desired automations.DesiredLifecycleState,
	observation automations.SourceObservation,
) automations.LifecycleOutcome {
	return lifecycleOutcome(
		desired,
		observation,
		automations.ConvergenceStatusCancelled,
		false,
	)
}

func fakeError(op string, code automations.ErrorCode, sentinel error) *automations.Error {
	return &automations.Error{Op: op, Code: code, Err: sentinel}
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
			{
				AutomationID: "auto-a",
				SourceID:     "source-a",
				Kind:         "schedule",
				State:        automations.DesiredLifecycleRunning,
			},
		},
		Observed: []automations.ObservedInstance{
			{
				AutomationID: "auto-a",
				SourceID:     "source-a",
				InstanceID:   "inst-a",
				State:        automations.ObservedLifecycleRunning,
			},
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
	if outcome.SourceID != "source-a" {
		t.Fatalf("Reconcile() outcome.SourceID = %q, want %q", outcome.SourceID, "source-a")
	}
	if outcome.Action != automations.ConvergenceActionUnchanged {
		t.Fatalf("Reconcile() outcome.Action = %q, want %q", outcome.Action, automations.ConvergenceActionUnchanged)
	}
	if outcome.Desired != automations.DesiredLifecycleRunning {
		t.Fatalf("Reconcile() outcome.Desired = %q, want %q", outcome.Desired, automations.DesiredLifecycleRunning)
	}
	if outcome.Observed != automations.ObservedLifecycleRunning {
		t.Fatalf("Reconcile() outcome.Observed = %q, want %q", outcome.Observed, automations.ObservedLifecycleRunning)
	}
	if outcome.Convergence != automations.ConvergenceStatusConverged {
		t.Fatalf("Reconcile() outcome.Convergence = %q, want %q", outcome.Convergence, automations.ConvergenceStatusConverged)
	}
}

func TestServiceReconcile_FakeTypedConvergenceAndIdempotentIdentity(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{ready: true}
	req := automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{
			{
				AutomationID: "auto-a",
				SourceID:     "source-progressing",
				Kind:         "schedule",
				State:        automations.DesiredLifecycleRunning,
			},
			{
				AutomationID: "auto-a",
				SourceID:     "source-failed",
				Kind:         "events",
				State:        automations.DesiredLifecycleRunning,
			},
		},
		Observed: []automations.ObservedInstance{
			{
				AutomationID: "auto-a",
				SourceID:     "source-progressing",
				InstanceID:   "inst-progressing",
				State:        automations.ObservedLifecycleStarting,
			},
			{
				AutomationID: "auto-a",
				SourceID:     "source-failed",
				InstanceID:   "inst-failed",
				State:        automations.ObservedLifecycleFailed,
			},
		},
	}

	first, err := svc.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("first Reconcile() unexpected error: %v", err)
	}
	second, err := svc.Reconcile(context.Background(), req)
	if err != nil {
		t.Fatalf("second Reconcile() unexpected error: %v", err)
	}
	if first.Outcomes[0].Convergence != automations.ConvergenceStatusProgressing {
		t.Fatalf("progressing convergence = %q, want %q", first.Outcomes[0].Convergence, automations.ConvergenceStatusProgressing)
	}
	if first.Outcomes[1].Convergence != automations.ConvergenceStatusFailed {
		t.Fatalf("failed convergence = %q, want %q", first.Outcomes[1].Convergence, automations.ConvergenceStatusFailed)
	}
	for i := range first.Outcomes {
		if second.Outcomes[i].AutomationID != first.Outcomes[i].AutomationID ||
			second.Outcomes[i].SourceID != first.Outcomes[i].SourceID ||
			second.Outcomes[i].InstanceID != first.Outcomes[i].InstanceID {
			t.Fatalf("repeated Reconcile() outcome %d identity = %#v, want %#v", i, second.Outcomes[i], first.Outcomes[i])
		}
	}
}

func TestServiceReconcile_FakeTypedInvalidDesiredSpecs(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{ready: true}
	_, err := svc.Reconcile(context.Background(), automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{{
			AutomationID: "",
			SourceID:     "source-a",
			Kind:         "schedule",
			State:        automations.DesiredLifecycleRunning,
		}},
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
			{
				AutomationID: "auto-a",
				SourceID:     "source-a",
				Kind:         "schedule",
				State:        automations.DesiredLifecycleRunning,
			},
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

func TestServiceCursorStatus_FakeSuccessDetachedValues(t *testing.T) {
	t.Parallel()

	var svc automations.Service = &fakeRootService{
		ready: true,
		instances: map[string]fakeInstance{
			"inst-a": {
				automationID: "auto-a",
				status:       automations.ObservedLifecycleRunning,
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
	if status.Status != automations.ObservedLifecycleRunning {
		t.Fatalf("GetStatus() Status = %q, want %q", status.Status, automations.ObservedLifecycleRunning)
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
				status:       automations.ObservedLifecycleRunning,
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

// peerConsumePublishedSlices is a peer-shaped consumer of the sealed Automations
// root. It depends only on automations.Service plain contracts (this file's
// imports are stdlib + pkg/services/automations) and never needs Automations
// implementation packages or cron/poller/watcher types.
func peerConsumePublishedSlices(ctx context.Context, svc automations.Service) error {
	if _, err := svc.Ready(ctx, automations.ReadyRequest{}); err != nil {
		return err
	}
	if _, err := svc.Reconcile(ctx, automations.ReconcileRequest{
		Desired: []automations.DesiredSpec{
			{
				AutomationID: "auto-seal",
				SourceID:     "source-seal",
				Kind:         "schedule",
				State:        automations.DesiredLifecycleRunning,
			},
		},
		Observed: []automations.ObservedInstance{
			{
				AutomationID: "auto-seal",
				SourceID:     "source-seal",
				InstanceID:   "inst-seal",
				State:        automations.ObservedLifecycleRunning,
			},
		},
	}); err != nil {
		return err
	}
	identity := automations.SourceIdentity{AutomationID: "auto-seal", SourceID: "source-seal"}
	_, err := svc.StartSource(ctx, automations.StartSourceRequest{
		Identity: identity,
		Kind:     "schedule",
	})
	if err != nil {
		return err
	}
	if _, err := svc.SourceStatus(ctx, automations.SourceStatusRequest{Identity: identity}); err != nil {
		return err
	}
	if _, err := svc.StopSource(ctx, automations.StopSourceRequest{Identity: identity}); err != nil {
		return err
	}
	if _, err := svc.WaitSource(ctx, automations.WaitSourceRequest{
		Identity: identity,
		Desired:  automations.DesiredLifecycleStopped,
	}); err != nil {
		return err
	}
	if _, err := svc.GetStatus(ctx, automations.GetStatusRequest{InstanceID: "inst-seal"}); err != nil {
		return err
	}
	if _, err := svc.GetCursor(ctx, automations.GetCursorRequest{InstanceID: "inst-seal"}); err != nil {
		return err
	}
	return nil
}

func TestServiceRootSealed_AllPublishedSlicesReachableThroughOneService(t *testing.T) {
	t.Parallel()

	svc := &fakeRootService{
		ready: true,
		instances: map[string]fakeInstance{
			"inst-seal": {
				automationID: "auto-seal",
				status:       automations.ObservedLifecycleRunning,
				cursor:       "cursor-seal",
				checkpoint:   "checkpoint-seal",
			},
		},
	}
	var root automations.Service = svc

	if err := peerConsumePublishedSlices(context.Background(), root); err != nil {
		t.Fatalf("peerConsumePublishedSlices() unexpected error: %v", err)
	}

	// Representative typed failures remain distinguishable on the same singular root.
	_, reconcileErr := root.Reconcile(context.Background(), automations.ReconcileRequest{})
	assertTypedAutomationsError(t, "Reconcile", reconcileErr, automations.ErrorCodeInvalid, automations.ErrInvalidRequest)

	_, missingSourceErr := root.SourceStatus(context.Background(), automations.SourceStatusRequest{
		Identity: automations.SourceIdentity{
			AutomationID: "auto-seal",
			SourceID:     "missing-seal-source",
		},
	})
	assertTypedAutomationsError(t, "SourceStatus", missingSourceErr, automations.ErrorCodeNotFound, automations.ErrNotFound)

	_, missingInstanceErr := root.GetStatus(context.Background(), automations.GetStatusRequest{
		InstanceID: "missing-seal-instance",
	})
	assertTypedAutomationsError(t, "GetStatus", missingInstanceErr, automations.ErrorCodeNotFound, automations.ErrNotFound)

	_, staleCursorErr := root.GetCursor(context.Background(), automations.GetCursorRequest{
		InstanceID:     "inst-seal",
		ExpectedCursor: "cursor-stale",
	})
	assertTypedAutomationsError(t, "GetCursor", staleCursorErr, automations.ErrorCodeConflict, automations.ErrConflict)
}

func assertTypedAutomationsError(t *testing.T, op string, err error, code automations.ErrorCode, sentinel error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s() error = nil, want typed Automations error", op)
	}
	var typed *automations.Error
	if !errors.As(err, &typed) {
		t.Fatalf("%s() error type = %T, want *automations.Error", op, err)
	}
	if typed.Code != code {
		t.Fatalf("%s() error code = %q, want %q", op, typed.Code, code)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("%s() error = %v, want errors.Is %v", op, err, sentinel)
	}
}
