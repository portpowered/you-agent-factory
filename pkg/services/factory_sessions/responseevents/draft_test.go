package responseevents_test

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/responseevents"
)

func TestDraft_ValidatesWithoutPublicationMetadata(t *testing.T) {
	t.Parallel()

	draft := responseevents.Draft{
		RunID: "run-1",
		Kind:  responseevents.KindMessage,
		Phase: responseevents.PhaseDelta,
		Provenance: responseevents.Provenance{
			Provider:       "fake",
			Delivery:       responseevents.DeliveryNativeStream,
			Representation: responseevents.RepresentationDelta,
			Fidelity:       responseevents.FidelityLossless,
		},
		Payload: json.RawMessage(`{"contentBlockIndex":0,"contentBlockKind":"TEXT","textDelta":"hello"}`),
		ItemID:  "message-1",
	}

	if err := responseevents.ValidateDraft(draft); err != nil {
		t.Fatalf("ValidateDraft() error = %v", err)
	}

	encoded, err := json.Marshal(draft)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	for _, forbidden := range []string{"eventId", "sequence", "recordedAt", "factorySessionId"} {
		if _, ok := fields[forbidden]; ok {
			t.Fatalf("draft unexpectedly owns publication field %q", forbidden)
		}
	}
}

func TestValidateDraft_RejectsInvalidCanonicalKindPhaseCombination(t *testing.T) {
	t.Parallel()

	err := responseevents.ValidateDraft(responseevents.Draft{
		Kind:    responseevents.KindUsage,
		Phase:   responseevents.PhaseDelta,
		Payload: json.RawMessage(`{"totalTokens":1}`),
	})
	if err == nil {
		t.Fatal("ValidateDraft() error = nil, want invalid kind/phase error")
	}
}
