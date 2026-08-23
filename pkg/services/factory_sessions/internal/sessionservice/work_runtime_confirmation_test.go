package service

import (
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestAnnotateRuntimeWorkStateSequencesUsesLatestCanonicalStateFact(t *testing.T) {
	t.Parallel()

	workIDs := []string{"work-live"}
	ledger := &admissionProjectionLedger{
		streamGeneration: "generation-live",
		events: []recordings.FactoryEvent{
			{
				Type:    factorydefinitions.FactoryEventTypeWorkRequest,
				Context: factorydefinitions.FactoryEventContext{Sequence: 2, WorkIDs: &workIDs},
				Payload: []byte(`{"works":[{"workId":"work-live"}]}`),
			},
			{
				Type:    factorydefinitions.FactoryEventTypeWorkStateChange,
				Context: factorydefinitions.FactoryEventContext{Sequence: 5, WorkIDs: &workIDs},
				Payload: []byte(`{"workId":"work-live"}`),
			},
		},
	}
	snapshot := work.ReadSnapshot{Items: []work.ReadModel{{WorkID: "work-live"}}}

	annotateRuntimeWorkStateSequences(&snapshot, ledger)

	if snapshot.StreamGenerationID != "generation-live" {
		t.Fatalf("stream generation = %q, want generation-live", snapshot.StreamGenerationID)
	}
	if len(snapshot.Items) != 1 || !snapshot.Items[0].CurrentStateSequenceKnown || snapshot.Items[0].CurrentStateSequence != 5 {
		t.Fatalf("runtime state cursor = %#v, want known sequence 5", snapshot.Items)
	}
}
