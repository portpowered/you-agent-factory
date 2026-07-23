package responseevents_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/responseevents"
)

func TestFactoryResponseEvent_MarshalEmitsDeclaredEnumStrings(t *testing.T) {
	t.Parallel()

	recordedAt := time.Date(2026, 7, 12, 23, 4, 5, 0, time.UTC)
	event := responseevents.FactoryResponseEvent{
		SchemaVersion:    responseevents.SchemaVersionV1,
		EventID:          "evt-001",
		Sequence:         7,
		RecordedAt:       recordedAt,
		FactorySessionID: "session-abc",
		RunID:            "run-xyz",
		Kind:             responseevents.KindMessage,
		Phase:            responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider:        "example-provider",
			NativeEventType: "response.output_text.delta",
			Delivery:        responseevents.DeliveryNativeStream,
			Representation:  responseevents.RepresentationDelta,
			Fidelity:        responseevents.FidelityLossless,
		},
		Payload:            json.RawMessage(`{"text":"hello"}`),
		DispatchID:         "dispatch-1",
		TurnID:             "turn-1",
		ItemID:             "item-1",
		ParentItemID:       "parent-item-0",
		ProviderSessionRef: "provider-session-ref-1",
	}

	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}

	encoded := string(payload)
	for _, want := range []string{
		`"schemaVersion":"agent-factory.response-event.v1"`,
		`"eventId":"evt-001"`,
		`"sequence":7`,
		`"factorySessionId":"session-abc"`,
		`"runId":"run-xyz"`,
		`"kind":"MESSAGE"`,
		`"phase":"DELTA"`,
		`"delivery":"NATIVE_STREAM"`,
		`"representation":"DELTA"`,
		`"fidelity":"LOSSLESS"`,
		`"dispatchId":"dispatch-1"`,
		`"turnId":"turn-1"`,
		`"itemId":"item-1"`,
		`"parentItemId":"parent-item-0"`,
		`"providerSessionRef":"provider-session-ref-1"`,
	} {
		if !strings.Contains(encoded, want) {
			t.Fatalf("marshaled envelope missing %q in %s", want, encoded)
		}
	}
}

func TestDeclaredKindsAndPhasesAreDistinctFromLegacyFragmentKinds(t *testing.T) {
	t.Parallel()

	legacyKinds := []string{
		"PROGRESS_FRAGMENT",
		"RESPONSE_FRAGMENT",
		"STREAM_COMPLETED",
		"STREAM_FAILED",
		"STREAM_COMPACTION_SIGNAL",
	}
	declaredKinds := []responseevents.Kind{
		responseevents.KindSession,
		responseevents.KindRun,
		responseevents.KindTurn,
		responseevents.KindMessage,
		responseevents.KindReasoning,
		responseevents.KindTool,
		responseevents.KindFileChange,
		responseevents.KindPlan,
		responseevents.KindProgress,
		responseevents.KindUsage,
		responseevents.KindError,
		responseevents.KindStreamGap,
	}
	for _, kind := range declaredKinds {
		for _, legacy := range legacyKinds {
			if string(kind) == legacy {
				t.Fatalf("declared kind %q must not alias legacy fragment kind %q", kind, legacy)
			}
		}
	}

	declaredPhases := []responseevents.Phase{
		responseevents.PhaseStarted,
		responseevents.PhaseDelta,
		responseevents.PhaseUpdated,
		responseevents.PhaseCompleted,
		responseevents.PhaseFailed,
		responseevents.PhaseCanceled,
	}
	seen := make(map[responseevents.Phase]struct{}, len(declaredPhases))
	for _, phase := range declaredPhases {
		if _, ok := seen[phase]; ok {
			t.Fatalf("duplicate declared phase %q", phase)
		}
		seen[phase] = struct{}{}
	}
}

func TestProvenanceEnumsMarshalToDeclaredStrings(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body any
		want string
	}{
		{
			name: "delivery",
			body: responseevents.DeliveryNativeFinal,
			want: `"NATIVE_FINAL"`,
		},
		{
			name: "representation snapshot",
			body: responseevents.RepresentationSnapshot,
			want: `"SNAPSHOT"`,
		},
		{
			name: "representation notification",
			body: responseevents.RepresentationNotification,
			want: `"NOTIFICATION"`,
		},
		{
			name: "fidelity normalized",
			body: responseevents.FidelityNormalized,
			want: `"NORMALIZED"`,
		},
		{
			name: "fidelity final only",
			body: responseevents.FidelityFinalOnly,
			want: `"FINAL_ONLY"`,
		},
		{
			name: "fidelity lifecycle only",
			body: responseevents.FidelityLifecycleOnly,
			want: `"LIFECYCLE_ONLY"`,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			payload, err := json.Marshal(tc.body)
			if err != nil {
				t.Fatalf("marshal %s: %v", tc.name, err)
			}
			if string(payload) != tc.want {
				t.Fatalf("marshal %s = %s, want %s", tc.name, payload, tc.want)
			}
		})
	}
}
