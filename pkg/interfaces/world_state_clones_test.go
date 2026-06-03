package interfaces

import "testing"

func TestCloneFactoryWorldDispatchCompletion_ResolvesFailureMetadataOnlyInput(t *testing.T) {
	original := FactoryWorldDispatchCompletion{
		DispatchID: "dispatch-1",
		Result: WorkstationResult{
			Outcome: "FAILED",
			FailureMetadata: &WorkFailureMetadata{
				Family: WorkFailureFamilyRetryable,
				Type:   WorkFailureTypeTimeout,
			},
		},
	}

	cloned := CloneFactoryWorldDispatchCompletion(original)
	if cloned.Result.FailureMetadata == nil || cloned.Result.FailureMetadata.Type != WorkFailureTypeTimeout {
		t.Fatalf("cloned failure metadata = %#v, want retryable timeout", cloned.Result.FailureMetadata)
	}
	if cloned.Result.ProviderFailure != nil {
		t.Fatalf("cloned provider failure = %#v, want nil internal field", cloned.Result.ProviderFailure)
	}
	if original.Result.ProviderFailure != nil {
		t.Fatal("original provider failure should remain nil for failure_metadata-only input")
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
			ProviderFailure: &WorkFailureMetadata{
				Family: WorkFailureFamilyRetryable,
				Type:   WorkFailureTypeTimeout,
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
	cloned.Result.FailureMetadata.Family = WorkFailureFamilyTerminal
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

	if original.Result.ProviderFailure.Family != WorkFailureFamilyRetryable {
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
