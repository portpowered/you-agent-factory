package providers

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestMockWorkers_ScriptDefaultAcceptProducesSuccessfulScriptResult(t *testing.T) {
	support.SkipLongFunctional(t, "slow mock-worker script accept sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     sharedMockScriptAcceptWorkID,
		WorkTypeID: "task",
		TraceID:    "trace-shared-mock-script-accept",
		Payload:    []byte("mock script accept payload"),
	})

	scenario, listed := runSharedMockFactory(t, dir, 5*time.Second)
	assertScriptMockPlaces(t, listed, "done")
	assertListedWorkText(t, listed, "task", "done", "mock worker accepted")
	scenario.stop(t)
}

func TestMockWorkers_ScriptRejectConfigRoutesFailureAndLogsCommandOutput(t *testing.T) {
	support.SkipLongFunctional(t, "slow mock-worker script reject sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkID:     sharedMockScriptRejectWorkID,
		WorkTypeID: "task",
		TraceID:    "trace-shared-mock-script-reject",
		Payload:    []byte("mock script reject payload"),
	})
	scenario, _ := runSharedMockFactory(t, dir, 5*time.Second)
	assertScriptMockRejected(t, scenario)
	fixture := scenario.fixture
	scenario.stop(t)

	record := findSharedRuntimeLogRecord(t, fixture, dir, 9)
	if record["exit_code"] != float64(9) {
		t.Fatalf("logged exit_code = %#v, want 9", record["exit_code"])
	}
	if _, ok := record["stdout"]; ok {
		t.Fatalf("completion log unexpectedly included stdout: %#v", record["stdout"])
	}
	if _, ok := record["stderr"]; ok {
		t.Fatalf("completion log unexpectedly included stderr: %#v", record["stderr"])
	}
}

func TestMockWorkers_ScriptRejectConfigWithZeroExitCodeStillRoutesFailure(t *testing.T) {
	support.SkipLongFunctional(t, "slow mock-worker zero-exit rejection sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("mock script reject zero exit payload"))
	exitCode := 0

	configPath := support.WriteMockWorkersConfig(t, rejectedScriptMockConfig(exitCode))
	process := support.BuildProcess(t, serviceedges.Edges{})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--dir", dir, "--with-mock-workers", configPath, "--no-record",
	})
	err := process.Execute(inputs.Input)
	if err == nil || !strings.Contains(err.Error(), "rejectConfig.exitCode must be between 1 and 255") {
		t.Fatalf("Process.Execute() error = %v, want public exit-code validation; stderr=%q", err, inputs.Stderr())
	}
}

func TestMockWorkers_ScriptConfigExecutesCommandRunnerSideEffect(t *testing.T) {
	support.SkipLongFunctional(t, "slow mock-worker command-runner side-effect sweep")
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("mock script command payload"))
	sideEffectPath := filepath.Join(t.TempDir(), "mock-script-side-effect.txt")

	configPath := support.WriteMockWorkersConfig(t, &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName: "script-worker", WorkstationName: "run-script",
			RunType: workers.MockWorkerRunTypeScript,
			ScriptConfig: &workers.MockWorkerScriptConfig{
				Command: os.Args[0],
				Args: []string{
					"-test.run=TestMockWorkers_ScriptHelper", "--",
					"write-file", sideEffectPath, "script side effect",
				},
			},
		}},
	})
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir, Args: []string{"--with-mock-workers", configPath},
	})
	defer server.Stop(t)
	support.WaitForTerminalStatus(t, server.URL(), 5*time.Second)
	assertScriptMockPlaces(t, support.ListDefaultSessionWork(t, server.URL()), "done")

	raw, err := os.ReadFile(sideEffectPath)
	if err != nil {
		t.Fatalf("read mock script side effect: %v", err)
	}
	if string(raw) != "script side effect" {
		t.Fatalf("side effect content = %q, want %q", raw, "script side effect")
	}
	assertListedWorkText(t, support.ListDefaultSessionWork(t, server.URL()), "task", "done", "mock script helper wrote file")
}

func rejectedScriptMockConfig(exitCode int) *workers.MockWorkersConfig {
	return &workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		WorkerName: "script-worker", WorkstationName: "run-script",
		RunType: workers.MockWorkerRunTypeReject,
		RejectConfig: &workers.MockWorkerRejectConfig{
			Stdout: "script configured stdout", Stderr: "script configured stderr", ExitCode: &exitCode,
		},
	}}}
}

func assertScriptMockPlaces(t *testing.T, listed factoryapi.ListWorkResponse, terminalState string) {
	t.Helper()
	for _, state := range []string{"init", "done", "failed"} {
		want := 0
		if state == terminalState {
			want = 1
		}
		if got := support.CountWorkAtCustomerState(listed, "task:"+state); got != want {
			t.Errorf("task:%s token count = %d, want %d", state, got, want)
		}
	}
}

func assertScriptMockRejected(t *testing.T, scenario *sharedProviderScenario) {
	t.Helper()
	assertScriptMockPlaces(t, scenario.listWork(t), "failed")
	for _, event := range scenario.factoryEvents(t) {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Outcome != factoryapi.WorkOutcomeFailed || payload.Error == nil ||
			!strings.Contains(*payload.Error, "script configured stderr") {
			t.Fatalf("dispatch response = %#v, want configured script failure", payload)
		}
		return
	}
	t.Fatal("Factory Event history did not contain dispatch response")
}

func TestMockWorkers_ScriptHelper(t *testing.T) {
	support.SkipLongFunctional(t, "slow mock-worker helper sweep")
	if len(os.Args) < 4 {
		return
	}

	mode := os.Args[len(os.Args)-3]
	if mode == "write-file" {
		path := os.Args[len(os.Args)-2]
		content := os.Args[len(os.Args)-1]
		writeMockWorkerScriptHelperFile(path, content)
		return
	}
	if len(os.Args) < 5 {
		return
	}
	mode = os.Args[len(os.Args)-4]
	if mode != "sleep-write-file" {
		return
	}

	sleepMS, err := strconv.Atoi(os.Args[len(os.Args)-3])
	if err != nil {
		fmt.Fprintf(os.Stderr, "parse sleep duration: %v\n", err)
		os.Exit(2)
	}
	path := os.Args[len(os.Args)-2]
	content := os.Args[len(os.Args)-1]
	time.Sleep(time.Duration(sleepMS) * time.Millisecond)
	writeMockWorkerScriptHelperFile(path, content)
}

func writeMockWorkerScriptHelperFile(path string, content string) {
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		fmt.Fprintf(os.Stderr, "write side effect: %v\n", err)
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, "mock script helper wrote file")
	os.Exit(0)
}
