package recording_sidecar_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	api "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// The build owner supplies the deliverable. Both readers are real, sequential
// host processes using one isolated profile; no test process hydrates captures.
func TestWorkerSessionHistoryRestart(t *testing.T) {
	t.Parallel()
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
	seedsBefore, err := filepath.Glob(filepath.Join(factory, "inputs", "task", "*", "seed-*.json"))
	if err != nil || len(seedsBefore) != 1 {
		t.Fatalf("known-name seed = %v, %v", seedsBefore, err)
	}
	if err := os.Rename(seedsBefore[0], filepath.Join(filepath.Dir(seedsBefore[0]), "seed-archived-name.json")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "worker.cjs"), []byte("console.log('history-restart COMPLETE');"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	historyCLI(t, ctx, binary, project, env, "run", "--dir", factory, "--quiet")
	// Startup watches the Current Factory inputs. Consume this test-owned seed
	// explicitly so restarting the host cannot submit another attempt.
	seeds, err := filepath.Glob(filepath.Join(factory, "inputs", "task", "*", "seed-*.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, seed := range seeds {
		if err := os.Remove(seed); err != nil {
			t.Fatal(err)
		}
	}
	// No native provider files exist: script output is captured by production
	// wiring. Replace their conventional directories with unreadable file paths
	// so a future provider-file fallback cannot supply this replay.
	for _, name := range []string{".codex", ".cursor", ".claude"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte("not a provider directory"), 0o000); err != nil {
			t.Fatal(err)
		}
	}
	first := startHistoryHost(t, ctx, binary, project, env)
	before := readNamedHistorySnapshot(t, ctx, binary, project, env, first.url)
	first.stop(t, ctx, binary, project, env)
	second := startHistoryHost(t, ctx, binary, project, env)
	after := readNamedHistorySnapshot(t, ctx, binary, project, env, second.url)
	second.stop(t, ctx, binary, project, env)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("restart changed captured identity/ordered replay: before=%+v after=%+v", before, after)
	}
}

type historySnapshot struct {
	Observation api.WorkerSessionObservation
	Logs        api.WorkerSessionLogPage
}

func readNamedHistorySnapshot(t *testing.T, ctx context.Context, binary, project string, env []string, server string) historySnapshot {
	t.Helper()
	snapshot := readHistorySnapshot(t, ctx, binary, project, env, server)
	observation := snapshot.Observation
	if observation.WorkName == nil || *observation.WorkName != "seed-archived-name" || observation.WorkId == nil || *observation.WorkId == "" {
		t.Fatalf("archived known name = %+v", observation)
	}
	table := historyCLI(t, ctx, binary, project, env, "--server", server, "worker-sessions", "list", "--history", "archived")
	if !bytes.Contains(table, []byte("seed-archived-name")) {
		t.Fatalf("archived table lost Work name: %s", table)
	}
	return snapshot
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
	for _, action := range []string{"LIST", "READ", "logs"} {
		args := map[string]any{"action": action}
		if action == "LIST" {
			args["history"] = "archived"
		} else {
			args["workerSessionId"] = observation.WorkerSessionId
		}
		if action == "logs" {
			args["action"], args["view"], args["limit"] = "READ", "logs", 1000
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: args})
		if err != nil || result.IsError || len(result.Content) != 1 {
			t.Fatalf("MCP %s: result=%+v error=%v", action, result, err)
		}
		content, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("MCP %s content: %T", action, result.Content[0])
		}
		assertHistoryMCPResult(t, action, content.Text, observation, logs)
	}
}

func assertHistoryMCPResult(t *testing.T, action, payload string, observation api.WorkerSessionObservation, logs api.WorkerSessionLogPage) {
	t.Helper()
	var envelope struct {
		Result struct {
			Sessions []api.WorkerSessionObservation `json:"sessions"`
			Session  api.WorkerSessionObservation   `json:"session"`
			Logs     api.WorkerSessionLogPage       `json:"logs"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		t.Fatal(err)
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
