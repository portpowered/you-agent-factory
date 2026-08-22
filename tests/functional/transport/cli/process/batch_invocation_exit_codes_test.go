package process_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

type batchProcessReport struct {
	Status   string                `json:"status"`
	Failures []batchProcessFailure `json:"failures"`
}

type batchProcessFailure struct {
	WorkID    string `json:"workId,omitempty"`
	WorkName  string `json:"workName"`
	WorkState string `json:"workState"`
	Reason    string `json:"reason"`
}

// TestBuiltCLIBatchExitCodesReportSingleWorkOutcome proves the ordinary
// --work path, which is distinct from the characterized one-shot --named
// path, returns process status and stdout from the submitted Work outcome.
func TestBuiltCLIBatchExitCodesReportSingleWorkOutcome(t *testing.T) {
	harness := builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
	buildContext, cancelBuild := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancelBuild()
	binaryPath := buildYouBinary(t, buildContext, testutil.MustRepoRoot(t))

	t.Run("success quiet result exits zero", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
		defer cancel()
		session := newConfiguredGoalSession(t, ctx, harness, "compiled-batch-success")
		writeBatchCurrentFactory(t, session.WorkDir)
		workFile := writeBatchWork(t, "single successful batch Work")
		mockWorkersPath := writeBatchMockWorkers(t, workers.MockWorkerRunTypeAccept)

		result, err := runBuiltYouBinary(ctx, binaryPath, session, batchRunArgs(
			session, workFile, mockWorkersPath, "--quiet",
		)...)
		if err != nil {
			t.Fatalf("compiled batch success: %v\nstdout:\n%s\nstderr:\n%s", err, result.Stdout, result.Stderr)
		}
		if result.ExitCode != 0 {
			t.Fatalf("success exit code = %d, want 0; stdout=%q stderr=%q", result.ExitCode, result.Stdout, result.Stderr)
		}
		if result.Stdout != "Batch completed successfully.\n" {
			t.Fatalf("success stdout = %q, want truthful batch success result", result.Stdout)
		}
		if result.Stderr != "" {
			t.Fatalf("success stderr = %q, want empty", result.Stderr)
		}
	})

	for _, policy := range []struct {
		name  string
		extra string
	}{
		{name: "default"},
		{name: "verbose", extra: "--verbose"},
	} {
		policy := policy
		t.Run("success "+policy.name+" policy keeps result", func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
			defer cancel()
			session := newConfiguredGoalSession(t, ctx, harness, "compiled-batch-success-"+policy.name)
			writeBatchCurrentFactory(t, session.WorkDir)
			workFile := writeBatchWork(t, "single "+policy.name+" batch Work")
			mockWorkersPath := writeBatchMockWorkers(t, workers.MockWorkerRunTypeAccept)

			args := batchRunArgs(session, workFile, mockWorkersPath)
			if policy.extra != "" {
				args = append(args, policy.extra)
			}
			result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
			if err != nil || result.ExitCode != 0 {
				t.Fatalf("compiled batch %s success: %v; exit=%d stdout=%q stderr=%q", policy.name, err, result.ExitCode, result.Stdout, result.Stderr)
			}
			if !strings.Contains(result.Stdout, "Batch completed successfully.") {
				t.Fatalf("%s stdout = %q, want truthful batch success result", policy.name, result.Stdout)
			}
		})
	}

	t.Run("failed terminal Work exits nonzero with human detail", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
		defer cancel()
		session := newConfiguredGoalSession(t, ctx, harness, "compiled-batch-human-failure")
		writeBatchCurrentFactory(t, session.WorkDir)
		workFile := writeBatchWork(t, "single failing batch Work")
		mockWorkersPath := writeBatchMockWorkers(t, workers.MockWorkerRunTypeReject)

		result, err := runBuiltYouBinary(ctx, binaryPath, session, batchRunArgs(
			session, workFile, mockWorkersPath, "--quiet",
		)...)
		if err == nil {
			t.Fatalf("compiled batch failure succeeded: %#v", result)
		}
		if result.ExitCode == 0 {
			t.Fatalf("failure exit code = %d, want non-zero", result.ExitCode)
		}
		for _, want := range []string{
			"Batch failed:",
			`Work "single failing batch Work"`,
			"prompt-task:failed",
		} {
			if !strings.Contains(result.Stdout, want) {
				t.Fatalf("failure stdout missing %q:\n%s", want, result.Stdout)
			}
		}
		if strings.TrimSpace(result.Stderr) == "" {
			t.Fatal("failure stderr is empty; want the non-zero batch error diagnostic")
		}
	})

	t.Run("failed terminal Work JSON is parseable", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 45*time.Second)
		defer cancel()
		session := newConfiguredGoalSession(t, ctx, harness, "compiled-batch-json-failure")
		writeBatchCurrentFactory(t, session.WorkDir)
		workFile := writeBatchWork(t, "single JSON failing batch Work")
		mockWorkersPath := writeBatchMockWorkers(t, workers.MockWorkerRunTypeReject)

		args := batchRunArgs(session, workFile, mockWorkersPath)
		args = append([]string{"--json"}, args...)
		result, err := runBuiltYouBinary(ctx, binaryPath, session, args...)
		if err == nil {
			t.Fatalf("compiled batch JSON failure succeeded: %#v", result)
		}
		if result.ExitCode == 0 {
			t.Fatalf("JSON failure exit code = %d, want non-zero", result.ExitCode)
		}
		var report batchProcessReport
		if decodeErr := json.Unmarshal([]byte(result.Stdout), &report); decodeErr != nil {
			t.Fatalf("JSON batch stdout is not parseable: %v\nstdout:\n%s\nstderr:\n%s", decodeErr, result.Stdout, result.Stderr)
		}
		if report.Status != "FAILED" || len(report.Failures) != 1 {
			t.Fatalf("JSON batch report = %#v, want one failure", report)
		}
		failure := report.Failures[0]
		if failure.WorkName != "single JSON failing batch Work" || failure.WorkState != "prompt-task:failed" || strings.TrimSpace(failure.Reason) == "" {
			t.Fatalf("JSON failure = %#v, want Work name, terminal state, and reason", failure)
		}
	})
}

func batchRunArgs(
	session *builtcliacceptance.Session,
	workFile string,
	mockWorkersPath string,
	extra ...string,
) []string {
	args := append([]string{}, session.RuntimeLogDirFlags()...)
	args = append(args, session.ServerFlags()...)
	args = append(args,
		"run",
		"--work", workFile,
		"--with-mock-workers="+mockWorkersPath,
		"--no-record",
	)
	return append(args, extra...)
}

func writeBatchCurrentFactory(t testing.TB, workingDirectory string) {
	t.Helper()
	sourcePath := writeStdinRunFactory(t, workingDirectory)
	sourceDir := filepath.Dir(sourcePath)
	destinationDir := filepath.Join(workingDirectory, "factory")
	if err := os.MkdirAll(filepath.Join(destinationDir, "workstations", stdinRunWorkstationName), 0o755); err != nil {
		t.Fatalf("create Current Factory directory: %v", err)
	}
	factoryJSON, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read batch Current Factory fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destinationDir, "factory.json"), factoryJSON, 0o600); err != nil {
		t.Fatalf("write batch Current Factory fixture: %v", err)
	}
	workstationConfig, err := os.ReadFile(filepath.Join(sourceDir, "workstations", stdinRunWorkstationName, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read batch workstation fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destinationDir, "workstations", stdinRunWorkstationName, "AGENTS.md"), workstationConfig, 0o644); err != nil {
		t.Fatalf("write batch workstation fixture: %v", err)
	}
	workerDir := filepath.Join(destinationDir, "workers", stdinRunWorkerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create batch worker directory: %v", err)
	}
	workerConfig := "---\ntype: SCRIPT_WORKER\ncommand: echo\nargs:\n  - batch-exit-fixture\n---\n"
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(workerConfig), 0o644); err != nil {
		t.Fatalf("write batch worker fixture: %v", err)
	}
}

func writeBatchWork(t testing.TB, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch-work.json")
	request := work.WorkRequest{
		Type: work.WorkRequestTypeFactoryRequestBatch,
		Works: []work.Work{{
			Name: name, WorkID: strings.ToLower(strings.ReplaceAll(name, " ", "-")),
			WorkTypeID: stdinRunWorkTypeName, TraceID: "batch-exit-trace", Payload: "batch exit contract",
		}},
	}
	data, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal batch Work: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write batch Work: %v", err)
	}
	return path
}

func writeBatchMockWorkers(t testing.TB, runType workers.MockWorkerRunType) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "batch-mock-workers.json")
	config := workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		WorkerName: stdinRunWorkerName, WorkstationName: stdinRunWorkstationName, RunType: runType,
	}}}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatalf("marshal batch mock workers: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write batch mock workers: %v", err)
	}
	return path
}
