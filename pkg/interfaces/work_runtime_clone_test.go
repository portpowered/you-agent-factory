package interfaces

import "testing"

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
