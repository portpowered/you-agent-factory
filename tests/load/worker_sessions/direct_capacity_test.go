package workersessions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/initializer/application"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttp "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
)

// Dedicated controlled workload: ten simultaneous STARTs on one MCP connection,
// then fill to saved cap10 or omitted cap16. At most16 effects, two sequential
// joined phases, zero inference/downloads. Timings are diagnostic; no assertion
// claims the real-provider two-second or endurance gates.
func TestDirectCapacityTenConcurrent(t *testing.T) {
	for _, capacity := range []int{10, 16} {
		t.Run(fmt.Sprintf("capacity-%d", capacity), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			runner := &fanoutProvider{started: make(chan struct{}, capacity), release: make(chan struct{})}
			_, host, directory, _ := fanoutHost(t, ctx, capacity, runner)
			t.Cleanup(runner.unblock)
			process, environment := fanoutClient(t, ctx, directory)
			session := fanoutMCP(t, ctx, process, host, directory, environment)
			fanoutWorkload(t, ctx, session, process, host, directory, environment, capacity, runner)
		})
	}
}

type fanoutResponse struct {
	id                    string
	result                *mcp.CallToolResult
	err                   error
	started, acknowledged time.Time
}

func fanoutWorkload(t *testing.T, ctx context.Context, session *mcp.ClientSession, process *application.Process, host, directory string, environment []string, capacity int, runner *fanoutProvider) {
	t.Helper()
	start := func(id string) fanoutResponse {
		began := time.Now()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{
			"requestId": id, "workerSessionId": id, "prompt": "controlled load", "provider": "codex", "model": "controlled-model", "reasoningEffort": "high", "workingRoot": directory}})
		return fanoutResponse{id, result, err, began, time.Now()}
	}
	responses := make(chan fanoutResponse, 10)
	for index := range 10 {
		go func() { responses <- start(fmt.Sprintf("fanout-%02d", index)) }()
	}
	ids := make([]string, 0, capacity)
	for range 10 {
		select {
		case result := <-responses:
			assertFanoutAccepted(t, result)
			ids = append(ids, result.id)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	for index := 10; index < capacity; index++ {
		result := start(fmt.Sprintf("fanout-%02d", index))
		assertFanoutAccepted(t, result)
		ids = append(ids, result.id)
	}
	for range capacity {
		select {
		case <-runner.started:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	refused := start("excess")
	assertFanoutRefusal(t, refused, host, capacity)
	if runner.calls.Load() != int32(capacity) || runner.peak.Load() != int32(capacity) {
		t.Fatalf("effect bound calls=%d peak=%d", runner.calls.Load(), runner.peak.Load())
	}
	runner.unblock()
	followFanout(t, ctx, process, host, directory, environment, ids)
	readFanout(t, ctx, session, ids)
	if runner.active.Load() != 0 {
		t.Fatalf("joined live effects=%d", runner.active.Load())
	}
	t.Logf("capacity=%d acknowledged=%d peakEffects=%d joinedEffects=0; controlled timestamps above are not real-provider latency evidence", capacity, len(ids), runner.peak.Load())
}

func assertFanoutRefusal(t *testing.T, refused fanoutResponse, host string, capacity int) {
	t.Helper()
	if refused.err != nil || refused.result == nil || !refused.result.IsError {
		t.Fatalf("full host refusal transport=%v", refused.err)
	}
	var failure struct {
		Error struct {
			Code      string `json:"code"`
			Retryable bool   `json:"retryable"`
			Details   struct {
				Host     string `json:"host"`
				Capacity int    `json:"capacity"`
			} `json:"details"`
		} `json:"error"`
	}
	decodeFanoutResult(t, refused.result, &failure)
	if failure.Error.Code != "worker_session.capacity_exceeded" || !failure.Error.Retryable || failure.Error.Details.Host != host || failure.Error.Details.Capacity != capacity {
		t.Fatalf("capacity refusal=%+v", failure)
	}
}

func followFanout(t *testing.T, ctx context.Context, process *application.Process, host, directory string, environment, ids []string) {
	t.Helper()
	var output bytes.Buffer
	args := append([]string{"you", "--server", host, "--json", "worker-sessions", "follow", "--terminal"}, ids...)
	if err := process.Execute(root.Input{Context: ctx, Args: args, Env: environment, WorkingDirectory: directory, Stdout: &output, Stderr: io.Discard}); err != nil {
		t.Fatal(err)
	}
	if lines := strings.Count(strings.TrimSpace(output.String()), "\n") + 1; lines != len(ids) {
		t.Fatalf("terminal lines=%d want=%d", lines, len(ids))
	}
}

func readFanout(t *testing.T, ctx context.Context, session *mcp.ClientSession, ids []string) {
	t.Helper()
	for _, id := range ids {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "you.subagent", Arguments: map[string]any{"action": "READ", "workerSessionId": id}})
		if err != nil || result == nil || result.IsError {
			t.Fatalf("terminal READ %s: %v", id, err)
		}
		var observation struct {
			Result struct {
				Session struct {
					ID    string `json:"workerSessionId"`
					State string `json:"state"`
				} `json:"session"`
			} `json:"result"`
		}
		decodeFanoutResult(t, result, &observation)
		if observation.Result.Session.ID != id || observation.Result.Session.State != "COMPLETED" {
			t.Fatalf("terminal READ=%+v", observation)
		}
	}
}

func assertFanoutAccepted(t *testing.T, response fanoutResponse) {
	t.Helper()
	if response.err != nil || response.result == nil || response.result.IsError {
		t.Fatalf("START %s transport=%v envelope=%+v", response.id, response.err, response.result)
	}
	var accepted struct {
		Result struct {
			Accepted bool   `json:"accepted"`
			ID       string `json:"workerSessionId"`
		} `json:"result"`
	}
	decodeFanoutResult(t, response.result, &accepted)
	if !accepted.Result.Accepted || accepted.Result.ID != response.id {
		t.Fatalf("admission=%+v", accepted)
	}
	t.Logf("id=%s startUTC=%s acknowledgedUTC=%s elapsed=%s", response.id, response.started.UTC().Format(time.RFC3339Nano), response.acknowledged.UTC().Format(time.RFC3339Nano), response.acknowledged.Sub(response.started))
}

func decodeFanoutResult(t *testing.T, result *mcp.CallToolResult, value any) {
	t.Helper()
	encoded, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		if len(result.Content) != 1 {
			t.Fatal("expected one success envelope")
		}
		content, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatal("expected text success envelope")
		}
		encoded = []byte(content.Text)
	}
	if err := json.Unmarshal(encoded, value); err != nil {
		t.Fatal(err)
	}
}

type fanoutProvider struct {
	started             chan struct{}
	release             chan struct{}
	once                sync.Once
	calls, active, peak atomic.Int32
}

func (r *fanoutProvider) unblock() { r.once.Do(func() { close(r.release) }) }

func (r *fanoutProvider) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.calls.Add(1)
	active := r.active.Add(1)
	defer r.active.Add(-1)
	for prior := r.peak.Load(); active > prior; prior = r.peak.Load() {
		if r.peak.CompareAndSwap(prior, active) {
			break
		}
	}
	r.started <- struct{}{}
	select {
	case <-r.release:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	return platformprocess.CommandResult{Stdout: []byte("{\"type\":\"item.completed\",\"item\":{\"id\":\"final\",\"type\":\"agent_message\",\"text\":\"controlled completion\"}}\n{\"type\":\"turn.completed\"}\n")}, nil
}

func fanoutHost(t *testing.T, ctx context.Context, capacity int, runner *fanoutProvider) (*application.Process, string, string, []string) {
	t.Helper()
	directory, home := t.TempDir(), t.TempDir()
	factoryDir := filepath.Join(directory, "factory")
	if err := os.MkdirAll(factoryDir, 0700); err != nil {
		t.Fatal(err)
	}
	scopedLatencyFactory(t, factoryDir)
	path := filepath.Join(home, ".you-agent-factory", "config.json")
	config := []byte(`{"defaults":{}}`)
	if capacity == 10 {
		config = []byte(`{"defaults":{"directSessionCapacity":10}}`)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, config, 0600); err != nil {
		t.Fatal(err)
	}
	ready := make(chan string, 1)
	process, err := root.BuildProcess(ctx, edges.Edges{FactorySessionsWorkingDirectory: platformfilesystem.Local{WorkingDirectory: directory}, FactorySessionResolveHomeDirectory: func() (string, error) { return home, nil }, ProviderCommandRunner: runner,
		ProviderCatalogProbe: func(_ context.Context, d providers.Descriptor) (providers.Descriptor, error) {
			d.Readiness = providers.ReadinessReady
			return d, nil
		},
		APIServerStarter: func(ctx context.Context, request platformhttp.StartRequest) error {
			server := httptest.NewServer(request.Handler)
			defer server.Close()
			address := server.Listener.Addr().(*net.TCPAddr)
			if request.OnBound != nil {
				request.OnBound(platformhttp.Binding{Host: address.IP.String(), Port: address.Port})
			}
			ready <- server.URL
			<-ctx.Done()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := process.Close(closeCtx); err != nil {
			t.Error(err)
		}
	})
	environment := builtcliacceptance.ProcessEnvForIsolatedHome(home)
	hostCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	var hostErr error
	go func() {
		hostErr = process.Execute(root.Input{Context: hostCtx, Args: []string{"you", "server"}, Env: environment, WorkingDirectory: directory, Stdout: io.Discard, Stderr: io.Discard})
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("load host did not join")
		}
	})
	select {
	case host := <-ready:
		return process, host, directory, environment
	case <-done:
		t.Fatalf("host before readiness=%v", hostErr)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	return nil, "", "", nil
}

// A remote MCP caller owns a separate process lifecycle from the host. Reuse
// that one caller for the connection and joined watcher across the whole phase.
func fanoutClient(t *testing.T, ctx context.Context, directory string) (*application.Process, []string) {
	t.Helper()
	process, err := root.BuildProcess(ctx, edges.Edges{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := process.Close(closeCtx); err != nil {
			t.Error(err)
		}
	})
	environment := builtcliacceptance.ProcessEnvForIsolatedHome(t.TempDir())
	if err := process.Execute(root.Input{Context: ctx, Args: []string{"you", "init", "--provider", "codex"}, Env: environment, WorkingDirectory: directory, Stdout: io.Discard, Stderr: io.Discard}); err != nil {
		t.Fatal(err)
	}
	return process, environment
}

func fanoutMCP(t *testing.T, ctx context.Context, process *application.Process, host, directory string, environment []string) *mcp.ClientSession {
	t.Helper()
	stdinRead, stdinWrite := io.Pipe()
	stdoutRead, stdoutWrite := io.Pipe()
	mcpCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		err := process.Execute(root.Input{Context: mcpCtx, Args: []string{"you", "--server", host, "server", "mcp"}, Env: environment, WorkingDirectory: directory, Stdin: stdinRead, Stdout: stdoutWrite, Stderr: io.Discard})
		_ = stdinRead.CloseWithError(err)
		_ = stdoutWrite.CloseWithError(err)
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		_ = stdinWrite.Close()
		_ = stdinRead.Close()
		_ = stdoutWrite.Close()
		_ = stdoutRead.Close()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			t.Error("load MCP did not join")
		}
	})
	client := mcp.NewClient(&mcp.Implementation{Name: "bounded-direct-fanout", Version: "test"}, nil)
	session, err := client.Connect(ctx, &mcp.IOTransport{Reader: stdoutRead, Writer: stdinWrite}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}
