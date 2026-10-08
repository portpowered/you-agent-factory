package cli_rest_journeys_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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

func workScopedRoute(index int) string { return fmt.Sprintf("worker-session-scoped-%03d", index) }

// The customer authors a sequence of workstations over one Work. Each attempt
// has its own provider identity; no private observation or history is seeded.
func writeWorkScopedFactory(t *testing.T, c *workerSessionsCLICase) {
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
		support.WriteWorkstationConfig(t, c.factoryDir, name, "---\ntype: MODEL_WORKSTATION\n---\nworker-session-route="+workScopedRoute(index)+"\n")
		c.registerRoutes(t, workScopedRoute(index))
		input = output
	}
	raw, err := json.Marshal(map[string]any{"name": "scoped-list", "workTypes": []any{map[string]any{"name": "task", "states": states}}, "workers": []any{map[string]any{"name": "worker"}}, "workstations": stations})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(c.factoryDir, "factory.json"), raw, 0600); err != nil {
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
	args := []string{"--server", observer.URL, "--debug", "worker-sessions", "list", "--work-id", workID, "--output", "json"}
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
	c.registerRoutes(t, "worker-session-scoped-default")
	ctx := t.Context()
	workID := submitWork(t, ctx, f.process, functionalEnvironment(f.homeDir), f.hostFactory,
		f.baseURL, "~default", "worker-session-scoped-default")
	row := waitForWorkerSessionState(t, ctx, f.process, functionalEnvironment(f.homeDir), f.hostFactory,
		f.baseURL, "~default", workID, "COMPLETED")
	endpoint := f.baseURL + "/factory-sessions/~default/worker-sessions?workId=" + url.QueryEscape(workID)
	waitForDefaultScopedCommit(t, ctx, endpoint)
	omitted := observeWorkScopedRead(t, ctx, c, "~default", workID, endpoint, true)
	explicit := observeWorkScopedRead(t, ctx, c, "~default", workID, endpoint, false)
	assertNormalizedFleetJSONEqual(t, "default versus explicit default", []byte(omitted.Stdout()), []byte(explicit.Stdout()))
	var listed workerSessionListJSON
	decodeCLIJSON(t, omitted, &listed)
	if len(listed.Sessions) != 1 || listed.Sessions[0].WorkerSessionID != row.WorkerSessionID ||
		listed.Sessions[0].WorkID == nil || *listed.Sessions[0].WorkID != workID {
		t.Fatalf("default selector lost owned Work: %#v", listed)
	}
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
		if len(listed.Sessions) == 1 && listed.Sessions[0].ConfirmationState == "CONFIRMED" &&
			listed.Sessions[0].TokenUsage != nil {
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
