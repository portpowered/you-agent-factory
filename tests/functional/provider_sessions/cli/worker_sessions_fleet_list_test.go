package cli_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestWorkerSessionsCLIEmptyErrorBodiesUseStableDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name string
		body string
	}{
		{name: "empty", body: ""},
		{name: "malformed", body: "{"},
		{name: "missing message", body: `{"code":"UPSTREAM_FAILURE"}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet {
					t.Errorf("worker-sessions list method = %s, want GET", r.Method)
				}
				w.WriteHeader(http.StatusBadGateway)
				_, _ = fmt.Fprint(w, test.body)
			}))
			t.Cleanup(server.Close)

			process := support.BuildProcess(t, serviceedges.Edges{})
			support.CleanupProcess(t, process)
			inputs := support.FakeInputs(t.Context(), []string{
				"you", "--json", "--server", server.URL, "worker-sessions", "list",
			})
			if err := process.Execute(inputs.Input); err == nil {
				t.Fatal("Process.Execute(worker-sessions list) error = nil, want backend failure")
			}
			combined := inputs.Stdout() + inputs.Stderr()
			if !strings.Contains(combined, "WORKER_SESSION_LIST_FAILED") ||
				strings.Contains(combined, "UPSTREAM_FAILURE") {
				t.Fatalf("worker-sessions diagnostic = %q, want stable fallback code", combined)
			}
		})
	}

	t.Run("closed server uses transport-unreachable diagnostic", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
		serverURL := server.URL
		server.Close()

		process := support.BuildProcess(t, serviceedges.Edges{})
		support.CleanupProcess(t, process)
		inputs := support.FakeInputs(t.Context(), []string{
			"you", "--json", "--server", serverURL, "worker-sessions", "list",
		})
		if err := process.Execute(inputs.Input); err == nil {
			t.Fatal("Process.Execute(worker-sessions list) error = nil, want transport failure")
		}
		combined := inputs.Stdout() + inputs.Stderr()
		if !strings.Contains(combined, "FACTORY_UNREACHABLE") || strings.Contains(combined, "worker sessions") {
			t.Fatalf("closed-server diagnostic = %q, want stable unreachable code", combined)
		}
	})

	t.Run("redirect loop preserves bounded transport failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", r.URL.String())
			w.WriteHeader(http.StatusTemporaryRedirect)
		}))
		t.Cleanup(server.Close)

		process := support.BuildProcess(t, serviceedges.Edges{})
		support.CleanupProcess(t, process)
		inputs := support.FakeInputs(t.Context(), []string{
			"you", "--json", "--server", server.URL, "worker-sessions", "list",
		})
		if err := process.Execute(inputs.Input); err == nil {
			t.Fatal("Process.Execute(worker-sessions list) error = nil, want redirect-loop failure")
		}
		combined := inputs.Stdout() + inputs.Stderr()
		if !strings.Contains(combined, "FACTORY_UNREACHABLE") || strings.Contains(combined, "Location") {
			t.Fatalf("redirect-loop diagnostic = %q, want stable unreachable code", combined)
		}
	})
}

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
	homeDir := t.TempDir()
	gate := make(chan struct{})
	runner := support.NewGatedSuccessCommandRunner("Fleet fixture COMPLETE", gate)
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

	expectedWorks := submitFleetWorks(t, ctx, process, env, factoryDir, baseURL)
	assertFleetState(t, waitForFleetWorkerSessionsState(t, ctx, process, env, factoryDir, baseURL, "RUNNING", len(expectedWorks)), expectedWorks, "RUNNING")
	close(gate)
	assertFleetState(t, waitForFleetWorkerSessionsState(t, ctx, process, env, factoryDir, baseURL, "COMPLETED", len(expectedWorks)), expectedWorks, "COMPLETED")
	assertFleetWorkerSessionList(t, ctx, process, env, factoryDir, baseURL, expectedWorks, false)

	server.Stop(t)
	if err := server.Err(); err != nil {
		t.Fatalf("server Process.Execute: %v\nstdout:\n%s\nstderr:\n%s", err, serverInputs.Stdout(), serverInputs.Stderr())
	}
}

func submitFleetWorks(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL string) map[string]string {
	t.Helper()
	expectedWorks := make(map[string]string, 3)
	for _, name := range []string{"worker-session-fleet-alpha", "worker-session-fleet-beta", "worker-session-fleet-gamma"} {
		workID := submitWork(t, ctx, process, env, factoryDir, baseURL, name)
		expectedWorks[workID] = name
	}
	return expectedWorks
}

func assertFleetState(t *testing.T, sessions []workerSessionJSON, expectedWorks map[string]string, state string) {
	t.Helper()
	for _, session := range sessions {
		if session.WorkID == nil || session.WorkName == nil || session.StartedAt == nil || session.DurationMillis == nil {
			t.Fatalf("%s fleet observation omitted Work or timing facts: %#v", state, session)
		}
		if want, ok := expectedWorks[*session.WorkID]; !ok || *session.WorkName != want || session.State != state {
			t.Fatalf("%s fleet observation attribution = %#v, expected Work map %#v", state, session, expectedWorks)
		}
		if state == "COMPLETED" && *session.DurationMillis < 0 {
			t.Fatalf("terminal fleet observation duration = %d, want non-negative", *session.DurationMillis)
		}
	}
}

func assertFleetWorkerSessionList(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL string, expectedWorks map[string]string, requireProviderSession bool) {
	t.Helper()
	cliList := fetchFleetCLIList(t, ctx, process, env, factoryDir, baseURL, 10)
	if len(cliList.Sessions) != len(expectedWorks) {
		t.Fatalf("fleet CLI session count = %d, want %d: %#v", len(cliList.Sessions), len(expectedWorks), cliList)
	}
	byID := assertFleetCLIObservations(t, cliList, requireProviderSession)
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

func assertFleetCLIObservations(t *testing.T, list workerSessionListJSON, requireProviderSession bool) map[string]workerSessionJSON {
	t.Helper()
	byID := make(map[string]workerSessionJSON, len(list.Sessions))
	for _, session := range list.Sessions {
		if session.WorkerSessionID == "" || session.WorkID == nil || session.WorkName == nil || session.StartedAt == nil || session.DurationMillis == nil {
			t.Fatalf("fleet CLI observation omitted required attribution/timing facts: %#v", session)
		}
		if requireProviderSession && (session.ProviderSession == nil || session.ProviderSession.Provider != "codex" || session.ProviderSession.Kind != "session_id") {
			t.Fatalf("fleet CLI observation omitted provider/kind: %#v", session)
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
