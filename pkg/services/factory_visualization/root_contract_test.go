package factory_visualization_test

import (
	"context"
	"errors"
	"testing"
	"time"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

// fakeRootPeer is a peer-shaped Root implementer that uses only the published
// Factory Visualization root package. It stays inert until Activate.
type fakeRootPeer struct {
	state      factoryvisualization.LifecycleState
	subscribed bool
	presented  bool

	observeView          factoryvisualization.ProjectedView
	observeSnapshotOK    bool
	observeReconstructOK bool
}

func (f *fakeRootPeer) Observe(
	ctx context.Context,
	req factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	if req.Mode == "" {
		return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
			Kind:    factoryvisualization.ProjectionErrorInvalidInput,
			Message: "observe Factory visualization: required request parameters are missing",
		}
	}
	if req.Mode != factoryvisualization.ObserveModeRetainedThenLive {
		return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
			Kind:    factoryvisualization.ProjectionErrorInvalidInput,
			Message: "observe Factory visualization: observe mode is not supported",
		}
	}
	if req.Reconnect != nil {
		if req.Reconnect.AfterSequence != nil && *req.Reconnect.AfterSequence < 0 {
			return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
				Kind:    factoryvisualization.ProjectionErrorInvalidInput,
				Message: "observe Factory visualization: reconnect after_sequence is invalid",
			}
		}
		if req.Reconnect.AfterEventID == "" && req.Reconnect.AfterSequence == nil {
			return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
				Kind:    factoryvisualization.ProjectionErrorInvalidInput,
				Message: "observe Factory visualization: reconnect cursor is empty",
			}
		}
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return factoryvisualization.ObserveResult{}, err
		}
	}
	if !f.observeSnapshotOK {
		return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
			Kind:    factoryvisualization.ProjectionErrorSnapshotUnavailable,
			Message: "observe Factory visualization: snapshot is unavailable",
		}
	}
	if !f.observeReconstructOK {
		return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
			Kind:    factoryvisualization.ProjectionErrorReconstructionFailed,
			Message: "observe Factory visualization: projection reconstruction failed",
		}
	}
	return factoryvisualization.ObserveResult{View: f.observeView}, nil
}

func (f *fakeRootPeer) Activate(
	ctx context.Context,
	req factoryvisualization.ActivateRequest,
) (factoryvisualization.ActivateResult, error) {
	if req.Mode == "" {
		return factoryvisualization.ActivateResult{}, &factoryvisualization.LifecycleError{
			Kind:    factoryvisualization.LifecycleErrorMissingParameters,
			Message: "activate Factory visualization: required request parameters are missing",
		}
	}
	if ctx == nil {
		return factoryvisualization.ActivateResult{}, &factoryvisualization.LifecycleError{
			Kind:    factoryvisualization.LifecycleErrorMissingParameters,
			Message: "activate Factory visualization: context is required",
		}
	}
	if err := ctx.Err(); err != nil {
		return factoryvisualization.ActivateResult{}, err
	}
	if f.state == factoryvisualization.LifecycleStateStarted {
		return factoryvisualization.ActivateResult{}, &factoryvisualization.LifecycleError{
			Kind:    factoryvisualization.LifecycleErrorAlreadyActivated,
			Message: "activate Factory visualization: already activated",
		}
	}
	f.subscribed = true
	f.presented = true
	f.state = factoryvisualization.LifecycleStateStarted
	return factoryvisualization.ActivateResult{State: f.state}, nil
}

func (f *fakeRootPeer) Join(
	ctx context.Context,
	_ factoryvisualization.JoinRequest,
) (factoryvisualization.JoinResult, error) {
	if f.state != factoryvisualization.LifecycleStateStarted {
		return factoryvisualization.JoinResult{}, &factoryvisualization.LifecycleError{
			Kind:    factoryvisualization.LifecycleErrorNotActivated,
			Message: "join Factory visualization: not activated",
		}
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return factoryvisualization.JoinResult{}, err
		}
	}
	return factoryvisualization.JoinResult{State: f.state}, nil
}

func (f *fakeRootPeer) StopDrain(
	_ context.Context,
	_ factoryvisualization.StopDrainRequest,
) (factoryvisualization.StopDrainResult, error) {
	f.subscribed = false
	f.state = factoryvisualization.LifecycleStateStopped
	return factoryvisualization.StopDrainResult{State: f.state}, nil
}

func TestRootContractFakePeerInertNotActivated(t *testing.T) {
	t.Parallel()

	peer := &fakeRootPeer{state: factoryvisualization.LifecycleStateInert}
	var root factoryvisualization.Root = peer

	_, err := root.Join(context.Background(), factoryvisualization.JoinRequest{})
	if err == nil {
		t.Fatal("Join on inert Root: error = nil, want not-activated failure")
	}
	var lifeErr *factoryvisualization.LifecycleError
	if !errors.As(err, &lifeErr) {
		t.Fatalf("Join on inert Root: error = %T (%v), want *LifecycleError", err, err)
	}
	if lifeErr.Kind != factoryvisualization.LifecycleErrorNotActivated {
		t.Fatalf("Join on inert Root: kind = %q, want %q", lifeErr.Kind, factoryvisualization.LifecycleErrorNotActivated)
	}
	if peer.subscribed || peer.presented {
		t.Fatal("inert Root peer must not subscribe or present after Join")
	}
	if peer.state != factoryvisualization.LifecycleStateInert {
		t.Fatalf("inert Root peer state = %q, want %q", peer.state, factoryvisualization.LifecycleStateInert)
	}
}

func TestRootLifecycleActivateSuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	peer := &fakeRootPeer{state: factoryvisualization.LifecycleStateInert}
	var root factoryvisualization.Root = peer

	if peer.subscribed || peer.presented {
		t.Fatal("constructed Root peer must remain inert before Activate")
	}

	_, err := root.Activate(context.Background(), factoryvisualization.ActivateRequest{})
	var lifeErr *factoryvisualization.LifecycleError
	if !errors.As(err, &lifeErr) || lifeErr.Kind != factoryvisualization.LifecycleErrorMissingParameters {
		t.Fatalf("Activate missing parameters: error = %v, want MissingParameters", err)
	}
	if peer.subscribed || peer.presented || peer.state != factoryvisualization.LifecycleStateInert {
		t.Fatal("missing-parameter Activate must leave peer inert")
	}

	result, err := root.Activate(context.Background(), factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Activate explicit request: error = %v", err)
	}
	if result.State != factoryvisualization.LifecycleStateStarted {
		t.Fatalf("Activate result state = %q, want %q", result.State, factoryvisualization.LifecycleStateStarted)
	}
	if !peer.subscribed || !peer.presented {
		t.Fatal("successful Activate must reach started/running side effects through published vocabulary")
	}

	_, err = root.Activate(context.Background(), factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateModeRetainedThenLive,
	})
	if !errors.As(err, &lifeErr) || lifeErr.Kind != factoryvisualization.LifecycleErrorAlreadyActivated {
		t.Fatalf("Activate already activated: error = %v, want AlreadyActivated", err)
	}

	joinResult, err := root.Join(context.Background(), factoryvisualization.JoinRequest{})
	if err != nil {
		t.Fatalf("Join after Activate: error = %v", err)
	}
	if joinResult.State != factoryvisualization.LifecycleStateStarted {
		t.Fatalf("Join result state = %q, want %q", joinResult.State, factoryvisualization.LifecycleStateStarted)
	}

	stopResult, err := root.StopDrain(context.Background(), factoryvisualization.StopDrainRequest{})
	if err != nil {
		t.Fatalf("StopDrain: error = %v", err)
	}
	if stopResult.State != factoryvisualization.LifecycleStateStopped {
		t.Fatalf("StopDrain result state = %q, want %q", stopResult.State, factoryvisualization.LifecycleStateStopped)
	}
}

func TestConcreteServiceImplementsRoot(t *testing.T) {
	t.Parallel()

	// Compile-time reachability: existing lifecycle Service remains the Root
	// implementer so activation stays on the singular peer-facing seam.
	var _ factoryvisualization.Root = (*factoryvisualization.Service)(nil)
}

func TestRootLiveProjectionSuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, time.July, 23, 20, 0, 0, 0, time.UTC)
	peer := &fakeRootPeer{
		state:                factoryvisualization.LifecycleStateInert,
		observeSnapshotOK:    true,
		observeReconstructOK: true,
		observeView: factoryvisualization.ProjectedView{
			TickCount:          7,
			RetainedEventCount: 2,
			ObservedAt:         observedAt,
		},
	}
	var root factoryvisualization.Root = peer

	_, err := root.Observe(context.Background(), factoryvisualization.ObserveRequest{})
	var projErr *factoryvisualization.ProjectionError
	if !errors.As(err, &projErr) || projErr.Kind != factoryvisualization.ProjectionErrorInvalidInput {
		t.Fatalf("Observe missing parameters: error = %v, want InvalidInput", err)
	}

	neg := -1
	_, err = root.Observe(context.Background(), factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
		Reconnect: &factoryvisualization.ObserveReconnectCursor{
			AfterSequence: &neg,
		},
	})
	if !errors.As(err, &projErr) || projErr.Kind != factoryvisualization.ProjectionErrorInvalidInput {
		t.Fatalf("Observe invalid reconnect: error = %v, want InvalidInput", err)
	}

	peer.observeSnapshotOK = false
	_, err = root.Observe(context.Background(), factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
	})
	if !errors.As(err, &projErr) || projErr.Kind != factoryvisualization.ProjectionErrorSnapshotUnavailable {
		t.Fatalf("Observe unavailable snapshot: error = %v, want SnapshotUnavailable", err)
	}

	peer.observeSnapshotOK = true
	peer.observeReconstructOK = false
	_, err = root.Observe(context.Background(), factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
	})
	if !errors.As(err, &projErr) || projErr.Kind != factoryvisualization.ProjectionErrorReconstructionFailed {
		t.Fatalf("Observe reconstruction failure: error = %v, want ReconstructionFailed", err)
	}

	peer.observeReconstructOK = true
	result, err := root.Observe(context.Background(), factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Observe valid path: error = %v", err)
	}
	if result.View.TickCount != 7 {
		t.Fatalf("Observe TickCount = %d, want 7", result.View.TickCount)
	}
	if result.View.RetainedEventCount != 2 {
		t.Fatalf("Observe RetainedEventCount = %d, want 2", result.View.RetainedEventCount)
	}
	if !result.View.ObservedAt.Equal(observedAt) {
		t.Fatalf("Observe ObservedAt = %v, want %v", result.View.ObservedAt, observedAt)
	}
	if peer.subscribed || peer.presented {
		t.Fatal("Observe must not imply Activate subscription/presentation side effects on inert peer")
	}
}
