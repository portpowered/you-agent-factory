package factory_visualization_test

import (
	"context"
	"errors"
	"strconv"
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

	nextPresentationID int
	presentations      map[factoryvisualization.PresentationSessionID]*fakePresentationSession
}

type fakePresentationSession struct {
	mode         factoryvisualization.PresentationDeliveryMode
	records      [][]byte
	closed       bool
	finalized    bool
	progressSeen bool
	capacity     int
	queued       int
}

func (f *fakeRootPeer) presentation(
	id factoryvisualization.PresentationSessionID,
) (*fakePresentationSession, error) {
	if f.presentations == nil {
		return nil, &factoryvisualization.PresentationError{
			Kind:    factoryvisualization.PresentationErrorInvalidInput,
			Message: "presentation session is unknown",
		}
	}
	session, ok := f.presentations[id]
	if !ok {
		return nil, &factoryvisualization.PresentationError{
			Kind:    factoryvisualization.PresentationErrorInvalidInput,
			Message: "presentation session is unknown",
		}
	}
	return session, nil
}

func (f *fakeRootPeer) OpenPresentation(
	_ context.Context,
	req factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	if req.Mode == "" {
		return factoryvisualization.OpenPresentationResult{}, &factoryvisualization.PresentationError{
			Kind:    factoryvisualization.PresentationErrorInvalidInput,
			Message: "open presentation: required request parameters are missing",
		}
	}
	if req.Mode != factoryvisualization.PresentationDeliveryBestEffort &&
		req.Mode != factoryvisualization.PresentationDeliveryLossless {
		return factoryvisualization.OpenPresentationResult{}, &factoryvisualization.PresentationError{
			Kind:    factoryvisualization.PresentationErrorInvalidInput,
			Message: "open presentation: delivery mode is not supported",
		}
	}
	if f.presentations == nil {
		f.presentations = map[factoryvisualization.PresentationSessionID]*fakePresentationSession{}
	}
	f.nextPresentationID++
	id := factoryvisualization.PresentationSessionID(
		"peer-presentation-" + strconv.Itoa(f.nextPresentationID),
	)
	capacity := 2
	if req.Mode == factoryvisualization.PresentationDeliveryLossless {
		capacity = 0
	}
	f.presentations[id] = &fakePresentationSession{
		mode:     req.Mode,
		capacity: capacity,
	}
	return factoryvisualization.OpenPresentationResult{SessionID: id, Mode: req.Mode}, nil
}

func (f *fakeRootPeer) PresentProgress(
	_ context.Context,
	req factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	session, err := f.presentation(req.SessionID)
	if err != nil {
		return factoryvisualization.PresentProgressResult{}, err
	}
	if session.closed || session.finalized {
		return factoryvisualization.PresentProgressResult{}, &factoryvisualization.PresentationError{
			Kind:    factoryvisualization.PresentationErrorEnqueueAfterClose,
			Message: "present progress: presentation output is closed",
		}
	}
	accepted := 0
	for _, record := range req.Records {
		if session.mode == factoryvisualization.PresentationDeliveryBestEffort &&
			session.capacity > 0 && session.queued >= session.capacity {
			return factoryvisualization.PresentProgressResult{AcceptedCount: accepted}, &factoryvisualization.PresentationError{
				Kind:    factoryvisualization.PresentationErrorBackpressureRejected,
				Message: "present progress: best-effort backlog rejected record",
			}
		}
		payload := append([]byte(nil), record.Payload...)
		session.records = append(session.records, payload)
		session.queued++
		session.progressSeen = true
		accepted++
	}
	return factoryvisualization.PresentProgressResult{AcceptedCount: accepted}, nil
}

func (f *fakeRootPeer) FinalizePresentation(
	_ context.Context,
	req factoryvisualization.FinalizePresentationRequest,
) (factoryvisualization.FinalizePresentationResult, error) {
	session, err := f.presentation(req.SessionID)
	if err != nil {
		return factoryvisualization.FinalizePresentationResult{}, err
	}
	if session.finalized {
		return factoryvisualization.FinalizePresentationResult{
			Finalized:    false,
			ProgressSeen: session.progressSeen,
		}, nil
	}
	if req.Terminal == nil {
		session.finalized = true
		session.closed = true
		return factoryvisualization.FinalizePresentationResult{}, &factoryvisualization.PresentationError{
			Kind:    factoryvisualization.PresentationErrorFinalizeWithoutWriter,
			Message: "finalize presentation: terminal writer is required",
		}
	}
	session.finalized = true
	session.closed = true
	session.records = append(session.records, append([]byte(nil), req.Terminal.Payload...))
	return factoryvisualization.FinalizePresentationResult{
		Finalized:    true,
		ProgressSeen: session.progressSeen,
	}, nil
}

func (f *fakeRootPeer) ClosePresentation(
	_ context.Context,
	req factoryvisualization.ClosePresentationRequest,
) (factoryvisualization.ClosePresentationResult, error) {
	session, err := f.presentation(req.SessionID)
	if err != nil {
		return factoryvisualization.ClosePresentationResult{}, err
	}
	session.closed = true
	session.finalized = true
	return factoryvisualization.ClosePresentationResult{
		DroppedCount: 0,
	}, nil
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

func requireLifecycleError(
	t *testing.T,
	err error,
	kind factoryvisualization.LifecycleErrorKind,
	label string,
) {
	t.Helper()
	var lifeErr *factoryvisualization.LifecycleError
	if !errors.As(err, &lifeErr) || lifeErr.Kind != kind {
		t.Fatalf("%s: error = %v, want %s", label, err, kind)
	}
}

func requireProjectionError(
	t *testing.T,
	err error,
	kind factoryvisualization.ProjectionErrorKind,
	label string,
) {
	t.Helper()
	var projErr *factoryvisualization.ProjectionError
	if !errors.As(err, &projErr) || projErr.Kind != kind {
		t.Fatalf("%s: error = %v, want %s", label, err, kind)
	}
}

func requirePresentationError(
	t *testing.T,
	err error,
	kind factoryvisualization.PresentationErrorKind,
	label string,
) {
	t.Helper()
	var presErr *factoryvisualization.PresentationError
	if !errors.As(err, &presErr) || presErr.Kind != kind {
		t.Fatalf("%s: error = %v, want %s", label, err, kind)
	}
}

func TestRootContractFakePeerInertNotActivated(t *testing.T) {
	t.Parallel()

	peer := &fakeRootPeer{state: factoryvisualization.LifecycleStateInert}
	var root factoryvisualization.Root = peer

	_, err := root.Join(context.Background(), factoryvisualization.JoinRequest{})
	if err == nil {
		t.Fatal("Join on inert Root: error = nil, want not-activated failure")
	}
	requireLifecycleError(t, err, factoryvisualization.LifecycleErrorNotActivated, "Join on inert Root")
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
	requireLifecycleError(t, err, factoryvisualization.LifecycleErrorMissingParameters, "Activate missing parameters")
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
	requireLifecycleError(t, err, factoryvisualization.LifecycleErrorAlreadyActivated, "Activate already activated")

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
	requireProjectionError(t, err, factoryvisualization.ProjectionErrorInvalidInput, "Observe missing parameters")

	neg := -1
	_, err = root.Observe(context.Background(), factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
		Reconnect: &factoryvisualization.ObserveReconnectCursor{
			AfterSequence: &neg,
		},
	})
	requireProjectionError(t, err, factoryvisualization.ProjectionErrorInvalidInput, "Observe invalid reconnect")

	peer.observeSnapshotOK = false
	_, err = root.Observe(context.Background(), factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
	})
	requireProjectionError(t, err, factoryvisualization.ProjectionErrorSnapshotUnavailable, "Observe unavailable snapshot")

	peer.observeSnapshotOK = true
	peer.observeReconstructOK = false
	_, err = root.Observe(context.Background(), factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
	})
	requireProjectionError(t, err, factoryvisualization.ProjectionErrorReconstructionFailed, "Observe reconstruction failure")

	peer.observeReconstructOK = true
	result, err := root.Observe(context.Background(), factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("Observe valid path: error = %v", err)
	}
	if result.View.TickCount != 7 || result.View.RetainedEventCount != 2 {
		t.Fatalf("Observe view = %#v, want tick 7 retained 2", result.View)
	}
	if !result.View.ObservedAt.Equal(observedAt) {
		t.Fatalf("Observe ObservedAt = %v, want %v", result.View.ObservedAt, observedAt)
	}
	if peer.subscribed || peer.presented {
		t.Fatal("Observe must not imply Activate subscription/presentation side effects on inert peer")
	}
}

func TestRootPresentationDrainSuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	peer := &fakeRootPeer{state: factoryvisualization.LifecycleStateInert}
	var root factoryvisualization.Root = peer

	_, err := root.OpenPresentation(context.Background(), factoryvisualization.OpenPresentationRequest{})
	requirePresentationError(t, err, factoryvisualization.PresentationErrorInvalidInput, "OpenPresentation missing parameters")

	assertPeerPresentationSuccessDrain(t, root, peer)
	assertPeerPresentationTypedFailures(t, root)

	if peer.subscribed {
		t.Fatal("presentation/drain must not require Activate subscription ownership on the peer")
	}
}

func assertPeerPresentationSuccessDrain(
	t *testing.T,
	root factoryvisualization.Root,
	peer *fakeRootPeer,
) {
	t.Helper()
	opened, err := root.OpenPresentation(context.Background(), factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryLossless,
	})
	if err != nil {
		t.Fatalf("OpenPresentation lossless: error = %v", err)
	}
	if opened.SessionID == "" || opened.Mode != factoryvisualization.PresentationDeliveryLossless {
		t.Fatalf("OpenPresentation result = %#v, want lossless session", opened)
	}

	progress, err := root.PresentProgress(context.Background(), factoryvisualization.PresentProgressRequest{
		SessionID: opened.SessionID,
		Records: []factoryvisualization.ProgressRecord{
			{Payload: []byte("one")},
			{Payload: []byte("two")},
		},
	})
	if err != nil || progress.AcceptedCount != 2 {
		t.Fatalf("PresentProgress: result=%#v err=%v", progress, err)
	}

	finalized, err := root.FinalizePresentation(context.Background(), factoryvisualization.FinalizePresentationRequest{
		SessionID: opened.SessionID,
		Terminal:  &factoryvisualization.TerminalWrite{Payload: []byte("done")},
	})
	if err != nil || !finalized.Finalized || !finalized.ProgressSeen {
		t.Fatalf("FinalizePresentation result = %#v err=%v", finalized, err)
	}
	session := peer.presentations[opened.SessionID]
	if len(session.records) != 3 {
		t.Fatalf("drained records = %#v, want ordered progress then terminal", session.records)
	}
	if string(session.records[0]) != "one" || string(session.records[1]) != "two" || string(session.records[2]) != "done" {
		t.Fatalf("record order = %#v, want one,two,done", session.records)
	}

	_, err = root.PresentProgress(context.Background(), factoryvisualization.PresentProgressRequest{
		SessionID: opened.SessionID,
		Records:   []factoryvisualization.ProgressRecord{{Payload: []byte("late")}},
	})
	requirePresentationError(t, err, factoryvisualization.PresentationErrorEnqueueAfterClose, "PresentProgress after finalize")
}

func assertPeerPresentationTypedFailures(
	t *testing.T,
	root factoryvisualization.Root,
) {
	t.Helper()
	bestEffort, err := root.OpenPresentation(context.Background(), factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryBestEffort,
	})
	if err != nil {
		t.Fatalf("OpenPresentation best-effort: error = %v", err)
	}
	if _, err := root.PresentProgress(context.Background(), factoryvisualization.PresentProgressRequest{
		SessionID: bestEffort.SessionID,
		Records: []factoryvisualization.ProgressRecord{
			{Payload: []byte("a")},
			{Payload: []byte("b")},
		},
	}); err != nil {
		t.Fatalf("PresentProgress fill capacity: error = %v", err)
	}
	_, err = root.PresentProgress(context.Background(), factoryvisualization.PresentProgressRequest{
		SessionID: bestEffort.SessionID,
		Records:   []factoryvisualization.ProgressRecord{{Payload: []byte("c")}},
	})
	requirePresentationError(t, err, factoryvisualization.PresentationErrorBackpressureRejected, "PresentProgress backpressure")

	_, err = root.FinalizePresentation(context.Background(), factoryvisualization.FinalizePresentationRequest{
		SessionID: bestEffort.SessionID,
		Terminal:  nil,
	})
	requirePresentationError(t, err, factoryvisualization.PresentationErrorFinalizeWithoutWriter, "FinalizePresentation without terminal")
}

// TestRootContractInvariants_AllSlicesThroughSingularRoot seals the Factory
// Visualization root-contract packet for IMP-VIS unlock: lifecycle, live
// projection, and presentation/drain slices are reachable through one named
// Root, a peer-shaped consumer exercises success and typed-failure paths using
// only the root package (no Recordings ledger storage, Runtime Petri/JS
// internals, or transport codec/writer types), construction stays inert until
// Activate, and no second peer-facing Visualization authority is required.
func TestRootContractInvariants_AllSlicesThroughSingularRoot(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, time.July, 24, 3, 55, 0, 0, time.UTC)
	peer := &fakeRootPeer{
		state:                factoryvisualization.LifecycleStateInert,
		observeSnapshotOK:    true,
		observeReconstructOK: true,
		observeView: factoryvisualization.ProjectedView{
			TickCount:          11,
			RetainedEventCount: 4,
			ObservedAt:         observedAt,
		},
	}
	var root factoryvisualization.Root = peer
	ctx := context.Background()

	assertSealedLifecycleSlice(t, root, peer, ctx)
	assertSealedLiveProjectionSlice(t, root, peer, observedAt, ctx)
	assertSealedPresentationDrainSlice(t, root, ctx)

	stopped, err := root.StopDrain(ctx, factoryvisualization.StopDrainRequest{})
	if err != nil {
		t.Fatalf("lifecycle StopDrain success: %v", err)
	}
	if stopped.State != factoryvisualization.LifecycleStateStopped {
		t.Fatalf("lifecycle StopDrain state = %q, want %q", stopped.State, factoryvisualization.LifecycleStateStopped)
	}
	if peer.subscribed {
		t.Fatal("StopDrain must clear subscription ownership on the peer")
	}

	// Compile-time singular-authority proof: concrete Service remains the one
	// production Root implementer for the sealed slices.
	var _ factoryvisualization.Root = (*factoryvisualization.Service)(nil)
}

func assertSealedLifecycleSlice(
	t *testing.T,
	root factoryvisualization.Root,
	peer *fakeRootPeer,
	ctx context.Context,
) {
	t.Helper()
	_, err := root.Join(ctx, factoryvisualization.JoinRequest{})
	requireLifecycleError(t, err, factoryvisualization.LifecycleErrorNotActivated, "lifecycle inert Join")
	if peer.subscribed || peer.presented || peer.state != factoryvisualization.LifecycleStateInert {
		t.Fatal("constructed Root must remain inert until explicit Activate")
	}

	_, err = root.Activate(ctx, factoryvisualization.ActivateRequest{})
	requireLifecycleError(t, err, factoryvisualization.LifecycleErrorMissingParameters, "lifecycle Activate missing parameters")
	if peer.subscribed || peer.presented || peer.state != factoryvisualization.LifecycleStateInert {
		t.Fatal("missing-parameter Activate must leave Root inert")
	}

	activated, err := root.Activate(ctx, factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("lifecycle Activate success: %v", err)
	}
	if activated.State != factoryvisualization.LifecycleStateStarted {
		t.Fatalf("lifecycle Activate state = %q, want %q", activated.State, factoryvisualization.LifecycleStateStarted)
	}
	_, err = root.Activate(ctx, factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateModeRetainedThenLive,
	})
	requireLifecycleError(t, err, factoryvisualization.LifecycleErrorAlreadyActivated, "lifecycle Activate already activated")
}

func assertSealedLiveProjectionSlice(
	t *testing.T,
	root factoryvisualization.Root,
	peer *fakeRootPeer,
	observedAt time.Time,
	ctx context.Context,
) {
	t.Helper()
	_, err := root.Observe(ctx, factoryvisualization.ObserveRequest{})
	requireProjectionError(t, err, factoryvisualization.ProjectionErrorInvalidInput, "live-projection invalid input")

	peer.observeSnapshotOK = false
	_, err = root.Observe(ctx, factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
	})
	requireProjectionError(t, err, factoryvisualization.ProjectionErrorSnapshotUnavailable, "live-projection snapshot unavailable")

	peer.observeSnapshotOK = true
	peer.observeReconstructOK = false
	_, err = root.Observe(ctx, factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
	})
	requireProjectionError(t, err, factoryvisualization.ProjectionErrorReconstructionFailed, "live-projection reconstruction failed")

	peer.observeReconstructOK = true
	observed, err := root.Observe(ctx, factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveModeRetainedThenLive,
	})
	if err != nil {
		t.Fatalf("live-projection Observe success: %v", err)
	}
	if observed.View.TickCount != 11 || observed.View.RetainedEventCount != 4 {
		t.Fatalf("live-projection view = %#v, want tick 11 retained 4", observed.View)
	}
	if !observed.View.ObservedAt.Equal(observedAt) {
		t.Fatalf("live-projection ObservedAt = %v, want %v", observed.View.ObservedAt, observedAt)
	}
}

func assertSealedPresentationDrainSlice(
	t *testing.T,
	root factoryvisualization.Root,
	ctx context.Context,
) {
	t.Helper()
	opened, err := root.OpenPresentation(ctx, factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryLossless,
	})
	if err != nil {
		t.Fatalf("presentation OpenPresentation success: %v", err)
	}
	progress, err := root.PresentProgress(ctx, factoryvisualization.PresentProgressRequest{
		SessionID: opened.SessionID,
		Records: []factoryvisualization.ProgressRecord{
			{Payload: []byte("seal-one")},
			{Payload: []byte("seal-two")},
		},
	})
	if err != nil || progress.AcceptedCount != 2 {
		t.Fatalf("presentation PresentProgress success: result=%#v err=%v", progress, err)
	}
	finalized, err := root.FinalizePresentation(ctx, factoryvisualization.FinalizePresentationRequest{
		SessionID: opened.SessionID,
		Terminal:  &factoryvisualization.TerminalWrite{Payload: []byte("seal-done")},
	})
	if err != nil || !finalized.Finalized || !finalized.ProgressSeen {
		t.Fatalf("presentation FinalizePresentation success: result=%#v err=%v", finalized, err)
	}
	_, err = root.PresentProgress(ctx, factoryvisualization.PresentProgressRequest{
		SessionID: opened.SessionID,
		Records:   []factoryvisualization.ProgressRecord{{Payload: []byte("late")}},
	})
	requirePresentationError(t, err, factoryvisualization.PresentationErrorEnqueueAfterClose, "presentation enqueue after close")

	bestEffort, err := root.OpenPresentation(ctx, factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryBestEffort,
	})
	if err != nil {
		t.Fatalf("presentation OpenPresentation best-effort: %v", err)
	}
	if _, err := root.PresentProgress(ctx, factoryvisualization.PresentProgressRequest{
		SessionID: bestEffort.SessionID,
		Records: []factoryvisualization.ProgressRecord{
			{Payload: []byte("a")},
			{Payload: []byte("b")},
		},
	}); err != nil {
		t.Fatalf("presentation fill best-effort capacity: %v", err)
	}
	_, err = root.PresentProgress(ctx, factoryvisualization.PresentProgressRequest{
		SessionID: bestEffort.SessionID,
		Records:   []factoryvisualization.ProgressRecord{{Payload: []byte("c")}},
	})
	requirePresentationError(t, err, factoryvisualization.PresentationErrorBackpressureRejected, "presentation backpressure")

	_, err = root.FinalizePresentation(ctx, factoryvisualization.FinalizePresentationRequest{
		SessionID: bestEffort.SessionID,
		Terminal:  nil,
	})
	requirePresentationError(t, err, factoryvisualization.PresentationErrorFinalizeWithoutWriter, "presentation finalize without writer")
}
