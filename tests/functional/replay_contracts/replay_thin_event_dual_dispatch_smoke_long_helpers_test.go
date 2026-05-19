//go:build functionallong

package replay_contracts

import (
	"encoding/json"
	"testing"

	factoryboundary "github.com/portpowered/infinite-you/pkg/api"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func assertThinEventReconstructedModelReader(t *testing.T, smoke dualDispatchSmokeFixture, worldState interfaces.FactoryWorldState) {
	t.Helper()

	dispatchID := thinEventDispatchIDForWork(t, smoke.artifact.Events, smoke.modelWorkID)
	completion := thinEventCompletedDispatchForID(t, worldState.CompletedDispatches, dispatchID)
	if completion.Result.Outcome != string(interfaces.OutcomeAccepted) {
		t.Fatalf("model dispatch outcome = %q, want ACCEPTED", completion.Result.Outcome)
	}
	if len(completion.InputWorkItems) != 1 || completion.InputWorkItems[0].ID != smoke.modelWorkID {
		t.Fatalf("model input work items = %#v, want %q", completion.InputWorkItems, smoke.modelWorkID)
	}
	if len(completion.OutputWorkItems) != 1 || completion.OutputWorkItems[0].WorkTypeID != "task" {
		t.Fatalf("model output work items = %#v, want one terminal task output", completion.OutputWorkItems)
	}
	if len(completion.TraceIDs) != 1 || completion.TraceIDs[0] != smoke.traceID {
		t.Fatalf("model completion trace IDs = %#v, want [%q]", completion.TraceIDs, smoke.traceID)
	}

	attempts := worldState.InferenceAttemptsByDispatchID[dispatchID]
	if len(attempts) != 1 {
		t.Fatalf("model inference attempts = %#v, want one attempt", attempts)
	}
	for _, attempt := range attempts {
		if attempt.InferenceRequestID == "" || attempt.Response == "" || attempt.ResponseTime.IsZero() {
			t.Fatalf("model attempt = %#v, want request ID, response text, and response time", attempt)
		}
		if attempt.DispatchID != dispatchID {
			t.Fatalf("model attempt dispatch = %q, want %q", attempt.DispatchID, dispatchID)
		}
	}
}

func assertThinEventReconstructedScriptReader(t *testing.T, smoke dualDispatchSmokeFixture, worldState interfaces.FactoryWorldState) {
	t.Helper()

	dispatchID := thinEventDispatchIDForWork(t, smoke.artifact.Events, smoke.scriptWorkID)
	completion := thinEventCompletedDispatchForID(t, worldState.CompletedDispatches, dispatchID)
	if completion.Result.Outcome != string(interfaces.OutcomeAccepted) {
		t.Fatalf("script dispatch outcome = %q, want ACCEPTED", completion.Result.Outcome)
	}
	if len(completion.InputWorkItems) != 1 || completion.InputWorkItems[0].ID != smoke.scriptWorkID {
		t.Fatalf("script input work items = %#v, want %q", completion.InputWorkItems, smoke.scriptWorkID)
	}
	if len(completion.TraceIDs) != 1 || completion.TraceIDs[0] != smoke.traceID {
		t.Fatalf("script completion trace IDs = %#v, want [%q]", completion.TraceIDs, smoke.traceID)
	}

	requests := worldState.ScriptRequestsByDispatchID[dispatchID]
	if len(requests) != 1 {
		t.Fatalf("script requests = %#v, want one request", requests)
	}
	var request interfaces.FactoryWorldScriptRequest
	for _, candidate := range requests {
		request = candidate
	}
	if request.Command != "script-tool" || len(request.Args) != 2 || request.Args[0] != "--mode" || request.Args[1] != "smoke" {
		t.Fatalf("script request = %#v, want script-tool [--mode smoke]", request)
	}
	if request.RequestTime.IsZero() {
		t.Fatalf("script request time is zero: %#v", request)
	}

	responses := worldState.ScriptResponsesByDispatchID[dispatchID]
	if len(responses) != 1 {
		t.Fatalf("script responses = %#v, want one response", responses)
	}
	var response interfaces.FactoryWorldScriptResponse
	for _, candidate := range responses {
		response = candidate
	}
	if response.ScriptRequestID != request.ScriptRequestID {
		t.Fatalf("script response request ID = %q, want %q", response.ScriptRequestID, request.ScriptRequestID)
	}
	if response.Outcome != string(factoryapi.ScriptExecutionOutcomeSucceeded) ||
		response.Stdout != "script dispatch complete" ||
		response.Stderr != "" ||
		response.ExitCode == nil || *response.ExitCode != 0 ||
		response.ResponseTime.IsZero() {
		t.Fatalf("script response = %#v, want succeeded stdout/stderr/exit_code details", response)
	}
}

func assertThinEventWorkstationRequestProjection(t *testing.T, smoke dualDispatchSmokeFixture, worldState interfaces.FactoryWorldState) {
	t.Helper()

	projection := factoryboundary.BuildFactoryWorldWorkstationRequestProjectionSlice(worldState)
	if projection.WorkstationRequestsByDispatchId == nil {
		t.Fatal("workstation request projection missing request map")
	}
	requests := *projection.WorkstationRequestsByDispatchId

	modelDispatchID := thinEventDispatchIDForWork(t, smoke.artifact.Events, smoke.modelWorkID)
	model := requests[modelDispatchID]
	assertReplayProjectionOmitsInferenceFields(
		t,
		model.Request,
		[]string{"requestTime", "prompt", "worktree", "workingDirectory"},
	)
	if model.Response == nil {
		t.Fatalf("model workstation response projection = %#v, want thin dispatch summary without inference response detail", model.Response)
	}
	assertReplayProjectionOmitsInferenceFields(
		t,
		model.Response,
		[]string{"providerSession", "diagnostics", "responseText"},
	)
	modelAttempts := worldState.InferenceAttemptsByDispatchID[modelDispatchID]
	if len(modelAttempts) != 1 {
		t.Fatalf("model inference attempts = %#v, want one attempt", modelAttempts)
	}
	for _, attempt := range modelAttempts {
		if attempt.InferenceRequestID == "" || attempt.RequestTime.IsZero() || attempt.Prompt == "" || attempt.Response == "" {
			t.Fatalf("model inference attempt = %#v, want request ID plus prompt and response detail", attempt)
		}
	}

	scriptDispatchID := thinEventDispatchIDForWork(t, smoke.artifact.Events, smoke.scriptWorkID)
	script := requests[scriptDispatchID]
	if script.Request.ScriptRequest == nil || script.Request.ScriptRequest.Command == nil || *script.Request.ScriptRequest.Command != "script-tool" {
		t.Fatalf("script workstation request = %#v, want script request command", script.Request.ScriptRequest)
	}
	if script.Response == nil || script.Response.ScriptResponse == nil {
		t.Fatalf("script workstation response = %#v, want script response", script.Response)
	}
	if script.Response.ScriptResponse.Outcome == nil || *script.Response.ScriptResponse.Outcome != string(factoryapi.ScriptExecutionOutcomeSucceeded) {
		t.Fatalf("script workstation outcome = %#v, want SUCCEEDED", script.Response.ScriptResponse)
	}
	if script.Response.ScriptResponse.ExitCode == nil || *script.Response.ScriptResponse.ExitCode != 0 {
		t.Fatalf("script workstation exit code = %#v, want 0", script.Response.ScriptResponse.ExitCode)
	}
	if script.Request.TraceIds == nil || len(*script.Request.TraceIds) != 1 || (*script.Request.TraceIds)[0] != smoke.traceID {
		t.Fatalf("script workstation trace IDs = %#v, want [%q]", script.Request.TraceIds, smoke.traceID)
	}
}

func assertReplayProjectionOmitsInferenceFields(t *testing.T, payload any, keys []string) {
	t.Helper()

	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal(%T): %v", payload, err)
	}

	var raw map[string]any
	if err := json.Unmarshal(encoded, &raw); err != nil {
		t.Fatalf("Unmarshal(%T): %v", payload, err)
	}
	for _, key := range keys {
		if _, ok := raw[key]; ok {
			t.Fatalf("%T unexpectedly carried retired inference-owned field %q: %#v", payload, key, raw[key])
		}
	}
}

func thinEventDispatchIDForWork(t *testing.T, events []factoryapi.FactoryEvent, workID string) string {
	t.Helper()

	index := requireThinEventDispatchRequestIndexForWork(t, events, workID)
	return thinEventDispatchIDFromEvent(t, events[index], workID)
}

func thinEventCompletedDispatchForID(
	t *testing.T,
	completions []interfaces.FactoryWorldDispatchCompletion,
	dispatchID string,
) interfaces.FactoryWorldDispatchCompletion {
	t.Helper()

	for _, completion := range completions {
		if completion.DispatchID == dispatchID {
			return completion
		}
	}
	t.Fatalf("completed dispatches = %#v, want dispatch %q", completions, dispatchID)
	return interfaces.FactoryWorldDispatchCompletion{}
}
