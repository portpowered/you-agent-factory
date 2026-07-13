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

func TestMapFragment_UnsupportedKindsReturnTypedError(t *testing.T) {
	t.Parallel()

	unsupported := []responsestream.EventKind{
		responsestream.EventKindResponseFragment,
		responsestream.EventKindStreamCompleted,
		responsestream.EventKindStreamFailed,
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
