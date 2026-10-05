package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestInterruptCapturedRecipePreservesExecutionAndReference(t *testing.T) {
	t.Parallel()
	r, plan, store := newDurableInterruptFixture(t)
	plan.execution.Execution.ProcessEnvironment = []string{"API_TOKEN=private-inherited-credential", "PATH=private-inherited-path"}
	plan.execution.Execution.Model = "accepted-model"
	plan.execution.Execution.Args = []string{"--accepted-option"}
	// Identical member names in separate objects remain valid token content.
	plan.execution.Execution.InputTokens = []any{map[string]any{"key": "first"}, []any{map[string]any{"key": "second"}}}
	plan.request.ReplacementMessage = "exact\nreplacement\" bytes"
	operation, err := r.beginInterruptIntent(t.Context(), plan)
	if err != nil {
		t.Fatal(err)
	}
	var input durableInterruptInput
	if err := json.Unmarshal(store.input, &input); err != nil {
		t.Fatal(err)
	}
	if !validInterruptInput(input, plan.request, plan.dispatchID) || input.ProviderReference.ID != plan.reference.ID || input.Execution.Execution.Model != "accepted-model" || !reflect.DeepEqual(input.Execution.Execution.Args, plan.execution.Execution.Args) || !reflect.DeepEqual(input.Execution.Execution.InputTokens, plan.execution.Execution.InputTokens) {
		t.Fatalf("accepted recipe lost execution facts: %#v", input)
	}
	if strings.Contains(string(store.input), "private-inherited") || len(input.Execution.Execution.ProcessEnvironment) != 0 {
		t.Fatal("inherited environment reached the durable recipe")
	}
	assertInterruptInputSchema(t, store.input)
	result := r.interruptResultSnapshot(plan.request, workersessions.InterruptPhaseValidation, false)
	if err := r.commitInterruptResult(t.Context(), operation, result, workersessions.ErrInterruptSourceConflict); err != nil {
		t.Fatal(err)
	}
	plan.execution.Execution.Args[0] = "changed-after-acceptance"
	plan.reference.ID = "changed-after-acceptance"
	replayed, found, err := r.replayDurableInterrupt(t.Context(), plan.request)
	if !found || !errors.Is(err, workersessions.ErrInterruptSourceConflict) || !reflect.DeepEqual(replayed, result) {
		t.Fatalf("captured recipe replay=%#v found=%v err=%v", replayed, found, err)
	}
}

// The contract subject is the actual persisted recipe, not a file inventory.
func assertInterruptInputSchema(t *testing.T, payload []byte) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "recordings", "internal", "services", "worker_capture", "schemas", "control-input.v1.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var schemaDocument, inputDocument any
	if err := json.Unmarshal(data, &schemaDocument); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, &inputDocument); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("control-input.v1.schema.json", schemaDocument); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("control-input.v1.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(inputDocument); err != nil {
		t.Fatalf("persisted recipe violates C5: %v", err)
	}
}

func TestInterruptUnsafeRecipeRefusesBeforeSourceCancellation(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"env-override", "sensitive-prompt", "fail-closed", "secret-argument", "escaped-secret-token", "secret-reference", "secret-key", "non-json-token"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			r, plan, store := newDurableInterruptFixture(t)
			configureUnsafeInterruptRecipe(&plan, scenario)
			calls := 0
			plan.supervision.installCancel(func() { calls++ })
			result, err := r.runInterrupt(plan)
			if !errors.Is(err, recordings.ErrInvalidRecordingRedactionRequest) || result.Phase != workersessions.InterruptPhaseValidation || result.Source.State != workersessions.StateRunning || calls != 0 {
				t.Fatalf("unsafe recipe result=%#v err=%v effects=%d", result, err, calls)
			}
			if len(store.input) != 0 || len(store.records) != 0 || strings.Contains(err.Error(), "private-recipe") {
				t.Fatal("unsafe recipe was persisted or leaked in diagnostics")
			}
			assertNoSuccessor(t, r, "successor")
		})
	}
}

func configureUnsafeInterruptRecipe(plan *interruptPlan, scenario string) {
	secret := "private-recipe\nsecret\""
	plan.execution.Execution.ProcessEnvironment = []string{"API_TOKEN=" + secret}
	switch scenario {
	case "env-override":
		plan.execution.Execution.EnvVars = map[string]string{"CUSTOM_VALUE": "private-recipe-override"}
	case "sensitive-prompt":
		plan.execution.Execution.PromptRedaction = &workers.PromptRedaction{RedactSystemPrompt: true}
	case "fail-closed":
		plan.execution.Execution.PromptRedaction = &workers.PromptRedaction{FailClosed: true}
	case "secret-argument":
		plan.execution.Execution.Args = []string{"prefix " + secret}
	case "escaped-secret-token":
		plan.execution.Execution.InputTokens = []any{map[string]any{"nested": []any{secret}}}
	case "secret-reference":
		plan.reference.ID = secret
	case "secret-key":
		plan.execution.Execution.InputTokens = []any{map[string]any{secret: "value"}}
	case "non-json-token":
		plan.execution.Execution.InputTokens = []any{make(chan struct{})}
	}
}

func TestInterruptCapturedRecipeRejectsCorruptionAndRetainsLegacyReplay(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"legacy", "version", "replacement", "attempt", "reference", "workstation", "env", "unknown", "trailing", "model-tamper", "duplicate-version", "duplicate-reference", "duplicate-token-key"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			r, plan, store := newDurableInterruptFixture(t)
			operation, err := r.beginInterruptIntent(t.Context(), plan)
			if err != nil {
				t.Fatal(err)
			}
			result := r.interruptResultSnapshot(plan.request, workersessions.InterruptPhaseValidation, false)
			if err := r.commitInterruptResult(t.Context(), operation, result, workersessions.ErrInterruptSourceConflict); err != nil {
				t.Fatal(err)
			}
			mutateInterruptRecipe(store, plan, scenario)
			calls := 0
			plan.supervision.installCancel(func() { calls++ })
			replayed, found, replayErr := r.replayDurableInterrupt(t.Context(), plan.request)
			want := recordings.ErrWorkerRecordingPersistence
			switch scenario {
			case "legacy":
				want = workersessions.ErrInterruptSourceConflict
			case "replacement":
				want = workersessions.ErrInterruptRequestIDConflict
			}
			if !found || !errors.Is(replayErr, want) || replayed.Accepted || calls != 0 || len(store.records) != 2 {
				t.Fatalf("recipe replay=%#v found=%v err=%v effects=%d", replayed, found, replayErr, calls)
			}
			assertNoSuccessor(t, r, "successor")
		})
	}
}

func mutateInterruptRecipe(store *interruptInputStore, plan interruptPlan, scenario string) {
	if strings.HasPrefix(scenario, "duplicate-") {
		switch scenario {
		case "duplicate-version":
			store.input = bytes.Replace(store.input, []byte(`"version":1`), []byte(`"version":2,"version":1`), 1)
		case "duplicate-reference":
			store.input = bytes.Replace(store.input, []byte(`"id":"exact-provider-session"`), []byte(`"id":"private-recipe-secret","id":"exact-provider-session"`), 1)
		case "duplicate-token-key":
			var document map[string]json.RawMessage
			_ = json.Unmarshal(store.input, &document)
			var execution map[string]json.RawMessage
			_ = json.Unmarshal(document["execution"], &execution)
			var request map[string]json.RawMessage
			_ = json.Unmarshal(execution["Execution"], &request)
			request["input_tokens"] = json.RawMessage(`[{"key":"private-recipe-secret","key":"safe"}]`)
			execution["Execution"], _ = json.Marshal(request)
			document["execution"], _ = json.Marshal(execution)
			store.input, _ = json.Marshal(document)
		}
		updateInterruptRecipeDigest(store)
		return
	}
	var input durableInterruptInput
	_ = json.Unmarshal(store.input, &input)
	switch scenario {
	case "legacy":
		store.input, _ = json.Marshal(plan.request)
		digest := sha256.Sum256(store.input)
		store.records[len(store.records)-1].Operation.InputDigest = hex.EncodeToString(digest[:])
		return
	case "version":
		input.Version++
	case "replacement":
		input.ReplacementMessage = "different replacement"
	case "attempt":
		input.Execution.Execution.Dispatch.DispatchID = "other-attempt"
	case "reference":
		input.ProviderReference.ID = ""
	case "workstation":
		input.Execution.WorkstationName = "different-workstation"
	case "env":
		input.Execution.Execution.EnvVars = map[string]string{"TOKEN": "private-recipe-secret"}
	case "unknown":
		store.input = append(store.input[:len(store.input)-1], []byte(`,"privateFutureField":"private-recipe-secret"}`)...)
		updateInterruptRecipeDigest(store)
		return
	case "trailing":
		store.input = append(store.input, []byte(` {}`)...)
		updateInterruptRecipeDigest(store)
		return
	case "model-tamper":
		input.Execution.Execution.Model = "different-model"
		store.input, _ = json.Marshal(input)
		return
	}
	store.input, _ = json.Marshal(input)
	updateInterruptRecipeDigest(store)
}

func updateInterruptRecipeDigest(store *interruptInputStore) {
	digest := sha256.Sum256(store.input)
	store.records[len(store.records)-1].Operation.InputDigest = hex.EncodeToString(digest[:])
}
