package projections

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestFactoryRelationsFromGenerated_PreservesRequestNameAndContextResolution(t *testing.T) {
	reducer := newFactoryWorldReducer(1)
	reducer.stateValue.WorkRequestsByID["request-1"] = interfaces.WorkRequestPayload{
		RequestID: "request-1",
		WorkItems: []interfaces.FactoryWorkItem{
			{ID: "work-parent", DisplayName: "parent"},
			{ID: "work-child", DisplayName: "child"},
			{ID: "work-prerequisite", DisplayName: "prerequisite"},
		},
	}

	relations := []factoryapi.Relation{
		{
			Type:           factoryapi.RelationTypeParentChild,
			SourceWorkName: "child",
			TargetWorkName: "parent",
		},
		{
			Type:           factoryapi.RelationTypeDependsOn,
			SourceWorkName: "",
			TargetWorkName: "prerequisite",
			RequiredState:  stringPtrForProjectionTest("complete"),
		},
		{
			Type:           factoryapi.RelationTypeDependsOn,
			SourceWorkName: "child",
			TargetWorkName: "missing",
		},
	}

	got := reducer.factoryRelationsFromGenerated(&relations, factoryapi.FactoryEventContext{
		RequestId: stringPtrForProjectionTest("request-1"),
		TraceIds:  &[]string{"trace-1"},
		WorkIds:   &[]string{"work-child", "work-prerequisite"},
	})

	if len(got) != 2 {
		t.Fatalf("converted relation count = %d, want 2 (%#v)", len(got), got)
	}
	if got[0].SourceWorkID != "work-child" || got[0].TargetWorkID != "work-parent" {
		t.Fatalf("first relation = %#v, want child -> parent resolved by request names", got[0])
	}
	if got[1].SourceWorkID != "work-child" || got[1].TargetWorkID != "work-prerequisite" || got[1].RequiredState != "complete" {
		t.Fatalf("second relation = %#v, want context fallback source and preserved required state", got[1])
	}
}

func TestFactoryRelationsFromGenerated_PreservesNilInput(t *testing.T) {
	reducer := newFactoryWorldReducer(1)

	if got := reducer.factoryRelationsFromGenerated(nil, factoryapi.FactoryEventContext{}); got != nil {
		t.Fatalf("nil relations = %#v, want nil", got)
	}
}
