package fragmentmap_test

import (
	"encoding/json"
	"reflect"
	"testing"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responsestream/fragmentmap"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

var internalFragmentFixtureMatrix = []struct {
	name             string
	fragment         responsestream.Event
	wantKind         responseevents.Kind
	wantPhase        responseevents.Phase
	wantFidelity     responseevents.Fidelity
	wantItemIDSynth  bool
	wantProviderGram string // verbatim payload substring proving no grammar parsing
}{
	{
		name: "progress fragment",
		fragment: responsestream.Event{
			Sequence:   1,
			RecordedAt: time.Date(2026, 7, 12, 18, 0, 0, 0, time.UTC),
			Kind:       responsestream.EventKindProgressFragment,
			Type:       responsestream.EventTypeProgress,
			DispatchID: "dispatch-matrix",
			Payload:    "planning next step",
		},
		wantKind:     responseevents.KindProgress,
		wantPhase:    responseevents.PhaseUpdated,
		wantFidelity: responseevents.FidelityNormalized,
	},
	{
		name: "response fragment",
		fragment: responsestream.Event{
			Sequence:   2,
			RecordedAt: time.Date(2026, 7, 12, 18, 1, 0, 0, time.UTC),
			Kind:       responsestream.EventKindResponseFragment,
			Type:       responsestream.EventTypeTextDelta,
			DispatchID: "dispatch-matrix",
			Payload:    `{"type":"response.output_text.delta","delta":"hello"}`,
		},
		wantKind:         responseevents.KindMessage,
		wantPhase:        responseevents.PhaseDelta,
		wantFidelity:     responseevents.FidelityNormalized,
		wantItemIDSynth:  true,
		wantProviderGram: `{"type":"response.output_text.delta","delta":"hello"}`,
	},
	{
		name: "stream completed terminal",
		fragment: responsestream.Event{
			Sequence:   3,
			RecordedAt: time.Date(2026, 7, 12, 18, 2, 0, 0, time.UTC),
			Kind:       responsestream.EventKindStreamCompleted,
			DispatchID: "dispatch-matrix",
		},
		wantKind:     responseevents.KindRun,
		wantPhase:    responseevents.PhaseCompleted,
		wantFidelity: responseevents.FidelityNormalized,
	},
	{
		name: "stream failed terminal",
		fragment: responsestream.Event{
			Sequence:   4,
			RecordedAt: time.Date(2026, 7, 12, 18, 3, 0, 0, time.UTC),
			Kind:       responsestream.EventKindStreamFailed,
			Type:       responsestream.EventTypeFailed,
			DispatchID: "dispatch-matrix",
			Payload:    "provider stream failed",
		},
		wantKind:     responseevents.KindError,
		wantPhase:    responseevents.PhaseFailed,
		wantFidelity: responseevents.FidelityNormalized,
	},
	{
		name: "compaction signal",
		fragment: responsestream.Event{
			Sequence:   5,
			RecordedAt: time.Date(2026, 7, 12, 18, 4, 0, 0, time.UTC),
			Kind:       responsestream.EventKindCompactionSignal,
			DispatchID: "dispatch-matrix",
			Compaction: &responsestream.CompactionSummary{
				Reason:                responsestream.CompactionReasonTruncated,
				DroppedSequenceCount:  2,
				FirstRetainedSequence: 6,
				LastDroppedSequence:   5,
			},
		},
		wantKind:     responseevents.KindStreamGap,
		wantPhase:    responseevents.PhaseUpdated,
		wantFidelity: responseevents.FidelityLossy,
	},
}

func assertFixtureMatrixMappedEvent(
	t *testing.T,
	event responseevents.FactoryResponseEvent,
	tc struct {
		name             string
		fragment         responsestream.Event
		wantKind         responseevents.Kind
		wantPhase        responseevents.Phase
		wantFidelity     responseevents.Fidelity
		wantItemIDSynth  bool
		wantProviderGram string
	},
) {
	t.Helper()

	if event.Kind != tc.wantKind || event.Phase != tc.wantPhase {
		t.Fatalf("kind/phase = %q/%q, want %q/%q", event.Kind, event.Phase, tc.wantKind, tc.wantPhase)
	}
	if event.Provenance.Fidelity != tc.wantFidelity {
		t.Fatalf("fidelity = %q, want %q", event.Provenance.Fidelity, tc.wantFidelity)
	}
	if event.Provenance.Fidelity == responseevents.FidelityLossless {
		t.Fatal("fragment-sourced events must not claim LOSSLESS fidelity")
	}
	if event.Provenance.Delivery != responseevents.DeliverySynthesized {
		t.Fatalf("delivery = %q, want SYNTHESIZED", event.Provenance.Delivery)
	}

	if tc.wantItemIDSynth {
		if event.ItemID == "" {
			t.Fatal("expected synthesized itemId")
		}
	} else if event.ItemID != "" {
		t.Fatalf("itemId = %q, want empty for non-message fragments", event.ItemID)
	}

	if tc.wantProviderGram != "" {
		var payload responseevents.MessageDeltaPayload
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			t.Fatalf("unmarshal message delta payload: %v", err)
		}
		if payload.TextDelta != tc.wantProviderGram {
			t.Fatalf("textDelta = %q, want verbatim fragment payload %q", payload.TextDelta, tc.wantProviderGram)
		}
	}

	if err := responseevents.ValidateEvent(event); err != nil {
		t.Fatalf("ValidateEvent() error = %v", err)
	}
}

func TestMapFragment_FixtureMatrixMapsEveryLegacyFragmentKind(t *testing.T) {
	t.Parallel()

	ctx := fragmentmap.Context{
		FactorySessionID: "session-matrix",
		RunID:            "run-matrix",
	}

	for _, tc := range internalFragmentFixtureMatrix {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			events, err := fragmentmap.MapFragment(ctx, tc.fragment)
			if err != nil {
				t.Fatalf("MapFragment() error = %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("event count = %d, want 1", len(events))
			}

			assertFixtureMatrixMappedEvent(t, events[0], tc)
		})
	}
}

func TestMapFragment_FixtureMatrixMappingIsDeterministic(t *testing.T) {
	t.Parallel()

	ctx := fragmentmap.Context{
		FactorySessionID: "session-matrix",
		RunID:            "run-matrix",
	}

	for _, tc := range internalFragmentFixtureMatrix {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			first, err := fragmentmap.MapFragment(ctx, tc.fragment)
			if err != nil {
				t.Fatalf("first MapFragment() error = %v", err)
			}
			second, err := fragmentmap.MapFragment(ctx, tc.fragment)
			if err != nil {
				t.Fatalf("second MapFragment() error = %v", err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("mapping is not deterministic:\nfirst=%#v\nsecond=%#v", first, second)
			}
		})
	}
}

func TestMapFragment_ResponseFragmentItemIDStableAcrossMatrixDeltas(t *testing.T) {
	t.Parallel()

	ctx := fragmentmap.Context{
		FactorySessionID: "session-matrix",
		RunID:            "run-matrix",
	}
	base := responsestream.Event{
		Kind:       responsestream.EventKindResponseFragment,
		Type:       responsestream.EventTypeTextDelta,
		DispatchID: "dispatch-matrix",
		ProviderSessionRef: &workerexecution.ProviderSessionMetadata{
			Provider: string(modelprovider.ProviderCursor),
			Kind:     "session_id",
			ID:       "cursor-session-matrix",
		},
	}

	first, err := fragmentmap.MapFragment(ctx, responsestream.Event{
		Sequence:           10,
		RecordedAt:         time.Date(2026, 7, 12, 19, 0, 0, 0, time.UTC),
		Kind:               base.Kind,
		Type:               base.Type,
		DispatchID:         base.DispatchID,
		ProviderSessionRef: base.ProviderSessionRef,
		Payload:            "alpha",
	})
	if err != nil {
		t.Fatalf("first MapFragment() error = %v", err)
	}
	second, err := fragmentmap.MapFragment(ctx, responsestream.Event{
		Sequence:           11,
		RecordedAt:         time.Date(2026, 7, 12, 19, 0, 1, 0, time.UTC),
		Kind:               base.Kind,
		Type:               base.Type,
		DispatchID:         base.DispatchID,
		ProviderSessionRef: base.ProviderSessionRef,
		Payload:            "beta",
	})
	if err != nil {
		t.Fatalf("second MapFragment() error = %v", err)
	}

	if first[0].ItemID != second[0].ItemID {
		t.Fatalf("itemId not stable across related deltas: first=%q second=%q", first[0].ItemID, second[0].ItemID)
	}
	if first[0].EventID == second[0].EventID {
		t.Fatal("eventId must differ per fragment sequence")
	}
}

func TestMapFragment_CoversEveryDeclaredLegacyFragmentKind(t *testing.T) {
	t.Parallel()

	supported := map[responsestream.EventKind]struct{}{
		responsestream.EventKindProgressFragment: {},
		responsestream.EventKindResponseFragment: {},
		responsestream.EventKindStreamCompleted:  {},
		responsestream.EventKindStreamFailed:     {},
		responsestream.EventKindCompactionSignal: {},
	}
	covered := make(map[responsestream.EventKind]struct{}, len(internalFragmentFixtureMatrix))
	for _, tc := range internalFragmentFixtureMatrix {
		covered[tc.fragment.Kind] = struct{}{}
	}

	for kind := range supported {
		if _, ok := covered[kind]; !ok {
			t.Fatalf("fixture matrix missing legacy fragment kind %q", kind)
		}
	}
	if len(covered) != len(supported) {
		t.Fatalf("fixture matrix kind count = %d, want %d declared kinds", len(covered), len(supported))
	}
}

func TestLegacyPublisher_RemainsCallableForAllFragmentKinds(t *testing.T) {
	t.Parallel()

	stream := responsestream.NewSessionResponseStream(platformclock.Real{})
	publisher := responsestream.NewPublisher(stream, nil)

	fragments := []responsestream.Event{
		{Kind: responsestream.EventKindProgressFragment, Payload: "progress"},
		{Kind: responsestream.EventKindResponseFragment, Payload: "response"},
		{Kind: responsestream.EventKindStreamCompleted},
		{Kind: responsestream.EventKindStreamFailed, Payload: "failed"},
		{Kind: responsestream.EventKindCompactionSignal, Compaction: &responsestream.CompactionSummary{
			Reason: responsestream.CompactionReasonTruncated,
		}},
	}

	for i, fragment := range fragments {
		stored := publisher.Publish(fragment)
		if stored.Sequence != int64(i+1) {
			t.Fatalf("fragment %d sequence = %d, want %d", i, stored.Sequence, i+1)
		}
	}

	if diagnostics := publisher.Diagnostics(); diagnostics.PublishedCount != int64(len(fragments)) {
		t.Fatalf("published count = %d, want %d", diagnostics.PublishedCount, len(fragments))
	}
}
