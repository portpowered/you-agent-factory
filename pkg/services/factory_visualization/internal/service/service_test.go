package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/contracts"
	activationlifecycle "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/activation_lifecycle"
	liveviewprojection "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/live_view_projection"
	responseeventpresentation "github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/services/response_event_presentation"
	"github.com/portpowered/infinite-you/pkg/services/factory_visualization/internal/testing/recordingsstub"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

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

type activationOwnerStub struct {
	activate  func(context.Context, activationlifecycle.ActivateRequest) (activationlifecycle.ActivateResult, error)
	join      func(context.Context, activationlifecycle.JoinRequest) (activationlifecycle.JoinResult, error)
	stopDrain func(context.Context, activationlifecycle.StopDrainRequest) (activationlifecycle.StopDrainResult, error)
}

func (activationOwnerStub) Start(context.Context) error { return nil }
func (activationOwnerStub) Stop(context.Context) error  { return nil }
func (activationOwnerStub) Wait(context.Context) error  { return nil }
func (stub activationOwnerStub) Activate(ctx context.Context, req activationlifecycle.ActivateRequest) (activationlifecycle.ActivateResult, error) {
	if stub.activate != nil {
		return stub.activate(ctx, req)
	}
	return activationlifecycle.ActivateResult{}, nil
}
func (stub activationOwnerStub) Join(ctx context.Context, req activationlifecycle.JoinRequest) (activationlifecycle.JoinResult, error) {
	if stub.join != nil {
		return stub.join(ctx, req)
	}
	return activationlifecycle.JoinResult{}, nil
}
func (stub activationOwnerStub) StopDrain(ctx context.Context, req activationlifecycle.StopDrainRequest) (activationlifecycle.StopDrainResult, error) {
	if stub.stopDrain != nil {
		return stub.stopDrain(ctx, req)
	}
	return activationlifecycle.StopDrainResult{}, nil
}
func (activationOwnerStub) RetainedEvents() []factorydefinitions.FactoryEvent { return nil }
func (activationOwnerStub) ReconnectCursor() *factorydefinitions.FactoryEventReconnectCursor {
	return nil
}

type projectionOwnerStub struct {
	observe func(context.Context, liveviewprojection.ObserveRequest) (liveviewprojection.ObserveResult, error)
}

func (projectionOwnerStub) Start(context.Context) error { return nil }
func (projectionOwnerStub) Stop(context.Context) error  { return nil }
func (projectionOwnerStub) Wait(context.Context) error  { return nil }
func (stub projectionOwnerStub) Observe(ctx context.Context, req liveviewprojection.ObserveRequest) (liveviewprojection.ObserveResult, error) {
	if stub.observe != nil {
		return stub.observe(ctx, req)
	}
	return liveviewprojection.ObserveResult{}, nil
}
func (projectionOwnerStub) ReconnectCursor() *factorydefinitions.FactoryEventReconnectCursor {
	return nil
}

// The root tests control output acceptance and drain; real queue ordering,
// backpressure and cleanup are exercised by response_event_presentation tests.
type presentationOwnerStub struct {
	responseeventpresentation.Service
}

func newPresentationOwner() responseeventpresentation.Service { return presentationOwnerStub{} }
func (presentationOwnerStub) OpenBestEffortOutput(w io.Writer) Output {
	return &controlledOutput{writer: w}
}
func (presentationOwnerStub) OpenLosslessOutput(w io.Writer) Output {
	return &controlledOutput{writer: w}
}

type controlledOutput struct {
	writer     io.Writer
	accepted   [][]byte
	enqueueErr error
}

func (out *controlledOutput) Enqueue(payload []byte) error {
	if out.enqueueErr != nil {
		return out.enqueueErr
	}
	out.accepted = append(out.accepted, append([]byte(nil), payload...))
	return nil
}
func (out *controlledOutput) CloseAndDrain() error {
	for _, payload := range out.accepted {
		if _, err := out.writer.Write(appendPresentationLine(payload)); err != nil {
			return err
		}
	}
	out.accepted = nil
	return nil
}
func (out *controlledOutput) WithWriterExclusive(write func(io.Writer) error) error {
	return write(out.writer)
}
func (*controlledOutput) Dropped() int { return 0 }

func requireServiceLifecycleError(t *testing.T, err error, kind LifecycleErrorKind, label string) {
	t.Helper()
	var lifeErr *LifecycleError
	if !errors.As(err, &lifeErr) || lifeErr.Kind != kind {
		t.Fatalf("%s: error = %v, want %s", label, err, kind)
	}
}

func TestServiceRootLifecycleInertConstructionAndTypedActivate(t *testing.T) {
	t.Parallel()
	cause := errors.New("selected lifecycle failure")
	for _, kind := range []activationlifecycle.LifecycleErrorKind{
		activationlifecycle.LifecycleErrorNotActivated,
		activationlifecycle.LifecycleErrorMissingParameters,
		activationlifecycle.LifecycleErrorAlreadyActivated,
	} {
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()
			calls := 0
			activation := activationOwnerStub{
				activate: func(ctx context.Context, req activationlifecycle.ActivateRequest) (activationlifecycle.ActivateResult, error) {
					calls++
					if ctx != t.Context() || req.Mode != activationlifecycle.ActivateModeRetainedThenLive {
						t.Fatal("activation request/context not forwarded")
					}
					return activationlifecycle.ActivateResult{}, &activationlifecycle.LifecycleError{Kind: kind, Cause: cause}
				},
				join: func(context.Context, activationlifecycle.JoinRequest) (activationlifecycle.JoinResult, error) {
					return activationlifecycle.JoinResult{}, &activationlifecycle.LifecycleError{Kind: kind, Cause: cause}
				},
			}
			root, err := assembleRoot(activation, projectionOwnerStub{}, newPresentationOwner(), &sourceStub{}, &recordingsstub.Service{}, fixedClock{}, SinkFunc(func(View) { t.Fatal("unexpected presentation") }), nil)
			if err != nil {
				t.Fatal(err)
			}
			if calls != 0 {
				t.Fatal("construction activated collaborator")
			}
			_, err = root.Activate(t.Context(), ActivateRequest{Mode: ActivateModeRetainedThenLive})
			requireServiceLifecycleError(t, err, LifecycleErrorKind(kind), "Activate")
			if !errors.Is(err, cause) || calls != 1 {
				t.Fatalf("activation: calls=%d error=%v", calls, err)
			}
			_, err = root.Join(t.Context(), JoinRequest{})
			requireServiceLifecycleError(t, err, LifecycleErrorKind(kind), "Join")
		})
	}
	root := &Service{activation: activationOwnerStub{
		activate: func(context.Context, activationlifecycle.ActivateRequest) (activationlifecycle.ActivateResult, error) {
			return activationlifecycle.ActivateResult{State: activationlifecycle.LifecycleStateStarted}, nil
		},
		stopDrain: func(context.Context, activationlifecycle.StopDrainRequest) (activationlifecycle.StopDrainResult, error) {
			return activationlifecycle.StopDrainResult{State: activationlifecycle.LifecycleStateStopped}, nil
		},
	}}
	if result, err := root.Activate(t.Context(), ActivateRequest{Mode: ActivateModeRetainedThenLive}); err != nil || result.State != LifecycleStateStarted {
		t.Fatalf("Activate = (%+v,%v)", result, err)
	}
	if result, err := root.StopDrain(t.Context(), StopDrainRequest{}); err != nil || result.State != LifecycleStateStopped {
		t.Fatalf("StopDrain = (%+v,%v)", result, err)
	}
}

func TestServiceRootObserveDetachedViewAndTypedFailures(t *testing.T) {
	t.Parallel()
	now := time.Unix(9, 0)
	sequence := 3
	request := ObserveRequest{Mode: ObserveModeRetainedThenLive, Reconnect: &ObserveReconnectCursor{AfterEventID: "history", AfterSequence: &sequence}}
	projection := projectionOwnerStub{observe: func(ctx context.Context, req liveviewprojection.ObserveRequest) (liveviewprojection.ObserveResult, error) {
		if ctx != t.Context() || req.Mode != liveviewprojection.ObserveModeRetainedThenLive || req.Reconnect == nil || req.Reconnect.AfterEventID != "history" || *req.Reconnect.AfterSequence != sequence {
			t.Fatalf("projection request=%+v", req)
		}
		req.Reconnect.AfterEventID = "collaborator-change"
		return liveviewprojection.ObserveResult{View: liveviewprojection.ProjectedView{TickCount: 9, RetainedEventCount: 1, ObservedAt: now}}, nil
	}}
	root := &Service{projection: projection}
	result, err := root.Observe(t.Context(), request)
	if err != nil || result.View.TickCount != 9 || result.View.RetainedEventCount != 1 || !result.View.ObservedAt.Equal(now) {
		t.Fatalf("Observe=(%+v,%v)", result, err)
	}
	if request.Reconnect.AfterEventID != "history" {
		t.Fatal("observation mutated caller cursor")
	}
	cause := errors.New("projection unavailable")
	for _, kind := range []liveviewprojection.ProjectionErrorKind{liveviewprojection.ProjectionErrorInvalidInput, liveviewprojection.ProjectionErrorSnapshotUnavailable, liveviewprojection.ProjectionErrorReconstructionFailed} {
		root.projection = projectionOwnerStub{observe: func(context.Context, liveviewprojection.ObserveRequest) (liveviewprojection.ObserveResult, error) {
			return liveviewprojection.ObserveResult{}, &liveviewprojection.ProjectionError{Kind: kind, Cause: cause}
		}}
		_, err := root.Observe(t.Context(), request)
		var projected *ProjectionError
		if !errors.As(err, &projected) || projected.Kind != ProjectionErrorKind(kind) || !errors.Is(err, cause) {
			t.Fatalf("Observe error=%v, want %s and cause", err, kind)
		}
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
	root, err := assembleRoot(activationOwnerStub{}, projectionOwnerStub{}, newPresentationOwner(), &sourceStub{}, &recordingsstub.Service{}, fixedClock{now: time.Unix(1, 0)}, SinkFunc(func(View) {}), nil)
	if err != nil {
		t.Fatal(err)
	}
	return root
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
	// Queue saturation belongs to the presentation component. Here the
	// controlled output proves the root maps its failure to the public error.
	service.presentations[blocked.SessionID].output = &controlledOutput{enqueueErr: contracts.ErrBacklogFull}
	_, err = root.PresentProgress(context.Background(), PresentProgressRequest{
		SessionID: blocked.SessionID,
		Records:   []ProgressRecord{{Payload: []byte("overflow")}},
	})
	if !errors.As(err, &presErr) || presErr.Kind != PresentationErrorBackpressureRejected {
		t.Fatalf("PresentProgress backpressure: error = %v, want BackpressureRejected", err)
	}
}

// Root delegation does not read runtime facts; unexpected calls fail through
// the unimplemented embedded port rather than exercising another component.
type sourceStub struct{ Source }

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

type scopeSourceStub struct {
	subscribeHook func()
}

func (s scopeSourceStub) SubscribeFactoryEvents(
	context.Context,
	*factorydefinitions.FactoryEventReconnectCursor,
	factorydefinitions.FactoryEventReconnectScope,
) (*factorydefinitions.FactoryEventStream, error) {
	if s.subscribeHook != nil {
		s.subscribeHook()
	}
	return &factorydefinitions.FactoryEventStream{
		Events: make(chan factorydefinitions.FactoryEvent),
	}, nil
}

func (s scopeSourceStub) GetRuntimeSnapshotFacts(context.Context) (*liveviewprojection.RuntimeSnapshotFacts, error) {
	return &liveviewprojection.RuntimeSnapshotFacts{}, nil
}

type scopeClock struct{}

func (scopeClock) Now() time.Time { return time.Unix(1, 0) }

func TestScopeOpeningRejectsMissingResources(t *testing.T) {
	t.Parallel()

	clock := scopeClock{}
	sink := factoryvisualization.SinkFunc(func(factoryvisualization.View) {})
	projections := &recordingsstub.Service{}
	source := scopeSourceStub{}
	tests := []struct {
		name string
		new  func() (*Service, error)
		want string
	}{
		{
			name: "source",
			new: func() (*Service, error) {
				return openScopeForTest(nil, projections, clock, sink, nil)
			},
			want: "event source is required",
		},
		{
			name: "projections",
			new: func() (*Service, error) {
				return openScopeForTest(source, nil, clock, sink, nil)
			},
			want: "recordings service is required",
		},
		{
			name: "clock",
			new: func() (*Service, error) {
				return openScopeForTest(source, projections, nil, sink, nil)
			},
			want: "clock is required",
		},
		{
			name: "sink",
			new: func() (*Service, error) {
				return openScopeForTest(source, projections, clock, nil, nil)
			},
			want: "presentation sink is required",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			root, err := test.new()
			if root != nil {
				t.Fatal("ScopeOpening() returned non-nil root, want nil")
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ScopeOpening() error = %v, want %q", err, test.want)
			}
		})
	}
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

func mustNewScopeRoot(t *testing.T) factoryvisualization.Root {
	t.Helper()
	root, err := openScopeForTest(
		scopeSourceStub{},
		&recordingsstub.Service{},
		scopeClock{},
		factoryvisualization.SinkFunc(func(factoryvisualization.View) {}),
		nil,
	)
	if err != nil {
		t.Fatalf("ScopeOpening() error = %v", err)
	}
	if root == nil {
		t.Fatal("ScopeOpening() returned nil root")
	}
	return root
}

func TestScopeOpeningServesPublishedPeerBehavior(t *testing.T) {
	t.Parallel()

	root := mustNewScopeRoot(t)

	_, err := root.OpenPresentation(context.Background(), factoryvisualization.OpenPresentationRequest{})
	requirePresentationError(
		t,
		err,
		factoryvisualization.PresentationErrorInvalidInput,
		"OpenPresentation missing parameters",
	)

	opened, err := root.OpenPresentation(context.Background(), factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryBestEffort,
	})
	if err != nil {
		t.Fatalf("OpenPresentation best-effort: error = %v", err)
	}
	if opened.SessionID == "" {
		t.Fatalf("OpenPresentation result = %#v, want non-empty session id", opened)
	}
	if opened.Mode != factoryvisualization.PresentationDeliveryBestEffort {
		t.Fatalf("OpenPresentation mode = %q, want %q", opened.Mode, factoryvisualization.PresentationDeliveryBestEffort)
	}
}

func TestNewRootConstructsInertRoot(t *testing.T) {
	t.Parallel()

	subscribeCalls := 0
	presentCalls := 0
	projectionReads := 0
	source := scopeSourceStub{
		subscribeHook: func() {
			subscribeCalls++
			panic("event subscription started during inert construction")
		},
	}
	projections := newScopeRecordingProjectionStub(func() {
		projectionReads++
		panic("projection read during inert construction")
	})
	sink := factoryvisualization.SinkFunc(func(factoryvisualization.View) {
		presentCalls++
		panic("presentation sink invoked during inert construction")
	})

	root, err := openScopeForTest(
		source,
		projections,
		scopeClock{},
		sink,
		nil,
	)
	if err != nil {
		t.Fatalf("ScopeOpening() error = %v", err)
	}
	if root == nil {
		t.Fatal("ScopeOpening() returned nil root")
	}
	var peer factoryvisualization.Root = root
	if subscribeCalls != 0 || presentCalls != 0 || projectionReads != 0 {
		t.Fatalf(
			"ScopeOpening() side effects: subscribe=%d present=%d projection=%d, want inert construction",
			subscribeCalls, presentCalls, projectionReads,
		)
	}

	_, err = peer.Join(context.Background(), factoryvisualization.JoinRequest{})
	requireLifecycleError(t, err, factoryvisualization.LifecycleErrorNotActivated, "Join before Activate")
	if subscribeCalls != 0 || presentCalls != 0 || projectionReads != 0 {
		t.Fatal("Join before Activate must not subscribe, present, or read projections")
	}
}

type scopeRecordingProjectionStub struct {
	*recordingsstub.Service
	reconstructHook func()
}

func newScopeRecordingProjectionStub(hook func()) *scopeRecordingProjectionStub {
	stub := &scopeRecordingProjectionStub{Service: &recordingsstub.Service{}, reconstructHook: hook}
	stub.ReconstructWorldStateFn = func(request recordings.ReconstructWorldStateRequest) (recordings.ReconstructWorldStateResult, error) {
		if stub.reconstructHook != nil {
			stub.reconstructHook()
		}
		return recordings.ReconstructWorldStateResult{
			WorldState: recordings.WorldStateView{
				SchemaVersion: recordings.WorldStateViewSchemaV1,
				Payload:       `{"topology":{}}`,
			},
		}, nil
	}
	return stub
}

func openScopeForTest(source factoryvisualization.Source, peer recordings.Service, clock factoryvisualization.Clock, sink factoryvisualization.Sink, reportError factoryvisualization.ErrorReporter) (*Service, error) {
	owner := NewScopeOwner(
		func(activationlifecycle.EventSource, activationlifecycle.Clock, activationlifecycle.ViewSink, activationlifecycle.ErrorReporter) activationlifecycle.Service {
			return activationOwnerStub{join: func(context.Context, activationlifecycle.JoinRequest) (activationlifecycle.JoinResult, error) {
				return activationlifecycle.JoinResult{}, &activationlifecycle.LifecycleError{Kind: activationlifecycle.LifecycleErrorNotActivated}
			}}
		},
		func(func() []factorydefinitions.FactoryEvent, liveviewprojection.Source, liveviewprojection.Clock, liveviewprojection.Sink, liveviewprojection.ErrorReporter) liveviewprojection.Service {
			return projectionOwnerStub{}
		},
		newPresentationOwner(), peer,
	)
	return owner.Open(source, clock, sink, reportError)
}

type activationAdapterSourceStub struct {
	Source
	subscribe func(context.Context, *factorydefinitions.FactoryEventReconnectCursor, factorydefinitions.FactoryEventReconnectScope) (*factorydefinitions.FactoryEventStream, error)
	facts     *liveviewprojection.RuntimeSnapshotFacts
	err       error
}

func (stub activationAdapterSourceStub) SubscribeFactoryEvents(ctx context.Context, cursor *factorydefinitions.FactoryEventReconnectCursor, scope factorydefinitions.FactoryEventReconnectScope) (*factorydefinitions.FactoryEventStream, error) {
	return stub.subscribe(ctx, cursor, scope)
}
func (stub activationAdapterSourceStub) GetRuntimeSnapshotFacts(context.Context) (*liveviewprojection.RuntimeSnapshotFacts, error) {
	return stub.facts, stub.err
}

func TestActivationEventSourceAdapterForwardsSelectedReads(t *testing.T) {
	t.Parallel()
	cursor := &factorydefinitions.FactoryEventReconnectCursor{AfterEventID: "selected-event"}
	stream := &factorydefinitions.FactoryEventStream{History: []factorydefinitions.FactoryEvent{{Id: "retained"}}}
	cause := errors.New("selected read failure")
	adapter := ActivationEventSourceAdapter{Source: activationAdapterSourceStub{subscribe: func(ctx context.Context, got *factorydefinitions.FactoryEventReconnectCursor, scope factorydefinitions.FactoryEventReconnectScope) (*factorydefinitions.FactoryEventStream, error) {
		if ctx != t.Context() || got != cursor {
			t.Fatal("subscription did not forward selected context/cursor")
		}
		return stream, cause
	}}}
	got, err := adapter.SubscribeFactoryEvents(t.Context(), cursor, factorydefinitions.FactoryEventReconnectScope{})
	if got != stream || !errors.Is(err, cause) {
		t.Fatalf("subscription=(%+v,%v)", got, err)
	}
	for _, test := range []struct {
		name  string
		facts *liveviewprojection.RuntimeSnapshotFacts
		err   error
	}{
		{name: "missing"},
		{name: "failure", err: cause},
		{name: "selected", facts: &liveviewprojection.RuntimeSnapshotFacts{RuntimeObservation: liveviewprojection.RuntimeObservation{TickCount: 9}, ActiveThrottlePauses: []factorydefinitions.ActiveThrottlePause{{}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			observation, err := (ActivationEventSourceAdapter{Source: activationAdapterSourceStub{facts: test.facts, err: test.err}}).GetEngineObservation(t.Context())
			if !errors.Is(err, test.err) {
				t.Fatalf("observation error=%v", err)
			}
			if test.facts == nil {
				if observation != nil {
					t.Fatalf("observation=%+v, want nil", observation)
				}
				return
			}
			if observation == nil || observation.TickCount != 9 || len(observation.ActiveThrottlePauses) != 1 {
				t.Fatalf("observation=%+v", observation)
			}
		})
	}
	if _, err := (ActivationEventSourceAdapter{}).GetEngineObservation(t.Context()); err == nil {
		t.Fatal("missing source observation succeeded")
	}
	if _, err := (ActivationEventSourceAdapter{}).SubscribeFactoryEvents(t.Context(), nil, factorydefinitions.FactoryEventReconnectScope{}); err == nil {
		t.Fatal("missing source subscription succeeded")
	}
}

func TestActivationViewSinkAdapterPreservesSelectedView(t *testing.T) {
	t.Parallel()
	selected := activationlifecycle.View{EngineObservation: activationlifecycle.EngineObservation{TickCount: 9}, RenderData: recordings.SimpleDashboardRenderData{InFlightDispatchCount: 7}, ObservedAt: time.Unix(9, 0)}
	var got View
	adapter := ActivationViewSinkAdapter{Sink: SinkFunc(func(view View) { got = view })}
	adapter.PresentFactoryView(selected)
	if got.Runtime.TickCount != 9 || got.RenderData.InFlightDispatchCount != 7 || !got.ObservedAt.Equal(selected.ObservedAt) {
		t.Fatalf("view=%+v", got)
	}
	(ActivationViewSinkAdapter{}).PresentFactoryView(selected)
}
