//go:build functionallong

package replay_contracts

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestWorkerPublicContractSmoke_CanonicalWorkerExecutesAndKeepsRuntimeOnlyFieldsPrivate(t *testing.T) {
	support.SkipLongFunctional(t, "slow worker public-contract replay sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	support.WriteAgentConfig(t, dir, "worker-a", workerPublicContractSmokeWorkerConfig())
	support.WriteAgentConfig(t, dir, "worker-b", workerPublicContractSmokeWorkerConfig())

	loaded, err := factoryconfig.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	assertWorkerPublicContractInternalRuntime(t, loaded, "worker-a")

	flattened, err := factoryconfig.FlattenFactoryConfig(dir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}
	assertWorkerPublicContractPublicJSON(t, flattened)
	flattenedFactory, err := factoryconfig.GeneratedFactoryFromOpenAPIJSON(flattened)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(flattened): %v", err)
	}
	assertWorkerPublicContractPublicFactory(t, flattenedFactory, "worker-a")
	assertWorkerPublicContractPublicRuntime(t, flattenedFactory, "worker-a")

	artifactPath := filepath.Join(t.TempDir(), "worker-public-contract-smoke.replay.json")
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		Name:       "worker-public-contract-smoke",
		WorkID:     "work-worker-public-contract-smoke",
		WorkTypeID: "task",
		TraceID:    "trace-worker-public-contract-smoke",
		Payload:    []byte(`{"title":"worker public contract smoke"}`),
	})
	runner := testutil.NewProviderCommandRunner(
		workers.CommandResult{Stdout: []byte("Step one done. COMPLETE")},
		workers.CommandResult{Stdout: []byte("Done. COMPLETE")},
	)
	harness := testutil.NewServiceTestHarness(t, dir,
		testutil.WithProviderCommandRunner(runner),
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithRecordPath(artifactPath),
	)

	harness.RunUntilComplete(t, 10*time.Second)
	harness.Assert().
		PlaceTokenCount("task:complete", 1).
		HasNoTokenInPlace("task:init").
		HasNoTokenInPlace("task:failed")
	assertWorkerPublicContractProviderRequest(t, runner)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	runStarted := requireFactoryOnlyRunStartedPayload(t, artifact.Events)
	runStartedJSON, err := json.Marshal(runStarted.Factory)
	if err != nil {
		t.Fatalf("marshal run-started factory: %v", err)
	}
	assertWorkerPublicContractPublicJSON(t, runStartedJSON)
	assertWorkerPublicContractPublicFactory(t, runStarted.Factory, "worker-a")
	assertWorkerPublicContractPublicRuntime(t, runStarted.Factory, "worker-a")

	flattenedWorker := requireWorkerPublicContractWorker(t, flattenedFactory, "worker-a")
	recordedWorker := requireWorkerPublicContractWorker(t, runStarted.Factory, "worker-a")
	if stringPointerValue(recordedWorker.ExecutorProvider) != stringPointerValue(flattenedWorker.ExecutorProvider) ||
		stringPointerValue(recordedWorker.ModelProvider) != stringPointerValue(flattenedWorker.ModelProvider) ||
		stringPointerValue(recordedWorker.StopToken) != stringPointerValue(flattenedWorker.StopToken) {
		t.Fatalf("flattened and recorded public worker payloads diverged\nflattened: %#v\nrecorded:  %#v", flattenedWorker, recordedWorker)
	}
}

func assertWorkerPublicContractInternalRuntime(
	t *testing.T,
	loaded *factoryconfig.LoadedFactoryConfig,
	workerName string,
) {
	t.Helper()

	worker, ok := loaded.Worker(workerName)
	if !ok {
		t.Fatalf("expected worker %q in runtime config", workerName)
	}
	if worker.ExecutorProvider != "script_wrap" {
		t.Fatalf("executor provider = %q, want script_wrap", worker.ExecutorProvider)
	}
	if worker.ModelProvider != string(workers.ModelProviderClaude) {
		t.Fatalf("model provider = %q, want %q", worker.ModelProvider, workers.ModelProviderClaude)
	}
	if worker.SessionID != "" {
		t.Fatalf("session id = %q, want empty runtime-owned field", worker.SessionID)
	}
	if worker.Concurrency != 0 {
		t.Fatalf("concurrency = %d, want runtime-owned field to stay empty", worker.Concurrency)
	}
}

func assertWorkerPublicContractPublicRuntime(t *testing.T, generated factoryapi.Factory, workerName string) {
	t.Helper()

	runtimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(generated)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	worker, ok := runtimeCfg.Worker(workerName)
	if !ok {
		t.Fatalf("expected worker %q in generated runtime config", workerName)
	}
	if worker.ExecutorProvider != "script_wrap" {
		t.Fatalf("generated runtime executor provider = %q, want script_wrap", worker.ExecutorProvider)
	}
	if worker.ModelProvider != string(workers.ModelProviderClaude) {
		t.Fatalf("generated runtime model provider = %q, want %q", worker.ModelProvider, workers.ModelProviderClaude)
	}
	if worker.SessionID != "" {
		t.Fatalf("generated runtime session id = %q, want empty", worker.SessionID)
	}
	if worker.Concurrency != 0 {
		t.Fatalf("generated runtime concurrency = %d, want 0", worker.Concurrency)
	}
}

func assertWorkerPublicContractProviderRequest(t *testing.T, runner *testutil.ProviderCommandRunner) {
	t.Helper()

	if runner.CallCount() != 2 {
		t.Fatalf("provider command runner call count = %d, want 2", runner.CallCount())
	}
	req := runner.Requests()[0]
	if req.Command != string(workers.ModelProviderClaude) {
		t.Fatalf("provider command = %q, want %q", req.Command, workers.ModelProviderClaude)
	}
	for _, arg := range req.Args {
		if arg == "--resume" {
			t.Fatalf("provider args should not include runtime-owned resume flag: %#v", req.Args)
		}
	}
	support.AssertArgsContainSequence(t, req.Args, []string{"--model", "claude-sonnet-4-20250514"})
	if !strings.Contains(strings.Join(req.Args, " "), "claude-sonnet-4-20250514") {
		t.Fatalf("provider args = %#v, want canonical model selection", req.Args)
	}
}

func assertWorkerPublicContractPublicFactory(t *testing.T, generated factoryapi.Factory, workerName string) {
	t.Helper()

	worker := requireWorkerPublicContractWorker(t, generated, workerName)
	if stringPointerValue(worker.ExecutorProvider) != string(factoryapi.WorkerProviderScriptWrap) {
		t.Fatalf("public worker executorProvider = %q, want %q", stringPointerValue(worker.ExecutorProvider), factoryapi.WorkerProviderScriptWrap)
	}
	if stringPointerValue(worker.ModelProvider) != string(factoryapi.WorkerModelProviderClaude) {
		t.Fatalf("public worker modelProvider = %q, want %q", stringPointerValue(worker.ModelProvider), factoryapi.WorkerModelProviderClaude)
	}
	if stringPointerValue(worker.Type) != interfaces.WorkerTypeModel {
		t.Fatalf("public worker type = %q, want %q", stringPointerValue(worker.Type), interfaces.WorkerTypeModel)
	}
	if stringPointerValue(worker.StopToken) != "COMPLETE" {
		t.Fatalf("public worker stopToken = %q, want COMPLETE", stringPointerValue(worker.StopToken))
	}
}

func requireWorkerPublicContractWorker(t *testing.T, factory factoryapi.Factory, name string) factoryapi.Worker {
	t.Helper()

	if factory.Workers == nil {
		t.Fatal("public factory workers = nil")
	}
	for _, worker := range *factory.Workers {
		if worker.Name == name {
			return worker
		}
	}
	t.Fatalf("public factory workers = %#v, want worker %q", *factory.Workers, name)
	return factoryapi.Worker{}
}

func assertWorkerPublicContractPublicJSON(t *testing.T, data []byte) {
	t.Helper()

	text := string(data)
	for _, required := range []string{`"executorProvider"`, `"modelProvider"`} {
		if !strings.Contains(text, required) {
			t.Fatalf("public worker payload missing canonical key %s: %s", required, text)
		}
	}
	for _, forbidden := range []string{`"provider"`, `"sessionId"`, `"concurrency"`} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("public worker payload contains retired key %s: %s", forbidden, text)
		}
	}
}

func workerPublicContractSmokeWorkerConfig() string {
	return `---
type: MODEL_WORKER
model: claude-sonnet-4-20250514
executorProvider: script_wrap
modelProvider: claude
stopToken: COMPLETE
---
Execute the task.
`
}
