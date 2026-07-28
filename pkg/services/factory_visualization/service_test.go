package factory_visualization

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/testing/recordingsstub"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// FND-12 captured visualization-activation typed-failure baseline: activation
// construct fails with an explicit missing-dependency error. Invoked by
// `make fnd-12-visualization-behavior-baselines`.
func TestNewRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	clock := fixedClock{now: time.Unix(1, 0)}
	sink := SinkFunc(func(View) {})
	projections := &recordingsstub.Service{}
	source := &sourceStub{}
	tests := []struct {
		name string
		new  func() (*Service, error)
		want string
	}{
		{"source", func() (*Service, error) {
			return New(nil, projections, clock, sink, nil)
		}, "event source"},
		{"projections", func() (*Service, error) {
			return New(source, nil, clock, sink, nil)
		}, "projection service"},
		{"clock", func() (*Service, error) {
			return New(source, projections, nil, sink, nil)
		}, "clock"},
		{"sink", func() (*Service, error) {
			return New(source, projections, clock, nil, nil)
		}, "presentation sink"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := test.new()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("New() error = %v, want %q", err, test.want)
			}
		})
	}
}

// FND-12 captured visualization-activation success baseline: Start against a
// valid event source projects retained-then-live events and emits observable
// Views. Invoked by `make fnd-12-visualization-behavior-baselines`.
func TestServiceProjectsRetainedAndLiveFactoryEvents(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 20, 10, 0, 0, 0, time.UTC)
	live := make(chan factorydefinitions.FactoryEvent, 1)
	history := event("history", 3)
	source := &sourceStub{
		stream: &factorydefinitions.FactoryEventStream{
			History: []factorydefinitions.FactoryEvent{history},
			Events:  live,
		},
		snapshot: visualizationSnapshotFacts(3),
	}
	projected := make(chan []factorydefinitions.FactoryEvent, 2)
	projections := &recordingsstub.Service{
		ReconstructWorldStateFn: func(request recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error) {
			if request.SelectedTick != 3 {
				t.Fatalf("projection tick = %d, want 3", request.SelectedTick)
			}
			events := make([]factorydefinitions.FactoryEvent, len(request.Events))
			for index, event := range request.Events {
				events[index] = factorydefinitions.FactoryEvent{
					Id: string(event.ID),
					Context: factorydefinitions.FactoryEventContext{
						Sequence: int(event.Sequence),
					},
				}
			}
			projected <- events
			return recordings.ReconstructWorldStateResult{
				WorldState: recordings.WorldStateView{
					SchemaVersion: recordings.WorldStateViewSchemaV1,
					Payload:       `{"topology":{}}`,
				},
			}, nil
		},
	}
	rendered := make(chan View, 2)
	service, err := New(
		source,
		projections,
		fixedClock{now: now},
		SinkFunc(func(view View) { rendered <- view }),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if got := <-projected; len(got) != 1 || got[0].Id != history.Id {
		t.Fatalf("initial projection events = %#v", got)
	}
	if got := <-rendered; !got.ObservedAt.Equal(now) || got.Runtime.TickCount != 3 {
		t.Fatalf("initial view = %#v", got)
	}

	liveEvent := event("live", 4)
	live <- liveEvent
	if got := <-projected; len(got) != 2 || got[1].Id != liveEvent.Id {
		t.Fatalf("live projection events = %#v", got)
	}
	<-rendered

	service.mu.Lock()
	cursor := service.activation.ReconnectCursor()
	service.mu.Unlock()
	if cursor == nil || cursor.AfterEventID != liveEvent.Id ||
		cursor.AfterSequence == nil || *cursor.AfterSequence != 4 {
		t.Fatalf("cursor = %#v, want live event", cursor)
	}
	cancel()
	if err := service.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func TestServiceReportsProjectionReadFailureWithoutStoppingSubscription(t *testing.T) {
	t.Parallel()

	live := make(chan factorydefinitions.FactoryEvent)
	readFailure := errors.New("snapshot unavailable")
	reported := make(chan error, 1)
	service, err := New(
		&sourceStub{
			stream:      &factorydefinitions.FactoryEventStream{Events: live},
			snapshotErr: readFailure,
		},
		&recordingsstub.Service{},
		fixedClock{},
		SinkFunc(func(View) { t.Fatal("sink called after snapshot failure") }),
		func(err error) { reported <- err },
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := service.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if got := <-reported; !errors.Is(got, readFailure) {
		t.Fatalf("reported error = %v, want %v", got, readFailure)
	}
	cancel()
	if err := service.Wait(context.Background()); err != nil {
		t.Fatalf("Wait() error = %v", err)
	}
}

func requireServiceLifecycleError(t *testing.T, err error, kind LifecycleErrorKind, label string) {
	t.Helper()
	var lifeErr *LifecycleError
	if !errors.As(err, &lifeErr) || lifeErr.Kind != kind {
		t.Fatalf("%s: error = %v, want %s", label, err, kind)
	}
}

func TestServiceRootLifecycleInertConstructionAndTypedActivate(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	live := make(chan factorydefinitions.FactoryEvent)
	source := &sourceStub{
		stream:   &factorydefinitions.FactoryEventStream{Events: live},
		snapshot: visualizationSnapshotFacts(1),
	}
	source.subscribeHook = func() { subscribeCalls++ }
	presentCalls := 0
	service, err := New(
		source,
		&recordingsstub.Service{},
		fixedClock{now: time.Unix(1, 0)},
		SinkFunc(func(View) { presentCalls++ }),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var root Root = service
	if subscribeCalls != 0 || presentCalls != 0 {
		t.Fatalf("New() side effects: subscribe=%d present=%d, want inert construction", subscribeCalls, presentCalls)
	}

	_, err = root.Join(context.Background(), JoinRequest{})
	requireServiceLifecycleError(t, err, LifecycleErrorNotActivated, "Join before Activate")
	if subscribeCalls != 0 || presentCalls != 0 {
		t.Fatal("Join before Activate must not subscribe or present")
	}

	_, err = root.Activate(context.Background(), ActivateRequest{})
	requireServiceLifecycleError(t, err, LifecycleErrorMissingParameters, "Activate missing parameters")
	if subscribeCalls != 0 {
		t.Fatal("missing-parameter Activate must not subscribe")
	}

	_, err = root.Activate(context.Background(), ActivateRequest{Mode: ActivateMode("UNSUPPORTED")})
	requireServiceLifecycleError(t, err, LifecycleErrorMissingParameters, "Activate unsupported mode")
	if subscribeCalls != 0 {
		t.Fatal("unsupported-mode Activate must not subscribe")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result, err := root.Activate(ctx, ActivateRequest{Mode: ActivateModeRetainedThenLive})
	if err != nil {
		t.Fatalf("Activate: error = %v", err)
	}
	if result.State != LifecycleStateStarted {
		t.Fatalf("Activate state = %q, want %q", result.State, LifecycleStateStarted)
	}
	if subscribeCalls != 1 {
		t.Fatalf("subscribe calls = %d, want 1 after explicit Activate", subscribeCalls)
	}

	_, err = root.Activate(ctx, ActivateRequest{Mode: ActivateModeRetainedThenLive})
	requireServiceLifecycleError(t, err, LifecycleErrorAlreadyActivated, "Activate already activated")

	cancel()
	if _, err := root.StopDrain(context.Background(), StopDrainRequest{}); err != nil {
		t.Fatalf("StopDrain: error = %v", err)
	}
}

func TestServiceRootObserveDetachedViewAndTypedFailures(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.July, 23, 21, 0, 0, 0, time.UTC)
	live := make(chan factorydefinitions.FactoryEvent)
	history := event("history", 3)
	source := &sourceStub{
		stream: &factorydefinitions.FactoryEventStream{
			History: []factorydefinitions.FactoryEvent{history},
			Events:  live,
		},
		snapshot: visualizationSnapshotFacts(9),
	}
	service, err := New(
		source,
		&recordingsstub.Service{},
		fixedClock{now: now},
		SinkFunc(func(View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var root Root = service

	_, err = root.Observe(context.Background(), ObserveRequest{})
	var projErr *ProjectionError
	if !errors.As(err, &projErr) || projErr.Kind != ProjectionErrorInvalidInput {
		t.Fatalf("Observe missing parameters: error = %v, want InvalidInput", err)
	}

	source.snapshot = nil
	_, err = root.Observe(context.Background(), ObserveRequest{Mode: ObserveModeRetainedThenLive})
	if !errors.As(err, &projErr) || projErr.Kind != ProjectionErrorSnapshotUnavailable {
		t.Fatalf("Observe unavailable snapshot: error = %v, want SnapshotUnavailable", err)
	}

	source.snapshot = visualizationSnapshotFacts(9)
	service, err = New(
		source,
		&recordingsstub.Service{
			ReconstructWorldStateFn: func(recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error) {
				return recordings.ReconstructWorldStateResult{}, errors.New("reconstruct boom")
			},
		},
		fixedClock{now: now},
		SinkFunc(func(View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("New() after reconstruction stub: error = %v", err)
	}
	root = service
	_, err = root.Observe(context.Background(), ObserveRequest{Mode: ObserveModeRetainedThenLive})
	if !errors.As(err, &projErr) || projErr.Kind != ProjectionErrorReconstructionFailed {
		t.Fatalf("Observe reconstruction failure: error = %v, want ReconstructionFailed", err)
	}

	service, err = New(
		source,
		&recordingsstub.Service{},
		fixedClock{now: now},
		SinkFunc(func(View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("New() after success stub: error = %v", err)
	}
	root = service
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := root.Activate(ctx, ActivateRequest{Mode: ActivateModeRetainedThenLive}); err != nil {
		t.Fatalf("Activate: error = %v", err)
	}

	result, err := root.Observe(context.Background(), ObserveRequest{Mode: ObserveModeRetainedThenLive})
	if err != nil {
		t.Fatalf("Observe after Activate: error = %v", err)
	}
	if result.View.TickCount != 9 {
		t.Fatalf("Observe TickCount = %d, want 9", result.View.TickCount)
	}
	if result.View.RetainedEventCount != 1 {
		t.Fatalf("Observe RetainedEventCount = %d, want 1", result.View.RetainedEventCount)
	}
	if !result.View.ObservedAt.Equal(now) {
		t.Fatalf("Observe ObservedAt = %v, want %v", result.View.ObservedAt, now)
	}

	cancel()
	if _, err := root.StopDrain(context.Background(), StopDrainRequest{}); err != nil {
		t.Fatalf("StopDrain: error = %v", err)
	}
}

func TestServiceRootPresentationDrainSuccessAndTypedFailures(t *testing.T) {
	t.Parallel()

	service := mustNewRootPresentationService(t)
	var root Root = service

	_, err := root.OpenPresentation(context.Background(), OpenPresentationRequest{})
	var presErr *PresentationError
	if !errors.As(err, &presErr) || presErr.Kind != PresentationErrorInvalidInput {
		t.Fatalf("OpenPresentation missing parameters: error = %v, want InvalidInput", err)
	}

	assertServicePresentationSuccessDrain(t, root, service)
	assertServicePresentationTypedFailures(t, root, service)
}

func mustNewRootPresentationService(t *testing.T) *Service {
	t.Helper()
	live := make(chan factorydefinitions.FactoryEvent)
	service, err := New(
		&sourceStub{
			stream:   &factorydefinitions.FactoryEventStream{Events: live},
			snapshot: visualizationSnapshotFacts(1),
		},
		&recordingsstub.Service{},
		fixedClock{now: time.Unix(1, 0)},
		SinkFunc(func(View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return service
}

func assertServicePresentationSuccessDrain(t *testing.T, root Root, service *Service) {
	t.Helper()
	opened, err := root.OpenPresentation(context.Background(), OpenPresentationRequest{
		Mode: PresentationDeliveryLossless,
	})
	if err != nil {
		t.Fatalf("OpenPresentation: error = %v", err)
	}
	progress, err := root.PresentProgress(context.Background(), PresentProgressRequest{
		SessionID: opened.SessionID,
		Records: []ProgressRecord{
			{Payload: []byte("alpha")},
			{Payload: []byte("beta")},
		},
	})
	if err != nil {
		t.Fatalf("PresentProgress: error = %v", err)
	}
	if progress.AcceptedCount != 2 {
		t.Fatalf("PresentProgress AcceptedCount = %d, want 2", progress.AcceptedCount)
	}
	finalized, err := root.FinalizePresentation(context.Background(), FinalizePresentationRequest{
		SessionID: opened.SessionID,
		Terminal:  &TerminalWrite{Payload: []byte("omega")},
	})
	if err != nil {
		t.Fatalf("FinalizePresentation: error = %v", err)
	}
	if !finalized.Finalized || !finalized.ProgressSeen {
		t.Fatalf("FinalizePresentation result = %#v", finalized)
	}
	got := service.presentations[opened.SessionID].writer.String()
	if got != "alpha\nbeta\nomega\n" {
		t.Fatalf("drained presentation = %q, want alpha/beta/omega", got)
	}
	_, err = root.PresentProgress(context.Background(), PresentProgressRequest{
		SessionID: opened.SessionID,
		Records:   []ProgressRecord{{Payload: []byte("late")}},
	})
	var presErr *PresentationError
	if !errors.As(err, &presErr) || presErr.Kind != PresentationErrorEnqueueAfterClose {
		t.Fatalf("PresentProgress after finalize: error = %v, want EnqueueAfterClose", err)
	}
}

func assertServicePresentationTypedFailures(t *testing.T, root Root, service *Service) {
	t.Helper()
	var presErr *PresentationError
	bestEffort, err := root.OpenPresentation(context.Background(), OpenPresentationRequest{
		Mode: PresentationDeliveryBestEffort,
	})
	if err != nil {
		t.Fatalf("OpenPresentation best-effort: error = %v", err)
	}
	_, err = root.FinalizePresentation(context.Background(), FinalizePresentationRequest{
		SessionID: bestEffort.SessionID,
	})
	if !errors.As(err, &presErr) || presErr.Kind != PresentationErrorFinalizeWithoutWriter {
		t.Fatalf("FinalizePresentation without terminal: error = %v, want FinalizeWithoutWriter", err)
	}

	blocked, err := root.OpenPresentation(context.Background(), OpenPresentationRequest{
		Mode: PresentationDeliveryBestEffort,
	})
	if err != nil {
		t.Fatalf("OpenPresentation blocked best-effort: error = %v", err)
	}
	blockedSession := service.presentations[blocked.SessionID]
	writer := newGatedPresentationWriter()
	blockedSession.mu.Lock()
	_ = blockedSession.output.CloseAndDrain()
	blockedSession.output = openBestEffortOutput(writer)
	blockedSession.closed = false
	blockedSession.finalized = false
	blockedSession.mu.Unlock()

	// Occupy the consumer on a blocked write before filling the bounded queue so
	// capacity enqueues cannot race a free slot into the overflow PresentProgress.
	if _, err := root.PresentProgress(context.Background(), PresentProgressRequest{
		SessionID: blocked.SessionID,
		Records:   []ProgressRecord{{Payload: []byte("block")}},
	}); err != nil {
		writer.release()
		t.Fatalf("seed blocked write: %v", err)
	}
	waitForPresentationWriteAttempt(t, writer)

	for i := 0; i < DefaultProgressQueueCapacity; i++ {
		if _, err := root.PresentProgress(context.Background(), PresentProgressRequest{
			SessionID: blocked.SessionID,
			Records:   []ProgressRecord{{Payload: []byte("x")}},
		}); err != nil {
			writer.release()
			t.Fatalf("fill backlog item %d: %v", i, err)
		}
	}
	_, err = root.PresentProgress(context.Background(), PresentProgressRequest{
		SessionID: blocked.SessionID,
		Records:   []ProgressRecord{{Payload: []byte("overflow")}},
	})
	writer.release()
	if !errors.As(err, &presErr) || presErr.Kind != PresentationErrorBackpressureRejected {
		t.Fatalf("PresentProgress backpressure: error = %v, want BackpressureRejected", err)
	}
}

func event(id string, sequence int) factorydefinitions.FactoryEvent {
	return factorydefinitions.FactoryEvent{
		Id: id,
		Context: factorydefinitions.FactoryEventContext{
			Sequence: sequence,
			Tick:     sequence,
		},
	}
}

type sourceStub struct {
	stream        *factorydefinitions.FactoryEventStream
	subscribeErr  error
	subscribeHook func()
	snapshot      *liveviewprojection.RuntimeSnapshotFacts
	snapshotErr   error
}

func (s *sourceStub) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	if s.subscribeHook != nil {
		s.subscribeHook()
	}
	return s.stream, s.subscribeErr
}

func (s *sourceStub) GetRuntimeSnapshotFacts(context.Context) (*liveviewprojection.RuntimeSnapshotFacts, error) {
	return s.snapshot, s.snapshotErr
}

func visualizationSnapshotFacts(tick int) *liveviewprojection.RuntimeSnapshotFacts {
	return &liveviewprojection.RuntimeSnapshotFacts{
		RuntimeObservation: liveviewprojection.RuntimeObservation{TickCount: tick},
	}
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }
