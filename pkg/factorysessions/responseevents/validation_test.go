package responseevents_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
)

func TestValidateEvent_AcceptsApprovedKindPhaseMatrix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		kind  responseevents.Kind
		phase responseevents.Phase
	}{
		{name: "session started", kind: responseevents.KindSession, phase: responseevents.PhaseStarted},
		{name: "run started", kind: responseevents.KindRun, phase: responseevents.PhaseStarted},
		{name: "turn started", kind: responseevents.KindTurn, phase: responseevents.PhaseStarted},
		{name: "message started", kind: responseevents.KindMessage, phase: responseevents.PhaseStarted},
		{name: "message delta", kind: responseevents.KindMessage, phase: responseevents.PhaseDelta},
		{name: "message completed", kind: responseevents.KindMessage, phase: responseevents.PhaseCompleted},
		{name: "reasoning delta", kind: responseevents.KindReasoning, phase: responseevents.PhaseDelta},
		{name: "tool started", kind: responseevents.KindTool, phase: responseevents.PhaseStarted},
		{name: "tool delta", kind: responseevents.KindTool, phase: responseevents.PhaseDelta},
		{name: "file change updated", kind: responseevents.KindFileChange, phase: responseevents.PhaseUpdated},
		{name: "plan updated", kind: responseevents.KindPlan, phase: responseevents.PhaseUpdated},
		{name: "progress updated", kind: responseevents.KindProgress, phase: responseevents.PhaseUpdated},
		{name: "usage updated", kind: responseevents.KindUsage, phase: responseevents.PhaseUpdated},
		{name: "stream gap updated", kind: responseevents.KindStreamGap, phase: responseevents.PhaseUpdated},
		{name: "error updated", kind: responseevents.KindError, phase: responseevents.PhaseUpdated},
		{name: "error failed", kind: responseevents.KindError, phase: responseevents.PhaseFailed},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			event := sampleEvent(t, tc.kind, tc.phase, samplePayload(t, tc.kind, tc.phase))
			if err := responseevents.ValidateEvent(event); err != nil {
				t.Fatalf("ValidateEvent() error = %v", err)
			}
		})
	}
}

func TestValidateEvent_RejectsInvalidKindPhasePairsWithActionableErrors(t *testing.T) {
	t.Parallel()

	event := sampleEvent(t, responseevents.KindMessage, responseevents.PhaseUpdated, json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"hi"}]}`))
	err := responseevents.ValidateEvent(event)
	assertValidationField(t, err, "phase")
	if !strings.Contains(err.Error(), "allowed phases: STARTED, DELTA, COMPLETED") {
		t.Fatalf("expected allowed phase list in error, got %v", err)
	}

	event = sampleEvent(t, responseevents.KindStreamGap, responseevents.PhaseDelta, json.RawMessage(`{"fromSequence":1,"toSequence":4}`))
	err = responseevents.ValidateEvent(event)
	assertValidationField(t, err, "phase")
	if !strings.Contains(err.Error(), "allowed phases: UPDATED") {
		t.Fatalf("expected STREAM_GAP phase constraint in error, got %v", err)
	}

	event = sampleEvent(t, responseevents.KindError, responseevents.PhaseCompleted, json.RawMessage(`{"code":"retry","message":"temporary"}`))
	err = responseevents.ValidateEvent(event)
	assertValidationField(t, err, "phase")
	if !strings.Contains(err.Error(), "allowed phases: UPDATED, FAILED") {
		t.Fatalf("expected ERROR phase constraint in error, got %v", err)
	}
}

func TestValidateEvent_RejectsKindPayloadMismatchesWithActionableErrors(t *testing.T) {
	t.Parallel()

	event := sampleEvent(
		t,
		responseevents.KindMessage,
		responseevents.PhaseDelta,
		json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"hi"}]}`),
	)
	err := responseevents.ValidateEvent(event)
	assertValidationField(t, err, "payload")
	if !strings.Contains(err.Error(), "MessageDeltaPayload") {
		t.Fatalf("expected MessageDeltaPayload constraint in error, got %v", err)
	}

	event = sampleEvent(
		t,
		responseevents.KindTool,
		responseevents.PhaseDelta,
		json.RawMessage(`{"toolCallId":"call-1","toolName":"read_file"}`),
	)
	err = responseevents.ValidateEvent(event)
	assertValidationField(t, err, "payload")
	if !strings.Contains(err.Error(), "ToolDeltaPayload") {
		t.Fatalf("expected ToolDeltaPayload constraint in error, got %v", err)
	}

	event = sampleEvent(
		t,
		responseevents.KindMessage,
		responseevents.PhaseStarted,
		json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"hi"}`),
	)
	err = responseevents.ValidateEvent(event)
	assertValidationField(t, err, "payload")
	if !strings.Contains(err.Error(), "MessagePayload") {
		t.Fatalf("expected MessagePayload constraint in error, got %v", err)
	}
}

func TestValidateEvent_RejectsInvalidPayloadBasics(t *testing.T) {
	t.Parallel()

	event := sampleEvent(t, responseevents.KindMessage, responseevents.PhaseStarted, nil)
	err := responseevents.ValidateEvent(event)
	assertValidationField(t, err, "payload")

	event = sampleEvent(t, responseevents.KindMessage, responseevents.PhaseStarted, json.RawMessage(`not-json`))
	err = responseevents.ValidateEvent(event)
	assertValidationField(t, err, "payload")
	if !strings.Contains(err.Error(), "valid JSON") {
		t.Fatalf("expected valid JSON constraint in error, got %v", err)
	}
}

func TestValidateEvent_RejectsRequiredPayloadFields(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		kind      responseevents.Kind
		phase     responseevents.Phase
		payload   json.RawMessage
		wantField string
	}{
		{
			name:      "file change missing path",
			kind:      responseevents.KindFileChange,
			phase:     responseevents.PhaseUpdated,
			payload:   json.RawMessage(`{"operation":"MODIFY"}`),
			wantField: "payload.path",
		},
		{
			name:      "progress missing label",
			kind:      responseevents.KindProgress,
			phase:     responseevents.PhaseUpdated,
			payload:   json.RawMessage(`{"message":"building"}`),
			wantField: "payload.label",
		},
		{
			name:      "error missing code",
			kind:      responseevents.KindError,
			phase:     responseevents.PhaseUpdated,
			payload:   json.RawMessage(`{"message":"temporary"}`),
			wantField: "payload.code",
		},
		{
			name:      "stream gap negative sequence",
			kind:      responseevents.KindStreamGap,
			phase:     responseevents.PhaseUpdated,
			payload:   json.RawMessage(`{"fromSequence":-1,"toSequence":4}`),
			wantField: "payload.fromSequence",
		},
		{
			name:      "message missing role",
			kind:      responseevents.KindMessage,
			phase:     responseevents.PhaseStarted,
			payload:   json.RawMessage(`{"contentBlocks":[{"kind":"TEXT","text":"hi"}]}`),
			wantField: "payload.role",
		},
		{
			name:      "message delta negative index",
			kind:      responseevents.KindMessage,
			phase:     responseevents.PhaseDelta,
			payload:   json.RawMessage(`{"contentBlockIndex":-1,"contentBlockKind":"TEXT","textDelta":"hi"}`),
			wantField: "payload.contentBlockIndex",
		},
		{
			name:      "tool missing tool call id",
			kind:      responseevents.KindTool,
			phase:     responseevents.PhaseStarted,
			payload:   json.RawMessage(`{"toolName":"read_file","status":"started"}`),
			wantField: "payload.toolCallId",
		},
		{
			name:      "tool delta missing output",
			kind:      responseevents.KindTool,
			phase:     responseevents.PhaseDelta,
			payload:   json.RawMessage(`{"toolCallId":"call-1"}`),
			wantField: "payload.outputDelta",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			event := sampleEvent(t, tc.kind, tc.phase, tc.payload)
			err := responseevents.ValidateEvent(event)
			assertValidationField(t, err, tc.wantField)
		})
	}
}

func TestValidateEvent_AcceptsDeclaredContentBlockKinds(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		payload json.RawMessage
	}{
		{
			name:    "reasoning summary",
			payload: json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"REASONING_SUMMARY","text":"thinking"}]}`),
		},
		{
			name:    "tool request",
			payload: json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"TOOL_REQUEST","toolCallId":"call-1","toolName":"read_file"}]}`),
		},
		{
			name:    "image ref",
			payload: json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"IMAGE_REF","imageRef":"img://1"}]}`),
		},
		{
			name:    "resource ref",
			payload: json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"RESOURCE_REF","resourceRef":"res://1"}]}`),
		},
		{
			name:    "structured output",
			payload: json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"STRUCTURED_OUTPUT","structuredOutput":{"status":"ok"}}]}`),
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			event := sampleEvent(t, responseevents.KindMessage, responseevents.PhaseCompleted, tc.payload)
			if err := responseevents.ValidateEvent(event); err != nil {
				t.Fatalf("ValidateEvent() error = %v", err)
			}
		})
	}
}

func TestValidateEvent_ValidatesDeclaredContentBlockKinds(t *testing.T) {
	t.Parallel()

	event := sampleEvent(
		t,
		responseevents.KindMessage,
		responseevents.PhaseCompleted,
		json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"UNKNOWN","text":"hi"}]}`),
	)
	err := responseevents.ValidateEvent(event)
	assertValidationField(t, err, "payload.contentBlocks[0].kind")
	if !strings.Contains(err.Error(), "TEXT, REASONING_SUMMARY, TOOL_REQUEST") {
		t.Fatalf("expected declared content block kinds in error, got %v", err)
	}
}

func TestCapabilities_MarshalDeclaredFlags(t *testing.T) {
	t.Parallel()

	capabilities := responseevents.Capabilities{
		NativeStreaming:    true,
		MessageDeltas:      true,
		MessageSnapshots:   true,
		ReasoningSummaries: true,
		ToolLifecycle:      true,
		ToolOutputDeltas:   true,
		FileChanges:        true,
		Plans:              true,
		Usage:              true,
		StableItemIDs:      true,
		ProviderReconnect:  true,
	}

	payload, err := json.Marshal(capabilities)
	if err != nil {
		t.Fatalf("marshal capabilities: %v", err)
	}
	encoded := string(payload)
	for _, want := range []string{
		`"nativeStreaming":true`,
		`"messageDeltas":true`,
		`"messageSnapshots":true`,
		`"reasoningSummaries":true`,
		`"toolLifecycle":true`,
		`"toolOutputDeltas":true`,
		`"fileChanges":true`,
		`"plans":true`,
		`"usage":true`,
		`"stableItemIds":true`,
		`"providerReconnect":true`,
	} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("capabilities JSON missing %q in %s", want, encoded)
		}
	}
}

func sampleEvent(t *testing.T, kind responseevents.Kind, phase responseevents.Phase, payload json.RawMessage) responseevents.FactoryResponseEvent {
	t.Helper()
	return responseevents.FactoryResponseEvent{
		SchemaVersion:    responseevents.SchemaVersionV1,
		EventID:          "evt-test",
		Sequence:         1,
		RecordedAt:       time.Date(2026, 7, 13, 0, 0, 0, 0, time.UTC),
		FactorySessionID: "session-test",
		RunID:            "run-test",
		Kind:             kind,
		Phase:            phase,
		Provenance: responseevents.Provenance{
			Provider:        "example-provider",
			NativeEventType: "example.event",
			Delivery:        responseevents.DeliveryNativeStream,
			Representation:  responseevents.RepresentationDelta,
			Fidelity:        responseevents.FidelityLossless,
		},
		Payload: payload,
	}
}

func samplePayload(t *testing.T, kind responseevents.Kind, phase responseevents.Phase) json.RawMessage {
	t.Helper()

	switch kind {
	case responseevents.KindMessage:
		return sampleMessagePayload(phase)
	case responseevents.KindTool:
		return sampleToolPayload(phase)
	default:
		return sampleStaticKindPayload(t, kind)
	}
}

func sampleMessagePayload(phase responseevents.Phase) json.RawMessage {
	if phase == responseevents.PhaseDelta {
		return json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"hello"}`)
	}
	return json.RawMessage(`{"role":"assistant","contentBlocks":[{"kind":"TEXT","text":"hello"}]}`)
}

func sampleToolPayload(phase responseevents.Phase) json.RawMessage {
	if phase == responseevents.PhaseDelta {
		return json.RawMessage(`{"toolCallId":"call-1","outputDelta":"partial"}`)
	}
	return json.RawMessage(`{"toolCallId":"call-1","toolName":"read_file","status":"started"}`)
}

func sampleStaticKindPayload(t *testing.T, kind responseevents.Kind) json.RawMessage {
	t.Helper()

	switch kind {
	case responseevents.KindSession:
		return json.RawMessage(`{"status":"active","capabilities":{"nativeStreaming":true}}`)
	case responseevents.KindRun:
		return json.RawMessage(`{"status":"running"}`)
	case responseevents.KindTurn:
		return json.RawMessage(`{"turnIndex":1,"status":"active"}`)
	case responseevents.KindReasoning:
		return json.RawMessage(`{"summaryDelta":"thinking"}`)
	case responseevents.KindFileChange:
		return json.RawMessage(`{"path":"README.md","operation":"MODIFY"}`)
	case responseevents.KindPlan:
		return json.RawMessage(`{"summary":"step plan","steps":[{"id":"1","description":"inspect"}]}`)
	case responseevents.KindProgress:
		return json.RawMessage(`{"label":"compile","message":"building"}`)
	case responseevents.KindUsage:
		return json.RawMessage(`{"inputTokens":10,"outputTokens":5,"totalTokens":15,"model":"example-model"}`)
	case responseevents.KindError:
		return json.RawMessage(`{"code":"temporary","message":"retry later","retryable":true,"retryAttempt":1}`)
	case responseevents.KindStreamGap:
		return json.RawMessage(`{"fromSequence":10,"toSequence":20,"reason":"retention"}`)
	default:
		t.Fatalf("unsupported sample kind %q", kind)
		return nil
	}
}

func assertValidationField(t *testing.T, err error, wantField string) {
	t.Helper()
	var validationErr *responseevents.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T: %v", err, err)
	}
	if validationErr.Field != wantField {
		t.Fatalf("ValidationError.Field = %q, want %q (message: %s)", validationErr.Field, wantField, validationErr.Error())
	}
}
