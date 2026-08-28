package cli_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

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

	caseFixture := newWorkerSessionsCLICase(t)
	fixture := caseFixture.fixture
	process := fixture.process
	factoryDir := caseFixture.factoryDir
	env := functionalEnvironment(fixture.homeDir)
	baseURL := fixture.baseURL
	caseFixture.registerRoutes(t, fleetWorkNames()...)
	routeStart := fixture.runner.CallCount()
	fixture.resetFleetGate()
	defer fixture.releaseFleetGate()
	fleetProviderSessionIDs := map[string]string{
		"worker-session-fleet-alpha": "session_fixture_codex_fleet_alpha",
		"worker-session-fleet-beta":  "session_fixture_codex_fleet_beta",
		"worker-session-fleet-gamma": "session_fixture_codex_fleet_gamma",
	}
	factorySessionIDsByName := make(map[string]string, len(fleetWorkNames()))
	for _, name := range fleetWorkNames() {
		factorySessionIDsByName[name] = caseFixture.openSession(t)
	}

	expectedWorks, factorySessionIDs := submitFleetWorks(t, ctx, process, env, factoryDir, baseURL, factorySessionIDsByName)
	providerIDs := make(map[string]string, len(expectedWorks))
	for workID, workName := range expectedWorks {
		providerIDs[workID] = fleetProviderSessionIDs[workName]
	}
	if err := fixture.runner.WaitForCalls(ctx, routeStart+len(expectedWorks)); err != nil {
		t.Fatalf("wait for fleet provider dispatches: %v", err)
	}
	runningOrder := assertFleetState(t, waitForFleetWorkerSessionsState(t, ctx, process, env, factoryDir, baseURL, "RUNNING", len(expectedWorks)), expectedWorks, factorySessionIDs, providerIDs, "RUNNING")
	fixture.releaseFleetGate()
	completedOrder := assertFleetState(t, waitForFleetWorkerSessionsState(t, ctx, process, env, factoryDir, baseURL, "COMPLETED", len(expectedWorks)), expectedWorks, factorySessionIDs, providerIDs, "COMPLETED")
	assertSameWorkerSessionOrder(t, runningOrder, completedOrder, "RUNNING", "COMPLETED")
	assertFleetWorkerSessionList(t, ctx, process, env, factoryDir, baseURL, factorySessionIDs, expectedWorks, providerIDs, true, completedOrder)
	assertFleetWorkerSessionList(t, ctx, process, env, factoryDir, baseURL, factorySessionIDs, expectedWorks, providerIDs, true, completedOrder)
	assertProviderCommandRoutesSince(t, fixture.runner, routeStart, map[string]struct{}{
		"worker-session-fleet-alpha": {},
		"worker-session-fleet-beta":  {},
		"worker-session-fleet-gamma": {},
	})
	caseFixture.closeRoute(t, "worker-session-fleet-alpha")
	caseFixture.closeRoute(t, "worker-session-fleet-beta")
	caseFixture.closeRoute(t, "worker-session-fleet-gamma")
}

func fleetWorkNames() []string {
	return []string{
		"worker-session-fleet-alpha",
		"worker-session-fleet-beta",
		"worker-session-fleet-gamma",
	}
}

func submitFleetWorks(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL string, factorySessionIDsByName map[string]string) (map[string]string, map[string]string) {
	t.Helper()
	expectedWorks := make(map[string]string, 3)
	expectedFactorySessionIDs := make(map[string]string, 3)
	for _, name := range fleetWorkNames() {
		factorySessionID, ok := factorySessionIDsByName[name]
		if !ok || strings.TrimSpace(factorySessionID) == "" {
			t.Fatalf("fleet Work %s has no explicit Factory Session", name)
		}
		workID := submitWork(t, ctx, process, env, factoryDir, baseURL, factorySessionID, name)
		expectedWorks[workID] = name
		expectedFactorySessionIDs[workID] = factorySessionID
	}
	return expectedWorks, expectedFactorySessionIDs
}

func assertFleetState(t *testing.T, sessions []workerSessionJSON, expectedWorks, factorySessionIDs, providerIDs map[string]string, state string) []string {
	t.Helper()
	if len(sessions) != len(expectedWorks) {
		t.Fatalf("%s fleet session count = %d, want %d: %#v", state, len(sessions), len(expectedWorks), sessions)
	}
	order := make([]string, 0, len(sessions))
	seen := make(map[string]struct{}, len(sessions))
	for _, session := range sessions {
		if session.WorkID == nil || session.WorkName == nil || session.StartedAt == nil || session.DurationMillis == nil {
			t.Fatalf("%s fleet observation omitted Work or timing facts: %#v", state, session)
		}
		if want, ok := expectedWorks[*session.WorkID]; !ok || *session.WorkName != want || session.State != state {
			t.Fatalf("%s fleet observation attribution = %#v, expected Work map %#v", state, session, expectedWorks)
		}
		wantFactorySessionID := factorySessionIDs[*session.WorkID]
		if session.FactorySessionID == nil || *session.FactorySessionID != wantFactorySessionID {
			t.Fatalf("%s fleet observation Factory Session = %#v, want %s for Work %s", state, session.FactorySessionID, wantFactorySessionID, *session.WorkID)
		}
		if state == "COMPLETED" && (session.ProviderSession == nil || session.ProviderSession.Provider != "codex" || session.ProviderSession.Kind != "session_id" || session.ProviderSession.ID != providerIDs[*session.WorkID]) {
			t.Fatalf("%s fleet observation provider identity = %#v, want %s", state, session.ProviderSession, providerIDs[*session.WorkID])
		}
		if state == "COMPLETED" && *session.DurationMillis < 0 {
			t.Fatalf("terminal fleet observation duration = %d, want non-negative", *session.DurationMillis)
		}
		if session.WorkerSessionID == "" {
			t.Fatalf("%s fleet observation omitted Worker Session identity: %#v", state, session)
		}
		if _, duplicate := seen[session.WorkerSessionID]; duplicate {
			t.Fatalf("%s fleet observations duplicated Worker Session %q", state, session.WorkerSessionID)
		}
		seen[session.WorkerSessionID] = struct{}{}
		order = append(order, session.WorkerSessionID)
	}
	assertAscendingWorkerSessionOrder(t, order, state)
	return order
}

func assertSameWorkerSessionOrder(t *testing.T, want, got []string, wantState, gotState string) {
	t.Helper()
	if len(want) != len(got) {
		t.Fatalf("%s/%s Worker Session order lengths differ: %d != %d", wantState, gotState, len(want), len(got))
	}
	for index := range want {
		if want[index] != got[index] {
			t.Fatalf("%s/%s Worker Session order differs at position %d: %q != %q", wantState, gotState, index, want[index], got[index])
		}
	}
}

func assertAscendingWorkerSessionOrder(t *testing.T, order []string, state string) {
	t.Helper()
	if !sort.StringsAreSorted(order) {
		t.Fatalf("%s fleet Worker Session order = %v, want ascending identity order", state, order)
	}
}

func assertFleetWorkerSessionList(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL string, factorySessionIDs map[string]string, expectedWorks map[string]string, providerIDs map[string]string, requireProviderSession bool, expectedOrders ...[]string) {
	t.Helper()
	cliList := fetchFleetCLIList(t, ctx, process, env, factoryDir, baseURL, 10)
	if len(cliList.Sessions) != len(expectedWorks) {
		t.Fatalf("fleet CLI session count = %d, want %d: %#v", len(cliList.Sessions), len(expectedWorks), cliList)
	}
	var expectedOrder []string
	if len(expectedOrders) > 1 {
		t.Fatalf("fleet assertion received %d expected Worker Session orders, want at most one", len(expectedOrders))
	}
	if len(expectedOrders) == 1 {
		expectedOrder = append([]string(nil), expectedOrders[0]...)
	} else {
		expectedOrder = workerSessionOrder(t, cliList.Sessions, "CLI terminal")
	}
	assertFleetCLIObservations(t, cliList, factorySessionIDs, providerIDs, requireProviderSession, expectedOrder)
	assertFleetWorkAttribution(t, cliList, expectedWorks)
	assertFleetCLIOutputLimit(t, ctx, process, env, factoryDir, baseURL, expectedOrder)
	assertFleetHTTPMatchesCLI(t, ctx, baseURL, cliList, expectedOrder)
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

func assertFleetCLIObservations(t *testing.T, list workerSessionListJSON, factorySessionIDs map[string]string, providerIDs map[string]string, requireProviderSession bool, expectedOrder []string) {
	t.Helper()
	if len(list.Sessions) != len(expectedOrder) {
		t.Fatalf("fleet CLI session order length = %d, want %d: %#v", len(list.Sessions), len(expectedOrder), list)
	}
	seen := make(map[string]struct{}, len(list.Sessions))
	for index, session := range list.Sessions {
		if session.WorkerSessionID == "" || session.WorkID == nil || session.WorkName == nil || session.StartedAt == nil || session.DurationMillis == nil {
			t.Fatalf("fleet CLI observation omitted required attribution/timing facts: %#v", session)
		}
		if session.WorkerSessionID != expectedOrder[index] {
			t.Fatalf("fleet CLI Worker Session at position %d = %q, want %q", index, session.WorkerSessionID, expectedOrder[index])
		}
		if _, duplicate := seen[session.WorkerSessionID]; duplicate {
			t.Fatalf("fleet CLI observations duplicated Worker Session %q", session.WorkerSessionID)
		}
		seen[session.WorkerSessionID] = struct{}{}
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
	}
	assertAscendingWorkerSessionOrder(t, expectedOrder, "CLI terminal")
}

func workerSessionOrder(t *testing.T, sessions []workerSessionJSON, state string) []string {
	t.Helper()
	order := make([]string, 0, len(sessions))
	seen := make(map[string]struct{}, len(sessions))
	for _, session := range sessions {
		if session.WorkerSessionID == "" {
			t.Fatalf("%s fleet observation omitted Worker Session identity: %#v", state, session)
		}
		if _, duplicate := seen[session.WorkerSessionID]; duplicate {
			t.Fatalf("%s fleet observations duplicated Worker Session %q", state, session.WorkerSessionID)
		}
		seen[session.WorkerSessionID] = struct{}{}
		order = append(order, session.WorkerSessionID)
	}
	assertAscendingWorkerSessionOrder(t, order, state)
	return order
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

func assertFleetCLIOutputLimit(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL string, expectedOrder []string) {
	t.Helper()
	limited := fetchFleetCLIList(t, ctx, process, env, factoryDir, baseURL, 1)
	if len(limited.Sessions) != 1 {
		t.Fatalf("fleet CLI limit result count = %d, want 1: %#v", len(limited.Sessions), limited)
	}
	if len(expectedOrder) == 0 || limited.Sessions[0].WorkerSessionID != expectedOrder[0] {
		t.Fatalf("fleet CLI limit Worker Session = %q, want first deterministic observation %q", limited.Sessions[0].WorkerSessionID, expectedOrder[0])
	}
}

func assertFleetHTTPMatchesCLI(t *testing.T, ctx context.Context, baseURL string, cliList workerSessionListJSON, expectedOrder []string) {
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
	for index, session := range apiList.Sessions {
		cliSession := cliList.Sessions[index]
		if session.WorkerSessionId != expectedOrder[index] || session.WorkerSessionId != cliSession.WorkerSessionID {
			t.Fatalf("fleet HTTP/CLI Worker Session at position %d = HTTP %q, CLI %q, want %q", index, session.WorkerSessionId, cliSession.WorkerSessionID, expectedOrder[index])
		}
		if session.WorkId == nil || session.WorkName == nil || *session.WorkId != *cliSession.WorkID || *session.WorkName != *cliSession.WorkName {
			t.Fatalf("fleet HTTP/CLI Work attribution mismatch for %s: HTTP=%#v CLI=%#v", session.WorkerSessionId, session, cliSession)
		}
		if session.FactorySessionId == nil || cliSession.FactorySessionID == nil || *session.FactorySessionId != *cliSession.FactorySessionID {
			t.Fatalf("fleet HTTP/CLI Factory Session mismatch for %s: HTTP=%#v CLI=%#v", session.WorkerSessionId, session, cliSession)
		}
		if session.AttemptId != cliSession.AttemptID || string(session.DurationBasis) != cliSession.DurationBasis || string(session.State) != cliSession.State {
			t.Fatalf("fleet HTTP/CLI lifecycle field mismatch for %s: HTTP=%#v CLI=%#v", session.WorkerSessionId, session, cliSession)
		}
		if session.DurationMillis == nil || cliSession.DurationMillis == nil || *session.DurationMillis != *cliSession.DurationMillis {
			t.Fatalf("fleet HTTP/CLI duration mismatch for %s: HTTP=%#v CLI=%#v", session.WorkerSessionId, session.DurationMillis, cliSession.DurationMillis)
		}
		if session.StartedAt == nil || cliSession.StartedAt == nil || !session.StartedAt.Equal(*cliSession.StartedAt) {
			t.Fatalf("fleet HTTP/CLI start timestamp mismatch for %s: HTTP=%#v CLI=%#v", session.WorkerSessionId, session.StartedAt, cliSession.StartedAt)
		}
		if session.ProviderSession == nil || cliSession.ProviderSession == nil || session.ProviderSession.Provider != cliSession.ProviderSession.Provider || session.ProviderSession.Kind != cliSession.ProviderSession.Kind || session.ProviderSession.Id != cliSession.ProviderSession.ID {
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
