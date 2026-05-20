package interfaces

import (
	"encoding/json"
	"testing"
)

func TestWorkDispatch_JSONOmitsWorkerOwnedFieldsByDefault(t *testing.T) {
	t.Parallel()

	payload, err := json.Marshal(WorkDispatch{})
	if err != nil {
		t.Fatalf("json marshal unexpected error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("json unmarshal unexpected error: %v", err)
	}

	for _, key := range []string{
		"dispatch_id",
		"transition_id",
		"input_tokens",
	} {
		if _, ok := got[key]; !ok {
			t.Fatalf("expected %q in payload: %s", key, string(payload))
		}
	}

	for _, key := range []string{
		"workstation_type",
		"model",
		"model_provider",
		"session_id",
		"env_vars",
		"system_prompt",
		"user_message",
		"output_schema",
		"worktree",
		"working_directory",
	} {
		if _, ok := got[key]; ok {
			t.Fatalf("did not expect worker-owned field %q on WorkDispatch: %s", key, string(payload))
		}
	}
}

func TestWorkDispatch_StoresDispatchOwnedFields(t *testing.T) {
	t.Parallel()

	got := marshalPayload(t, testWorkDispatch())

	for key, want := range map[string]any{
		"dispatch_id":               "dispatch-1",
		"transition_id":             "step-1",
		"worker_type":               "agent-worker",
		"workstation_name":          "review",
		"project_id":                "project-1",
		"current_chaining_trace_id": "chain-current-1",
	} {
		if got[key] != want {
			t.Fatalf("%s = %q, want %q", key, got[key], want)
		}
	}

	previous, ok := got["previous_chaining_trace_ids"].([]any)
	if !ok || len(previous) != 2 || previous[0] != "chain-a" || previous[1] != "chain-b" {
		t.Fatalf("previous_chaining_trace_ids = %#v, want [chain-a chain-b]", got["previous_chaining_trace_ids"])
	}

	execSection, ok := got["execution"].(map[string]any)
	if !ok || execSection["current_tick"] != float64(14) {
		t.Fatalf("execution = %#v, want current_tick 14", got["execution"])
	}

	bindings, ok := got["input_bindings"].(map[string]any)
	if !ok || len(bindings) != 1 {
		t.Fatalf("input_bindings = %#v, want one binding", got["input_bindings"])
	}
}

func TestCloneWorkDispatch_DetachesWorkerBoundarySlicesAndMaps(t *testing.T) {
	t.Parallel()

	original := testWorkDispatch()
	clone := CloneWorkDispatch(original)

	clone.PreviousChainingTraceIDs[0] = "changed"
	clone.Execution.WorkIDs[0] = "changed"
	clone.InputTokens[0] = map[string]any{"id": "changed"}
	clone.InputBindings["source"][0] = "changed"

	if original.PreviousChainingTraceIDs[0] != "chain-a" {
		t.Fatalf("previous chaining IDs mutated original: %#v", original.PreviousChainingTraceIDs)
	}
	if original.Execution.WorkIDs[0] != "w1" {
		t.Fatalf("execution work IDs mutated original: %#v", original.Execution.WorkIDs)
	}
	if original.InputBindings["source"][0] != "a" {
		t.Fatalf("input bindings mutated original: %#v", original.InputBindings)
	}
}

func TestCloneWorkstationExecutionRequest_DetachesRuntimeFields(t *testing.T) {
	t.Parallel()

	original := WorkstationExecutionRequest{
		Dispatch:         testWorkDispatch(),
		WorkerType:       "worker-a",
		WorkstationType:  "review",
		ProjectID:        "project-override",
		InputTokens:      []any{map[string]any{"id": "token-2"}},
		SystemPrompt:     "system",
		UserMessage:      "user",
		OutputSchema:     "{}",
		EnvVars:          map[string]string{"TASK": "dispatch"},
		Worktree:         "/tmp/worktree",
		WorkingDirectory: "/tmp/working",
	}

	clone := CloneWorkstationExecutionRequest(original)
	clone.Dispatch.InputBindings["source"][0] = "changed"
	clone.InputTokens[0] = map[string]any{"id": "changed"}
	clone.EnvVars["TASK"] = "changed"

	if original.Dispatch.InputBindings["source"][0] != "a" {
		t.Fatalf("dispatch bindings mutated original: %#v", original.Dispatch.InputBindings)
	}
	if original.EnvVars["TASK"] != "dispatch" {
		t.Fatalf("env vars mutated original: %#v", original.EnvVars)
	}
}

func TestCloneProviderInferenceRequest_DetachesProviderFields(t *testing.T) {
	t.Parallel()

	original := ProviderInferenceRequest{
		Dispatch:          testWorkDispatch(),
		WorkerType:        "worker-a",
		WorkstationType:   "review",
		ProjectID:         "project-override",
		InputTokens:       []any{map[string]any{"id": "token-2"}},
		SystemPrompt:      "system",
		UserMessage:       "user",
		OutputSchema:      "{}",
		ToolExecutionMode: RunnerToolExecutionModeRequired,
		RequiredOptionalCapabilities: []RunnerOptionalCapability{
			RunnerOptionalCapabilityStructuredOutput,
			RunnerOptionalCapabilitySessionResume,
		},
		EnvVars:          map[string]string{"TASK": "dispatch"},
		Worktree:         "/tmp/worktree",
		WorkingDirectory: "/tmp/working",
		Model:            "model-x",
		ModelProvider:    "acme",
		SessionID:        "session-1",
	}

	clone := CloneProviderInferenceRequest(original)
	clone.Dispatch.InputBindings["source"][0] = "changed"
	clone.InputTokens[0] = map[string]any{"id": "changed"}
	clone.RequiredOptionalCapabilities[0] = RunnerOptionalCapabilityImageInput
	clone.EnvVars["TASK"] = "changed"

	if original.Dispatch.InputBindings["source"][0] != "a" {
		t.Fatalf("dispatch bindings mutated original: %#v", original.Dispatch.InputBindings)
	}
	if original.RequiredOptionalCapabilities[0] != RunnerOptionalCapabilityStructuredOutput {
		t.Fatalf("required optional capabilities mutated original: %#v", original.RequiredOptionalCapabilities)
	}
	if original.EnvVars["TASK"] != "dispatch" {
		t.Fatalf("env vars mutated original: %#v", original.EnvVars)
	}
}

func testWorkDispatch() WorkDispatch {
	return WorkDispatch{
		DispatchID:             "dispatch-1",
		TransitionID:           "step-1",
		WorkerType:             "agent-worker",
		WorkstationName:        "review",
		ProjectID:              "project-1",
		CurrentChainingTraceID: "chain-current-1",
		PreviousChainingTraceIDs: []string{
			"chain-a",
			"chain-b",
		},
		Execution:     ExecutionMetadata{CurrentTick: 14, RequestID: "req-1", WorkIDs: []string{"w1", "w2"}},
		InputTokens:   []any{map[string]any{"id": "token-1"}},
		InputBindings: map[string][]string{"source": {"a", "b"}},
	}
}

func marshalPayload(t *testing.T, value any) map[string]any {
	t.Helper()

	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json marshal unexpected error: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("json unmarshal unexpected error: %v", err)
	}
	return got
}
