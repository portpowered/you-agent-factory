package mock

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const plainBatchDrainTestTimeout = 15 * time.Second

// plainBatchScenario is the isolation scope for one no-server invocation.
// These rows intentionally retain the local CLI's invocation-scoped default
// Factory Session because the public no-server command has no session selector.
// The scope still gives every row a unique Factory, selector, request/trace,
// cancellation gate, runtime identity, HOME, and input/config path.
type plainBatchScenario struct {
	factoryDir      string
	homeDir         string
	factoryName     string
	workerName      string
	workstationName string
	requestID       string
	traceID         string
	workName        string
}

func newPlainBatchScenario(t *testing.T) *plainBatchScenario {
	t.Helper()
	identity := uuid.NewString()
	scenario := &plainBatchScenario{
		homeDir:         t.TempDir(),
		factoryName:     "plain-batch-factory-" + identity,
		workerName:      "plain-batch-worker-" + identity,
		workstationName: "plain-batch-process-" + identity,
		requestID:       "plain-batch-request-" + identity,
		traceID:         "plain-batch-trace-" + identity,
		workName:        "plain-batch-work-" + identity,
	}
	scenario.factoryDir = scaffoldPlainBatchDrainFactory(t, scenario)
	return scenario
}

// testPlainBatchDrainReportsStrandedWork proves the no-server customer path
// returns the canonical incomplete-drain diagnostic after a deterministic mock
// worker completes its dispatch but leaves Work in PROCESSING. It runs after
// the shared host has been stopped so this one-shot activation reuses the same
// root without overlapping the host runtime.
func testPlainBatchDrainReportsStrandedWork(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	fixture.prepareLocalActivation(t)
	scenario := newPlainBatchScenario(t)
	workFile := writePlainBatchDrainWork(t, scenario)
	mockWorkersFile := writePlainBatchDrainMockWorkers(t, scenario)

	inputs := plainBatchInputs(t, scenario, workFile, false)
	inputs.Input.Args = append(inputs.Input.Args, "--with-mock-workers", mockWorkersFile)

	command := support.StartProcessCommand(t, &sharedWorkersMockLocalProcess{
		fixture: fixture,
		tb:      t,
	}, inputs.Input)
	// ProcessCommand.Done is the deterministic completion signal. This bounded
	// guard exists only to turn the original plain-batch hang into a test
	// failure rather than leaving the suite blocked indefinitely.
	timer := time.NewTimer(plainBatchDrainTestTimeout)
	defer timer.Stop()
	select {
	case <-command.Done():
	case <-timer.C:
		t.Fatal("plain finite batch did not return after dispatch activity drained")
	}
	command.AcceptError()

	err := command.Err()
	if err == nil {
		t.Fatalf("Process.Execute() error = %v, want incomplete-drain failure", err)
	}
	support.RequireSafeCLIDiagnostic(t, inputs.Stderr())
	if stdout := inputs.Stdout(); stdout != "" {
		t.Fatalf("stdout = %q, want no success or completion output", stdout)
	}
}

func plainBatchInputs(
	t *testing.T,
	scenario *plainBatchScenario,
	workFile string,
	continuous bool,
) *support.CapturedInputs {
	t.Helper()
	args := []string{"you", "run", "--dir", scenario.factoryDir, "--no-record", "--quiet"}
	if continuous {
		args = append(args, "--continuously")
	}
	if workFile != "" {
		args = append(args, "--work", workFile)
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.WorkingDirectory = scenario.factoryDir
	inputs.Env = append(os.Environ(), "HOME="+scenario.homeDir, "USERPROFILE="+scenario.homeDir)
	return inputs
}

func scaffoldPlainBatchDrainFactory(t *testing.T, scenario *plainBatchScenario) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"name": scenario.factoryName,
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": scenario.workerName}},
		"workstations": []map[string]any{{
			"name":      scenario.workstationName,
			"worker":    scenario.workerName,
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "processing"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	})
	support.WriteAgentConfig(t, dir, scenario.workerName, support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir
}

func writePlainBatchDrainWork(t *testing.T, scenario *plainBatchScenario) string {
	return writePlainBatchDrainWorkState(t, scenario, "init")
}

func writePlainBatchDrainWorkState(t *testing.T, scenario *plainBatchScenario, state string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plain-batch-drain-work.json")
	payload := map[string]any{
		"requestId": scenario.requestID,
		"type":      "FACTORY_REQUEST_BATCH",
		"works": []map[string]any{{
			"name":         scenario.workName,
			"workTypeName": "task",
			"state":        state,
			"traceId":      scenario.traceID,
			"payload":      map[string]string{"purpose": "plain drain regression"},
		}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal plain-batch Work: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write plain-batch Work: %v", err)
	}
	return path
}

func writePlainBatchDrainMockWorkers(t *testing.T, scenario *plainBatchScenario) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plain-batch-drain-mock-workers.json")
	payload := workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      scenario.workerName,
			WorkstationName: scenario.workstationName,
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal plain-batch mock workers: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("write plain-batch mock workers: %v", err)
	}
	return path
}
