package run

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func TestRunFactoryInvocation_LiveAndReplayPreserveCanonicalJavaScriptOrder(t *testing.T) {
	events := canonicalJavaScriptPhaseCheckpointPhaseEvents()

	outputs := make([]string, 0, 2)
	for _, source := range []struct {
		name       string
		replayPath string
	}{{name: "live"}, {name: "replay", replayPath: "recording.json"}} {
		t.Run(source.name, func(t *testing.T) {
			var output bytes.Buffer
			operation := testInvocationOperation{invokeFactory: func(
				_ context.Context,
				target factorysessions.InvocationTarget,
				_ factorysessions.InvocationRequest,
				consume factorysessions.FactoryEventConsumer,
			) (factorysessions.FactoryInvocationOutcome, error) {
				if target.ReplayPath != source.replayPath {
					t.Fatalf("ReplayPath = %q, want %q", target.ReplayPath, source.replayPath)
				}
				if source.name == "live" {
					consume(events[:2])
					consume(events[2:4])
					consume(events[4:])
				} else {
					consume(events)
				}
				return factorysessions.FactoryInvocationOutcome{Result: interfaces.FactoryInvocationResult{
					RequestID: "request-js", Status: interfaces.InvocationTerminalStatusCompleted,
					PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "complete"}},
				}}, nil
			}}
			cfg := RunConfig{
				InvocationOutputMode: InvocationOutputResponseStream,
				JSONOutput:           true, Output: &output, ReplayPath: source.replayPath,
			}
			if err := runFactoryInvocation(
				context.Background(), cfg, invocationTarget(cfg, nil, nil),
				factoryapi.InvocationRequest{}, operation, testResponsePresentation(),
			); err != nil {
				t.Fatalf("run Factory invocation: %v", err)
			}
			outputs = append(outputs, output.String())
			assertPhaseCheckpointPhasePresentation(t, output.String())
		})
	}
	if outputs[0] != outputs[1] {
		t.Fatalf("live and replay presentation differ:\nlive=%s\nreplay=%s", outputs[0], outputs[1])
	}
}

func assertPhaseCheckpointPhasePresentation(t *testing.T, output string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 7 {
		t.Fatalf("records = %d, want six Factory Events and one terminal result:\n%s", len(lines), output)
	}
	wantTypes := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeSessionStarted,
		interfaces.FactoryEventTypeOrchestratorPhaseChanged,
		interfaces.FactoryEventTypeOrchestratorCheckpointWritten,
		interfaces.FactoryEventTypeOrchestratorPhaseChanged,
		interfaces.FactoryEventTypeOrchestratorPhaseChanged,
		interfaces.FactoryEventTypeOrchestratorPhaseChanged,
	}
	previousSequence := 0
	previousSessionSequence := 0
	for index, wantType := range wantTypes {
		var record factoryEventJSONRecord
		if err := json.Unmarshal([]byte(lines[index]), &record); err != nil {
			t.Fatalf("decode Factory Event record %d: %v", index, err)
		}
		if record.RecordType != factoryEventJSONRecordType || record.Event.Type != wantType {
			t.Fatalf("record %d = %#v, want %s %s", index, record, factoryEventJSONRecordType, wantType)
		}
		if record.Event.Context.Sequence <= previousSequence || record.Event.Context.SessionSequence == nil ||
			*record.Event.Context.SessionSequence <= previousSessionSequence {
			t.Fatalf("record %d sequence context is not strictly increasing: %#v", index, record.Event.Context)
		}
		previousSequence = record.Event.Context.Sequence
		previousSessionSequence = *record.Event.Context.SessionSequence
	}
	var terminalPhase interfaces.OrchestratorPhaseChangedEventPayload
	var terminalRecord factoryEventJSONRecord
	if err := json.Unmarshal([]byte(lines[len(wantTypes)-1]), &terminalRecord); err != nil {
		t.Fatalf("decode terminal phase record: %v", err)
	}
	if err := json.Unmarshal(terminalRecord.Event.Payload, &terminalPhase); err != nil {
		t.Fatalf("decode terminal phase payload: %v", err)
	}
	if terminalPhase.PhaseStatus != interfaces.OrchestratorPhaseStatusCompleted {
		t.Fatalf("terminal phase status = %q, want COMPLETED", terminalPhase.PhaseStatus)
	}
	if !strings.Contains(lines[6], `"recordType":"invocation_result"`) {
		t.Fatalf("terminal invocation record = %q", lines[6])
	}
}

func TestRunFactoryInvocation_LiveEventIsWrittenBeforeOperationCompletes(t *testing.T) {
	var output lockedTestBuffer
	published := make(chan struct{})
	release := make(chan struct{})
	events := canonicalJavaScriptFactoryEvents()
	operation := testInvocationOperation{invokeFactory: func(
		_ context.Context,
		_ factorysessions.InvocationTarget,
		_ factorysessions.InvocationRequest,
		consume factorysessions.FactoryEventConsumer,
	) (factorysessions.FactoryInvocationOutcome, error) {
		consume(events[:1])
		close(published)
		<-release
		consume(events[1:])
		return factorysessions.FactoryInvocationOutcome{Result: interfaces.FactoryInvocationResult{
			RequestID: "request-live", Status: interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "complete"}},
		}}, nil
	}}
	cfg := RunConfig{
		InvocationOutputMode: InvocationOutputResponseStream,
		JSONOutput:           true, Output: &output,
	}
	done := make(chan error, 1)
	go func() {
		done <- runFactoryInvocation(
			context.Background(), cfg, invocationTarget(cfg, nil, nil),
			factoryapi.InvocationRequest{}, operation, testResponsePresentation(),
		)
	}()

	<-published
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(output.String(), `"recordType":"factory_event"`) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if got := output.String(); !strings.Contains(got, `"recordType":"factory_event"`) {
		t.Fatalf("live Factory Event was not written before operation completion: %q", got)
	}
	select {
	case err := <-done:
		t.Fatalf("invocation completed before release: %v", err)
	default:
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("run Factory invocation: %v", err)
	}
	assertCanonicalJavaScriptPresentation(t, output.String())
}

func TestHumanFactoryEventRenderer_WritesTerminalSuccessAndFailureLast(t *testing.T) {
	t.Parallel()

	t.Run("success after lifecycle", func(t *testing.T) {
		var output bytes.Buffer
		renderer := openTestHumanFactoryEventRenderer(t, &output, testResponsePresentation())
		renderer.PresentFactoryEvents(canonicalJavaScriptFactoryEvents()[:1])
		if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
			Status: interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText, Text: "complete",
			}},
		}); err != nil {
			t.Fatalf("write terminal success: %v", err)
		}
		if got := output.String(); !strings.HasSuffix(got, responseStreamPrimaryResultHeader+"\ncomplete") {
			t.Fatalf("success output does not end with primary result: %q", got)
		}
	})

	t.Run("failure includes public terminal context", func(t *testing.T) {
		var output bytes.Buffer
		renderer := openTestHumanFactoryEventRenderer(t, &output, testResponsePresentation())
		if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
			Status:    interfaces.InvocationTerminalStatusFailed,
			ErrorCode: "WORK_FAILED", Message: "worker stopped",
			SessionID: "session-1", WorkID: "work-1", WorkName: "research", WorkState: "FAILED",
		}); err != nil {
			t.Fatalf("write terminal failure: %v", err)
		}
		want := "--- invocation outcome ---\n" +
			"status: FAILED\nerror: WORK_FAILED\nmessage: worker stopped\n" +
			"session: session-1\nworkId: work-1\nworkName: research\nworkState: FAILED\n"
		if got := output.String(); got != want {
			t.Fatalf("failure output = %q, want %q", got, want)
		}
	})
}

func TestJSONFactoryEventRenderer_FinalizesTerminalRecordOnce(t *testing.T) {
	t.Parallel()
	var output bytes.Buffer
	renderer := openTestJSONFactoryEventRenderer(t, &output, testResponsePresentation())
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

func TestFactoryEventRenderers_RejectMissingPresentationEdges(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name string
		open func() error
	}{
		{
			name: "human output",
			open: func() error {
				_, err := invocationFactoryEventRenderer(RunConfig{
					InvocationOutputMode: InvocationOutputResponseStream,
					Output:               nil,
				}, testResponsePresentation())
				return err
			},
		},
		{
			name: "human presentation",
			open: func() error {
				_, err := invocationFactoryEventRenderer(RunConfig{
					InvocationOutputMode: InvocationOutputResponseStream,
					Output:               &bytes.Buffer{},
				}, nil)
				return err
			},
		},
		{
			name: "json output",
			open: func() error {
				_, err := invocationFactoryEventRenderer(RunConfig{
					InvocationOutputMode: InvocationOutputResponseStream,
					JSONOutput:           true,
					Output:               nil,
				}, testResponsePresentation())
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := test.open(); err == nil {
				t.Fatal("constructor did not return error")
			}
		})
	}
}

type lockedTestBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (buffer *lockedTestBuffer) Write(payload []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.Write(payload)
}

func (buffer *lockedTestBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}

func assertCanonicalJavaScriptPresentation(t *testing.T, output string) {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 4 {
		t.Fatalf("records = %d, want three Factory Events and one terminal result:\n%s", len(lines), output)
	}
	wantTypes := []interfaces.FactoryEventType{
		interfaces.FactoryEventTypeSessionStarted,
		interfaces.FactoryEventTypeOrchestratorPhaseChanged,
		interfaces.FactoryEventTypeOrchestratorCheckpointWritten,
	}
	for i, wantType := range wantTypes {
		var record factoryEventJSONRecord
		if err := json.Unmarshal([]byte(lines[i]), &record); err != nil {
			t.Fatalf("decode Factory Event record %d: %v", i, err)
		}
		if record.RecordType != factoryEventJSONRecordType || record.Event.Type != wantType {
			t.Fatalf("record %d = %#v, want %s %s", i, record, factoryEventJSONRecordType, wantType)
		}
		if record.Event.Context.SessionSequence == nil || *record.Event.Context.SessionSequence != i+1 {
			t.Fatalf("record %d sessionSequence = %#v, want %d", i, record.Event.Context.SessionSequence, i+1)
		}
	}
	if !strings.Contains(lines[3], `"recordType":"invocation_result"`) {
		t.Fatalf("terminal record = %q", lines[3])
	}
}

func canonicalJavaScriptFactoryEvents() []interfaces.FactoryEvent {
	phaseName := "synthesize"
	events := []interfaces.FactoryEvent{
		canonicalFactoryEventWithPayload(1, interfaces.FactoryEventTypeSessionStarted, interfaces.FactorySessionStartedEventPayload{}),
		canonicalFactoryEventWithPayload(2, interfaces.FactoryEventTypeOrchestratorPhaseChanged, interfaces.OrchestratorPhaseChangedEventPayload{
			PhaseStatus: interfaces.OrchestratorPhaseStatusActive,
		}),
		canonicalFactoryEventWithPayload(3, interfaces.FactoryEventTypeOrchestratorCheckpointWritten, interfaces.OrchestratorCheckpointWrittenEventPayload{
			Label: "draft-ready", ResumabilityStatus: interfaces.CheckpointResumabilityStatusResumable,
		}),
	}
	events[1].Context.PhaseName = &phaseName
	return events
}

func canonicalJavaScriptPhaseCheckpointPhaseEvents() []interfaces.FactoryEvent {
	plan := "plan"
	execute := "execute"
	events := []interfaces.FactoryEvent{
		canonicalFactoryEventWithPayload(1, interfaces.FactoryEventTypeSessionStarted, interfaces.FactorySessionStartedEventPayload{}),
		canonicalFactoryEventWithPayload(2, interfaces.FactoryEventTypeOrchestratorPhaseChanged, interfaces.OrchestratorPhaseChangedEventPayload{PhaseStatus: interfaces.OrchestratorPhaseStatusActive}),
		canonicalFactoryEventWithPayload(3, interfaces.FactoryEventTypeOrchestratorCheckpointWritten, interfaces.OrchestratorCheckpointWrittenEventPayload{Label: "plan-ready", ResumabilityStatus: interfaces.CheckpointResumabilityStatusResumable}),
		canonicalFactoryEventWithPayload(4, interfaces.FactoryEventTypeOrchestratorPhaseChanged, interfaces.OrchestratorPhaseChangedEventPayload{PhaseStatus: interfaces.OrchestratorPhaseStatusCompleted}),
		canonicalFactoryEventWithPayload(5, interfaces.FactoryEventTypeOrchestratorPhaseChanged, interfaces.OrchestratorPhaseChangedEventPayload{PhaseStatus: interfaces.OrchestratorPhaseStatusActive}),
		canonicalFactoryEventWithPayload(6, interfaces.FactoryEventTypeOrchestratorPhaseChanged, interfaces.OrchestratorPhaseChangedEventPayload{PhaseStatus: interfaces.OrchestratorPhaseStatusCompleted}),
	}
	events[1].Context.PhaseName = &plan
	events[2].Context.PhaseName = &plan
	events[3].Context.PhaseName = &plan
	events[4].Context.PhaseName = &execute
	events[5].Context.PhaseName = &execute
	return events
}

func canonicalFactoryEventFixture(sequence int, eventType interfaces.FactoryEventType) interfaces.FactoryEvent {
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

func canonicalFactoryEventWithPayload(sequence int, eventType interfaces.FactoryEventType, payload any) interfaces.FactoryEvent {
	event := canonicalFactoryEventFixture(sequence, eventType)
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	event.Payload = encoded
	return event
}

func TestHumanFactoryEventRenderer_FailuresAreUnderstandable(t *testing.T) {
	t.Parallel()

	events := []interfaces.FactoryEvent{
		canonicalFactoryEventWithPayload(1, interfaces.FactoryEventTypeInferenceResponse, workerexecution.InferenceResponseEventPayload{
			Attempt: 2, Outcome: workerexecution.InferenceOutcomeFailed,
			FailureDetail: &workerexecution.InferenceResponseFailureDetail{Message: "model request timed out"},
		}),
		canonicalFactoryEventWithPayload(2, interfaces.FactoryEventTypeDispatchResponse, workerexecution.DispatchResponseEventPayload{
			TransitionID: "release review", Outcome: workerexecution.OutcomeFailed,
			FailureDetail: &workerexecution.FailureDetail{Message: "worker timed out"},
		}),
	}
	var output strings.Builder
	renderer := openTestHumanFactoryEventRenderer(t, &output, testResponsePresentation())
	renderer.PresentFactoryEvents(events)
	renderer.StopProgressRendering()
	want := "[1] inference failed (attempt 2) — model request timed out\n" +
		"[2] workstation failed: release review — worker timed out\n"
	if got := output.String(); got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestInvocationFactoryEventRenderer_HumanModeDoesNotDependOnStdoutTTY(t *testing.T) {
	t.Parallel()

	outputs := make([]string, 0, 2)
	for _, outputIsTTY := range []bool{true, false} {
		var output strings.Builder
		renderer, err := invocationFactoryEventRenderer(RunConfig{
			InvocationOutputMode: InvocationOutputResponseStream,
			OutputIsTTY:          outputIsTTY,
			Output:               &output,
		}, testResponsePresentation())
		if err != nil {
			t.Fatalf("invocationFactoryEventRenderer(outputIsTTY=%t): %v", outputIsTTY, err)
		}
		renderer.PresentFactoryEvents(canonicalJavaScriptFactoryEvents())
		if err := renderer.WriteFinalInvocationResult(apisurface.FactoryInvocationResult{
			Status:        interfaces.InvocationTerminalStatusCompleted,
			PrimaryResult: []work.WorkContentPart{{Type: work.WorkContentPartTypeText, Text: "complete"}},
		}); err != nil {
			t.Fatalf("writeFinalInvocationResult(outputIsTTY=%t): %v", outputIsTTY, err)
		}
		outputs = append(outputs, output.String())
	}
	if outputs[0] != outputs[1] {
		t.Fatalf("TTY and redirected human output differ:\ntty=%q\nredirected=%q", outputs[0], outputs[1])
	}
}
