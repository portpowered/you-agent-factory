package token

import (
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

func TestCloneDetachesNestedTokenState(t *testing.T) {
	original := Token{Color: Color{
		Tags:                map[string]string{"owner": "original"},
		Content:             []work.WorkContentPart{{Type: work.WorkContentPartTypeJSON, JSON: []byte(`{"ok":true}`)}},
		InvocationArguments: &work.InvocationArguments{Arguments: map[string]work.InvocationArgument{"input": {Values: []string{"a"}}}},
	}, History: History{TotalVisits: map[string]int{"ready": 1}, FailureLog: []Failure{{Error: "original"}}}}

	clone := Clone(original)
	clone.Color.Tags["owner"] = "clone"
	clone.Color.Content[0].JSON[0] = '['
	clone.Color.InvocationArguments.Arguments["input"] = work.InvocationArgument{Values: []string{"b"}}
	clone.History.TotalVisits["ready"] = 2
	clone.History.FailureLog[0].Error = "clone"

	if original.Color.Tags["owner"] != "original" || string(original.Color.Content[0].JSON) != `{"ok":true}` || original.History.TotalVisits["ready"] != 1 || original.History.FailureLog[0].Error != "original" {
		t.Fatalf("Clone() mutated original nested state: %#v", original)
	}
	if original.Color.InvocationArguments.Arguments["input"].Values[0] != "a" {
		t.Fatalf("Clone() mutated original invocation arguments: %#v", original.Color.InvocationArguments)
	}
}

func TestCloneToken_DetachesNestedMutableRuntimeFields(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	original := Token{
		ID:      "token-1",
		PlaceID: "place-1",
		Color: Color{
			Name:                     "task",
			RequestID:                "request-1",
			WorkID:                   "work-1",
			WorkTypeID:               "type-1",
			DataType:                 DataTypeWork,
			ChainingTraceDepth:       2,
			CurrentChainingTraceID:   "trace-current",
			PreviousChainingTraceIDs: []string{"trace-a", "trace-b"},
			TraceID:                  "trace-current",
			ParentID:                 "parent-1",
			Tags:                     map[string]string{"priority": "high"},
			Relations: []work.Relation{{
				Type:         work.RelationDependsOn,
				TargetWorkID: "work-upstream",
			}},
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeText,
				Text: "original content",
			}},
			Payload: []byte("payload"),
		},
		CreatedAt: now,
		EnteredAt: now.Add(time.Minute),
		History: History{
			TotalVisits:         map[string]int{"queued": 1},
			ConsecutiveFailures: map[string]int{"queued": 2},
			PlaceVisits:         map[string]int{"queued": 3},
			TotalDuration:       5 * time.Minute,
			LastError:           "timeout",
			FailureLog: []Failure{{
				TransitionID: "transition-1",
				Timestamp:    now,
				Error:        "timeout",
				Attempt:      2,
			}},
		},
	}

	cloned := Clone(original)
	cloned.Color.PreviousChainingTraceIDs[0] = "trace-z"
	cloned.Color.Tags["priority"] = "low"
	cloned.Color.Relations[0].TargetWorkID = "work-mutated"
	cloned.Color.Content[0].Text = "mutated content"
	cloned.Color.Payload[0] = 'P'
	cloned.History.TotalVisits["queued"] = 9
	cloned.History.ConsecutiveFailures["queued"] = 8
	cloned.History.PlaceVisits["queued"] = 7
	cloned.History.FailureLog[0].Error = "mutated"

	if original.Color.PreviousChainingTraceIDs[0] != "trace-a" {
		t.Fatalf("original previous chaining traces = %#v, want trace-a unchanged", original.Color.PreviousChainingTraceIDs)
	}
	if original.Color.Tags["priority"] != "high" {
		t.Fatalf("original tags = %#v, want priority unchanged", original.Color.Tags)
	}
	if original.Color.Relations[0].TargetWorkID != "work-upstream" {
		t.Fatalf("original relations = %#v, want target unchanged", original.Color.Relations)
	}
	if original.Color.Content[0].Text != "original content" {
		t.Fatalf("original content = %#v, want text unchanged", original.Color.Content)
	}
	if string(original.Color.Payload) != "payload" {
		t.Fatalf("original payload = %q, want payload unchanged", original.Color.Payload)
	}
	if original.History.TotalVisits["queued"] != 1 {
		t.Fatalf("original total visits = %#v, want queued=1 unchanged", original.History.TotalVisits)
	}
	if original.History.ConsecutiveFailures["queued"] != 2 {
		t.Fatalf("original consecutive failures = %#v, want queued=2 unchanged", original.History.ConsecutiveFailures)
	}
	if original.History.PlaceVisits["queued"] != 3 {
		t.Fatalf("original place visits = %#v, want queued=3 unchanged", original.History.PlaceVisits)
	}
	if original.History.FailureLog[0].Error != "timeout" {
		t.Fatalf("original failure log = %#v, want timeout unchanged", original.History.FailureLog)
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this clone contract test keeps nil, empty, and detached-copy assertions together on the public seam.
func TestCloneToken_PreserveNilAndEmptyValues(t *testing.T) {
	tests := []struct {
		name  string
		token Token
	}{
		{
			name: "nil fields stay nil",
			token: Token{
				Color:   Color{},
				History: History{},
			},
		},
		{
			name: "empty but allocated slices and maps become detached empty values",
			token: Token{
				Color: Color{
					PreviousChainingTraceIDs: []string{},
					Tags:                     map[string]string{},
					Relations:                []work.Relation{},
					Payload:                  []byte{},
				},
				History: History{
					TotalVisits:         map[string]int{},
					ConsecutiveFailures: map[string]int{},
					PlaceVisits:         map[string]int{},
					FailureLog:          []Failure{},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cloned := Clone(tc.token)

			assertNilMatches(t, tc.token.Color.PreviousChainingTraceIDs == nil, cloned.Color.PreviousChainingTraceIDs == nil, "previous chaining trace ids")
			assertNilMatches(t, tc.token.Color.Tags == nil, cloned.Color.Tags == nil, "tags")
			assertNilMatches(t, tc.token.Color.Relations == nil, cloned.Color.Relations == nil, "relations")
			assertNilMatches(t, tc.token.Color.Content == nil, cloned.Color.Content == nil, "content")
			assertNilMatches(t, tc.token.Color.Payload == nil, cloned.Color.Payload == nil, "payload")
			assertNilMatches(t, tc.token.History.TotalVisits == nil, cloned.History.TotalVisits == nil, "total visits")
			assertNilMatches(t, tc.token.History.ConsecutiveFailures == nil, cloned.History.ConsecutiveFailures == nil, "consecutive failures")
			assertNilMatches(t, tc.token.History.PlaceVisits == nil, cloned.History.PlaceVisits == nil, "place visits")
			assertNilMatches(t, tc.token.History.FailureLog == nil, cloned.History.FailureLog == nil, "failure log")

			if tc.token.Color.PreviousChainingTraceIDs != nil && len(cloned.Color.PreviousChainingTraceIDs) != 0 {
				t.Fatalf("cloned previous chaining trace ids = %#v, want detached empty slice", cloned.Color.PreviousChainingTraceIDs)
			}
			if tc.token.Color.Tags != nil && len(cloned.Color.Tags) != 0 {
				t.Fatalf("cloned tags = %#v, want detached empty map", cloned.Color.Tags)
			}
			if tc.token.Color.Relations != nil && len(cloned.Color.Relations) != 0 {
				t.Fatalf("cloned relations = %#v, want detached empty slice", cloned.Color.Relations)
			}
			if tc.token.Color.Content != nil && len(cloned.Color.Content) != 0 {
				t.Fatalf("cloned content = %#v, want detached empty slice", cloned.Color.Content)
			}
			if tc.token.Color.Payload != nil && len(cloned.Color.Payload) != 0 {
				t.Fatalf("cloned payload = %#v, want detached empty bytes", cloned.Color.Payload)
			}
			if tc.token.History.TotalVisits != nil && len(cloned.History.TotalVisits) != 0 {
				t.Fatalf("cloned total visits = %#v, want detached empty map", cloned.History.TotalVisits)
			}
			if tc.token.History.ConsecutiveFailures != nil && len(cloned.History.ConsecutiveFailures) != 0 {
				t.Fatalf("cloned consecutive failures = %#v, want detached empty map", cloned.History.ConsecutiveFailures)
			}
			if tc.token.History.PlaceVisits != nil && len(cloned.History.PlaceVisits) != 0 {
				t.Fatalf("cloned place visits = %#v, want detached empty map", cloned.History.PlaceVisits)
			}
			if tc.token.History.FailureLog != nil && len(cloned.History.FailureLog) != 0 {
				t.Fatalf("cloned failure log = %#v, want detached empty slice", cloned.History.FailureLog)
			}
		})
	}
}

func assertNilMatches(t *testing.T, wantNil bool, gotNil bool, field string) {
	t.Helper()
	if wantNil != gotNil {
		t.Fatalf("%s nil state = %t, want %t", field, gotNil, wantNil)
	}
}

func TestClearGuardBlockingFields_PreservesFailureHistory(t *testing.T) {
	t.Parallel()

	history := &History{
		TotalVisits:         map[string]int{"place-1": 1},
		ConsecutiveFailures: map[string]int{"transition-1": 2},
		PlaceVisits:         map[string]int{"place-1": 3},
		LastError:           "boom",
		FailureLog:          []Failure{{TransitionID: "transition-1", Error: "boom", Attempt: 2}},
	}

	ClearGuardBlockingFields(history)

	if len(history.TotalVisits) != 0 || history.TotalVisits == nil {
		t.Fatalf("TotalVisits = %#v, want empty non-nil map", history.TotalVisits)
	}
	if len(history.ConsecutiveFailures) != 0 || history.ConsecutiveFailures == nil {
		t.Fatalf("ConsecutiveFailures = %#v, want empty non-nil map", history.ConsecutiveFailures)
	}
	if len(history.PlaceVisits) != 0 || history.PlaceVisits == nil {
		t.Fatalf("PlaceVisits = %#v, want empty non-nil map", history.PlaceVisits)
	}
	if history.LastError != "boom" {
		t.Fatalf("LastError = %q, want boom", history.LastError)
	}
	if len(history.FailureLog) != 1 || history.FailureLog[0].Error != "boom" {
		t.Fatalf("FailureLog = %#v, want preserved failure history", history.FailureLog)
	}

	ClearGuardBlockingFields(nil)
}
