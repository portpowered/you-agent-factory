package cli_rest_journeys_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const workScopedAttemptCount = 200

func testWorkerSessionsListWorkScopedFreshCommit(t *testing.T) {
	t.Parallel()
	c := newWorkerSessionsCLICase(t)
	c.registerRoutes(t, "worker-session-scoped-fresh")
	f := c.fixture
	f.runner.mu.Lock()
	gate := f.runner.definitions["worker-session-scoped-fresh"].gate
	f.runner.mu.Unlock()
	defer gate.release()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	sessionID := c.openSession(t)
	env := functionalEnvironment(f.homeDir)
	workID := submitWork(t, ctx, f.process, env, c.factoryDir, f.baseURL, sessionID, "worker-session-scoped-fresh")
	active := waitForWorkerSessionState(t, ctx, f.process, env, c.factoryDir, f.baseURL, sessionID, workID, "RUNNING")
	inputs := executeCLI(t, ctx, f.process, env, c.factoryDir, "--server", f.baseURL,
		"worker-sessions", "list", "--session", sessionID, "--work-id", workID, "--output", "json")
	var before workerSessionListJSON
	decodeCLIJSON(t, inputs, &before)
	if len(before.Sessions) != 1 || before.Sessions[0].WorkerSessionID != active.WorkerSessionID || before.Sessions[0].TokenUsage != nil {
		t.Fatalf("uncommitted provider facts appeared: %+v", before)
	}
	gate.release()
	endpoint := f.baseURL + "/factory-sessions/" + sessionID + "/worker-sessions?workId=" + url.QueryEscape(workID)
	waitForScopedUsageCommit(t, ctx, endpoint)
	after := observeWorkScopedRead(t, ctx, c, sessionID, workID, endpoint, false)
	var committed workerSessionListJSON
	decodeCLIJSON(t, after, &committed)
	if len(committed.Sessions) != 1 || committed.Sessions[0].WorkerSessionID != active.WorkerSessionID || committed.Sessions[0].State != "COMPLETED" {
		t.Fatalf("subsequent read lost committed identity/state: %+v", committed)
	}
	assertScopedCapturedUsage(t, committed.Sessions[0])
}

func waitForScopedUsageCommit(t *testing.T, ctx context.Context, endpoint string) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		listed := support.GetJSON[workerSessionListJSON](t, endpoint)
		if len(listed.Sessions) == 1 && listed.Sessions[0].State == "COMPLETED" && listed.Sessions[0].TokenUsage != nil {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("usage never committed: %v", ctx.Err())
		}
	}
}

func workScopedRoute(index int) string { return fmt.Sprintf("worker-session-scoped-%03d", index) }

// The customer authors a sequence of workstations over one Work. Each attempt
// has its own provider identity; no private observation or history is seeded.
func writeWorkScopedFactory(t *testing.T, c *workerSessionsCLICase) {
	t.Helper()
	writeWorkScopedFactoryDefinition(t, c.factoryDir)
	for index := range workScopedAttemptCount {
		c.registerRoutes(t, workScopedRoute(index))
	}
}

func writeWorkScopedFactoryDefinition(t *testing.T, factoryDir string) {
	t.Helper()
	states := []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "done", "type": "TERMINAL"}, map[string]any{"name": "failed", "type": "FAILED"}}
	stations := make([]any, 0, workScopedAttemptCount)
	input := "init"
	for index := range workScopedAttemptCount {
		output := "done"
		if index+1 < workScopedAttemptCount {
			output = fmt.Sprintf("stage-%03d", index)
			states = append(states, map[string]any{"name": output, "type": "PROCESSING"})
		}
		name := fmt.Sprintf("step-%03d", index)
		stations = append(stations, map[string]any{"name": name, "worker": "worker", "inputs": []any{map[string]any{"workType": "task", "state": input}}, "outputs": []any{map[string]any{"workType": "task", "state": output}}, "onFailure": []any{map[string]any{"workType": "task", "state": "failed"}}})
		support.WriteWorkstationConfig(t, factoryDir, name, "---\ntype: MODEL_WORKSTATION\n---\nworker-session-route="+workScopedRoute(index)+"\n")
		input = output
	}
	raw, err := json.Marshal(map[string]any{"name": "scoped-list", "workTypes": []any{map[string]any{"name": "task", "states": states}}, "workers": []any{map[string]any{"name": "worker"}}, "workstations": stations})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
}

func testWorkerSessionsListWorkScopedBoundedParity(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	c := newWorkerSessionsCLICase(t)
	writeWorkScopedFactory(t, c)
	f := c.fixture
	sessionID := c.openSession(t)
	env := functionalEnvironment(f.homeDir)
	workID := submitWork(t, ctx, f.process, env, c.factoryDir, f.baseURL, sessionID, "scoped-list-many-attempts")
	endpoint := f.baseURL + "/factory-sessions/" + sessionID + "/worker-sessions?workId=" + url.QueryEscape(workID)
	// Completion is observed through the public projection because dispatch and
	// committed capture publication are asynchronous, separate runtime steps.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		listed := support.GetJSON[workerSessionListJSON](t, endpoint)
		complete := len(listed.Sessions) == workScopedAttemptCount
		for _, row := range listed.Sessions {
			complete = complete && row.State == "COMPLETED" && row.TokenUsage != nil
		}
		if complete {
			break
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("200 attempts never committed: rows=%d: %v", len(listed.Sessions), ctx.Err())
		}
	}
	// A separate peer Factory Session proves Work/Session isolation while
	// remaining parallel with independent scenarios on the reusable process.
	peer := newWorkerSessionsCLICase(t)
	peer.registerRoutes(t, "worker-session-scoped-peer")
	peerSession := peer.openSession(t)
	peerWork := submitWork(t, ctx, f.process, env, peer.factoryDir, f.baseURL, peerSession, "worker-session-scoped-peer")
	waitForWorkerSessionState(t, ctx, f.process, env, peer.factoryDir, f.baseURL, peerSession, peerWork, "COMPLETED")
	assertWorkScopedSingleRead(t, ctx, c, sessionID, workID, endpoint)
}

func assertWorkScopedSingleRead(t *testing.T, ctx context.Context, c *workerSessionsCLICase, sessionID, workID, endpoint string) {
	t.Helper()
	inputs := observeWorkScopedRead(t, ctx, c, sessionID, workID, endpoint, false)
	var cli workerSessionListJSON
	decodeCLIJSON(t, inputs, &cli)
	assertCommittedScopedRows(t, cli, sessionID, workID)
	human := executeCLI(t, ctx, c.fixture.process, functionalEnvironment(c.fixture.homeDir), c.factoryDir,
		"--server", c.fixture.baseURL, "worker-sessions", "list", "--session", sessionID, "--work-id", workID)
	for _, row := range cli.Sessions {
		for _, fact := range []string{row.WorkerSessionID, row.ProviderSession.ID, workID, row.State} {
			if !strings.Contains(human.Stdout(), fact) {
				t.Fatalf("human table omitted fact %q", fact)
			}
		}
	}
}

func observeWorkScopedRead(t *testing.T, ctx context.Context, c *workerSessionsCLICase, sessionID, workID, endpoint string, omitSession bool) *support.CapturedInputs {
	t.Helper()
	f := c.fixture
	target, err := url.Parse(f.baseURL)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	var mu sync.Mutex
	var calls []string
	observer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		mu.Unlock()
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(observer.Close)
	args := []string{"--server", observer.URL, "--json", "--debug", "worker-sessions", "list", "--work-id", workID, "--output", "json"}
	if !omitSession {
		args = append(args, "--session", sessionID)
	}
	inputs := executeCLI(t, ctx, f.process, functionalEnvironment(f.homeDir), c.factoryDir, args...)
	mu.Lock()
	observed := append([]string(nil), calls...)
	mu.Unlock()
	want := "GET /factory-sessions/" + sessionID + "/worker-sessions?workId=" + url.QueryEscape(workID)
	if len(observed) != 1 || observed[0] != want {
		t.Fatalf("CLI calls=%v, want exactly %s", observed, want)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	httpRaw, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != http.StatusOK {
		t.Fatalf("scoped HTTP status=%d read=%v", response.StatusCode, err)
	}
	// Compare raw representations so new observation fields cannot disappear
	// behind the test's intentionally narrow typed assertions.
	assertNormalizedFleetJSONEqual(t, "complete scoped attempts", []byte(inputs.Stdout()), httpRaw)
	t.Logf("scoped read; all calls=%v; diagnostics:\n%s", observed, inputs.Stderr())
	return inputs
}

func assertCommittedScopedRows(t *testing.T, cli workerSessionListJSON, sessionID, workID string) {
	t.Helper()
	seen := make(map[string]bool)
	providerSeen := make(map[string]bool)
	for index, row := range cli.Sessions {
		if row.WorkID == nil || *row.WorkID != workID || row.FactorySessionID == nil || *row.FactorySessionID != sessionID || seen[row.WorkerSessionID] {
			t.Fatalf("lost isolation/unique identity: %#v", row)
		}
		if row.ProviderSession == nil || providerSeen[row.ProviderSession.ID] || !row.ProviderSessionAvailable {
			t.Fatalf("lost unique provider association: %#v", row)
		}
		seen[row.WorkerSessionID] = true
		providerSeen[row.ProviderSession.ID] = true
		if row.StartedAt == nil || (index > 0 && row.StartedAt.Before(*cli.Sessions[index-1].StartedAt)) {
			t.Fatalf("attempt %d lost chronological order", index)
		}
		assertScopedCapturedUsage(t, row)
	}
	if len(cli.Sessions) != workScopedAttemptCount {
		t.Fatalf("CLI rows=%d", len(cli.Sessions))
	}
}

func assertScopedCapturedUsage(t *testing.T, row workerSessionJSON) {
	t.Helper()
	if row.TokenUsage == nil || row.TokenUsage.InputTokens == nil || *row.TokenUsage.InputTokens != 8 || row.TokenUsage.OutputTokens == nil || *row.TokenUsage.OutputTokens != 12 {
		t.Fatalf("lost captured usage: %#v", row)
	}
}

// Only this default-selection cell runs before parallel Session scenarios.
// The Current Factory is the shared host; named Sessions remain independent.
func testWorkerSessionsListWorkScopedDefault(t *testing.T) {
	c := newWorkerSessionsCLICase(t)
	f := c.fixture
	for index := range workScopedAttemptCount {
		c.registerRoutes(t, workScopedRoute(index))
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	workID := submitWork(t, ctx, f.process, functionalEnvironment(f.homeDir), f.hostFactory,
		f.baseURL, "~default", "worker-session-scoped-default")
	endpoint := f.baseURL + "/factory-sessions/~default/worker-sessions?workId=" + url.QueryEscape(workID)
	waitForDefaultScopedCommit(t, ctx, endpoint)
	omitted := observeWorkScopedRead(t, ctx, c, "~default", workID, endpoint, true)
	explicit := observeWorkScopedRead(t, ctx, c, "~default", workID, endpoint, false)
	assertNormalizedFleetJSONEqual(t, "default versus explicit default", []byte(omitted.Stdout()), []byte(explicit.Stdout()))
	var listed workerSessionListJSON
	decodeCLIJSON(t, omitted, &listed)
	if len(listed.Sessions) != workScopedAttemptCount || listed.Sessions[0].FactorySessionID == nil {
		t.Fatalf("default selector lost complete Work: rows=%d", len(listed.Sessions))
	}
	assertCommittedScopedRows(t, listed, *listed.Sessions[0].FactorySessionID, workID)
}

func waitForDefaultScopedCommit(t *testing.T, ctx context.Context, endpoint string) {
	t.Helper()
	// The default host publishes recording confirmation after the source-native
	// terminal. Public reads must observe that separate commit before comparing
	// two requests; neither a fixed delay nor provider completion proves it.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		listed := support.GetJSON[workerSessionListJSON](t, endpoint)
		complete := len(listed.Sessions) == workScopedAttemptCount
		for _, row := range listed.Sessions {
			complete = complete && row.ConfirmationState == "CONFIRMED" && row.State == "COMPLETED" && row.TokenUsage != nil
		}
		if complete {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("default Work capture never committed: %v", ctx.Err())
		}
	}
}

func testWorkerSessionsListWorkScopedEmpty(t *testing.T) {
	t.Parallel()
	c := newWorkerSessionsCLICase(t)
	// An accepted Work with no eligible workstation has no attempts. This is
	// distinct from a missing Work and requires no internal history seeding.
	definition := `{"name":"empty-attempts","workTypes":[{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"done","type":"TERMINAL"}]}],"workers":[],"workstations":[]}`
	if err := os.WriteFile(filepath.Join(c.factoryDir, "factory.json"), []byte(definition), 0600); err != nil {
		t.Fatal(err)
	}
	f, ctx := c.fixture, t.Context()
	sessionID := c.openSession(t)
	workID := submitWork(t, ctx, f.process, functionalEnvironment(f.homeDir), c.factoryDir, f.baseURL, sessionID, "scoped-empty")
	endpoint := f.baseURL + "/factory-sessions/" + sessionID + "/worker-sessions?workId=" + url.QueryEscape(workID)
	inputs := observeWorkScopedRead(t, ctx, c, sessionID, workID, endpoint, false)
	var listed workerSessionListJSON
	decodeCLIJSON(t, inputs, &listed)
	if len(listed.Sessions) != 0 || !strings.Contains(inputs.Stdout(), `"sessions":[]`) {
		t.Fatalf("empty Work list=%s", inputs.Stdout())
	}
	human := executeCLI(t, ctx, f.process, functionalEnvironment(f.homeDir), c.factoryDir,
		"--server", f.baseURL, "worker-sessions", "list", "--session", sessionID, "--work-id", workID)
	if strings.TrimSpace(human.Stdout()) != "No worker sessions found." {
		t.Fatalf("empty human list=%q", human.Stdout())
	}
}

func testWorkerSessionsListWorkScopedOptionalCapture(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"slow", "missing", "failed"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			c := newWorkerSessionsCLICase(t)
			route := "worker-session-optional-" + kind
			c.registerRoutes(t, route)
			f := c.fixture
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			sessionID := c.openSession(t)
			workID := submitWork(t, ctx, f.process, functionalEnvironment(f.homeDir), c.factoryDir, f.baseURL, sessionID, route)
			row := waitForWorkerSessionState(t, ctx, f.process, functionalEnvironment(f.homeDir), c.factoryDir, f.baseURL, sessionID, workID, "COMPLETED")
			endpoint := f.baseURL + "/factory-sessions/" + sessionID + "/worker-sessions?workId=" + url.QueryEscape(workID)
			waitForScopedUsageCommit(t, ctx, endpoint)
			before := observeWorkScopedRead(t, ctx, c, sessionID, workID, endpoint, false)
			assertOptionalCaptureReadUnavailable(t, ctx, c, sessionID, row.WorkerSessionID, kind)
			after := observeWorkScopedRead(t, ctx, c, sessionID, workID, endpoint, false)
			var actual workerSessionListJSON
			decodeCLIJSON(t, after, &actual)
			if len(actual.Sessions) != 1 || actual.Sessions[0].WorkerSessionID != row.WorkerSessionID || actual.Sessions[0].Transcript != "UNAVAILABLE" {
				t.Fatalf("optional failure lost row/unavailable fact: %+v", actual)
			}
			assertScopedCapturedUsage(t, actual.Sessions[0])
			var expectedRaw map[string]any
			if err := json.Unmarshal([]byte(before.Stdout()), &expectedRaw); err != nil {
				t.Fatal(err)
			}
			expectedRaw["sessions"].([]any)[0].(map[string]any)["transcript"] = "UNAVAILABLE"
			raw, err := json.Marshal(expectedRaw)
			if err != nil {
				t.Fatal(err)
			}
			assertNormalizedFleetJSONEqual(t, "optional unavailable preserves all other facts", raw, []byte(after.Stdout()))
		})
	}
}

func assertOptionalCaptureReadUnavailable(t *testing.T, ctx context.Context, c *workerSessionsCLICase, sessionID, workerSessionID, kind string) {
	t.Helper()
	f := c.fixture
	var fault error
	switch kind {
	case "missing":
		fault = fs.ErrNotExist
	case "failed":
		fault = errors.New("private-capture-path sentinel-secret")
	}
	reached := f.captureReads.fault(t, workerSessionID, fault)
	// A public transcript read confirms the selected activity boundary is
	// unavailable. Lists must retain snapshot facts without reacquiring it.
	probeCtx, stop := context.WithCancel(ctx)
	defer stop()
	probe := support.FakeInputs(probeCtx, []string{
		"you", "--server", f.baseURL, "worker-sessions", "read", "--session", sessionID,
		"--worker-session-id", workerSessionID, "--output", "json",
	})
	probe.Input.Env = functionalEnvironment(f.homeDir)
	probe.Input.WorkingDirectory = c.factoryDir
	done := make(chan error, 1)
	go func() { done <- f.process.Execute(probe.Input) }()
	select {
	case <-reached:
	case <-ctx.Done():
		t.Fatalf("selected activity never reached fault boundary: %v", ctx.Err())
	}
	if kind == "slow" {
		stop()
	}
	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		t.Fatalf("faulted activity probe did not return: %v", ctx.Err())
	}
	stop()
	if kind == "slow" && !errors.Is(err, context.Canceled) {
		t.Fatalf("slow activity error=%v, want caller cancellation", err)
	}
	if err == nil {
		t.Fatal("faulted optional activity unexpectedly readable")
	}
	if strings.Contains(probe.Stdout()+probe.Stderr(), "sentinel-secret") {
		t.Fatal("capture fault disclosed private details")
	}
}
