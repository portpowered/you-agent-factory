package failuremetadatatests

import (
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func TestCloneFactoryWorldDispatchCompletion_ResolvesFailureMetadataOnlyInput(t *testing.T) {
	original := interfaces.FactoryWorldDispatchCompletion{
		DispatchID: "dispatch-1",
		Result: interfaces.WorkstationResult{
			Outcome: "FAILED",
			FailureMetadata: &workerexecution.WorkFailureMetadata{
				Family: workerexecution.WorkFailureFamilyRetryable,
				Type:   workerexecution.WorkFailureTypeTimeout,
			},
		},
	}

	cloned := interfaces.CloneFactoryWorldDispatchCompletion(original)
	if cloned.Result.FailureMetadata == nil || cloned.Result.FailureMetadata.Type != workerexecution.WorkFailureTypeTimeout {
		t.Fatalf("cloned failure metadata = %#v, want retryable timeout", cloned.Result.FailureMetadata)
	}
}

func TestCloneFactoryWorldDispatchCompletion_ClonesCanonicalProviderMetadataAndSafeDiagnostics(t *testing.T) {
	original := testFactoryWorldDispatchCompletion()
	cloned := interfaces.CloneFactoryWorldDispatchCompletion(original)
	mutateClonedDispatchCompletion(&cloned)
	assertOriginalDispatchCompletionUnchanged(t, original)
}

func testFactoryWorldDispatchCompletion() interfaces.FactoryWorldDispatchCompletion {
	return interfaces.FactoryWorldDispatchCompletion{
		DispatchID: "dispatch-1",
		Result: interfaces.WorkstationResult{
			Outcome: "FAILED",
			FailureMetadata: &workerexecution.WorkFailureMetadata{
				Family: workerexecution.WorkFailureFamilyRetryable,
				Type:   workerexecution.WorkFailureTypeTimeout,
			},
		},
		WorkItemIDs: []string{"work-1"},
		ConsumedInputs: []interfaces.WorkstationInput{{
			TokenID: "token-1",
			WorkItem: &work.FactoryWorkItem{
				ID:                       "work-1",
				WorkTypeID:               "task",
				PreviousChainingTraceIDs: []string{"chain-a"},
				Tags:                     map[string]string{"priority": "high"},
			},
		}},
		PreviousChainingTraceIDs: []string{"chain-a", "chain-b"},
		TraceIDs:                 []string{"trace-1"},
		ProviderSession: &workerexecution.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "sess-1",
		},
		Diagnostics: &workerexecution.SafeWorkDiagnostics{
			RenderedPrompt: &workerexecution.SafeRenderedPromptDiagnostic{
				SystemPromptHash: "system-hash",
				Variables:        map[string]string{"prompt_source": "factory-renderer"},
			},
			Provider: &workerexecution.SafeProviderDiagnostic{
				Provider:         "openai",
				Model:            "gpt-5.4",
				RequestMetadata:  map[string]string{"session_id": "sess-1"},
				ResponseMetadata: map[string]string{"retry_count": "0"},
			},
		},
		TerminalWork: &interfaces.FactoryTerminalWork{
			WorkItem: work.FactoryWorkItem{
				ID:                       "work-1",
				WorkTypeID:               "task",
				PreviousChainingTraceIDs: []string{"chain-a"},
				Tags:                     map[string]string{"priority": "high"},
			},
			Status: "FAILED",
		},
	}
}

func mutateClonedDispatchCompletion(cloned *interfaces.FactoryWorldDispatchCompletion) {
	cloned.Result.FailureMetadata.Family = workerexecution.WorkFailureFamilyTerminal
	cloned.ProviderSession.ID = "sess-2"
	cloned.Diagnostics.RenderedPrompt.Variables["prompt_source"] = "mutated"
	cloned.Diagnostics.Provider.RequestMetadata["session_id"] = "sess-2"
	cloned.PreviousChainingTraceIDs[0] = "chain-z"
	cloned.ConsumedInputs[0].WorkItem.PreviousChainingTraceIDs[0] = "chain-z"
	cloned.ConsumedInputs[0].WorkItem.Tags["priority"] = "low"
	cloned.TerminalWork.WorkItem.PreviousChainingTraceIDs[0] = "chain-z"
	cloned.TerminalWork.WorkItem.Tags["priority"] = "terminal-low"
}

func assertOriginalDispatchCompletionUnchanged(t *testing.T, original interfaces.FactoryWorldDispatchCompletion) {
	t.Helper()

	if original.Result.FailureMetadata.Family != workerexecution.WorkFailureFamilyRetryable {
		t.Fatalf("original failure metadata = %#v, want retryable metadata unchanged", original.Result.FailureMetadata)
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
