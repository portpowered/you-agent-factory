package mock

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const namedAgyMockModel = "gemini-3.6-flash-high"

// TestNamedAgyMockPreservesDispatchMetadataAndCompletionLog proves the
// Workers-owned mock feature through the canonical root process. The named
// mock intercepts an Antigravity attempt, retains its configured exit code in
// the command completion log, and keeps the source Work correlation without
// invoking the replaceable live ProviderCommandRunner edge.
func TestNamedAgyMockPreservesDispatchMetadataAndCompletionLog(t *testing.T) {
	t.Parallel()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderAntigravity,
		namedAgyMockModel,
	))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		RequestID:  "agy-mock-request",
		WorkID:     "agy-mock-work",
		Name:       "agy-mock-work",
		WorkTypeID: "task",
		TraceID:    "agy-mock-trace",
		Payload:    []byte(`{"title":"named Agy mock"}`),
	})

	logDir := t.TempDir()
	exitCode := 7
	mockConfigPath := support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "worker",
			WorkstationName: "process",
			RunType:         workers.MockWorkerRunTypeReject,
			RejectConfig: &workers.MockWorkerRejectConfig{
				Stdout:   "configured Agy stdout",
				Stderr:   "configured Agy stderr",
				ExitCode: &exitCode,
			},
		}},
	})
	liveRunner := support.NewRecordingCommandRunner("live Agy edge must not run")
	process, err := support.BuildProcessWithContext(t.Context(), serviceedges.Edges{
		ProviderCommandRunner: liveRunner,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	homeDir := t.TempDir()
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", dir,
		"--with-mock-workers", mockConfigPath,
		"--runtime-log-dir", logDir,
		"--no-record", "--quiet",
	})
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute() error = %v; stdout=%q stderr=%q", err, inputs.Stdout(), inputs.Stderr())
	}
	closeContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := process.Close(closeContext); err != nil {
		t.Fatalf("close root process: %v", err)
	}

	if liveRunner.CallCount() != 0 {
		t.Fatalf("live Agy edge calls = %d, want zero for named mock dispatch", liveRunner.CallCount())
	}
	record := findNamedAgyRuntimeLogRecord(
		t,
		requireNamedAgyRuntimeLogPath(t, logDir),
		"command_runner.completed",
	)
	if record["exit_code"] != float64(exitCode) {
		t.Fatalf("logged Agy exit_code = %#v, want %d", record["exit_code"], exitCode)
	}
	for key, want := range map[string]any{
		"request_id": "agy-mock-request",
		"trace_id":   "agy-mock-trace",
		"work_id":    "agy-mock-work",
	} {
		if record[key] != want {
			t.Fatalf("logged Agy %s = %#v, want %q", key, record[key], want)
		}
	}
}

func requireNamedAgyRuntimeLogPath(t *testing.T, logDir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(logDir, "*", "*", "*", "*-runtime-log-*.log"))
	if err != nil {
		t.Fatalf("glob Agy runtime log path: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("Agy runtime log paths under %s = %v, want exactly one", logDir, matches)
	}
	return matches[0]
}

func findNamedAgyRuntimeLogRecord(t *testing.T, path, eventName string) map[string]any {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open Agy runtime log %s: %v", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var record map[string]any
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			t.Fatalf("decode Agy runtime log record: %v", err)
		}
		if record["event_name"] == eventName {
			return record
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan Agy runtime log %s: %v", path, err)
	}
	t.Fatalf("Agy runtime log %s did not contain event_name %q", path, eventName)
	return nil
}
