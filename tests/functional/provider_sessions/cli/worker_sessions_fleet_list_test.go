package cli_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	platformprocessmemory "github.com/portpowered/infinite-you/pkg/platform/processmemory"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	boundedFleetObservationCount = 24
	boundedFleetPageSize         = 20
	boundedFleetSessionCount     = 3
)

// Fleet scope is process-wide: --session cannot isolate its matching rows.
// Keep admission through gate release in one global observation window, as
// with CLIConcurrent; independent diagnostic cells below run in parallel.
func TestWorkerSessionsFleetActiveListDeadlineRepair(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	c := newWorkerSessionsCLICase(t)
	f := c.fixture
	env := functionalEnvironment(f.homeDir)
	c.registerRoutes(t, append(fleetWorkNames(), "worker-session-cli-success")...)
	f.resetFleetGate()
	defer f.releaseFleetGate()
	owners := make(map[string]string)
	for _, name := range fleetWorkNames() {
		owners[name] = c.openSession(t)
	}
	terminalOwner := c.openSession(t)
	terminalWork := submitWork(t, ctx, f.process, env, c.factoryDir, f.baseURL, terminalOwner, "worker-session-cli-success")
	waitForWorkerSessionState(t, ctx, f.process, env, c.factoryDir, f.baseURL, terminalOwner, terminalWork, "COMPLETED")
	start := f.runner.CallCount()
	works, sessions := submitFleetWorks(t, ctx, f.process, env, c.factoryDir, f.baseURL, owners)
	if err := f.runner.WaitForCalls(ctx, start+len(works)); err != nil {
		t.Fatal(err)
	}
	expectedIDs := make(map[string]string)
	for workID := range works {
		row := waitForWorkerSessionState(t, ctx, f.process, env, c.factoryDir, f.baseURL, sessions[workID], workID, "RUNNING")
		expectedIDs[workID] = row.WorkerSessionID
	}
	// The command-runner edge holds RUNNING before returning provider output.
	// STARTING is covered by component tests: this public edge cannot hold it.
	for _, scope := range []string{"all", "factory"} {
		var order []string
		token := ""
		for page := 0; page < 2; page++ {
			args := []string{"--server", f.baseURL, "worker-sessions", "list", "--scope", scope,
				"--state", "RUNNING", "--state", "STARTING", "--max-results", "2", "--output", "json"}
			query := url.Values{"scope": {scope}, "state": {"RUNNING", "STARTING"}, "maxResults": {"2"}}
			if token != "" {
				args = append(args, "--next-token", token)
				query.Set("nextToken", token)
			}
			started := time.Now()
			inputs := executeCLI(t, ctx, f.process, env, c.factoryDir, args...)
			t.Logf("active fleet scope=%s page=%d CLI elapsed=%s existing client timeout=10s", scope, page, time.Since(started))
			var cli workerSessionListJSON
			decodeCLIJSON(t, inputs, &cli)
			httpPage := fetchBoundedFleetHTTPPageWithQuery(t, ctx, f.baseURL, query)
			if httpPage.status != http.StatusOK {
				t.Fatalf("active HTTP status=%d body=%s", httpPage.status, httpPage.raw)
			}
			for _, row := range httpPage.list.Sessions {
				assertActiveFleetObservation(t, row, works, sessions, expectedIDs)
			}
			assertActiveFleetPageParity(t, []byte(inputs.Stdout()), httpPage.raw)
			wantCount := 2 - page
			if len(cli.Sessions) != wantCount || cli.PaginationContext == nil || cli.PaginationContext.MaxResults != 2 {
				t.Fatalf("active page %d = %#v, want %d rows bounded by 2", page, cli, wantCount)
			}
			for _, row := range cli.Sessions {
				assertActiveFleetObservation(t, row, works, sessions, expectedIDs)
				order = append(order, row.WorkerSessionID)
			}
			token = cli.PaginationContext.NextToken
			if (page == 0) != (token != "") {
				t.Fatalf("active page %d cursor=%q", page, token)
			}
		}
		assertAscendingWorkerSessionOrder(t, order, scope)
		if len(order) != len(expectedIDs) || order[0] == order[1] || order[1] == order[2] {
			t.Fatalf("active pages duplicated or omitted identities: %v", order)
		}
		alias := executeCLI(t, ctx, f.process, env, c.factoryDir, "--server", f.baseURL,
			"worker-sessions", "list", "--scope", scope, "--state", "RUNNING", "--state", "STARTING", "--limit", "2", "--output", "json")
		var aliasPage workerSessionListJSON
		decodeCLIJSON(t, alias, &aliasPage)
		if len(aliasPage.Sessions) != 2 || aliasPage.Sessions[0].WorkerSessionID != order[0] || aliasPage.Sessions[1].WorkerSessionID != order[1] {
			t.Fatalf("limit alias selected different active page: %#v", aliasPage)
		}
	}
	for workID := range works {
		inputs := executeCLI(t, ctx, f.process, env, c.factoryDir, "--server", f.baseURL,
			"worker-sessions", "list", "--session", sessions[workID], "--work-id", workID, "--output", "json")
		var scoped workerSessionListJSON
		decodeCLIJSON(t, inputs, &scoped)
		if len(scoped.Sessions) != 1 {
			t.Fatalf("Work-scoped list=%#v, want single owned attempt", scoped)
		}
		assertActiveFleetObservation(t, scoped.Sessions[0], works, sessions, expectedIDs)
		httpScoped := support.GetJSON[workerSessionListJSON](t, f.baseURL+"/factory-sessions/"+sessions[workID]+"/worker-sessions?workId="+url.QueryEscape(workID))
		for _, row := range httpScoped.Sessions {
			assertActiveFleetObservation(t, row, works, sessions, expectedIDs)
		}
		left, _ := json.Marshal(scoped)
		right, _ := json.Marshal(httpScoped)
		assertActiveFleetPageParity(t, left, right)
	}
	for _, query := range []url.Values{
		{"scope": {"direct"}, "state": {"RUNNING", "STARTING"}, "limit": {"20"}},
		{"scope": {"all"}, "state": {"CANCELED"}, "limit": {"20"}},
	} {
		args := []string{"--server", f.baseURL, "worker-sessions", "list", "--scope", query.Get("scope"), "--limit", "20", "--output", "json"}
		for _, state := range query["state"] {
			args = append(args, "--state", state)
		}
		inputs := executeCLI(t, ctx, f.process, env, c.factoryDir, args...)
		var empty workerSessionListJSON
		decodeCLIJSON(t, inputs, &empty)
		assertBoundedFleetEmptyPage(t, empty, "active no-match CLI")
		httpPage := fetchBoundedFleetHTTPPageWithQuery(t, ctx, f.baseURL, query)
		if httpPage.status != http.StatusOK {
			t.Fatalf("no-match HTTP=%d %s", httpPage.status, httpPage.raw)
		}
		assertBoundedFleetEmptyPage(t, httpPage.list, "active no-match HTTP")
		assertNormalizedFleetJSONEqual(t, "active no-match", []byte(inputs.Stdout()), httpPage.raw)
	}
	assertBoundedFleetMalformedToken(t, ctx, f.process, env, c.factoryDir, f.baseURL)
	f.providerFiles.mu.Lock()
	f.providerFiles.blockedPath = filepath.Clean(filepath.Join(f.homeDir, ".codex", "sessions", "2026", "07", "27", "rollout-session_fixture_codex_fleet_alpha.jsonl"))
	f.providerFiles.blockedCalls = 0
	f.providerFiles.mu.Unlock()
	defer func() {
		f.providerFiles.mu.Lock()
		f.providerFiles.blockedPath = ""
		f.providerFiles.mu.Unlock()
	}()
	for _, scope := range []string{"all", "factory"} {
		inputs := executeCLI(t, ctx, f.process, env, c.factoryDir, "--server", f.baseURL,
			"worker-sessions", "list", "--scope", scope, "--state", "RUNNING", "--state", "STARTING", "--max-results", "25", "--output", "json")
		var lost workerSessionListJSON
		decodeCLIJSON(t, inputs, &lost)
		if len(lost.Sessions) != 3 {
			t.Fatalf("optional provider loss hid active identities: %#v", lost)
		}
		for _, row := range lost.Sessions {
			assertActiveFleetObservation(t, row, works, sessions, expectedIDs)
			if row.WorkName != nil && *row.WorkName == "worker-session-fleet-alpha" && row.Transcript != "UNAVAILABLE" {
				t.Fatalf("unreadable provider transcript claimed availability: %#v", row)
			}
		}
		httpPage := fetchBoundedFleetHTTPPageWithQuery(t, ctx, f.baseURL, url.Values{"scope": {scope}, "state": {"RUNNING", "STARTING"}, "maxResults": {"25"}})
		if httpPage.status != http.StatusOK {
			t.Fatalf("optional loss HTTP status=%d body=%s", httpPage.status, httpPage.raw)
		}
		for _, row := range httpPage.list.Sessions {
			assertActiveFleetObservation(t, row, works, sessions, expectedIDs)
		}
		assertActiveFleetPageParity(t, []byte(inputs.Stdout()), httpPage.raw)
	}
	// The available terminal sibling uses a different path and keeps all usage
	// and transcript facts while the active row's provider file is unavailable.
	assertSuccessfulWorkerSession(t, ctx, f.process, env, c.factoryDir, f.baseURL, terminalOwner, terminalWork)
	f.providerFiles.mu.Lock()
	blockedCalls := f.providerFiles.blockedCalls
	f.providerFiles.blockedPath = ""
	f.providerFiles.mu.Unlock()
	if blockedCalls == 0 {
		t.Fatal("optional-loss scenario never reached the exact owned filesystem fault")
	}
	gate := &workerSessionReadGate{token: "owned-canceled-read", received: make(chan struct{}), drained: make(chan struct{})}
	f.api.readMu.Lock()
	f.api.readGate = gate
	f.api.readMu.Unlock()
	defer func() {
		f.api.readMu.Lock()
		f.api.readGate = nil
		f.api.readMu.Unlock()
	}()
	readCtx, cancelRead := context.WithCancel(ctx)
	defer cancelRead()
	canceledInputs := support.FakeInputs(readCtx, []string{"you", "--server", f.baseURL, "worker-sessions", "list",
		"--state", "RUNNING", "--next-token", gate.token, "--output", "json"})
	canceledInputs.Input.Env = env
	canceledInputs.Input.WorkingDirectory = c.factoryDir
	done := make(chan error, 1)
	go func() { done <- f.process.Execute(canceledInputs.Input) }()
	select {
	case <-gate.received:
	case <-ctx.Done():
		t.Fatal("cancellation request never reached the owned HTTP gate")
	}
	cancelRead()
	select {
	case err := <-done:
		if err == nil || strings.TrimSpace(canceledInputs.Stdout()) != "" {
			t.Fatalf("canceled fleet read returned success: err=%v stdout=%s", err, canceledInputs.Stdout())
		}
		assertFleetJSONErrorCode(t, []byte(canceledInputs.Stderr()), "FACTORY_UNREACHABLE", "canceled fleet read")
	case <-ctx.Done():
		t.Fatal("canceled CLI read did not join")
	}
	select {
	case <-gate.drained:
	case <-ctx.Done():
		t.Fatal("owned canceled HTTP request did not drain")
	}
	for workID := range works {
		row := waitForWorkerSessionState(t, ctx, f.process, env, c.factoryDir, f.baseURL, sessions[workID], workID, "RUNNING")
		assertActiveFleetObservation(t, row, works, sessions, expectedIDs)
	}
	f.releaseFleetGate()
	for workID := range works {
		waitForWorkerSessionState(t, ctx, f.process, env, c.factoryDir, f.baseURL, sessions[workID], workID, "COMPLETED")
	}
	t.Run("unreachable", func(t *testing.T) {
		t.Parallel()
		inputs, err := executeCLIExpectError(t, t.Context(), f.process, env, c.factoryDir,
			"--server", "http://127.0.0.1:0", "worker-sessions", "list", "--output", "json")
		if err == nil || strings.TrimSpace(inputs.Stdout()) != "" {
			t.Fatalf("unreachable host returned success: err=%v stdout=%s", err, inputs.Stdout())
		}
		assertFleetJSONErrorCode(t, []byte(inputs.Stderr()), "FACTORY_UNREACHABLE", "active unreachable")
	})
}

func assertActiveFleetObservation(t *testing.T, row workerSessionJSON, works, sessions, ids map[string]string) {
	t.Helper()
	if row.WorkID == nil || row.WorkName == nil || row.FactorySessionID == nil {
		t.Fatalf("active row omitted attribution: %#v", row)
	}
	workID := *row.WorkID
	if row.WorkerSessionID == "" || row.WorkerSessionID != ids[workID] || *row.WorkName != works[workID] || *row.FactorySessionID != sessions[workID] || row.State != "RUNNING" || row.Direct {
		t.Fatalf("active row changed identity/scope/state: %#v", row)
	}
	if row.AttemptID == "" || row.StartedAt == nil || row.DurationMillis == nil || *row.DurationMillis < 0 || row.EndedAt != nil || !containsString(row.WorkIDs, workID) {
		t.Fatalf("active row omitted attempt/timing/correlation: %#v", row)
	}
	if works[workID] == "worker-session-fleet-alpha" {
		if row.ProviderSession == nil || !row.ProviderSessionAvailable || row.ProviderSession.ID != "session_fixture_codex_fleet_alpha" || row.ProviderSession.Provider != "codex" || row.ProviderSession.Kind != "session_id" {
			t.Fatalf("streamed active row lost provider reference: %#v", row)
		}
		if row.Transcript == "AVAILABLE" {
			if row.TokenUsage == nil || row.TokenUsage.InputTokens == nil || *row.TokenUsage.InputTokens != 8 || row.TokenUsage.OutputTokens == nil || *row.TokenUsage.OutputTokens != 12 || row.TokenUsage.TotalTokens == nil || *row.TokenUsage.TotalTokens != 20 {
				t.Fatalf("available provider usage changed or disappeared: %#v", row.TokenUsage)
			}
		} else if row.Transcript != "UNAVAILABLE" || row.TokenUsage != nil {
			// Current active usage comes from the optional native projection;
			// missing storage must report absence, never synthesize zero usage.
			t.Fatalf("unavailable provider usage is untruthful: %#v", row)
		}
		return
	}
	if row.ProviderSession != nil || row.ProviderSessionAvailable || row.TokenUsage != nil || row.EndedAt != nil {
		t.Fatalf("held command invented unavailable provider/usage/terminal facts: %#v", row)
	}
}

func assertActiveFleetPageParity(t *testing.T, cli, httpBody []byte) {
	t.Helper()
	// Duration advances between requests; compare its nonnegative presence above
	// and preserve exact parity for all stable captured facts and cursors.
	stripDuration := func(raw []byte) []byte {
		var value map[string]any
		if err := json.Unmarshal(raw, &value); err != nil {
			t.Fatal(err)
		}
		if rows, ok := value["sessions"].([]any); ok {
			for _, row := range rows {
				delete(row.(map[string]any), "durationMillis")
			}
		}
		result, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	assertNormalizedFleetJSONEqual(t, "active page", stripDuration(cli), stripDuration(httpBody))
}

// TestWorkerSessionsFleetListBoundedRootPages is the public promotion witness
// for the bounded fleet reader. It uses one reusable root process and an
// isolated set of explicit Factory Sessions, while the assertions stay at the
// customer-facing CLI and HTTP boundaries.
func TestWorkerSessionsFleetListBoundedRootPages(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	caseFixture := newWorkerSessionsCLICase(t)
	fixture := caseFixture.fixture
	process := fixture.process
	factoryDir := caseFixture.factoryDir
	env := functionalEnvironment(fixture.homeDir)
	baseURL := fixture.baseURL
	workNames := boundedFleetWorkNames()
	if len(workNames) != boundedFleetObservationCount {
		t.Fatalf("bounded fleet fixture has %d work names, want %d", len(workNames), boundedFleetObservationCount)
	}
	caseFixture.registerRoutes(t, workNames...)
	routeStart := fixture.runner.CallCount()

	factorySessionIDs := make([]string, 0, boundedFleetSessionCount)
	for index := 0; index < boundedFleetSessionCount; index++ {
		factorySessionIDs = append(factorySessionIDs, caseFixture.openSession(t))
	}
	sourceCount := countBoundedFleetCatalogSources(t, baseURL, factorySessionIDs)

	expected := make(map[string]boundedFleetExpectedObservation, len(workNames))
	expectedByName := make(map[string]boundedFleetExpectedObservation, len(workNames))
	for index, name := range workNames {
		factorySessionID := factorySessionIDs[index%len(factorySessionIDs)]
		workID := submitWork(t, ctx, process, env, factoryDir, baseURL, factorySessionID, name)
		observation := boundedFleetExpectedObservation{
			WorkID:            workID,
			WorkName:          name,
			FactorySessionID:  factorySessionID,
			ProviderSessionID: boundedFleetProviderSessionID(index),
			State:             boundedFleetState(index),
		}
		expected[workID] = observation
		expectedByName[name] = observation
	}

	for _, observation := range expectedByName {
		listed := waitForWorkerSessionState(
			t, ctx, process, env, factoryDir, baseURL,
			observation.FactorySessionID, observation.WorkID, observation.State,
		)
		if listed.WorkID == nil || *listed.WorkID != observation.WorkID {
			t.Fatalf("bounded fleet Work %s resolved to %#v", observation.WorkID, listed)
		}
	}
	if err := fixture.runner.WaitForCalls(ctx, routeStart+len(workNames)); err != nil {
		t.Fatalf("wait for bounded fleet provider dispatches: %v", err)
	}
	wantRoutes := make(map[string]struct{}, len(workNames))
	for _, workName := range workNames {
		wantRoutes[workName] = struct{}{}
	}
	assertProviderCommandRoutesSince(t, fixture.runner, routeStart, wantRoutes)

	firstCLI := fetchBoundedFleetCLIPage(t, ctx, process, env, factoryDir, baseURL, "")
	secondCLI := fetchBoundedFleetCLIPage(t, ctx, process, env, factoryDir, baseURL, firstCLI.list.PaginationContext.NextToken)
	assertBoundedFleetPage(t, firstCLI.list, expected, boundedFleetPageSize, true, "CLI first")
	assertBoundedFleetPage(t, secondCLI.list, expected, boundedFleetObservationCount-boundedFleetPageSize, false, "CLI continuation")
	assertBoundedFleetCompleteSelection(t, firstCLI.list, secondCLI.list, expected)

	firstHTTP := fetchBoundedFleetHTTPPage(t, ctx, baseURL, "")
	secondHTTP := fetchBoundedFleetHTTPPage(t, ctx, baseURL, firstHTTP.list.PaginationContext.NextToken)
	if strings.TrimSpace(firstHTTP.headers.Get("Content-Type")) == "" {
		t.Fatal("HTTP first page omitted Content-Type header")
	}
	assertBoundedFleetPage(t, firstHTTP.list, expected, boundedFleetPageSize, true, "HTTP first")
	assertBoundedFleetPage(t, secondHTTP.list, expected, boundedFleetObservationCount-boundedFleetPageSize, false, "HTTP continuation")
	if firstCLI.status != firstHTTP.status || secondCLI.status != secondHTTP.status {
		t.Fatalf("CLI/HTTP page statuses differ: CLI=(%d,%d) HTTP=(%d,%d)", firstCLI.status, secondCLI.status, firstHTTP.status, secondHTTP.status)
	}
	assertNormalizedFleetJSONEqual(t, "first page", firstCLI.raw, firstHTTP.raw)
	assertNormalizedFleetJSONEqual(t, "continuation page", secondCLI.raw, secondHTTP.raw)
	assertBoundedFleetCompleteSelection(t, firstHTTP.list, secondHTTP.list, expected)

	assertBoundedFleetNoMatch(t, ctx, process, env, factoryDir, baseURL)
	assertBoundedFleetMalformedToken(t, ctx, process, env, factoryDir, baseURL)

	workingSetHighWater, workingSetErr := processWorkingSetHighWater()
	commitBytes, commitErr := platformprocessmemory.CurrentCommit()
	host, hostErr := os.Hostname()
	if hostErr != nil {
		host = "unknown"
	}
	t.Logf(
		"bounded fleet witness fixture=worker-session-fleet-v1 host=%s go=%s/%s version=%s sources=%d pageReadCallsPerRequest=%d rows=%d pageSize=%d firstCLIElapsed=%s firstCLIBytes=%d firstHTTPElapsed=%s firstHTTPBytes=%d firstHTTPStatus=%d firstHTTPContentType=%q workingSetHighWaterBytes=%d workingSetError=%v commitBytes=%d commitError=%v",
		host, runtime.GOOS, runtime.GOARCH, runtime.Version(), sourceCount, sourceCount,
		boundedFleetObservationCount, boundedFleetPageSize, firstCLI.elapsed, len(firstCLI.raw),
		firstHTTP.elapsed, len(firstHTTP.raw), firstHTTP.status, firstHTTP.headers.Get("Content-Type"),
		workingSetHighWater, workingSetErr, commitBytes, commitErr,
	)
}

type boundedFleetExpectedObservation struct {
	WorkID            string
	WorkName          string
	FactorySessionID  string
	ProviderSessionID string
	State             string
}

type boundedFleetPageCapture struct {
	list    workerSessionListJSON
	raw     []byte
	elapsed time.Duration
	status  int
	headers http.Header
}

func boundedFleetWorkNames() []string {
	names := make([]string, boundedFleetObservationCount)
	for index := range names {
		names[index] = fmt.Sprintf("worker-session-fleet-page-%02d", index+1)
	}
	return names
}

func boundedFleetProviderSessionID(index int) string {
	return fmt.Sprintf("session_fixture_codex_fleet_page_%02d", index+1)
}

func boundedFleetState(index int) string {
	if index%2 == 0 {
		return "COMPLETED"
	}
	return "FAILED"
}

func countBoundedFleetCatalogSources(t *testing.T, baseURL string, expectedSessionIDs []string) int {
	t.Helper()
	listed := support.GetJSON[factoryapi.ListFactorySessionsResponse](t, strings.TrimSuffix(baseURL, "/")+"/factory-sessions")
	known := make(map[string]struct{}, len(listed.Sessions))
	for _, session := range listed.Sessions {
		if strings.TrimSpace(session.Id) != "" {
			known[session.Id] = struct{}{}
		}
	}
	for _, expectedSessionID := range expectedSessionIDs {
		if _, ok := known[expectedSessionID]; !ok {
			t.Fatalf("bounded fleet Factory Session %q missing from root catalog: %#v", expectedSessionID, listed)
		}
	}
	// The root fleet catalog always starts with the direct Worker Sessions
	// service, then appends one source for each live Factory Session.
	return 1 + len(known)
}

func fetchBoundedFleetCLIPage(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL, nextToken string) boundedFleetPageCapture {
	t.Helper()
	args := []string{
		"you", "--server", baseURL, "worker-sessions", "list",
		"--state", "COMPLETED", "--state", "FAILED", "--limit", fmt.Sprint(boundedFleetPageSize), "--output", "json",
	}
	if strings.TrimSpace(nextToken) != "" {
		args = append(args, "--next-token", nextToken)
	}
	inputs := support.FakeInputs(ctx, args)
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = factoryDir
	started := time.Now()
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("bounded fleet CLI page token=%q: %v\nstdout:\n%s\nstderr:\n%s", nextToken, err, inputs.Stdout(), inputs.Stderr())
	}
	raw := []byte(inputs.Stdout())
	var listed workerSessionListJSON
	if err := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &listed); err != nil {
		t.Fatalf("decode bounded fleet CLI page token=%q: %v\nstdout:\n%s", nextToken, err, inputs.Stdout())
	}
	return boundedFleetPageCapture{list: listed, raw: raw, elapsed: time.Since(started), status: http.StatusOK}
}

func fetchBoundedFleetHTTPPage(t *testing.T, ctx context.Context, baseURL, nextToken string) boundedFleetPageCapture {
	t.Helper()
	query := url.Values{}
	query.Add("state", "COMPLETED")
	query.Add("state", "FAILED")
	query.Set("limit", fmt.Sprint(boundedFleetPageSize))
	if strings.TrimSpace(nextToken) != "" {
		query.Set("nextToken", nextToken)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/worker-sessions?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build bounded fleet HTTP request: %v", err)
	}
	started := time.Now()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("bounded fleet HTTP page token=%q: %v", nextToken, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read bounded fleet HTTP page token=%q: %v", nextToken, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("bounded fleet HTTP page token=%q status=%d body=%s", nextToken, response.StatusCode, body)
	}
	var listed workerSessionListJSON
	if err := json.Unmarshal(body, &listed); err != nil {
		t.Fatalf("decode bounded fleet HTTP page token=%q: %v\nbody:\n%s", nextToken, err, body)
	}
	return boundedFleetPageCapture{
		list: listed, raw: body, elapsed: time.Since(started), status: response.StatusCode, headers: response.Header.Clone(),
	}
}

func assertBoundedFleetPage(t *testing.T, page workerSessionListJSON, expected map[string]boundedFleetExpectedObservation, wantRows int, wantNext bool, label string) {
	t.Helper()
	if len(page.Sessions) != wantRows {
		t.Fatalf("%s row count=%d, want %d: %#v", label, len(page.Sessions), wantRows, page)
	}
	if page.Sessions == nil {
		t.Fatalf("%s sessions is nil", label)
	}
	if page.PaginationContext == nil || page.PaginationContext.MaxResults != boundedFleetPageSize {
		t.Fatalf("%s pagination=%#v, want maxResults=%d", label, page.PaginationContext, boundedFleetPageSize)
	}
	if (strings.TrimSpace(page.PaginationContext.NextToken) != "") != wantNext {
		t.Fatalf("%s nextToken=%q, want present=%t", label, page.PaginationContext.NextToken, wantNext)
	}
	order := make([]string, 0, len(page.Sessions))
	seen := make(map[string]struct{}, len(page.Sessions))
	for _, session := range page.Sessions {
		if session.WorkerSessionID == "" {
			t.Fatalf("%s row omitted Worker Session identity: %#v", label, session)
		}
		if _, duplicate := seen[session.WorkerSessionID]; duplicate {
			t.Fatalf("%s duplicated Worker Session identity %q", label, session.WorkerSessionID)
		}
		seen[session.WorkerSessionID] = struct{}{}
		order = append(order, session.WorkerSessionID)
		assertBoundedFleetObservation(t, label, session, expected)
	}
	assertAscendingWorkerSessionOrder(t, order, label)
}

func assertBoundedFleetObservation(t *testing.T, label string, session workerSessionJSON, expected map[string]boundedFleetExpectedObservation) {
	t.Helper()
	if session.WorkID == nil {
		t.Fatalf("%s row %q omitted Work ID: %#v", label, session.WorkerSessionID, session)
	}
	want, ok := expected[*session.WorkID]
	if !ok {
		t.Fatalf("%s row %q has unexpected Work ID %q", label, session.WorkerSessionID, *session.WorkID)
	}
	if session.WorkName == nil || *session.WorkName != want.WorkName || session.State != want.State {
		t.Fatalf("%s row %q attribution=%#v, want name=%q state=%q", label, session.WorkerSessionID, session, want.WorkName, want.State)
	}
	if session.FactorySessionID == nil || *session.FactorySessionID != want.FactorySessionID || session.Direct {
		t.Fatalf("%s row %q Factory/Direct attribution=%#v, want factory=%q direct=false", label, session.WorkerSessionID, session, want.FactorySessionID)
	}
	if session.ProviderSession == nil || !session.ProviderSessionAvailable || session.ProviderSession.Provider != "codex" || session.ProviderSession.Kind != "session_id" || session.ProviderSession.ID != want.ProviderSessionID {
		t.Fatalf("%s row %q provider identity=%#v available=%t, want %q", label, session.WorkerSessionID, session.ProviderSession, session.ProviderSessionAvailable, want.ProviderSessionID)
	}
	if session.AttemptID == "" || session.StartedAt == nil || session.EndedAt == nil || session.DurationMillis == nil || *session.DurationMillis < 0 || session.DurationBasis != "RECORDED_TIMESTAMPS" {
		t.Fatalf("%s row %q omitted recorded timing facts: %#v", label, session.WorkerSessionID, session)
	}
	if session.Transcript != "AVAILABLE" || session.ConfirmationState != "UNCONFIRMED" || session.Parse.EventCount == 0 {
		t.Fatalf("%s row %q omitted transcript/recording/parse facts: transcript=%q confirmation=%q parse=%#v", label, session.WorkerSessionID, session.Transcript, session.ConfirmationState, session.Parse)
	}
	if !containsString(session.WorkIDs, want.WorkID) {
		t.Fatalf("%s row %q Work IDs=%v, want %q", label, session.WorkerSessionID, session.WorkIDs, want.WorkID)
	}
	if want.State == "COMPLETED" {
		if session.Failure != nil && string(session.Failure) != "null" {
			t.Fatalf("%s successful row %q has failure=%s", label, session.WorkerSessionID, session.Failure)
		}
		if session.TokenUsage == nil || session.TokenUsage.InputTokens == nil || *session.TokenUsage.InputTokens != 8 || session.TokenUsage.OutputTokens == nil || *session.TokenUsage.OutputTokens != 12 || session.TokenUsage.TotalTokens == nil || *session.TokenUsage.TotalTokens != 20 {
			t.Fatalf("%s successful row %q token usage=%#v, want 8/12/20", label, session.WorkerSessionID, session.TokenUsage)
		}
		return
	}
	var failure struct {
		Kind string `json:"kind"`
	}
	if err := json.Unmarshal(session.Failure, &failure); err != nil || strings.TrimSpace(failure.Kind) == "" {
		t.Fatalf("%s failed row %q omitted typed failure=%s", label, session.WorkerSessionID, session.Failure)
	}
	if !strings.Contains(strings.ToLower(string(session.Failure)), "auth") && !strings.Contains(strings.ToLower(string(session.Failure)), "401") {
		t.Fatalf("%s failed row %q omitted authentication diagnosis=%s", label, session.WorkerSessionID, session.Failure)
	}
}

func assertBoundedFleetCompleteSelection(t *testing.T, first, second workerSessionListJSON, expected map[string]boundedFleetExpectedObservation) {
	t.Helper()
	all := append(append([]workerSessionJSON(nil), first.Sessions...), second.Sessions...)
	if len(all) != len(expected) {
		t.Fatalf("bounded fleet concatenated rows=%d, want %d", len(all), len(expected))
	}
	order := make([]string, 0, len(all))
	seenWorkerSessions := make(map[string]struct{}, len(all))
	seenWorks := make(map[string]struct{}, len(all))
	for _, session := range all {
		if _, duplicate := seenWorkerSessions[session.WorkerSessionID]; duplicate {
			t.Fatalf("bounded fleet concatenated pages duplicated Worker Session %q", session.WorkerSessionID)
		}
		seenWorkerSessions[session.WorkerSessionID] = struct{}{}
		if session.WorkID == nil {
			t.Fatalf("bounded fleet concatenated row omitted Work ID: %#v", session)
		}
		if _, duplicate := seenWorks[*session.WorkID]; duplicate {
			t.Fatalf("bounded fleet concatenated pages duplicated Work %q", *session.WorkID)
		}
		seenWorks[*session.WorkID] = struct{}{}
		order = append(order, session.WorkerSessionID)
	}
	if len(seenWorks) != len(expected) {
		t.Fatalf("bounded fleet concatenated unique Work count=%d, want %d", len(seenWorks), len(expected))
	}
	for workID := range expected {
		if _, ok := seenWorks[workID]; !ok {
			t.Fatalf("bounded fleet concatenated pages omitted Work %q", workID)
		}
	}
	assertAscendingWorkerSessionOrder(t, order, "bounded fleet concatenated")
}

func assertBoundedFleetNoMatch(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL string) {
	t.Helper()
	inputs := support.FakeInputs(ctx, []string{
		"you", "--server", baseURL, "worker-sessions", "list", "--state", "RUNNING", "--limit", fmt.Sprint(boundedFleetPageSize), "--output", "json",
	})
	inputs.Input.Env = append([]string(nil), env...)
	inputs.Input.WorkingDirectory = factoryDir
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("bounded fleet no-match CLI: %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	var cliList workerSessionListJSON
	decodeCLIJSON(t, inputs, &cliList)
	assertBoundedFleetEmptyPage(t, cliList, "CLI no-match")

	page := fetchBoundedFleetHTTPPageWithQuery(t, ctx, baseURL, url.Values{
		"state": {"RUNNING"}, "limit": {fmt.Sprint(boundedFleetPageSize)},
	})
	assertBoundedFleetEmptyPage(t, page.list, "HTTP no-match")
	assertNormalizedFleetJSONEqual(t, "no-match page", []byte(inputs.Stdout()), page.raw)
}

func assertBoundedFleetEmptyPage(t *testing.T, page workerSessionListJSON, label string) {
	t.Helper()
	if page.Sessions == nil || len(page.Sessions) != 0 {
		t.Fatalf("%s sessions=%#v, want non-nil empty array", label, page.Sessions)
	}
	if page.PaginationContext == nil || page.PaginationContext.MaxResults != boundedFleetPageSize || strings.TrimSpace(page.PaginationContext.NextToken) != "" {
		t.Fatalf("%s pagination=%#v, want maxResults=%d without token", label, page.PaginationContext, boundedFleetPageSize)
	}
}

func assertBoundedFleetMalformedToken(t *testing.T, ctx context.Context, process support.Process, env []string, factoryDir, baseURL string) {
	t.Helper()
	inputs, err := executeCLIExpectError(t, ctx, process, env, factoryDir,
		"--server", baseURL, "worker-sessions", "list", "--state", "COMPLETED", "--limit", fmt.Sprint(boundedFleetPageSize), "--next-token", "%%%", "--output", "json",
	)
	if err == nil {
		t.Fatal("bounded fleet malformed CLI token returned nil error")
	}
	if strings.TrimSpace(inputs.Stdout()) != "" {
		t.Fatalf("bounded fleet malformed CLI token emitted success stdout: %s", inputs.Stdout())
	}
	assertFleetJSONErrorCode(t, []byte(inputs.Stderr()), "BAD_REQUEST", "CLI malformed token")

	page := fetchBoundedFleetHTTPPageWithQuery(t, ctx, baseURL, url.Values{
		"state": {"COMPLETED"}, "limit": {fmt.Sprint(boundedFleetPageSize)}, "nextToken": {"%%%"},
	})
	if page.status != http.StatusBadRequest {
		t.Fatalf("HTTP malformed token status=%d, want %d body=%s", page.status, http.StatusBadRequest, page.raw)
	}
	assertFleetJSONErrorCode(t, page.raw, "BAD_REQUEST", "HTTP malformed token")
}

func fetchBoundedFleetHTTPPageWithQuery(t *testing.T, ctx context.Context, baseURL string, query url.Values) boundedFleetPageCapture {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/worker-sessions?" + query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		t.Fatalf("build bounded fleet HTTP request: %v", err)
	}
	started := time.Now()
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("bounded fleet HTTP request: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read bounded fleet HTTP response: %v", err)
	}
	capture := boundedFleetPageCapture{
		raw: body, elapsed: time.Since(started), status: response.StatusCode, headers: response.Header.Clone(),
	}
	if response.StatusCode == http.StatusOK {
		if err := json.Unmarshal(body, &capture.list); err != nil {
			t.Fatalf("decode bounded fleet HTTP response: %v\nbody:\n%s", err, body)
		}
	}
	return capture
}

func assertFleetJSONErrorCode(t *testing.T, raw []byte, want, label string) {
	t.Helper()
	var payload struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("%s error JSON: %v\nraw=%s", label, err, raw)
	}
	if payload.Code != want {
		t.Fatalf("%s error code=%q, want %q", label, payload.Code, want)
	}
}

func assertNormalizedFleetJSONEqual(t *testing.T, label string, left, right []byte) {
	t.Helper()
	var leftValue, rightValue any
	if err := json.Unmarshal(left, &leftValue); err != nil {
		t.Fatalf("decode %s CLI JSON: %v\nraw=%s", label, err, left)
	}
	if err := json.Unmarshal(right, &rightValue); err != nil {
		t.Fatalf("decode %s HTTP JSON: %v\nraw=%s", label, err, right)
	}
	leftNormalized, err := json.Marshal(normalizeFleetJSONNulls(leftValue))
	if err != nil {
		t.Fatalf("normalize %s CLI JSON: %v", label, err)
	}
	rightNormalized, err := json.Marshal(normalizeFleetJSONNulls(rightValue))
	if err != nil {
		t.Fatalf("normalize %s HTTP JSON: %v", label, err)
	}
	if string(leftNormalized) != string(rightNormalized) {
		t.Fatalf("%s HTTP/CLI JSON differs after null normalization:\nCLI=%s\nHTTP=%s", label, leftNormalized, rightNormalized)
	}
}

func normalizeFleetJSONNulls(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, child := range typed {
			if child == nil {
				continue
			}
			normalized[key] = normalizeFleetJSONNulls(child)
		}
		return normalized
	case []any:
		for index := range typed {
			typed[index] = normalizeFleetJSONNulls(typed[index])
		}
		return typed
	default:
		return value
	}
}

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
