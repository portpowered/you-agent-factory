package petri

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestMarking_AddToken(t *testing.T) {
	m := NewMarking("wf-1")

	token := &interfaces.Token{
		ID:      "t1",
		PlaceID: "place-a",
		Color: interfaces.TokenColor{
			WorkID:     "work-1",
			WorkTypeID: "code-change",
		},
		CreatedAt: time.Now(),
		EnteredAt: time.Now(),
	}

	m.AddToken(token)

	if _, ok := m.Tokens["t1"]; !ok {
		t.Fatal("expected token t1 in Tokens map")
	}

	ids := m.PlaceTokens["place-a"]
	if len(ids) != 1 || ids[0] != "t1" {
		t.Fatalf("expected PlaceTokens[place-a] = [t1], got %v", ids)
	}
}

func TestMarking_AddMultipleTokensSamePlace(t *testing.T) {
	m := NewMarking("wf-1")

	m.AddToken(&interfaces.Token{ID: "t1", PlaceID: "place-a"})
	m.AddToken(&interfaces.Token{ID: "t2", PlaceID: "place-a"})

	ids := m.PlaceTokens["place-a"]
	if len(ids) != 2 {
		t.Fatalf("expected 2 tokens in place-a, got %d", len(ids))
	}
}

func TestMarking_RemoveToken(t *testing.T) {
	m := NewMarking("wf-1")

	m.AddToken(&interfaces.Token{ID: "t1", PlaceID: "place-a"})
	m.AddToken(&interfaces.Token{ID: "t2", PlaceID: "place-a"})

	m.RemoveToken("t1")

	if _, ok := m.Tokens["t1"]; ok {
		t.Fatal("expected token t1 to be removed")
	}

	ids := m.PlaceTokens["place-a"]
	if len(ids) != 1 || ids[0] != "t2" {
		t.Fatalf("expected PlaceTokens[place-a] = [t2], got %v", ids)
	}
}

func TestMarking_RemoveLastTokenCleansPlaceIndex(t *testing.T) {
	m := NewMarking("wf-1")

	m.AddToken(&interfaces.Token{ID: "t1", PlaceID: "place-a"})
	m.RemoveToken("t1")

	if _, ok := m.PlaceTokens["place-a"]; ok {
		t.Fatal("expected place-a to be removed from PlaceTokens when empty")
	}
}

func TestMarking_RemoveNonExistentToken(t *testing.T) {
	m := NewMarking("wf-1")
	// Should not panic
	m.RemoveToken("nonexistent")
}

func TestMarking_TokensInPlace(t *testing.T) {
	m := NewMarking("wf-1")

	m.AddToken(&interfaces.Token{ID: "t1", PlaceID: "place-a", Color: interfaces.TokenColor{WorkID: "w1"}})
	m.AddToken(&interfaces.Token{ID: "t2", PlaceID: "place-a", Color: interfaces.TokenColor{WorkID: "w2"}})
	m.AddToken(&interfaces.Token{ID: "t3", PlaceID: "place-b", Color: interfaces.TokenColor{WorkID: "w3"}})

	tokensA := m.TokensInPlace("place-a")
	if len(tokensA) != 2 {
		t.Fatalf("expected 2 tokens in place-a, got %d", len(tokensA))
	}

	tokensB := m.TokensInPlace("place-b")
	if len(tokensB) != 1 {
		t.Fatalf("expected 1 token in place-b, got %d", len(tokensB))
	}

	tokensC := m.TokensInPlace("place-c")
	if len(tokensC) != 0 {
		t.Fatalf("expected 0 tokens in place-c, got %d", len(tokensC))
	}
}

func TestMarking_SnapshotIsIndependent(t *testing.T) {
	m := NewMarking("wf-1")

	m.AddToken(&interfaces.Token{
		ID:      "t1",
		PlaceID: "place-a",
		Color: interfaces.TokenColor{
			WorkID:     "work-1",
			WorkTypeID: "code-change",
			Tags:       map[string]string{"key": "value"},
			Payload:    []byte("hello"),
		},
		History: interfaces.TokenHistory{
			TotalVisits:         map[string]int{"tr1": 3},
			ConsecutiveFailures: map[string]int{"tr1": 1},
			PlaceVisits:         map[string]int{"place-a": 2},
			FailureLog:          []interfaces.FailureRecord{{TransitionID: "tr1", Error: "oops", Attempt: 1}},
		},
	})
	m.TraceContext["trace"] = "abc"

	snap := m.Snapshot()

	// Mutate the original marking
	m.AddToken(&interfaces.Token{ID: "t2", PlaceID: "place-b"})
	m.Tokens["t1"].Color.Tags["key"] = "mutated"
	m.Tokens["t1"].Color.Payload[0] = 'X'
	m.Tokens["t1"].History.TotalVisits["tr1"] = 99
	m.Tokens["t1"].History.ConsecutiveFailures["tr1"] = 99
	m.Tokens["t1"].History.PlaceVisits["place-a"] = 99
	m.TraceContext["trace"] = "mutated"

	// Verify snapshot is unaffected
	if _, ok := snap.Tokens["t2"]; ok {
		t.Fatal("snapshot should not contain t2 added after snapshot")
	}

	snapToken := snap.Tokens["t1"]
	if snapToken.Color.Tags["key"] != "value" {
		t.Fatalf("snapshot tag mutated: got %q, want %q", snapToken.Color.Tags["key"], "value")
	}
	if snapToken.Color.Payload[0] != 'h' {
		t.Fatal("snapshot payload mutated")
	}
	if snapToken.History.TotalVisits["tr1"] != 3 {
		t.Fatalf("snapshot TotalVisits mutated: got %d, want 3", snapToken.History.TotalVisits["tr1"])
	}
	if snapToken.History.ConsecutiveFailures["tr1"] != 1 {
		t.Fatalf("snapshot ConsecutiveFailures mutated: got %d, want 1", snapToken.History.ConsecutiveFailures["tr1"])
	}
	if snapToken.History.PlaceVisits["place-a"] != 2 {
		t.Fatalf("snapshot PlaceVisits mutated: got %d, want 2", snapToken.History.PlaceVisits["place-a"])
	}
	if snap.TraceContext["trace"] != "abc" {
		t.Fatalf("snapshot TraceContext mutated: got %q, want %q", snap.TraceContext["trace"], "abc")
	}
}

func TestMarkingSnapshot_TokensInPlace(t *testing.T) {
	m := NewMarking("wf-1")
	m.AddToken(&interfaces.Token{ID: "t1", PlaceID: "place-a", Color: interfaces.TokenColor{WorkID: "w1"}})
	m.AddToken(&interfaces.Token{ID: "t2", PlaceID: "place-a", Color: interfaces.TokenColor{WorkID: "w2"}})

	snap := m.Snapshot()

	tokens := snap.TokensInPlace("place-a")
	if len(tokens) != 2 {
		t.Fatalf("expected 2 tokens in snapshot place-a, got %d", len(tokens))
	}

	empty := snap.TokensInPlace("nonexistent")
	if len(empty) != 0 {
		t.Fatalf("expected 0 tokens for nonexistent place, got %d", len(empty))
	}
}

func TestMarking_PlaceTokensIndexConsistency(t *testing.T) {
	m := NewMarking("wf-1")

	m.AddToken(&interfaces.Token{ID: "t1", PlaceID: "p1"})
	m.AddToken(&interfaces.Token{ID: "t2", PlaceID: "p1"})
	m.AddToken(&interfaces.Token{ID: "t3", PlaceID: "p2"})

	m.RemoveToken("t1")

	// p1 should have only t2
	if ids := m.PlaceTokens["p1"]; len(ids) != 1 || ids[0] != "t2" {
		t.Fatalf("expected [t2] in p1, got %v", ids)
	}

	// p2 should still have t3
	if ids := m.PlaceTokens["p2"]; len(ids) != 1 || ids[0] != "t3" {
		t.Fatalf("expected [t3] in p2, got %v", ids)
	}
}
