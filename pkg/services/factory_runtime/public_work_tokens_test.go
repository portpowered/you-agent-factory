package factory_test

import (
	"slices"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

func TestCollectPublicWorkTokens_MarkingOnlyUnchanged(t *testing.T) {
	t.Parallel()
	marking := &factoryruntime.PetriMarkingSnapshot{
		Tokens: map[string]*factoryruntime.RuntimeToken{
			"tok-1": testWorkToken("tok-1", "work-a", "task:init", "task"),
			"tok-2": testWorkToken("tok-2", "work-b", "task:review", "task"),
		},
	}

	got := factoryruntime.CollectPublicWorkTokens(marking.Tokens, nil)
	assertTokenIDs(t, got.Tokens, []string{"tok-1", "tok-2"})
	if len(got.InFlightOnlyByID) != 0 {
		t.Fatalf("InFlightOnlyByID = %#v, want empty", got.InFlightOnlyByID)
	}
}

func TestCollectPublicWorkTokens_DispatchOnlyVisible(t *testing.T) {
	t.Parallel()
	dispatchToken := factoryruntime.RuntimeToken{
		ID:      "tok-dispatch",
		PlaceID: "task:processing",
		Color: factoryruntime.RuntimeTokenColor{
			DataType:   factoryruntime.RuntimeTokenDataTypeWork,
			WorkID:     "work-in-flight",
			WorkTypeID: "task",
			Name:       "In flight item",
		},
	}
	dispatches := map[string]*interfaces.DispatchEntry{
		"dispatch-1": {
			DispatchID:     "dispatch-1",
			ConsumedTokens: []factoryruntime.RuntimeToken{dispatchToken},
		},
	}

	got := factoryruntime.CollectPublicWorkTokens(map[string]*factoryruntime.RuntimeToken{}, dispatches)
	if len(got.Tokens) != 1 {
		t.Fatalf("token count = %d, want 1", len(got.Tokens))
	}
	if got.Tokens[0].Color.WorkID != "work-in-flight" {
		t.Fatalf("work ID = %q, want work-in-flight", got.Tokens[0].Color.WorkID)
	}
	if _, ok := got.InFlightOnlyByID["tok-dispatch"]; !ok {
		t.Fatalf("InFlightOnlyByID = %#v, want tok-dispatch", got.InFlightOnlyByID)
	}
}

func TestCollectPublicWorkTokens_MarkingWinsOnWorkIDDedupe(t *testing.T) {
	t.Parallel()
	markingToken := testWorkToken("tok-mark", "work-shared", "task:init", "task")
	markingToken.Color.Name = "Marking copy"

	dispatchToken := factoryruntime.RuntimeToken{
		ID:      "tok-dispatch",
		PlaceID: "task:processing",
		Color: factoryruntime.RuntimeTokenColor{
			DataType:   factoryruntime.RuntimeTokenDataTypeWork,
			WorkID:     "work-shared",
			WorkTypeID: "task",
			Name:       "Dispatch copy",
		},
	}
	dispatches := map[string]*interfaces.DispatchEntry{
		"dispatch-1": {ConsumedTokens: []factoryruntime.RuntimeToken{dispatchToken}},
	}

	got := factoryruntime.CollectPublicWorkTokens(map[string]*factoryruntime.RuntimeToken{
		"tok-mark": markingToken,
	}, dispatches)

	assertTokenIDs(t, got.Tokens, []string{"tok-mark"})
	if got.Tokens[0].Color.Name != "Marking copy" {
		t.Fatalf("token name = %q, want Marking copy", got.Tokens[0].Color.Name)
	}
	if len(got.InFlightOnlyByID) != 0 {
		t.Fatalf("InFlightOnlyByID = %#v, want empty", got.InFlightOnlyByID)
	}
}

func TestCollectPublicWorkTokens_ExcludesResourceAndSystemTime(t *testing.T) {
	t.Parallel()
	marking := &factoryruntime.PetriMarkingSnapshot{
		Tokens: map[string]*factoryruntime.RuntimeToken{
			"tok-work": testWorkToken("tok-work", "work-visible", "task:init", "task"),
			"tok-resource": {
				ID: "tok-resource",
				Color: factoryruntime.RuntimeTokenColor{
					DataType: factoryruntime.RuntimeTokenDataTypeResource,
					WorkID:   "resource-1",
				},
			},
			"tok-system": {
				ID: "tok-system",
				Color: factoryruntime.RuntimeTokenColor{
					DataType:   factoryruntime.RuntimeTokenDataTypeWork,
					WorkTypeID: interfaces.SystemTimeWorkTypeID,
					WorkID:     "system-time",
				},
			},
		},
	}
	dispatches := map[string]*interfaces.DispatchEntry{
		"dispatch-1": {
			ConsumedTokens: []factoryruntime.RuntimeToken{
				{
					ID: "tok-dispatch-resource",
					Color: factoryruntime.RuntimeTokenColor{
						DataType: factoryruntime.RuntimeTokenDataTypeResource,
						WorkID:   "resource-dispatch",
					},
				},
				{
					ID: "tok-dispatch-system",
					Color: factoryruntime.RuntimeTokenColor{
						DataType:   factoryruntime.RuntimeTokenDataTypeWork,
						WorkTypeID: interfaces.SystemTimeWorkTypeID,
						WorkID:     "system-dispatch",
					},
				},
				{
					ID:      "tok-dispatch-work",
					PlaceID: "task:processing",
					Color: factoryruntime.RuntimeTokenColor{
						DataType:   factoryruntime.RuntimeTokenDataTypeWork,
						WorkID:     "work-dispatch-only",
						WorkTypeID: "task",
					},
				},
			},
		},
	}

	got := factoryruntime.CollectPublicWorkTokens(marking.Tokens, dispatches)
	assertTokenIDs(t, got.Tokens, []string{"tok-work", "tok-dispatch-work"})
	if _, ok := got.InFlightOnlyByID["tok-dispatch-work"]; !ok {
		t.Fatalf("InFlightOnlyByID = %#v, want tok-dispatch-work", got.InFlightOnlyByID)
	}
}

func TestIsPublicWorkToken(t *testing.T) {
	t.Parallel()
	if !factoryruntime.IsPublicWorkToken(testWorkToken("tok-1", "work-1", "task:init", "task")) {
		t.Fatal("expected public work token")
	}
	if factoryruntime.IsPublicWorkToken(&factoryruntime.RuntimeToken{
		Color: factoryruntime.RuntimeTokenColor{DataType: factoryruntime.RuntimeTokenDataTypeResource},
	}) {
		t.Fatal("resource token should not be public work")
	}
	if factoryruntime.IsPublicWorkToken(&factoryruntime.RuntimeToken{
		Color: factoryruntime.RuntimeTokenColor{WorkTypeID: interfaces.SystemTimeWorkTypeID},
	}) {
		t.Fatal("system time token should not be public work")
	}
}

func testWorkToken(id, workID, placeID, workTypeID string) *factoryruntime.RuntimeToken {
	return &factoryruntime.RuntimeToken{
		ID:      id,
		PlaceID: placeID,
		Color: factoryruntime.RuntimeTokenColor{
			DataType:   factoryruntime.RuntimeTokenDataTypeWork,
			WorkID:     workID,
			WorkTypeID: workTypeID,
		},
	}
}

func assertTokenIDs(t *testing.T, tokens []*factoryruntime.RuntimeToken, want []string) {
	t.Helper()
	got := tokenIDs(tokens)
	slices.Sort(got)
	wantSorted := append([]string(nil), want...)
	slices.Sort(wantSorted)
	if len(got) != len(wantSorted) {
		t.Fatalf("token count = %d, want %d; got %v", len(got), len(wantSorted), got)
	}
	for i := range wantSorted {
		if got[i] != wantSorted[i] {
			t.Fatalf("token IDs = %v, want %v", got, wantSorted)
		}
	}
}

func tokenIDs(tokens []*factoryruntime.RuntimeToken) []string {
	ids := make([]string, len(tokens))
	for i, token := range tokens {
		if token != nil {
			ids[i] = token.ID
		}
	}
	return ids
}
