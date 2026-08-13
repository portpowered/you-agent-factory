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

func TestHumanFactoryEventRendererDefersTerminalSuccessClaimsUntilInvocationOutcome(t *testing.T) {
	t.Parallel()

	terminalEvents := []interfaces.FactoryEvent{
		{
			Type:    interfaces.FactoryEventTypeSessionResultUpdated,
			Context: interfaces.FactoryEventContext{Sequence: 1},
			Payload: mustJSON(t, interfaces.FactorySessionResultUpdatedEventPayload{
				ResultStatus: interfaces.FactorySessionResultStatusFinal,
			}),
		},
		{
			Type:    interfaces.FactoryEventTypeSessionCompleted,
			Context: interfaces.FactoryEventContext{Sequence: 2},
			Payload: mustJSON(t, interfaces.FactorySessionCompletedEventPayload{
				FinalStatus: interfaces.FactorySessionLifecycleStatusSucceeded,
			}),
		},
	}

	t.Run("canceled invocation discards success claims", func(t *testing.T) {
		var output bytes.Buffer
		renderer := openHumanRenderer(t, &output)
		renderer.PresentFactoryEvents(terminalEvents)
		if got := output.String(); got != "" {
			t.Fatalf("output before terminal outcome = %q, want deferred success claims", got)
		}
		if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
			Status:    interfaces.InvocationTerminalStatusCanceled,
			ErrorCode: string(interfaces.InvocationErrorCodeCanceled),
		}); err != nil {
			t.Fatalf("WriteFinalInvocationResult: %v", err)
		}
		got := output.String()
		if !strings.Contains(got, "status: CANCELED") {
			t.Fatalf("output = %q, want canceled invocation outcome", got)
		}
		for _, forbidden := range []string{"final output updated: FINAL", "factory completed: SUCCEEDED", "--- primary result ---"} {
			if strings.Contains(got, forbidden) {
				t.Fatalf("output = %q, contains canceled-run success claim %q", got, forbidden)
			}
		}
	})

	t.Run("completed invocation retains success claims", func(t *testing.T) {
		var output bytes.Buffer
		renderer := openHumanRenderer(t, &output)
		renderer.PresentFactoryEvents(terminalEvents)
		if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
			Status:        interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "done"}},
		}); err != nil {
			t.Fatalf("WriteFinalInvocationResult: %v", err)
		}
		got := output.String()
		for _, required := range []string{"final output updated: FINAL", "factory completed: SUCCEEDED", "--- primary result ---", "done"} {
			if !strings.Contains(got, required) {
				t.Fatalf("output = %q, missing successful-run output %q", got, required)
			}
		}
		if strings.Index(got, "factory completed: SUCCEEDED") > strings.Index(got, "--- primary result ---") {
			t.Fatalf("output = %q, completion claim should precede primary result", got)
		}
	})
}

func openHumanRenderer(t *testing.T, output io.Writer) visualizationcli.FactoryEventRenderer {
	t.Helper()
	renderer, err := newTestService().OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output:               output,
		InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("OpenFactoryEventRenderer: %v", err)
	}
	return renderer
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
		"[0] workstation started: execute (work-1, work-2) [dispatch dispatch-1]\n" +
		"[0] workstation completed: execute (work-1, work-2) [dispatch dispatch-1]\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestHumanFactoryEventRenderer_PresentsConcurrentWorkerLifecycle(t *testing.T) {
	t.Parallel()

	dispatchOne := "dispatch-one"
	dispatchTwo := "dispatch-two"
	dispatchThree := "dispatch-three"
	workOne := []string{"work-one"}
	workTwo := []string{"work-two"}
	workThree := []string{"work-three"}
	queuedLabel := "build"
	queuedOne := mustJSON(t, interfaces.DispatchQueuedEventPayload{
		Label: &queuedLabel, InputWorkIDs: &workOne,
	})
	requestOne := mustJSON(t, interfaces.DispatchRequestEventPayload{
		TransitionID: "build",
		Inputs:       []interfaces.DispatchConsumedWorkRef{{WorkID: workOne[0]}},
	})
	requestTwo := mustJSON(t, interfaces.DispatchRequestEventPayload{
		TransitionID: "test",
		Inputs:       []interfaces.DispatchConsumedWorkRef{{WorkID: workTwo[0]}},
	})
	requestThree := mustJSON(t, interfaces.DispatchRequestEventPayload{
		TransitionID: "deploy",
		Inputs:       []interfaces.DispatchConsumedWorkRef{{WorkID: workThree[0]}},
	})
	workerOne := mustJSON(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-one"})
	workerTwo := mustJSON(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-two"})
	workerThree := mustJSON(t, interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-three"})
	failedDetail := &workerexecution.FailureDetail{Message: "tests failed"}
	failedResponse := mustJSON(t, workerexecution.DispatchResponseEventPayload{
		TransitionID: "test", Outcome: workerexecution.OutcomeFailed, FailureDetail: failedDetail,
	})
	acceptedResponse := mustJSON(t, workerexecution.DispatchResponseEventPayload{
		TransitionID: "build", Outcome: workerexecution.OutcomeAccepted,
	})
	interrupted := mustJSON(t, interfaces.DispatchInterruptedEventPayload{Reason: "operator cancelled"})

	service := visualizationcli.New(nil, factoryvisualizationwire.NewResponsePresentation())
	var output, progress bytes.Buffer
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output: &output, ProgressOutput: &progress, ProgressIsTTY: false,
		InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("open renderer: %v", err)
	}
	renderer.PresentFactoryEvents([]interfaces.FactoryEvent{
		{Type: interfaces.FactoryEventTypeDispatchQueued, Payload: queuedOne, Context: interfaces.FactoryEventContext{DispatchID: &dispatchOne}},
		{Type: interfaces.FactoryEventTypeDispatchRequest, Payload: requestOne, Context: interfaces.FactoryEventContext{DispatchID: &dispatchOne}},
		{Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc, Payload: workerOne, Context: interfaces.FactoryEventContext{DispatchID: &dispatchOne}},
		{Type: interfaces.FactoryEventTypeDispatchRequest, Payload: requestTwo, Context: interfaces.FactoryEventContext{DispatchID: &dispatchTwo}},
		{Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc, Payload: workerTwo, Context: interfaces.FactoryEventContext{DispatchID: &dispatchTwo}},
		{Type: interfaces.FactoryEventTypeDispatchResponse, Payload: failedResponse, Context: interfaces.FactoryEventContext{DispatchID: &dispatchTwo, WorkIDs: &workTwo}},
		{Type: interfaces.FactoryEventTypeDispatchRequest, Payload: requestThree, Context: interfaces.FactoryEventContext{DispatchID: &dispatchThree}},
		{Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc, Payload: workerThree, Context: interfaces.FactoryEventContext{DispatchID: &dispatchThree}},
		{Type: interfaces.FactoryEventTypeDispatchInterrupted, Payload: interrupted, Context: interfaces.FactoryEventContext{DispatchID: &dispatchThree}},
		{Type: interfaces.FactoryEventTypeDispatchResponse, Payload: acceptedResponse, Context: interfaces.FactoryEventContext{DispatchID: &dispatchOne, WorkIDs: &workOne}},
	})
	renderer.StopProgressRendering()

	want := "[0] workstation queued: build (work-one) [dispatch dispatch-one]\n" +
		"[0] workstation started: build (work-one) [dispatch dispatch-one]\n" +
		"[0] workstation started: test (work-two) [dispatch dispatch-two]\n" +
		"[0] workstation failed: test (work-two) [dispatch dispatch-two] — tests failed\n" +
		"[0] workstation started: deploy (work-three) [dispatch dispatch-three]\n" +
		"[0] workstation interrupted: deploy (work-three) [dispatch dispatch-three] — operator cancelled\n" +
		"[0] workstation completed: build (work-one) [dispatch dispatch-one]\n"
	if got := output.String(); got != want {
		t.Fatalf("concurrent worker output = %q, want %q", got, want)
	}
	for _, worker := range []string{"worker worker-one: active at build", "worker worker-two: active at test", "worker worker-three: active at deploy"} {
		if !strings.Contains(progress.String(), worker) {
			t.Fatalf("progress = %q, want %q", progress.String(), worker)
		}
	}
	if strings.ContainsAny(progress.String(), "\x1b\r") {
		t.Fatalf("non-TTY progress = %q, want no ANSI or cursor controls", progress.String())
	}
}

func TestFormatHumanWorkAccepted_PresentsPartialBatchWithoutFabricatedEdges(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(work.WorkRequestEventPayload{
		Works: []work.WorkRequestEventWork{
			{WorkID: "work-1", Name: "first"},
			{WorkID: "work-2", Name: "second", Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeJSON,
				JSON: json.RawMessage(`{"private":"provider payload"}`),
			}}},
			{Name: "independent"},
			{},
		},
		Relations: []work.WorkRequestEventRelation{
			{Type: work.WorkRelationDependsOn, SourceWorkName: "second", TargetWorkName: "first"},
			{Type: work.WorkRelationDependsOn, TargetWorkName: "independent"},
			{SourceWorkName: "independent", TargetWorkName: "first"},
		},
	})
	if err != nil {
		t.Fatalf("marshal partial batch payload: %v", err)
	}

	got := renderHumanWorkAcceptedEvent(t, payload)
	want := "[0] work accepted: 4 items\n" +
		"- work-1 (first)\n" +
		"- work-2 (second)\n" +
		"- independent\n" +
		"- (unnamed work)\n" +
		"- second depends on -> first"
	if got != want {
		t.Fatalf("partial batch output = %q, want %q", got, want)
	}
	if strings.Contains(got, "private") || strings.Contains(got, "independent depends") {
		t.Fatalf("partial batch output = %q, contains private payload or fabricated edge", got)
	}
}

func TestFormatHumanWorkAccepted_PresentsSingleItemRelationPayload(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(work.WorkRequestEventPayload{
		Works: []work.WorkRequestEventWork{{WorkID: "work-1", Name: "only"}},
		Relations: []work.WorkRequestEventRelation{{
			Type: work.WorkRelationDependsOn, SourceWorkName: "only", TargetWorkID: "work-0",
		}},
	})
	if err != nil {
		t.Fatalf("marshal single-item payload: %v", err)
	}

	got := renderHumanWorkAcceptedEvent(t, payload)
	want := "[0] work accepted: 1 item\n- work-1 (only)\n- only depends on -> work-0"
	if got != want {
		t.Fatalf("single-item output = %q, want %q", got, want)
	}
}

func renderHumanWorkAcceptedEvent(t *testing.T, payload []byte) string {
	t.Helper()

	service := visualizationcli.New(nil, factoryvisualizationwire.NewResponsePresentation())
	var output bytes.Buffer
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output: &output, InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("open renderer: %v", err)
	}
	renderer.PresentFactoryEvents([]interfaces.FactoryEvent{{
		Type:    interfaces.FactoryEventTypeWorkRequest,
		Payload: payload,
	}})
	renderer.StopProgressRendering()
	return strings.TrimSpace(output.String())
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
	workIDs := []string{"work-a"}
	queuedLabel := "execute"
	queuedPayload, err := json.Marshal(interfaces.DispatchQueuedEventPayload{
		Label: &queuedLabel, InputWorkIDs: &workIDs,
	})
	if err != nil {
		t.Fatalf("marshal queued payload: %v", err)
	}
	requestPayload, err := json.Marshal(interfaces.DispatchRequestEventPayload{
		TransitionID: "execute",
	})
	if err != nil {
		t.Fatalf("marshal request payload: %v", err)
	}
	workerPayload, err := json.Marshal(interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: "worker-session-a"})
	if err != nil {
		t.Fatalf("marshal worker association payload: %v", err)
	}
	renderer.PresentFactoryEvents([]interfaces.FactoryEvent{{
		Type: interfaces.FactoryEventTypeDispatchQueued, Payload: queuedPayload,
		Context: interfaces.FactoryEventContext{DispatchID: &dispatchID},
	}, {
		Type: interfaces.FactoryEventTypeDispatchRequest, Payload: requestPayload,
		Context: interfaces.FactoryEventContext{DispatchID: &dispatchID},
	}, {
		Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc, Payload: workerPayload,
		Context: interfaces.FactoryEventContext{DispatchID: &dispatchID},
	}})
	ticks <- time.Unix(1, 0)
	renderer.PresentFactoryEvents([]interfaces.FactoryEvent{{
		Type:    interfaces.FactoryEventTypeDispatchResponse,
		Context: interfaces.FactoryEventContext{DispatchID: &dispatchID},
	}})
	renderer.StopProgressRendering()
	got := progress.String()
	for _, want := range []string{"\x1b[", "⠋", "worker-session-a", "execute", "work-a", "[dispatch worker-a]"} {
		if !strings.Contains(got, want) {
			t.Fatalf("progress = %q, want %q", got, want)
		}
	}
	if !strings.HasSuffix(got, "\r\x1b[2K") {
		t.Fatalf("progress = %q, want terminal clear", got)
	}
}

func TestHumanFactoryEventRenderer_TTYProgressRemovesOnlyTerminalWorker(t *testing.T) {
	t.Parallel()

	service := visualizationcli.New(nil, factoryvisualizationwire.NewResponsePresentation())
	var output, progress bytes.Buffer
	ticks := make(chan time.Time)
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output: &output, ProgressOutput: &progress, ProgressIsTTY: true, ProgressTicks: ticks,
		InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("open renderer: %v", err)
	}
	request := func(dispatchID, workstation, workID string) interfaces.FactoryEvent {
		payload, marshalErr := json.Marshal(interfaces.DispatchRequestEventPayload{
			TransitionID: workstation, Inputs: []interfaces.DispatchConsumedWorkRef{{WorkID: workID}},
		})
		if marshalErr != nil {
			t.Fatalf("marshal request %s: %v", dispatchID, marshalErr)
		}
		return interfaces.FactoryEvent{Type: interfaces.FactoryEventTypeDispatchRequest, Payload: payload,
			Context: interfaces.FactoryEventContext{DispatchID: &dispatchID}}
	}
	associate := func(dispatchID, workerID string) interfaces.FactoryEvent {
		payload, marshalErr := json.Marshal(interfaces.DispatchWorkerSessionAssociationEventPayload{WorkerSessionID: workerID})
		if marshalErr != nil {
			t.Fatalf("marshal association %s: %v", dispatchID, marshalErr)
		}
		return interfaces.FactoryEvent{Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc, Payload: payload,
			Context: interfaces.FactoryEventContext{DispatchID: &dispatchID}}
	}
	firstDispatch, secondDispatch := "dispatch-first", "dispatch-second"
	renderer.PresentFactoryEvents([]interfaces.FactoryEvent{request(firstDispatch, "compile", "work-first"), associate(firstDispatch, "worker-first")})
	renderer.PresentFactoryEvents([]interfaces.FactoryEvent{request(secondDispatch, "verify", "work-second"), associate(secondDispatch, "worker-second")})
	combined := progress.String()
	for _, want := range []string{"worker-first", "compile", "work-first", "worker-second", "verify", "work-second"} {
		if !strings.Contains(combined, want) {
			t.Fatalf("combined progress = %q, want %q", combined, want)
		}
	}
	workFirst := []string{"work-first"}
	renderer.PresentFactoryEvents([]interfaces.FactoryEvent{{
		Type:    interfaces.FactoryEventTypeDispatchResponse,
		Context: interfaces.FactoryEventContext{DispatchID: &firstDispatch, WorkIDs: &workFirst},
	}})
	lastFrame := progress.String()[strings.LastIndex(progress.String(), "\r\x1b[2K"):]
	if !strings.Contains(lastFrame, "worker-second") || strings.Contains(lastFrame, "worker-first") {
		t.Fatalf("last progress frame = %q, want only second worker", lastFrame)
	}
	renderer.StopProgressRendering()
}

func TestHumanFactoryEventRenderer_TTYProgressUsesStableDistinctWorkerColors(t *testing.T) {
	t.Parallel()

	events := func() []interfaces.FactoryEvent {
		request := func(dispatchID, workstation, workID string) interfaces.FactoryEvent {
			payload, err := json.Marshal(interfaces.DispatchRequestEventPayload{
				TransitionID: workstation,
				Inputs:       []interfaces.DispatchConsumedWorkRef{{WorkID: workID}},
			})
			if err != nil {
				t.Fatalf("marshal request %s: %v", dispatchID, err)
			}
			return interfaces.FactoryEvent{
				Type: interfaces.FactoryEventTypeDispatchRequest, Payload: payload,
				Context: interfaces.FactoryEventContext{DispatchID: &dispatchID},
			}
		}
		associate := func(dispatchID, workerID string) interfaces.FactoryEvent {
			payload, err := json.Marshal(interfaces.DispatchWorkerSessionAssociationEventPayload{
				WorkerSessionID: workerID,
			})
			if err != nil {
				t.Fatalf("marshal association %s: %v", dispatchID, err)
			}
			return interfaces.FactoryEvent{
				Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc, Payload: payload,
				Context: interfaces.FactoryEventContext{DispatchID: &dispatchID},
			}
		}
		return []interfaces.FactoryEvent{
			request("dispatch-a", "compile", "work-a"),
			associate("dispatch-a", "worker-a"),
			request("dispatch-b", "verify", "work-b"),
			associate("dispatch-b", "worker-b"),
		}
	}

	render := func() string {
		service := visualizationcli.New(nil, factoryvisualizationwire.NewResponsePresentation())
		var output, progress bytes.Buffer
		renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
			Output: &output, ProgressOutput: &progress, ProgressIsTTY: true,
			InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
		})
		if err != nil {
			t.Fatalf("open renderer: %v", err)
		}
		renderer.PresentFactoryEvents(events())
		renderer.StopProgressRendering()
		return progress.String()
	}

	first, second := render(), render()
	if first != second {
		t.Fatalf("progress colors are not deterministic:\nfirst=%q\nsecond=%q", first, second)
	}
	workerAColor := progressColorForWorker(first, "worker-a")
	workerBColor := progressColorForWorker(first, "worker-b")
	if workerAColor == "" || workerBColor == "" {
		t.Fatalf("progress = %q, want color escapes for both workers", first)
	}
	if workerAColor == workerBColor {
		t.Fatalf("worker colors = %q and %q, want distinct colors", workerAColor, workerBColor)
	}
}

func TestHumanFactoryEventRenderer_TTYProgressMigratesSplitBatchColorsAndCleansUp(t *testing.T) {
	t.Parallel()

	const workerCapacity = 12 // humanWorkerProgressColors contains the supported palette.
	service := visualizationcli.New(nil, factoryvisualizationwire.NewResponsePresentation())
	var output, progress bytes.Buffer
	renderer, err := service.OpenFactoryEventRenderer(visualizationcli.FactoryEventRendererConfig{
		Output: &output, ProgressOutput: &progress, ProgressIsTTY: true,
		ProgressTicks:        make(chan time.Time),
		InvocationOutputMode: visualizationcli.InvocationOutputResponseStream,
	})
	if err != nil {
		t.Fatalf("open renderer: %v", err)
	}

	request := func(index int) interfaces.FactoryEvent {
		dispatchID := fmt.Sprintf("dispatch-%02d", index)
		workID := fmt.Sprintf("work-%02d", index)
		workstation := fmt.Sprintf("station-%02d", index)
		payload, marshalErr := json.Marshal(interfaces.DispatchRequestEventPayload{
			TransitionID: workstation,
			Inputs:       []interfaces.DispatchConsumedWorkRef{{WorkID: workID}},
		})
		if marshalErr != nil {
			t.Fatalf("marshal request %s: %v", dispatchID, marshalErr)
		}
		return interfaces.FactoryEvent{Type: interfaces.FactoryEventTypeDispatchRequest, Payload: payload,
			Context: interfaces.FactoryEventContext{DispatchID: &dispatchID}}
	}
	associate := func(index int) interfaces.FactoryEvent {
		dispatchID := fmt.Sprintf("dispatch-%02d", index)
		workerID := fmt.Sprintf("worker-%02d", index)
		payload, marshalErr := json.Marshal(interfaces.DispatchWorkerSessionAssociationEventPayload{
			WorkerSessionID: workerID,
		})
		if marshalErr != nil {
			t.Fatalf("marshal association %s: %v", dispatchID, marshalErr)
		}
		return interfaces.FactoryEvent{Type: interfaces.FactoryEventTypeDispatchWorkerSessionAssoc, Payload: payload,
			Context: interfaces.FactoryEventContext{DispatchID: &dispatchID}}
	}
	presentSplitBatch := func(index int) {
		renderer.PresentFactoryEvents([]interfaces.FactoryEvent{request(index)})
		renderer.PresentFactoryEvents([]interfaces.FactoryEvent{associate(index)})
	}

	for index := 0; index < workerCapacity; index++ {
		presentSplitBatch(index)
	}
	frame := latestTTYProgressFrame(progress.String())
	colors := make(map[string]string, workerCapacity)
	for index := 0; index < workerCapacity; index++ {
		workerID := fmt.Sprintf("worker-%02d", index)
		color := progressColorForWorker(frame, workerID)
		if color == "" {
			t.Fatalf("progress frame = %q, missing color for %s", frame, workerID)
		}
		if previous := colors[color]; previous != "" {
			t.Fatalf("progress frame = %q, workers %s and %s share color %s", frame, previous, workerID, color)
		}
		colors[color] = workerID
	}

	for index := 0; index < workerCapacity/2; index++ {
		dispatchID := fmt.Sprintf("dispatch-%02d", index)
		renderer.PresentFactoryEvents([]interfaces.FactoryEvent{{
			Type:    interfaces.FactoryEventTypeDispatchResponse,
			Context: interfaces.FactoryEventContext{DispatchID: &dispatchID},
		}})
	}
	for index := workerCapacity; index < workerCapacity+workerCapacity/2; index++ {
		presentSplitBatch(index)
	}
	frame = latestTTYProgressFrame(progress.String())
	colors = make(map[string]string, workerCapacity)
	for index := 0; index < workerCapacity/2; index++ {
		workerID := fmt.Sprintf("worker-%02d", index)
		if strings.Contains(frame, "worker "+workerID) {
			t.Fatalf("progress frame = %q, terminal worker %s remains active", frame, workerID)
		}
	}
	for index := workerCapacity / 2; index < workerCapacity+workerCapacity/2; index++ {
		workerID := fmt.Sprintf("worker-%02d", index)
		color := progressColorForWorker(frame, workerID)
		if color == "" {
			t.Fatalf("progress frame = %q, missing color for active %s", frame, workerID)
		}
		if previous := colors[color]; previous != "" {
			t.Fatalf("progress frame = %q, workers %s and %s share color %s after cleanup", frame, previous, workerID, color)
		}
		colors[color] = workerID
	}
	renderer.StopProgressRendering()
}

func progressColorForWorker(progress, workerID string) string {
	workerIndex := strings.Index(progress, "worker "+workerID)
	if workerIndex < 0 {
		return ""
	}
	prefix := progress[:workerIndex]
	start := strings.LastIndex(prefix, "\x1b[")
	if start < 0 {
		return ""
	}
	end := strings.Index(prefix[start:], "m")
	if end < 0 {
		return ""
	}
	return prefix[start+2 : start+end]
}

func latestTTYProgressFrame(progress string) string {
	const prefix = "\r\x1b[2K"
	start := strings.LastIndex(progress, prefix)
	if start < 0 {
		return progress
	}
	return progress[start+len(prefix):]
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

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal test payload: %v", err)
	}
	return payload
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
