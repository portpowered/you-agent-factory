package replay_contracts

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestReplayEventCountCountsMatchingEventTypes(t *testing.T) {
	artifact := &interfaces.ReplayArtifact{
		Events: []factoryapi.FactoryEvent{
			{Type: factoryapi.FactoryEventTypeDispatchRequest},
			{Type: factoryapi.FactoryEventTypeDispatchResponse},
			{Type: factoryapi.FactoryEventTypeDispatchRequest},
		},
	}

	if got := replayEventCount(artifact, factoryapi.FactoryEventTypeDispatchRequest); got != 2 {
		t.Fatalf("replayEventCount(dispatch request) = %d, want 2", got)
	}
	if got := replayEventCount(artifact, factoryapi.FactoryEventTypeWorkRequest); got != 0 {
		t.Fatalf("replayEventCount(work request) = %d, want 0", got)
	}
}

func TestFactoryRelationsValueReturnsUnderlyingSlice(t *testing.T) {
	var nilRelations *[]factoryapi.Relation
	if got := factoryRelationsValue(nilRelations); got != nil {
		t.Fatalf("factoryRelationsValue(nil) = %#v, want nil", got)
	}

	relations := []factoryapi.Relation{{
		Type:           factoryapi.RelationTypeDependsOn,
		SourceWorkName: "generated-beta",
		TargetWorkName: "generated-alpha",
	}}

	got := factoryRelationsValue(&relations)
	if len(got) != 1 {
		t.Fatalf("factoryRelationsValue(...) length = %d, want 1", len(got))
	}
	if got[0].SourceWorkName != relations[0].SourceWorkName || got[0].TargetWorkName != relations[0].TargetWorkName || got[0].Type != relations[0].Type {
		t.Fatalf("factoryRelationsValue(...) = %#v, want %#v", got[0], relations[0])
	}
}
