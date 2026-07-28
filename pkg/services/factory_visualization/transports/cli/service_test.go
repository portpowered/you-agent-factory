package cli_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestNewRequiresVisualizationRootAndPresentation(t *testing.T) {
	t.Parallel()

	presentation := factoryvisualization.NewResponsePresentation()
	if service := visualizationcli.New(nil, presentation); service != nil {
		t.Fatalf("New(nil, presentation) = %T, want nil", service)
	}
	if service := visualizationcli.New(&fakeRootPeer{}, nil); service != nil {
		t.Fatalf("New(root, nil) = %T, want nil", service)
	}
	if service := visualizationcli.New(&fakeRootPeer{}, presentation); service == nil {
		t.Fatal("New(root, presentation) = nil, want Visualization CLI service")
	}
}

func TestBuildVisualizationSink_SuppressesDashboardRendering(t *testing.T) {
	t.Parallel()

	service := newTestService()
	var output bytes.Buffer
	if sink := service.BuildVisualizationSink(visualizationcli.SinkConfig{
		Output:                     &output,
		SuppressDashboardRendering: true,
	}); sink != nil {
		t.Fatalf("sink = %T, want nil when dashboard rendering is suppressed", sink)
	}
}

func TestBuildVisualizationSink_MissingWriterProducesNoSink(t *testing.T) {
	t.Parallel()

	service := newTestService()
	if sink := service.BuildVisualizationSink(visualizationcli.SinkConfig{}); sink != nil {
		t.Fatalf("sink = %T, want nil when output writer is missing", sink)
	}
}

func TestBuildVisualizationSink_PresentsSimpleDashboard(t *testing.T) {
	t.Parallel()

	service := newTestService()
	var output bytes.Buffer
	sink := service.BuildVisualizationSink(visualizationcli.SinkConfig{Output: &output})
	if sink == nil {
		t.Fatal("sink = nil, want dashboard sink")
	}
	sink.PresentFactoryView(factoryvisualization.View{
		Runtime: factoryvisualization.RuntimeObservation{TickCount: 3},
		ObservedAt: time.Unix(1, 0),
	})
	if got := output.String(); !strings.Contains(got, "Tick: 3") {
		t.Fatalf("dashboard output = %q, want tick snapshot", got)
	}
}

func TestOpenFactoryEventRenderer_RejectsMissingWriter(t *testing.T) {
	t.Parallel()

	service := newTestService()
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if renderer != nil {
		t.Fatalf("renderer = %T, want nil", renderer)
	}
	if err == nil || !strings.Contains(err.Error(), "output writer is required") {
		t.Fatalf("error = %v, want missing output writer failure", err)
	}
}

func TestOpenFactoryEventRenderer_ReturnsNilForNonResponseStreamMode(t *testing.T) {
	t.Parallel()

	service := newTestService()
	var output bytes.Buffer
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output:               &output,
		InvocationOutputMode: "primary-result",
	})
	if err != nil {
		t.Fatalf("OpenFactoryEventRenderer: %v", err)
	}
	if renderer != nil {
		t.Fatalf("renderer = %T, want nil for non-response-stream mode", renderer)
	}
}

func TestOpenFactoryEventRenderer_HumanStreamUsesPresentationCollaborator(t *testing.T) {
	t.Parallel()

	root := &fakeRootPeer{}
	presentation := &recordingResponsePresentation{}
	service := visualizationcli.New(root, presentation)
	if service == nil {
		t.Fatal("New() = nil")
	}

	var output bytes.Buffer
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output:               &output,
		InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("OpenFactoryEventRenderer: %v", err)
	}
	renderer.PresentFactoryEvents([]interfaces.FactoryEvent{{
		Type: interfaces.FactoryEventTypeSessionStarted,
		Context: interfaces.FactoryEventContext{
			Sequence:         1,
			SessionSequence:  intPtr(1),
		},
	}})
	if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status:        interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "done"}},
	}); err != nil {
		t.Fatalf("WriteFinalInvocationResult: %v", err)
	}
	if !presentation.openedBestEffortStream {
		t.Fatal("presentation collaborator was not used for human stream")
	}
	if !strings.Contains(output.String(), "Factory Session started") {
		t.Fatalf("output = %q, want human Factory Event line", output.String())
	}
	if !strings.Contains(output.String(), "done") {
		t.Fatalf("output = %q, want terminal primary result", output.String())
	}
}

func TestOpenFactoryEventRenderer_JSONStreamUsesLosslessPresentation(t *testing.T) {
	t.Parallel()

	presentation := &recordingResponsePresentation{}
	service := visualizationcli.New(&fakeRootPeer{}, presentation)
	var output bytes.Buffer
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output:               &output,
		JSON:                 true,
		InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("OpenFactoryEventRenderer: %v", err)
	}
	renderer.PresentFactoryEvents([]interfaces.FactoryEvent{{
		Id:   "event-1",
		Type: interfaces.FactoryEventTypeSessionStarted,
		Context: interfaces.FactoryEventContext{
			Sequence:        1,
			SessionSequence: intPtr(1),
		},
		Payload: []byte(`{}`),
	}})
	if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status: interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "ok"}},
	}); err != nil {
		t.Fatalf("WriteFinalInvocationResult: %v", err)
	}
	if !presentation.openedLosslessStream {
		t.Fatal("presentation collaborator was not used for JSON stream")
	}
	if !strings.Contains(output.String(), `"recordType":"factory_event"`) {
		t.Fatalf("output = %q, want JSON factory_event record", output.String())
	}
	if !strings.Contains(output.String(), `"recordType":"invocation_result"`) {
		t.Fatalf("output = %q, want JSON invocation_result record", output.String())
	}
}

func TestOpenPresentationSession_MapsRootRejection(t *testing.T) {
	t.Parallel()

	service := newTestService()
	_, err := service.OpenPresentationSession(context.Background(), factoryvisualization.OpenPresentationRequest{})
	var presentationErr *factoryvisualization.PresentationError
	if !errors.As(err, &presentationErr) ||
		presentationErr.Kind != factoryvisualization.PresentationErrorInvalidInput {
		t.Fatalf("error = %v, want typed invalid-input presentation failure", err)
	}
}

func TestFinalizePresentationSession_MapsFinalizeWithoutWriterFailure(t *testing.T) {
	t.Parallel()

	service := newTestService()
	openResult, err := service.OpenPresentationSession(context.Background(), factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryLossless,
	})
	if err != nil {
		t.Fatalf("OpenPresentationSession: %v", err)
	}
	_, err = service.FinalizePresentationSession(context.Background(), factoryvisualization.FinalizePresentationRequest{
		SessionID: openResult.SessionID,
	})
	var presentationErr *factoryvisualization.PresentationError
	if !errors.As(err, &presentationErr) ||
		presentationErr.Kind != factoryvisualization.PresentationErrorFinalizeWithoutWriter {
		t.Fatalf("error = %v, want finalize-without-writer failure", err)
	}
}

func newTestService() visualizationcli.Service {
	return visualizationcli.New(&fakeRootPeer{}, factoryvisualization.NewResponsePresentation())
}

func intPtr(value int) *int {
	return &value
}

type fakeRootPeer struct {
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

func (f *fakeRootPeer) Activate(
	_ context.Context,
	_ factoryvisualization.ActivateRequest,
) (factoryvisualization.ActivateResult, error) {
	return factoryvisualization.ActivateResult{}, errors.New("not implemented")
}

func (f *fakeRootPeer) Join(
	_ context.Context,
	_ factoryvisualization.JoinRequest,
) (factoryvisualization.JoinResult, error) {
	return factoryvisualization.JoinResult{}, errors.New("not implemented")
}

func (f *fakeRootPeer) StopDrain(
	_ context.Context,
	_ factoryvisualization.StopDrainRequest,
) (factoryvisualization.StopDrainResult, error) {
	return factoryvisualization.StopDrainResult{}, nil
}

func (f *fakeRootPeer) Observe(
	_ context.Context,
	_ factoryvisualization.ObserveRequest,
) (factoryvisualization.ObserveResult, error) {
	return factoryvisualization.ObserveResult{}, errors.New("not implemented")
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
	if f.presentations == nil {
		f.presentations = map[factoryvisualization.PresentationSessionID]*fakePresentationSession{}
	}
	f.nextPresentationID++
	id := factoryvisualization.PresentationSessionID("peer-presentation-" + string(rune('a'+f.nextPresentationID-1)))
	capacity := 0
	if req.Mode == factoryvisualization.PresentationDeliveryBestEffort {
		capacity = 64
	}
	f.presentations[id] = &fakePresentationSession{mode: req.Mode, capacity: capacity}
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
			Kind: factoryvisualization.PresentationErrorEnqueueAfterClose,
		}
	}
	accepted := 0
	for _, record := range req.Records {
		if session.capacity > 0 && session.queued >= session.capacity {
			return factoryvisualization.PresentProgressResult{AcceptedCount: accepted}, &factoryvisualization.PresentationError{
				Kind: factoryvisualization.PresentationErrorBackpressureRejected,
			}
		}
		session.records = append(session.records, append([]byte(nil), record.Payload...))
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
	if req.Terminal == nil {
		session.finalized = true
		session.closed = true
		return factoryvisualization.FinalizePresentationResult{}, &factoryvisualization.PresentationError{
			Kind: factoryvisualization.PresentationErrorFinalizeWithoutWriter,
		}
	}
	session.finalized = true
	session.closed = true
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
	return factoryvisualization.ClosePresentationResult{}, nil
}

func (f *fakeRootPeer) presentation(
	id factoryvisualization.PresentationSessionID,
) (*fakePresentationSession, error) {
	if f.presentations == nil {
		return nil, &factoryvisualization.PresentationError{
			Kind: factoryvisualization.PresentationErrorInvalidInput,
		}
	}
	session, ok := f.presentations[id]
	if !ok {
		return nil, &factoryvisualization.PresentationError{
			Kind: factoryvisualization.PresentationErrorInvalidInput,
		}
	}
	return session, nil
}

type recordingResponsePresentation struct {
	openedBestEffortStream bool
	openedLosslessStream   bool
}

func (r *recordingResponsePresentation) OpenBestEffortOutput(writer io.Writer) factoryvisualization.Output {
	return factoryvisualization.NewResponsePresentation().OpenBestEffortOutput(writer)
}

func (r *recordingResponsePresentation) OpenLosslessOutput(writer io.Writer) factoryvisualization.Output {
	return factoryvisualization.NewResponsePresentation().OpenLosslessOutput(writer)
}

func (r *recordingResponsePresentation) OpenBestEffortFactoryEventStream(
	writer io.Writer,
	encode factoryvisualization.FactoryEventEncoder,
) interface {
	PresentFactoryEvents([]interfaces.FactoryEvent)
	Finalize(factoryvisualization.FinalResponseWriter) (bool, error)
	CloseAndDrain() error
} {
	r.openedBestEffortStream = true
	return factoryvisualization.NewResponsePresentation().OpenBestEffortFactoryEventStream(writer, encode)
}

func (r *recordingResponsePresentation) OpenLosslessFactoryEventStream(
	writer io.Writer,
	encode factoryvisualization.FactoryEventEncoder,
) interface {
	PresentFactoryEvents([]interfaces.FactoryEvent)
	Finalize(factoryvisualization.FinalResponseWriter) (bool, error)
	CloseAndDrain() error
} {
	r.openedLosslessStream = true
	return factoryvisualization.NewResponsePresentation().OpenLosslessFactoryEventStream(writer, encode)
}
