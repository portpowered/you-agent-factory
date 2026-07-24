package factory_visualization

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
)

// FND-12 captured visualization-activation typed-failure baseline: activation
// construct fails with an explicit missing-dependency error. Invoked by
// `make fnd-12-visualization-behavior-baselines`.
func TestNewRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	clock := fixedClock{now: time.Unix(1, 0)}
	sink := SinkFunc(func(View) {})
	projections := projectionStub{}
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
		snapshot: &factoryruntime.StateSnapshot{TickCount: 3},
	}
	projected := make(chan []factorydefinitions.FactoryEvent, 2)
	projections := projectionStub{
		reconstruct: func(events []factorydefinitions.FactoryEvent, tick int) (factorydefinitions.FactoryWorldState, error) {
			if tick != 3 {
				t.Fatalf("projection tick = %d, want 3", tick)
			}
			projected <- append([]factorydefinitions.FactoryEvent(nil), events...)
			return factorydefinitions.FactoryWorldState{}, nil
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
	if got := <-rendered; !got.ObservedAt.Equal(now) || got.EngineState.TickCount != 3 {
		t.Fatalf("initial view = %#v", got)
	}

	liveEvent := event("live", 4)
	live <- liveEvent
	if got := <-projected; len(got) != 2 || got[1].Id != liveEvent.Id {
		t.Fatalf("live projection events = %#v", got)
	}
	<-rendered

	service.mu.Lock()
	cursor := service.cursor
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
		projectionStub{},
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

func TestServiceRootLifecycleInertConstructionAndTypedActivate(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	live := make(chan factorydefinitions.FactoryEvent)
	source := &sourceStub{
		stream:   &factorydefinitions.FactoryEventStream{Events: live},
		snapshot: &factoryruntime.StateSnapshot{TickCount: 1},
	}
	source.subscribeHook = func() { subscribeCalls++ }
	presentCalls := 0
	service, err := New(
		source,
		projectionStub{},
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
	var lifeErr *LifecycleError
	if !errors.As(err, &lifeErr) || lifeErr.Kind != LifecycleErrorNotActivated {
		t.Fatalf("Join before Activate: error = %v, want NotActivated", err)
	}
	if subscribeCalls != 0 || presentCalls != 0 {
		t.Fatal("Join before Activate must not subscribe or present")
	}

	_, err = root.Activate(context.Background(), ActivateRequest{})
	if !errors.As(err, &lifeErr) || lifeErr.Kind != LifecycleErrorMissingParameters {
		t.Fatalf("Activate missing parameters: error = %v, want MissingParameters", err)
	}
	if subscribeCalls != 0 {
		t.Fatal("missing-parameter Activate must not subscribe")
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
	if !errors.As(err, &lifeErr) || lifeErr.Kind != LifecycleErrorAlreadyActivated {
		t.Fatalf("Activate already activated: error = %v, want AlreadyActivated", err)
	}

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
		snapshot: &factoryruntime.StateSnapshot{TickCount: 9},
	}
	service, err := New(
		source,
		projectionStub{},
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

	source.snapshot = &factoryruntime.StateSnapshot{TickCount: 9}
	service.projections = projectionStub{
		reconstruct: func([]factorydefinitions.FactoryEvent, int) (factorydefinitions.FactoryWorldState, error) {
			return factorydefinitions.FactoryWorldState{}, errors.New("reconstruct boom")
		},
	}
	_, err = root.Observe(context.Background(), ObserveRequest{Mode: ObserveModeRetainedThenLive})
	if !errors.As(err, &projErr) || projErr.Kind != ProjectionErrorReconstructionFailed {
		t.Fatalf("Observe reconstruction failure: error = %v, want ReconstructionFailed", err)
	}

	service.projections = projectionStub{}
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

	live := make(chan factorydefinitions.FactoryEvent)
	service, err := New(
		&sourceStub{
			stream:   &factorydefinitions.FactoryEventStream{Events: live},
			snapshot: &factoryruntime.StateSnapshot{TickCount: 1},
		},
		projectionStub{},
		fixedClock{now: time.Unix(1, 0)},
		SinkFunc(func(View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	var root Root = service

	_, err = root.OpenPresentation(context.Background(), OpenPresentationRequest{})
	var presErr *PresentationError
	if !errors.As(err, &presErr) || presErr.Kind != PresentationErrorInvalidInput {
		t.Fatalf("OpenPresentation missing parameters: error = %v, want InvalidInput", err)
	}

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

	session := service.presentations[opened.SessionID]
	got := session.writer.String()
	want := "alpha\nbeta\nomega\n"
	if got != want {
		t.Fatalf("drained presentation = %q, want %q", got, want)
	}

	_, err = root.PresentProgress(context.Background(), PresentProgressRequest{
		SessionID: opened.SessionID,
		Records:   []ProgressRecord{{Payload: []byte("late")}},
	})
	if !errors.As(err, &presErr) || presErr.Kind != PresentationErrorEnqueueAfterClose {
		t.Fatalf("PresentProgress after finalize: error = %v, want EnqueueAfterClose", err)
	}

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

	// Backpressure: block the Visualization-owned writer so best-effort backlog fills.
	blocked, err := root.OpenPresentation(context.Background(), OpenPresentationRequest{
		Mode: PresentationDeliveryBestEffort,
	})
	if err != nil {
		t.Fatalf("OpenPresentation blocked best-effort: error = %v", err)
	}
	blockedSession := service.presentations[blocked.SessionID]
	gate := make(chan struct{})
	blockedSession.mu.Lock()
	_ = blockedSession.output.CloseAndDrain()
	blockedSession.output = newBestEffortOutput(writerFunc(func(p []byte) (int, error) {
		<-gate
		return len(p), nil
	}))
	blockedSession.closed = false
	blockedSession.finalized = false
	blockedSession.mu.Unlock()

	for i := 0; i < defaultProgressQueueCapacity; i++ {
		if _, err := root.PresentProgress(context.Background(), PresentProgressRequest{
			SessionID: blocked.SessionID,
			Records:   []ProgressRecord{{Payload: []byte("x")}},
		}); err != nil {
			close(gate)
			t.Fatalf("fill backlog item %d: %v", i, err)
		}
	}
	_, err = root.PresentProgress(context.Background(), PresentProgressRequest{
		SessionID: blocked.SessionID,
		Records:   []ProgressRecord{{Payload: []byte("overflow")}},
	})
	close(gate)
	if !errors.As(err, &presErr) || presErr.Kind != PresentationErrorBackpressureRejected {
		t.Fatalf("PresentProgress backpressure: error = %v, want BackpressureRejected", err)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

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
	snapshot      *factoryruntime.StateSnapshot
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

func (s *sourceStub) GetEngineStateSnapshot(context.Context) (*factoryruntime.StateSnapshot, error) {
	return s.snapshot, s.snapshotErr
}

type projectionStub struct {
	reconstruct func([]factorydefinitions.FactoryEvent, int) (factorydefinitions.FactoryWorldState, error)
}

func (p projectionStub) ReconstructFactoryWorldState(
	events []factorydefinitions.FactoryEvent,
	tick int,
) (factorydefinitions.FactoryWorldState, error) {
	if p.reconstruct != nil {
		return p.reconstruct(events, tick)
	}
	return factorydefinitions.FactoryWorldState{}, nil
}

func (projectionStub) SimpleDashboardRenderData(
	factorydefinitions.FactoryWorldState,
) recordings.SimpleDashboardRenderData {
	return recordings.SimpleDashboardRenderData{}
}

func (projectionStub) ProjectActiveThrottlePauses(
	factorydefinitions.InitialStructurePayload,
	[]factorydefinitions.ActiveThrottlePause,
) []factorydefinitions.FactoryWorldThrottlePause {
	return nil
}

func (projectionStub) ProjectWorkstationRequests(
	factorydefinitions.FactoryWorldState,
) recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice {
	return recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{}
}

func (projectionStub) ValidateReconnectReplay(
	[]factorydefinitions.FactoryEvent,
	factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) error {
	return nil
}

type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }
