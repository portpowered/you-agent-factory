package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	activationlifecyclewire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle/wire"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	liveviewprojectionwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection/wire"
	responseeventpresentation "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation"
	responseeventpresentationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation/wire"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/testing/recordingsstub"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// FND-12 captured visualization-activation typed-failure baseline: activation
// construct fails with an explicit missing-dependency error. Invoked by
// `make fnd-12-visualization-behavior-baselines`.
func TestNewRejectsMissingDependencies(t *testing.T) {
	t.Parallel()

	clock := fixedClock{now: time.Unix(1, 0)}
	sink := SinkFunc(func(View) {})
	recordingsPeer := &recordingsstub.Service{}
	source := &sourceStub{}
	activation := activationOwnerStub{}
	projection := projectionOwnerStub{}
	presentation := newPresentationOwner()
	tests := []struct {
		name string
		new  func() (*Service, error)
		want string
	}{
		{"source", func() (*Service, error) {
			return assembleRoot(activation, projection, presentation, nil, recordingsPeer, clock, sink, nil)
		}, "event source"},
		{"recordings", func() (*Service, error) {
			return assembleRoot(activation, projection, presentation, source, nil, clock, sink, nil)
		}, "recordings service"},
		{"clock", func() (*Service, error) {
			return assembleRoot(activation, projection, presentation, source, recordingsPeer, nil, sink, nil)
		}, "clock"},
		{"sink", func() (*Service, error) {
			return assembleRoot(activation, projection, presentation, source, recordingsPeer, clock, nil, nil)
		}, "presentation sink"},
		{"presentation", func() (*Service, error) {
			return assembleRoot(activation, projection, nil, source, recordingsPeer, clock, sink, nil)
		}, "response event presentation service"},
		{"activation", func() (*Service, error) {
			return assembleRoot(nil, projection, presentation, source, recordingsPeer, clock, sink, nil)
		}, "activation lifecycle owner"},
		{"projection", func() (*Service, error) {
			return assembleRoot(activation, nil, presentation, source, recordingsPeer, clock, sink, nil)
		}, "live view projection owner"},
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

func newPresentationOwner() responseeventpresentation.Service {
	return responseeventpresentationwire.NewService()
}

func newComposedService(
	source Source,
	peer recordings.ProjectionService,
	clock Clock,
	sink Sink,
	presentation responseeventpresentation.Service,
	reportError ErrorReporter,
) (*Service, error) {
	if peer == nil {
		return nil, errors.New("test composition: projection service is required")
	}
	recordingsPeer, ok := peer.(recordings.Service)
	if !ok {
		return nil, errors.New("test composition: recordings.Service is required")
	}
	owner := NewScopeOwner(
		activationlifecyclewire.NewOwner(recordingsPeer).Open,
		liveviewprojectionwire.NewOwner(recordingsPeer).Open,
		presentation,
		recordingsPeer,
	)
	return owner.Open(source, clock, sink, reportError)
}

type activationOwnerStub struct{}

func (activationOwnerStub) Start(context.Context) error { return nil }
func (activationOwnerStub) Stop(context.Context) error  { return nil }
func (activationOwnerStub) Wait(context.Context) error  { return nil }
func (activationOwnerStub) Activate(context.Context, activationlifecycle.ActivateRequest) (activationlifecycle.ActivateResult, error) {
	return activationlifecycle.ActivateResult{}, nil
}
func (activationOwnerStub) Join(context.Context, activationlifecycle.JoinRequest) (activationlifecycle.JoinResult, error) {
	return activationlifecycle.JoinResult{}, nil
}
func (activationOwnerStub) StopDrain(context.Context, activationlifecycle.StopDrainRequest) (activationlifecycle.StopDrainResult, error) {
	return activationlifecycle.StopDrainResult{}, nil
}
func (activationOwnerStub) RetainedEvents() []factorydefinitions.FactoryEvent { return nil }
func (activationOwnerStub) ReconnectCursor() *factorydefinitions.FactoryEventReconnectCursor {
	return nil
}

type projectionOwnerStub struct{}

func (projectionOwnerStub) Start(context.Context) error { return nil }
func (projectionOwnerStub) Stop(context.Context) error  { return nil }
func (projectionOwnerStub) Wait(context.Context) error  { return nil }
func (projectionOwnerStub) Observe(context.Context, liveviewprojection.ObserveRequest) (liveviewprojection.ObserveResult, error) {
	return liveviewprojection.ObserveResult{}, nil
}
func (projectionOwnerStub) ReconnectCursor() *factorydefinitions.FactoryEventReconnectCursor {
	return nil
}

// FND-12 captured visualization-activation success baseline: Start against a
// valid event source projects retained-then-live events and emits observable
// Views. Invoked by `make fnd-12-visualization-behavior-baselines`.
// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
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
	service, err := newComposedService(
		source,
		projections,
		fixedClock{now: now},
		SinkFunc(func(view View) { rendered <- view }),
		newPresentationOwner(),
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
	service, err := newComposedService(
		&sourceStub{
			stream:      &factorydefinitions.FactoryEventStream{Events: live},
			snapshotErr: readFailure,
		},
		&recordingsstub.Service{},
		fixedClock{},
		SinkFunc(func(View) { t.Fatal("sink called after snapshot failure") }),
		newPresentationOwner(),
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
	service, err := newComposedService(
		source,
		&recordingsstub.Service{},
		fixedClock{now: time.Unix(1, 0)},
		SinkFunc(func(View) { presentCalls++ }),
		newPresentationOwner(),
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

// pkgmaintcheck:ignore-cyclomatic-complexity pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
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
	service, err := newComposedService(
		source,
		&recordingsstub.Service{},
		fixedClock{now: now},
		SinkFunc(func(View) {}),
		newPresentationOwner(),
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
	service, err = newComposedService(
		source,
		&recordingsstub.Service{
			ReconstructWorldStateFn: func(recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error) {
				return recordings.ReconstructWorldStateResult{}, errors.New("reconstruct boom")
			},
		},
		fixedClock{now: now},
		SinkFunc(func(View) {}),
		newPresentationOwner(),
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

	service, err = newComposedService(
		source,
		&recordingsstub.Service{},
		fixedClock{now: now},
		SinkFunc(func(View) {}),
		newPresentationOwner(),
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
	service, err := newComposedService(
		&sourceStub{
			stream:   &factorydefinitions.FactoryEventStream{Events: live},
			snapshot: visualizationSnapshotFacts(1),
		},
		&recordingsstub.Service{},
		fixedClock{now: time.Unix(1, 0)},
		SinkFunc(func(View) {}),
		newPresentationOwner(),
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
	blockedSession.output = service.presentationOwner.OpenBestEffortOutput(writer)
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

// The opening component receives controlled owner operations. No lifecycle,
// source or projection implementation participates in these selection witnesses.
type openingRuntimeReader func(string, func(*factorysessions.LiveRuntime) error) error

func (read openingRuntimeReader) WithRuntimeReadForSession(id string, consume func(*factorysessions.LiveRuntime) error) error {
	return read(id, consume)
}
func (openingRuntimeReader) WithRuntimeRead(func(*factorysessions.LiveRuntime) error) error {
	panic("opening must never select Current Factory")
}

type openingSinkRegistry struct {
	factoryvisualization.RuntimeSinkOwner
	sinks map[factoryvisualization.RuntimeSinkID]Sink
}

func (registry openingSinkRegistry) RuntimeSink(id factoryvisualization.RuntimeSinkID) (Sink, bool) {
	sink, ok := registry.sinks[id]
	return sink, ok
}

type openingRootStub struct{ Root }

func TestRuntimeOpeningKeepsSelectedEffectsAndPeerIsolation(t *testing.T) {
	t.Parallel()
	for _, id := range []string{"selected", "peer"} {
		t.Run(id, func(t *testing.T) { t.Parallel(); testRuntimeOpeningSelectedEffects(t, id) })
	}
}

func testRuntimeOpeningSelectedEffects(t *testing.T, id string) {
	t.Helper()
	core, logs := observer.New(zap.ErrorLevel)
	clock := fixedClock{now: time.Unix(123, 0)}
	runtime := &factorysessions.LiveRuntime{Clock: clock, LiveChangeLogger: zap.New(core).With(zap.String("session_id", id))}
	read := openingRuntimeReader(func(got string, consume func(*factorysessions.LiveRuntime) error) error {
		if got != id {
			return factorysessions.ErrSessionNotFound
		}
		return consume(runtime)
	})
	sinkID := factoryvisualization.RuntimeSinkID("sink-" + id)
	sink := &openingSinkStub{}
	source := &sourceStub{}
	root := &openingRootStub{}
	var reporter ErrorReporter
	observations := 0
	owner := NewRuntimeOpeningOwner(read,
		func(got string) Source {
			if got != id {
				t.Fatalf("source session = %q", got)
			}
			return source
		},
		func(gotSource Source, gotClock Clock, gotSink Sink, report ErrorReporter) (Root, error) {
			if gotSource != source || gotClock != clock || gotSink != sink {
				t.Fatal("opening substituted selected resources")
			}
			reporter = report
			return root, nil
		}, openingSinkRegistry{sinks: map[factoryvisualization.RuntimeSinkID]Sink{sinkID: sink}}, nil,
		func(got Root) {
			if got != root {
				t.Fatal("observer got another root")
			}
			observations++
		})
	got, err := owner.Open(t.Context(), id, sinkID)
	if err != nil || got != root || observations != 1 {
		t.Fatalf("Open = (%v, %v), observations=%d", got, err, observations)
	}
	// The acquired reporter keeps the original scoped sink even after
	// the selected runtime replaces its diagnostic resource.
	runtime.LiveChangeLogger = zap.NewNop()
	reporter(errors.New("selected projection failed"))
	assertOpeningErrorLog(t, logs, id)
	if got, err := owner.Open(t.Context(), "missing", sinkID); got != nil || !errors.Is(err, factorysessions.ErrSessionNotFound) || observations != 1 {
		t.Fatalf("missing Open = (%v, %v), observations=%d", got, err, observations)
	}
}

type openingSinkStub struct{ marker byte }

func (*openingSinkStub) PresentFactoryView(View) {}

func TestRuntimeOpeningResourceFailuresAndDisabledSelection(t *testing.T) {
	t.Parallel()
	cause := errors.New("scope failed")
	cases := []struct {
		name                   string
		sinkID                 factoryvisualization.RuntimeSinkID
		override               Sink
		cancel, missingRuntime bool
		wantError              error
		wantText               string
		wantReads              int
	}{
		{name: "disabled"},
		{name: "missing sink", sinkID: "missing", wantText: "unavailable"},
		{name: "cancelled", sinkID: "selected", cancel: true, wantError: context.Canceled},
		{name: "missing runtime", sinkID: "selected", missingRuntime: true, wantError: factorysessions.ErrRuntimeNotAvailable, wantReads: 1},
		{name: "scope failure", sinkID: "selected", wantError: cause, wantReads: 1},
		{name: "override", sinkID: "selected", override: &openingSinkStub{}, wantError: cause, wantReads: 1},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if test.cancel {
				cancel()
			}
			sink := &openingSinkStub{}
			registry := openingSinkRegistry{sinks: map[factoryvisualization.RuntimeSinkID]Sink{"selected": sink}}
			reads, observations := 0, 0
			owner := NewRuntimeOpeningOwner(openingRuntimeReader(func(_ string, consume func(*factorysessions.LiveRuntime) error) error {
				reads++
				if test.missingRuntime {
					return consume(nil)
				}
				return consume(&factorysessions.LiveRuntime{Clock: fixedClock{}, LiveChangeLogger: zap.NewNop()})
			}), func(string) Source { return &sourceStub{} },
				func(_ Source, _ Clock, got Sink, _ ErrorReporter) (Root, error) {
					if test.override != nil && got != test.override {
						t.Fatal("explicit override was lost")
					}
					return nil, cause
				}, registry, test.override, func(Root) { observations++ })
			got, err := owner.Open(ctx, "selected", test.sinkID)
			if got != nil || observations != 0 || reads != test.wantReads {
				t.Fatalf("Open = %v, observations=%d reads=%d", got, observations, reads)
			}
			if test.wantText != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantText) {
					t.Fatalf("error = %v, want %q", err, test.wantText)
				}
			} else if !errors.Is(err, test.wantError) {
				t.Fatalf("error = %v, want %v", err, test.wantError)
			}
		})
	}
}

func assertOpeningErrorLog(t *testing.T, logs *observer.ObservedLogs, id string) {
	t.Helper()
	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("selected error records = %+v", entries)
	}
	fields := entries[0].ContextMap()
	if fields["session_id"] != id || fields["error"] != "selected projection failed" {
		t.Fatalf("selected error fields = %+v", fields)
	}
}

func TestSharedRuntimeOpeningKeepsConcurrentEffectsScoped(t *testing.T) {
	t.Parallel()
	selectedCore, selectedLogs := observer.New(zap.ErrorLevel)
	peerCore, peerLogs := observer.New(zap.ErrorLevel)
	runtimes := map[string]*factorysessions.LiveRuntime{
		"selected": {Clock: fixedClock{now: time.Unix(1, 0)}, LiveChangeLogger: zap.New(selectedCore).With(zap.String("session_id", "selected"))},
		"peer":     {Clock: fixedClock{now: time.Unix(2, 0)}, LiveChangeLogger: zap.New(peerCore).With(zap.String("session_id", "peer"))},
	}
	selectedSink, peerSink := &openingViewSink{}, &openingViewSink{}
	owner := NewRuntimeOpeningOwner(openingRuntimeReader(func(id string, consume func(*factorysessions.LiveRuntime) error) error {
		return consume(runtimes[id])
	}), func(string) Source { return &sourceStub{} },
		func(_ Source, clock Clock, sink Sink, report ErrorReporter) (Root, error) {
			sink.PresentFactoryView(View{ObservedAt: clock.Now()})
			report(errors.New("selected projection failed"))
			return &openingRootStub{}, nil
		}, openingSinkRegistry{sinks: map[factoryvisualization.RuntimeSinkID]Sink{"selected": selectedSink, "peer": peerSink}}, nil, nil)
	for _, test := range []struct {
		id        string
		sink      *openingViewSink
		logs      *observer.ObservedLogs
		timestamp int64
	}{
		{"selected", selectedSink, selectedLogs, 1}, {"peer", peerSink, peerLogs, 2},
	} {
		t.Run(test.id, func(t *testing.T) {
			t.Parallel()
			if root, err := owner.Open(t.Context(), test.id, factoryvisualization.RuntimeSinkID(test.id)); err != nil || root == nil {
				t.Fatalf("Open = (%v, %v)", root, err)
			}
			if test.sink.view.ObservedAt != time.Unix(test.timestamp, 0) {
				t.Fatalf("observed timestamp = %v", test.sink.view.ObservedAt)
			}
			assertOpeningErrorLog(t, test.logs, test.id)
		})
	}
}

type openingViewSink struct{ view View }

func (sink *openingViewSink) PresentFactoryView(view View) { sink.view = view }
