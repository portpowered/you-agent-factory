package materialize

import (
	"slices"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
)

func TestCollectPublicWorkTokens_MarkingOnlyUnchanged(t *testing.T) {
	marking := &petri.MarkingSnapshot{
		Tokens: map[string]*factorytoken.Token{
			"tok-1": testWorkToken("tok-1", "work-a", "task:init", "task"),
			"tok-2": testWorkToken("tok-2", "work-b", "task:review", "task"),
		},
	}

	got := CollectPublicWorkTokens(marking, nil)
	assertTokenIDs(t, got.Tokens, []string{"tok-1", "tok-2"})
	if len(got.InFlightOnlyByID) != 0 {
		t.Fatalf("InFlightOnlyByID = %#v, want empty", got.InFlightOnlyByID)
	}
}

func TestCollectPublicWorkTokens_DispatchOnlyVisible(t *testing.T) {
	dispatchToken := factorytoken.Token{
		ID:      "tok-dispatch",
		PlaceID: "task:processing",
		Color: factorytoken.Color{
			DataType:   factorytoken.DataTypeWork,
			WorkID:     "work-in-flight",
			WorkTypeID: "task",
			Name:       "In flight item",
		},
	}
	dispatches := map[string]*interfaces.DispatchEntry{
		"dispatch-1": {
			DispatchID:     "dispatch-1",
			ConsumedTokens: []factorytoken.Token{dispatchToken},
		},
	}

	got := CollectPublicWorkTokens(&petri.MarkingSnapshot{Tokens: map[string]*factorytoken.Token{}}, dispatches)
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
	markingToken := testWorkToken("tok-mark", "work-shared", "task:init", "task")
	markingToken.Color.Name = "Marking copy"

	dispatchToken := factorytoken.Token{
		ID:      "tok-dispatch",
		PlaceID: "task:processing",
		Color: factorytoken.Color{
			DataType:   factorytoken.DataTypeWork,
			WorkID:     "work-shared",
			WorkTypeID: "task",
			Name:       "Dispatch copy",
		},
	}
	dispatches := map[string]*interfaces.DispatchEntry{
		"dispatch-1": {ConsumedTokens: []factorytoken.Token{dispatchToken}},
	}

	got := CollectPublicWorkTokens(&petri.MarkingSnapshot{Tokens: map[string]*factorytoken.Token{
		"tok-mark": markingToken,
	}}, dispatches)

	assertTokenIDs(t, got.Tokens, []string{"tok-mark"})
	if got.Tokens[0].Color.Name != "Marking copy" {
		t.Fatalf("token name = %q, want Marking copy", got.Tokens[0].Color.Name)
	}
	if len(got.InFlightOnlyByID) != 0 {
		t.Fatalf("InFlightOnlyByID = %#v, want empty", got.InFlightOnlyByID)
	}
}

func TestCollectPublicWorkTokens_ExcludesResourceAndSystemTime(t *testing.T) {
	marking := &petri.MarkingSnapshot{
		Tokens: map[string]*factorytoken.Token{
			"tok-work": testWorkToken("tok-work", "work-visible", "task:init", "task"),
			"tok-resource": {
				ID: "tok-resource",
				Color: factorytoken.Color{
					DataType: factorytoken.DataTypeResource,
					WorkID:   "resource-1",
				},
			},
			"tok-system": {
				ID: "tok-system",
				Color: factorytoken.Color{
					DataType:   factorytoken.DataTypeWork,
					WorkTypeID: interfaces.SystemTimeWorkTypeID,
					WorkID:     "system-time",
				},
			},
		},
	}
	dispatches := map[string]*interfaces.DispatchEntry{
		"dispatch-1": {
			ConsumedTokens: []factorytoken.Token{
				{
					ID: "tok-dispatch-resource",
					Color: factorytoken.Color{
						DataType: factorytoken.DataTypeResource,
						WorkID:   "resource-dispatch",
					},
				},
				{
					ID: "tok-dispatch-system",
					Color: factorytoken.Color{
						DataType:   factorytoken.DataTypeWork,
						WorkTypeID: interfaces.SystemTimeWorkTypeID,
						WorkID:     "system-dispatch",
					},
				},
				{
					ID:      "tok-dispatch-work",
					PlaceID: "task:processing",
					Color: factorytoken.Color{
						DataType:   factorytoken.DataTypeWork,
						WorkID:     "work-dispatch-only",
						WorkTypeID: "task",
					},
				},
			},
		},
	}

	got := CollectPublicWorkTokens(marking, dispatches)
	assertTokenIDs(t, got.Tokens, []string{"tok-work", "tok-dispatch-work"})
	if _, ok := got.InFlightOnlyByID["tok-dispatch-work"]; !ok {
		t.Fatalf("InFlightOnlyByID = %#v, want tok-dispatch-work", got.InFlightOnlyByID)
	}
}

func TestIsPublicWorkToken(t *testing.T) {
	if !IsPublicWorkToken(testWorkToken("tok-1", "work-1", "task:init", "task")) {
		t.Fatal("expected public work token")
	}
	if IsPublicWorkToken(&factorytoken.Token{
		Color: factorytoken.Color{DataType: factorytoken.DataTypeResource},
	}) {
		t.Fatal("resource token should not be public work")
	}
	if IsPublicWorkToken(&factorytoken.Token{
		Color: factorytoken.Color{WorkTypeID: interfaces.SystemTimeWorkTypeID},
	}) {
		t.Fatal("system time token should not be public work")
	}
}

func testWorkToken(id, workID, placeID, workTypeID string) *factorytoken.Token {
	return &factorytoken.Token{
		ID:      id,
		PlaceID: placeID,
		Color: factorytoken.Color{
			DataType:   factorytoken.DataTypeWork,
			WorkID:     workID,
			WorkTypeID: workTypeID,
		},
	}
}

func assertTokenIDs(t *testing.T, tokens []*factorytoken.Token, want []string) {
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

func tokenIDs(tokens []*factorytoken.Token) []string {
	ids := make([]string, len(tokens))
	for i, token := range tokens {
		if token != nil {
			ids[i] = token.ID
		}
	}
	return ids
}
