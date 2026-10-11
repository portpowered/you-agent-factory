package recording_sidecar_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type mockUsageRestartSnapshot struct {
	Observation api.WorkerSessionObservation
	Logs        api.WorkerSessionLogPage
	Page        api.WorkerSessionLogPage
}

// The invoking build supplies one deliverable; this test never compiles it.
// Host lifetimes serialize because a profile has exactly one capture writer.
func runMockUsageRestart(t *testing.T) {
	binary := invokeArtifactBinary(t)
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Minute)
	defer cancel()
	project := t.TempDir()
	home := filepath.Join(project, "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	env := append(builtcliacceptance.ProcessEnvForIsolatedHome(home), "PATH="+home)
	factory, config := writeMockUsageRestartFactory(t, project)
	first := startHistoryHost(t, ctx, binary, project, env, "--dir", factory, "--continuously", "--with-mock-workers", config)
	for _, name := range []string{"cross", "match", "zero", "omitted", "none"} {
		provider, model := "claude", "claude-sonnet-4-6"
		if name == "match" {
			provider, model = "codex", "gpt-5-codex"
		}
		id := "direct-" + name
		document := map[string]any{"workerSessionId": id, "requestId": id + "-request", "execution": map[string]any{
			"workstationName": id, "workingDirectory": project, "runnerId": provider, "executorProvider": provider,
			"modelProvider": provider, "model": model, "userMessage": "mock usage restart",
			"dispatch": map[string]any{"dispatchId": id + "-attempt", "workstationName": id},
		}}
		path := filepath.Join(project, id+".json")
		writeRecordingIntegrityJSON(t, path, document)
		historyCLI(t, ctx, binary, project, env, "--remote", "--server", first.url, "--json", "worker-sessions", "invoke", "--execution", path)
	}
	before := readMockUsageRestartSnapshots(t, ctx, binary, project, env, first.url, nil)
	removeHistorySeeds(t, factory) // completed requests must not be admitted again on reopen
	first.stop(t, ctx, binary, project, env)
	for _, provider := range []string{".claude", ".codex", ".cursor"} {
		if err := os.WriteFile(filepath.Join(home, provider), []byte("provider files unavailable"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	second := startHistoryHost(t, ctx, binary, project, env)
	after := readMockUsageRestartSnapshots(t, ctx, binary, project, env, second.url, before)
	for id, want := range before {
		got := after[id]
		want.Observation.ConfirmationState = got.Observation.ConfirmationState
		if !reflect.DeepEqual(want, got) {
			t.Fatalf("restart changed %s identity/counters/ordered capture/cursor", id)
		}
	}
	second.stop(t, ctx, binary, project, env)
	// Recordings are selected by invocation directory as well as operator
	// home. A foreign profile must not reuse the original recording directory.
	foreignProject := t.TempDir()
	foreignHome := filepath.Join(foreignProject, "home")
	if err := os.MkdirAll(foreignHome, 0o700); err != nil {
		t.Fatal(err)
	}
	foreignFactory := filepath.Join(foreignProject, "factory")
	if err := os.MkdirAll(foreignFactory, 0o700); err != nil {
		t.Fatal(err)
	}
	writeRecordingIntegrityJSON(t, filepath.Join(foreignFactory, "factory.json"), map[string]any{
		"name": "foreign-profile", "workTypes": []map[string]any{{"name": "task", "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "done", "type": "TERMINAL"}}}},
		"workers": []any{}, "workstations": []any{},
	})
	foreignEnv := append(builtcliacceptance.ProcessEnvForIsolatedHome(foreignHome), "PATH="+foreignHome)
	foreign := startHistoryHost(t, ctx, binary, foreignProject, foreignEnv)
	if rows := historyRows(t, ctx, binary, foreignProject, foreignEnv, foreign.url, "all"); len(rows) != 0 {
		t.Fatalf("foreign profile leaked captures: %+v", rows)
	}
	assertMockUsageRestartForeignList(t, invokeArtifactFixture{ctx: ctx, binary: binary, project: foreignProject, env: foreignEnv}, foreign.url)
	for id := range before {
		assertMockUsageRestartForeign(t, ctx, foreign.url, id)
	}
	foreign.stop(t, ctx, binary, foreignProject, foreignEnv)
}

func writeMockUsageRestartFactory(t *testing.T, project string) (string, string) {
	factory := filepath.Join(project, "factory")
	if err := os.MkdirAll(filepath.Join(factory, "inputs", "task"), 0o700); err != nil {
		t.Fatal(err)
	}
	var stations, agents, mocks []map[string]any
	usage := map[string]any{"provider": "codex", "model": "gpt-5-codex", "inputTokens": 17, "outputTokens": 5, "cachedInputTokens": 0, "reasoningOutputTokens": 0}
	for _, name := range []string{"cross", "match"} {
		provider, model := "claude", "claude-sonnet-4-6"
		if name == "match" {
			provider, model = "codex", "gpt-5-codex"
		}
		agents = append(agents, map[string]any{"name": name, "type": "AGENT_WORKER", "executorProvider": provider, "modelProvider": provider, "model": model})
		stations = append(stations, map[string]any{"name": name, "type": "AGENT_RUN", "worker": name, "inputs": []map[string]string{{"workType": "task", "state": "init" + name}}, "outputs": []map[string]string{{"workType": "task", "state": "done"}}, "onFailure": []map[string]string{{"workType": "task", "state": "failed"}}})
		mocks = append(mocks, map[string]any{"workstationName": name, "runType": "accept", "resultBody": map[string]string{"output": "MOCK_USAGE_FACTORY_" + name}, "usage": usage})
		testutil.WriteSeedRequest(t, factory, work.SubmitRequest{WorkID: "factory-" + name, WorkTypeID: "task", TargetState: "init" + name, Payload: []byte(`{"title":"usage restart"}`)})
	}
	states := []map[string]string{{"name": "initcross", "type": "INITIAL"}, {"name": "initmatch", "type": "INITIAL"}, {"name": "done", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"}}
	writeRecordingIntegrityJSON(t, filepath.Join(factory, "factory.json"), map[string]any{"name": "mock-usage-restart", "workTypes": []map[string]any{{"name": "task", "states": states}}, "workers": agents, "workstations": stations})
	for _, name := range []string{"cross", "match", "zero", "omitted", "none"} {
		entry := map[string]any{"workstationName": "direct-" + name, "runType": "accept", "resultBody": map[string]string{"output": "MOCK_USAGE_DIRECT_" + name}}
		switch name {
		case "cross", "match":
			entry["usage"] = usage
		case "zero":
			entry["usage"] = map[string]any{"provider": "codex", "model": "gpt-5-codex", "inputTokens": 0, "outputTokens": 5}
		case "omitted":
			entry["usage"] = map[string]any{"provider": "codex", "model": "gpt-5-codex", "outputTokens": 5}
		}
		mocks = append(mocks, entry)
	}
	config := filepath.Join(project, "mock-usage.json")
	writeRecordingIntegrityJSON(t, config, map[string]any{"mockWorkers": mocks})
	return factory, config
}

func readMockUsageRestartSnapshots(t *testing.T, ctx context.Context, binary, project string, env []string, server string, saved map[string]mockUsageRestartSnapshot) map[string]mockUsageRestartSnapshot {
	rows := awaitMockUsageRestartRows(t, ctx, binary, project, env, server)
	result := make(map[string]mockUsageRestartSnapshot)
	for _, row := range rows {
		result[row.WorkerSessionId] = readMockUsageRestartSnapshot(t, ctx, binary, project, env, server, row, saved)
	}
	return result
}

func awaitMockUsageRestartRows(t *testing.T, ctx context.Context, binary, project string, env []string, server string) []api.WorkerSessionObservation {
	// Commit completion is asynchronous; public capture health is the witness.
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	var rows []api.WorkerSessionObservation
	for {
		rows = historyRows(t, ctx, binary, project, env, server, "archived")
		ready := len(rows) == 7
		for _, row := range rows {
			ready = ready && row.RecordingHealth != nil && *row.RecordingHealth == "COMPLETE"
		}
		if ready {
			return rows
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("mock usage captures incomplete: %v %+v", ctx.Err(), rows)
		}
	}

}

func readMockUsageRestartSnapshot(t *testing.T, ctx context.Context, binary, project string, env []string, server string, row api.WorkerSessionObservation, saved map[string]mockUsageRestartSnapshot) mockUsageRestartSnapshot {
	id := row.WorkerSessionId
	var shown api.WorkerSessionObservation
	body := historyCLI(t, ctx, binary, project, env, "--server", server, "--json", "worker-sessions", "show", "--worker-session-id", id)
	if err := json.Unmarshal(body, &shown); err != nil {
		t.Fatal(err)
	}
	// Runtime confirmation is sampled independently of capture reads.
	row.ConfirmationState = shown.ConfirmationState
	if !reflect.DeepEqual(row, shown) {
		t.Fatalf("list/show captured facts differ: row=%+v shown=%s", row, body)
	}
	logs := historyLogPage(t, ctx, binary, project, env, server, id, "")
	assertMockUsageRestartFacts(t, shown, logs)
	f := invokeArtifactFixture{ctx: ctx, binary: binary, project: project, env: env}
	var httpShown api.WorkerSessionObservation
	readSummaryArtifactHTTP(t, f, server, "/worker-sessions/"+url.PathEscape(id), &httpShown)
	httpShown.ConfirmationState = shown.ConfirmationState
	if !reflect.DeepEqual(shown, httpShown) {
		t.Fatal("CLI/HTTP execution identity differs")
	}
	if shown.FactorySessionId != nil {
		body := historyCLI(t, ctx, binary, project, env, "--server", server, "--json", "worker-sessions", "show", "--session", *shown.FactorySessionId, "--worker-session-id", id)
		var scoped api.WorkerSessionObservation
		if err := json.Unmarshal(body, &scoped); err != nil {
			t.Fatal(err)
		}
		scoped.ConfirmationState = shown.ConfirmationState
		if !reflect.DeepEqual(shown, scoped) {
			t.Fatalf("original Factory scope changed: %s", body)
		}
		readSummaryArtifactHTTP(t, f, server, "/factory-sessions/"+url.PathEscape(*shown.FactorySessionId)+"/worker-sessions/"+url.PathEscape(id), &scoped)
		scoped.ConfirmationState = shown.ConfirmationState
		if !reflect.DeepEqual(shown, scoped) {
			t.Fatal("original Factory HTTP scope changed")
		}
	}
	token := ""
	if saved != nil {
		token = *saved[id].Page.NextToken
	}
	page := mockUsageRestartPage(t, f, server, shown, token)
	if saved == nil {
		if page.NextToken == nil || len(page.Events) != 1 {
			t.Fatal("first committed page did not return cursor")
		}
		mockUsageRestartPage(t, f, server, shown, *page.NextToken)
	} else {
		// Resume the saved cursor on the new host, then retain the identical
		// first page for full snapshot comparison.
		if !reflect.DeepEqual(page.Events, logs.Events[1:]) {
			t.Fatal("saved cursor changed committed suffix")
		}
		page = mockUsageRestartPage(t, f, server, shown, "")
	}
	assertHistoryMCPViews(t, ctx, binary, project, env, server, shown, logs, []string{"logs"})
	return mockUsageRestartSnapshot{shown, logs, page}
}

func assertMockUsageRestartFacts(t *testing.T, row api.WorkerSessionObservation, logs api.WorkerSessionLogPage) {
	id := row.WorkerSessionId
	name := strings.TrimPrefix(id, "direct-")
	if !row.Direct {
		if row.WorkId == nil || row.FactorySessionId == nil {
			t.Fatal("Factory attribution absent")
		}
		name = strings.TrimPrefix(*row.WorkId, "factory-")
	} else if row.FactorySessionId != nil {
		t.Fatal("direct attempt gained Factory attribution")
	}
	provider, model := "claude", "claude-sonnet-4-6"
	if name == "match" {
		provider, model = "codex", "gpt-5-codex"
	}
	if row.Provider == nil || *row.Provider != provider || row.Model == nil || *row.Model != model || row.AttemptId == "" || row.State != "COMPLETED" {
		t.Fatalf("execution identity changed: %+v", row)
	}
	marker := "MOCK_USAGE_DIRECT_" + name
	if !row.Direct {
		marker = "MOCK_USAGE_FACTORY_" + name
	}
	body, _ := json.Marshal(logs.Events)
	if !strings.Contains(string(body), marker) {
		t.Fatalf("configured entry did not execute: %s", body)
	}
	want := map[string]any{"origin": "SYNTHETIC", "inputTokens": float64(17), "outputTokens": float64(5), "cachedInputTokens": float64(0), "reasoningOutputTokens": float64(0), "totalTokens": float64(22)}
	if name == "zero" {
		want = map[string]any{"origin": "SYNTHETIC", "inputTokens": float64(0), "outputTokens": float64(5), "totalTokens": float64(5)}
	}
	if name == "omitted" {
		want = map[string]any{"origin": "SYNTHETIC", "outputTokens": float64(5), "totalTokens": float64(5)}
	}
	if name == "none" {
		if row.TokenUsage != nil || strings.Contains(string(body), `"kind":"USAGE"`) {
			t.Fatal("undeclared usage invented")
		}
		return
	}
	encoded, _ := json.Marshal(row.TokenUsage)
	var fact map[string]any
	if err := json.Unmarshal(encoded, &fact); err != nil || !reflect.DeepEqual(want, fact) {
		t.Fatalf("summary counters changed: %s %v", encoded, err)
	}
	want["model"] = "gpt-5-codex"
	assertRecordingIntegrityCapturedCounters(t, logs, want)
	for index, event := range logs.Events {
		if event.Event.Position != int64(index+1) || event.Event.SourceEventId == "" || event.Event.CapturedAt == nil {
			t.Fatal("committed order/source/time absent")
		}
		raw, _ := json.Marshal(event.Event.Payload)
		var draft workers.Draft
		if err := json.Unmarshal(raw, &draft); err != nil {
			t.Fatal(err)
		}
		if draft.Kind == workers.KindUsage && (draft.Provenance.Provider != provider || draft.DispatchID != row.AttemptId) {
			t.Fatal("usage attached to wrong execution/attempt")
		}
	}
}

func mockUsageRestartPage(t *testing.T, f invokeArtifactFixture, server string, shown api.WorkerSessionObservation, token string) api.WorkerSessionLogPage {
	id := shown.WorkerSessionId
	query := url.Values{}
	args := []string{"--server", server, "--json", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs"}
	mcpArgs := map[string]any{"action": "READ", "workerSessionId": id, "view": "logs"}
	if token == "" {
		query.Set("limit", "1")
		args = append(args, "--limit", "1")
		mcpArgs["limit"] = 1
	} else {
		query.Set("nextToken", token)
		args = append(args, "--next-token", token)
		mcpArgs["nextToken"] = token
	}
	var cli, remote api.WorkerSessionLogPage
	if err := json.Unmarshal(historyCLI(t, f.ctx, f.binary, f.project, f.env, args...), &cli); err != nil {
		t.Fatal(err)
	}
	readSummaryArtifactHTTP(t, f, server, "/worker-sessions/"+url.PathEscape(id)+"/logs?"+query.Encode(), &remote)
	command := exec.CommandContext(f.ctx, f.binary, "--server", server, "server", "mcp")
	command.Dir, command.Env = f.project, f.env
	client := mcp.NewClient(&mcp.Implementation{Name: "mock-usage-restart", Version: "test"}, nil)
	session, err := client.Connect(f.ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(f.ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: mcpArgs})
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("MCP cursor read: %+v %v", result, err)
	}
	var envelope struct {
		Result struct {
			Session api.WorkerSessionObservation `json:"session"`
			Logs    api.WorkerSessionLogPage     `json:"logs"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &envelope); err != nil {
		t.Fatal(err)
	}
	envelope.Result.Session.ConfirmationState = shown.ConfirmationState
	if !reflect.DeepEqual(cli, remote) || !reflect.DeepEqual(cli, envelope.Result.Logs) || !reflect.DeepEqual(shown, envelope.Result.Session) {
		t.Fatal("CLI/HTTP/MCP committed cursor facts differ")
	}
	return cli
}

func assertMockUsageRestartForeign(t *testing.T, ctx context.Context, server, id string) {
	for _, suffix := range []string{"", "/logs"} {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server+"/worker-sessions/"+url.PathEscape(id)+suffix, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var failure api.ErrorResponse
		err = json.NewDecoder(response.Body).Decode(&failure)
		_ = response.Body.Close()
		code := api.ErrorResponseCodeNOTFOUND
		if suffix != "" {
			code = "WORKER_SESSION_NOT_FOUND"
		}
		if err != nil || response.StatusCode != http.StatusNotFound || failure.Code != code {
			t.Fatalf("foreign profile disclosed capture: %d %+v %v", response.StatusCode, failure, err)
		}
	}
}

func assertMockUsageRestartForeignList(t *testing.T, f invokeArtifactFixture, server string) {
	var listed api.ListWorkerSessionsResponse
	readSummaryArtifactHTTP(t, f, server, "/worker-sessions?history=all", &listed)
	if len(listed.Sessions) != 0 {
		t.Fatal("foreign HTTP profile leaked captures")
	}
	command := exec.CommandContext(f.ctx, f.binary, "--server", server, "server", "mcp")
	command.Dir, command.Env = f.project, f.env
	client := mcp.NewClient(&mcp.Implementation{Name: "mock-usage-foreign-profile", Version: "test"}, nil)
	session, err := client.Connect(f.ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(f.ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{"action": "LIST", "history": "all"}})
	if err != nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("MCP foreign list: %+v %v", result, err)
	}
	var envelope struct {
		Result struct {
			Sessions []api.WorkerSessionObservation `json:"sessions"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(envelope.Result.Sessions) != 0 {
		t.Fatal("foreign MCP profile leaked captures")
	}
}
