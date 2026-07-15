package factorycontracts

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
)

func TestWorkDiagnosticsFromSafeWorkDiagnostics_NilSafe(t *testing.T) {
	if got := WorkDiagnosticsFromSafeWorkDiagnostics(nil); got != nil {
		t.Fatalf("WorkDiagnosticsFromSafeWorkDiagnostics(nil) = %#v, want nil", got)
	}
	if got := WorkDiagnosticsFromSafeWorkDiagnostics(&SafeWorkDiagnostics{}); got != nil {
		t.Fatalf("WorkDiagnosticsFromSafeWorkDiagnostics(empty) = %#v, want nil", got)
	}
}

func TestCanonicalFactoryGraphEntityIDFallsBackToLegacyName(t *testing.T) {
	t.Parallel()

	if got := CanonicalFactoryGraphEntityID("", "legacy-resource"); got != "legacy-resource" {
		t.Fatalf("CanonicalFactoryGraphEntityID fallback = %q, want legacy-resource", got)
	}
	if got := CanonicalFactoryGraphNodeID("resource", CanonicalFactoryGraphEntityID("", "legacy-resource")); got != "resource:legacy-resource" {
		t.Fatalf("CanonicalFactoryGraphNodeID fallback = %q, want resource:legacy-resource", got)
	}
}

func TestCanonicalFactoryGraphIDsStayStableAcrossRenamesWhenExplicitIDsExist(t *testing.T) {
	t.Parallel()

	beforeRenameWorkType := WorkTypeConfig{ID: "work-type-story", Name: "story"}
	beforeRenameState := StateConfig{ID: "state-queued", Name: "queued"}
	beforeRenameWorkstation := FactoryWorkstationConfig{ID: "workstation-review", Name: "review"}
	beforeNodeID := CanonicalFactoryGraphNodeID(
		"work-state",
		CanonicalFactoryGraphWorkStateID(beforeRenameWorkType, beforeRenameState),
	)
	beforeEdgeID := CanonicalFactoryGraphEdgeID(
		"workstation-input",
		beforeNodeID,
		CanonicalFactoryGraphNodeID(
			"workstation",
			CanonicalFactoryGraphWorkstationID(beforeRenameWorkstation),
		),
	)

	afterRenameWorkType := WorkTypeConfig{ID: "work-type-story", Name: "renamed-story"}
	afterRenameState := StateConfig{ID: "state-queued", Name: "renamed-queued"}
	afterRenameWorkstation := FactoryWorkstationConfig{ID: "workstation-review", Name: "renamed-review"}
	afterNodeID := CanonicalFactoryGraphNodeID(
		"work-state",
		CanonicalFactoryGraphWorkStateID(afterRenameWorkType, afterRenameState),
	)
	afterEdgeID := CanonicalFactoryGraphEdgeID(
		"workstation-input",
		afterNodeID,
		CanonicalFactoryGraphNodeID(
			"workstation",
			CanonicalFactoryGraphWorkstationID(afterRenameWorkstation),
		),
	)

	if beforeNodeID != afterNodeID {
		t.Fatalf("work-state node id changed across rename: before=%q after=%q", beforeNodeID, afterNodeID)
	}
	if beforeEdgeID != afterEdgeID {
		t.Fatalf("edge id changed across rename: before=%q after=%q", beforeEdgeID, afterEdgeID)
	}
}

func TestWorkDiagnosticsFromSafeWorkDiagnostics_ClonesMutableMaps(t *testing.T) {
	safe := &SafeWorkDiagnostics{
		RenderedPrompt: &SafeRenderedPromptDiagnostic{
			Variables: map[string]string{"prompt_source": "factory"},
		},
		Provider: &SafeProviderDiagnostic{
			RequestMetadata:  map[string]string{"session_id": "req-1"},
			ResponseMetadata: map[string]string{"retry_count": "0"},
		},
	}

	got := WorkDiagnosticsFromSafeWorkDiagnostics(safe)
	got.RenderedPrompt.Variables["prompt_source"] = "mutated"
	got.Provider.RequestMetadata["session_id"] = "mutated"
	got.Provider.ResponseMetadata["retry_count"] = "1"

	if safe.RenderedPrompt.Variables["prompt_source"] != "factory" {
		t.Fatalf("safe rendered prompt variables mutated = %#v", safe.RenderedPrompt.Variables)
	}
	if safe.Provider.RequestMetadata["session_id"] != "req-1" {
		t.Fatalf("safe request metadata mutated = %#v", safe.Provider.RequestMetadata)
	}
	if safe.Provider.ResponseMetadata["retry_count"] != "0" {
		t.Fatalf("safe response metadata mutated = %#v", safe.Provider.ResponseMetadata)
	}
}

func TestSafeWorkDiagnosticsRoundTrip_PreservesSafeFieldsOnly(t *testing.T) {
	original := &WorkDiagnostics{
		RenderedPrompt: &RenderedPromptDiagnostic{
			SystemPromptHash: "system-hash",
			UserMessageHash:  "user-hash",
			Variables: map[string]string{
				"prompt_source": "factory",
				"request_id":    "req-1",
				"secret":        "drop-me",
			},
		},
		Provider: &ProviderDiagnostic{
			Provider: "openai",
			Model:    "gpt-5.4",
			RequestMetadata: map[string]string{
				"session_id": "sess-1",
				"unsafe":     "drop-me",
			},
			ResponseMetadata: map[string]string{
				"retry_count": "0",
				"raw_body":    "drop-me",
			},
		},
		Command: &CommandDiagnostic{
			Command: "python",
			Stdin:   "raw prompt",
		},
		Panic: &PanicDiagnostic{
			Message: "boom",
			Stack:   "stack",
		},
		Metadata: map[string]string{"arbitrary": "drop-me"},
	}

	safe := SafeWorkDiagnosticsFromWorkDiagnostics(original)
	rehydrated := WorkDiagnosticsFromSafeWorkDiagnostics(safe)

	want := &WorkDiagnostics{
		RenderedPrompt: &RenderedPromptDiagnostic{
			SystemPromptHash: "system-hash",
			UserMessageHash:  "user-hash",
			Variables: map[string]string{
				"prompt_source": "factory",
				"request_id":    "req-1",
			},
		},
		Provider: &ProviderDiagnostic{
			Provider: "openai",
			Model:    "gpt-5.4",
			RequestMetadata: map[string]string{
				"session_id": "sess-1",
			},
			ResponseMetadata: map[string]string{
				"retry_count": "0",
			},
		},
	}

	if !reflect.DeepEqual(rehydrated, want) {
		t.Fatalf("rehydrated diagnostics = %#v, want %#v", rehydrated, want)
	}
	if rehydrated.Command != nil {
		t.Fatalf("rehydrated command diagnostics = %#v, want nil", rehydrated.Command)
	}
	if rehydrated.Panic != nil {
		t.Fatalf("rehydrated panic diagnostics = %#v, want nil", rehydrated.Panic)
	}
	if rehydrated.Metadata != nil {
		t.Fatalf("rehydrated metadata = %#v, want nil", rehydrated.Metadata)
	}
}

func TestGeneratedSafeWorkDiagnostics_ClonesMapsAndPreservesNil(t *testing.T) {
	diagnostics := &SafeWorkDiagnostics{
		RenderedPrompt: &SafeRenderedPromptDiagnostic{
			Variables: map[string]string{"prompt_source": "factory"},
		},
		Provider: &SafeProviderDiagnostic{
			RequestMetadata:  map[string]string{"session_id": "req-1"},
			ResponseMetadata: map[string]string{"retry_count": "0"},
		},
	}

	generated := GeneratedSafeWorkDiagnostics(diagnostics)
	(*generated.RenderedPrompt.Variables)["prompt_source"] = "mutated"
	(*generated.Provider.RequestMetadata)["session_id"] = "mutated"
	(*generated.Provider.ResponseMetadata)["retry_count"] = "1"

	if diagnostics.RenderedPrompt.Variables["prompt_source"] != "factory" {
		t.Fatalf("safe rendered prompt variables mutated = %#v", diagnostics.RenderedPrompt.Variables)
	}
	if diagnostics.Provider.RequestMetadata["session_id"] != "req-1" {
		t.Fatalf("safe request metadata mutated = %#v", diagnostics.Provider.RequestMetadata)
	}
	if diagnostics.Provider.ResponseMetadata["retry_count"] != "0" {
		t.Fatalf("safe response metadata mutated = %#v", diagnostics.Provider.ResponseMetadata)
	}

	if got := GeneratedSafeWorkDiagnostics(&SafeWorkDiagnostics{
		RenderedPrompt: &SafeRenderedPromptDiagnostic{Variables: nil},
		Provider: &SafeProviderDiagnostic{
			RequestMetadata:  nil,
			ResponseMetadata: map[string]string{},
		},
	}); got == nil {
		t.Fatal("GeneratedSafeWorkDiagnostics returned nil, want non-nil diagnostics shell")
	} else {
		assertNilStringMapPtr(t, got.RenderedPrompt.Variables, "rendered prompt variables")
		assertNilStringMapPtr(t, got.Provider.RequestMetadata, "request metadata")
		assertNilStringMapPtr(t, got.Provider.ResponseMetadata, "response metadata")
	}
}

func TestSafeWorkDiagnosticsFromGenerated_ClonesMapsAndPreservesNil(t *testing.T) {
	diagnostics := &factoryapi.SafeWorkDiagnostics{
		RenderedPrompt: &factoryapi.RenderedPromptDiagnostic{
			Variables: &factoryapi.StringMap{"prompt_source": "factory"},
		},
		Provider: &factoryapi.ProviderDiagnostic{
			RequestMetadata:  &factoryapi.StringMap{"session_id": "req-1"},
			ResponseMetadata: &factoryapi.StringMap{"retry_count": "0"},
		},
	}

	got := SafeWorkDiagnosticsFromGenerated(diagnostics)
	(*diagnostics.RenderedPrompt.Variables)["prompt_source"] = "mutated"
	(*diagnostics.Provider.RequestMetadata)["session_id"] = "mutated"
	(*diagnostics.Provider.ResponseMetadata)["retry_count"] = "1"

	if got.RenderedPrompt.Variables["prompt_source"] != "factory" {
		t.Fatalf("rendered prompt variables = %#v, want detached copy", got.RenderedPrompt.Variables)
	}
	if got.Provider.RequestMetadata["session_id"] != "req-1" {
		t.Fatalf("request metadata = %#v, want detached copy", got.Provider.RequestMetadata)
	}
	if got.Provider.ResponseMetadata["retry_count"] != "0" {
		t.Fatalf("response metadata = %#v, want detached copy", got.Provider.ResponseMetadata)
	}

	if got := SafeWorkDiagnosticsFromGenerated(&factoryapi.SafeWorkDiagnostics{
		RenderedPrompt: &factoryapi.RenderedPromptDiagnostic{Variables: nil},
		Provider: &factoryapi.ProviderDiagnostic{
			RequestMetadata:  nil,
			ResponseMetadata: &factoryapi.StringMap{},
		},
	}); got == nil {
		t.Fatal("SafeWorkDiagnosticsFromGenerated returned nil, want non-nil diagnostics shell")
	} else {
		if got.RenderedPrompt.Variables != nil {
			t.Fatalf("rendered prompt variables = %#v, want nil", got.RenderedPrompt.Variables)
		}
		if got.Provider.RequestMetadata != nil {
			t.Fatalf("request metadata = %#v, want nil", got.Provider.RequestMetadata)
		}
		if got.Provider.ResponseMetadata != nil {
			t.Fatalf("response metadata = %#v, want nil", got.Provider.ResponseMetadata)
		}
	}
}

func TestSafeWorkDiagnosticsFromGenerated_RoundTripPreservesObservableSafeFields(t *testing.T) {
	diagnostics := &factoryapi.SafeWorkDiagnostics{
		RenderedPrompt: &factoryapi.RenderedPromptDiagnostic{
			SystemPromptHash: stringPtr("system-hash"),
			UserMessageHash:  stringPtr("user-hash"),
			Variables: &factoryapi.StringMap{
				"prompt_source": "factory",
				"request_id":    "req-1",
			},
		},
		Provider: &factoryapi.ProviderDiagnostic{
			Provider: stringPtr("openai"),
			Model:    stringPtr("gpt-5.4"),
			RequestMetadata: &factoryapi.StringMap{
				"session_id": "sess-1",
			},
			ResponseMetadata: &factoryapi.StringMap{
				"retry_count": "0",
			},
		},
	}

	safe := SafeWorkDiagnosticsFromGenerated(diagnostics)
	rehydrated := WorkDiagnosticsFromSafeWorkDiagnostics(safe)

	(*diagnostics.RenderedPrompt.Variables)["prompt_source"] = "mutated"
	(*diagnostics.Provider.RequestMetadata)["session_id"] = "mutated"
	(*diagnostics.Provider.ResponseMetadata)["retry_count"] = "1"

	wantSafe := &SafeWorkDiagnostics{
		RenderedPrompt: &SafeRenderedPromptDiagnostic{
			SystemPromptHash: "system-hash",
			UserMessageHash:  "user-hash",
			Variables: map[string]string{
				"prompt_source": "factory",
				"request_id":    "req-1",
			},
		},
		Provider: &SafeProviderDiagnostic{
			Provider: "openai",
			Model:    "gpt-5.4",
			RequestMetadata: map[string]string{
				"session_id": "sess-1",
			},
			ResponseMetadata: map[string]string{
				"retry_count": "0",
			},
		},
	}
	if !reflect.DeepEqual(safe, wantSafe) {
		t.Fatalf("safe diagnostics = %#v, want %#v", safe, wantSafe)
	}

	wantWork := &WorkDiagnostics{
		RenderedPrompt: &RenderedPromptDiagnostic{
			SystemPromptHash: "system-hash",
			UserMessageHash:  "user-hash",
			Variables: map[string]string{
				"prompt_source": "factory",
				"request_id":    "req-1",
			},
		},
		Provider: &ProviderDiagnostic{
			Provider: "openai",
			Model:    "gpt-5.4",
			RequestMetadata: map[string]string{
				"session_id": "sess-1",
			},
			ResponseMetadata: map[string]string{
				"retry_count": "0",
			},
		},
	}
	if !reflect.DeepEqual(rehydrated, wantWork) {
		t.Fatalf("rehydrated diagnostics = %#v, want %#v", rehydrated, wantWork)
	}
}

func TestSafeWorkDiagnostics_InvocationRoundTripPreservesRedactedSummary(t *testing.T) {
	original := &WorkDiagnostics{
		Invocation: &InvocationDiagnostic{
			SignatureHash: "sig-123",
			Parameters: []InvocationParameterDiagnostic{
				{Name: "input", SourceKinds: []string{"NAMED"}, ValueCount: 1},
				{Name: "apiKey", SourceKinds: []string{"NAMED", "DEFAULT"}, ValueCount: 2, Redacted: true},
			},
		},
	}

	safe := SafeWorkDiagnosticsFromWorkDiagnostics(original)
	if safe == nil || safe.Invocation == nil {
		t.Fatalf("safe diagnostics = %#v, want invocation summary", safe)
	}
	rehydrated := WorkDiagnosticsFromSafeWorkDiagnostics(safe)
	if !reflect.DeepEqual(rehydrated, original) {
		t.Fatalf("rehydrated diagnostics = %#v, want %#v", rehydrated, original)
	}

	generated := GeneratedSafeWorkDiagnostics(safe)
	if generated == nil || generated.Invocation == nil || generated.Invocation.Parameters == nil || len(*generated.Invocation.Parameters) != 2 {
		t.Fatalf("generated diagnostics = %#v, want invocation parameters", generated)
	}
	(*generated.Invocation.Parameters)[0].SourceKinds = &[]string{"MUTATED"}
	if safe.Invocation.Parameters[0].SourceKinds[0] != "NAMED" {
		t.Fatalf("safe invocation parameter source kinds mutated = %#v", safe.Invocation.Parameters[0].SourceKinds)
	}
}

func assertNilStringMapPtr(t *testing.T, got *factoryapi.StringMap, field string) {
	t.Helper()
	if got != nil {
		t.Fatalf("%s = %#v, want nil", field, got)
	}
}

func stringPtr(value string) *string {
	return &value
}

func TestCloneWorkDiagnostics_Nil(t *testing.T) {
	if got := workerexecution.CloneWorkDiagnostics(nil); got != nil {
		t.Fatalf("CloneWorkDiagnostics(nil) = %#v, want nil", got)
	}
}

func TestCloneWorkDiagnostics_PreservesAbsentNestedValues(t *testing.T) {
	source := &WorkDiagnostics{
		RenderedPrompt: &RenderedPromptDiagnostic{},
		Provider:       &ProviderDiagnostic{},
		Command:        &CommandDiagnostic{},
		Panic:          &PanicDiagnostic{},
	}

	clone := workerexecution.CloneWorkDiagnostics(source)

	if clone == nil {
		t.Fatalf("CloneWorkDiagnostics(source) = nil, want clone")
	}
	if clone.RenderedPrompt == nil || clone.RenderedPrompt.Variables != nil {
		t.Fatalf("clone rendered prompt = %#v, want nil variables", clone.RenderedPrompt)
	}
	if clone.Provider == nil || clone.Provider.RequestMetadata != nil || clone.Provider.ResponseMetadata != nil {
		t.Fatalf("clone provider = %#v, want nil metadata maps", clone.Provider)
	}
	if clone.Command == nil || clone.Command.Args != nil || clone.Command.Env != nil {
		t.Fatalf("clone command = %#v, want nil args and env", clone.Command)
	}
	if clone.Metadata != nil {
		t.Fatalf("clone metadata = %#v, want nil", clone.Metadata)
	}
}

func TestCloneWorkDiagnostics_DetachesNestedMutableState(t *testing.T) {
	source := &WorkDiagnostics{
		RenderedPrompt: &RenderedPromptDiagnostic{
			SystemPromptHash: "system-hash",
			UserMessageHash:  "user-hash",
			Variables: map[string]string{
				"prompt_source": "factory",
			},
		},
		Provider: &ProviderDiagnostic{
			Provider: "openai",
			Model:    "gpt-5.4",
			RequestMetadata: map[string]string{
				"session_id": "sess-1",
			},
			ResponseMetadata: map[string]string{
				"retry_count": "0",
			},
		},
		Command: &CommandDiagnostic{
			Command: "python",
			Args:    []string{"run.py", "--verbose"},
			Env: map[string]string{
				"MODE": "test",
			},
			Stdout:     "stdout",
			Stderr:     "stderr",
			ExitCode:   1,
			TimedOut:   true,
			WorkingDir: "/tmp/work",
		},
		Panic: &PanicDiagnostic{
			Message: "boom",
			Stack:   "stack",
		},
		Metadata: map[string]string{
			"worktree": "alpha",
		},
	}

	clone := workerexecution.CloneWorkDiagnostics(source)

	source.RenderedPrompt.Variables["prompt_source"] = "mutated"
	source.Provider.RequestMetadata["session_id"] = "mutated"
	source.Provider.ResponseMetadata["retry_count"] = "9"
	source.Command.Args[0] = "mutated.py"
	source.Command.Env["MODE"] = "prod"
	source.Panic.Message = "changed"
	source.Panic.Stack = "changed-stack"
	source.Metadata["worktree"] = "beta"

	if got := clone.RenderedPrompt.Variables["prompt_source"]; got != "factory" {
		t.Fatalf("clone rendered prompt variable = %q, want %q", got, "factory")
	}
	if got := clone.Provider.RequestMetadata["session_id"]; got != "sess-1" {
		t.Fatalf("clone provider request metadata = %q, want %q", got, "sess-1")
	}
	if got := clone.Provider.ResponseMetadata["retry_count"]; got != "0" {
		t.Fatalf("clone provider response metadata = %q, want %q", got, "0")
	}
	if got := clone.Command.Args[0]; got != "run.py" {
		t.Fatalf("clone command arg = %q, want %q", got, "run.py")
	}
	if got := clone.Command.Env["MODE"]; got != "test" {
		t.Fatalf("clone command env = %q, want %q", got, "test")
	}
	if got := clone.Panic.Message; got != "boom" {
		t.Fatalf("clone panic message = %q, want %q", got, "boom")
	}
	if got := clone.Panic.Stack; got != "stack" {
		t.Fatalf("clone panic stack = %q, want %q", got, "stack")
	}
	if got := clone.Metadata["worktree"]; got != "alpha" {
		t.Fatalf("clone metadata = %q, want %q", got, "alpha")
	}
}

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
		Dispatch:              testWorkDispatch(),
		WorkerType:            "worker-a",
		WorkstationType:       "review",
		RunnerSelectionSource: RunnerSelectionSourceWorkstation,
		ProjectID:             "project-override",
		InputTokens:           []any{map[string]any{"id": "token-2"}},
		SystemPrompt:          "system",
		UserMessage:           "user",
		OutputSchema:          "{}",
		EnvVars:               map[string]string{"TASK": "dispatch"},
		Worktree:              "/tmp/worktree",
		WorkingDirectory:      "/tmp/working",
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
		ModelOperation:    "TTS",
		ModelBindings:     []ResolvedModelOperationBinding{{Slot: "text", Source: ModelOperationBindingSourceInput, Content: []WorkContentPart{{Type: WorkContentPartTypeText, Text: "hello"}}}},
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
		ModelLocality:    ModelLocalityLocal,
		SessionID:        "session-1",
		OpenCodeAgent:    "implementer",
	}

	clone := CloneProviderInferenceRequest(original)
	clone.Dispatch.InputBindings["source"][0] = "changed"
	clone.InputTokens[0] = map[string]any{"id": "changed"}
	clone.ModelBindings[0].Content[0].Text = "changed"
	clone.RequiredOptionalCapabilities[0] = RunnerOptionalCapabilityImageInput
	clone.EnvVars["TASK"] = "changed"

	if original.Dispatch.InputBindings["source"][0] != "a" {
		t.Fatalf("dispatch bindings mutated original: %#v", original.Dispatch.InputBindings)
	}
	if original.ModelBindings[0].Content[0].Text != "hello" {
		t.Fatalf("model bindings mutated original: %#v", original.ModelBindings)
	}
	if original.RequiredOptionalCapabilities[0] != RunnerOptionalCapabilityStructuredOutput {
		t.Fatalf("required optional capabilities mutated original: %#v", original.RequiredOptionalCapabilities)
	}
	if original.EnvVars["TASK"] != "dispatch" {
		t.Fatalf("env vars mutated original: %#v", original.EnvVars)
	}
	if clone.OpenCodeAgent != "implementer" {
		t.Fatalf("OpenCodeAgent = %q, want implementer", clone.OpenCodeAgent)
	}
	if original.OpenCodeAgent != "implementer" {
		t.Fatalf("OpenCodeAgent mutated original: %q", original.OpenCodeAgent)
	}
}

func TestWorkPayloadLineageProjection_RecordWorkRequestSnapshotDetachesContent(t *testing.T) {
	t.Parallel()

	projection := WorkPayloadLineageProjection{}
	original := FactoryWorkItem{
		ID:      "work-1",
		Content: []WorkContentPart{{Type: WorkContentPartTypeText, Text: "hello"}},
		Tags:    map[string]string{"origin": "request"},
	}

	projection.RecordWorkRequestSnapshot(7, "request-1", original)
	snapshotID := projection.InitialSnapshotIDByWorkID[original.ID]
	snapshot := projection.SnapshotsByID[snapshotID]
	snapshot.WorkItem.Content[0].Text = "changed"

	if original.Content[0].Text != "hello" {
		t.Fatalf("work item content mutated original: %#v", original.Content)
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
			Content: []WorkContentPart{{
				Type: WorkContentPartTypeText,
				Text: "original content",
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
