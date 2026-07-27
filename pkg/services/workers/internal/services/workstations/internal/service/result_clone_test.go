package service

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
)

func TestPoolDispatchTerminalResultDetachedFromExecutorAndCaller(t *testing.T) {
	t.Parallel()

	const (
		dispatchID   = "dispatch-detached-result"
		transitionID = "transition-detached-result"
	)
	executor := &recordingExecutor{result: mutableWorkResultFixture()}
	pool := New()
	startPool(t, pool, workstations.Route{
		WorkstationName: "review",
		Executor:        executor,
	})

	first, err := pool.Dispatch(
		context.Background(),
		dispatchRequest(dispatchID, transitionID, "review"),
	)
	if err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}

	mutateWorkResult(&executor.result, "executor-mutated")
	assertMutableWorkResultFixture(t, first.Result, dispatchID, transitionID)

	mutateWorkResult(&first.Result, "caller-mutated")
	record := pool.dispatches[dispatchID]
	late, err := record.commitCancellation(context.Canceled)
	if err != nil {
		t.Fatalf("late commitCancellation() error = %v", err)
	}
	assertMutableWorkResultFixture(t, late.Result, dispatchID, transitionID)
}

func mutableWorkResultFixture() workers.WorkResult {
	return workers.WorkResult{
		Outcome: workers.OutcomeAccepted,
		RecordedOutputWork: []work.FactoryWorkItem{{
			ID:                       "work-result",
			PreviousChainingTraceIDs: []string{"trace-parent"},
			Content: []work.WorkContentPart{{
				Type: work.WorkContentPartTypeJSON,
				JSON: json.RawMessage(`{"answer":42}`),
				Metadata: map[string]any{
					"nested":     map[string]any{"status": "original"},
					"steps":      []any{"first", map[string]any{"name": "second"}},
					"labels":     map[string]string{"owner": "original"},
					"categories": []string{"original"},
					"raw":        json.RawMessage(`{"status":"original"}`),
					"bytes":      []byte("original"),
				},
			}},
			Tags: map[string]string{"owner": "original"},
		}},
		FailureMetadata: &workers.WorkFailureMetadata{
			Family: workers.WorkFailureFamilyRetryable,
			Type:   workers.WorkFailureTypeTimeout,
		},
		ProviderSession: &workers.ProviderSessionMetadata{
			Provider: "codex",
			Kind:     "session_id",
			ID:       "session-original",
		},
		Diagnostics: &workers.WorkDiagnostics{
			RenderedPrompt: &workers.RenderedPromptDiagnostic{
				Variables: map[string]string{"prompt": "original"},
			},
			Provider: &workers.ProviderDiagnostic{
				RequestMetadata:  map[string]string{"request": "original"},
				ResponseMetadata: map[string]string{"response": "original"},
			},
			Invocation: &workers.InvocationDiagnostic{
				Parameters: []workers.InvocationParameterDiagnostic{{
					Name:        "input",
					SourceKinds: []string{"original"},
				}},
			},
			Command: &workers.CommandDiagnostic{
				Args: []string{"original"},
				Env:  map[string]string{"KEY": "original"},
			},
			Metadata: map[string]string{"diagnostic": "original"},
		},
	}
}

func mutateWorkResult(result *workers.WorkResult, value string) {
	item := &result.RecordedOutputWork[0]
	item.PreviousChainingTraceIDs[0] = value
	item.Content[0].JSON[0] = '!'
	item.Content[0].Metadata["nested"].(map[string]any)["status"] = value
	item.Content[0].Metadata["steps"].([]any)[1].(map[string]any)["name"] = value
	item.Content[0].Metadata["labels"].(map[string]string)["owner"] = value
	item.Content[0].Metadata["categories"].([]string)[0] = value
	item.Content[0].Metadata["raw"].(json.RawMessage)[0] = '!'
	item.Content[0].Metadata["bytes"].([]byte)[0] = '!'
	item.Tags["owner"] = value
	result.FailureMetadata.Type = workers.WorkFailureTypeAuthFailure
	result.ProviderSession.ID = value
	result.Diagnostics.RenderedPrompt.Variables["prompt"] = value
	result.Diagnostics.Provider.RequestMetadata["request"] = value
	result.Diagnostics.Provider.ResponseMetadata["response"] = value
	result.Diagnostics.Invocation.Parameters[0].SourceKinds[0] = value
	result.Diagnostics.Command.Args[0] = value
	result.Diagnostics.Command.Env["KEY"] = value
	result.Diagnostics.Metadata["diagnostic"] = value
}

func assertMutableWorkResultFixture(
	t *testing.T,
	result workers.WorkResult,
	dispatchID string,
	transitionID string,
) {
	t.Helper()
	want := mutableWorkResultFixture()
	want.DispatchID = dispatchID
	want.TransitionID = transitionID
	if !reflect.DeepEqual(result, want) {
		t.Fatalf("mutable result = %#v, want detached %#v", result, want)
	}
}
