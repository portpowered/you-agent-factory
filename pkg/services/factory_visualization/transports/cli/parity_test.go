package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	visualizationcli "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/cli"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const (
	parityResponseStreamPrimaryResultHeader     = "--- primary result ---"
	parityResponseStreamInvocationOutcomeHeader = "--- invocation outcome ---"
)

func TestAdapterParity_DashboardSinkPreservesAcceptedNoOutputOutcomes(t *testing.T) {
	t.Parallel()

	service := newTestService()
	var output bytes.Buffer

	if sink := service.BuildVisualizationSink(visualizationcli.SinkConfig{
		Output:                     &output,
		SuppressDashboardRendering: true,
	}); sink != nil {
		t.Fatalf("suppressed sink = %T, want nil", sink)
	}
	if sink := service.BuildVisualizationSink(visualizationcli.SinkConfig{}); sink != nil {
		t.Fatalf("missing-writer sink = %T, want nil", sink)
	}
	if output.Len() != 0 {
		t.Fatalf("no-output paths wrote %q", output.String())
	}
}

func TestAdapterParity_DashboardSinkRendersLiveViewBeforeTerminalWork(t *testing.T) {
	t.Parallel()

	service := newTestService()
	var output bytes.Buffer
	sink := service.BuildVisualizationSink(visualizationcli.SinkConfig{Output: &output})
	if sink == nil {
		t.Fatal("sink = nil, want dashboard sink")
	}

	sink.PresentFactoryView(factoryvisualization.View{
		Runtime: factoryvisualization.RuntimeObservation{
			TickCount:     7,
			FactoryState:  "RUNNING",
			RuntimeStatus: "ACTIVE",
		},
		ObservedAt: time.Unix(42, 0).UTC(),
	})
	got := output.String()
	for _, want := range []string{"Factory:", "Tick: 7", "RUNNING"} {
		if !strings.Contains(got, want) {
			t.Fatalf("dashboard output = %q, want %q", got, want)
		}
	}
}

func TestAdapterParity_HumanFactoryEventRendererWritesTerminalSuccessAndFailureLast(t *testing.T) {
	t.Parallel()

	t.Run("success after lifecycle", func(t *testing.T) {
		var output bytes.Buffer
		renderer := openAdapterHumanFactoryEventRenderer(t, &output)
		renderer.PresentFactoryEvents(canonicalAdapterJavaScriptFactoryEvents()[:1])
		if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
			Status: interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText, Text: "complete",
			}},
		}); err != nil {
			t.Fatalf("write terminal success: %v", err)
		}
		if got := output.String(); !strings.HasSuffix(got, parityResponseStreamPrimaryResultHeader+"\ncomplete") {
			t.Fatalf("success output does not end with primary result: %q", got)
		}
	})

	t.Run("failure includes public terminal context", func(t *testing.T) {
		var output bytes.Buffer
		renderer := openAdapterHumanFactoryEventRenderer(t, &output)
		if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
			Status:    interfaces.InvocationTerminalStatusFailed,
			ErrorCode: "WORK_FAILED", Message: "worker stopped",
			SessionID: "session-1", WorkID: "work-1", WorkName: "research", WorkState: "FAILED",
		}); err != nil {
			t.Fatalf("write terminal failure: %v", err)
		}
		want := parityResponseStreamInvocationOutcomeHeader + "\n" +
			"status: FAILED\nerror: WORK_FAILED\nmessage: worker stopped\n" +
			"session: session-1\nworkId: work-1\nworkName: research\nworkState: FAILED\n"
		if got := output.String(); got != want {
			t.Fatalf("failure output = %q, want %q", got, want)
		}
	})
}

func TestAdapterParity_JSONFactoryEventRendererFinalizesTerminalRecordOnce(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	renderer := openAdapterJSONFactoryEventRenderer(t, &output)
	result := apisurface.FactoryInvocationResult{
		Status: interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{
			Type: work.WorkContentPartTypeText, Text: "complete",
		}},
	}
	if err := renderer.WriteFinalInvocationResult(result); err != nil {
		t.Fatalf("write terminal record: %v", err)
	}
	if err := renderer.WriteFinalInvocationResult(result); err == nil {
		t.Fatal("duplicate terminal record write succeeded")
	}
	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != 1 || !strings.Contains(lines[0], `"recordType":"invocation_result"`) {
		t.Fatalf("terminal output = %q, want one invocation_result record", output.String())
	}
}

func TestAdapterParity_HumanFactoryEventFailuresAreUnderstandable(t *testing.T) {
	t.Parallel()

	events := []interfaces.FactoryEvent{
		canonicalAdapterFactoryEventWithPayload(1, interfaces.FactoryEventTypeInferenceResponse, workerexecution.InferenceResponseEventPayload{
			Attempt: 2, Outcome: workerexecution.InferenceOutcomeFailed,
			FailureDetail: &workerexecution.InferenceResponseFailureDetail{Message: "model request timed out"},
		}),
		canonicalAdapterFactoryEventWithPayload(2, interfaces.FactoryEventTypeDispatchResponse, workerexecution.DispatchResponseEventPayload{
			TransitionID: "release review", Outcome: workerexecution.OutcomeFailed,
			FailureDetail: &workerexecution.FailureDetail{Message: "worker timed out"},
		}),
	}
	var output strings.Builder
	renderer := openAdapterHumanFactoryEventRenderer(t, &output)
	renderer.PresentFactoryEvents(events)
	renderer.StopProgressRendering()
	want := "[1] inference failed (attempt 2) — model request timed out\n" +
		"[2] workstation failed: release review — worker timed out\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestAdapterParity_JSONFactoryEventRendererEmitsDiscriminatedSafeNDJSON(t *testing.T) {
	t.Parallel()

	events := canonicalAdapterJavaScriptFactoryEvents()
	var output strings.Builder
	renderer := openAdapterJSONFactoryEventRenderer(t, &output)
	renderer.PresentFactoryEvents(events)
	if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
		RequestID: "request-ndjson", Status: interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "complete"}},
	}); err != nil {
		t.Fatalf("writeFinalInvocationResult: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if len(lines) != len(events)+1 {
		t.Fatalf("NDJSON records = %d, want %d:\n%s", len(lines), len(events)+1, output.String())
	}
	for index, line := range lines[:len(events)] {
		assertAdapterFactoryEventNDJSONRecord(t, line, events[index], index)
	}
	if !strings.Contains(lines[len(lines)-1], `"recordType":"invocation_result"`) {
		t.Fatalf("terminal record = %q", lines[len(lines)-1])
	}
}

func TestAdapterParity_OpenFactoryEventRendererRejectsMissingCollaborators(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		open func() error
	}{
		{
			name: "human output",
			open: func() error {
				_, err := newTestService().OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
					InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
				})
				return err
			},
		},
		{
			name: "json output",
			open: func() error {
				_, err := newTestService().OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
					InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
					JSON:                 true,
				})
				return err
			},
		},
		{
			name: "missing presentation constructor",
			open: func() error {
				if service := visualizationcli.New(&fakeRootPeer{}, nil); service != nil {
					return fmt.Errorf("unexpected service %T", service)
				}
				return errors.New("presentation is required")
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if err := test.open(); err == nil {
				t.Fatal("constructor did not return error")
			}
		})
	}
}

func TestAdapterParity_FactoryEventRendererDrainFailureSurfacesFinalizeError(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	service := visualizationcli.New(&fakeRootPeer{}, &drainFailingPresentation{})
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output:               &output,
		InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("OpenFactoryEventRenderer: %v", err)
	}
	renderer.PresentFactoryEvents(canonicalAdapterJavaScriptFactoryEvents()[:1])
	err = renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
		Status:        interfaces.InvocationTerminalStatusCompleted,
		PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "done"}},
	})
	if err == nil || !strings.Contains(err.Error(), "drain failed") {
		t.Fatalf("error = %v, want finalize/drain failure", err)
	}
}

func TestAdapterParity_PresentationSessionHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	service := newTestService()

	canceled := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}

	_, err := service.OpenPresentationSession(canceled(), factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryLossless,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("OpenPresentationSession error = %v, want context cancellation", err)
	}

	openResult, err := service.OpenPresentationSession(context.Background(), factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryLossless,
	})
	if err != nil {
		t.Fatalf("OpenPresentationSession: %v", err)
	}

	_, err = service.PresentPresentationProgress(canceled(), factoryvisualization.PresentProgressRequest{
		SessionID: openResult.SessionID,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PresentPresentationProgress error = %v, want context cancellation", err)
	}

	_, err = service.FinalizePresentationSession(canceled(), factoryvisualization.FinalizePresentationRequest{
		SessionID: openResult.SessionID,
		Terminal:  &factoryvisualization.TerminalWrite{Payload: []byte(`{"status":"completed"}`)},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("FinalizePresentationSession error = %v, want context cancellation", err)
	}
}

func openAdapterHumanFactoryEventRenderer(
	t *testing.T,
	output io.Writer,
) visualizationcli.FactoryEventRenderer {
	t.Helper()
	service := newTestService()
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output:               output,
		InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("open human Factory Event renderer: %v", err)
	}
	if renderer == nil {
		t.Fatal("renderer = nil, want human Factory Event renderer")
	}
	return renderer
}

func openAdapterJSONFactoryEventRenderer(
	t *testing.T,
	output io.Writer,
) visualizationcli.FactoryEventRenderer {
	t.Helper()
	service := newTestService()
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output:               output,
		JSON:                 true,
		InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("open JSON Factory Event renderer: %v", err)
	}
	if renderer == nil {
		t.Fatal("renderer = nil, want JSON Factory Event renderer")
	}
	return renderer
}

func assertAdapterFactoryEventNDJSONRecord(
	t *testing.T,
	line string,
	want interfaces.FactoryEvent,
	index int,
) {
	t.Helper()
	var record map[string]json.RawMessage
	if err := json.Unmarshal([]byte(line), &record); err != nil {
		t.Fatalf("decode event record %d: %v", index, err)
	}
	if len(record) != 2 || string(record["recordType"]) != `"factory_event"` || len(record["event"]) == 0 {
		t.Fatalf("event record %d has invalid discriminator shape: %s", index, line)
	}
	var event interfaces.FactoryEvent
	if err := json.Unmarshal(record["event"], &event); err != nil {
		t.Fatalf("decode event %d: %v", index, err)
	}
	if event.Context.Sequence != want.Context.Sequence || event.Context.SessionSequence == nil ||
		*event.Context.SessionSequence != *want.Context.SessionSequence {
		t.Fatalf("event %d sequence context changed: %#v", index, event.Context)
	}
}

func canonicalAdapterJavaScriptFactoryEvents() []interfaces.FactoryEvent {
	phaseName := "synthesize"
	events := []interfaces.FactoryEvent{
		canonicalAdapterFactoryEventWithPayload(1, interfaces.FactoryEventTypeSessionStarted, interfaces.FactorySessionStartedEventPayload{}),
		canonicalAdapterFactoryEventWithPayload(2, interfaces.FactoryEventTypeOrchestratorPhaseChanged, interfaces.OrchestratorPhaseChangedEventPayload{
			PhaseStatus: interfaces.OrchestratorPhaseStatusActive,
		}),
		canonicalAdapterFactoryEventWithPayload(3, interfaces.FactoryEventTypeOrchestratorCheckpointWritten, interfaces.OrchestratorCheckpointWrittenEventPayload{
			Label: "draft-ready", ResumabilityStatus: interfaces.CheckpointResumabilityStatusResumable,
		}),
	}
	events[1].Context.PhaseName = &phaseName
	return events
}

func canonicalAdapterFactoryEventFixture(sequence int, eventType interfaces.FactoryEventType) interfaces.FactoryEvent {
	sessionID := "session-js"
	sessionSequence := sequence
	return interfaces.FactoryEvent{
		Id: fmt.Sprintf("factory-event-%d", sequence), SchemaVersion: interfaces.FactoryEventSchemaVersionV1,
		Type: eventType, Payload: json.RawMessage(`{}`),
		Context: interfaces.FactoryEventContext{
			EventTime: time.Unix(int64(sequence), 0).UTC(), Sequence: sequence,
			SessionID: &sessionID, SessionSequence: &sessionSequence,
		},
	}
}

func canonicalAdapterFactoryEventWithPayload(
	sequence int,
	eventType interfaces.FactoryEventType,
	payload any,
) interfaces.FactoryEvent {
	event := canonicalAdapterFactoryEventFixture(sequence, eventType)
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	event.Payload = encoded
	return event
}

type drainFailingPresentation struct{}

func (drainFailingPresentation) OpenBestEffortOutput(writer io.Writer) factoryvisualization.Output {
	return factoryvisualization.NewResponsePresentation().OpenBestEffortOutput(writer)
}

func (drainFailingPresentation) OpenLosslessOutput(writer io.Writer) factoryvisualization.Output {
	return factoryvisualization.NewResponsePresentation().OpenLosslessOutput(writer)
}

func (drainFailingPresentation) OpenBestEffortFactoryEventStream(
	writer io.Writer,
	encode factoryvisualization.FactoryEventEncoder,
) interface {
	PresentFactoryEvents([]interfaces.FactoryEvent)
	Finalize(factoryvisualization.FinalResponseWriter) (bool, error)
	CloseAndDrain() error
} {
	return &drainFailingFactoryEventStream{
		inner: factoryvisualization.NewResponsePresentation().OpenBestEffortFactoryEventStream(writer, encode),
	}
}

func (drainFailingPresentation) OpenLosslessFactoryEventStream(
	writer io.Writer,
	encode factoryvisualization.FactoryEventEncoder,
) interface {
	PresentFactoryEvents([]interfaces.FactoryEvent)
	Finalize(factoryvisualization.FinalResponseWriter) (bool, error)
	CloseAndDrain() error
} {
	return &drainFailingFactoryEventStream{
		inner: factoryvisualization.NewResponsePresentation().OpenLosslessFactoryEventStream(writer, encode),
	}
}

type drainFailingFactoryEventStream struct {
	inner interface {
		PresentFactoryEvents([]interfaces.FactoryEvent)
		Finalize(factoryvisualization.FinalResponseWriter) (bool, error)
		CloseAndDrain() error
	}
}

func (stream *drainFailingFactoryEventStream) PresentFactoryEvents(events []interfaces.FactoryEvent) {
	stream.inner.PresentFactoryEvents(events)
}

func (stream *drainFailingFactoryEventStream) Finalize(
	write factoryvisualization.FinalResponseWriter,
) (bool, error) {
	_, err := stream.inner.Finalize(write)
	if err != nil {
		return false, err
	}
	return false, errors.New("drain failed")
}

func (stream *drainFailingFactoryEventStream) CloseAndDrain() error {
	return errors.New("drain failed")
}
