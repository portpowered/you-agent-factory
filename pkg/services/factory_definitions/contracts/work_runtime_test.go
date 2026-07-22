package factorycontracts

import (
	"encoding/json"
	"reflect"
	"testing"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
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
