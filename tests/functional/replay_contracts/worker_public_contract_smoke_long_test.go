//go:build functionallong

package replay_contracts

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestWorkerPublicContractSmoke_CanonicalWorkerExecutesAndKeepsRuntimeOnlyFieldsPrivate(t *testing.T) {
	support.SkipLongFunctional(t, "slow worker public-contract replay sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	support.WriteAgentConfig(t, dir, "worker-a", workerPublicContractSmokeWorkerConfig())
	support.WriteAgentConfig(t, dir, "worker-b", workerPublicContractSmokeWorkerConfig())

	flattened, err := support.FlattenFactoryConfig(t, dir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}
	assertWorkerPublicContractPublicJSON(t, flattened)
	flattenedFactory, err := factorymapping.GeneratedFactoryFromOpenAPIJSON(flattened)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(flattened): %v", err)
	}
	assertWorkerPublicContractPublicFactory(t, flattenedFactory, "worker-a")

	artifactPath := filepath.Join(t.TempDir(), "worker-public-contract-smoke.replay.json")
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		Name:       "worker-public-contract-smoke",
		WorkID:     "work-worker-public-contract-smoke",
		WorkTypeID: "task",
		TraceID:    "trace-worker-public-contract-smoke",
		Payload:    []byte(`{"title":"worker public contract smoke"}`),
	})
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte(`{"type":"result","subtype":"success","is_error":false,"result":"Step one done. COMPLETE","session_id":"worker-contract-1"}` + "\n")},
		platformprocess.CommandResult{Stdout: []byte(`{"type":"result","subtype":"success","is_error":false,"result":"Done. COMPLETE","session_id":"worker-contract-2"}` + "\n")},
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Args:       []string{"--record", artifactPath},
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	assertReplaySessionPlaces(t, support.ListDefaultSessionWork(t, server.URL()), map[string]int{
		"task:complete": 1, "task:init": 0, "task:failed": 0,
	})
	assertWorkerPublicContractProviderRequest(t, runner)
	server.Stop(t)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	runStarted := requireFactoryOnlyRunStartedPayload(t, testutil.GeneratedFactoryEvents(t, artifact.Events))
	runStartedJSON, err := json.Marshal(runStarted.Factory)
	if err != nil {
		t.Fatalf("marshal run-started factory: %v", err)
	}
	assertWorkerPublicContractPublicJSON(t, runStartedJSON)
	assertWorkerPublicContractPublicFactory(t, runStarted.Factory, "worker-a")

	flattenedWorker := requireWorkerPublicContractWorker(t, flattenedFactory, "worker-a")
	recordedWorker := requireWorkerPublicContractWorker(t, runStarted.Factory, "worker-a")
	if stringPointerValue(recordedWorker.ExecutorProvider) != stringPointerValue(flattenedWorker.ExecutorProvider) ||
		stringPointerValue(recordedWorker.ModelProvider) != stringPointerValue(flattenedWorker.ModelProvider) ||
		stringPointerValue(recordedWorker.StopToken) != stringPointerValue(flattenedWorker.StopToken) {
		t.Fatalf("flattened and recorded public worker payloads diverged\nflattened: %#v\nrecorded:  %#v", flattenedWorker, recordedWorker)
	}
}

func assertWorkerPublicContractProviderRequest(t *testing.T, runner *testutil.ProviderCommandRunner) {
	t.Helper()

	if runner.CallCount() != 2 {
		t.Fatalf("provider command runner call count = %d, want 2", runner.CallCount())
	}
	req := runner.Requests()[0]
	if req.Command != string(modelprovider.ProviderClaude) {
		t.Fatalf("provider command = %q, want %q", req.Command, modelprovider.ProviderClaude)
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
	if stringPointerValue(worker.Type) != interfaces.WorkerTypeAgent {
		t.Fatalf("public worker type = %q, want %q", stringPointerValue(worker.Type), interfaces.WorkerTypeAgent)
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
