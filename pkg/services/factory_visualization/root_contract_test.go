package factory_visualization_test

import (
	"context"
	"errors"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

// fakeRootPeer is a peer-shaped Root implementer that uses only the published
// Factory Visualization root package. It stays inert until Activate.
type fakeRootPeer struct {
	state      factoryvisualization.LifecycleState
	subscribed bool
	presented  bool
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
