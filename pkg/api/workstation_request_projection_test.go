package api

import (
	"encoding/json"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	scriptProjectionActiveDispatchID      = "dispatch-script-active"
	scriptProjectionActiveRequestID       = "dispatch-script-active/script-request/1"
	scriptProjectionCompletedDispatchID   = "dispatch-script-completed"
	scriptProjectionCompletedRequestID    = "dispatch-script-completed/script-request/1"
	scriptProjectionCommand               = "script-tool"
	scriptProjectionActiveOutcome         = "SUCCEEDED"
	scriptProjectionCompletedOutcome      = "FAILED_EXIT_CODE"
	scriptProjectionCompletedStdout       = "script stdout\n"
	scriptProjectionCompletedFailurePlace = "task:failed"
)

// portos:func-length-exception owner=agent-factory reason=projection-regression-fixture review=2026-07-22 removal=split-active-and-completed-workstation-request-cases-before-next-api-projection-change
func TestBuildFactoryWorldWorkstationRequestProjectionSlice_ProjectsDispatchKeyedGeneratedContractFromCanonicalWorldState(t *testing.T) {
	t0 := time.Date(2026, 4, 21, 12, 0, 0, 0, time.UTC)
	activeWork := interfaces.FactoryWorkItem{
		ID:                     "work-active",
		WorkTypeID:             "task",
		DisplayName:            "Active story",
		CurrentChainingTraceID: "chain-active",
		TraceID:                "chain-active",
		PlaceID:                "task:init",
	}
	completedInput := interfaces.FactoryWorkItem{
		ID:                     "work-completed-input",
		WorkTypeID:             "task",
		DisplayName:            "Completed story",
		CurrentChainingTraceID: "chain-parent-a",
		TraceID:                "chain-parent-a",
		PlaceID:                "task:init",
	}
	completedOutput := interfaces.FactoryWorkItem{
		ID:                       "work-completed-output",
		WorkTypeID:               "task",
		DisplayName:              "Completed story",
		CurrentChainingTraceID:   "chain-completed",
		PreviousChainingTraceIDs: []string{"chain-parent-a", "chain-parent-z"},
		TraceID:                  "chain-completed",
		PlaceID:                  "task:done",
	}

	state := interfaces.FactoryWorldState{
		WorkItemsByID: map[string]interfaces.FactoryWorkItem{
			activeWork.ID:      activeWork,
			completedInput.ID:  completedInput,
			completedOutput.ID: completedOutput,
		},
		ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
			"dispatch-active": {
				DispatchID:   "dispatch-active",
				TransitionID: "review",
				Workstation:  interfaces.FactoryWorkstationRef{ID: "review", Name: "Review"},
				Provider:     "codex",
				Model:        "gpt-5.4",
				StartedAt:    t0.Add(time.Second),
				Inputs: []interfaces.WorkstationInput{{
					TokenID:  "token-active",
					PlaceID:  activeWork.PlaceID,
					WorkItem: &activeWork,
				}},
				WorkItemIDs:              []string{activeWork.ID},
				CurrentChainingTraceID:   "chain-active",
				PreviousChainingTraceIDs: []string{"chain-active"},
				TraceIDs:                 []string{activeWork.TraceID},
			},
		},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID:   "dispatch-completed",
			TransitionID: "review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "review", Name: "Review"},
			StartedAt:    t0.Add(2 * time.Second),
			CompletedAt:  t0.Add(4 * time.Second),
			Result: interfaces.WorkstationResult{
				Outcome:  string(interfaces.OutcomeAccepted),
				Feedback: "ready",
				Output:   "fallback output",
			},
			DurationMillis: 1200,
			WorkItemIDs:    []string{completedInput.ID},
			ConsumedInputs: []interfaces.WorkstationInput{{
				TokenID:  "token-completed",
				PlaceID:  completedInput.PlaceID,
				WorkItem: &completedInput,
			}},
			InputWorkItems:           []interfaces.FactoryWorkItem{completedInput},
			OutputWorkItems:          []interfaces.FactoryWorkItem{completedOutput},
			CurrentChainingTraceID:   "chain-parent-a",
			PreviousChainingTraceIDs: []string{"chain-parent-a", "chain-parent-z"},
			TraceIDs:                 []string{completedInput.TraceID},
			ProviderSession:          &interfaces.ProviderSessionMetadata{Provider: "openai", Kind: "session_id", ID: "session-1"},
			Diagnostics: &interfaces.SafeWorkDiagnostics{
				Provider: &interfaces.SafeProviderDiagnostic{
					Provider:         "openai",
					Model:            "gpt-5.4",
					RequestMetadata:  map[string]string{"prompt_source": "factory-renderer"},
					ResponseMetadata: map[string]string{"provider_session_id": "session-1", "retry_count": "0"},
				},
			},
			TerminalWork: &interfaces.FactoryTerminalWork{
				WorkItem: completedOutput,
				Status:   "TERMINAL",
			},
		}},
		InferenceAttemptsByDispatchID: map[string]map[string]interfaces.FactoryWorldInferenceAttempt{
			"dispatch-completed": {
				"dispatch-completed/inference/1": {
					DispatchID:         "dispatch-completed",
					TransitionID:       "review",
					InferenceRequestID: "dispatch-completed/inference/1",
					Attempt:            1,
					Prompt:             "Review the completed story.",
					WorkingDirectory:   "/workspace/completed",
					Worktree:           "/workspace/completed/.worktree",
					RequestTime:        t0.Add(3 * time.Second),
					Response:           "Approved",
					ResponseTime:       t0.Add(4 * time.Second),
				},
			},
		},
	}

	slice := BuildFactoryWorldWorkstationRequestProjectionSlice(state)
	if slice.WorkstationRequestsByDispatchId == nil {
		t.Fatal("workstation request slice missing generated projection map")
	}
	requests := *slice.WorkstationRequestsByDispatchId
	if len(requests) != 2 {
		t.Fatalf("workstation request count = %d, want 2", len(requests))
	}
	if requests["dispatch-active"].DispatchId != "dispatch-active" {
		t.Fatalf("active dispatch id = %q, want dispatch-active", requests["dispatch-active"].DispatchId)
	}
	if requests["dispatch-completed"].DispatchId != "dispatch-completed" {
		t.Fatalf("completed dispatch id = %q, want dispatch-completed", requests["dispatch-completed"].DispatchId)
	}

	active := requests["dispatch-active"]
	if active.Request.Provider != nil || active.Request.Model != nil {
		t.Fatalf("active inference summary provider/model = (%#v, %#v), want omitted dispatch-level inference detail", active.Request.Provider, active.Request.Model)
	}
	if active.Request.ConsumedTokens == nil || len(*active.Request.ConsumedTokens) != 1 || (*active.Request.ConsumedTokens)[0].TokenId != "token-active" {
		t.Fatalf("active consumed tokens = %#v, want token-active", active.Request.ConsumedTokens)
	}
	if active.Request.CurrentChainingTraceId == nil || *active.Request.CurrentChainingTraceId != "chain-active" {
		t.Fatalf("active current chaining trace ID = %#v, want chain-active", active.Request.CurrentChainingTraceId)
	}
	if active.Response != nil {
		t.Fatalf("active request response = %#v, want nil", active.Response)
	}

	completed := requests["dispatch-completed"]
	if completed.Request.RequestTime != nil ||
		completed.Request.Prompt != nil ||
		completed.Request.WorkingDirectory != nil ||
		completed.Request.Worktree != nil ||
		completed.Request.Provider != nil ||
		completed.Request.Model != nil ||
		completed.Request.RequestMetadata != nil {
		t.Fatalf("completed request inference summary = %#v, want omitted dispatch-level inference detail", completed.Request)
	}
	if completed.Response == nil || completed.Response.Outcome == nil || *completed.Response.Outcome != "ACCEPTED" {
		t.Fatalf("completed response = %#v, want accepted outcome", completed.Response)
	}
	if completed.Response.OutputMutations == nil ||
		len(*completed.Response.OutputMutations) != 1 ||
		(*completed.Response.OutputMutations)[0].TokenId != "work-completed-output" ||
		(*completed.Response.OutputMutations)[0].Type != string(interfaces.MutationCreate) {
		t.Fatalf("completed output mutations = %#v, want create mutation for work-completed-output", completed.Response.OutputMutations)
	}
	if completed.Response.ResponseText != nil ||
		completed.Response.ErrorClass != nil ||
		completed.Response.ProviderSession != nil ||
		completed.Response.Diagnostics != nil ||
		completed.Response.ResponseMetadata != nil {
		t.Fatalf("completed response inference summary = %#v, want omitted dispatch-level inference detail", completed.Response)
	}
	if completed.Response.OutputWorkItems == nil || len(*completed.Response.OutputWorkItems) != 1 {
		t.Fatalf("completed output work items = %#v, want one output", completed.Response.OutputWorkItems)
	}
	if completed.Request.PreviousChainingTraceIds == nil || len(*completed.Request.PreviousChainingTraceIds) != 2 {
		t.Fatalf("completed previous chaining trace IDs = %#v, want two predecessor chains", completed.Request.PreviousChainingTraceIds)
	}
	if outputItems := *completed.Response.OutputWorkItems; outputItems[0].PreviousChainingTraceIds == nil || len(*outputItems[0].PreviousChainingTraceIds) != 2 {
		t.Fatalf("completed output work item chaining lineage = %#v, want explicit previous chaining trace IDs", outputItems)
	}

	encoded, err := json.Marshal(slice)
	if err != nil {
		t.Fatalf("Marshal(slice): %v", err)
	}

	var roundTripped factoryapi.FactoryWorldWorkstationRequestProjectionSlice
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("Unmarshal(roundTripped): %v", err)
	}
	if roundTripped.WorkstationRequestsByDispatchId == nil {
		t.Fatal("round-tripped projection slice missing request map")
	}
	if got := (*roundTripped.WorkstationRequestsByDispatchId)["dispatch-completed"].Response; got == nil || got.ProviderSession != nil || got.ResponseText != nil || got.Diagnostics != nil {
		t.Fatalf("round-tripped completed response = %#v, want omitted dispatch-level inference detail", got)
	}

	state.ActiveDispatches["dispatch-active"].Inputs[0].WorkItem.DisplayName = "mutated active"
	state.CompletedDispatches[0].OutputWorkItems[0].DisplayName = "mutated output"
	if got := (*roundTripped.WorkstationRequestsByDispatchId)["dispatch-active"].Request.ConsumedTokens; got == nil || (*got)[0].Name == nil || *(*got)[0].Name != "Active story" {
		t.Fatalf("round-tripped active consumed token = %#v, want detached Active story", got)
	}
	if got := (*roundTripped.WorkstationRequestsByDispatchId)["dispatch-completed"].Response.OutputMutations; got == nil || (*got)[0].Token == nil || (*got)[0].Token.Name == nil || *(*got)[0].Token.Name != "Completed story" {
		t.Fatalf("round-tripped completed mutation token = %#v, want detached Completed story", got)
	}
}

func TestBuildFactoryWorldWorkstationRequestProjectionSlice_UsesTerminalWorkFallbackLineage(t *testing.T) {
	completedInput := interfaces.FactoryWorkItem{
		ID:                     "work-completed-input",
		WorkTypeID:             "task",
		DisplayName:            "Completed story",
		CurrentChainingTraceID: "chain-parent-a",
		TraceID:                "chain-parent-a",
		PlaceID:                "task:init",
	}
	terminalOutput := interfaces.FactoryWorkItem{
		ID:                       "work-terminal-output",
		WorkTypeID:               "task",
		DisplayName:              "Terminal story",
		CurrentChainingTraceID:   "chain-terminal",
		PreviousChainingTraceIDs: []string{"chain-parent-a", "chain-parent-z"},
		TraceID:                  "chain-terminal",
		PlaceID:                  "task:done",
	}

	slice := BuildFactoryWorldWorkstationRequestProjectionSlice(interfaces.FactoryWorldState{
		WorkItemsByID: map[string]interfaces.FactoryWorkItem{
			completedInput.ID: completedInput,
		},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID:   "dispatch-completed",
			TransitionID: "review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "review", Name: "Review"},
			StartedAt:    time.Date(2026, 4, 22, 19, 0, 0, 0, time.UTC),
			CompletedAt:  time.Date(2026, 4, 22, 19, 0, 1, 0, time.UTC),
			Result:       interfaces.WorkstationResult{Outcome: string(interfaces.OutcomeAccepted)},
			WorkItemIDs:  []string{completedInput.ID},
			ConsumedInputs: []interfaces.WorkstationInput{{
				TokenID:  "token-completed",
				PlaceID:  completedInput.PlaceID,
				WorkItem: &completedInput,
			}},
			InputWorkItems: completedInputSlice(completedInput),
			TerminalWork: &interfaces.FactoryTerminalWork{
				WorkItem: terminalOutput,
				Status:   "TERMINAL",
			},
		}},
	})

	if slice.WorkstationRequestsByDispatchId == nil {
		t.Fatal("workstation request slice missing generated projection map")
	}
	response := (*slice.WorkstationRequestsByDispatchId)["dispatch-completed"].Response
	if response == nil || response.OutputWorkItems == nil || len(*response.OutputWorkItems) != 1 {
		t.Fatalf("terminal fallback output work items = %#v, want one output item", response)
	}
	output := (*response.OutputWorkItems)[0]
	if output.CurrentChainingTraceId == nil || *output.CurrentChainingTraceId != "chain-terminal" {
		t.Fatalf("terminal fallback current chaining trace ID = %#v, want chain-terminal", output.CurrentChainingTraceId)
	}
	if output.PreviousChainingTraceIds == nil || len(*output.PreviousChainingTraceIds) != 2 || (*output.PreviousChainingTraceIds)[0] != "chain-parent-a" || (*output.PreviousChainingTraceIds)[1] != "chain-parent-z" {
		t.Fatalf("terminal fallback previous chaining trace IDs = %#v, want [chain-parent-a chain-parent-z]", output.PreviousChainingTraceIds)
	}
}

func TestWorkstationDispatchViewFromCompletion_OmitsInferenceOwnedSummaryFields(t *testing.T) {
	completion := interfaces.FactoryWorldDispatchCompletion{
		DispatchID:   "dispatch-completed",
		TransitionID: "review",
		Workstation:  interfaces.FactoryWorkstationRef{ID: "review", Name: "Review"},
		StartedAt:    time.Date(2026, 4, 22, 19, 0, 0, 0, time.UTC),
		CompletedAt:  time.Date(2026, 4, 22, 19, 0, 1, 0, time.UTC),
		Result:       interfaces.WorkstationResult{Outcome: string(interfaces.OutcomeAccepted)},
		ProviderSession: &interfaces.ProviderSessionMetadata{
			Provider: "openai",
			Kind:     "session_id",
			ID:       "session-fallback",
		},
		Diagnostics: &interfaces.SafeWorkDiagnostics{
			Provider: &interfaces.SafeProviderDiagnostic{
				Provider:         "openai",
				Model:            "gpt-5.4",
				RequestMetadata:  map[string]string{"working_directory": "/fallback/workdir"},
				ResponseMetadata: map[string]string{"provider_session_id": "session-fallback"},
			},
		},
	}

	view := workstationDispatchViewFromCompletion(completion, interfaces.FactoryWorldState{}, nil, nil)
	if view.Request.Provider != nil ||
		view.Request.Model != nil ||
		view.Request.RequestTime != nil ||
		view.Request.WorkingDirectory != nil ||
		view.Request.Worktree != nil ||
		view.Request.RequestMetadata != nil ||
		view.Request.Prompt != nil {
		t.Fatalf("completion request inference summary = %#v, want omitted dispatch-level inference detail", view.Request)
	}
	if view.Response == nil {
		t.Fatal("completion response = nil, want dispatch status summary")
	}
	if view.Response.ProviderSession != nil ||
		view.Response.Diagnostics != nil ||
		view.Response.ResponseMetadata != nil ||
		view.Response.ResponseText != nil ||
		view.Response.ErrorClass != nil {
		t.Fatalf("completion response inference summary = %#v, want omitted dispatch-level inference detail", view.Response)
	}
}

func TestBuildFactoryWorldWorkstationRequestProjectionSlice_PreservesScriptBackedDispatchDetails(t *testing.T) {
	t0 := time.Date(2026, 4, 23, 9, 0, 0, 0, time.UTC)
	workItem := interfaces.FactoryWorkItem{
		ID:          "work-scripted",
		WorkTypeID:  "task",
		DisplayName: "Scripted story",
		TraceID:     "trace-scripted",
		PlaceID:     "task:init",
	}
	exitCode := 124

	slice := BuildFactoryWorldWorkstationRequestProjectionSlice(interfaces.FactoryWorldState{
		WorkItemsByID: map[string]interfaces.FactoryWorkItem{
			workItem.ID: workItem,
		},
		ActiveDispatches: map[string]interfaces.FactoryWorldDispatch{
			scriptProjectionActiveDispatchID: {
				DispatchID:   scriptProjectionActiveDispatchID,
				TransitionID: "script-review",
				Workstation:  interfaces.FactoryWorkstationRef{ID: "script-review", Name: "Script Review"},
				StartedAt:    t0,
				Inputs: []interfaces.WorkstationInput{{
					TokenID:  "token-script-active",
					PlaceID:  workItem.PlaceID,
					WorkItem: &workItem,
				}},
				WorkItemIDs: []string{workItem.ID},
				TraceIDs:    []string{workItem.TraceID},
			},
		},
		CompletedDispatches: []interfaces.FactoryWorldDispatchCompletion{{
			DispatchID:   scriptProjectionCompletedDispatchID,
			TransitionID: "script-review",
			Workstation:  interfaces.FactoryWorkstationRef{ID: "script-review", Name: "Script Review"},
			StartedAt:    t0.Add(time.Minute),
			CompletedAt:  t0.Add(2 * time.Minute),
			Result: interfaces.WorkstationResult{
				Outcome:        string(interfaces.OutcomeRejected),
				FailureReason:  "script failed",
				FailureMessage: "script timed out",
			},
			DurationMillis: 12_000,
			WorkItemIDs:    []string{workItem.ID},
			ConsumedInputs: []interfaces.WorkstationInput{{
				TokenID:  "token-script-completed",
				PlaceID:  workItem.PlaceID,
				WorkItem: &workItem,
			}},
			InputWorkItems: []interfaces.FactoryWorkItem{workItem},
			TraceIDs:       []string{workItem.TraceID},
		}},
		ScriptRequestsByDispatchID: map[string]map[string]interfaces.FactoryWorldScriptRequest{
			scriptProjectionActiveDispatchID: {
				scriptProjectionActiveRequestID: {
					DispatchID:      scriptProjectionActiveDispatchID,
					TransitionID:    "script-review",
					ScriptRequestID: scriptProjectionActiveRequestID,
					Attempt:         1,
					Command:         scriptProjectionCommand,
					Args:            []string{"--mode", "active"},
					RequestTime:     t0.Add(5 * time.Second),
				},
			},
			scriptProjectionCompletedDispatchID: {
				scriptProjectionCompletedRequestID: {
					DispatchID:      scriptProjectionCompletedDispatchID,
					TransitionID:    "script-review",
					ScriptRequestID: scriptProjectionCompletedRequestID,
					Attempt:         2,
					Command:         scriptProjectionCommand,
					Args:            []string{"--mode", "completed"},
					RequestTime:     t0.Add(time.Minute + 5*time.Second),
				},
			},
		},
		ScriptResponsesByDispatchID: map[string]map[string]interfaces.FactoryWorldScriptResponse{
			scriptProjectionCompletedDispatchID: {
				scriptProjectionCompletedRequestID: {
					DispatchID:      scriptProjectionCompletedDispatchID,
					TransitionID:    "script-review",
					ScriptRequestID: scriptProjectionCompletedRequestID,
					Attempt:         2,
					Outcome:         scriptProjectionCompletedOutcome,
					Stdout:          scriptProjectionCompletedStdout,
					Stderr:          "script stderr\n",
					DurationMillis:  12_000,
					ExitCode:        &exitCode,
					FailureType:     "TIMEOUT",
					ResponseTime:    t0.Add(2*time.Minute - time.Second),
				},
			},
		},
	})

	if slice.WorkstationRequestsByDispatchId == nil {
		t.Fatal("workstation request slice missing generated projection map")
	}
	requests := *slice.WorkstationRequestsByDispatchId

	active := requests[scriptProjectionActiveDispatchID]
	if active.Request.ScriptRequest == nil {
		t.Fatalf("active script request = %#v, want projected script request", active.Request)
	}
	if active.Request.ScriptRequest.Command == nil || *active.Request.ScriptRequest.Command != scriptProjectionCommand {
		t.Fatalf("active script request command = %#v, want %q", active.Request.ScriptRequest.Command, scriptProjectionCommand)
	}
	if active.Request.ScriptRequest.Args == nil || len(*active.Request.ScriptRequest.Args) != 2 || (*active.Request.ScriptRequest.Args)[1] != "active" {
		t.Fatalf("active script request args = %#v, want [--mode active]", active.Request.ScriptRequest.Args)
	}
	if active.Request.Prompt != nil || active.Request.Provider != nil || active.Request.Model != nil {
		t.Fatalf("active request inference summary = %#v, want only script-backed detail", active.Request)
	}
	if active.Response != nil {
		t.Fatalf("active response = %#v, want nil without script response", active.Response)
	}

	completed := requests[scriptProjectionCompletedDispatchID]
	if completed.Request.ScriptRequest == nil {
		t.Fatalf("completed script request = %#v, want projected script request", completed.Request)
	}
	if completed.Request.ScriptRequest.Attempt == nil || *completed.Request.ScriptRequest.Attempt != 2 {
		t.Fatalf("completed script request attempt = %#v, want 2", completed.Request.ScriptRequest.Attempt)
	}
	if completed.Request.ScriptRequest.Args == nil || len(*completed.Request.ScriptRequest.Args) != 2 || (*completed.Request.ScriptRequest.Args)[1] != "completed" {
		t.Fatalf("completed script request args = %#v, want [--mode completed]", completed.Request.ScriptRequest.Args)
	}
	if completed.Request.RequestTime != nil || completed.Request.WorkingDirectory != nil || completed.Request.Worktree != nil {
		t.Fatalf("completed request inference summary = %#v, want omitted inference-owned detail", completed.Request)
	}
	if completed.Response == nil || completed.Response.ScriptResponse == nil {
		t.Fatalf("completed response = %#v, want projected script response", completed.Response)
	}
	if completed.Response.ScriptResponse.Outcome == nil || *completed.Response.ScriptResponse.Outcome != scriptProjectionCompletedOutcome {
		t.Fatalf("completed script response outcome = %#v, want %q", completed.Response.ScriptResponse.Outcome, scriptProjectionCompletedOutcome)
	}
	if completed.Response.ScriptResponse.ExitCode == nil || *completed.Response.ScriptResponse.ExitCode != exitCode {
		t.Fatalf("completed script response exit code = %#v, want %d", completed.Response.ScriptResponse.ExitCode, exitCode)
	}
	if completed.Response.ScriptResponse.Stdout == nil || *completed.Response.ScriptResponse.Stdout != scriptProjectionCompletedStdout {
		t.Fatalf("completed script response stdout = %#v, want %q", completed.Response.ScriptResponse.Stdout, scriptProjectionCompletedStdout)
	}
	if completed.Response.ScriptResponse.FailureType == nil || *completed.Response.ScriptResponse.FailureType != "TIMEOUT" {
		t.Fatalf("completed script response failure type = %#v, want TIMEOUT", completed.Response.ScriptResponse.FailureType)
	}
	if completed.Response.ResponseText != nil || completed.Response.ProviderSession != nil || completed.Response.Diagnostics != nil {
		t.Fatalf("completed response inference summary = %#v, want only script-backed detail", completed.Response)
	}
}

func completedInputSlice(item interfaces.FactoryWorkItem) []interfaces.FactoryWorkItem {
	return []interfaces.FactoryWorkItem{item}
}

func TestWorkItemRefsForAPIProjection_FilterCustomerWorkAndPreserveLineage(t *testing.T) {
	itemsByID := map[string]interfaces.FactoryWorkItem{
		"work-2": {ID: "work-2", WorkTypeID: "task", DisplayName: "Second", CurrentChainingTraceID: "chain-2", PreviousChainingTraceIDs: []string{"chain-0", "chain-1"}, TraceID: "trace-2"},
		"work-1": {ID: "work-1", WorkTypeID: "task", DisplayName: "First", CurrentChainingTraceID: "chain-1", PreviousChainingTraceIDs: []string{"chain-0"}, TraceID: "trace-1"},
		"time-1": {ID: "time-1", WorkTypeID: interfaces.SystemTimeWorkTypeID, DisplayName: "tick"},
	}

	refsByID := workItemRefsForIDs([]string{"work-2", "time-1", "work-1", "work-2"}, itemsByID)
	if len(refsByID) != 2 || refsByID[0].WorkID != "work-1" || refsByID[1].WorkID != "work-2" {
		t.Fatalf("workItemRefsForIDs = %#v, want sorted customer refs", refsByID)
	}
	if refsByID[0].CurrentChainingTraceID != "chain-1" || len(refsByID[1].PreviousChainingTraceIDs) != 2 {
		t.Fatalf("workItemRefsForIDs lineage = %#v, want explicit chaining fields", refsByID)
	}

	refsForItems := workItemRefsForItems([]interfaces.FactoryWorkItem{
		itemsByID["work-2"],
		itemsByID["time-1"],
		itemsByID["work-2"],
		itemsByID["work-1"],
	})
	if len(refsForItems) != 2 || refsForItems[0].WorkID != "work-2" || refsForItems[1].WorkID != "work-1" {
		t.Fatalf("workItemRefsForItems = %#v, want first-occurrence customer refs", refsForItems)
	}

	refsForInputs := workItemRefsForInputs([]interfaces.WorkstationInput{
		{WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "First", CurrentChainingTraceID: "chain-1", PreviousChainingTraceIDs: []string{"chain-0"}}},
		{WorkItem: &interfaces.FactoryWorkItem{ID: "time-1", WorkTypeID: interfaces.SystemTimeWorkTypeID, DisplayName: "tick"}},
		{WorkItem: &interfaces.FactoryWorkItem{ID: "work-1", WorkTypeID: "task", DisplayName: "First", CurrentChainingTraceID: "chain-1", PreviousChainingTraceIDs: []string{"chain-0"}}},
		{WorkItem: &interfaces.FactoryWorkItem{ID: "work-2", WorkTypeID: "task", DisplayName: "Second", CurrentChainingTraceID: "chain-2", PreviousChainingTraceIDs: []string{"chain-0", "chain-1"}}},
	})
	if len(refsForInputs) != 2 || refsForInputs[0].WorkID != "work-1" || refsForInputs[1].WorkID != "work-2" {
		t.Fatalf("workItemRefsForInputs = %#v, want first-occurrence customer refs", refsForInputs)
	}
}
