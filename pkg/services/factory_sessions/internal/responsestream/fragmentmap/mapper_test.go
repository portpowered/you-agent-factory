// backendsizecheck:ignore-file pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
// pkgmaintcheck:ignore-file-lines pre-existing baseline debt recorded 2026-08-08; refactor this code below the maintainability threshold and remove this exemption
package fragmentmap_test

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream/fragmentmap"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestMapFragment_ProgressFragmentEmitsProgressUpdated(t *testing.T) {
	t.Parallel()

	recordedAt := time.Date(2026, 7, 12, 18, 0, 0, 0, time.UTC)
	fragment := responsestream.Event{
		Sequence:   3,
		RecordedAt: recordedAt,
		Kind:       responsestream.EventKindProgressFragment,
		Type:       responsestream.EventTypeProgress,
		DispatchID: "dispatch-42",
		Payload:    "planning next step",
		ProviderSessionRef: &providers.SessionMetadata{
			Provider: string(modelprovider.ProviderCodex),
			Kind:     "session_id",
			ID:       "cursor-session-123",
		},
		ExternalEventType: "response.progress",
	}
	ctx := fragmentmap.Context{
		FactorySessionID: "session-abc",
		RunID:            "run-xyz",
	}

	events, err := fragmentmap.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("MapFragment() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}

	event := events[0]
	if event.Kind != responseevents.KindProgress || event.Phase != responseevents.PhaseUpdated {
		t.Fatalf("kind/phase = %q/%q, want PROGRESS/UPDATED", event.Kind, event.Phase)
	}
	if event.FactorySessionID != "session-abc" || event.RunID != "run-xyz" {
		t.Fatalf("session/run = %q/%q, want session-abc/run-xyz", event.FactorySessionID, event.RunID)
	}
	if event.Sequence != 3 || !event.RecordedAt.Equal(recordedAt) {
		t.Fatalf("sequence/recordedAt = %d/%v, want 3/%v", event.Sequence, event.RecordedAt, recordedAt)
	}
	if event.DispatchID != "dispatch-42" {
		t.Fatalf("dispatchId = %q, want dispatch-42", event.DispatchID)
	}
	if event.ProviderSessionRef != "cursor-session-123" {
		t.Fatalf("providerSessionRef = %q, want cursor-session-123", event.ProviderSessionRef)
	}

	var payload responseevents.ProgressPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("unmarshal progress payload: %v", err)
	}
	if payload.Label != "PROGRESS" || payload.Message != "planning next step" {
		t.Fatalf("progress payload = %#v, want label PROGRESS and planning message", payload)
	}

	if err := responseevents.ValidateEvent(event); err != nil {
		t.Fatalf("ValidateEvent() error = %v", err)
	}
}

func TestMapFragment_ProgressResponseTypesUseCanonicalMessagePhases(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		type_ responsestream.EventType
		phase responseevents.Phase
	}{
		{name: "text delta", type_: responsestream.EventTypeTextDelta, phase: responseevents.PhaseDelta},
		{name: "final text", type_: responsestream.EventTypeFinalText, phase: responseevents.PhaseCompleted},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			events, err := fragmentmap.MapFragment(fragmentmap.Context{
				FactorySessionID: "session-message",
				RunID:            "run-message",
			}, responsestream.Event{
				Kind:       responsestream.EventKindProgressFragment,
				Type:       tc.type_,
				Payload:    "message payload",
				DispatchID: "dispatch-message",
				Metadata:   map[string]string{"kind": "message"},
			})
			if err != nil {
				t.Fatalf("MapFragment() error = %v", err)
			}
			if len(events) != 1 || events[0].Kind != responseevents.KindMessage || events[0].Phase != tc.phase {
				t.Fatalf("mapped event = %#v, want MESSAGE/%s", events, tc.phase)
			}
			if err := responseevents.ValidateEvent(events[0]); err != nil {
				t.Fatalf("ValidateEvent() error = %v", err)
			}
		})
	}
}

func TestMapFragment_ProgressFragmentsUseLegalKindPhasePairs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		kind      string
		wantKind  responseevents.Kind
		wantPhase responseevents.Phase
	}{
		{name: "message content", kind: "message", wantKind: responseevents.KindMessage, wantPhase: responseevents.PhaseDelta},
		{name: "reasoning content", kind: "reasoning", wantKind: responseevents.KindReasoning, wantPhase: responseevents.PhaseDelta},
		{name: "tool content", kind: "tool", wantKind: responseevents.KindTool, wantPhase: responseevents.PhaseDelta},
		{name: "run lifecycle", kind: "run", wantKind: responseevents.KindProgress, wantPhase: responseevents.PhaseUpdated},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			events, err := fragmentmap.MapFragment(fragmentmap.Context{
				FactorySessionID: "session-progress",
				RunID:            "run-progress",
			}, responsestream.Event{
				Kind:     responsestream.EventKindProgressFragment,
				Type:     responsestream.EventTypeProgress,
				Payload:  "incremental content",
				Metadata: map[string]string{"kind": tc.kind, "item_id": "tool-call-1"},
			})
			if err != nil {
				t.Fatalf("MapFragment() error = %v", err)
			}
			if len(events) != 1 || events[0].Kind != tc.wantKind || events[0].Phase != tc.wantPhase {
				t.Fatalf("mapped event = %#v, want %s/%s", events, tc.wantKind, tc.wantPhase)
			}
			if err := responseevents.ValidateEvent(events[0]); err != nil {
				t.Fatalf("ValidateEvent() error = %v", err)
			}
		})
	}
}

func TestMapFragment_UsesExplicitProviderWithoutSessionReference(t *testing.T) {
	t.Parallel()

	events, err := fragmentmap.MapFragment(fragmentmap.Context{
		FactorySessionID: "factory-session-1",
		RunID:            "run-1",
	}, responsestream.Event{
		Kind:     responsestream.EventKindProgressFragment,
		Type:     responsestream.EventTypeProgress,
		Provider: "antigravity",
		Payload:  "final-only output",
	})
	if err != nil {
		t.Fatalf("MapFragment() error = %v", err)
	}
	if len(events) != 1 || events[0].Provenance.Provider != "antigravity" {
		t.Fatalf("mapped events = %#v, want one antigravity event", events)
	}
	if events[0].ProviderSessionRef != "" {
		t.Fatalf("mapped ProviderSessionRef = %q, want empty", events[0].ProviderSessionRef)
	}
}

func TestMapFragment_NativeToolProgressUsesObjectResultSummary(t *testing.T) {
	t.Parallel()

	fragment := responsestream.Event{
		Kind:              responsestream.EventKindProgressFragment,
		Type:              responsestream.EventTypeProgress,
		DispatchID:        "dispatch-tool",
		Payload:           "read completed",
		ExternalEventType: "tool.completed",
		Metadata: map[string]string{
			"correlation_id": "call-1",
			"tool_name":      "read",
		},
	}
	events, err := fragmentmap.MapFragment(fragmentmap.Context{
		FactorySessionID: "session-1",
		RunID:            "run-1",
	}, fragment)
	if err != nil {
		t.Fatalf("MapFragment() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}
	var payload struct {
		ResultSummary map[string]any `json:"resultSummary"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal tool payload: %v", err)
	}
	if got := payload.ResultSummary["detail"]; got != "read completed" {
		t.Fatalf("resultSummary.detail = %#v, want read completed", got)
	}
}

func TestMapFragment_NativeToolDeltaUsesDeltaPayload(t *testing.T) {
	t.Parallel()

	events, err := fragmentmap.MapFragment(fragmentmap.Context{
		FactorySessionID: "session-1",
		RunID:            "run-1",
	}, responsestream.Event{
		Kind:              responsestream.EventKindProgressFragment,
		Type:              responsestream.EventTypeProgress,
		DispatchID:        "dispatch-tool",
		Payload:           `{"city":"Oslo"}`,
		ExternalEventType: "tool.delta",
		Metadata: map[string]string{
			"correlation_id": "call-1",
			"tool_name":      "weather",
		},
	})
	if err != nil {
		t.Fatalf("MapFragment() error = %v", err)
	}
	if len(events) != 1 || events[0].Phase != responseevents.PhaseDelta ||
		events[0].Provenance.Representation != responseevents.RepresentationDelta {
		t.Fatalf("mapped events = %#v, want one TOOL/DELTA event", events)
	}
	var payload responseevents.ToolDeltaPayload
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal tool delta payload: %v", err)
	}
	if payload.ToolCallID != "call-1" || payload.OutputDelta != `{"city":"Oslo"}` {
		t.Fatalf("tool delta payload = %#v", payload)
	}
}

func TestMapFragment_ProgressProvenanceNeverClaimsLossless(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		fragment responsestream.Event
		want     responseevents.Fidelity
	}{
		{
			name: "normalized when payload is intact",
			fragment: responsestream.Event{
				Kind:    responsestream.EventKindProgressFragment,
				Type:    responsestream.EventTypeProgress,
				Payload: "still running",
			},
			want: responseevents.FidelityNormalized,
		},
		{
			name: "lossy when provider metadata marks truncation",
			fragment: responsestream.Event{
				Kind:    responsestream.EventKindProgressFragment,
				Type:    responsestream.EventTypeProgress,
				Payload: "truncated body",
				Metadata: map[string]string{
					"payload_truncated": "true",
				},
			},
			want: responseevents.FidelityLossy,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			events, err := fragmentmap.MapFragment(fragmentmap.Context{
				FactorySessionID: "session-1",
				RunID:            "run-1",
			}, tc.fragment)
			if err != nil {
				t.Fatalf("MapFragment() error = %v", err)
			}

			prov := events[0].Provenance
			if prov.Fidelity != tc.want {
				t.Fatalf("fidelity = %q, want %q", prov.Fidelity, tc.want)
			}
			if prov.Fidelity == responseevents.FidelityLossless {
				t.Fatal("fragment-sourced progress must not claim LOSSLESS fidelity")
			}
			if prov.Delivery != responseevents.DeliverySynthesized {
				t.Fatalf("delivery = %q, want SYNTHESIZED", prov.Delivery)
			}
			if prov.Representation != responseevents.RepresentationNotification {
				t.Fatalf("representation = %q, want NOTIFICATION", prov.Representation)
			}
		})
	}
}

func TestMapFragment_ProgressMappingIsDeterministic(t *testing.T) {
	t.Parallel()

	fragment := responsestream.Event{
		Sequence:   9,
		RecordedAt: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC),
		Kind:       responsestream.EventKindProgressFragment,
		Type:       responsestream.EventTypeStarted,
		DispatchID: "dispatch-deterministic",
		Payload:    "session warming up",
		Metadata: map[string]string{
			"runner_id": "codex",
		},
	}
	ctx := fragmentmap.Context{FactorySessionID: "session-det", RunID: "run-det"}

	first, err := fragmentmap.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("first MapFragment() error = %v", err)
	}
	second, err := fragmentmap.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("second MapFragment() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("mapping is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
	if first[0].Provenance.Provider != "codex" {
		t.Fatalf("provider = %q, want codex from metadata", first[0].Provenance.Provider)
	}
	if first[0].Provenance.NativeEventType != "STARTED" {
		t.Fatalf("nativeEventType = %q, want STARTED", first[0].Provenance.NativeEventType)
	}
}

func TestMapFragment_ProjectsReasoningAndSessionTitleProgress(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		fragment        responsestream.Event
		wantKind        responseevents.Kind
		wantPhase       responseevents.Phase
		wantPayloadText string
	}{
		{
			name: "reasoning delta",
			fragment: responsestream.Event{
				Kind: responsestream.EventKindProgressFragment, Type: responsestream.EventType("delta"),
				Payload: "considering the constraints", Metadata: map[string]string{"kind": "reasoning", "item_id": "thought-1"},
			},
			wantKind: responseevents.KindReasoning, wantPhase: responseevents.PhaseDelta, wantPayloadText: "considering the constraints",
		},
		{
			name: "session title update",
			fragment: responsestream.Event{
				Kind: responsestream.EventKindProgressFragment, Type: responsestream.EventType("updated"),
				Payload: "Planning the delivery", Metadata: map[string]string{"kind": "session", "title_present": "true"},
			},
			wantKind: responseevents.KindSession, wantPhase: responseevents.PhaseUpdated, wantPayloadText: "Planning the delivery",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			events, err := fragmentmap.MapFragment(fragmentmap.Context{FactorySessionID: "session-1", RunID: "run-1"}, tc.fragment)
			if err != nil {
				t.Fatalf("MapFragment() error = %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("event count = %d, want 1", len(events))
			}
			event := events[0]
			if event.Kind != tc.wantKind || event.Phase != tc.wantPhase {
				t.Fatalf("kind/phase = %q/%q, want %q/%q", event.Kind, event.Phase, tc.wantKind, tc.wantPhase)
			}

			if tc.wantKind == responseevents.KindReasoning {
				var payload responseevents.ReasoningPayload
				if err := json.Unmarshal(event.Payload, &payload); err != nil {
					t.Fatalf("unmarshal reasoning payload: %v", err)
				}
				if payload.SummaryDelta != tc.wantPayloadText {
					t.Fatalf("summary delta = %q, want %q", payload.SummaryDelta, tc.wantPayloadText)
				}
				return
			}

			var payload responseevents.SessionPayload
			if err := json.Unmarshal(event.Payload, &payload); err != nil {
				t.Fatalf("unmarshal session payload: %v", err)
			}
			if payload.Title == nil || *payload.Title != tc.wantPayloadText {
				t.Fatalf("session title = %v, want %q", payload.Title, tc.wantPayloadText)
			}
		})
	}
}

func TestMapFragment_ResponseFragmentEmitsMessageDelta(t *testing.T) {
	t.Parallel()

	recordedAt := time.Date(2026, 7, 12, 19, 0, 0, 0, time.UTC)
	fragment := responsestream.Event{
		Sequence:   5,
		RecordedAt: recordedAt,
		Kind:       responsestream.EventKindResponseFragment,
		Type:       responsestream.EventTypeTextDelta,
		DispatchID: "dispatch-42",
		Payload:    "hello ",
		ProviderSessionRef: &providers.SessionMetadata{
			Provider: string(modelprovider.ProviderCodex),
			Kind:     "session_id",
			ID:       "cursor-session-123",
		},
		ExternalEventType: "response.output_text.delta",
	}
	ctx := fragmentmap.Context{
		FactorySessionID: "session-abc",
		RunID:            "run-xyz",
	}

	events, err := fragmentmap.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("MapFragment() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}

	event := events[0]
	if event.Kind != responseevents.KindMessage || event.Phase != responseevents.PhaseDelta {
		t.Fatalf("kind/phase = %q/%q, want MESSAGE/DELTA", event.Kind, event.Phase)
	}
	assertMappedFragmentEnvelope(t, event, mappedFragmentEnvelopeExpectation{
		sessionID:          "session-abc",
		runID:              "run-xyz",
		sequence:           5,
		recordedAt:         recordedAt,
		dispatchID:         "dispatch-42",
		providerSessionRef: "cursor-session-123",
		wantNonEmptyItemID: true,
	})
	assertMessageDeltaPayload(t, event.Payload, "hello ")

	if err := responseevents.ValidateEvent(event); err != nil {
		t.Fatalf("ValidateEvent() error = %v", err)
	}
}

func TestMapFragment_ResponseFragmentStableItemIDAcrossDeltas(t *testing.T) {
	t.Parallel()

	ctx := fragmentmap.Context{FactorySessionID: "session-1", RunID: "run-1"}
	base := responsestream.Event{
		Kind:       responsestream.EventKindResponseFragment,
		Type:       responsestream.EventTypeTextDelta,
		DispatchID: "dispatch-shared",
	}

	first, err := fragmentmap.MapFragment(ctx, responsestream.Event{
		Sequence:   1,
		RecordedAt: time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC),
		Kind:       base.Kind,
		Type:       base.Type,
		DispatchID: base.DispatchID,
		Payload:    "alpha",
	})
	if err != nil {
		t.Fatalf("first MapFragment() error = %v", err)
	}
	second, err := fragmentmap.MapFragment(ctx, responsestream.Event{
		Sequence:   2,
		RecordedAt: time.Date(2026, 7, 12, 12, 0, 1, 0, time.UTC),
		Kind:       base.Kind,
		Type:       base.Type,
		DispatchID: base.DispatchID,
		Payload:    "beta",
	})
	if err != nil {
		t.Fatalf("second MapFragment() error = %v", err)
	}

	if first[0].ItemID != second[0].ItemID {
		t.Fatalf("itemId not stable across deltas: first=%q second=%q", first[0].ItemID, second[0].ItemID)
	}
	if first[0].EventID == second[0].EventID {
		t.Fatal("eventId must differ per fragment sequence even when itemId is stable")
	}
}

func TestMapFragment_ResponseFragmentDistinctItemIDsForDistinctDispatches(t *testing.T) {
	t.Parallel()

	ctx := fragmentmap.Context{FactorySessionID: "session-1", RunID: "run-1"}
	makeFragment := func(dispatchID, payload string) responsestream.Event {
		return responsestream.Event{
			Kind:       responsestream.EventKindResponseFragment,
			Type:       responsestream.EventTypeTextDelta,
			DispatchID: dispatchID,
			Payload:    payload,
		}
	}

	alpha, err := fragmentmap.MapFragment(ctx, makeFragment("dispatch-a", "alpha"))
	if err != nil {
		t.Fatalf("alpha MapFragment() error = %v", err)
	}
	beta, err := fragmentmap.MapFragment(ctx, makeFragment("dispatch-b", "beta"))
	if err != nil {
		t.Fatalf("beta MapFragment() error = %v", err)
	}

	if alpha[0].ItemID == beta[0].ItemID {
		t.Fatalf("distinct dispatches must have distinct itemIds: both %q", alpha[0].ItemID)
	}
}

func TestMapFragment_ResponseProvenanceNeverClaimsLossless(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		fragment responsestream.Event
		want     responseevents.Fidelity
	}{
		{
			name: "normalized when payload is intact",
			fragment: responsestream.Event{
				Kind:       responsestream.EventKindResponseFragment,
				Type:       responsestream.EventTypeTextDelta,
				DispatchID: "dispatch-1",
				Payload:    "stream chunk",
			},
			want: responseevents.FidelityNormalized,
		},
		{
			name: "lossy when provider metadata marks truncation",
			fragment: responsestream.Event{
				Kind:       responsestream.EventKindResponseFragment,
				Type:       responsestream.EventTypeTextDelta,
				DispatchID: "dispatch-1",
				Payload:    "truncated chunk",
				Metadata: map[string]string{
					"payload_truncated": "true",
				},
			},
			want: responseevents.FidelityLossy,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			events, err := fragmentmap.MapFragment(fragmentmap.Context{
				FactorySessionID: "session-1",
				RunID:            "run-1",
			}, tc.fragment)
			if err != nil {
				t.Fatalf("MapFragment() error = %v", err)
			}

			prov := events[0].Provenance
			if prov.Fidelity != tc.want {
				t.Fatalf("fidelity = %q, want %q", prov.Fidelity, tc.want)
			}
			if prov.Fidelity == responseevents.FidelityLossless {
				t.Fatal("fragment-sourced response must not claim LOSSLESS fidelity")
			}
			if prov.Delivery != responseevents.DeliverySynthesized {
				t.Fatalf("delivery = %q, want SYNTHESIZED", prov.Delivery)
			}
			if prov.Representation != responseevents.RepresentationDelta {
				t.Fatalf("representation = %q, want DELTA", prov.Representation)
			}
		})
	}
}

func TestMapFragment_ResponseMappingIsDeterministic(t *testing.T) {
	t.Parallel()

	fragment := responsestream.Event{
		Sequence:   11,
		RecordedAt: time.Date(2026, 7, 12, 14, 0, 0, 0, time.UTC),
		Kind:       responsestream.EventKindResponseFragment,
		Type:       responsestream.EventTypeTextDelta,
		DispatchID: "dispatch-deterministic",
		Payload:    "partial text",
		Metadata: map[string]string{
			"runner_id": "codex",
		},
	}
	ctx := fragmentmap.Context{FactorySessionID: "session-det", RunID: "run-det"}

	first, err := fragmentmap.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("first MapFragment() error = %v", err)
	}
	second, err := fragmentmap.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("second MapFragment() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("mapping is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestMapFragment_StreamCompletedEmitsRunCompleted(t *testing.T) {
	t.Parallel()

	recordedAt := time.Date(2026, 7, 12, 20, 0, 0, 0, time.UTC)
	fragment := responsestream.Event{
		Sequence:   7,
		RecordedAt: recordedAt,
		Kind:       responsestream.EventKindStreamCompleted,
		DispatchID: "dispatch-42",
		ProviderSessionRef: &providers.SessionMetadata{
			Provider: string(modelprovider.ProviderCodex),
			Kind:     "session_id",
			ID:       "cursor-session-123",
		},
		ExternalEventType: "response.completed",
	}
	ctx := fragmentmap.Context{
		FactorySessionID: "session-abc",
		RunID:            "run-xyz",
	}

	events, err := fragmentmap.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("MapFragment() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}

	event := events[0]
	if event.Kind != responseevents.KindRun || event.Phase != responseevents.PhaseCompleted {
		t.Fatalf("kind/phase = %q/%q, want RUN/COMPLETED", event.Kind, event.Phase)
	}
	if event.FactorySessionID != "session-abc" || event.RunID != "run-xyz" {
		t.Fatalf("session/run = %q/%q, want session-abc/run-xyz", event.FactorySessionID, event.RunID)
	}
	if event.Sequence != 7 || !event.RecordedAt.Equal(recordedAt) {
		t.Fatalf("sequence/recordedAt = %d/%v, want 7/%v", event.Sequence, event.RecordedAt, recordedAt)
	}
	if event.DispatchID != "dispatch-42" {
		t.Fatalf("dispatchId = %q, want dispatch-42", event.DispatchID)
	}
	if event.ProviderSessionRef != "cursor-session-123" {
		t.Fatalf("providerSessionRef = %q, want cursor-session-123", event.ProviderSessionRef)
	}
	if event.ItemID != "" {
		t.Fatal("terminal run completion must not synthesize itemId")
	}

	var payload responseevents.RunPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("unmarshal run payload: %v", err)
	}
	if payload.Status != "completed" {
		t.Fatalf("run status = %q, want completed", payload.Status)
	}

	if err := responseevents.ValidateEvent(event); err != nil {
		t.Fatalf("ValidateEvent() error = %v", err)
	}
}

func TestMapFragment_StreamFailedEmitsErrorFailed(t *testing.T) {
	t.Parallel()

	recordedAt := time.Date(2026, 7, 12, 20, 5, 0, 0, time.UTC)
	fragment := responsestream.Event{
		Sequence:   8,
		RecordedAt: recordedAt,
		Kind:       responsestream.EventKindStreamFailed,
		Type:       responsestream.EventTypeFailed,
		DispatchID: "dispatch-99",
		Payload:    "normalized provider failure",
		ProviderSessionRef: &providers.SessionMetadata{
			Provider: string(modelprovider.ProviderCodex),
			Kind:     "session_id",
			ID:       "cursor-session-456",
		},
		ExternalEventType: "response.failed",
		Metadata: map[string]string{
			"work_failure_type": string(workerexecution.WorkFailureTypeTimeout),
			"retryable":         "true",
		},
	}
	ctx := fragmentmap.Context{
		FactorySessionID: "session-abc",
		RunID:            "run-xyz",
	}

	events, err := fragmentmap.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("MapFragment() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}

	event := events[0]
	if event.Kind != responseevents.KindError || event.Phase != responseevents.PhaseFailed {
		t.Fatalf("kind/phase = %q/%q, want ERROR/FAILED", event.Kind, event.Phase)
	}
	if event.DispatchID != "dispatch-99" {
		t.Fatalf("dispatchId = %q, want dispatch-99", event.DispatchID)
	}
	if event.ItemID != "" {
		t.Fatal("terminal error must not synthesize itemId")
	}

	var payload responseevents.ErrorPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	if payload.Code != "timeout" || payload.Message != "normalized provider failure" || !payload.Retryable {
		t.Fatalf("error payload = %#v, want retryable timeout code and provider failure message", payload)
	}

	if err := responseevents.ValidateEvent(event); err != nil {
		t.Fatalf("ValidateEvent() error = %v", err)
	}
}

func TestMapFragment_StreamFailedUsesDefaultMessageWhenPayloadEmpty(t *testing.T) {
	t.Parallel()

	events, err := fragmentmap.MapFragment(fragmentmap.Context{
		FactorySessionID: "session-1",
		RunID:            "run-1",
	}, responsestream.Event{
		Kind:       responsestream.EventKindStreamFailed,
		DispatchID: "dispatch-empty",
	})
	if err != nil {
		t.Fatalf("MapFragment() error = %v", err)
	}

	var payload responseevents.ErrorPayload
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	if payload.Message != "dispatch stream failed" {
		t.Fatalf("message = %q, want dispatch stream failed", payload.Message)
	}
}

func TestMapFragment_StreamFailedCanceledUsesCanceledCode(t *testing.T) {
	t.Parallel()

	events, err := fragmentmap.MapFragment(fragmentmap.Context{
		FactorySessionID: "session-1",
		RunID:            "run-1",
	}, responsestream.Event{
		Kind:       responsestream.EventKindStreamFailed,
		Type:       responsestream.EventTypeCanceled,
		DispatchID: "dispatch-cancel",
		Payload:    "provider canceled",
	})
	if err != nil {
		t.Fatalf("MapFragment() error = %v", err)
	}

	var payload responseevents.ErrorPayload
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal error payload: %v", err)
	}
	if payload.Code != "stream_canceled" {
		t.Fatalf("code = %q, want stream_canceled", payload.Code)
	}
}

func TestMapFragment_TerminalProvenanceNeverClaimsLossless(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		fragment responsestream.Event
		want     responseevents.Fidelity
	}{
		{
			name: "completed normalized when intact",
			fragment: responsestream.Event{
				Kind: responsestream.EventKindStreamCompleted,
			},
			want: responseevents.FidelityNormalized,
		},
		{
			name: "failed lossy when payload truncated",
			fragment: responsestream.Event{
				Kind:    responsestream.EventKindStreamFailed,
				Payload: "truncated failure",
				Metadata: map[string]string{
					"payload_truncated": "true",
				},
			},
			want: responseevents.FidelityLossy,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			events, err := fragmentmap.MapFragment(fragmentmap.Context{
				FactorySessionID: "session-1",
				RunID:            "run-1",
			}, tc.fragment)
			if err != nil {
				t.Fatalf("MapFragment() error = %v", err)
			}

			prov := events[0].Provenance
			if prov.Fidelity != tc.want {
				t.Fatalf("fidelity = %q, want %q", prov.Fidelity, tc.want)
			}
			if prov.Fidelity == responseevents.FidelityLossless {
				t.Fatal("fragment-sourced terminal events must not claim LOSSLESS fidelity")
			}
			if prov.Delivery != responseevents.DeliverySynthesized {
				t.Fatalf("delivery = %q, want SYNTHESIZED", prov.Delivery)
			}
		})
	}
}

func TestMapFragment_TerminalMappingIsDeterministic(t *testing.T) {
	t.Parallel()

	fragment := responsestream.Event{
		Sequence:   12,
		RecordedAt: time.Date(2026, 7, 12, 15, 0, 0, 0, time.UTC),
		Kind:       responsestream.EventKindStreamCompleted,
		DispatchID: "dispatch-deterministic",
		Metadata: map[string]string{
			"runner_id": "codex",
		},
	}
	ctx := fragmentmap.Context{FactorySessionID: "session-det", RunID: "run-det"}

	first, err := fragmentmap.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("first MapFragment() error = %v", err)
	}
	second, err := fragmentmap.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("second MapFragment() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("mapping is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestMapFragment_CompactionSignalEmitsStreamGapUpdated(t *testing.T) {
	t.Parallel()

	recordedAt := time.Date(2026, 7, 12, 21, 0, 0, 0, time.UTC)
	fragment := responsestream.Event{
		Sequence:   10,
		RecordedAt: recordedAt,
		Kind:       responsestream.EventKindCompactionSignal,
		DispatchID: "dispatch-42",
		Compaction: &responsestream.CompactionSummary{
			Reason:                responsestream.CompactionReasonTruncated,
			DroppedSequenceCount:  2,
			FirstRetainedSequence: 3,
			LastDroppedSequence:   2,
		},
		ProviderSessionRef: &providers.SessionMetadata{
			Provider: string(modelprovider.ProviderCodex),
			Kind:     "session_id",
			ID:       "cursor-session-123",
		},
	}
	ctx := fragmentmap.Context{
		FactorySessionID: "session-abc",
		RunID:            "run-xyz",
	}

	events, err := fragmentmap.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("MapFragment() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("event count = %d, want 1", len(events))
	}

	event := events[0]
	if event.Kind != responseevents.KindStreamGap || event.Phase != responseevents.PhaseUpdated {
		t.Fatalf("kind/phase = %q/%q, want STREAM_GAP/UPDATED", event.Kind, event.Phase)
	}
	assertMappedFragmentEnvelope(t, event, mappedFragmentEnvelopeExpectation{
		sessionID:          "session-abc",
		runID:              "run-xyz",
		sequence:           10,
		recordedAt:         recordedAt,
		dispatchID:         "dispatch-42",
		providerSessionRef: "cursor-session-123",
		wantEmptyItemID:    true,
	})
	assertStreamGapPayload(t, event.Payload, 1, 2, 3, "truncated")

	if err := responseevents.ValidateEvent(event); err != nil {
		t.Fatalf("ValidateEvent() error = %v", err)
	}
}

func TestMapFragment_CompactionGapPayloadHandlesMissingBounds(t *testing.T) {
	t.Parallel()

	events, err := fragmentmap.MapFragment(fragmentmap.Context{
		FactorySessionID: "session-1",
		RunID:            "run-1",
	}, responsestream.Event{
		Kind: responsestream.EventKindCompactionSignal,
		Compaction: &responsestream.CompactionSummary{
			Reason:               responsestream.CompactionReasonCoalesced,
			DroppedSequenceCount: 3,
		},
	})
	if err != nil {
		t.Fatalf("MapFragment() error = %v", err)
	}

	var payload responseevents.StreamGapPayload
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal stream gap payload: %v", err)
	}
	if payload.FromSequence != 0 || payload.ToSequence != 0 {
		t.Fatalf("gap bounds = %d/%d, want 0/0 when sequence bounds absent", payload.FromSequence, payload.ToSequence)
	}
	if payload.FirstAvailableSequence != 1 {
		t.Fatalf("first available sequence = %d, want 1 when sequence bounds are absent", payload.FirstAvailableSequence)
	}
	if payload.Reason != "coalesced" {
		t.Fatalf("gap reason = %q, want coalesced", payload.Reason)
	}
}

func TestMapFragment_CompactionProvenanceAlwaysLossy(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		fragment responsestream.Event
	}{
		{
			name: "with compaction summary",
			fragment: responsestream.Event{
				Kind: responsestream.EventKindCompactionSignal,
				Compaction: &responsestream.CompactionSummary{
					Reason: responsestream.CompactionReasonAgeEvicted,
				},
			},
		},
		{
			name: "without compaction summary",
			fragment: responsestream.Event{
				Kind: responsestream.EventKindCompactionSignal,
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			events, err := fragmentmap.MapFragment(fragmentmap.Context{
				FactorySessionID: "session-1",
				RunID:            "run-1",
			}, tc.fragment)
			if err != nil {
				t.Fatalf("MapFragment() error = %v", err)
			}

			prov := events[0].Provenance
			if prov.Fidelity != responseevents.FidelityLossy {
				t.Fatalf("fidelity = %q, want LOSSY", prov.Fidelity)
			}
			if prov.Delivery != responseevents.DeliverySynthesized {
				t.Fatalf("delivery = %q, want SYNTHESIZED", prov.Delivery)
			}
			if prov.Representation != responseevents.RepresentationNotification {
				t.Fatalf("representation = %q, want NOTIFICATION", prov.Representation)
			}
		})
	}
}

func TestMapFragment_CompactionMappingIsDeterministic(t *testing.T) {
	t.Parallel()

	fragment := responsestream.Event{
		Sequence:   13,
		RecordedAt: time.Date(2026, 7, 12, 16, 0, 0, 0, time.UTC),
		Kind:       responsestream.EventKindCompactionSignal,
		DispatchID: "dispatch-deterministic",
		Compaction: &responsestream.CompactionSummary{
			Reason:                responsestream.CompactionReasonTruncated,
			DroppedSequenceCount:  1,
			FirstRetainedSequence: 4,
			LastDroppedSequence:   3,
		},
		Metadata: map[string]string{
			"runner_id": "codex",
		},
	}
	ctx := fragmentmap.Context{FactorySessionID: "session-det", RunID: "run-det"}

	first, err := fragmentmap.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("first MapFragment() error = %v", err)
	}
	second, err := fragmentmap.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("second MapFragment() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("mapping is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
}

// TestMapFragment_ACPPlanCarriesItsEntries covers the plan payload built from
// an ACP plan update.
//
// The ACP client already captures the entry list, and keeping only a summary
// string discarded it, so nothing downstream could render a real plan no
// matter what the provider reported. These cells pin that the entries survive,
// and that a provider reporting none still yields the prior summary-only
// behavior rather than an error.
func TestMapFragment_ACPPlanCarriesItsEntries(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		payload     string
		entries     string
		wantSummary string
		wantSteps   []responseevents.PlanStep
	}{
		{
			name:        "entries become ordered steps",
			payload:     "ACP plan",
			entries:     `[{"content":"first","status":"in_progress"},{"title":"second"}]`,
			wantSummary: "ACP plan",
			wantSteps: []responseevents.PlanStep{
				{ID: "1", Description: "first", Status: "in_progress"},
				{ID: "2", Description: "second"},
			},
		},
		{
			// A title is the fallback description, so an entry carrying only a
			// title is still renderable rather than dropped.
			name:        "an entry with neither content nor title is skipped",
			payload:     "",
			entries:     `[{"content":"  ","title":"  "},{"content":"kept"}]`,
			wantSummary: "ACP plan updated",
			wantSteps:   []responseevents.PlanStep{{ID: "2", Description: "kept"}},
		},
		{
			name:        "no entries leaves the summary alone",
			payload:     "just a summary",
			entries:     "",
			wantSummary: "just a summary",
		},
		{
			name:        "malformed entries leave the summary alone",
			payload:     "still a summary",
			entries:     "{not json",
			wantSummary: "still a summary",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			metadata := map[string]string{"kind": "plan", "item_id": "plan"}
			if tc.entries != "" {
				metadata["entries"] = tc.entries
			}
			events, err := fragmentmap.MapFragment(
				fragmentmap.Context{FactorySessionID: "session-1", RunID: "run-1"},
				responsestream.Event{
					Kind: responsestream.EventKindProgressFragment,
					Type: responsestream.EventTypeProgress,
					// A bare native type keeps this on the metadata-driven
					// path rather than the dotted native-adapter one.
					ExternalEventType: "updated",
					Payload:           tc.payload,
					DispatchID:        "dispatch-1",
					Metadata:          metadata,
				},
			)
			if err != nil {
				t.Fatalf("MapFragment() error = %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("mapped events = %d, want exactly 1", len(events))
			}
			if events[0].Kind != responseevents.KindPlan || events[0].Phase != responseevents.PhaseUpdated {
				t.Fatalf("mapped event = %q/%q, want PLAN/UPDATED", events[0].Kind, events[0].Phase)
			}
			var payload responseevents.PlanPayload
			if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
				t.Fatalf("decode PlanPayload: %v", err)
			}
			if payload.Summary != tc.wantSummary {
				t.Fatalf("summary = %q, want %q", payload.Summary, tc.wantSummary)
			}
			if !reflect.DeepEqual(payload.Steps, tc.wantSteps) {
				t.Fatalf("steps = %+v, want %+v", payload.Steps, tc.wantSteps)
			}
		})
	}
}
