package cli_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type activeFleetFixture struct {
	c                           *workerSessionsCLICase
	env                         []string
	works, sessions, ids        map[string]string
	terminalOwner, terminalWork string
}

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
	cohort := activeFleetFixture{c: c, env: env, works: works, sessions: sessions, ids: expectedIDs,
		terminalOwner: terminalOwner, terminalWork: terminalWork}
	cohort.assertPages(t, ctx)
	cohort.assertWorkReads(t, ctx)
	cohort.assertEmptyReads(t, ctx)
	assertBoundedFleetMalformedToken(t, ctx, f.process, env, c.factoryDir, f.baseURL)
	cohort.assertOptionalLoss(t, ctx)
	cohort.assertCanceledRead(t, ctx)
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

func (a activeFleetFixture) assertPages(t *testing.T, ctx context.Context) {
	t.Helper()
	c := a.c
	f := c.fixture
	env := a.env
	works, sessions, expectedIDs := a.works, a.sessions, a.ids
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
}

func (a activeFleetFixture) assertWorkReads(t *testing.T, ctx context.Context) {
	t.Helper()
	c := a.c
	f := c.fixture
	env := a.env
	works, sessions, expectedIDs := a.works, a.sessions, a.ids
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
}

func (a activeFleetFixture) assertEmptyReads(t *testing.T, ctx context.Context) {
	t.Helper()
	c := a.c
	f := c.fixture
	env := a.env
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
}

func (a activeFleetFixture) assertOptionalLoss(t *testing.T, ctx context.Context) {
	t.Helper()
	c := a.c
	f := c.fixture
	env := a.env
	works, sessions, expectedIDs := a.works, a.sessions, a.ids
	terminalOwner, terminalWork := a.terminalOwner, a.terminalWork
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
}

func (a activeFleetFixture) assertCanceledRead(t *testing.T, ctx context.Context) {
	t.Helper()
	c := a.c
	f := c.fixture
	env := a.env
	works, sessions, expectedIDs := a.works, a.sessions, a.ids
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
