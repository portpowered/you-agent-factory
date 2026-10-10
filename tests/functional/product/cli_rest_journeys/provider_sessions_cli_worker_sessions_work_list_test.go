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

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const workScopedAttemptCount = 3

// Both scopes admit the same explicit Work identities atomically. A two-input
// workstation creates one physical attempt correlated with both Works.
func testWorkerSessionsListWorkScopedCorrelatedScopes(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	seen := make(map[string]bool)
	var reread []func()
	for index := range 2 {
		c := newWorkerSessionsCLICase(t)
		route := fmt.Sprintf("worker-session-correlated-scope-%d", index)
		c.registerRoutes(t, route)
		writeCorrelatedScopedFactory(t, c.factoryDir, route)
		sessionID := c.openSession(t)
		request := `{"requestId":"correlated-request","type":"FACTORY_REQUEST_BATCH","works":[{"workId":"correlated-plan","name":"plan","workTypeName":"plan","payload":{}},{"workId":"correlated-task","name":"task","workTypeName":"task","payload":{}}]}`
		executeCLI(t, ctx, c.fixture.process, functionalEnvironment(c.fixture.homeDir), c.factoryDir,
			"--server", c.fixture.baseURL, "--json", "submit", "batch", "--session", sessionID, request)
		var physicalID string
		for _, workID := range []string{"correlated-plan", "correlated-task"} {
			endpoint := c.fixture.baseURL + "/factory-sessions/" + sessionID + "/worker-sessions?workId=" + workID
			waitForScopedUsageCommit(t, ctx, endpoint)
			inputs := observeWorkScopedRead(t, ctx, c, sessionID, workID, endpoint, false)
			reread = append(reread, func() {
				after := observeWorkScopedRead(t, ctx, c, sessionID, workID, endpoint, false)
				assertNormalizedFleetJSONEqual(t, "coexisting scopes preserve complete correlated rows", []byte(inputs.Stdout()), []byte(after.Stdout()))
			})
			var listed workerSessionListJSON
			decodeCLIJSON(t, inputs, &listed)
			if len(listed.Sessions) != 1 {
				t.Fatalf("scope %s Work %s rows=%+v, want one physical attempt", sessionID, workID, listed)
			}
			row := listed.Sessions[0]
			if row.FactorySessionID == nil || *row.FactorySessionID != sessionID || !containsString(row.WorkIDs, "correlated-plan") || !containsString(row.WorkIDs, "correlated-task") {
				t.Fatalf("lost scope or multi-Work correlation: %+v", row)
			}
			assertScopedCapturedUsage(t, row)
			if physicalID == "" {
				physicalID = row.WorkerSessionID
			} else if physicalID != row.WorkerSessionID {
				t.Fatalf("participating Works returned different physical attempts: %s / %s", physicalID, row.WorkerSessionID)
			}
		}
		if physicalID == "" || seen[physicalID] {
			t.Fatalf("physical identity leaked across Factory Sessions: %q", physicalID)
		}
		seen[physicalID] = true
	}
	for _, read := range reread {
		read()
	}
}

func writeCorrelatedScopedFactory(t *testing.T, factoryDir, route string) {
	t.Helper()
	types := make([]any, 0, 2)
	inputs, outputs := make([]any, 0, 2), make([]any, 0, 2)
	for _, kind := range []string{"plan", "task"} {
		types = append(types, map[string]any{"name": kind, "states": []any{map[string]any{"name": "init", "type": "INITIAL"}, map[string]any{"name": "done", "type": "TERMINAL"}}})
		inputs = append(inputs, map[string]any{"workType": kind, "state": "init"})
		outputs = append(outputs, map[string]any{"workType": kind, "state": "done"})
	}
	raw, err := json.Marshal(map[string]any{"name": "correlated-scopes", "workTypes": types, "workers": []any{map[string]any{"name": "worker"}}, "workstations": []any{map[string]any{"name": "pair", "worker": "worker", "inputs": inputs, "outputs": outputs}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "factory.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	support.WriteWorkstationConfig(t, factoryDir, "pair", "---\ntype: MODEL_WORKSTATION\n---\nworker-session-route="+route+"\n")
}

// Selected durable reads fail the entire customer list, unlike optional
// transcript reads. Each fault is isolated by physical Worker identity on the
// shared process; request cancellation must drain without canceling the Work.
func testWorkerSessionsListWorkScopedSelectedReadFailure(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"cancel", "corrupt", "unavailable"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			c := newWorkerSessionsCLICase(t)
			route := "worker-session-selected-read-" + kind
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
			var failure error
			if kind == "corrupt" {
				failure = fmt.Errorf("private-capture-path sentinel-secret: %w", recordings.ErrWorkerRecordingReplay)
			}
			if kind == "unavailable" {
				failure = fmt.Errorf("private-capture-path sentinel-secret: %w", recordings.ErrMissingWorkerRecordingReader)
			}
			fault, clear := f.captureReads.summaryFault(t, row.WorkerSessionID, failure)
			if kind == "cancel" {
				assertScopedSelectedReadCancellation(t, ctx, c, sessionID, workID, fault)
			} else {
				assertScopedSelectedReadError(t, ctx, c, sessionID, workID, endpoint, kind)
				if calls := fault.calls.Load(); calls != 2 {
					t.Fatalf("selected %s calls=%d, want one per HTTP/CLI request", kind, calls)
				}
			}
			clear()
			after := observeWorkScopedRead(t, ctx, c, sessionID, workID, endpoint, false)
			assertNormalizedFleetJSONEqual(t, "selected read recovery preserves complete committed facts", []byte(before.Stdout()), []byte(after.Stdout()))
		})
	}
}

func assertScopedSelectedReadCancellation(t *testing.T, ctx context.Context, c *workerSessionsCLICase, sessionID, workID string, fault *workerSessionSummaryFault) {
	t.Helper()
	f := c.fixture
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	inputs := support.FakeInputs(readCtx, []string{"you", "--server", f.baseURL, "worker-sessions", "list", "--session", sessionID, "--work-id", workID, "--output", "json"})
	inputs.Input.Env = functionalEnvironment(f.homeDir)
	inputs.Input.WorkingDirectory = c.factoryDir
	done := make(chan error, 1)
	go func() { done <- f.process.Execute(inputs.Input) }()
	select {
	case <-fault.reached:
	case <-ctx.Done():
		t.Fatalf("selected summary read never reached: %v", ctx.Err())
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) || strings.TrimSpace(inputs.Stdout()) != "" {
			t.Fatalf("canceled list err=%v stdout=%q", err, inputs.Stdout())
		}
	case <-ctx.Done():
		t.Fatalf("canceled list did not return: %v", ctx.Err())
	}
	assertFleetJSONErrorCode(t, []byte(inputs.Stderr()), "WORKER_SESSION_LIST_FAILED", "selected summary cancellation")
	select {
	case <-fault.drained:
	case <-ctx.Done():
		t.Fatalf("selected summary read did not drain: %v", ctx.Err())
	}
	if calls := fault.calls.Load(); calls != 1 {
		t.Fatalf("canceled selected read calls=%d, want one with no retry", calls)
	}
}

func assertScopedSelectedReadError(t *testing.T, ctx context.Context, c *workerSessionsCLICase, sessionID, workID, endpoint, kind string) {
	t.Helper()
	wantStatus, wantCode := http.StatusInternalServerError, "WORKER_SESSION_RECORDING_CORRUPT"
	if kind == "unavailable" {
		wantStatus, wantCode = http.StatusServiceUnavailable, "WORKER_SESSION_RECORDING_UNAVAILABLE"
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
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != wantStatus {
		t.Fatalf("selected %s read status=%d err=%v body=%s", kind, response.StatusCode, err, body)
	}
	assertFleetJSONErrorCode(t, body, wantCode, "selected read "+kind)
	f := c.fixture
	inputs, err := executeCLIExpectError(t, ctx, f.process, functionalEnvironment(f.homeDir), c.factoryDir,
		"--server", f.baseURL, "worker-sessions", "list", "--session", sessionID, "--work-id", workID, "--output", "json")
	if err == nil || strings.TrimSpace(inputs.Stdout()) != "" {
		t.Fatalf("selected %s CLI err=%v stdout=%q", kind, err, inputs.Stdout())
	}
	assertFleetJSONErrorCode(t, []byte(inputs.Stderr()), wantCode, "selected read "+kind+" CLI")
	for _, output := range []string{string(body), inputs.Stderr()} {
		if strings.Contains(output, "sentinel-secret") || strings.Contains(output, "private-capture-path") || strings.Contains(output, `"sessions"`) {
			t.Fatalf("selected %s read exposed private details or successful partial rows: %s", kind, output)
		}
	}
}

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
	waitForScopedAttemptsCommit(t, ctx, endpoint, 1)
}

func waitForScopedAttemptsCommit(t *testing.T, ctx context.Context, endpoint string, attempts int) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		listed := support.GetJSON[workerSessionListJSON](t, endpoint)
		complete := len(listed.Sessions) == attempts
		for _, row := range listed.Sessions {
			complete = complete && row.State == "COMPLETED" && row.TokenUsage != nil
		}
		if complete {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("%d attempts never committed: rows=%d: %v", attempts, len(listed.Sessions), ctx.Err())
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
	waitForScopedAttemptsCommit(t, ctx, endpoint, workScopedAttemptCount)
	// A separate peer Factory Session proves Work/Session isolation while
	// remaining parallel with independent scenarios on the reusable process.
	peer := newWorkerSessionsCLICase(t)
	peer.registerRoutes(t, "worker-session-scoped-peer")
	peerSession := peer.openSession(t)
	peerWork := submitWork(t, ctx, f.process, env, peer.factoryDir, f.baseURL, peerSession, "worker-session-scoped-peer")
	waitForWorkerSessionState(t, ctx, f.process, env, peer.factoryDir, f.baseURL, peerSession, peerWork, "COMPLETED")
	unrelated := submitWork(t, ctx, f.process, env, c.factoryDir, f.baseURL, sessionID, "scoped-list-unrelated")
	waitForScopedAttemptsCommit(t, ctx, f.baseURL+"/factory-sessions/"+sessionID+"/worker-sessions?workId="+url.QueryEscape(unrelated), workScopedAttemptCount)
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
	resolvedID := *listed.Sessions[0].FactorySessionID
	resolvedEndpoint := f.baseURL + "/factory-sessions/" + resolvedID + "/worker-sessions?workId=" + url.QueryEscape(workID)
	resolved := observeWorkScopedRead(t, ctx, c, resolvedID, workID, resolvedEndpoint, false)
	assertNormalizedFleetJSONEqual(t, "default alias versus resolved session", []byte(omitted.Stdout()), []byte(resolved.Stdout()))
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
	for _, command := range []string{"show", "read"} {
		inputs, err := executeCLIExpectError(t, ctx, f.process, functionalEnvironment(f.homeDir), c.factoryDir, "--server", f.baseURL, "worker-sessions", command, "--session", sessionID, "--worker-session-id", "unknown-worker", "--output", "json")
		if err == nil || strings.TrimSpace(inputs.Stdout()) != "" {
			t.Fatalf("unknown Worker %s returned content: %v %s", command, err, inputs.Stdout())
		}
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
			if len(actual.Sessions) != 1 || actual.Sessions[0].WorkerSessionID != row.WorkerSessionID || actual.Sessions[0].Transcript != "AVAILABLE" {
				t.Fatalf("activity failure lost committed summary availability: %+v", actual)
			}
			assertScopedCapturedUsage(t, actual.Sessions[0])
			var expectedRaw map[string]any
			if err := json.Unmarshal([]byte(before.Stdout()), &expectedRaw); err != nil {
				t.Fatal(err)
			}
			raw, err := json.Marshal(expectedRaw)
			if err != nil {
				t.Fatal(err)
			}
			assertNormalizedFleetJSONEqual(t, "unavailable activity preserves committed summary facts", raw, []byte(after.Stdout()))
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
