package interfaces

import (
	"testing"
	"time"
)

func TestCloneToken_DetachesNestedMutableRuntimeFields(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	original := Token{
		ID:      "token-1",
		PlaceID: "place-1",
		Color: TokenColor{
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
			Relations: []Relation{{
				Type:         RelationDependsOn,
				TargetWorkID: "work-upstream",
			}},
			Payload: []byte("payload"),
		},
		CreatedAt: now,
		EnteredAt: now.Add(time.Minute),
		History: TokenHistory{
			TotalVisits:         map[string]int{"queued": 1},
			ConsecutiveFailures: map[string]int{"queued": 2},
			PlaceVisits:         map[string]int{"queued": 3},
			TotalDuration:       5 * time.Minute,
			LastError:           "timeout",
			FailureLog: []FailureRecord{{
				TransitionID: "transition-1",
				Timestamp:    now,
				Error:        "timeout",
				Attempt:      2,
			}},
		},
	}

	cloned := CloneToken(original)
	cloned.Color.PreviousChainingTraceIDs[0] = "trace-z"
	cloned.Color.Tags["priority"] = "low"
	cloned.Color.Relations[0].TargetWorkID = "work-mutated"
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
				Color:   TokenColor{},
				History: TokenHistory{},
			},
		},
		{
			name: "empty but allocated slices and maps become detached empty values",
			token: Token{
				Color: TokenColor{
					PreviousChainingTraceIDs: []string{},
					Tags:                     map[string]string{},
					Relations:                []Relation{},
					Payload:                  []byte{},
				},
				History: TokenHistory{
					TotalVisits:         map[string]int{},
					ConsecutiveFailures: map[string]int{},
					PlaceVisits:         map[string]int{},
					FailureLog:          []FailureRecord{},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cloned := CloneToken(tc.token)

			assertNilMatches(t, tc.token.Color.PreviousChainingTraceIDs == nil, cloned.Color.PreviousChainingTraceIDs == nil, "previous chaining trace ids")
			assertNilMatches(t, tc.token.Color.Tags == nil, cloned.Color.Tags == nil, "tags")
			assertNilMatches(t, tc.token.Color.Relations == nil, cloned.Color.Relations == nil, "relations")
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

func TestCloneProviderMetadata_PreserveNilValuesAndDetachCopies(t *testing.T) {
	if CloneProviderSessionMetadata(nil) != nil {
		t.Fatal("CloneProviderSessionMetadata(nil) = non-nil, want nil")
	}
	if CloneProviderFailureMetadata(nil) != nil {
		t.Fatal("CloneProviderFailureMetadata(nil) = non-nil, want nil")
	}

	session := &ProviderSessionMetadata{
		Provider: "openai",
		Kind:     "session_id",
		ID:       "sess-1",
	}
	clonedSession := CloneProviderSessionMetadata(session)
	clonedSession.ID = "sess-2"
	if session.ID != "sess-1" {
		t.Fatalf("original provider session = %#v, want sess-1 unchanged", session)
	}

	failure := &ProviderFailureMetadata{
		Family: ProviderErrorFamilyRetryable,
		Type:   ProviderErrorTypeTimeout,
	}
	clonedFailure := CloneProviderFailureMetadata(failure)
	clonedFailure.Family = ProviderErrorFamilyTerminal
	clonedFailure.Type = ProviderErrorTypeInternalServerError
	if failure.Family != ProviderErrorFamilyRetryable || failure.Type != ProviderErrorTypeTimeout {
		t.Fatalf("original provider failure = %#v, want retryable timeout unchanged", failure)
	}
}

func assertNilMatches(t *testing.T, wantNil bool, gotNil bool, field string) {
	t.Helper()
	if wantNil != gotNil {
		t.Fatalf("%s nil state = %t, want %t", field, gotNil, wantNil)
	}
}

func TestCloneFactoryWorldDispatchCompletion_ClonesCanonicalProviderMetadataAndSafeDiagnostics(t *testing.T) {
	original := testFactoryWorldDispatchCompletion()
	cloned := CloneFactoryWorldDispatchCompletion(original)
	mutateClonedDispatchCompletion(cloned)
	assertOriginalDispatchCompletionUnchanged(t, original)
}

func testFactoryWorldDispatchCompletion() FactoryWorldDispatchCompletion {
	return FactoryWorldDispatchCompletion{
		DispatchID: "dispatch-1",
		Result: WorkstationResult{
			Outcome: "FAILED",
			ProviderFailure: &ProviderFailureMetadata{
				Family: ProviderErrorFamilyRetryable,
				Type:   ProviderErrorTypeTimeout,
			},
		},
		WorkItemIDs: []string{"work-1"},
		ConsumedInputs: []WorkstationInput{{
			TokenID: "token-1",
			WorkItem: &FactoryWorkItem{
				ID:                       "work-1",
				WorkTypeID:               "task",
				PreviousChainingTraceIDs: []string{"chain-a"},
				Tags:                     map[string]string{"priority": "high"},
			},
		}},
		PreviousChainingTraceIDs: []string{"chain-a", "chain-b"},
		TraceIDs:                 []string{"trace-1"},
		ProviderSession: &ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-1",
		},
		Diagnostics: &SafeWorkDiagnostics{
			RenderedPrompt: &SafeRenderedPromptDiagnostic{
				SystemPromptHash: "system-hash",
				Variables:        map[string]string{"prompt_source": "factory-renderer"},
			},
			Provider: &SafeProviderDiagnostic{
				Provider:         "openai",
				Model:            "gpt-5.4",
				RequestMetadata:  map[string]string{"session_id": "sess-1"},
				ResponseMetadata: map[string]string{"retry_count": "0"},
			},
		},
		TerminalWork: &FactoryTerminalWork{
			WorkItem: FactoryWorkItem{
				ID:                       "work-1",
				WorkTypeID:               "task",
				PreviousChainingTraceIDs: []string{"chain-a"},
				Tags:                     map[string]string{"priority": "high"},
			},
			Status: "FAILED",
		},
	}
}

func mutateClonedDispatchCompletion(cloned FactoryWorldDispatchCompletion) {
	cloned.Result.ProviderFailure.Family = ProviderErrorFamilyTerminal
	cloned.ProviderSession.ID = "sess-2"
	cloned.Diagnostics.RenderedPrompt.Variables["prompt_source"] = "mutated"
	cloned.Diagnostics.Provider.RequestMetadata["session_id"] = "sess-2"
	cloned.PreviousChainingTraceIDs[0] = "chain-z"
	cloned.ConsumedInputs[0].WorkItem.PreviousChainingTraceIDs[0] = "chain-z"
	cloned.ConsumedInputs[0].WorkItem.Tags["priority"] = "low"
	cloned.TerminalWork.WorkItem.PreviousChainingTraceIDs[0] = "chain-z"
	cloned.TerminalWork.WorkItem.Tags["priority"] = "terminal-low"
}

func assertOriginalDispatchCompletionUnchanged(t *testing.T, original FactoryWorldDispatchCompletion) {
	t.Helper()

	if original.Result.ProviderFailure.Family != ProviderErrorFamilyRetryable {
		t.Fatalf("original provider failure = %#v, want retryable metadata unchanged", original.Result.ProviderFailure)
	}
	if original.ProviderSession.ID != "sess-1" {
		t.Fatalf("original provider session = %#v, want sess-1 unchanged", original.ProviderSession)
	}
	if original.Diagnostics.RenderedPrompt.Variables["prompt_source"] != "factory-renderer" {
		t.Fatalf("original rendered prompt = %#v, want prompt_source unchanged", original.Diagnostics.RenderedPrompt)
	}
	if original.Diagnostics.Provider.RequestMetadata["session_id"] != "sess-1" {
		t.Fatalf("original request metadata = %#v, want session_id unchanged", original.Diagnostics.Provider.RequestMetadata)
	}
	if original.PreviousChainingTraceIDs[0] != "chain-a" {
		t.Fatalf("original previous chaining trace IDs = %#v, want chain-a unchanged", original.PreviousChainingTraceIDs)
	}
	if original.ConsumedInputs[0].WorkItem.PreviousChainingTraceIDs[0] != "chain-a" {
		t.Fatalf("original consumed input previous chaining trace IDs = %#v, want chain-a unchanged", original.ConsumedInputs[0].WorkItem.PreviousChainingTraceIDs)
	}
	if original.ConsumedInputs[0].WorkItem.Tags["priority"] != "high" {
		t.Fatalf("original consumed input tags = %#v, want high unchanged", original.ConsumedInputs[0].WorkItem.Tags)
	}
	if original.TerminalWork.WorkItem.PreviousChainingTraceIDs[0] != "chain-a" {
		t.Fatalf("original terminal work previous chaining trace IDs = %#v, want chain-a unchanged", original.TerminalWork.WorkItem.PreviousChainingTraceIDs)
	}
	if original.TerminalWork.WorkItem.Tags["priority"] != "high" {
		t.Fatalf("original terminal work tags = %#v, want high unchanged", original.TerminalWork.WorkItem.Tags)
	}
}

func TestCloneFactoryWorldProviderSessionRecord_ClonesCanonicalSafeContracts(t *testing.T) {
	original := FactoryWorldProviderSessionRecord{
		DispatchID: "dispatch-1",
		ProviderSession: ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-1",
		},
		Diagnostics: &SafeWorkDiagnostics{
			Provider: &SafeProviderDiagnostic{
				RequestMetadata: map[string]string{"session_id": "sess-1"},
			},
		},
		ConsumedInputs: []WorkstationInput{{
			TokenID: "token-1",
			WorkItem: &FactoryWorkItem{
				ID:                       "work-1",
				WorkTypeID:               "task",
				PreviousChainingTraceIDs: []string{"chain-a"},
				Tags:                     map[string]string{"priority": "high"},
			},
		}},
		PreviousChainingTraceIDs: []string{"chain-a", "chain-b"},
		TraceIDs:                 []string{"trace-1"},
	}

	cloned := CloneFactoryWorldProviderSessionRecord(original)

	cloned.ProviderSession.ID = "sess-2"
	cloned.Diagnostics.Provider.RequestMetadata["session_id"] = "sess-2"
	cloned.PreviousChainingTraceIDs[0] = "chain-z"
	cloned.ConsumedInputs[0].WorkItem.PreviousChainingTraceIDs[0] = "chain-z"
	cloned.ConsumedInputs[0].WorkItem.Tags["priority"] = "low"
	cloned.TraceIDs[0] = "trace-2"

	if original.ProviderSession.ID != "sess-1" {
		t.Fatalf("original provider session = %#v, want sess-1 unchanged", original.ProviderSession)
	}
	if original.Diagnostics.Provider.RequestMetadata["session_id"] != "sess-1" {
		t.Fatalf("original diagnostics = %#v, want session_id unchanged", original.Diagnostics)
	}
	if original.PreviousChainingTraceIDs[0] != "chain-a" {
		t.Fatalf("original previous chaining trace IDs = %#v, want chain-a unchanged", original.PreviousChainingTraceIDs)
	}
	if original.ConsumedInputs[0].WorkItem.PreviousChainingTraceIDs[0] != "chain-a" {
		t.Fatalf("original consumed input previous chaining trace IDs = %#v, want chain-a unchanged", original.ConsumedInputs[0].WorkItem.PreviousChainingTraceIDs)
	}
	if original.ConsumedInputs[0].WorkItem.Tags["priority"] != "high" {
		t.Fatalf("original consumed input tags = %#v, want high unchanged", original.ConsumedInputs[0].WorkItem.Tags)
	}
	if original.TraceIDs[0] != "trace-1" {
		t.Fatalf("original trace IDs = %#v, want trace-1 unchanged", original.TraceIDs)
	}
}
