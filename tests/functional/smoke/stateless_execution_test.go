package smoke

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

type dispatchRecorder struct {
	mu         sync.Mutex
	dispatches []interfaces.WorkDispatch
}

func (r *dispatchRecorder) Execute(_ context.Context, dispatch interfaces.WorkDispatch) (interfaces.WorkResult, error) {
	r.mu.Lock()
	r.dispatches = append(r.dispatches, dispatch)
	r.mu.Unlock()

	return interfaces.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      interfaces.OutcomeAccepted,
	}, nil
}

func (r *dispatchRecorder) Dispatches() []interfaces.WorkDispatch {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]interfaces.WorkDispatch, len(r.dispatches))
	copy(out, r.dispatches)
	return out
}

func TestStatelessExecution_SharedExecutorResolvesDifferentWorkstations(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "stateless_collector"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item":"shared-executor"}`))

	provider := testutil.NewMockProvider(
		interfaces.InferenceResponse{Content: "Stage 1 done. COMPLETE"},
		interfaces.InferenceResponse{Content: "Stage 2 done. COMPLETE"},
	)
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:failed")

	calls := provider.Calls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 provider calls, got %d", len(calls))
	}

	if calls[0].WorkstationType != "step1" {
		t.Fatalf("expected first call workstation step1, got %q", calls[0].WorkstationType)
	}
	if calls[1].WorkstationType != "step2" {
		t.Fatalf("expected second call workstation step2, got %q", calls[1].WorkstationType)
	}

	if calls[0].Model != "test-model" || calls[1].Model != "test-model" {
		t.Fatalf("expected both calls to resolve model test-model, got %q and %q", calls[0].Model, calls[1].Model)
	}

	if !strings.Contains(calls[0].UserMessage, "Step 1 workstation.") {
		t.Fatalf("expected first call user message to contain step1 prompt, got %q", calls[0].UserMessage)
	}
	if !strings.Contains(calls[1].UserMessage, "Step 2 workstation.") {
		t.Fatalf("expected second call user message to contain step2 prompt, got %q", calls[1].UserMessage)
	}
}

func TestStatelessExecution_ThinDispatchCarriesLookupReferencesOnly(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "stateless_collector"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item":"thin-dispatch"}`))

	recorder := &dispatchRecorder{}
	h := testutil.NewServiceTestHarness(t, dir)
	h.SetCustomExecutor("agent", recorder)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:failed")

	dispatches := recorder.Dispatches()
	if len(dispatches) != 2 {
		t.Fatalf("expected 2 raw dispatches, got %d", len(dispatches))
	}

	for i, dispatch := range dispatches {
		if dispatch.WorkerType != "agent" {
			t.Fatalf("dispatch %d: expected worker type agent, got %q", i, dispatch.WorkerType)
		}
		if dispatch.WorkstationName == "" {
			t.Fatalf("dispatch %d: expected workstation name for runtime lookup", i)
		}
		if len(dispatch.InputTokens) == 0 {
			t.Fatalf("dispatch %d: expected input tokens for runtime resolution", i)
		}
	}

	if dispatches[0].WorkstationName != "step1" {
		t.Fatalf("expected first raw dispatch workstation step1, got %q", dispatches[0].WorkstationName)
	}
	if dispatches[1].WorkstationName != "step2" {
		t.Fatalf("expected second raw dispatch workstation step2, got %q", dispatches[1].WorkstationName)
	}
}

func TestStatelessExecution_DifferentWorkstationsResolveDifferentWorkers(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "stateless_collector"))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"item":"different-workers"}`))

	rewriteStatelessCollectorForDifferentWorkers(t, dir)

	work := map[string][]interfaces.InferenceResponse{
		"agent-a": {{Content: "Stage 1 done. COMPLETE"}},
		"agent-b": {{Content: "Stage 2 done. COMPLETE"}},
	}
	provider := testutil.NewMockWorkerMapProvider(work)

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProvider(provider),
		testutil.WithFullWorkerPoolAndScriptWrap(),
	)

	h.RunUntilComplete(t, 10*time.Second)

	h.Assert().
		PlaceTokenCount("task:done", 1).
		HasNoTokenInPlace("task:failed")

	if provider.CallCount("agent-a") != 1 {
		t.Fatalf("expected agent-a called 1 time, got %d", provider.CallCount("agent-a"))
	}
	if provider.CallCount("agent-b") != 1 {
		t.Fatalf("expected agent-b called 1 time, got %d", provider.CallCount("agent-b"))
	}

	first := provider.Calls("agent-a")[0]
	second := provider.Calls("agent-b")[0]
	if first.WorkstationType != "step1" {
		t.Fatalf("expected agent-a workstation step1, got %q", first.WorkstationType)
	}
	if second.WorkstationType != "step2" {
		t.Fatalf("expected agent-b workstation step2, got %q", second.WorkstationType)
	}
	if !strings.Contains(first.UserMessage, "Step 1 workstation.") {
		t.Fatalf("expected agent-a prompt from step1 workstation, got %q", first.UserMessage)
	}
	if !strings.Contains(second.UserMessage, "Step 2 workstation.") {
		t.Fatalf("expected agent-b prompt from step2 workstation, got %q", second.UserMessage)
	}
}

func rewriteStatelessCollectorForDifferentWorkers(t *testing.T, dir string) {
	t.Helper()

	factoryPath := filepath.Join(dir, "factory.json")
	data, err := os.ReadFile(factoryPath)
	if err != nil {
		t.Fatalf("read factory.json: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("unmarshal factory.json: %v", err)
	}

	cfg["workers"] = []map[string]any{
		{"name": "agent-a"},
		{"name": "agent-b"},
	}

	workstations := cfg["workstations"].([]any)
	workstations[0].(map[string]any)["worker"] = "agent-a"
	workstations[1].(map[string]any)["worker"] = "agent-b"

	updated, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal factory.json: %v", err)
	}
	if err := os.WriteFile(factoryPath, updated, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	source := filepath.Join(dir, "workers", "agent", "AGENTS.md")
	agentConfig, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("read worker AGENTS.md: %v", err)
	}

	for _, workerName := range []string{"agent-a", "agent-b"} {
		workerDir := filepath.Join(dir, "workers", workerName)
		if err := os.MkdirAll(workerDir, 0o755); err != nil {
			t.Fatalf("create worker dir %s: %v", workerName, err)
		}
		if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), agentConfig, 0o644); err != nil {
			t.Fatalf("write worker AGENTS.md %s: %v", workerName, err)
		}
	}

	if err := os.RemoveAll(filepath.Join(dir, "workers", "agent")); err != nil {
		t.Fatalf("remove original worker dir: %v", err)
	}
}
