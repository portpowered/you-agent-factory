package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
	factoryvisualizationwire "github.com/portpowered/infinite-you/pkg/services/factory_visualization/wire"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestNewRequiresPresentation(t *testing.T) {
	t.Parallel()

	presentation := factoryvisualizationwire.NewResponsePresentation()
	if service := visualizationcli.New(nil, presentation); service == nil {
		t.Fatal("New(nil, presentation) = nil, want Visualization CLI service")
	}
	if service := visualizationcli.New(&fakeRootPeer{}, nil); service != nil {
		t.Fatalf("New(root, nil) = %T, want nil", service)
	}
	if service := visualizationcli.New(&fakeRootPeer{}, presentation); service == nil {
		t.Fatal("New(root, presentation) = nil, want Visualization CLI service")
	}
}

func TestNewFromPresentationConstructsPresentationOnlyAdapter(t *testing.T) {
	t.Parallel()

	if service := visualizationcli.NewFromPresentation(nil); service != nil {
		t.Fatalf("NewFromPresentation(nil) = %T, want nil", service)
	}
	service := visualizationcli.NewFromPresentation(factoryvisualizationwire.NewResponsePresentation())
	if service == nil {
		t.Fatal("NewFromPresentation(presentation) = nil, want Visualization CLI service")
	}
	_, err := service.OpenPresentationSession(context.Background(), factoryvisualization.OpenPresentationRequest{})
	if err == nil || !strings.Contains(err.Error(), "visualization root is required") {
		t.Fatalf("OpenPresentationSession error = %v, want missing root failure", err)
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
		Runtime:    factoryvisualization.RuntimeObservation{TickCount: 3},
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
			Sequence:        1,
			SessionSequence: intPtr(1),
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
	if !strings.Contains(output.String(), "factory started") {
		t.Fatalf("output = %q, want human Factory Event line", output.String())
	}
	if !strings.Contains(output.String(), "done") {
		t.Fatalf("output = %q, want terminal primary result", output.String())
	}
}

func TestFormatHumanWorkAccepted_UsesContentForGeneratedWorkName(t *testing.T) {
	payload, err := json.Marshal(work.WorkRequestEventPayload{
		Works: []work.WorkRequestEventWork{{
			Name:   "work-1",
			WorkID: "request-work-1",
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "Add a focused ACP regression test",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	presentation := factoryvisualizationwire.NewResponsePresentation()
	service := visualizationcli.New(nil, presentation)
	var output bytes.Buffer
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output: &output, InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("open renderer: %v", err)
	}
	renderer.PresentFactoryEvents([]interfaces.FactoryEvent{{
		Type: interfaces.FactoryEventTypeWorkRequest, Payload: payload,
	}})
	renderer.StopProgressRendering()
	if got, want := strings.TrimSpace(output.String()), "[0] work accepted: Add a focused ACP regression test"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestHumanFactoryEventRenderer_PresentsBatchWorkAndDispatchWorkIDs(t *testing.T) {
	t.Parallel()

	batchPayload, err := json.Marshal(work.WorkRequestEventPayload{
		Works: []work.WorkRequestEventWork{
			{WorkID: "work-1", Name: "first", Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "first task"}}},
			{WorkID: "work-2", Name: "second", Content: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "second task"}}},
		},
		Relations: []work.WorkRequestEventRelation{{
			Type: work.WorkRelationDependsOn, SourceWorkName: "second", TargetWorkID: "work-1",
		}},
	})
	if err != nil {
		t.Fatalf("marshal batch payload: %v", err)
	}
	workIDs := []string{"work-1", "work-2"}
	dispatchID := "dispatch-1"
	requestPayload, err := json.Marshal(interfaces.DispatchRequestEventPayload{
		TransitionID: "execute", Inputs: []interfaces.DispatchConsumedWorkRef{{WorkID: "work-1"}, {WorkID: "work-2"}},
	})
	if err != nil {
		t.Fatalf("marshal dispatch request: %v", err)
	}
	responsePayload, err := json.Marshal(workerexecution.DispatchResponseEventPayload{TransitionID: "execute", Outcome: workerexecution.OutcomeAccepted})
	if err != nil {
		t.Fatalf("marshal dispatch response: %v", err)
	}
	service := visualizationcli.New(nil, factoryvisualizationwire.NewResponsePresentation())
	var output bytes.Buffer
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output: &output, InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("open renderer: %v", err)
	}
	renderer.PresentFactoryEvents([]interfaces.FactoryEvent{
		{Type: interfaces.FactoryEventTypeWorkRequest, Payload: batchPayload},
		{Type: interfaces.FactoryEventTypeDispatchRequest, Payload: requestPayload, Context: interfaces.FactoryEventContext{DispatchID: &dispatchID}},
		{Type: interfaces.FactoryEventTypeDispatchResponse, Payload: responsePayload, Context: interfaces.FactoryEventContext{DispatchID: &dispatchID, WorkIDs: &workIDs}},
	})
	renderer.StopProgressRendering()
	want := "[0] work accepted: 2 items\n- work-1 (first): first task\n- work-2 (second): second task\n- second depends on -> work-1\n" +
		"[0] workstation started: execute (work-1, work-2)\n[0] workstation completed: execute (work-1, work-2)\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestHumanFactoryEventRenderer_TTYProgressUsesInjectedTicksAndStops(t *testing.T) {
	t.Parallel()

	service := visualizationcli.New(nil, factoryvisualizationwire.NewResponsePresentation())
	var output, progress bytes.Buffer
	ticks := make(chan time.Time, 1)
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output: &output, ProgressOutput: &progress, ProgressIsTTY: true, ProgressTicks: ticks,
		InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("open renderer: %v", err)
	}
	dispatchID := "worker-a"
	renderer.PresentFactoryEvents([]interfaces.FactoryEvent{{
		Type: interfaces.FactoryEventTypeDispatchRequest, Context: interfaces.FactoryEventContext{DispatchID: &dispatchID},
	}})
	ticks <- time.Unix(1, 0)
	renderer.PresentFactoryEvents([]interfaces.FactoryEvent{{
		Type: interfaces.FactoryEventTypeDispatchResponse, Context: interfaces.FactoryEventContext{DispatchID: &dispatchID},
	}})
	renderer.StopProgressRendering()
	if got := progress.String(); !strings.Contains(got, "\x1b[") || !strings.Contains(got, "⠋") || !strings.HasSuffix(got, "\r\x1b[2K") {
		t.Fatalf("progress = %q, want colored spinner and terminal clear", got)
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
		Status:        interfaces.InvocationTerminalStatusCompleted,
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
	return visualizationcli.New(&fakeRootPeer{}, factoryvisualizationwire.NewResponsePresentation())
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
	ctx context.Context,
	req factoryvisualization.OpenPresentationRequest,
) (factoryvisualization.OpenPresentationResult, error) {
	if err := ctx.Err(); err != nil {
		return factoryvisualization.OpenPresentationResult{}, err
	}
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
	ctx context.Context,
	req factoryvisualization.PresentProgressRequest,
) (factoryvisualization.PresentProgressResult, error) {
	if err := ctx.Err(); err != nil {
		return factoryvisualization.PresentProgressResult{}, err
	}
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
	ctx context.Context,
	req factoryvisualization.FinalizePresentationRequest,
) (factoryvisualization.FinalizePresentationResult, error) {
	if err := ctx.Err(); err != nil {
		return factoryvisualization.FinalizePresentationResult{}, err
	}
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
	return factoryvisualizationwire.NewResponsePresentation().OpenBestEffortOutput(writer)
}

func (r *recordingResponsePresentation) OpenLosslessOutput(writer io.Writer) factoryvisualization.Output {
	return factoryvisualizationwire.NewResponsePresentation().OpenLosslessOutput(writer)
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
	return factoryvisualizationwire.NewResponsePresentation().OpenBestEffortFactoryEventStream(writer, encode)
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
	return factoryvisualizationwire.NewResponsePresentation().OpenLosslessFactoryEventStream(writer, encode)
}
