package mock

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const plainBatchDrainTestTimeout = 15 * time.Second
const plainBatchContinuousIdleObservation = 500 * time.Millisecond

// TestPlainBatchDrainReportsStrandedWork proves the no-server customer path
// returns the canonical incomplete-drain diagnostic after a deterministic mock
// worker completes its dispatch but leaves Work in PROCESSING.
func TestPlainBatchDrainReportsStrandedWork(t *testing.T) {
	t.Parallel()
	factoryDir := scaffoldPlainBatchDrainFactory(t)
	workFile := writePlainBatchDrainWork(t)
	mockWorkersFile := writePlainBatchDrainMockWorkers(t)

	process := support.BuildProcess(t, serviceedges.Edges{})
	support.CleanupProcess(t, process)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", factoryDir, "--no-record", "--quiet",
		"--work", workFile, "--with-mock-workers", mockWorkersFile,
	})
	inputs.WorkingDirectory = factoryDir
	homeDir := t.TempDir()
	inputs.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	command := support.StartProcessCommand(t, process, inputs.Input)
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

func TestPlainBatchDrainPreservesFiniteAndContinuousCounterexamples(t *testing.T) {
	t.Parallel()
	factoryDir := scaffoldPlainBatchDrainFactory(t)

	for _, scenario := range []struct {
		name     string
		workFile func(*testing.T) string
	}{
		{name: "empty"},
		{name: "terminal work", workFile: func(t *testing.T) string {
			return writePlainBatchDrainWorkState(t, "complete")
		}},
	} {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			var workFile string
			if scenario.workFile != nil {
				workFile = scenario.workFile(t)
			}
			inputs := plainBatchInputs(t, factoryDir, workFile, false)
			process := support.BuildProcess(t, serviceedges.Edges{})
			support.CleanupProcess(t, process)

			if err := process.Execute(inputs.Input); err != nil {
				t.Fatalf("finite plain batch error = %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
			}
			wantStdout := ""
			if scenario.workFile != nil {
				wantStdout = "Batch completed successfully.\n"
			}
			if inputs.Stdout() != wantStdout || inputs.Stderr() != "" {
				t.Fatalf("finite plain success output = stdout:%q stderr:%q, want stdout:%q and quiet stderr", inputs.Stdout(), inputs.Stderr(), wantStdout)
			}
		})
	}

	t.Run("continuous idle", func(t *testing.T) {
		inputs := plainBatchInputs(t, factoryDir, "", true)
		process := support.BuildProcess(t, serviceedges.Edges{})
		support.CleanupProcess(t, process)
		command := support.StartProcessCommand(t, process, inputs.Input)

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

func TestPlainBatchDrainRejectsCancellationBeforeRuntimeActivation(t *testing.T) {
	t.Parallel()
	factoryDir := scaffoldPlainBatchDrainFactory(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputs := plainBatchInputs(t, factoryDir, "", true)
	inputs.Input.Context = ctx

	process := support.BuildProcess(t, serviceedges.Edges{
		FactorySessionIDGenerator: func() string {
			cancel()
			return "preactivation-canceled-session"
		},
	})
	support.CleanupProcess(t, process)

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("canceled continuous plain batch error = %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stdout() != "" || inputs.Stderr() != "" {
		t.Fatalf("canceled pre-activation output = stdout:%q stderr:%q, want quiet output", inputs.Stdout(), inputs.Stderr())
	}
}

func TestPlainBatchDrainStopsAfterWorkerActivationCancellation(t *testing.T) {
	t.Parallel()
	factoryDir := scaffoldPlainBatchDrainFactory(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	inputs := support.FakeInputs(ctx, []string{
		"you", "run", "--dir", factoryDir, "--continuously", "--with-server", "--no-record", "--quiet",
	})
	inputs.WorkingDirectory = factoryDir
	homeDir := t.TempDir()
	inputs.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	process := support.BuildProcess(t, serviceedges.Edges{
		FactoryRuntimeInputDirectoryWalker: func(string, fs.WalkDirFunc) error {
			cancel()
			return nil
		},
	})
	support.CleanupProcess(t, process)

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("canceled service-mode plain batch error = %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stdout() != "" || inputs.Stderr() != "" {
		t.Fatalf("canceled post-activation output = stdout:%q stderr:%q, want quiet output", inputs.Stdout(), inputs.Stderr())
	}
}

func plainBatchInputs(t *testing.T, factoryDir, workFile string, continuous bool) *support.CapturedInputs {
	t.Helper()
	args := []string{"you", "run", "--dir", factoryDir, "--no-record", "--quiet"}
	if continuous {
		args = append(args, "--continuously")
	}
	if workFile != "" {
		args = append(args, "--work", workFile)
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.WorkingDirectory = factoryDir
	homeDir := t.TempDir()
	inputs.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	return inputs
}

func scaffoldPlainBatchDrainFactory(t *testing.T) string {
	t.Helper()

	dir := support.ScaffoldFactory(t, map[string]any{
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "processing", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]string{{"name": "worker-a"}},
		"workstations": []map[string]any{{
			"name":      "process",
			"worker":    "worker-a",
			"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
			"outputs":   []map[string]string{{"workType": "task", "state": "processing"}},
			"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
		}},
	})
	support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	return dir
}

func writePlainBatchDrainWork(t *testing.T) string {
	return writePlainBatchDrainWorkState(t, "init")
}

func writePlainBatchDrainWorkState(t *testing.T, state string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plain-batch-drain-work.json")
	payload := map[string]any{
		"requestId": "plain-batch-drain",
		"type":      "FACTORY_REQUEST_BATCH",
		"works": []map[string]any{{
			"name":         "stranded-work",
			"workTypeName": "task",
			"state":        state,
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

func writePlainBatchDrainMockWorkers(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "plain-batch-drain-mock-workers.json")
	payload := workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "worker-a",
			WorkstationName: "process",
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
