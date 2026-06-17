package interfaces

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
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
	if got := CloneWorkDiagnostics(nil); got != nil {
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

	clone := CloneWorkDiagnostics(source)

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

	clone := CloneWorkDiagnostics(source)

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

func TestCloneProviderMetadata_PreserveNilValuesAndDetachCopies(t *testing.T) {
	if CloneProviderSessionMetadata(nil) != nil {
		t.Fatal("CloneProviderSessionMetadata(nil) = non-nil, want nil")
	}
	if CloneWorkFailureMetadata(nil) != nil {
		t.Fatal("CloneWorkFailureMetadata(nil) = non-nil, want nil")
	}

	session := &ProviderSessionMetadata{
		Provider: "openai",
		Kind:     "session_id",
		ID:       "sess-1",
	}
	clonedSession := CloneProviderSessionMetadata(session)
	clonedSession.ID = "sess-2"
	if session.ID != "sess-1" {
		t.Fatalf("original provider session = %#v, want sess-1 unchanged", session)
	}

	failure := &WorkFailureMetadata{
		Family: WorkFailureFamilyRetryable,
		Type:   WorkFailureTypeTimeout,
	}
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

func assertNilMatches(t *testing.T, wantNil bool, gotNil bool, field string) {
	t.Helper()
	if wantNil != gotNil {
		t.Fatalf("%s nil state = %t, want %t", field, gotNil, wantNil)
	}
}

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

func TestBuildPendingFactoryGraphTopology_DerivesCanonicalNodesAndEdges(t *testing.T) {
	t.Parallel()

	cfg := &FactoryConfig{
		ResourceManifest: &PortableResourceManifestConfig{
			BundledFiles: []BundledFileConfig{
				{ID: "doc-guide", Type: BundledFileTypeDoc, TargetPath: "docs/guide.md"},
				{Type: BundledFileTypeScript, TargetPath: "scripts/build.sh"},
				{Type: BundledFileTypeInput, TargetPath: "inputs/request.json"},
				{Type: BundledFileTypeRootHelper, TargetPath: "Makefile"},
				{Type: "UNSUPPORTED", TargetPath: "ignored.txt"},
			},
		},
		Resources: []ResourceConfig{
			{ID: "resource-api", Name: "api"},
		},
		Workers: []WorkerConfig{
			{
				ID:   "worker-exec",
				Name: "executor",
				Resources: []ResourceConfig{
					{Name: "api"},
				},
			},
		},
		WorkTypes: []WorkTypeConfig{
			{
				ID:   "worktype-story",
				Name: "story",
				States: []StateConfig{
					{ID: "state-ready", Name: "ready"},
					{ID: "state-done", Name: "done"},
				},
			},
		},
		Workstations: []FactoryWorkstationConfig{
			{
				ID:             "workstation-review",
				Name:           "review",
				WorkerTypeName: "executor",
				Resources: []ResourceConfig{
					{ID: "resource-api", Name: "api"},
				},
				Inputs: []IOConfig{
					{WorkTypeName: "story", StateName: "ready"},
					{WorkTypeName: "missing", StateName: "ready"},
				},
				Outputs: []IOConfig{
					{WorkTypeName: "story", StateName: "done"},
				},
				OnContinue: []IOConfig{
					{WorkTypeName: "story", StateName: "done"},
				},
				OnFailure: []IOConfig{
					{WorkTypeName: "story", StateName: "ready"},
				},
				OnRejection: []IOConfig{
					{WorkTypeName: "story", StateName: "done"},
				},
			},
		},
	}

	topology := BuildPendingFactoryGraphTopology(cfg)

	for _, nodeID := range []string{
		"doc:doc-guide",
		"script:scripts/build.sh",
		"doc:scripts/build.sh",
		"input:inputs/request.json",
		"doc:inputs/request.json",
		"root-helper:Makefile",
		"doc:Makefile",
		"resource:resource-api",
		"worker:worker-exec",
		"work-type:worktype-story",
		"work-state:worktype-story:state-ready",
		"work-state:worktype-story:state-done",
		"workstation:workstation-review",
	} {
		if _, ok := topology.NodeIDs[nodeID]; !ok {
			t.Fatalf("topology.NodeIDs missing %q", nodeID)
		}
	}
	if _, ok := topology.NodeIDs["doc:ignored.txt"]; !ok {
		t.Fatalf("topology.NodeIDs missing legacy compatibility node for unsupported bundled file target")
	}
	if _, ok := topology.NodeIDs["unsupported:ignored.txt"]; ok {
		t.Fatalf("topology.NodeIDs unexpectedly contained typed unsupported bundled file node")
	}

	for _, edgeID := range []string{
		"worker-resource:resource:resource-api->worker:worker-exec",
		"work-type-state:work-type:worktype-story->work-state:worktype-story:state-ready",
		"work-type-state:work-type:worktype-story->work-state:worktype-story:state-done",
		"worker-assignment:worker:worker-exec->workstation:workstation-review",
		"workstation-resource:resource:resource-api->workstation:workstation-review",
		"workstation-input:work-state:worktype-story:state-ready->workstation:workstation-review",
		"workstation-output:workstation:workstation-review->work-state:worktype-story:state-done",
		"workstation-on-continue:workstation:workstation-review->work-state:worktype-story:state-done",
		"workstation-on-failure:workstation:workstation-review->work-state:worktype-story:state-ready",
		"workstation-on-rejection:workstation:workstation-review->work-state:worktype-story:state-done",
	} {
		if _, ok := topology.EdgeIDs[edgeID]; !ok {
			t.Fatalf("topology.EdgeIDs missing %q", edgeID)
		}
	}
}

func TestCloneSubprocessExecutionRequest_DetachesMutableFields(t *testing.T) {
	t.Parallel()

	request := SubprocessExecutionRequest{
		Command:                  "runner",
		Args:                     []string{"--flag"},
		Stdin:                    []byte("stdin"),
		Env:                      []string{"KEY=value"},
		PreviousChainingTraceIDs: []string{"chain-a"},
		Execution: ExecutionMetadata{
			WorkIDs: []string{"work-1"},
		},
		InputTokens:   []any{"token"},
		InputBindings: map[string][]string{"slot": {"work-1"}},
	}

	cloned := CloneSubprocessExecutionRequest(request)
	cloned.Args[0] = "--changed"
	cloned.Stdin[0] = 'X'
	cloned.Env[0] = "KEY=changed"
	cloned.PreviousChainingTraceIDs[0] = "chain-z"
	cloned.Execution.WorkIDs[0] = "work-2"
	cloned.InputTokens[0] = "changed"
	cloned.InputBindings["slot"][0] = "work-2"

	if request.Args[0] != "--flag" {
		t.Fatalf("original subprocess args mutated: %#v", request.Args)
	}
	if string(request.Stdin) != "stdin" {
		t.Fatalf("original subprocess stdin mutated: %q", string(request.Stdin))
	}
	if request.Env[0] != "KEY=value" {
		t.Fatalf("original subprocess env mutated: %#v", request.Env)
	}
	if request.PreviousChainingTraceIDs[0] != "chain-a" {
		t.Fatalf("original subprocess trace ids mutated: %#v", request.PreviousChainingTraceIDs)
	}
	if request.Execution.WorkIDs[0] != "work-1" {
		t.Fatalf("original subprocess execution mutated: %#v", request.Execution)
	}
	if request.InputTokens[0] != "token" {
		t.Fatalf("original subprocess input tokens mutated: %#v", request.InputTokens)
	}
	if request.InputBindings["slot"][0] != "work-1" {
		t.Fatalf("original subprocess input bindings mutated: %#v", request.InputBindings)
	}
}

func TestClearGuardBlockingFields_PreservesFailureHistory(t *testing.T) {
	t.Parallel()

	history := &TokenHistory{
		TotalVisits:         map[string]int{"place-1": 1},
		ConsecutiveFailures: map[string]int{"transition-1": 2},
		PlaceVisits:         map[string]int{"place-1": 3},
		LastError:           "boom",
		FailureLog:          []FailureRecord{{TransitionID: "transition-1", Error: "boom", Attempt: 2}},
	}

	ClearGuardBlockingFields(history)

	if len(history.TotalVisits) != 0 || history.TotalVisits == nil {
		t.Fatalf("TotalVisits = %#v, want empty non-nil map", history.TotalVisits)
	}
	if len(history.ConsecutiveFailures) != 0 || history.ConsecutiveFailures == nil {
		t.Fatalf("ConsecutiveFailures = %#v, want empty non-nil map", history.ConsecutiveFailures)
	}
	if len(history.PlaceVisits) != 0 || history.PlaceVisits == nil {
		t.Fatalf("PlaceVisits = %#v, want empty non-nil map", history.PlaceVisits)
	}
	if history.LastError != "boom" {
		t.Fatalf("LastError = %q, want boom", history.LastError)
	}
	if len(history.FailureLog) != 1 || history.FailureLog[0].Error != "boom" {
		t.Fatalf("FailureLog = %#v, want preserved failure history", history.FailureLog)
	}

	ClearGuardBlockingFields(nil)
}

func TestCanonicalBundledFileHelpers(t *testing.T) {
	t.Parallel()

	if got := CanonicalBundledFileID(" explicit-id ", "docs/guide.md"); got != "explicit-id" {
		t.Fatalf("CanonicalBundledFileID explicit = %q, want explicit-id", got)
	}
	if got := CanonicalBundledFileID("", "docs/guide.md"); got != "docs/guide.md" {
		t.Fatalf("CanonicalBundledFileID fallback = %q, want docs/guide.md", got)
	}

	for _, tt := range []struct {
		fileType string
		wantKind string
	}{
		{fileType: BundledFileTypeDoc, wantKind: "doc"},
		{fileType: BundledFileTypeScript, wantKind: "script"},
		{fileType: BundledFileTypeInput, wantKind: "input"},
		{fileType: BundledFileTypeRootHelper, wantKind: "root-helper"},
		{fileType: "unknown", wantKind: ""},
	} {
		if got := CanonicalBundledFileGraphNodeKind(tt.fileType); got != tt.wantKind {
			t.Fatalf("CanonicalBundledFileGraphNodeKind(%q) = %q, want %q", tt.fileType, got, tt.wantKind)
		}
	}

	file := BundledFileConfig{ID: "file-1", Type: BundledFileTypeDoc, TargetPath: "docs/guide.md"}
	if got := CanonicalBundledFileGraphNodeID(file); got != "doc:file-1" {
		t.Fatalf("CanonicalBundledFileGraphNodeID = %q, want doc:file-1", got)
	}
	if got := CanonicalBundledFileGraphNodeID(BundledFileConfig{Type: "unknown", TargetPath: "docs/guide.md"}); got != "" {
		t.Fatalf("CanonicalBundledFileGraphNodeID unsupported = %q, want empty", got)
	}
	if got := CanonicalBundledFileGraphNodeID(BundledFileConfig{Type: BundledFileTypeDoc}); got != "" {
		t.Fatalf("CanonicalBundledFileGraphNodeID empty id = %q, want empty", got)
	}

	if !IsBundledFileGraphNodeID("doc:file-1") || !IsBundledFileGraphNodeID("script:file-1") ||
		!IsBundledFileGraphNodeID("input:file-1") || !IsBundledFileGraphNodeID("root-helper:file-1") {
		t.Fatal("IsBundledFileGraphNodeID did not recognize supported bundled-file node prefixes")
	}
	if IsBundledFileGraphNodeID("resource:file-1") {
		t.Fatal("IsBundledFileGraphNodeID recognized unrelated node prefix")
	}
}

func TestCanonicalGraphEntityHelpers(t *testing.T) {
	t.Parallel()

	resource := ResourceConfig{ID: "resource-api", Name: "api"}
	worker := WorkerConfig{ID: "worker-exec", Name: "executor"}
	workType := WorkTypeConfig{ID: "worktype-story", Name: "story"}
	state := StateConfig{ID: "state-ready", Name: "ready"}

	if got := CanonicalFactoryGraphResourceID(resource); got != "resource-api" {
		t.Fatalf("CanonicalFactoryGraphResourceID = %q, want resource-api", got)
	}
	if got := CanonicalFactoryGraphWorkerID(worker); got != "worker-exec" {
		t.Fatalf("CanonicalFactoryGraphWorkerID = %q, want worker-exec", got)
	}
	if got := CanonicalFactoryGraphWorkTypeID(workType); got != "worktype-story" {
		t.Fatalf("CanonicalFactoryGraphWorkTypeID = %q, want worktype-story", got)
	}
	if got := CanonicalFactoryGraphWorkStateID(workType, state); got != "worktype-story:state-ready" {
		t.Fatalf("CanonicalFactoryGraphWorkStateID = %q, want worktype-story:state-ready", got)
	}
}

func TestCloneFactoryWorldInferenceAttemptsByDispatchID_DeepCopyMutableFields(t *testing.T) {
	t.Parallel()

	exitCode := 7
	attempts := map[string]map[string]FactoryWorldInferenceAttempt{
		"dispatch-1": {
			"request-1": {
				DispatchID: "dispatch-1",
				ExitCode:   &exitCode,
				ProviderSession: &ProviderSessionMetadata{
					Provider: "codex",
					ID:       "sess-1",
				},
				Diagnostics: &SafeWorkDiagnostics{
					Provider: &SafeProviderDiagnostic{
						RequestMetadata: map[string]string{"session_id": "sess-1"},
					},
				},
			},
		},
		"dispatch-empty": {},
	}

	cloned := CloneFactoryWorldInferenceAttemptsByDispatchID(attempts)
	if _, ok := cloned["dispatch-empty"]; ok {
		t.Fatalf("CloneFactoryWorldInferenceAttemptsByDispatchID preserved empty dispatch entry: %#v", cloned)
	}

	attempt := cloned["dispatch-1"]["request-1"]
	attempt.ProviderSession.ID = "sess-2"
	attempt.Diagnostics.Provider.RequestMetadata["session_id"] = "sess-2"
	*attempt.ExitCode = 9

	originalAttempt := attempts["dispatch-1"]["request-1"]
	if originalAttempt.ProviderSession.ID != "sess-1" {
		t.Fatalf("original inference attempt provider session mutated: %#v", originalAttempt.ProviderSession)
	}
	if originalAttempt.Diagnostics.Provider.RequestMetadata["session_id"] != "sess-1" {
		t.Fatalf("original inference attempt diagnostics mutated: %#v", originalAttempt.Diagnostics.Provider.RequestMetadata)
	}
	if *originalAttempt.ExitCode != 7 {
		t.Fatalf("original inference attempt exit code mutated: %#v", originalAttempt.ExitCode)
	}

	if got := CloneFactoryWorldInferenceAttemptsByDispatchID(nil); got != nil {
		t.Fatalf("CloneFactoryWorldInferenceAttemptsByDispatchID(nil) = %#v, want nil", got)
	}
}

func TestSortedStringKeysAndAppendUniqueString(t *testing.T) {
	t.Parallel()

	if got := sortedStringKeys(nil); got != nil {
		t.Fatalf("sortedStringKeys(nil) = %#v, want nil", got)
	}

	keys := sortedStringKeys(map[string]WorkPayloadRef{
		"b": {},
		"a": {},
		"c": {},
	})
	if !reflect.DeepEqual(keys, []string{"a", "b", "c"}) {
		t.Fatalf("sortedStringKeys = %#v, want sorted keys", keys)
	}

	values := appendUniqueString([]string{"a"}, "a")
	if !reflect.DeepEqual(values, []string{"a"}) {
		t.Fatalf("appendUniqueString duplicate = %#v, want unchanged slice", values)
	}
	values = appendUniqueString(values, "b")
	if !reflect.DeepEqual(values, []string{"a", "b"}) {
		t.Fatalf("appendUniqueString new value = %#v, want appended slice", values)
	}
}

func TestWorkerWorkstationCompatibilityHelpers(t *testing.T) {
	t.Parallel()

	if !ExemptFromWorkerWorkstationCompatibility(FactoryWorkstationConfig{Type: WorkstationTypeLogical}) {
		t.Fatal("logical move workstation should be exempt")
	}
	if !ExemptFromWorkerWorkstationCompatibility(FactoryWorkstationConfig{Type: WorkstationTypeClassify}) {
		t.Fatal("classifier workstation should be exempt")
	}
	if ExemptFromWorkerWorkstationCompatibility(FactoryWorkstationConfig{Type: WorkstationTypeAgent}) {
		t.Fatal("agent workstation should not be exempt")
	}

	if got := EffectiveWorkstationTypeForCompatibility(FactoryWorkstationConfig{Type: "  "}); got != WorkstationTypeModel {
		t.Fatalf("blank standard workstation type = %q, want %q", got, WorkstationTypeModel)
	}
	if got := EffectiveWorkstationTypeForCompatibility(FactoryWorkstationConfig{Kind: WorkstationKindPoller}); got != "" {
		t.Fatalf("blank poller workstation type = %q, want empty", got)
	}
	if got := EffectiveWorkstationTypeForCompatibility(FactoryWorkstationConfig{Type: "  " + WorkstationTypeScript + "  "}); got != WorkstationTypeScript {
		t.Fatalf("trimmed workstation type = %q, want %q", got, WorkstationTypeScript)
	}

	for workerType, want := range map[string]WorkerWorkstationBehaviorClass{
		WorkerTypeInference: WorkerWorkstationBehaviorInference,
		WorkerTypeModel:     WorkerWorkstationBehaviorInference,
		WorkerTypeAgent:     WorkerWorkstationBehaviorAgent,
		WorkerTypeScript:    WorkerWorkstationBehaviorScript,
		WorkerTypePoller:    WorkerWorkstationBehaviorPoller,
		WorkerTypeHosted:    WorkerWorkstationBehaviorPoller,
	} {
		got, ok := WorkerBehaviorClass(workerType)
		if !ok || got != want {
			t.Fatalf("WorkerBehaviorClass(%q) = (%q, %v), want (%q, true)", workerType, got, ok, want)
		}
	}
	if got, ok := WorkerBehaviorClass("CUSTOM"); ok || got != "" {
		t.Fatalf("WorkerBehaviorClass(custom) = (%q, %v), want empty false", got, ok)
	}

	expected, ok := ExpectedWorkerBehaviorClassForWorkstation(
		FactoryWorkstationConfig{Type: WorkstationTypeInference},
		WorkerTypeInference,
	)
	if !ok || expected != WorkerWorkstationBehaviorInference {
		t.Fatalf("ExpectedWorkerBehaviorClassForWorkstation inference = (%q, %v), want (%q, true)", expected, ok, WorkerWorkstationBehaviorInference)
	}
	expected, ok = ExpectedWorkerBehaviorClassForWorkstation(
		FactoryWorkstationConfig{Type: WorkstationTypeModel},
		WorkerTypeScript,
	)
	if !ok || expected != WorkerWorkstationBehaviorScript {
		t.Fatalf("ExpectedWorkerBehaviorClassForWorkstation script alias = (%q, %v), want (%q, true)", expected, ok, WorkerWorkstationBehaviorScript)
	}
	expected, ok = ExpectedWorkerBehaviorClassForWorkstation(
		FactoryWorkstationConfig{Kind: WorkstationKindPoller},
		WorkerTypeHosted,
	)
	if !ok || expected != WorkerWorkstationBehaviorPoller {
		t.Fatalf("ExpectedWorkerBehaviorClassForWorkstation poller = (%q, %v), want (%q, true)", expected, ok, WorkerWorkstationBehaviorPoller)
	}
	if got, ok := ExpectedWorkerBehaviorClassForWorkstation(FactoryWorkstationConfig{Type: WorkstationTypeLogical}, WorkerTypeInference); ok || got != "" {
		t.Fatalf("ExpectedWorkerBehaviorClassForWorkstation logical = (%q, %v), want empty false", got, ok)
	}
}

func TestWorkerWorkstationCompatibilityBehaviorProjection(t *testing.T) {
	t.Parallel()

	if got := EffectiveWorkstationBehaviorClass("", WorkstationKindStandard, true); got != WorkstationTypeAgent {
		t.Fatalf("EffectiveWorkstationBehaviorClass blank standard = %q, want %q", got, WorkstationTypeAgent)
	}
	if got := EffectiveWorkstationBehaviorClass("", WorkstationKindPoller, true); got != WorkstationTypePoller {
		t.Fatalf("EffectiveWorkstationBehaviorClass blank poller = %q, want %q", got, WorkstationTypePoller)
	}
	if got := EffectiveWorkstationBehaviorClass("", WorkstationKindStandard, false); got != "" {
		t.Fatalf("EffectiveWorkstationBehaviorClass without worker = %q, want empty", got)
	}

	for _, tt := range []struct {
		workerType      string
		workstationType string
		kind            WorkstationKind
	}{
		{workerType: WorkerTypeModel, workstationType: WorkstationTypeModel},
		{workerType: WorkerTypeScript, workstationType: WorkstationTypeModel},
		{workerType: WorkerTypeInference, workstationType: "", kind: WorkstationKindStandard},
		{workerType: WorkerTypeHosted, workstationType: "", kind: WorkstationKindPoller},
	} {
		if !IsLegacyGrandfatheredWorkerWorkstationPair(tt.workerType, tt.workstationType, tt.kind) {
			t.Fatalf("IsLegacyGrandfatheredWorkerWorkstationPair(%q, %q, %q) = false, want true", tt.workerType, tt.workstationType, tt.kind)
		}
	}
	if IsLegacyGrandfatheredWorkerWorkstationPair(WorkerTypeAgent, WorkstationTypeInference, "") {
		t.Fatal("agent worker and inference workstation should not be grandfathered")
	}

	if !RequiresWorkerWorkstationBehaviorCompatibility(WorkstationTypeAgent, "", "worker-name") {
		t.Fatal("agent workstation with bound worker should require compatibility")
	}
	if RequiresWorkerWorkstationBehaviorCompatibility(WorkstationTypeLogical, "", "worker-name") {
		t.Fatal("logical workstation should not require compatibility")
	}
	if RequiresWorkerWorkstationBehaviorCompatibility(WorkstationTypeAgent, "", "") {
		t.Fatal("workstation without bound worker should not require compatibility")
	}

	if !CompatibleWorkerWorkstationBehavior(WorkerTypeModel, WorkstationTypeModel, "") {
		t.Fatal("legacy model worker/model workstation should be compatible")
	}
	if !CompatibleWorkerWorkstationBehavior("", WorkstationTypeAgent, "") {
		t.Fatal("empty worker type should be treated as compatible")
	}
	if CompatibleWorkerWorkstationBehavior(WorkerTypeAgent, WorkstationTypeInference, "") {
		t.Fatal("agent worker and inference workstation should not be compatible")
	}

	if got := RuntimeBehaviorClassLabel(WorkerTypeInference); got != "inference" {
		t.Fatalf("RuntimeBehaviorClassLabel inference = %q, want inference", got)
	}
	if got := RuntimeBehaviorClassLabel("  CUSTOM_BEHAVIOR  "); got != "custom_behavior" {
		t.Fatalf("RuntimeBehaviorClassLabel custom = %q, want custom_behavior", got)
	}

	if got := WorkerWorkstationBehaviorMismatchMessage("review", "", WorkstationKindPoller, "planner", ""); got == "" {
		t.Fatal("WorkerWorkstationBehaviorMismatchMessage returned empty string")
	}

	if got := PublicWorkerTypeForFactoryUsage(
		WorkerConfig{Name: "", Type: WorkerTypeModel},
		[]FactoryWorkstationConfig{{Type: WorkstationTypeAgent, WorkerTypeName: "executor"}},
	); got != WorkerTypeInference {
		t.Fatalf("model worker without name = %q, want %q", got, WorkerTypeInference)
	}
	if got := PublicWorkerTypeForFactoryUsage(WorkerConfig{Name: "executor", Type: WorkerTypeAgent}, nil); got != WorkerTypeAgent {
		t.Fatalf("non-model worker type projection = %q, want %q", got, WorkerTypeAgent)
	}
}
