package interfaces

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func TestCloneProviderMetadata_PreserveNilValuesAndDetachCopies(t *testing.T) {
	if CloneProviderSessionMetadata(nil) != nil {
		t.Fatal("CloneProviderSessionMetadata(nil) = non-nil, want nil")
	}
	if CloneWorkFailureMetadata(nil) != nil {
		t.Fatal("CloneWorkFailureMetadata(nil) = non-nil, want nil")
	}

	session := &ProviderSessionMetadata{Provider: "openai", Kind: "session_id", ID: "sess-1"}
	clonedSession := CloneProviderSessionMetadata(session)
	clonedSession.ID = "sess-2"
	if session.ID != "sess-1" {
		t.Fatalf("original provider session = %#v, want sess-1 unchanged", session)
	}

	failure := &WorkFailureMetadata{Family: WorkFailureFamilyRetryable, Type: WorkFailureTypeTimeout}
	clonedFailure := CloneWorkFailureMetadata(failure)
	clonedFailure.Family = WorkFailureFamilyTerminal
	clonedFailure.Type = WorkFailureTypeInternalServerError
	if failure.Family != WorkFailureFamilyRetryable || failure.Type != WorkFailureTypeTimeout {
		t.Fatalf("original provider failure = %#v, want retryable timeout unchanged", failure)
	}
}

func TestCanonicalProviderSessionProvider(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "empty", input: "", expected: ""},
		{name: "cursor already canonical", input: "cursor", expected: "cursor"},
		{name: "legacy cursor command", input: string(ModelProviderCursor), expected: "cursor"},
		{name: "cursor alias", input: "cursor-agent", expected: "cursor"},
		{name: "other provider unchanged", input: "codex", expected: "codex"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CanonicalProviderSessionProvider(tc.input); got != tc.expected {
				t.Fatalf("CanonicalProviderSessionProvider(%q) = %q, want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestProviderSessionMetadataFromGenerated_CanonicalizesLegacyCursorProvider(t *testing.T) {
	session := ProviderSessionMetadataFromGenerated(&factoryapi.ProviderSessionMetadata{
		Provider: stringPtr("agent"),
		Kind:     stringPtr("session_id"),
		Id:       stringPtr("cursor-session-123"),
	})
	if session == nil {
		t.Fatal("session = nil, want canonical provider session metadata")
	}
	if session.Provider != "cursor" || session.Kind != "session_id" || session.ID != "cursor-session-123" {
		t.Fatalf("session = %#v, want canonical cursor session metadata", session)
	}
}

func TestCloneFactoryWorldProviderSessionRecord_ClonesCanonicalSafeContracts(t *testing.T) {
	original := FactoryWorldProviderSessionRecord{
		DispatchID:      "dispatch-1",
		ProviderSession: ProviderSessionMetadata{Provider: "codex", Kind: "session_id", ID: "sess-1"},
		Diagnostics: &SafeWorkDiagnostics{
			Provider: &SafeProviderDiagnostic{RequestMetadata: map[string]string{"session_id": "sess-1"}},
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
