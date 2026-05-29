package runtimelogtests

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// pkgmaintcheck:ignore-cyclomatic-complexity this runtime-log smoke test keeps the structured log contract inline across correlated record assertions.
func TestFactoryService_RunWritesStructuredRuntimeLogFile(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)
	logDir := filepath.Join(homeDir, ".you-agent-factory", "logs")
	runtimeInstanceID := "runtime-log-test"
	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		RuntimeInstanceID: runtimeInstanceID,
		RuntimeLogConfig: logging.RuntimeLogConfig{
			MaxSize:    9,
			MaxBackups: 8,
			MaxAge:     7,
			Compress:   true,
		},
	})
	if err != nil {
		t.Fatalf("service.BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	logPath := requireRuntimeLogPath(t, logDir, runtimeInstanceID)
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 || lines[0] == "" {
		t.Fatalf("expected at least one runtime log record in %s", logPath)
	}

	var foundStartup bool
	for _, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		assertRuntimeRecordTimestamp(t, record)
		if record["runtime_instance_id"] != runtimeInstanceID {
			t.Fatalf("runtime_instance_id = %#v, want %q", record["runtime_instance_id"], runtimeInstanceID)
		}
		if record["msg"] != "factory started" {
			continue
		}

		foundStartup = true
		assertRuntimeStartupLogSelection(t, record, logPath, logDir)
		if record["runtime_log_appender"] != logging.RuntimeLogAppenderZapRollingFile {
			t.Fatalf("runtime_log_appender = %#v, want %q", record["runtime_log_appender"], logging.RuntimeLogAppenderZapRollingFile)
		}
		if record["runtime_log_max_size_mb"] != float64(9) {
			t.Fatalf("runtime_log_max_size_mb = %#v, want 9", record["runtime_log_max_size_mb"])
		}
		if record["runtime_log_max_backups"] != float64(8) {
			t.Fatalf("runtime_log_max_backups = %#v, want 8", record["runtime_log_max_backups"])
		}
		if record["runtime_log_max_age_days"] != float64(7) {
			t.Fatalf("runtime_log_max_age_days = %#v, want 7", record["runtime_log_max_age_days"])
		}
		if record["runtime_log_compress"] != true {
			t.Fatalf("runtime_log_compress = %#v, want true", record["runtime_log_compress"])
		}
		if record["runtime_env_log_channel"] != logging.RuntimeEnvLogChannelRecord {
			t.Fatalf("runtime_env_log_channel = %#v, want %q", record["runtime_env_log_channel"], logging.RuntimeEnvLogChannelRecord)
		}
		if record["runtime_success_command_output"] != logging.RuntimeSuccessCommandOutputPolicy {
			t.Fatalf("runtime_success_command_output = %#v, want %q", record["runtime_success_command_output"], logging.RuntimeSuccessCommandOutputPolicy)
		}
		if record["runtime_failure_command_output"] != logging.RuntimeFailureCommandOutputPolicy {
			t.Fatalf("runtime_failure_command_output = %#v, want %q", record["runtime_failure_command_output"], logging.RuntimeFailureCommandOutputPolicy)
		}
		if record["record_command_diagnostics"] != logging.RuntimeRecordCommandDiagnosticsMode {
			t.Fatalf("record_command_diagnostics = %#v, want %q", record["record_command_diagnostics"], logging.RuntimeRecordCommandDiagnosticsMode)
		}
	}
	if !foundStartup {
		t.Fatalf("expected factory started record in runtime log:\n%s", data)
	}
}

func assertRuntimeStartupLogSelection(t *testing.T, record map[string]any, logPath, logDir string) {
	t.Helper()

	if record["runtime_log_path"] != logPath {
		t.Fatalf("runtime_log_path = %#v, want %q", record["runtime_log_path"], logPath)
	}
	if record["runtime_log_root"] != logDir {
		t.Fatalf("runtime_log_root = %#v, want %q", record["runtime_log_root"], logDir)
	}
	startTime, ok := record["runtime_log_start_time_utc"].(string)
	if !ok || startTime == "" {
		t.Fatalf("runtime_log_start_time_utc = %#v, want non-empty RFC3339 timestamp", record["runtime_log_start_time_utc"])
	}
	parsedStartTime, err := time.Parse(time.RFC3339Nano, startTime)
	if err != nil {
		t.Fatalf("runtime_log_start_time_utc = %q, want RFC3339 timestamp: %v", startTime, err)
	}
	if parsedStartTime.Location() != time.UTC {
		t.Fatalf("runtime_log_start_time_utc location = %s, want UTC", parsedStartTime.Location())
	}
}

func assertRuntimeRecordTimestamp(t *testing.T, record map[string]any) {
	t.Helper()

	ts, ok := record["ts"].(float64)
	if !ok {
		t.Fatalf("ts = %#v, want numeric zap production timestamp in record %#v", record["ts"], record)
	}
	if ts <= 0 {
		t.Fatalf("ts = %v, want positive timestamp in record %#v", ts, record)
	}
}

func TestFactoryService_RunWritesCorrelationFieldsToRuntimeLog(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, "initial-work.json")
	work := interfaces.SubmitRequest{
		RequestID:  "request-log-context",
		WorkID:     "work-log-context",
		WorkTypeID: "task",
		TraceID:    "trace-log-context",
		Payload:    []byte(`{"task":"correlate me"}`),
	}
	writeWorkRequestFile(t, workFile, work)

	runtimeInstanceID := "runtime-log-context-test"
	logDir := t.TempDir()
	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		RuntimeInstanceID: runtimeInstanceID,
		RuntimeLogDir:     logDir,
		WorkFile:          workFile,
	})
	if err != nil {
		t.Fatalf("service.BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	logPath := requireRuntimeLogPath(t, logDir, runtimeInstanceID)
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}

	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		assertRuntimeRecordTimestamp(t, record)
		if record["msg"] != "dispatcher: dispatching work to worker" {
			continue
		}
		if record["request_id"] != "request-log-context" {
			t.Fatalf("request_id = %#v, want request-log-context in record %#v", record["request_id"], record)
		}
		if record["trace_id"] != "trace-log-context" {
			t.Fatalf("trace_id = %#v, want trace-log-context in record %#v", record["trace_id"], record)
		}
		if record["work_id"] != "work-log-context" {
			t.Fatalf("work_id = %#v, want work-log-context in record %#v", record["work_id"], record)
		}
		return
	}
	t.Fatalf("expected correlated dispatcher log record in runtime log:\n%s", data)
}

func TestFactoryService_RunWritesWorkerPoolLifecycleEventsToRuntimeLog(t *testing.T) {
	work, logPath := runRuntimeLogFixture(t, runtimeLogFixtureOptions{
		runtimeInstanceID: "runtime-worker-pool-log-test",
		workFileName:      "initial-work.json",
		work: interfaces.SubmitRequest{
			RequestID:  "request-worker-pool-log",
			WorkID:     "work-worker-pool-log",
			WorkTypeID: "task",
			TraceID:    "trace-worker-pool-log",
			Payload:    []byte(`{"task":"exercise worker pool lifecycle logs"}`),
		},
	})

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}

	wantStatuses := map[string]string{
		workers.WorkLogEventWorkerPoolSubmitted:         "submitted",
		workers.WorkLogEventWorkerPoolExecutorEntered:   "entered_executor",
		workers.WorkLogEventWorkerPoolResponseSubmitted: "response_submitted",
	}
	found := make(map[string]bool, len(wantStatuses))
	var dispatchID string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		assertRuntimeRecordTimestamp(t, record)
		eventName, ok := record["event_name"].(string)
		if !ok {
			continue
		}
		wantStatus, ok := wantStatuses[eventName]
		if !ok {
			continue
		}
		assertRuntimeLogWorkContext(t, record, eventName, work)
		if record["status"] != wantStatus {
			t.Fatalf("status = %#v, want %q in record %#v", record["status"], wantStatus, record)
		}
		if record["dispatch_id"] == "" {
			t.Fatalf("expected dispatch_id in record %#v", record)
		}
		if dispatchID == "" {
			dispatchID, _ = record["dispatch_id"].(string)
		} else if record["dispatch_id"] != dispatchID {
			t.Fatalf("dispatch_id = %#v, want same dispatch_id %q in record %#v", record["dispatch_id"], dispatchID, record)
		}
		found[eventName] = true
	}
	for eventName := range wantStatuses {
		if !found[eventName] {
			t.Fatalf("expected worker-pool lifecycle event %q in runtime log:\n%s", eventName, data)
		}
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this command-runner log test keeps stdout, stderr, and correlation assertions on one reviewer-readable flow.
func TestFactoryService_RunWritesCommandRunnerEventsWithOutputsToRuntimeLog(t *testing.T) {
	work, logPath := runRuntimeLogFixture(t, runtimeLogFixtureOptions{
		runtimeInstanceID: "runtime-command-runner-log-test",
		workFileName:      "initial-work.json",
		scriptArgs:        []string{"--mode", "fixture"},
		work: interfaces.SubmitRequest{
			RequestID:  "request-command-runner-log",
			WorkID:     "work-command-runner-log",
			WorkTypeID: "task",
			TraceID:    "trace-command-runner-log",
			Payload:    []byte(`{"task":"exercise command runner logs"}`),
		},
		commandRunnerOverride: recordingDiagnosticsCommandRunner{},
	})

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}

	found := map[string]bool{
		workers.WorkLogEventCommandRunnerRequested: false,
		workers.WorkLogEventCommandRunnerCompleted: false,
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		assertRuntimeRecordTimestamp(t, record)
		eventName, ok := record["event_name"].(string)
		if !ok {
			continue
		}
		if _, ok := found[eventName]; !ok {
			continue
		}
		assertRuntimeLogWorkContext(t, record, eventName, work)
		if record["command"] != "script-tool" {
			t.Fatalf("command = %#v, want script-tool in record %#v", record["command"], record)
		}
		switch eventName {
		case workers.WorkLogEventCommandRunnerRequested:
			if record["status"] != "requested" {
				t.Fatalf("request status = %#v, want requested in record %#v", record["status"], record)
			}
		case workers.WorkLogEventCommandRunnerCompleted:
			if record["status"] != "succeeded" {
				t.Fatalf("completion status = %#v, want succeeded in record %#v", record["status"], record)
			}
			if _, ok := record["stdout"]; ok {
				t.Fatalf("completion record includes unexpected stdout on success in record %#v", record)
			}
			if _, ok := record["stderr"]; ok {
				t.Fatalf("completion record includes unexpected stderr on success in record %#v", record)
			}
			if record["exit_code"] != float64(0) {
				t.Fatalf("exit_code = %#v, want 0 in record %#v", record["exit_code"], record)
			}
		}
		found[eventName] = true
	}
	for eventName, ok := range found {
		if !ok {
			t.Fatalf("expected command runner event %q in runtime log:\n%s", eventName, data)
		}
	}
}

func TestFactoryService_RunDeduplicatesEnvPayloadBetweenRecordDiagnosticsAndRuntimeSystemLogs(t *testing.T) {
	runner := &recordingCommandRunnerWithCapture{}
	work, logPath, recordPath := runRuntimeLogAndReplayFixture(t, runtimeLogFixtureOptions{
		runtimeInstanceID: "runtime-env-dedupe-test",
		workFileName:      "initial-work.json",
		scriptArgs:        []string{"--mode", "fixture"},
		work: interfaces.SubmitRequest{
			RequestID:  "request-env-dedupe",
			WorkID:     "work-env-dedupe",
			WorkTypeID: "task",
			TraceID:    "trace-env-dedupe",
			Payload:    []byte(`{"task":"assert env dedupe between channels"}`),
		},
		commandRunnerOverride: runner,
	})

	artifact, err := replay.Load(recordPath)
	if err != nil {
		t.Fatalf("Load(recording): %v", err)
	}
	completions := serviceReplayDispatchCompletedEvents(t, artifact)
	if len(completions) != 1 {
		t.Fatalf("recorded completions = %d, want 1", len(completions))
	}
	completion := completions[0].Payload
	encodedCompletion, err := json.Marshal(completion)
	if err != nil {
		t.Fatalf("marshal completion: %v", err)
	}
	if strings.Contains(string(encodedCompletion), "env_count") || strings.Contains(string(encodedCompletion), "env_keys") {
		t.Fatalf("recorded completion leaked environment diagnostics: %s", encodedCompletion)
	}
	if len(runner.request.Env) == 0 {
		t.Fatalf("captured command request had no env entries")
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}
	records := parseRuntimeLogRecords(t, string(data))
	requestRecord := findRuntimeLogEvent(records, workers.WorkLogEventCommandRunnerRequested)
	if requestRecord == nil {
		t.Fatalf("missing requested command-runner event in runtime log:\n%s", data)
	}
	if _, ok := requestRecord["env_count"]; ok {
		t.Fatalf("requested command-runner event includes env_count (should be record-only): %#v", requestRecord["env_count"])
	}
	if requestRecord["command"] != "script-tool" {
		t.Fatalf("command = %#v, want script-tool in requested record %#v", requestRecord["command"], requestRecord)
	}
	_ = work
}

func TestFactoryService_RunMirrorsVerboseCommandRunnerEventsToFileAndLogger(t *testing.T) {
	defaultLog, defaultObserved := runVerboseRuntimeLogFixture(t, false)
	if runtimeLogHasEvent(t, defaultLog, workers.WorkLogEventCommandRunnerRequestDetails) {
		t.Fatalf("default runtime log unexpectedly contained verbose request details:\n%s", defaultLog)
	}
	if len(defaultObserved.FilterField(zap.String("event_name", workers.WorkLogEventCommandRunnerRequestDetails)).All()) != 0 {
		t.Fatal("default logger unexpectedly received verbose request details")
	}

	verboseLog, verboseObserved := runVerboseRuntimeLogFixture(t, true)
	verboseRecord := runtimeLogEventRecord(t, verboseLog, workers.WorkLogEventCommandRunnerRequestDetails)
	if verboseRecord["request_id"] != "request-verbose-command-log" {
		t.Fatalf("request_id = %#v, want request-verbose-command-log in record %#v", verboseRecord["request_id"], verboseRecord)
	}
	if verboseRecord["trace_id"] != "trace-verbose-command-log" {
		t.Fatalf("trace_id = %#v, want trace-verbose-command-log in record %#v", verboseRecord["trace_id"], verboseRecord)
	}
	if verboseRecord["work_id"] != "work-verbose-command-log" {
		t.Fatalf("work_id = %#v, want work-verbose-command-log in record %#v", verboseRecord["work_id"], verboseRecord)
	}
	if !runtimeLogHasEvent(t, verboseLog, workers.WorkLogEventCommandRunnerOutputDetails) {
		t.Fatalf("verbose runtime log missing output details:\n%s", verboseLog)
	}
	if len(verboseObserved.FilterField(zap.String("event_name", workers.WorkLogEventCommandRunnerRequestDetails)).All()) == 0 {
		t.Fatal("verbose logger did not receive request detail record")
	}
	if len(verboseObserved.FilterField(zap.String("event_name", workers.WorkLogEventCommandRunnerOutputDetails)).All()) == 0 {
		t.Fatal("verbose logger did not receive output detail record")
	}
}

func TestFactoryService_RunWritesEndToEndCorrelatedWorkLogSmoke(t *testing.T) {
	core, observed := observer.New(zap.InfoLevel)
	work, logPath, recordPath := runRuntimeLogAndReplayFixture(t, runtimeLogFixtureOptions{
		runtimeInstanceID: "runtime-work-log-smoke-test",
		workFileName:      "initial-work.json",
		scriptArgs:        []string{"--mode", "smoke"},
		work: interfaces.SubmitRequest{
			RequestID:  "request-work-log-smoke",
			WorkID:     "work-log-smoke",
			WorkTypeID: "task",
			TraceID:    "trace-work-log-smoke",
			Payload:    []byte(`{"task":"correlate runtime logs with replay records"}`),
		},
		verbose:               true,
		logger:                zap.New(core),
		commandRunnerOverride: recordingDiagnosticsCommandRunner{},
	})

	dispatchID := assertReplayCorrelationForWork(t, recordPath, work)
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}
	assertRuntimeLogCorrelationForWork(t, string(data), work, dispatchID)
	if len(observed.FilterField(zap.String("event_name", workers.WorkLogEventCommandRunnerRequestDetails)).All()) == 0 {
		t.Fatal("verbose command request details were not mirrored to the command-line logger")
	}
	if len(observed.FilterField(zap.String("event_name", workers.WorkLogEventCommandRunnerOutputDetails)).All()) == 0 {
		t.Fatal("verbose command output details were not mirrored to the command-line logger")
	}
}

type runtimeLogFixtureOptions struct {
	runtimeInstanceID     string
	workFileName          string
	work                  interfaces.SubmitRequest
	scriptArgs            []string
	logger                *zap.Logger
	verbose               bool
	commandRunnerOverride workers.CommandRunner
}

func runRuntimeLogFixture(t *testing.T, opts runtimeLogFixtureOptions) (interfaces.SubmitRequest, string) {
	t.Helper()
	work, logPath, _ := runRuntimeLogAndReplayFixture(t, opts)
	return work, logPath
}

func runRuntimeLogAndReplayFixture(t *testing.T, opts runtimeLogFixtureOptions) (interfaces.SubmitRequest, string, string) {
	t.Helper()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	if len(opts.scriptArgs) > 0 {
		writeScriptWorkerAgentsMDWithCommand(t, dir, "worker-a", "script-tool", opts.scriptArgs)
	} else {
		writeWorkstationAgentsMD(t, dir, "process")
	}
	writeWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	workFile := filepath.Join(dir, opts.workFileName)
	writeWorkRequestFile(t, workFile, opts.work)
	logDir := t.TempDir()
	recordPath := filepath.Join(t.TempDir(), "recording.json")

	logger := opts.logger
	if logger == nil {
		logger = zap.NewNop()
	}
	svc, err := service.BuildFactoryService(context.Background(), &service.FactoryServiceConfig{
		Dir:                   dir,
		Logger:                logger,
		Verbose:               opts.verbose,
		RuntimeInstanceID:     opts.runtimeInstanceID,
		RuntimeLogDir:         logDir,
		RecordPath:            recordPath,
		WorkFile:              workFile,
		CommandRunnerOverride: opts.commandRunnerOverride,
	})
	if err != nil {
		t.Fatalf("service.BuildFactoryService: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := svc.Run(ctx); err != nil {
		t.Fatalf("Run: %v", err)
	}

	return opts.work, requireRuntimeLogPath(t, logDir, opts.runtimeInstanceID), recordPath
}

func requireRuntimeLogPath(t *testing.T, logDir, runtimeInstanceID string) string {
	t.Helper()

	matches, err := filepath.Glob(filepath.Join(logDir, "*", "*", "*-"+runtimeInstanceID+"-*.log"))
	if err != nil {
		t.Fatalf("glob runtime log path: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("runtime log paths for %q under %s = %v, want exactly one", runtimeInstanceID, logDir, matches)
	}
	return matches[0]
}

func assertRuntimeLogWorkContext(t *testing.T, record map[string]any, eventName string, work interfaces.SubmitRequest) {
	t.Helper()
	if record["request_id"] != work.RequestID {
		t.Fatalf("%s request_id = %#v, want %q in record %#v", eventName, record["request_id"], work.RequestID, record)
	}
	if record["trace_id"] != work.TraceID {
		t.Fatalf("%s trace_id = %#v, want %q in record %#v", eventName, record["trace_id"], work.TraceID, record)
	}
	if record["work_id"] != work.WorkID {
		t.Fatalf("%s work_id = %#v, want %q in record %#v", eventName, record["work_id"], work.WorkID, record)
	}
}

func runVerboseRuntimeLogFixture(t *testing.T, verbose bool) (string, *observer.ObservedLogs) {
	t.Helper()
	core, observed := observer.New(zap.InfoLevel)
	_, logPath := runRuntimeLogFixture(t, runtimeLogFixtureOptions{
		runtimeInstanceID: "runtime-verbose-command-log-test",
		workFileName:      "initial-work.json",
		scriptArgs:        []string{"--mode", "fixture"},
		work: interfaces.SubmitRequest{
			RequestID:  "request-verbose-command-log",
			WorkID:     "work-verbose-command-log",
			WorkTypeID: "task",
			TraceID:    "trace-verbose-command-log",
			Payload:    []byte(`{"task":"exercise verbose command logs"}`),
		},
		logger:                zap.New(core),
		verbose:               verbose,
		commandRunnerOverride: recordingDiagnosticsCommandRunner{},
	})
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read runtime log %s: %v", logPath, err)
	}
	return string(data), observed
}

func assertReplayCorrelationForWork(t *testing.T, recordPath string, work interfaces.SubmitRequest) string {
	t.Helper()
	artifact, err := replay.Load(recordPath)
	if err != nil {
		t.Fatalf("Load(recording): %v", err)
	}
	submissions := serviceReplayWorkRequestEvents(t, artifact)
	if len(submissions) != 1 {
		t.Fatalf("recorded submissions = %d, want 1", len(submissions))
	}
	submission := submissions[0]
	if serviceStringValue(submission.Event.Context.RequestId) != work.RequestID ||
		serviceFirstStringValue(submission.Event.Context.TraceIds) != work.TraceID ||
		serviceFirstStringValue(submission.Event.Context.WorkIds) != work.WorkID {
		t.Fatalf("recorded request IDs = (%q, %q, %q), want (%q, %q, %q)",
			serviceStringValue(submission.Event.Context.RequestId),
			serviceFirstStringValue(submission.Event.Context.TraceIds),
			serviceFirstStringValue(submission.Event.Context.WorkIds),
			work.RequestID, work.TraceID, work.WorkID)
	}

	dispatches := serviceReplayDispatchCreatedEvents(t, artifact)
	if len(dispatches) != 1 {
		t.Fatalf("recorded dispatches = %d, want 1", len(dispatches))
	}
	dispatch := dispatches[0]
	if serviceStringValue(dispatch.Event.Context.RequestId) != work.RequestID ||
		serviceFirstStringValue(dispatch.Event.Context.TraceIds) != work.TraceID ||
		serviceFirstStringValue(dispatch.Event.Context.WorkIds) != work.WorkID {
		t.Fatalf("recorded dispatch event metadata = %#v, want request/trace/work IDs from %#v", dispatch.Event.Context, work)
	}

	completions := serviceReplayDispatchCompletedEvents(t, artifact)
	if len(completions) != 1 {
		t.Fatalf("recorded completions = %d, want 1", len(completions))
	}
	dispatchID := serviceStringValue(dispatch.Event.Context.DispatchId)
	completionDispatchID := serviceStringValue(completions[0].Event.Context.DispatchId)
	if completionDispatchID != dispatchID {
		t.Fatalf("recorded completion dispatch ID = %q, want %q", completionDispatchID, dispatchID)
	}
	if serviceStringValue(completions[0].Payload.Output) != "script done" {
		t.Fatalf("recorded completion output = %q, want script done", serviceStringValue(completions[0].Payload.Output))
	}
	return dispatchID
}

func assertRuntimeLogCorrelationForWork(t *testing.T, data string, work interfaces.SubmitRequest, dispatchID string) {
	t.Helper()
	records := parseRuntimeLogRecords(t, data)
	requiredEvents := map[string]struct {
		status     string
		dispatchID string
	}{
		workers.WorkLogEventWorkerPoolSubmitted:         {status: "submitted", dispatchID: dispatchID},
		workers.WorkLogEventWorkerPoolExecutorEntered:   {status: "entered_executor", dispatchID: dispatchID},
		workers.WorkLogEventWorkerPoolResponseSubmitted: {status: "response_submitted", dispatchID: dispatchID},
		workers.WorkLogEventCommandRunnerRequested:      {status: "requested"},
		workers.WorkLogEventCommandRunnerCompleted:      {status: "succeeded"},
		workers.WorkLogEventCommandRunnerRequestDetails: {status: "verbose"},
		workers.WorkLogEventCommandRunnerOutputDetails:  {status: "verbose"},
	}
	for eventName, want := range requiredEvents {
		record := findRuntimeLogEvent(records, eventName)
		if record == nil {
			t.Fatalf("expected runtime log event %q in:\n%s", eventName, data)
		}
		assertRuntimeLogWorkContext(t, record, eventName, work)
		if record["status"] != want.status {
			t.Fatalf("%s status = %#v, want %q in record %#v", eventName, record["status"], want.status, record)
		}
		if want.dispatchID != "" && record["dispatch_id"] != want.dispatchID {
			t.Fatalf("%s dispatch_id = %#v, want %q in record %#v", eventName, record["dispatch_id"], want.dispatchID, record)
		}
	}

	completionRecord := findRuntimeLogEvent(records, workers.WorkLogEventCommandRunnerCompleted)
	if _, ok := completionRecord["stdout"]; ok {
		t.Fatalf("command completion includes unexpected stdout in record %#v", completionRecord)
	}
	if _, ok := completionRecord["stderr"]; ok {
		t.Fatalf("command completion includes unexpected stderr in record %#v", completionRecord)
	}
}

func runtimeLogHasEvent(t *testing.T, data, eventName string) bool {
	t.Helper()
	return runtimeLogEventRecord(t, data, eventName) != nil
}

func runtimeLogEventRecord(t *testing.T, data, eventName string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		if record["event_name"] == eventName {
			return record
		}
	}
	return nil
}

func parseRuntimeLogRecords(t *testing.T, data string) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		records = append(records, record)
	}
	return records
}

func findRuntimeLogEvent(records []map[string]any, eventName string) map[string]any {
	for _, record := range records {
		if record["event_name"] == eventName {
			return record
		}
	}
	return nil
}

type recordingDiagnosticsCommandRunner struct{}

func (recordingDiagnosticsCommandRunner) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{Stdout: []byte("script done\n"), Stderr: []byte("script details\n")}, nil
}

type recordingCommandRunnerWithCapture struct {
	request workers.CommandRequest
}

func (r *recordingCommandRunnerWithCapture) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.request = req
	return workers.CommandResult{Stdout: []byte("script done\n"), Stderr: []byte("script details\n")}, nil
}
