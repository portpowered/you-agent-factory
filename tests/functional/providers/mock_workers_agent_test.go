package providers

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestMockWorkers_AgentDefaultAcceptMovesWorkToOutputPlace(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     sharedMockAgentAcceptWorkID,
		WorkTypeID: "task",
		TraceID:    "trace-shared-mock-agent-accept",
		Payload:    []byte("mock accept payload"),
	})

	scenario, listed := runSharedMockFactory(t, dir, 5*time.Second)
	for placeID, want := range map[string]int{"task:done": 1, "task:init": 0} {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
	scenario.stop(t)
}

func TestMockWorkers_AgentRejectConfigRoutesFailureWithoutLoggingCommandOutput(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_with_arcs"))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     sharedMockAgentRejectWorkID,
		WorkTypeID: "task",
		TraceID:    "trace-shared-mock-agent-reject",
		Payload:    []byte("mock reject payload"),
	})
	scenario, _ := runSharedMockFactory(t, dir, 5*time.Second)
	assertMockAgentRejected(t, scenario)
	fixture := scenario.fixture
	scenario.stop(t)

	record := findSharedRuntimeLogRecord(t, fixture, dir, 7)
	if record["exit_code"] != float64(7) {
		t.Fatalf("logged exit_code = %#v, want 7", record["exit_code"])
	}
	if _, ok := record["stdout"]; ok {
		t.Fatalf("failure record unexpectedly included stdout: %#v", record)
	}
	if _, ok := record["stderr"]; ok {
		t.Fatalf("failure record unexpectedly included stderr: %#v", record)
	}
}

func TestMockWorkers_AgentRejectConfigWithZeroExitCodeIsRejectedAtCustomerBoundary(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_with_arcs"))
	testutil.WriteSeedFile(t, dir, "task", []byte("mock reject zero exit payload"))
	exitCode := 0

	configPath := support.WriteMockWorkersConfig(t, rejectedAgentMockConfig(exitCode))
	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", dir, "--with-mock-workers", configPath, "--no-record",
	})
	err := process.Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), "rejectConfig.exitCode must be between 1 and 255") {
		t.Fatalf("Process.Execute() error = %v, want public exit-code validation; stderr=%q", err, inputs.Stderr())
	}
}

func rejectedAgentMockConfig(exitCode int) *workers.MockWorkersConfig {
	return &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "worker",
			WorkstationName: "process",
			RunType:         workers.MockWorkerRunTypeReject,
			RejectConfig: &workers.MockWorkerRejectConfig{
				Stdout: "configured stdout", Stderr: "configured stderr", ExitCode: &exitCode,
			},
		}},
	}
}

func assertMockAgentRejected(t *testing.T, scenario *sharedProviderScenario) {
	t.Helper()
	listed := scenario.listWork(t)
	for placeID, want := range map[string]int{"task:failed": 1, "task:init": 0} {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}
	for _, event := range scenario.factoryEvents(t) {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Outcome != factoryapi.WorkOutcomeFailed ||
			payload.ProviderFailure == nil ||
			payload.ProviderFailure.Type == nil ||
			string(*payload.ProviderFailure.Type) != string(workerexecution.WorkFailureTypePermanentBadRequest) ||
			payload.Error == nil ||
			!strings.Contains(*payload.Error, "provider error: permanent_bad_request: provider rejected the execution request") {
			t.Fatalf("dispatch response = %#v, want neutral terminal provider refusal", payload)
		}
		return
	}
	t.Fatal("Factory Event history did not contain dispatch response")
}

func requireAnyRuntimeLogPath(t *testing.T, logDir string) string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(logDir, "*", "*", "*", "*-runtime-log-*.log"))
	if err != nil {
		t.Fatalf("glob runtime log path: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("runtime log paths under %s = %v, want exactly one", logDir, matches)
	}
	return matches[0]
}
