package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestWorkerSessionsFleetListCLIConcurrent observes several sessions held in
// flight at the same time, then compares the terminal fleet projection through
// both the CLI and HTTP surfaces. The gate makes the active-state assertion
// deterministic without relying on timing or the live daemon.
func TestWorkerSessionsFleetListCLIConcurrent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	factoryDir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "executor_success"))
	support.ClearSeedInputs(t, factoryDir)
	support.WriteAgentConfig(t, factoryDir, "worker", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "fixture-model"))
	writeWorkerSessionRouteWorkstation(t, factoryDir)
	homeDir := t.TempDir()
	gate := make(chan struct{})
	successStdout := readProviderFixture(t, "codex", "success", "stdout.jsonl")
	successRollout := readProviderFixture(t, "codex", "success", "rollout.jsonl")
	fleetProviderSessionIDs := map[string]string{
		"worker-session-fleet-alpha": "session_fixture_codex_fleet_alpha",
		"worker-session-fleet-beta":  "session_fixture_codex_fleet_beta",
		"worker-session-fleet-gamma": "session_fixture_codex_fleet_gamma",
	}
	routes := make(map[string]platformprocess.CommandResult, len(fleetProviderSessionIDs))
	for workName, providerSessionID := range fleetProviderSessionIDs {
		stdout := bytes.ReplaceAll(successStdout, []byte(workerSessionsCodexSuccessID), []byte(providerSessionID))
		rollout := bytes.ReplaceAll(successRollout, []byte(workerSessionsCodexSuccessID), []byte(providerSessionID))
		writeCodexRollout(t, homeDir, providerSessionID, rollout)
		routes[workName] = platformprocess.CommandResult{Stdout: stdout}
	}
	runner := newProviderCommandRouteRunner(routes, gate)
	api := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		APIServerStarter:                    api.Start,
		ProviderCommandRunner:               runner,
		ProviderSessionResolveHomeDirectory: func() (string, error) { return homeDir, nil },
	})
	support.CleanupProcess(t, process)

	env := functionalEnvironment(homeDir)
	serverInputs := support.FakeInputs(ctx, []string{
		"you", "run", "--dir", factoryDir, "--continuously", "--with-server", "--quiet",
	})
	serverInputs.Input.Env = env
	serverInputs.Input.WorkingDirectory = factoryDir
	server := support.StartProcessCommand(t, process, serverInputs.Input)
	baseURL := api.WaitForURL(t)
	factorySessionID := openExplicitWorkerSession(t, baseURL, factoryDir)
	var releaseGate sync.Once
	release := func() { releaseGate.Do(func() { close(gate) }) }
	defer func() {
		release()
		support.CloseFactorySessionAt(t, baseURL, factorySessionID)
		assertFactorySessionAbsent(t, baseURL, factorySessionID, factoryDir)
		server.Stop(t)
		if err := server.Err(); err != nil {
			t.Errorf("server Process.Execute: %v\nstdout:\n%s\nstderr:\n%s", err, serverInputs.Stdout(), serverInputs.Stderr())
		}
	}()

	expectedWorks := submitFleetWorks(t, ctx, process, env, factoryDir, baseURL, factorySessionID)
	providerIDs := make(map[string]string, len(expectedWorks))
	for workID, workName := range expectedWorks {
		providerIDs[workID] = fleetProviderSessionIDs[workName]
	}
	if err := runner.WaitForCalls(ctx, len(expectedWorks)); err != nil {
		t.Fatalf("wait for fleet provider dispatches: %v", err)
	}
	assertFleetState(t, waitForFleetWorkerSessionsState(t, ctx, process, env, factoryDir, baseURL, "RUNNING", len(expectedWorks)), expectedWorks, factorySessionID, providerIDs, "RUNNING")
	release()
	assertFleetState(t, waitForFleetWorkerSessionsState(t, ctx, process, env, factoryDir, baseURL, "COMPLETED", len(expectedWorks)), expectedWorks, factorySessionID, providerIDs, "COMPLETED")
	factorySessionIDs := make(map[string]string, len(expectedWorks))
	for workID := range expectedWorks {
		factorySessionIDs[workID] = factorySessionID
	}
	assertFleetWorkerSessionList(t, ctx, process, env, factoryDir, baseURL, factorySessionIDs, expectedWorks, providerIDs, true)
	assertProviderCommandRoutes(t, runner, map[string]struct{}{
		"worker-session-fleet-alpha": {},
		"worker-session-fleet-beta":  {},
		"worker-session-fleet-gamma": {},
	})
}

func submitFleetWorks(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, factorySessionID string) map[string]string {
	t.Helper()
	expectedWorks := make(map[string]string, 3)
	for _, name := range []string{"worker-session-fleet-alpha", "worker-session-fleet-beta", "worker-session-fleet-gamma"} {
		workID := submitWork(t, ctx, process, env, factoryDir, baseURL, factorySessionID, name)
		expectedWorks[workID] = name
	}
	return expectedWorks
}

func assertFleetState(t *testing.T, sessions []workerSessionJSON, expectedWorks map[string]string, factorySessionID string, providerIDs map[string]string, state string) {
	t.Helper()
	for _, session := range sessions {
		if session.WorkID == nil || session.WorkName == nil || session.StartedAt == nil || session.DurationMillis == nil {
			t.Fatalf("%s fleet observation omitted Work or timing facts: %#v", state, session)
		}
		if want, ok := expectedWorks[*session.WorkID]; !ok || *session.WorkName != want || session.State != state {
			t.Fatalf("%s fleet observation attribution = %#v, expected Work map %#v", state, session, expectedWorks)
		}
		if session.FactorySessionID == nil || *session.FactorySessionID != factorySessionID {
			t.Fatalf("%s fleet observation Factory Session = %#v, want %s", state, session.FactorySessionID, factorySessionID)
		}
		if state == "COMPLETED" && (session.ProviderSession == nil || session.ProviderSession.Provider != "codex" || session.ProviderSession.Kind != "session_id" || session.ProviderSession.ID != providerIDs[*session.WorkID]) {
			t.Fatalf("%s fleet observation provider identity = %#v, want %s", state, session.ProviderSession, providerIDs[*session.WorkID])
		}
		if state == "COMPLETED" && *session.DurationMillis < 0 {
			t.Fatalf("terminal fleet observation duration = %d, want non-negative", *session.DurationMillis)
		}
	}
}

func assertFleetWorkerSessionList(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL string, factorySessionIDs map[string]string, expectedWorks map[string]string, providerIDs map[string]string, requireProviderSession bool) {
	t.Helper()
	cliList := fetchFleetCLIList(t, ctx, process, env, factoryDir, baseURL, 10)
	if len(cliList.Sessions) != len(expectedWorks) {
		t.Fatalf("fleet CLI session count = %d, want %d: %#v", len(cliList.Sessions), len(expectedWorks), cliList)
	}
	byID := assertFleetCLIObservations(t, cliList, factorySessionIDs, providerIDs, requireProviderSession)
	assertFleetWorkAttribution(t, cliList, expectedWorks)
	assertFleetCLIOutputLimit(t, ctx, process, env, factoryDir, baseURL)
	assertFleetHTTPMatchesCLI(t, ctx, baseURL, cliList, byID)
}

func fetchFleetCLIList(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL string, limit int) workerSessionListJSON {
	t.Helper()
	inputs := executeCLI(t, ctx, process, env, factoryDir,
		"--server", baseURL, "worker-sessions", "list",
		"--state", "COMPLETED", "--state", "FAILED", "--limit", fmt.Sprint(limit), "--output", "json")
	var result workerSessionListJSON
	decodeCLIJSON(t, inputs, &result)
	return result
}

func assertFleetCLIObservations(t *testing.T, list workerSessionListJSON, factorySessionIDs map[string]string, providerIDs map[string]string, requireProviderSession bool) map[string]workerSessionJSON {
	t.Helper()
	byID := make(map[string]workerSessionJSON, len(list.Sessions))
	for _, session := range list.Sessions {
		if session.WorkerSessionID == "" || session.WorkID == nil || session.WorkName == nil || session.StartedAt == nil || session.DurationMillis == nil {
			t.Fatalf("fleet CLI observation omitted required attribution/timing facts: %#v", session)
		}
		if requireProviderSession && (session.ProviderSession == nil || session.ProviderSession.Provider != "codex" || session.ProviderSession.Kind != "session_id") {
			t.Fatalf("fleet CLI observation omitted provider/kind: %#v", session)
		}
		wantFactorySessionID, ok := factorySessionIDs[*session.WorkID]
		if !ok || session.FactorySessionID == nil || *session.FactorySessionID != wantFactorySessionID {
			t.Fatalf("fleet CLI observation Factory Session = %#v, want %s for Work %s", session.FactorySessionID, wantFactorySessionID, *session.WorkID)
		}
		if requireProviderSession && session.WorkID != nil && session.ProviderSession.ID != providerIDs[*session.WorkID] {
			t.Fatalf("fleet CLI observation provider identity = %#v, want %s", session.ProviderSession, providerIDs[*session.WorkID])
		}
		if session.State != "COMPLETED" && session.State != "FAILED" {
			t.Fatalf("fleet CLI observation state = %q, want terminal state", session.State)
		}
		assertFleetFailureKind(t, session)
		byID[session.WorkerSessionID] = session
	}
	return byID
}

func assertFleetFailureKind(t *testing.T, session workerSessionJSON) {
	t.Helper()
	if session.State != "FAILED" {
		return
	}
	var failure struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(session.Failure, &failure); err != nil || strings.TrimSpace(failure.Kind) == "" {
		t.Fatalf("fleet CLI failed observation omitted failure kind: %s", session.Failure)
	}
}

func assertFleetWorkAttribution(t *testing.T, list workerSessionListJSON, expectedWorks map[string]string) {
	t.Helper()
	for workID, workName := range expectedWorks {
		if !fleetListContainsWork(list, workID, workName) {
			t.Fatalf("fleet CLI list omitted Work attribution %s/%s: %#v", workID, workName, list)
		}
	}
}

func fleetListContainsWork(list workerSessionListJSON, workID, workName string) bool {
	for _, session := range list.Sessions {
		if session.WorkID != nil && session.WorkName != nil && *session.WorkID == workID && *session.WorkName == workName {
			return true
		}
	}
	return false
}

func assertFleetCLIOutputLimit(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL string) {
	t.Helper()
	limited := fetchFleetCLIList(t, ctx, process, env, factoryDir, baseURL, 1)
	if len(limited.Sessions) != 1 {
		t.Fatalf("fleet CLI limit result count = %d, want 1: %#v", len(limited.Sessions), limited)
	}
}

func assertFleetHTTPMatchesCLI(t *testing.T, ctx context.Context, baseURL string, cliList workerSessionListJSON, byID map[string]workerSessionJSON) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(baseURL, "/")+"/worker-sessions?state=COMPLETED&state=FAILED&limit=10", nil)
	if err != nil {
		t.Fatalf("build fleet HTTP request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("fleet HTTP list request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("fleet HTTP list status = %d, want 200", response.StatusCode)
	}
	var apiList factoryapi.ListWorkerSessionsResponse
	if err := json.NewDecoder(response.Body).Decode(&apiList); err != nil {
		t.Fatalf("decode fleet HTTP list: %v", err)
	}
	if len(apiList.Sessions) != len(cliList.Sessions) {
		t.Fatalf("fleet HTTP session count = %d, CLI count = %d", len(apiList.Sessions), len(cliList.Sessions))
	}
	for _, session := range apiList.Sessions {
		cliSession, ok := byID[session.WorkerSessionId]
		if !ok {
			t.Fatalf("fleet HTTP returned Worker Session %q absent from CLI list", session.WorkerSessionId)
		}
		if session.WorkId == nil || session.WorkName == nil || *session.WorkId != *cliSession.WorkID || *session.WorkName != *cliSession.WorkName {
			t.Fatalf("fleet HTTP/CLI Work attribution mismatch for %s: HTTP=%#v CLI=%#v", session.WorkerSessionId, session, cliSession)
		}
		if session.FactorySessionId == nil || cliSession.FactorySessionID == nil || *session.FactorySessionId != *cliSession.FactorySessionID {
			t.Fatalf("fleet HTTP/CLI Factory Session mismatch for %s: HTTP=%#v CLI=%#v", session.WorkerSessionId, session, cliSession)
		}
		if session.ProviderSession == nil || cliSession.ProviderSession == nil || session.ProviderSession.Id != cliSession.ProviderSession.ID {
			t.Fatalf("fleet HTTP/CLI provider-session mismatch for %s: HTTP=%#v CLI=%#v", session.WorkerSessionId, session, cliSession)
		}
	}
}

func waitForFleetWorkerSessionsState(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, state string, count int) []workerSessionJSON {
	t.Helper()
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var lastOutput string
	for {
		inputs := support.FakeInputs(ctx, []string{
			"you", "--server", baseURL, "worker-sessions", "list", "--state", state, "--limit", "20", "--output", "json",
		})
		inputs.Input.Env = append([]string(nil), env...)
		inputs.Input.WorkingDirectory = factoryDir
		if err := process.Execute(inputs.Input); err == nil {
			var listed workerSessionListJSON
			if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &listed); decodeErr == nil {
				lastOutput = inputs.Stdout()
				if len(listed.Sessions) >= count {
					return listed.Sessions
				}
			}
		} else {
			lastOutput = inputs.Stdout() + "\n" + inputs.Stderr() + "\n" + err.Error()
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d fleet Worker Sessions in %s: %s", count, state, lastOutput)
		case <-ctx.Done():
			t.Fatalf("waiting for fleet Worker Sessions in %s canceled: %v", state, ctx.Err())
		}
	}
}
