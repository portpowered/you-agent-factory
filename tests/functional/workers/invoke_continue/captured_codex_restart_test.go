package acceptance

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Both bindings share the production graph in each epoch. Reconstruction is
// sequential because the recording writer must join before the same profile
// reopens; independent owned profiles run in parallel with the other tests.
func TestCapturedCodexToolJoinedRestart(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	host, home, err := prepareInvokeContinuePackageRoot(t, root)
	if err != nil {
		t.Fatal(err)
	}
	route := &invokeContinueStaticCommandRoute{}
	setup := invokeContinueScenarioSetup{}
	for _, binding := range []string{"direct", "factory"} {
		runner := &recordingStreamingRetryRunner{newInvokeContinueResettableProviderCommandRunner(platformprocess.CommandResult{Stdout: capturedCodexToolOutput()})}
		if err := appendInvokeContinueScenario(root, &setup.scenarios, &route.routes, binding, runner, runner, nil, nil, nil, nil); err != nil {
			t.Fatal(err)
		}
	}
	first := startContinuationRestartHost(t, root, host, home, route)
	before := make([]summaryRestartSnapshot, len(setup.scenarios))
	t.Run("capture", func(t *testing.T) {
		for index, cell := range setup.scenarios {
			t.Run(cell.name, func(t *testing.T) {
				t.Parallel()
				scenario := cell
				scenario.homeDirectory = home
				scenario.fixture = &invokeContinuePackageFixture{process: first.process, baseURL: first.baseURL}
				id := captureCodexRestartBinding(t, &scenario)
				awaitContinuationRestartLogs(t, first, home, scenario.workingDirectory, id, "captured-tools-thread")
				assertCapturedCodexToolReads(t, t.Context(), scenario.fixture, &scenario, id)
				// Each child writes its own slot, not a shared map.
				before[index] = readSummaryRestartSnapshot(t, first, home, scenario.workingDirectory, id)
			})
		}
	})
	if err := first.command.stop(); err != nil {
		t.Fatal(err)
	}
	if err := first.process.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".codex", ".cursor", ".claude"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte("provider history unavailable"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	fresh := startContinuationRestartHost(t, root, host, home, route)
	for index, cell := range setup.scenarios {
		scenario := cell
		scenario.homeDirectory = home
		scenario.fixture = &invokeContinuePackageFixture{process: fresh.process, baseURL: fresh.baseURL}
		want := before[index]
		id := want.observation.WorkerSessionId
		got := readSummaryRestartSnapshot(t, fresh, home, cell.workingDirectory, id)
		want.observation.ConfirmationState = got.observation.ConfirmationState
		if !reflect.DeepEqual(want, got) || cell.providerRunner.CallCount() != 1 {
			t.Fatal("joined restart changed acknowledged mixed-content facts or reran provider")
		}
		assertCapturedCodexToolReads(t, t.Context(), scenario.fixture, &scenario, id)
	}
}

func captureCodexRestartBinding(t *testing.T, scenario *invokeContinueScenario) string {
	t.Helper()
	fixture := scenario.fixture
	if scenario.name == "direct" {
		id := "joined-direct-tools"
		path := capturedCodexExecutionPath(t, scenario, id)
		summaryRestartCLI(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, scenario.homeDirectory, scenario.workingDirectory, "invoke", "--execution", path)
		return id
	}
	capturedCodexFactory(t, scenario.workingDirectory)
	opened := support.OpenFactorySessionAt(t, fixture.baseURL, scenario.workingDirectory)
	t.Cleanup(func() { support.CloseFactorySessionAt(t, fixture.baseURL, opened.Session.Id) })
	item := support.SubmitSessionWorkAt(t, fixture.baseURL, opened.Session.Id, api.SubmitWorkRequest{WorkTypeName: "task", Payload: "joined tool capture"})
	if item.WorkId == nil {
		t.Fatal("joined Factory submission lost Work identity")
	}
	support.WaitForSessionTerminalStatus(t, fixture.baseURL, opened.Session.Id, 30*time.Second)
	rows := support.GetJSON[api.ListWorkerSessionsResponse](t, fixture.baseURL+"/factory-sessions/"+opened.Session.Id+"/worker-sessions?workId="+*item.WorkId)
	if len(rows.Sessions) != 1 || rows.Sessions[0].FactorySessionId == nil || *rows.Sessions[0].FactorySessionId != opened.Session.Id {
		t.Fatal("joined Factory capture lost scope")
	}
	return rows.Sessions[0].WorkerSessionId
}

func TestCapturedCodexToolMalformedItem(t *testing.T) {
	t.Parallel()
	fixture := ensureInvokeContinuePackageFixture(t)
	scenario := fixture.scenario(t, "captured-codex-malformed")
	defer scenario.close(t)
	id := scenarioScopedID(scenario, "malformed")
	path := capturedCodexExecutionPath(t, scenario, id)
	request := t7RemoteCLIInputs(scenario, t.Context(), fixture.baseURL, "invoke", "--execution", path)
	if err := fixture.process.Execute(request.Input); err == nil {
		t.Fatal("malformed native item reported success")
	}
	admittedCalls := scenario.providerRunner.CallCount()
	awaitContinuationRestartLogs(t, invokeContinueStartedProcess{process: fixture.process, baseURL: fixture.baseURL}, scenario.homeDirectory, scenario.workingDirectory, id, "malformed-thread")
	page := support.GetJSON[api.WorkerSessionLogPage](t, fixture.baseURL+"/worker-sessions/"+id+"/logs")
	row := requesterObservation(t, fixture, scenario, t.Context(), id)
	// COMPLETE measures the admitted recording, including its safe omission
	// diagnostic and failed terminal. The invalid item was never admitted as
	// tool content; it must not become a successful execution or native tool.
	if row.State != "FAILED" || row.Failure == nil || row.TokenUsage != nil || len(page.Events) == 0 {
		t.Fatal("malformed item invented successful execution or usage")
	}
	assertCapturedCodexMalformedReads(t, fixture, scenario, id)
	if admittedCalls == 0 || scenario.providerRunner.CallCount() != admittedCalls {
		t.Fatal("malformed readback reran provider")
	}
}

func assertCapturedCodexMalformedReads(t *testing.T, fixture *invokeContinuePackageFixture, scenario *invokeContinueScenario, id string) {
	t.Helper()
	for _, view := range []string{"logs", "transcript"} {
		status, body := t7HTTP(t, t.Context(), http.MethodGet, fixture.baseURL+"/worker-sessions/"+id+"/"+view, nil)
		read := t7RemoteCLIInputs(scenario, t.Context(), fixture.baseURL, "read", "--worker-session-id", id, "--view", view)
		if status != http.StatusOK || fixture.process.Execute(read.Input) != nil {
			t.Fatal("malformed capture prefix unavailable")
		}
		var cli, httpValue, mcp any
		if json.Unmarshal([]byte(body), &httpValue) != nil || json.Unmarshal([]byte(read.Stdout()), &cli) != nil || json.Unmarshal(capturedCodexMCPRead(t, t.Context(), fixture, scenario, id, view), &mcp) != nil || !reflect.DeepEqual(cli, httpValue) || !reflect.DeepEqual(mcp, httpValue) {
			t.Fatal("malformed prefix public parity differs")
		}
		if strings.Contains(body, "malformed-private") || strings.Contains(body, "captured-credential") || strings.Contains(body, `"kind":"TOOL"`) || strings.Contains(body, "tool_call") {
			t.Fatal("malformed item leaked private data or invented a tool")
		}
	}
}

func capturedCodexMalformedOutput() []byte {
	return []byte(`{"type":"thread.started","thread_id":"malformed-thread"}
{"type":"item.completed","item":{"id":"malformed-private","type":"command_execution","command":{"password":"captured-credential"},"aggregated_output":"malformed-private"}}
`)
}
