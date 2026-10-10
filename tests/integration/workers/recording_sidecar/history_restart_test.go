package recording_sidecar_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// The build owner supplies the deliverable. Both readers are real, sequential
// host processes using one isolated profile; no test process hydrates captures.
func TestWorkerSessionHistoryRestart(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"originating-artifact", "legacy-unavailable", "legacy-current-board"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			runWorkerSessionHistoryRestart(t, name)
		})
	}
}

func TestWorkerSessionShutdownCauseRestart(t *testing.T) {
	t.Parallel()
	binary, project, home, factory, ready, env := incompleteHistoryFixture(t)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	mock, err := json.Marshal(workers.MockWorkersConfig{MockWorkers: []workers.MockWorkerConfig{{
		WorkerName: "worker", WorkstationName: "process", RunType: workers.MockWorkerRunTypeScript,
		ScriptConfig: &workers.MockWorkerScriptConfig{Command: node, Args: []string{filepath.Join(project, "worker.cjs"), ready}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	mockPath := filepath.Join(project, "mock.json")
	if err := os.WriteFile(mockPath, mock, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	first := startHistoryHost(t, ctx, binary, project, env, "--dir", factory, "--continuously", "--with-mock-workers="+mockPath)
	live := awaitShutdownHistoryAttempt(t, ctx, binary, project, env, first.url, ready)
	if live.TerminalCause != nil || live.StartedAt == nil || live.EndedAt != nil {
		t.Fatalf("live attempt facts = %+v", live)
	}
	removeHistorySeeds(t, factory)
	first.stop(t, ctx, binary, project, env)
	for _, name := range []string{".codex", ".cursor", ".claude"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte("not a provider directory"), 0o000); err != nil {
			t.Fatal(err)
		}
	}
	second := startHistoryHost(t, ctx, binary, project, env)
	before := readShutdownHistorySnapshot(t, ctx, binary, project, env, second.url)
	if before.Observation.WorkerSessionId != live.WorkerSessionId || !reflect.DeepEqual(before.Observation.StartedAt, live.StartedAt) {
		t.Fatalf("shutdown changed original identity/start: live=%+v archive=%+v", live, before.Observation)
	}
	second.stop(t, ctx, binary, project, env)
	third := startHistoryHost(t, ctx, binary, project, env)
	after := readShutdownHistorySnapshot(t, ctx, binary, project, env, third.url)
	third.stop(t, ctx, binary, project, env)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("restart changed terminal cause/times/captured records: before=%+v after=%+v", before, after)
	}
}

func readShutdownHistorySnapshot(t *testing.T, ctx context.Context, binary, project string, env []string, server string) historySnapshot {
	t.Helper()
	rows := historyRows(t, ctx, binary, project, env, server, "archived")
	if len(rows) != 1 {
		t.Fatalf("shutdown archive membership = %+v", rows)
	}
	observation := rows[0]
	assertShutdownHistoryTerminalFacts(t, observation)
	selected := historyCLI(t, ctx, binary, project, env, "--server", server, "--json", "worker-sessions", "show", "--worker-session-id", observation.WorkerSessionId)
	var shown api.WorkerSessionObservation
	if err := json.Unmarshal(selected, &shown); err != nil || !reflect.DeepEqual(observation, shown) {
		t.Fatalf("shutdown list/show disagree: %s (%v)", selected, err)
	}
	if active := historyRows(t, ctx, binary, project, env, server, "active"); len(active) != 0 {
		t.Fatalf("shutdown restored execution authority: %+v", active)
	}
	assertHistoryHTTP(t, ctx, server, observation)
	logs := historyLogPage(t, ctx, binary, project, env, server, observation.WorkerSessionId, "")
	if logs.Health != "COMPLETE" || len(logs.Events) == 0 || logs.NextToken != nil {
		t.Fatalf("shutdown captured history = %+v", logs)
	}
	for _, event := range logs.Events {
		if event.Event.CapturedAt == nil {
			t.Fatal("shutdown capture lost committed capturedAt")
		}
	}
	assertHistoryMCP(t, ctx, binary, project, env, server, observation, logs)
	assertHistoryReplay(t, ctx, binary, project, env, server, logs)
	return historySnapshot{Observation: observation, Logs: logs}
}

func assertShutdownHistoryTerminalFacts(t *testing.T, observation api.WorkerSessionObservation) {
	t.Helper()
	if observation.State != "CANCELED" || observation.TerminalCause == nil || *observation.TerminalCause != api.WorkerSessionTerminalCauseOperatorCancel ||
		observation.StartedAt == nil || observation.EndedAt == nil || observation.DurationMillis == nil ||
		observation.RecordingHealth == nil || *observation.RecordingHealth != "COMPLETE" {
		t.Fatalf("shutdown terminal facts = %+v", observation)
	}
}

func awaitShutdownHistoryAttempt(t *testing.T, ctx context.Context, binary, project string, env []string, server, ready string) api.WorkerSessionObservation {
	t.Helper()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(ready); err == nil {
			rows := historyRows(t, ctx, binary, project, env, server, "active")
			if len(rows) == 1 && rows[0].State == "RUNNING" {
				return rows[0]
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("owned mock attempt did not reach physical readiness")
		}
	}
}

func runWorkerSessionHistoryRestart(t *testing.T, mode string) {
	t.Helper()
	binary := os.Getenv("INFINITE_YOU_PREBUILT_ARTIFACT")
	if binary == "" {
		if os.Getenv("INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT") == "1" {
			t.Fatal("INFINITE_YOU_PREBUILT_ARTIFACT is required")
		}
		t.Skip("prebuilt CLI required")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("prebuilt artifact sha256=%x", sha256.Sum256(artifact))
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	home, temp := filepath.Join(project, "home"), filepath.Join(project, "temp")
	for _, path := range []string{home, temp} {
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	env := cleanupEnvironment(home, temp)
	factory := writeCleanupFactory(t, project, node, filepath.Join(project, "ready"), filepath.Join(project, "release"))
	renameHistorySeed(t, factory)
	if err := os.WriteFile(filepath.Join(project, "worker.cjs"), []byte("console.log('history-restart COMPLETE');"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	runArgs := []string{"run", "--dir", factory, "--quiet"}
	var readerArgs []string
	if mode == "legacy-current-board" {
		board := filepath.Join(project, "board.json")
		scope := uuid.NewString()
		// Select the same scoped path as the configured-board lookup policy.
		// Readers own a different scope so they inspect the closed execution
		// without restoring its runtime observations as live handles.
		runArgs = append(runArgs, "--session", scope, "--record", filepath.Join(project, "board."+scope+".json"))
		readerArgs = []string{"--dir", factory, "--record", board, "--continuously"}
	}
	historyCLI(t, ctx, binary, project, env, runArgs...)
	// Startup watches the Current Factory inputs. Consume this test-owned seed
	// explicitly so restarting the host cannot submit another attempt.
	removeHistorySeeds(t, factory)
	// No native provider files exist: script output is captured by production
	// wiring. Replace their conventional directories with unreadable file paths
	// so a future provider-file fallback cannot supply this replay.
	for _, name := range []string{".codex", ".cursor", ".claude"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte("not a provider directory"), 0o000); err != nil {
			t.Fatal(err)
		}
	}
	if mode != "originating-artifact" {
		downgradeHistoryCapture(t, project)
	}
	first := startHistoryHost(t, ctx, binary, project, env, readerArgs...)
	before := readNamedHistorySnapshot(t, ctx, binary, project, env, first.url, mode == "legacy-unavailable")
	first.stop(t, ctx, binary, project, env)
	second := startHistoryHost(t, ctx, binary, project, env, readerArgs...)
	after := readNamedHistorySnapshot(t, ctx, binary, project, env, second.url, mode == "legacy-unavailable")
	second.stop(t, ctx, binary, project, env)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("restart changed captured identity/ordered replay: before=%+v after=%+v", before, after)
	}
}

func renameHistorySeed(t *testing.T, factory string) {
	t.Helper()
	seeds, err := filepath.Glob(filepath.Join(factory, "inputs", "task", "*", "seed-*.json"))
	if err != nil || len(seeds) != 1 {
		t.Fatalf("known-name seed = %v, %v", seeds, err)
	}
	if err := os.Rename(seeds[0], filepath.Join(filepath.Dir(seeds[0]), "seed-archived-name.json")); err != nil {
		t.Fatal(err)
	}
}

func removeHistorySeeds(t *testing.T, factory string) {
	t.Helper()
	seeds, err := filepath.Glob(filepath.Join(factory, "inputs", "task", "*", "seed-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, seed := range seeds {
		if err := os.Remove(seed); err != nil {
			t.Fatal(err)
		}
	}
}

type historySnapshot struct {
	Observation api.WorkerSessionObservation
	Logs        api.WorkerSessionLogPage
}

func readNamedHistorySnapshot(t *testing.T, ctx context.Context, binary, project string, env []string, server string, legacy bool) historySnapshot {
	t.Helper()
	snapshot := readHistorySnapshot(t, ctx, binary, project, env, server)
	observation := snapshot.Observation
	if observation.WorkId == nil || *observation.WorkId == "" {
		t.Fatalf("archived Work identity = %+v", observation)
	}
	if legacy && observation.WorkName != nil {
		t.Fatalf("legacy capture without an available scoped artifact borrowed attribution: %+v", observation)
	}
	// Script execution has no inference-provider fact in its capture. This
	// remains independent of optional Work-name availability; named provider
	// captures retain their facts in the public attribution journey.
	if observation.Provider != nil {
		t.Fatalf("script capture invented an inference provider: %+v", observation)
	}
	if !legacy && (observation.WorkName == nil || *observation.WorkName != "seed-archived-name") {
		t.Fatalf("archived known name = %+v", observation)
	}
	table := historyCLI(t, ctx, binary, project, env, "--server", server, "worker-sessions", "list", "--history", "archived")
	if !legacy && !bytes.Contains(table, []byte("seed-archived-name")) {
		t.Fatalf("archived table lost Work name: %s", table)
	}
	if legacy {
		lines := strings.Split(strings.TrimSpace(string(table)), "\n")
		if len(lines) != 2 || len(strings.Fields(lines[1])) < 4 || strings.Fields(lines[1])[0] != "-" {
			t.Fatalf("legacy unavailable Work marker: %s", table)
		}
	}
	return snapshot
}

// Downgrade only the test-owned persisted capture to its pre-provenance/name shape.
// All real admissions, canonical history, identities and captured logs survive.
// Readers exercise both a validated configured-board candidate and the
// authorized unavailable result when no candidate is configured.
func downgradeHistoryCapture(t *testing.T, home string) {
	t.Helper()
	removed := 0
	err := filepath.WalkDir(home, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		if !strings.HasSuffix(path, ".worker.jsonl") && filepath.Base(filepath.Dir(path)) != "catalog" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := bytes.Split(data, []byte("\n"))
		for i, line := range lines {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			var record map[string]json.RawMessage
			if err := json.Unmarshal(line, &record); err != nil {
				return err
			}
			if _, ok := record["originatingArtifact"]; !ok {
				continue
			}
			delete(record, "originatingArtifact")
			delete(record, "workName")
			removed++
			lines[i], err = json.Marshal(record)
			if err != nil {
				return err
			}
		}
		return os.WriteFile(path, bytes.Join(lines, []byte("\n")), 0o600)
	})
	if err != nil || removed < 2 {
		t.Fatalf("legacy capture downgrade: removed=%d error=%v", removed, err)
	}
}

func readHistorySnapshot(t *testing.T, ctx context.Context, binary, project string, env []string, server string) historySnapshot {
	t.Helper()
	list := historyCLI(t, ctx, binary, project, env, "--server", server, "--json", "worker-sessions", "list", "--history", "archived")
	var rows api.ListWorkerSessionsResponse
	if err := json.Unmarshal(list, &rows); err != nil {
		t.Fatalf("archived list: %v: %s", err, list)
	}
	if len(rows.Sessions) != 1 {
		t.Fatalf("archived sessions = %+v", rows.Sessions)
	}
	observation := rows.Sessions[0]
	selected := historyCLI(t, ctx, binary, project, env, "--server", server, "--json", "worker-sessions", "show", "--worker-session-id", observation.WorkerSessionId)
	var selectedObservation api.WorkerSessionObservation
	if err := json.Unmarshal(selected, &selectedObservation); err != nil {
		t.Fatalf("selected archived observation = %s, %v", selected, err)
	}
	if !reflect.DeepEqual(observation, selectedObservation) {
		t.Fatalf("list/show disagree: list=%+v show=%+v", observation, selectedObservation)
	}
	assertHistoryHTTP(t, ctx, server, observation)

	if observation.State != "COMPLETED" || observation.RecordingHealth == nil || *observation.RecordingHealth != "COMPLETE" ||
		observation.TerminalCause == nil || *observation.TerminalCause != api.WorkerSessionTerminalCauseCompleted {
		t.Fatalf("ended history = %+v", observation)
	}
	active := historyCLI(t, ctx, binary, project, env, "--server", server, "--json", "worker-sessions", "list", "--history", "active")
	if err := json.Unmarshal(active, &rows); err != nil || len(rows.Sessions) != 0 {
		t.Fatalf("dead execution became active: %s (%v)", active, err)
	}
	page := readHistoryLogs(t, ctx, binary, project, env, server, observation.WorkerSessionId)
	assertHistoryMCP(t, ctx, binary, project, env, server, observation, page)
	assertHistoryReplay(t, ctx, binary, project, env, server, page)
	return historySnapshot{Observation: observation, Logs: page}
}

func assertHistoryHTTP(t *testing.T, ctx context.Context, server string, observation api.WorkerSessionObservation) {
	t.Helper()
	for _, path := range []string{"/worker-sessions?history=archived", "/worker-sessions/" + observation.WorkerSessionId} {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, server+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var selected api.WorkerSessionObservation
		if path == "/worker-sessions?history=archived" {
			var page api.ListWorkerSessionsResponse
			err = json.NewDecoder(response.Body).Decode(&page)
			if len(page.Sessions) != 1 {
				t.Errorf("HTTP archived membership: %+v", page)
			} else {
				selected = page.Sessions[0]
			}
		} else {
			err = json.NewDecoder(response.Body).Decode(&selected)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK || err != nil || !reflect.DeepEqual(observation, selected) {
			t.Fatalf("HTTP %s disagrees with CLI: status=%d error=%v observation=%+v", path, response.StatusCode, err, selected)
		}
	}
}

func assertHistoryMCP(t *testing.T, ctx context.Context, binary, project string, env []string, server string, observation api.WorkerSessionObservation, logs api.WorkerSessionLogPage) {
	t.Helper()
	assertHistoryMCPViews(t, ctx, binary, project, env, server, observation, logs, []string{"LIST", "READ", "logs", "events"})
}

func assertHistoryMCPViews(t *testing.T, ctx context.Context, binary, project string, env []string, server string, observation api.WorkerSessionObservation, logs api.WorkerSessionLogPage, views []string, transcripts ...api.WorkerSessionTranscriptResponse) {
	t.Helper()
	command := exec.CommandContext(ctx, binary, "--server", server, "server", "mcp")
	command.Dir, command.Env = project, env
	var diagnostics bytes.Buffer
	command.Stderr = &diagnostics
	client := mcp.NewClient(&mcp.Implementation{Name: "archived-name-restart", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	if err != nil {
		t.Fatalf("MCP connect: %v", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Errorf("MCP close: %v", err)
		}
	}()
	for _, action := range views {
		args := map[string]any{"action": action}
		if action == "LIST" {
			args["history"] = "archived"
		} else {
			args["workerSessionId"] = observation.WorkerSessionId
		}
		if action == "logs" || action == "events" || action == "transcript" {
			args["action"], args["view"], args["limit"] = "READ", action, 1000
			if action == "transcript" {
				delete(args, "limit")
			}
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: args})
		if err != nil || result.IsError || len(result.Content) != 1 {
			t.Fatalf("MCP %s: result=%+v error=%v", action, result, err)
		}
		content, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("MCP %s content: %T", action, result.Content[0])
		}
		if action == "transcript" {
			assertHistoryMCPTranscript(t, content.Text, transcripts)
		} else {
			assertHistoryMCPResult(t, action, content.Text, observation, logs)
		}
	}
}

func assertHistoryMCPTranscript(t *testing.T, payload string, transcripts []api.WorkerSessionTranscriptResponse) {
	t.Helper()
	var envelope struct {
		Result struct {
			Transcript api.WorkerSessionTranscriptResponse `json:"transcript"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		t.Fatal(err)
	}
	if len(transcripts) != 1 || !reflect.DeepEqual(transcripts[0], envelope.Result.Transcript) {
		t.Fatalf("MCP transcript changed captured text/order/identity: %s", payload)
	}
}

func assertHistoryMCPResult(t *testing.T, action, payload string, observation api.WorkerSessionObservation, logs api.WorkerSessionLogPage) {
	t.Helper()
	var envelope struct {
		Result struct {
			Sessions []api.WorkerSessionObservation `json:"sessions"`
			Session  api.WorkerSessionObservation   `json:"session"`
			Logs     api.WorkerSessionLogPage       `json:"logs"`
			Events   struct {
				Events    []api.WorkerSessionEvent `json:"events"`
				Truncated bool                     `json:"truncated"`
			} `json:"events"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		t.Fatal(err)
	}
	if action == "events" {
		if envelope.Result.Events.Truncated {
			t.Fatal("MCP replay unexpectedly truncated")
		}
		assertHistoryReplayFrames(t, envelope.Result.Events.Events, logs)
		return
	}
	if action == "logs" {
		if !reflect.DeepEqual(logs, envelope.Result.Logs) {
			t.Fatalf("MCP logs changed ordered captured replay: %s", payload)
		}
		return
	}
	selected := envelope.Result.Session
	if action == "LIST" {
		if len(envelope.Result.Sessions) != 1 {
			t.Fatalf("MCP archived membership: %s", payload)
		}
		selected = envelope.Result.Sessions[0]
	}
	if !reflect.DeepEqual(observation, selected) {
		t.Fatalf("MCP %s disagrees with CLI: %+v", action, selected)
	}
}

func readHistoryLogs(t *testing.T, ctx context.Context, binary, project string, env []string, server, id string) api.WorkerSessionLogPage {
	t.Helper()
	args := []string{"--server", server, "--json", "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--limit", "2"}
	base := len(args)
	var result api.WorkerSessionLogPage
	for count := 0; count < 20; count++ {
		logs := historyCLI(t, ctx, binary, project, env, args...)
		var page api.WorkerSessionLogPage
		if err := json.Unmarshal(logs, &page); err != nil {
			t.Fatalf("logs: %v: %s", err, logs)
		}
		if page.Health != "COMPLETE" || page.CommittedPosition == 0 || len(page.Events) == 0 {
			t.Fatalf("missing committed page: %s", logs)
		}
		if count == 0 {
			result = page
		} else {
			if page.CommittedPosition != result.CommittedPosition || page.RecordingGenerationId != result.RecordingGenerationId {
				t.Fatalf("replay watermark changed: %+v", page)
			}
			result.Events = append(result.Events, page.Events...)
		}
		result.NextToken = page.NextToken
		if page.NextToken == nil {
			encoded, err := json.Marshal(result.Events)
			if err != nil || !bytes.Contains(encoded, []byte("history-restart")) {
				t.Fatalf("missing captured output: %s (%v)", encoded, err)
			}
			if count == 0 {
				t.Fatal("fixture did not exercise cursor paging")
			}
			return result
		}
		args = append(args[:base], "--next-token", *page.NextToken)
	}
	t.Fatal("captured paging did not reach stable terminal")
	return result
}

func historyCLI(t *testing.T, ctx context.Context, binary, project string, env []string, args ...string) []byte {
	t.Helper()
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir, command.Env = project, env
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("CLI %v: %v\nstdout=%s\nstderr=%s", args, err, &stdout, &stderr)
	}
	return stdout.Bytes()
}

type historyHost struct {
	url     string
	command *exec.Cmd
	done    chan error
	output  *historyOutput
	stopped bool
}

func startHistoryHost(t *testing.T, ctx context.Context, binary, project string, env []string, runArgs ...string) *historyHost {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	output := &historyOutput{changed: make(chan struct{}, 1)}
	args := []string{"server", "--listen", address}
	if len(runArgs) != 0 {
		args = append([]string{"run", "--with-server", "--listen", address}, runArgs...)
	}
	command := exec.CommandContext(ctx, binary, args...)
	command.Dir, command.Env, command.Stdout, command.Stderr = project, env, output, output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	host := &historyHost{url: "http://" + address, command: command, done: make(chan error, 1), output: output}
	go func() { host.done <- command.Wait() }()
	t.Cleanup(func() {
		if !host.stopped {
			_ = command.Process.Kill()
			<-host.done
		}
	})
	for {
		if bytes.Contains(output.snapshot(), []byte("Dashboard URL: "+host.url)) {
			return host
		}
		select {
		case <-output.changed:
		case err := <-host.done:
			host.stopped = true
			t.Fatalf("host exited before readiness: %v: %s", err, output.snapshot())
		case <-ctx.Done():
			t.Fatalf("host readiness: %v: %s", ctx.Err(), output.snapshot())
		}
	}
}

func (host *historyHost) stop(t *testing.T, ctx context.Context, binary, project string, env []string) {
	t.Helper()
	historyCLI(t, ctx, binary, project, env, "--server", host.url, "server", "stop")
	select {
	case err := <-host.done:
		host.stopped = true
		if err != nil {
			t.Fatalf("host shutdown: %v: %s", err, host.output.snapshot())
		}
	case <-ctx.Done():
		t.Fatal(fmt.Errorf("host shutdown: %w", ctx.Err()))
	}
}

type historyOutput struct {
	mu      sync.Mutex
	buffer  bytes.Buffer
	changed chan struct{}
}

func (output *historyOutput) Write(data []byte) (int, error) {
	output.mu.Lock()
	n, err := output.buffer.Write(data)
	output.mu.Unlock()
	select {
	case output.changed <- struct{}{}:
	default:
	}
	return n, err
}

func (output *historyOutput) snapshot() []byte {
	output.mu.Lock()
	defer output.mu.Unlock()
	return bytes.Clone(output.buffer.Bytes())
}
