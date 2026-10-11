package mock

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	generatedclient "github.com/portpowered/infinite-you/pkg/transports/http/client"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const mockUsageWorkID = "mock-usage-costs"

func testMockUsageBusinessInvalid(t *testing.T, fixture *sharedWorkersMockFixture) {
	dir := declaredResultFactory(t)
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{WorkID: "mock-usage-business", WorkTypeID: "task", Payload: []byte(`{"title":"invalid business output"}`)})
	fixture.useCommandRunnersFor(t, dir, nil, nil)
	session := fixture.openSession(t, dir)
	defer session.closeAndAssertGone(t)
	listed, events := session.terminalObservations(t, 15*time.Second)
	item := singleMockUsageWork(t, listed)
	if item.State == nil || item.State.Type != factoryapi.WorkStateTypeFAILED {
		t.Fatalf("business-invalid Work was not failed: %+v", item)
	}
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 || dispatches[0].Response == nil || dispatches[0].Response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("business dispatch outcome: %+v", dispatches)
	}
	id, observation := usageWorkerSession(t, fixture.server.URL(), session.id, item)
	if observation.State != factoryapi.WorkerSessionObservationStateCompleted {
		t.Fatalf("business failure changed physical Worker outcome: %+v", observation)
	}
	body := executeMockUsageCLI(t, fixture, dir, "--json", "--server", fixture.server.URL(), "worker-sessions", "read", "--worker-session-id", id, "--view", "logs")
	assertCapturedMockUsage(t, body, "codex", "gpt-5-codex", "MOCK_USAGE_BUSINESS_OK", 22)
	assertMockUsageReadParity(t, fixture.server, dir, nil, observation, body)
}

func testMockUsageNativePassthrough(t *testing.T, fixture *sharedWorkersMockFixture) {
	dir := matchedMockRejectionFactory(t, "agent")
	stdout := support.CodexSuccessStdoutWithUsage("NATIVE_USAGE_OK", 17, 5)
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{Stdout: stdout})
	fixture.useCommandRunnersFor(t, dir, runner, nil)
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{WorkID: "native-usage", WorkTypeID: "task", Payload: []byte(`{"title":"native usage"}`)})
	session := fixture.openSession(t, dir)
	defer session.closeAndAssertGone(t)
	listed, _ := session.terminalObservations(t, 15*time.Second)
	item := singleMockUsageWork(t, listed)
	id, observation := usageWorkerSession(t, fixture.server.URL(), session.id, item)
	if len(runner.Requests()) != 1 || observation.TokenUsage == nil {
		t.Fatalf("native passthrough did not execute once with usage: %+v", observation)
	}
	assertMockUsageObservationToken(t, observation.TokenUsage.InputTokens, 17, "native input")
	assertMockUsageObservationToken(t, observation.TokenUsage.OutputTokens, 5, "native output")
	if observation.TokenUsage.TotalTokens != nil {
		t.Fatal("native unavailable total was inferred")
	}
	if observation.TokenUsage.Origin != nil && *observation.TokenUsage.Origin == factoryapi.ProviderSessionTokenUsageOriginSYNTHETIC {
		t.Fatal("native usage was relabeled synthetic")
	}
	body := executeMockUsageCLI(t, fixture, dir, "--json", "--server", fixture.server.URL(), "worker-sessions", "read", "--worker-session-id", id, "--view", "logs")
	if strings.Contains(body, `"origin":"SYNTHETIC"`) || !strings.Contains(body, "NATIVE_USAGE_OK") {
		t.Fatalf("native passthrough capture changed: %s", body)
	}
	assertMockUsageReadParity(t, fixture.server, dir, nil, observation, body)
}

func testMockUsageCapture(t *testing.T, fixture *sharedWorkersMockFixture) {
	for _, provider := range []string{"claude", "codex"} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			model := "gpt-5-codex"
			if provider == "claude" {
				model = "claude-sonnet-4-6"
			}
			dir := testutil.CopyFixtureDir(t, support.AgentFactoryPath(t, "examples/simple-tasks"))
			support.WriteAgentConfig(t, dir, "executor", support.BuildModelWorkerConfig(modelprovider.Provider(provider), model))
			support.ClearSeedInputs(t, dir)
			testutil.WriteSeedRequest(t, dir, work.SubmitRequest{WorkID: "mock-usage-capture", WorkTypeID: "story", Payload: []byte(`{"title":"capture usage"}`)})
			fixture.useCommandRunnersFor(t, dir, nil, nil)
			session := fixture.openSession(t, dir)
			listed, _ := session.terminalObservations(t, 15*time.Second)
			defer session.closeAndAssertGone(t)
			item := singleMockUsageWork(t, listed)
			id, observation := usageWorkerSession(t, fixture.server.URL(), session.id, item)
			if observation.Provider == nil || *observation.Provider != provider || observation.Model == nil || *observation.Model != model || observation.FactorySessionId == nil || *observation.FactorySessionId != session.id || observation.WorkId == nil || *observation.WorkId != "mock-usage-capture" {
				t.Fatalf("actual execution identity changed: %+v", observation)
			}
			assertMockUsageObservationToken(t, observation.TokenUsage.TotalTokens, 22, "total")
			assertMockUsageObservationToken(t, observation.TokenUsage.InputTokens, 17, "input")
			assertMockUsageObservationToken(t, observation.TokenUsage.OutputTokens, 5, "output")
			assertMockUsageObservationToken(t, observation.TokenUsage.CachedInputTokens, 0, "cached")
			assertMockUsageObservationToken(t, observation.TokenUsage.ReasoningOutputTokens, 0, "reasoning")
			body := executeMockUsageCLI(t, fixture, dir, "--json", "--server", fixture.server.URL(), "worker-sessions", "read", "--worker-session-id", id, "--view", "logs")
			assertCapturedMockUsage(t, body, provider, "gpt-5-codex", "MOCK_USAGE_FACTORY_OK", 22)
			assertMockUsageReadParity(t, fixture.server, dir, nil, observation, body)
			for _, scoped := range []bool{false, true} {
				args := []string{"--json", "--server", fixture.server.URL(), "worker-sessions", "show", "--worker-session-id", id}
				if scoped {
					args = append(args, "--session", session.id)
				}
				var summary factoryapi.WorkerSessionObservation
				if err := json.Unmarshal([]byte(executeMockUsageCLI(t, fixture, dir, args...)), &summary); err != nil || summary.Model == nil || *summary.Model != model || summary.TokenUsage == nil || *summary.TokenUsage.TotalTokens != 22 {
					t.Fatalf("CLI summary disagrees with HTTP list: %+v %v", summary, err)
				}
			}
		})
	}
}

// Read each committed record through customer transports, including cursor
// polling. The adapter reuses the scenario's root process and owns only pipes.
func assertMockUsageReadParity(t *testing.T, server *support.FunctionalAPIServer, dir string, env []string, observation factoryapi.WorkerSessionObservation, body string) {
	t.Helper()
	connection, ctx := mockUsageMCP(t, server, dir, env)
	var whole factoryapi.WorkerSessionLogPage
	if err := json.Unmarshal([]byte(body), &whole); err != nil {
		t.Fatal(err)
	}
	var events []factoryapi.WorkerSessionEvent
	token := ""
	for {
		args := map[string]any{"action": "READ", "workerSessionId": observation.WorkerSessionId, "view": "logs", "limit": 1}
		query := url.Values{"limit": {"1"}}
		cliArgs := []string{"you", "--json", "--server", server.URL(), "worker-sessions", "read", "--worker-session-id", observation.WorkerSessionId, "--view", "logs", "--limit", "1"}
		path := "/worker-sessions/" + url.PathEscape(observation.WorkerSessionId) + "/logs"
		if token != "" {
			args["nextToken"] = token
			query.Set("nextToken", token)
			cliArgs = append(cliArgs, "--next-token", token)
		}
		result, err := connection.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: args})
		if err != nil || result.IsError || len(result.Content) != 1 {
			t.Fatalf("MCP usage read: %+v %v", result, err)
		}
		var envelope struct {
			Result struct {
				Session factoryapi.WorkerSessionObservation `json:"session"`
				Logs    factoryapi.WorkerSessionLogPage     `json:"logs"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &envelope); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(envelope.Result.Session.TokenUsage, observation.TokenUsage) || !reflect.DeepEqual(envelope.Result.Session.Model, observation.Model) || !reflect.DeepEqual(envelope.Result.Session.Provider, observation.Provider) {
			t.Fatalf("MCP changed execution/usage facts: %+v", envelope.Result.Session)
		}
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL()+path+"?"+query.Encode(), nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var httpPage factoryapi.WorkerSessionLogPage
		err = json.NewDecoder(response.Body).Decode(&httpPage)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("HTTP usage logs: %d %v", response.StatusCode, err)
		}
		input := support.FakeInputs(ctx, cliArgs)
		input.Input.WorkingDirectory = dir
		if env != nil {
			input.Input.Env = env
		}
		if err := server.Execute(t, input.Input); err != nil {
			t.Fatalf("CLI usage logs: %v %s", err, input.Stderr())
		}
		var cliPage factoryapi.WorkerSessionLogPage
		if err := json.Unmarshal([]byte(input.Stdout()), &cliPage); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(httpPage, cliPage) || !reflect.DeepEqual(httpPage, envelope.Result.Logs) {
			t.Fatalf("paged CLI/HTTP/MCP usage differs: %+v %+v %+v", cliPage, httpPage, envelope.Result.Logs)
		}
		events = append(events, httpPage.Events...)
		if httpPage.NextToken == nil || *httpPage.NextToken == "" {
			break
		}
		if token == *httpPage.NextToken {
			t.Fatal("cursor did not advance")
		}
		token = *httpPage.NextToken
	}
	if !reflect.DeepEqual(events, whole.Events) {
		t.Fatalf("cursor replay changed ordered committed records: %+v %+v", events, whole.Events)
	}
}

func mockUsageMCP(t *testing.T, server *support.FunctionalAPIServer, dir string, env []string) (*mcp.ClientSession, context.Context) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	stdinRead, stdinWrite := io.Pipe()
	stdoutRead, stdoutWrite := io.Pipe()
	inputs := support.FakeInputs(ctx, []string{"you", "--server", server.URL(), "server", "mcp"})
	inputs.Input.WorkingDirectory = dir
	if env != nil {
		inputs.Input.Env = env
	}
	inputs.Input.Stdin, inputs.Input.Stdout = stdinRead, stdoutWrite
	done := make(chan error, 1)
	go func() {
		err := server.Execute(t, inputs.Input)
		_ = stdinRead.CloseWithError(err)
		_ = stdoutWrite.CloseWithError(err)
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		_ = stdinWrite.Close()
		_ = stdinRead.Close()
		_ = stdoutRead.Close()
		_ = stdoutWrite.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
				t.Errorf("MCP shutdown: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Error("MCP did not join")
		}
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "mock-usage-read", Version: "test"}, nil)
	connection, err := client.Connect(ctx, &mcp.IOTransport{Reader: stdoutRead, Writer: stdinWrite}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return connection, ctx
}

func assertMockUsagePeerCursor(t *testing.T, server *support.FunctionalAPIServer, dir string, env []string, source, peer string) {
	t.Helper()
	connection, ctx := mockUsageMCP(t, server, dir, env)
	result, err := connection.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{"action": "READ", "workerSessionId": source, "view": "logs", "limit": 1}})
	if err != nil || result.IsError {
		t.Fatalf("source cursor read: %+v %v", result, err)
	}
	var envelope struct {
		Result struct {
			Logs factoryapi.WorkerSessionLogPage `json:"logs"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Result.Logs.NextToken == nil {
		t.Fatal("source history did not produce a cursor")
	}
	token := *envelope.Result.Logs.NextToken
	result, err = connection.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{"action": "READ", "workerSessionId": peer, "view": "logs", "nextToken": token}})
	if err != nil || !result.IsError {
		t.Fatalf("foreign cursor MCP: %+v %v", result, err)
	}
	var failure struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &failure); err != nil || failure.Error.Code != "worker_session.invalid_request" {
		t.Fatalf("foreign cursor MCP error: %+v %v", failure, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL()+"/worker-sessions/"+url.PathEscape(peer)+"/logs?nextToken="+url.QueryEscape(token), nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	var httpFailure factoryapi.ErrorResponse
	err = json.NewDecoder(response.Body).Decode(&httpFailure)
	_ = response.Body.Close()
	if err != nil || response.StatusCode != http.StatusBadRequest || httpFailure.Code != factoryapi.ErrorResponseCodeBADREQUEST {
		t.Fatalf("foreign cursor HTTP: %d %+v %v", response.StatusCode, httpFailure, err)
	}
	inputs := support.FakeInputs(ctx, []string{"you", "--json", "--server", server.URL(), "worker-sessions", "read", "--worker-session-id", peer, "--view", "logs", "--next-token", token})
	inputs.Input.WorkingDirectory, inputs.Input.Env = dir, env
	if err := server.Execute(t, inputs.Input); err == nil {
		t.Fatal("foreign cursor CLI succeeded")
	}
	var cliFailure factoryapi.ErrorResponse
	if err := json.Unmarshal([]byte(inputs.Stderr()), &cliFailure); err != nil || cliFailure.Code != httpFailure.Code || inputs.Stdout() != "" {
		t.Fatalf("foreign cursor CLI: %s %s %v", inputs.Stdout(), inputs.Stderr(), err)
	}
}

func assertCapturedMockUsage(t testing.TB, body, provider, model, marker string, total int64) {
	t.Helper()
	var page factoryapi.WorkerSessionLogPage
	if err := json.Unmarshal([]byte(body), &page); err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, event := range page.Events {
		raw, err := json.Marshal(event.Event.Payload)
		if err != nil {
			t.Fatal(err)
		}
		var draft workers.Draft
		if err := json.Unmarshal(raw, &draft); err != nil {
			t.Fatal(err)
		}
		if draft.Kind != workers.KindUsage {
			continue
		}
		count++
		var usage workers.UsagePayload
		if err := json.Unmarshal(draft.Payload, &usage); err != nil || usage.Origin != "SYNTHETIC" || usage.Model != model || usage.TotalTokens != total || draft.Provenance.Provider != provider {
			t.Fatalf("captured declaration identity/counters: %+v %v", draft, err)
		}
	}
	if count != 1 || !strings.Contains(body, marker) {
		t.Fatalf("want one usage and known output, got %d: %s", count, body)
	}
}

// testMockWorkerUsageIsVisibleAndPriceableThroughSharedProcess proves the
// documented mock-worker path produces one correlated, priceable usage row
// without invoking a live provider or reading a recording fixture.
func testMockWorkerUsageIsVisibleAndPriceableThroughSharedProcess(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	factoryDir := testutil.CopyFixtureDir(t, support.AgentFactoryPath(t, "examples/simple-tasks"))
	support.WriteAgentConfig(t, factoryDir, "executor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	support.ClearSeedInputs(t, factoryDir)
	testutil.WriteSeedRequest(t, factoryDir, work.SubmitRequest{
		WorkID:     mockUsageWorkID,
		Name:       "mock usage pricing",
		WorkTypeID: "story",
		TraceID:    "trace-mock-usage-costs",
		Payload:    []byte(`{"title":"price mock usage"}`),
	})

	fixture.useCommandRunnersFor(t, factoryDir, nil, nil)
	session := fixture.openSession(t, factoryDir)
	listed, _ := session.terminalObservations(t, 15*time.Second)
	defer session.closeAndAssertGone(t)
	workItem := singleMockUsageWork(t, listed)
	workerSessionID, observation := usageWorkerSession(t, fixture.server.URL(), session.id, workItem)

	listOutput := executeMockUsageCLI(t, fixture, factoryDir,
		"--server", fixture.server.URL(), "worker-sessions", "list", "--session", session.id,
		"--work-id", *workItem.WorkId)

	costOutput := executeMockUsageCLI(t, fixture, factoryDir,
		"--server", fixture.server.URL(), "metrics", "costs", "--session", session.id)
	assertMockUsageListOutput(t, listOutput, workerSessionID)
	assertMockUsageObservation(t, observation, workerSessionID, session.id, workItem)
	assertMockUsageCostOutput(t, costOutput)
	logs := executeMockUsageCLI(t, fixture, factoryDir, "--json", "--server", fixture.server.URL(), "worker-sessions", "read", "--worker-session-id", workerSessionID, "--view", "logs")
	if !strings.Contains(logs, `"origin":"SYNTHETIC"`) || !strings.Contains(logs, `"delivery":"SYNTHESIZED"`) {
		t.Fatalf("captured mock usage lost public origin/delivery: %s", logs)
	}
	summary := executeMockUsageCLI(t, fixture, factoryDir, "--json", "--server", fixture.server.URL(), "worker-sessions", "show", "--worker-session-id", workerSessionID)
	if !strings.Contains(summary, `"origin":"SYNTHETIC"`) {
		t.Fatalf("CLI mock summary lost origin: %s", summary)
	}

	costJSON := executeMockUsageCLI(t, fixture, factoryDir,
		"--json", "--server", fixture.server.URL(), "metrics", "costs", "--session", session.id)
	assertMockUsageCostReport(t, costJSON, workerSessionID, workItem)
}

func singleMockUsageWork(t testing.TB, listed factoryapi.ListWorkResponse) factoryapi.Work {
	t.Helper()
	if len(listed.Results) != 1 {
		t.Fatalf("mock usage Work count = %d, want one: %#v", len(listed.Results), listed.Results)
	}
	return listed.Results[0]
}

func usageWorkerSession(
	t testing.TB,
	baseURL, sessionID string,
	workItem factoryapi.Work,
) (string, factoryapi.WorkerSessionObservation) {
	t.Helper()
	if workItem.WorkId == nil || strings.TrimSpace(*workItem.WorkId) == "" {
		t.Fatalf("mock usage Work has no Work ID: %#v", workItem)
	}
	sessions, err := support.WaitForObservation(
		2*time.Second,
		func() (factoryapi.ListWorkerSessionsResponse, error) {
			endpoint := strings.TrimSuffix(baseURL, "/") +
				"/factory-sessions/" + url.PathEscape(sessionID) +
				"/worker-sessions?workId=" + url.QueryEscape(*workItem.WorkId)
			return support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, endpoint), nil
		},
		func(value factoryapi.ListWorkerSessionsResponse) bool {
			return usageWorkerSessionObservationFromResponse(value).WorkerSessionId != ""
		},
	)
	if err != nil {
		t.Fatalf("waiting for usage-bearing Worker Session: %v", err)
	}
	observation := usageWorkerSessionObservationFromResponse(sessions)
	return observation.WorkerSessionId, observation
}

func usageWorkerSessionObservationFromResponse(sessions factoryapi.ListWorkerSessionsResponse) factoryapi.WorkerSessionObservation {
	var usageObservation factoryapi.WorkerSessionObservation
	usageCount := 0
	for _, session := range sessions.Sessions {
		if session.TokenUsage == nil {
			continue
		}
		usageObservation = session
		usageCount++
	}
	if usageCount == 1 {
		return usageObservation
	}
	return factoryapi.WorkerSessionObservation{}
}

func executeMockUsageCLI(
	t testing.TB,
	fixture *sharedWorkersMockFixture,
	workingDirectory string,
	args ...string,
) string {
	t.Helper()
	inputs, err := fixture.executeCLI(t, workingDirectory, args...)
	if err != nil {
		t.Fatalf("execute public CLI %v: %v\nstdout=%s\nstderr=%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	return inputs.Stdout()
}

func assertMockUsageListOutput(t testing.TB, output, workerSessionID string) {
	t.Helper()
	for _, expected := range []string{
		"WORKER SESSION ID",
		workerSessionID,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("worker-sessions list output missing %q:\n%s", expected, output)
		}
	}
}

func assertMockUsageObservation(
	t testing.TB,
	observation factoryapi.WorkerSessionObservation,
	workerSessionID, sessionID string,
	workItem factoryapi.Work,
) {
	t.Helper()
	if observation.WorkerSessionId != workerSessionID || observation.FactorySessionId == nil ||
		*observation.FactorySessionId != sessionID || observation.WorkId == nil || workItem.WorkId == nil ||
		*observation.WorkId != *workItem.WorkId || observation.Model == nil || *observation.Model != "gpt-5-codex" ||
		observation.TokenUsage == nil {
		t.Fatalf("Worker Session observation = %#v, want correlated session/work/model/usage", observation)
	}
	usage := observation.TokenUsage
	if usage.Origin == nil || *usage.Origin != factoryapi.ProviderSessionTokenUsageOriginSYNTHETIC {
		t.Fatalf("mock usage origin = %+v", usage)
	}
	assertMockUsageObservationToken(t, usage.InputTokens, 1_000_000, "input")
	assertMockUsageObservationToken(t, usage.CachedInputTokens, 400_000, "cached input")
	assertMockUsageObservationToken(t, usage.OutputTokens, 500_000, "output")
	assertMockUsageObservationToken(t, usage.ReasoningOutputTokens, 100_000, "reasoning output")
}

func assertMockUsageObservationToken(t testing.TB, got *int, want int, name string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("Worker Session %s tokens = %v, want %d", name, got, want)
	}
}

func assertMockUsageCostOutput(t testing.TB, output string) {
	t.Helper()
	for _, expected := range []string{
		"Status: PRICED",
		"Cost (USD): $5.80",
		"Unpriced dispatches: 0",
		"Unpriced usage: 0 rows",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("metrics costs output missing %q:\n%s", expected, output)
		}
	}
}

func assertMockUsageCostReport(
	t testing.TB,
	output string,
	workerSessionID string,
	workItem factoryapi.Work,
) {
	t.Helper()
	var report generatedclient.CostsReport
	if err := json.Unmarshal([]byte(output), &report); err != nil {
		t.Fatalf("decode metrics costs JSON: %v\noutput=%s", err, output)
	}
	if report.Status != generatedclient.CostsReportStatus("PRICED") {
		t.Fatalf("metrics costs status = %q, want PRICED", report.Status)
	}
	if report.KnownCost == nil || *report.KnownCost != "5.8" || report.PricedSubtotal == nil || *report.PricedSubtotal != "5.8" {
		t.Fatalf("metrics costs amounts = known=%v subtotal=%v, want exact 5.8", report.KnownCost, report.PricedSubtotal)
	}
	if report.UnpricedDispatchCount != 0 || report.Coverage.EncounteredRows != 1 || report.Coverage.PricedRows != 1 ||
		report.Coverage.UnpricedRows != 0 || report.Coverage.EncounteredProviderModels != 1 || report.Coverage.PricedProviderModels != 1 ||
		report.Coverage.UnpricedProviderModels != 0 || len(report.LineItems) != 1 {
		t.Fatalf("metrics costs coverage = %#v, line items=%d, want one fully priced row", report.Coverage, len(report.LineItems))
	}
	item := report.LineItems[0]
	if item.Provider == nil || *item.Provider != "CODEX" || item.Model == nil || *item.Model != "gpt-5-codex" ||
		item.WorkerSessionId == nil || *item.WorkerSessionId != workerSessionID || item.WorkId == nil ||
		workItem.WorkId == nil || *item.WorkId != *workItem.WorkId || item.Status != generatedclient.CostsLineItemStatus("PRICED") ||
		item.PricedAmount == nil || *item.PricedAmount != "5.8" {
		t.Fatalf("metrics costs line item = %#v, want correlated priced codex row", item)
	}
	assertMockUsageToken(t, item.InputTokens, 1_000_000, "input")
	assertMockUsageToken(t, item.CachedInputTokens, 400_000, "cached input")
	assertMockUsageToken(t, item.OutputTokens, 500_000, "output")
	assertMockUsageToken(t, item.ReasoningOutputTokens, 100_000, "reasoning output")
}

func assertMockUsageToken(t testing.TB, got *int64, want int64, name string) {
	t.Helper()
	if got == nil || *got != want {
		t.Fatalf("metrics costs %s tokens = %v, want %d", name, got, want)
	}
}
