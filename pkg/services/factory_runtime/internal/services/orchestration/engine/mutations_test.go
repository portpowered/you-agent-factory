package engine

import (
	"testing"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factorytoken "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/token"
)

func testPlaces() map[string]*petri.Place {
	return map[string]*petri.Place{
		"wt:init":    {ID: "wt:init", TypeID: "wt", State: "init"},
		"wt:process": {ID: "wt:process", TypeID: "wt", State: "process"},
		"wt:done":    {ID: "wt:done", TypeID: "wt", State: "done"},
	}
}

func testToken(id, placeID string) *factorytoken.Token {
	return &factorytoken.Token{
		ID:        id,
		PlaceID:   placeID,
		CreatedAt: time.Now(),
		EnteredAt: time.Now(),
		Color: factorytoken.Color{
			WorkID:     "work-" + id,
			WorkTypeID: "wt",
		},
		History: factorytoken.History{
			TotalVisits:         make(map[string]int),
			ConsecutiveFailures: make(map[string]int),
			PlaceVisits:         make(map[string]int),
		},
	}
}

func TestApplyMove(t *testing.T) {
	marking := petri.NewMarking("wf-1")
	tok := testToken("t1", "wt:init")
	marking.AddToken(tok)
	places := testPlaces()

	now := time.Unix(1, 0)
	err := applyMutations(marking, places, []interfaces.MarkingMutation{
		{Type: interfaces.MutationMove, TokenID: "t1", FromPlace: "wt:init", ToPlace: "wt:process", Reason: "transition fired"},
	}, now)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Token should be in new place
	if marking.Tokens["t1"].PlaceID != "wt:process" {
		t.Errorf("expected PlaceID wt:process, got %s", marking.Tokens["t1"].PlaceID)
	}
	// Place index should be updated
	if len(marking.TokensInPlace("wt:init")) != 0 {
		t.Errorf("expected no tokens in wt:init")
	}
	if len(marking.TokensInPlace("wt:process")) != 1 {
		t.Errorf("expected 1 token in wt:process")
	}
	if !marking.Tokens["t1"].EnteredAt.Equal(now) {
		t.Errorf("EnteredAt = %v, want injected %v", marking.Tokens["t1"].EnteredAt, now)
	}
}

func TestApplyCreate(t *testing.T) {
	marking := petri.NewMarking("wf-1")
	places := testPlaces()

	newTok := testToken("t2", "")
	err := applyMutations(marking, places, []interfaces.MarkingMutation{
		{Type: interfaces.MutationCreate, ToPlace: "wt:init", Reason: "token submitted", NewToken: newTok},
	}, time.Unix(1, 0))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := marking.Tokens["t2"]; !ok {
		t.Fatal("token t2 not found in marking")
	}
	if marking.Tokens["t2"].PlaceID != "wt:init" {
		t.Errorf("expected PlaceID wt:init, got %s", marking.Tokens["t2"].PlaceID)
	}
	if len(marking.TokensInPlace("wt:init")) != 1 {
		t.Errorf("expected 1 token in wt:init")
	}
}

func TestApplyCreate_PreservesPrecomputedEnteredAt(t *testing.T) {
	marking := petri.NewMarking("wf-1")
	places := testPlaces()
	expectedEnteredAt := time.Date(2026, time.April, 6, 8, 1, 0, 0, time.UTC)

	newTok := testToken("t2", "")
	newTok.EnteredAt = expectedEnteredAt
	err := applyMutations(marking, places, []interfaces.MarkingMutation{
		{Type: interfaces.MutationCreate, ToPlace: "wt:init", Reason: "token submitted", NewToken: newTok},
	}, time.Unix(1, 0))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !marking.Tokens["t2"].EnteredAt.Equal(expectedEnteredAt) {
		t.Fatalf("EnteredAt = %v, want %v", marking.Tokens["t2"].EnteredAt, expectedEnteredAt)
	}
}

func TestApplyConsume(t *testing.T) {
	marking := petri.NewMarking("wf-1")
	tok := testToken("t3", "wt:done")
	marking.AddToken(tok)
	places := testPlaces()

	err := applyMutations(marking, places, []interfaces.MarkingMutation{
		{Type: interfaces.MutationConsume, TokenID: "t3", FromPlace: "wt:done", Reason: "resource released"},
	}, time.Unix(1, 0))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := marking.Tokens["t3"]; ok {
		t.Error("token t3 should have been consumed")
	}
	if len(marking.TokensInPlace("wt:done")) != 0 {
		t.Error("expected no tokens in wt:done")
	}
	if _, ok := marking.PlaceTokens["wt:done"]; ok {
		t.Error("expected wt:done place index to be removed after consume")
	}
}

func TestApplyMoveNonExistentToken(t *testing.T) {
	marking := petri.NewMarking("wf-1")
	places := testPlaces()

	err := applyMutations(marking, places, []interfaces.MarkingMutation{
		{Type: interfaces.MutationMove, TokenID: "ghost", FromPlace: "wt:init", ToPlace: "wt:process"},
	}, time.Unix(1, 0))

	if err == nil {
		t.Fatal("expected error for non-existent token")
	}
}

func TestApplyMoveNonExistentPlace(t *testing.T) {
	marking := petri.NewMarking("wf-1")
	tok := testToken("t1", "wt:init")
	marking.AddToken(tok)
	places := testPlaces()

	err := applyMutations(marking, places, []interfaces.MarkingMutation{
		{Type: interfaces.MutationMove, TokenID: "t1", FromPlace: "wt:init", ToPlace: "wt:nowhere"},
	}, time.Unix(1, 0))

	if err == nil {
		t.Fatal("expected error for non-existent to-place")
	}
}

func TestApplyConsumeNonExistentToken(t *testing.T) {
	marking := petri.NewMarking("wf-1")
	places := testPlaces()

	err := applyMutations(marking, places, []interfaces.MarkingMutation{
		{Type: interfaces.MutationConsume, TokenID: "ghost", FromPlace: "wt:init"},
	}, time.Unix(1, 0))

	if err == nil {
		t.Fatal("expected error for consuming non-existent token")
	}
}

func TestApplyCreateNonExistentPlace(t *testing.T) {
	marking := petri.NewMarking("wf-1")
	places := testPlaces()

	newTok := testToken("t1", "")
	err := applyMutations(marking, places, []interfaces.MarkingMutation{
		{Type: interfaces.MutationCreate, ToPlace: "wt:nowhere", NewToken: newTok},
	}, time.Unix(1, 0))

	if err == nil {
		t.Fatal("expected error for non-existent to-place")
	}
}

func TestApplyMultipleMutations(t *testing.T) {
	marking := petri.NewMarking("wf-1")
	tok := testToken("t1", "wt:init")
	marking.AddToken(tok)
	places := testPlaces()

	newTok := testToken("t2", "")
	err := applyMutations(marking, places, []interfaces.MarkingMutation{
		{Type: interfaces.MutationMove, TokenID: "t1", FromPlace: "wt:init", ToPlace: "wt:process", Reason: "advance"},
		{Type: interfaces.MutationCreate, ToPlace: "wt:init", Reason: "new work", NewToken: newTok},
	}, time.Unix(1, 0))

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if marking.Tokens["t1"].PlaceID != "wt:process" {
		t.Errorf("t1 should be in wt:process")
	}
	if marking.Tokens["t2"].PlaceID != "wt:init" {
		t.Errorf("t2 should be in wt:init")
	}
}

func TestApplyCreateMissingNewToken(t *testing.T) {
	marking := petri.NewMarking("wf-1")
	places := testPlaces()

	err := applyMutations(marking, places, []interfaces.MarkingMutation{
		{Type: interfaces.MutationCreate, ToPlace: "wt:init"},
	}, time.Unix(1, 0))

	if err == nil {
		t.Fatal("expected error for CREATE without NewToken")
	}
}
