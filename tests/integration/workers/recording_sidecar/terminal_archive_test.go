package recording_sidecar_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type terminalArchiveCell struct {
	name, provider, state string
	direct                bool
}

type terminalArchiveSnapshot struct {
	Observation api.WorkerSessionObservation
	Logs        api.WorkerSessionLogPage
	Page        api.WorkerSessionLogPage
	Transcript  api.WorkerSessionTranscriptResponse
}

// I-S consumes a build-owned artifact, with declarative mocks at the production
// worker boundary. Host lifetimes serialize to preserve the sole profile writer.
func TestPrebuiltTerminalArchiveObservations(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	project := t.TempDir()
	home, temp := filepath.Join(project, "home"), filepath.Join(project, "temp")
	for _, path := range []string{home, temp} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	f := invokeArtifactFixture{ctx: ctx, binary: invokeArtifactBinary(t), project: project, home: home, env: append(cleanupEnvironment(home, temp), "PATH="+temp)}
	cells := terminalArchiveCells()
	factory, mock := writeTerminalArchiveFactory(t, f, cells)
	older, latest := filepath.Join(project, "older.json"), filepath.Join(project, "latest.json")
	empty := startHistoryHost(t, ctx, f.binary, project, f.env, "--dir", factory, "--record", older, "--continuously", "--with-mock-workers", mock)
	empty.stop(t, ctx, f.binary, project, f.env)
	host := startHistoryHost(t, ctx, f.binary, project, f.env, "--dir", factory, "--resume", older, "--record", latest, "--continuously", "--with-mock-workers", mock)
	admitTerminalArchiveCells(t, f, host.url, cells)
	awaitTerminalArchiveGates(t, f, cells)
	// Pause business dispatch before ending physical attempts: no retry can
	// produce a successor and obscure the exact capture under test.
	historyCLI(t, ctx, f.binary, project, f.env, "--server", host.url, "--json", "session", "pause")
	endTerminalArchiveCells(t, f, host.url, cells)
	rows := awaitTerminalArchiveRows(t, f, host.url, len(cells))
	before := readTerminalArchiveSnapshots(t, f, host.url, rows)
	host.stop(t, ctx, f.binary, project, f.env)
	for _, source := range []string{latest, older} {
		reopened := startHistoryHost(t, ctx, f.binary, project, f.env, "--dir", factory, "--resume", source, "--record", source+".reopened", "--continuously", "--with-mock-workers", mock)
		historyCLI(t, ctx, f.binary, project, f.env, "--server", reopened.url, "--json", "session", "pause")
		afterRows := awaitTerminalArchiveRows(t, f, reopened.url, len(cells))
		// A live board may dispatch requeued canceled Work during activation.
		// Compare exact original attempts, not a global row-count invariant.
		var originals []api.WorkerSessionObservation
		for _, row := range afterRows {
			if _, ok := before[row.WorkerSessionId]; ok {
				originals = append(originals, row)
			}
		}
		after := readTerminalArchiveSnapshots(t, f, reopened.url, originals)
		for id, want := range before {
			if !reflect.DeepEqual(want, after[id]) {
				t.Fatalf("%s changed committed facts/content/cursor for %s", filepath.Base(source), id)
			}
		}
		reopened.stop(t, ctx, f.binary, project, f.env)
	}
	t.Log("16 actual declarative Codex/Claude Direct/Factory terminal cells; exact committed CLI/HTTP/MCP reads and original Factory/Work names survive joined latest/older-board recovery")
}

func terminalArchiveCells() []terminalArchiveCell {
	var cells []terminalArchiveCell
	for _, direct := range []bool{false, true} {
		for _, provider := range []string{"codex", "claude"} {
			for _, state := range []string{"COMPLETED", "FAILED", "CANCELED", "TERMINATED"} {
				cells = append(cells, terminalArchiveCell{name: fmt.Sprintf("archive-%t-%s-%s", direct, provider, state), provider: provider, state: state, direct: direct})
			}
		}
	}
	return cells
}

func writeTerminalArchiveFactory(t *testing.T, f invokeArtifactFixture, cells []terminalArchiveCell) (string, string) {
	t.Helper()
	factory := filepath.Join(f.project, "factory")
	if err := os.Mkdir(factory, 0o700); err != nil {
		t.Fatal(err)
	}
	var types, agents, stations, mocks []map[string]any
	for _, cell := range cells {
		mock := map[string]any{"workstationName": cell.name, "runType": "accept", "resultBody": map[string]string{"output": cell.name + " COMPLETE"},
			"gateConfig": map[string]string{"arrivedFile": filepath.Join(f.project, cell.name+".arrived"), "releaseFile": filepath.Join(f.project, cell.name+".release"), "timeout": "120s"},
			"usage":      map[string]any{"provider": cell.provider, "model": "archive-model", "inputTokens": 17, "outputTokens": 5}}
		if cell.state == "FAILED" {
			mock["runType"] = "reject"
			delete(mock, "resultBody")
			mock["rejectConfig"] = map[string]any{"stderr": cell.name + " failure", "exitCode": 1}
		}
		mocks = append(mocks, mock)
		if cell.direct {
			continue
		}
		types = append(types, map[string]any{"name": cell.name, "states": []map[string]string{{"name": "init", "type": "INITIAL"}, {"name": "done", "type": "TERMINAL"}, {"name": "failed", "type": "FAILED"}}})
		agents = append(agents, map[string]any{"name": cell.name, "type": "AGENT_WORKER", "executorProvider": cell.provider, "modelProvider": cell.provider, "model": "archive-model"})
		stations = append(stations, map[string]any{"name": cell.name, "type": "AGENT_RUN", "worker": cell.name, "inputs": []map[string]string{{"workType": cell.name, "state": "init"}}, "outputs": []map[string]string{{"workType": cell.name, "state": "done"}}, "onFailure": []map[string]string{{"workType": cell.name, "state": "failed"}}})
	}
	writeRecordingIntegrityJSON(t, filepath.Join(factory, "factory.json"), map[string]any{"name": "terminal-archive", "workTypes": types, "workers": agents, "workstations": stations})
	config := filepath.Join(f.project, "mock.json")
	writeRecordingIntegrityJSON(t, config, map[string]any{"mockWorkers": mocks})
	return factory, config
}

func admitTerminalArchiveCells(t *testing.T, f invokeArtifactFixture, server string, cells []terminalArchiveCell) {
	t.Helper()
	for _, cell := range cells {
		if !cell.direct {
			payload, _ := json.Marshal(map[string]any{"name": cell.name, "workTypeName": cell.name, "payload": map[string]string{"title": cell.name}})
			request, err := http.NewRequestWithContext(f.ctx, http.MethodPost, server+"/factory-sessions/~default/work", bytes.NewReader(payload))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Content-Type", "application/json")
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			var result api.SubmitWorkResponse
			err = json.NewDecoder(response.Body).Decode(&result)
			_ = response.Body.Close()
			if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusCreated || err != nil || !result.Accepted {
				t.Fatalf("admit %s: status=%d result=%+v error=%v", cell.name, response.StatusCode, result, err)
			}
			continue
		}
		document := filepath.Join(f.project, cell.name+".json")
		writeRecordingIntegrityJSON(t, document, map[string]any{"workerSessionId": cell.name, "requestId": cell.name + "-request", "execution": map[string]any{
			"workstationName": cell.name, "workingDirectory": f.project, "runnerId": cell.provider, "executorProvider": cell.provider, "modelProvider": cell.provider, "model": "archive-model", "userMessage": cell.name,
			"dispatch": map[string]string{"dispatchId": cell.name + "-attempt", "workstationName": cell.name}}})
		historyCLI(t, f.ctx, f.binary, f.project, f.env, "--remote", "--server", server, "--json", "worker-sessions", "invoke", "--execution", document, "--async", "--retry-max-attempts", "1")
	}
}

func awaitTerminalArchiveGates(t *testing.T, f invokeArtifactFixture, cells []terminalArchiveCell) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for _, cell := range cells {
		for {
			if _, err := os.Stat(filepath.Join(f.project, cell.name+".arrived")); err == nil {
				break
			}
			select {
			case <-ticker.C:
			case <-f.ctx.Done():
				t.Fatalf("mock %s failed to arrive: %v", cell.name, f.ctx.Err())
			}
		}
	}
}

func endTerminalArchiveCells(t *testing.T, f invokeArtifactFixture, server string, cells []terminalArchiveCell) {
	t.Helper()
	rows := historyRows(t, f.ctx, f.binary, f.project, f.env, server, "active")
	if len(rows) != len(cells) {
		t.Fatalf("expected exactly %d gated attempts: %+v", len(cells), rows)
	}
	for _, cell := range cells {
		id := ""
		for _, row := range rows {
			if terminalArchiveName(row) == cell.name {
				id = row.WorkerSessionId
			}
		}
		if id == "" {
			t.Fatalf("missing physical attempt %s", cell.name)
		}
		switch cell.state {
		case "CANCELED", "TERMINATED":
			action := "cancel"
			if cell.state == "TERMINATED" {
				action = "terminate"
			}
			historyCLI(t, f.ctx, f.binary, f.project, f.env, "--server", server, "--json", "worker-sessions", action, id)
		default:
			if err := os.WriteFile(filepath.Join(f.project, cell.name+".release"), []byte("release"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
}

func awaitTerminalArchiveRows(t *testing.T, f invokeArtifactFixture, server string, count int) []api.WorkerSessionObservation {
	t.Helper()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		rows := historyRows(t, f.ctx, f.binary, f.project, f.env, server, "archived")
		ready := len(rows) >= count
		for _, row := range rows {
			ready = ready && row.RecordingHealth != nil && *row.RecordingHealth == "COMPLETE"
		}
		if ready {
			return rows
		}
		select {
		case <-ticker.C:
		case <-f.ctx.Done():
			t.Fatalf("committed archive readiness: %v rows=%+v", f.ctx.Err(), rows)
		}
	}
}

func readTerminalArchiveSnapshots(t *testing.T, f invokeArtifactFixture, server string, rows []api.WorkerSessionObservation) map[string]terminalArchiveSnapshot {
	t.Helper()

	result := make(map[string]terminalArchiveSnapshot)
	for _, row := range rows {
		snapshot := readTerminalArchiveSnapshot(t, f, server, row)
		result[row.WorkerSessionId] = snapshot
	}
	assertTerminalArchiveLists(t, f, server, rows, result)
	return result
}

func assertTerminalArchiveScope(t *testing.T, f invokeArtifactFixture, server string, row api.WorkerSessionObservation) {
	t.Helper()
	if row.FactorySessionId == nil || row.WorkId == nil || row.WorkName == nil || *row.WorkName != terminalArchiveName(row) {
		t.Fatalf("original scope/name absent: %+v", row)
	}
	base := "/factory-sessions/" + *row.FactorySessionId
	var page api.ListWorkerSessionsResponse
	readSummaryArtifactHTTP(t, f, server, base+"/worker-sessions?workId="+*row.WorkId, &page)
	assertTerminalArchiveMembership(t, row, page.Sessions)
	var shown api.WorkerSessionObservation
	readSummaryArtifactHTTP(t, f, server, base+"/worker-sessions/"+row.WorkerSessionId, &shown)
	if !reflect.DeepEqual(row, shown) {
		t.Fatalf("own-scope READ differs: %+v", shown)
	}
	body := historyCLI(t, f.ctx, f.binary, f.project, f.env, "--server", server, "--json", "worker-sessions", "list", "--session", *row.FactorySessionId, "--work-id", *row.WorkId)
	if err := json.Unmarshal(body, &page); err != nil {
		t.Fatalf("own-scope CLI LIST differs: %s (%v)", body, err)
	}
	assertTerminalArchiveMembership(t, row, page.Sessions)
	// Completed declared-usage mocks have a captured provider association.
	if row.State == "COMPLETED" {
		var transcript, scoped api.WorkerSessionTranscriptResponse
		readSummaryArtifactHTTP(t, f, server, "/worker-sessions/"+row.WorkerSessionId+"/transcript", &transcript)
		readSummaryArtifactHTTP(t, f, server, base+"/worker-sessions/"+row.WorkerSessionId+"/transcript", &scoped)
		if !reflect.DeepEqual(transcript, scoped) {
			t.Fatal("original scope changed transcript")
		}

	}
}

func terminalArchiveName(row api.WorkerSessionObservation) string {
	if row.Direct {
		return row.WorkerSessionId
	}
	if row.Labels != nil {
		for _, label := range *row.Labels {
			if name, ok := strings.CutPrefix(label, "workstation:"); ok {
				return name
			}
		}
	}
	return ""
}

func assertTerminalArchiveLists(t *testing.T, f invokeArtifactFixture, server string, rows []api.WorkerSessionObservation, snapshots map[string]terminalArchiveSnapshot) {
	t.Helper()
	var page api.ListWorkerSessionsResponse
	readSummaryArtifactHTTP(t, f, server, "/worker-sessions?history=archived", &page)
	assertTerminalArchiveOriginals(t, rows, page.Sessions)
	command := exec.CommandContext(f.ctx, f.binary, "--server", server, "server", "mcp")
	command.Dir, command.Env = f.project, f.env
	var diagnostics bytes.Buffer
	command.Stderr = &diagnostics
	client := mcp.NewClient(&mcp.Implementation{Name: "terminal-archive", Version: "test"}, nil)
	session, err := client.Connect(f.ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Error(err)
		}
	}()
	response, err := session.CallTool(f.ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{"action": "LIST", "history": "archived"}})
	if err != nil || response.IsError || len(response.Content) != 1 {
		t.Fatalf("MCP LIST: %+v error=%v stderr=%s", response, err, &diagnostics)
	}
	content, ok := response.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected MCP content %T", response.Content[0])
	}
	var envelope struct {
		Result struct {
			Sessions []api.WorkerSessionObservation `json:"sessions"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(content.Text), &envelope); err != nil {
		t.Fatal(err)
	}
	assertTerminalArchiveOriginals(t, rows, envelope.Result.Sessions)
	for _, row := range rows {
		assertTerminalArchiveMCPReads(t, f, session, row, snapshots[row.WorkerSessionId])
	}

}

func assertTerminalArchiveOriginals(t *testing.T, want, got []api.WorkerSessionObservation) {
	t.Helper()
	for _, row := range want {
		found := false
		for _, actual := range got {
			if actual.WorkerSessionId == row.WorkerSessionId {
				found = reflect.DeepEqual(row, actual)
			}
		}
		if !found {
			t.Fatalf("LIST lost/changed original attempt %s", row.WorkerSessionId)
		}
	}
}

func assertTerminalArchiveMembership(t *testing.T, row api.WorkerSessionObservation, rows []api.WorkerSessionObservation) {
	t.Helper()
	assertTerminalArchiveOriginals(t, []api.WorkerSessionObservation{row}, rows)
	for _, actual := range rows {
		if !reflect.DeepEqual(actual.FactorySessionId, row.FactorySessionId) || !reflect.DeepEqual(actual.WorkId, row.WorkId) || !reflect.DeepEqual(actual.WorkName, row.WorkName) {
			t.Fatalf("foreign Work scope/name in LIST: %+v", actual)
		}
	}
}

func readTerminalArchiveSnapshot(t *testing.T, f invokeArtifactFixture, server string, row api.WorkerSessionObservation) terminalArchiveSnapshot {
	t.Helper()
	var snapshot terminalArchiveSnapshot
	body := historyCLI(t, f.ctx, f.binary, f.project, f.env, "--server", server, "--json", "worker-sessions", "show", "--worker-session-id", row.WorkerSessionId)
	if err := json.Unmarshal(body, &snapshot.Observation); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(row, snapshot.Observation) {
		t.Fatalf("LIST/READ differ: row=%+v show=%+v", row, snapshot.Observation)
	}
	state := terminalArchiveName(row)[strings.LastIndex(terminalArchiveName(row), "-")+1:]
	if string(row.State) != state || row.ConfirmationState != "CONFIRMED" || row.EndedAt == nil || row.DurationMillis == nil {
		t.Fatalf("untruthful committed %s: %+v", state, row)
	}
	snapshot.Logs = historyLogPage(t, f.ctx, f.binary, f.project, f.env, server, row.WorkerSessionId, "")
	readSummaryArtifactHTTP(t, f, server, "/worker-sessions/"+row.WorkerSessionId, &snapshot.Observation)
	if !reflect.DeepEqual(row, snapshot.Observation) {
		t.Fatal("HTTP READ differs from CLI LIST")
	}
	readSummaryArtifactHTTP(t, f, server, "/worker-sessions/"+row.WorkerSessionId+"/logs?limit=1", &snapshot.Page)
	if snapshot.Page.NextToken == nil {
		t.Fatal("committed prefix omitted cursor")
	}
	continued := historyLogPage(t, f.ctx, f.binary, f.project, f.env, server, row.WorkerSessionId, *snapshot.Page.NextToken)
	if !reflect.DeepEqual(continued.Events, snapshot.Logs.Events[1:]) || continued.RecordingGenerationId != snapshot.Logs.RecordingGenerationId {
		t.Fatal("acknowledged cursor changed committed content/generation")
	}

	if row.State == "COMPLETED" {
		snapshot.Transcript = readTerminalArchiveTranscript(t, f, server, row)
	}
	if !row.Direct {
		assertTerminalArchiveScope(t, f, server, row)
	}
	return snapshot
}

func readTerminalArchiveTranscript(t *testing.T, f invokeArtifactFixture, server string, row api.WorkerSessionObservation) api.WorkerSessionTranscriptResponse {
	t.Helper()
	var captured api.WorkerSessionTranscriptResponse

	usage := row.TokenUsage
	if usage == nil || usage.Origin == nil || *usage.Origin != api.ProviderSessionTokenUsageOriginSYNTHETIC || usage.InputTokens == nil || *usage.InputTokens != 17 || usage.OutputTokens == nil || *usage.OutputTokens != 5 {
		t.Fatalf("completed mock lost declared synthetic usage: %+v", usage)
	}
	body := historyCLI(t, f.ctx, f.binary, f.project, f.env, "--server", server, "--json", "worker-sessions", "read", "--worker-session-id", row.WorkerSessionId, "--view", "transcript")
	if err := json.Unmarshal(body, &captured); err != nil {
		t.Fatal(err)
	}
	var transcript api.WorkerSessionTranscriptResponse
	readSummaryArtifactHTTP(t, f, server, "/worker-sessions/"+row.WorkerSessionId+"/transcript", &transcript)
	if !reflect.DeepEqual(transcript, captured) {
		t.Fatal("CLI/HTTP transcript differs")
	}

	return captured
}

func assertTerminalArchiveMCPReads(t *testing.T, f invokeArtifactFixture, session *mcp.ClientSession, row api.WorkerSessionObservation, snapshot terminalArchiveSnapshot) {
	t.Helper()
	for _, view := range []string{"summary", "logs", "transcript"} {
		if view == "transcript" && row.State != "COMPLETED" {
			continue
		}
		args := map[string]any{"action": "READ", "workerSessionId": row.WorkerSessionId, "view": view}
		result, err := session.CallTool(f.ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: args})
		if err != nil || result.IsError || len(result.Content) != 1 {
			t.Fatalf("MCP %s: %+v error=%v", view, result, err)
		}
		content, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("unexpected MCP content %T", result.Content[0])
		}
		switch view {
		case "transcript":
			assertHistoryMCPTranscript(t, content.Text, []api.WorkerSessionTranscriptResponse{snapshot.Transcript})
		case "logs":
			assertHistoryMCPResult(t, "logs", content.Text, row, snapshot.Logs)
		default:
			assertHistoryMCPResult(t, "READ", content.Text, row, snapshot.Logs)
		}
	}
}
