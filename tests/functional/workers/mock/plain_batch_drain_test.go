package mock

import (
	"context"
	"encoding/json"
	"errors"
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
const plainBatchContinuousIdleObservation = 500 * time.Millisecond

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
	gateID          string
	runtimeID       string
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
		gateID:          "plain-batch-gate-" + identity,
		runtimeID:       "plain-batch-runtime-" + identity,
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

func testPlainBatchDrainPreservesFiniteAndContinuousCounterexamples(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	fixture.prepareLocalActivation(t)

	for _, scenario := range []struct {
		name    string
		hasWork bool
	}{
		{name: "empty"},
		{name: "terminal work", hasWork: true},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			invocation := newPlainBatchScenario(t)
			var workFile string
			if scenario.hasWork {
				workFile = writePlainBatchDrainWorkState(t, invocation, "complete")
			}
			inputs := plainBatchInputs(t, invocation, workFile, false)

			if err := fixture.executeLocal(t, inputs.Input); err != nil {
				t.Fatalf("finite plain batch error = %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
			}
			wantStdout := ""
			if scenario.hasWork {
				wantStdout = "Batch completed successfully.\n"
			}
			if inputs.Stdout() != wantStdout || inputs.Stderr() != "" {
				t.Fatalf("finite plain success output = stdout:%q stderr:%q, want stdout:%q and quiet stderr", inputs.Stdout(), inputs.Stderr(), wantStdout)
			}
		})
	}

	t.Run("continuous idle", func(t *testing.T) {
		invocation := newPlainBatchScenario(t)
		inputs := plainBatchInputs(t, invocation, "", true)
		command := support.StartProcessCommand(t, &sharedWorkersMockLocalProcess{
			fixture: fixture,
			tb:      t,
		}, inputs.Input)

		// Process.Execute exposes no public idle event for a continuous plain
		// run, and this empty scenario has no edge callback that can certify
		// idleness. A bounded observation is therefore required for this
		// negative-liveness assertion: Done must remain open while the run is
		// idle; without it a regression would leave the test blocked forever.
		idleTimer := time.NewTimer(plainBatchContinuousIdleObservation)
		defer idleTimer.Stop()
		select {
		case <-command.Done():
			command.AcceptError()
			t.Fatalf("continuous plain batch exited while idle: err=%v stdout=%q stderr=%q", command.Err(), inputs.Stdout(), inputs.Stderr())
		case <-idleTimer.C:
		}
		command.Stop(t)
		if err := command.Err(); err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("continuous plain batch cancellation error = %v", err)
		}
		if inputs.Stdout() != "" {
			t.Fatalf("continuous plain output = stdout:%q, want quiet output", inputs.Stdout())
		}
		if stderr := inputs.Stderr(); stderr != "" && stderr != "Error: context canceled\n" {
			t.Fatalf("continuous plain stderr = %q, want empty or the cancellation diagnostic", stderr)
		}
	})
}

func testPlainBatchDrainRejectsCancellationBeforeRuntimeActivation(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	fixture.prepareLocalActivation(t)
	scenario := newPlainBatchScenario(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputs := plainBatchInputs(t, scenario, "", true)
	inputs.Input.Context = ctx

	fixture.sessionIDGenerator.armCancellation(cancel, scenario.runtimeID)

	if err := fixture.executeLocal(t, inputs.Input); err != nil {
		t.Fatalf("canceled continuous plain batch error = %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	if got := fixture.sessionIDGenerator.lastGeneratedID(); got != scenario.runtimeID {
		t.Fatalf("local runtime identity = %q, want unique scenario identity %q", got, scenario.runtimeID)
	}
	if inputs.Stdout() != "" || inputs.Stderr() != "" {
		t.Fatalf("canceled pre-activation output = stdout:%q stderr:%q, want quiet output", inputs.Stdout(), inputs.Stderr())
	}
}

func testPlainBatchDrainStopsAfterWorkerActivationCancellation(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	fixture.prepareLocalActivation(t)
	scenario := newPlainBatchScenario(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputs := plainBatchInputs(t, scenario, "", true)
	inputs.Input.Context = ctx
	inputs.Input.Args = append(inputs.Input.Args, "--with-server")

	fixture.inputDirectoryWalker.armCancellation(cancel, scenario.gateID)

	if err := fixture.executeLocal(t, inputs.Input); err != nil {
		t.Fatalf("canceled service-mode plain batch error = %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	if got := fixture.inputDirectoryWalker.lastCancellationID(); got != scenario.gateID {
		t.Fatalf("worker-activation cancellation gate = %q, want unique scenario gate %q", got, scenario.gateID)
	}
	if inputs.Stdout() != "" || inputs.Stderr() != "" {
		t.Fatalf("canceled post-activation output = stdout:%q stderr:%q, want quiet output", inputs.Stdout(), inputs.Stderr())
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
