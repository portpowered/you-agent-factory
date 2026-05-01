package providers

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/agent-factory/pkg/api/generated"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/logging"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

const runtimeLoggingSmokeEnvKey = "AGENT_FACTORY_RUNTIME_LOGGING_SMOKE_ENV"

type runtimeLoggingSmokeRunner struct {
	stdout   string
	stderr   string
	exitCode int
}

func (r runtimeLoggingSmokeRunner) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{
		Stdout:   []byte(r.stdout),
		Stderr:   []byte(r.stderr),
		ExitCode: r.exitCode,
	}, nil
}

func TestRuntimeLoggingSmoke_SuccessAndFailureRespectOutputEnvAndRollingPolicies(t *testing.T) {
	t.Setenv(runtimeLoggingSmokeEnvKey, "runtime-logging-smoke-value")

	rollingConfig := logging.RuntimeLogConfig{
		MaxSize:    1,
		MaxBackups: 2,
		MaxAge:     3,
		Compress:   true,
	}

	t.Run("SuccessSuppressesSystemOutputAndRecordsEnvDiagnostics", func(t *testing.T) {
		result := runRuntimeLoggingSmoke(t, runtimeLoggingSmokeRunner{
			stdout:   "success stdout payload",
			stderr:   "success stderr payload",
			exitCode: 0,
		}, rollingConfig)

		result.harness.Assert().
			PlaceTokenCount("task:done", 1).
			HasNoTokenInPlace("task:init").
			HasNoTokenInPlace("task:failed")

		completionRecord := requireRuntimeLogEvent(t, result.records, workers.WorkLogEventCommandRunnerCompleted)
		if completionRecord["status"] != "succeeded" {
			t.Fatalf("success completion status = %#v, want succeeded in record %#v", completionRecord["status"], completionRecord)
		}
		if _, ok := completionRecord["stdout"]; ok {
			t.Fatalf("success completion unexpectedly included stdout in system log: %#v", completionRecord)
		}
		if _, ok := completionRecord["stderr"]; ok {
			t.Fatalf("success completion unexpectedly included stderr in system log: %#v", completionRecord)
		}

		completion := requireRecordedCompletion(t, result.artifact)
		if stringPointerValue(completion.Output) != "success stdout payload" {
			t.Fatalf("recorded completion output = %q, want success stdout payload", stringPointerValue(completion.Output))
		}
		assertRuntimeRecordsDoNotDuplicateEnvDiagnostics(t, result.records)
		assertRuntimeStartupRollingPolicy(t, result.records, result.logPath, rollingConfig)
	})

	t.Run("FailureIncludesSystemOutputAndRecordsEnvDiagnostics", func(t *testing.T) {
		result := runRuntimeLoggingSmoke(t, runtimeLoggingSmokeRunner{
			stdout:   "failure stdout context",
			stderr:   "failure stderr context",
			exitCode: 23,
		}, rollingConfig)

		result.harness.Assert().
			PlaceTokenCount("task:failed", 1).
			HasNoTokenInPlace("task:init").
			HasNoTokenInPlace("task:done")

		completionRecord := requireRuntimeLogEvent(t, result.records, workers.WorkLogEventCommandRunnerCompleted)
		if completionRecord["status"] != "failed" {
			t.Fatalf("failure completion status = %#v, want failed in record %#v", completionRecord["status"], completionRecord)
		}
		if completionRecord["exit_code"] != float64(23) {
			t.Fatalf("failure exit_code = %#v, want 23 in record %#v", completionRecord["exit_code"], completionRecord)
		}
		if completionRecord["stdout"] != "failure stdout context" {
			t.Fatalf("failure stdout = %#v, want failure stdout context in record %#v", completionRecord["stdout"], completionRecord)
		}
		if completionRecord["stderr"] != "failure stderr context" {
			t.Fatalf("failure stderr = %#v, want failure stderr context in record %#v", completionRecord["stderr"], completionRecord)
		}

		completion := requireRecordedCompletion(t, result.artifact)
		if stringPointerValue(completion.Error) != "failure stderr context" {
			t.Fatalf("recorded completion error = %q, want failure stderr context", stringPointerValue(completion.Error))
		}
		assertRuntimeRecordsDoNotDuplicateEnvDiagnostics(t, result.records)
		assertRuntimeStartupRollingPolicy(t, result.records, result.logPath, rollingConfig)
	})
}

type runtimeLoggingSmokeResult struct {
	harness  *testutil.ServiceTestHarness
	records  []map[string]any
	artifact *interfaces.ReplayArtifact
	logPath  string
}

func runRuntimeLoggingSmoke(t *testing.T, runner workers.CommandRunner, rollingConfig logging.RuntimeLogConfig) runtimeLoggingSmokeResult {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedRequest(t, dir, interfaces.SubmitRequest{
		RequestID:  "request-runtime-logging-smoke",
		WorkID:     "work-runtime-logging-smoke",
		WorkTypeID: "task",
		TraceID:    "trace-runtime-logging-smoke",
		Payload:    []byte("exercise runtime logging smoke policies"),
	})

	logDir := t.TempDir()
	recordPath := filepath.Join(t.TempDir(), "runtime-logging-smoke.replay.json")
	runtimeInstanceID := "runtime-logging-smoke"

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
		testutil.WithRecordPath(recordPath),
		testutil.WithRuntimeLogDir(logDir),
		testutil.WithRuntimeInstanceID(runtimeInstanceID),
		testutil.WithRuntimeLogConfig(rollingConfig),
	)
	h.RunUntilComplete(t, 10*time.Second)

	logPath := filepath.Join(logDir, runtimeInstanceID+".log")
	return runtimeLoggingSmokeResult{
		harness:  h,
		records:  readRuntimeLoggingSmokeRecords(t, logPath),
		artifact: testutil.LoadReplayArtifact(t, recordPath),
		logPath:  logPath,
	}
}

func readRuntimeLoggingSmokeRecords(t *testing.T, path string) []map[string]any {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", path, err)
	}

	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		records = append(records, record)
	}
	if len(records) == 0 {
		t.Fatalf("runtime log %s contained no records", path)
	}
	return records
}

func requireRuntimeLogEvent(t *testing.T, records []map[string]any, eventName string) map[string]any {
	t.Helper()

	for _, record := range records {
		if record["event_name"] == eventName {
			return record
		}
	}
	t.Fatalf("runtime log missing event_name %q in records %#v", eventName, records)
	return nil
}

func requireRuntimeLogMessage(t *testing.T, records []map[string]any, message string) map[string]any {
	t.Helper()

	for _, record := range records {
		if record["msg"] == message {
			return record
		}
	}
	t.Fatalf("runtime log missing msg %q in records %#v", message, records)
	return nil
}

func requireRecordedCompletion(t *testing.T, artifact *interfaces.ReplayArtifact) factoryapi.DispatchResponseEventPayload {
	t.Helper()

	completions := replayDispatchCompletedEvents(t, artifact)
	if len(completions) != 1 {
		t.Fatalf("recorded completions = %d, want 1", len(completions))
	}
	return completions[0]
}

func replayDispatchCompletedEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []factoryapi.DispatchResponseEventPayload {
	t.Helper()

	events := make([]factoryapi.DispatchResponseEventPayload, 0)
	for _, event := range artifact.Events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode DISPATCH_RESPONSE event %q: %v", event.Id, err)
		}
		events = append(events, payload)
	}
	return events
}

func assertRuntimeRecordsDoNotDuplicateEnvDiagnostics(t *testing.T, records []map[string]any) {
	t.Helper()

	for _, record := range records {
		if _, ok := record["env_count"]; ok {
			t.Fatalf("runtime system log duplicated env_count in record %#v", record)
		}
		if _, ok := record["env_keys"]; ok {
			t.Fatalf("runtime system log duplicated env_keys in record %#v", record)
		}
	}
}

func assertRuntimeStartupRollingPolicy(t *testing.T, records []map[string]any, logPath string, want logging.RuntimeLogConfig) {
	t.Helper()

	startup := requireRuntimeLogMessage(t, records, "factory started")
	if startup["runtime_log_path"] != logPath {
		t.Fatalf("runtime_log_path = %#v, want %q in record %#v", startup["runtime_log_path"], logPath, startup)
	}
	if startup["runtime_log_appender"] != "zap_rolling_file" {
		t.Fatalf("runtime_log_appender = %#v, want zap_rolling_file in record %#v", startup["runtime_log_appender"], startup)
	}
	if startup["runtime_log_max_size_mb"] != float64(want.MaxSize) {
		t.Fatalf("runtime_log_max_size_mb = %#v, want %d in record %#v", startup["runtime_log_max_size_mb"], want.MaxSize, startup)
	}
	if startup["runtime_log_max_backups"] != float64(want.MaxBackups) {
		t.Fatalf("runtime_log_max_backups = %#v, want %d in record %#v", startup["runtime_log_max_backups"], want.MaxBackups, startup)
	}
	if startup["runtime_log_max_age_days"] != float64(want.MaxAge) {
		t.Fatalf("runtime_log_max_age_days = %#v, want %d in record %#v", startup["runtime_log_max_age_days"], want.MaxAge, startup)
	}
	if startup["runtime_log_compress"] != want.Compress {
		t.Fatalf("runtime_log_compress = %#v, want %t in record %#v", startup["runtime_log_compress"], want.Compress, startup)
	}
	if startup["runtime_env_log_channel"] != "record" {
		t.Fatalf("runtime_env_log_channel = %#v, want record in record %#v", startup["runtime_env_log_channel"], startup)
	}
	if startup["runtime_success_command_output"] != "suppressed" {
		t.Fatalf("runtime_success_command_output = %#v, want suppressed in record %#v", startup["runtime_success_command_output"], startup)
	}
	if startup["runtime_failure_command_output"] != "included" {
		t.Fatalf("runtime_failure_command_output = %#v, want included in record %#v", startup["runtime_failure_command_output"], startup)
	}
	if startup["record_command_diagnostics"] != "preserved" {
		t.Fatalf("record_command_diagnostics = %#v, want preserved in record %#v", startup["record_command_diagnostics"], startup)
	}
}

func stringPointerValue[T ~string](value *T) string {
	if value == nil {
		return ""
	}
	return string(*value)
}
