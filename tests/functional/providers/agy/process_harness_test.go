package agy

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const agyFunctionalModel = agyGoldenModel

// TestAgyConductorSuccessThroughRootBuildProcess proves successful Agy
// print-mode execution through the customer process boundary and
// Providers-backed command adapter.
func TestAgyConductorSuccessThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", strings.Replace(
		support.BuildModelWorkerConfig(modelprovider.ProviderAntigravity, agyFunctionalModel),
		"stopToken: COMPLETE",
		"skipPermissions: true\nstopToken: COMPLETE",
		1,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"agy conductor success"}`))

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: []byte(`{"event":"result","result":{"conversation_id":"agy-conductor-success","status":"SUCCESS","response":"agy functional answer COMPLETE","duration_seconds":1.0,"num_turns":1,"usage":{"input_tokens":1,"output_tokens":1,"thinking_tokens":0,"cache_read_tokens":0,"total_tokens":2}}}` + "\n"),
	})

	_, listed, events, responseEvents := support.RunFactoryToCompletionWithEdgesAndResponseEvents(
		t,
		dir,
		agyFunctionalEdges(runner),
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed work = %d, want 0", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("agy command runner calls = %d, want 1 through Providers path", runner.CallCount())
	}
	request := runner.LastRequest()
	if !containsArgPair(request.Args, "--model", agyFunctionalModel) {
		t.Fatalf("argv = %#v, want --model %s", request.Args, agyFunctionalModel)
	}
	if !containsArg(request.Args, "-p") || !containsArgPair(request.Args, "--output-format", "stream-json") {
		t.Fatalf("argv = %#v, want shell-free -p print mode with stream-json output", request.Args)
	}
	assertAgyFinalOnlyCompletion(t, events, responseEvents, "agy functional answer COMPLETE")
}

// TestAgyNamedMockPreservesDispatchMetadataAndCompletionLog proves the
// Antigravity provider follows the same Workers-owned command projection as
// the other provider adapters. A named mock must intercept the Agy command,
// retain the configured exit code in diagnostics, and carry the source work
// correlation into command_runner.completed without invoking the live edge.
func TestAgyNamedMockPreservesDispatchMetadataAndCompletionLog(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderAntigravity,
		agyFunctionalModel,
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
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Args: []string{
			"--with-mock-workers", mockConfigPath,
			"--runtime-log-dir", logDir,
		},
		Edges: serviceedges.Edges{ProviderCommandRunner: liveRunner},
	})
	support.WaitForTerminalStatus(t, server.URL(), 5*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want one named Agy mock rejection; listed=%#v", got, listed)
	}
	if liveRunner.CallCount() != 0 {
		t.Fatalf("live Agy edge calls = %d, want zero for named mock dispatch", liveRunner.CallCount())
	}
	server.Stop(t)

	record := findAgyRuntimeLogRecord(t, requireAgyRuntimeLogPath(t, logDir), "command_runner.completed")
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

// TestAgyNativeFailureThroughRootBuildProcessIsSafe proves native Agy failures
// remain safe and observable through the customer process boundary.
func TestAgyNativeFailureThroughRootBuildProcessIsSafe(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderAntigravity,
		agyFunctionalModel,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"agy native failure"}`))

	const leaked = "/tmp/secret-key"
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout:   []byte("authentication failed: token path " + leaked + " leaked"),
		ExitCode: 1,
	})

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		agyFunctionalEdges(runner),
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("completed work = %d, want 0 after native failure", got)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("agy command runner calls = %d, want 1", runner.CallCount())
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Factory events: %v", err)
	}
	payload := string(encoded)
	if strings.Contains(payload, leaked) || strings.Contains(payload, "secret-key") {
		t.Fatalf("Factory events leaked unsafe Agy failure detail: %s", payload)
	}
	assertAgyProviderSession(t, events, factoryapi.InferenceOutcomeFailed, string(modelprovider.ProviderAntigravity))
}

// TestAgyTimeoutFailureThroughRootBuildProcess proves timeout normalization
// through the customer process boundary without leaking partial output.
func TestAgyTimeoutFailureThroughRootBuildProcess(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderAntigravity,
		agyFunctionalModel,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"agy timeout"}`))

	runner := newErroringCommandRunner(context.DeadlineExceeded)

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		agyFunctionalEdges(runner),
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.callCount() < 1 {
		t.Fatalf("agy command runner calls = %d, want at least one retryable timeout attempt", runner.callCount())
	}
	reason := terminalFailureReason(t, events)
	if reason != factoryapi.WorkFailureTypeTimeout {
		t.Fatalf("failure reason = %q, want %q", reason, factoryapi.WorkFailureTypeTimeout)
	}
	assertAgyProviderSession(t, events, factoryapi.InferenceOutcomeFailed, string(modelprovider.ProviderAntigravity))
}

// TestAgyCommandCancellationThroughRootBuildProcessIsCanonical proves
// cancellation returns the canonical outcome through the Providers command
// adapter.
func TestAgyCommandCancellationThroughRootBuildProcessIsCanonical(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.WriteAgentConfig(t, dir, "worker", support.BuildModelWorkerConfig(
		modelprovider.ProviderAntigravity,
		agyFunctionalModel,
	))
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"title":"agy command cancel"}`))

	runner := newErroringCommandRunner(context.Canceled)

	_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		agyFunctionalEdges(runner),
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("failed work = %d, want 1; listed=%#v", got, listed)
	}
	if runner.callCount() != 1 {
		t.Fatalf("agy command runner calls = %d, want 1", runner.callCount())
	}
	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal Factory events: %v", err)
	}
	payload := string(encoded)
	if !strings.Contains(payload, "provider invocation was canceled") {
		t.Fatalf("Factory events missing canonical cancellation outcome: %s", payload)
	}
}

func agyFunctionalEdges(runner platformprocess.CommandRunner) serviceedges.Edges {
	return serviceedges.Edges{ProviderCommandRunner: runner}
}

// erroringCommandRunner is a test double proving Providers command-runner
// native errors, such as context deadline or cancellation, reach the same
// canonical outcome as a real subprocess failure.
type erroringCommandRunner struct {
	mu       sync.Mutex
	err      error
	requests []platformprocess.CommandRequest
}

func newErroringCommandRunner(err error) *erroringCommandRunner {
	return &erroringCommandRunner{err: err}
}

func (r *erroringCommandRunner) Run(
	_ context.Context,
	req platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requests = append(r.requests, req)
	return platformprocess.CommandResult{}, r.err
}

func (r *erroringCommandRunner) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.requests)
}

func assertAgyFinalOnlyCompletion(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	responseEvents []factoryapi.FactoryResponseEvent,
	wantOutput string,
) {
	t.Helper()

	var completedMessages int
	for _, event := range responseEvents {
		switch event.Kind {
		case factoryapi.FactoryResponseEventKindMessage:
			if event.Phase == factoryapi.FactoryResponseEventPhaseDelta {
				t.Fatalf("final-only Agy replay fabricated message delta: %#v", event)
			}
			if event.Phase == factoryapi.FactoryResponseEventPhaseCompleted {
				completedMessages++
			}
		case factoryapi.FactoryResponseEventKindTool, factoryapi.FactoryResponseEventKindUsage:
			t.Fatalf("final-only Agy replay fabricated lifecycle: %#v", event)
		}
	}
	if completedMessages != 1 {
		t.Fatalf("completed message events = %d, want exactly one terminal result", completedMessages)
	}

	dispatchOutput := ""
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Output != nil && *payload.Output != "" {
			dispatchOutput = *payload.Output
		}
	}
	if dispatchOutput != wantOutput {
		t.Fatalf("dispatch output = %q, want %q", dispatchOutput, wantOutput)
	}
}

func assertAgyProviderSession(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	wantOutcome factoryapi.InferenceOutcome,
	wantProvider string,
) {
	t.Helper()

	var found bool
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.Outcome != wantOutcome {
			continue
		}
		if payload.ProviderSession == nil || payload.ProviderSession.Provider == nil {
			t.Fatal("inference response missing provider session metadata")
		}
		if got := support.StringPointerValue(payload.ProviderSession.Provider); got != wantProvider {
			t.Fatalf("provider session provider = %q, want %q", got, wantProvider)
		}
		found = true
		break
	}
	if !found {
		t.Fatalf("missing inference response with outcome %q", wantOutcome)
	}
}

func terminalFailureReason(t *testing.T, events []factoryapi.FactoryEvent) factoryapi.WorkFailureType {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelResponse {
			continue
		}
		payload, err := support.AsInferenceResponseObservation(event)
		if err != nil {
			t.Fatalf("decode inference response: %v", err)
		}
		if payload.Outcome == factoryapi.InferenceOutcomeFailed && payload.FailureDetail != nil {
			return payload.FailureDetail.Reason
		}
	}
	t.Fatal("missing failed inference response with failure detail")
	return ""
}

func containsArg(args []string, expected string) bool {
	for _, arg := range args {
		if arg == expected {
			return true
		}
	}
	return false
}

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}

func requireAgyRuntimeLogPath(t *testing.T, logDir string) string {
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

func findAgyRuntimeLogRecord(t *testing.T, path, eventName string) map[string]any {
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
