package compat_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream"
	"github.com/portpowered/infinite-you/pkg/factorysessions/responsestream/compat"
	"github.com/portpowered/infinite-you/pkg/interfaces"
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
		ProviderSessionRef: &interfaces.ProviderSessionMetadata{
			Provider: string(interfaces.ModelProviderCursor),
			Kind:     "session_id",
			ID:       "cursor-session-123",
		},
		ExternalEventType: "response.progress",
	}
	ctx := compat.Context{
		FactorySessionID: "session-abc",
		RunID:            "run-xyz",
	}

	events, err := compat.MapFragment(ctx, fragment)
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

			events, err := compat.MapFragment(compat.Context{
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
	ctx := compat.Context{FactorySessionID: "session-det", RunID: "run-det"}

	first, err := compat.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("first MapFragment() error = %v", err)
	}
	second, err := compat.MapFragment(ctx, fragment)
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
		ProviderSessionRef: &interfaces.ProviderSessionMetadata{
			Provider: string(interfaces.ModelProviderCursor),
			Kind:     "session_id",
			ID:       "cursor-session-123",
		},
		ExternalEventType: "response.output_text.delta",
	}
	ctx := compat.Context{
		FactorySessionID: "session-abc",
		RunID:            "run-xyz",
	}

	events, err := compat.MapFragment(ctx, fragment)
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
	if event.FactorySessionID != "session-abc" || event.RunID != "run-xyz" {
		t.Fatalf("session/run = %q/%q, want session-abc/run-xyz", event.FactorySessionID, event.RunID)
	}
	if event.Sequence != 5 || !event.RecordedAt.Equal(recordedAt) {
		t.Fatalf("sequence/recordedAt = %d/%v, want 5/%v", event.Sequence, event.RecordedAt, recordedAt)
	}
	if event.DispatchID != "dispatch-42" {
		t.Fatalf("dispatchId = %q, want dispatch-42", event.DispatchID)
	}
	if event.ProviderSessionRef != "cursor-session-123" {
		t.Fatalf("providerSessionRef = %q, want cursor-session-123", event.ProviderSessionRef)
	}
	if event.ItemID == "" {
		t.Fatal("itemId must be synthesized for response fragments")
	}

	var payload responseevents.MessageDeltaPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("unmarshal message delta payload: %v", err)
	}
	if payload.ContentBlockIndex != 0 || payload.ContentBlockKind != responseevents.ContentBlockText {
		t.Fatalf("delta block = index %d kind %q, want 0/TEXT", payload.ContentBlockIndex, payload.ContentBlockKind)
	}
	if payload.TextDelta != "hello " {
		t.Fatalf("textDelta = %q, want hello ", payload.TextDelta)
	}

	if err := responseevents.ValidateEvent(event); err != nil {
		t.Fatalf("ValidateEvent() error = %v", err)
	}
}

func TestMapFragment_ResponseFragmentStableItemIDAcrossDeltas(t *testing.T) {
	t.Parallel()

	ctx := compat.Context{FactorySessionID: "session-1", RunID: "run-1"}
	base := responsestream.Event{
		Kind:       responsestream.EventKindResponseFragment,
		Type:       responsestream.EventTypeTextDelta,
		DispatchID: "dispatch-shared",
	}

	first, err := compat.MapFragment(ctx, responsestream.Event{
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
	second, err := compat.MapFragment(ctx, responsestream.Event{
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

	ctx := compat.Context{FactorySessionID: "session-1", RunID: "run-1"}
	makeFragment := func(dispatchID, payload string) responsestream.Event {
		return responsestream.Event{
			Kind:       responsestream.EventKindResponseFragment,
			Type:       responsestream.EventTypeTextDelta,
			DispatchID: dispatchID,
			Payload:    payload,
		}
	}

	alpha, err := compat.MapFragment(ctx, makeFragment("dispatch-a", "alpha"))
	if err != nil {
		t.Fatalf("alpha MapFragment() error = %v", err)
	}
	beta, err := compat.MapFragment(ctx, makeFragment("dispatch-b", "beta"))
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

			events, err := compat.MapFragment(compat.Context{
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
	ctx := compat.Context{FactorySessionID: "session-det", RunID: "run-det"}

	first, err := compat.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("first MapFragment() error = %v", err)
	}
	second, err := compat.MapFragment(ctx, fragment)
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
		ProviderSessionRef: &interfaces.ProviderSessionMetadata{
			Provider: string(interfaces.ModelProviderCursor),
			Kind:     "session_id",
			ID:       "cursor-session-123",
		},
		ExternalEventType: "response.completed",
	}
	ctx := compat.Context{
		FactorySessionID: "session-abc",
		RunID:            "run-xyz",
	}

	events, err := compat.MapFragment(ctx, fragment)
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
		ProviderSessionRef: &interfaces.ProviderSessionMetadata{
			Provider: string(interfaces.ModelProviderCursor),
			Kind:     "session_id",
			ID:       "cursor-session-456",
		},
		ExternalEventType: "response.failed",
	}
	ctx := compat.Context{
		FactorySessionID: "session-abc",
		RunID:            "run-xyz",
	}

	events, err := compat.MapFragment(ctx, fragment)
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
	if payload.Code != "stream_failed" || payload.Message != "normalized provider failure" {
		t.Fatalf("error payload = %#v, want stream_failed code and provider failure message", payload)
	}

	if err := responseevents.ValidateEvent(event); err != nil {
		t.Fatalf("ValidateEvent() error = %v", err)
	}
}

func TestMapFragment_StreamFailedUsesDefaultMessageWhenPayloadEmpty(t *testing.T) {
	t.Parallel()

	events, err := compat.MapFragment(compat.Context{
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

	events, err := compat.MapFragment(compat.Context{
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

			events, err := compat.MapFragment(compat.Context{
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
	ctx := compat.Context{FactorySessionID: "session-det", RunID: "run-det"}

	first, err := compat.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("first MapFragment() error = %v", err)
	}
	second, err := compat.MapFragment(ctx, fragment)
	if err != nil {
		t.Fatalf("second MapFragment() error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("mapping is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}
}

func TestMapFragment_UnsupportedKindsReturnTypedError(t *testing.T) {
	t.Parallel()

	unsupported := []responsestream.EventKind{
		responsestream.EventKindCompactionSignal,
	}
	ctx := compat.Context{FactorySessionID: "session-1", RunID: "run-1"}

	for _, kind := range unsupported {
		kind := kind
		t.Run(string(kind), func(t *testing.T) {
			t.Parallel()

			_, err := compat.MapFragment(ctx, responsestream.Event{Kind: kind})
			if !errors.Is(err, compat.ErrUnsupportedFragmentKind) {
				t.Fatalf("MapFragment() error = %v, want ErrUnsupportedFragmentKind", err)
			}
		})
	}
}
