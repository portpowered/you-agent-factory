package workersessions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Dedicated bounded stress cell: one Direct Worker Session, one script,
// 10,048 small progress records, and a five-minute ceiling. The real default
// Events ring (10,000 records) is evicted while the observer's HTTP connection
// is gated. Capture commits are observed between 64-record batches to avoid
// conflating ring eviction with capture overload. This proves continuity under
// eviction, not L1 throughput, latency, retained size, or delivered OS restart.
func TestCapturedFollowBackfillsEvictedEventsRing(t *testing.T) {
	if testing.Short() {
		t.Skip("dedicated Events retention stress cell")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	dir := prepareEvictionFactory(t)
	gate := &evictionConnection{entered: make(chan struct{}), release: make(chan struct{})}
	runner := &evictionScript{started: make(chan struct{}), batches: make(chan int), finish: make(chan struct{})}
	baseURL, execute := startEvictionHost(t, ctx, dir, gate, runner)
	id := admitEvictionScript(t, ctx, baseURL, runner.started)
	prefix := waitEvictionCapture(t, ctx, baseURL, id, 2)
	if prefix.NextToken == nil || prefix.CommittedPosition != 2 {
		t.Fatalf("initial capture not resumable: %+v", prefix)
	}
	output := evictionOutput{ctx: ctx, release: gate.release}
	done := make(chan error, 1)
	go func() {
		done <- execute(ctx, &output, "--server", baseURL, "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--follow", "--output", "json")
	}()
	waitEvictionSignal(t, ctx, gate.entered)
	// The observer has drained positions 1..2; hold its notification connection
	// until the source ring has lost that prefix. Its durable token stays valid.
	for emitted := 0; emitted < 10048; emitted += 64 {
		select {
		case runner.batches <- 64:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
		waitEvictionCapture(t, ctx, baseURL, id, int64(emitted+66))
		if emitted%2048 == 0 {
			t.Logf("committed progress records=%d", emitted+64)
		}
	}
	assertEvictedPublicEvents(t, ctx, baseURL, id)
	close(gate.release)
	close(runner.finish)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("durable follow after actual ring eviction: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("follow did not drain terminal capture")
	}
	assertEvictionReplay(t, ctx, baseURL, id, output.Bytes(), *prefix.NextToken, execute)
}

type evictionConnection struct {
	claimed atomic.Bool
	entered chan struct{}
	release chan struct{}
}

// Hold consumer output after the initial two records while the source ring is
// primed. This avoids repeated follower page reads contending with the same
// capture writer during setup, and represents a paused customer observer.
type evictionOutput struct {
	bytes.Buffer
	ctx     context.Context
	release <-chan struct{}
	writes  int
}

func (output *evictionOutput) Write(data []byte) (int, error) {
	if output.writes >= 2 {
		select {
		case <-output.release:
		case <-output.ctx.Done():
			return 0, output.ctx.Err()
		}
	}
	output.writes++
	return output.Buffer.Write(data)
}

func (gate *evictionConnection) wrap(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/events") && gate.claimed.CompareAndSwap(false, true) {
			close(gate.entered)
			select {
			case <-gate.release:
			case <-request.Context().Done():
				return
			}
		}
		handler.ServeHTTP(w, request)
	})
}

type evictionScript struct {
	started chan struct{}
	batches chan int
	finish  chan struct{}
}

func (runner *evictionScript) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.RunStreaming(ctx, request, nil)
}

func (runner *evictionScript) RunStreaming(ctx context.Context, _ platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	if observe == nil {
		return platformprocess.CommandResult{}, errors.New("script requires streaming observation")
	}
	observe(platformprocess.OutputStreamStdout, []byte("initial committed progress\n"))
	close(runner.started)
	position := 0
	for {
		select {
		case count := <-runner.batches:
			for range count {
				position++
				observe(platformprocess.OutputStreamStdout, fmt.Appendf(nil, "progress %d\n", position))
			}
		case <-runner.finish:
			return platformprocess.CommandResult{}, nil
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
}

func prepareEvictionFactory(t *testing.T) string {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, filepath.Join(testutil.MustRepoRoot(t), "tests", "functional_test", "testdata", "executor_success"))
	if err := os.RemoveAll(filepath.Join(dir, "inputs")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workers", "worker", "AGENTS.md"), []byte("---\ntype: SCRIPT_WORKER\ncommand: controlled-eviction-script\n---\nEmit controlled progress.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

type evictionExecute func(context.Context, io.Writer, ...string) error
type evictionWorkingDirectory string

func (dir evictionWorkingDirectory) Getwd() (string, error) { return string(dir), nil }

func startEvictionHost(t *testing.T, ctx context.Context, dir string, gate *evictionConnection, runner *evictionScript) (string, evictionExecute) {
	t.Helper()
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	ready := make(chan string, 1)
	process, err := root.BuildProcess(ctx, edges.Edges{
		ScriptCommandRunner: runner, FactorySessionsWorkingDirectory: evictionWorkingDirectory(dir),
		ProviderSessionResolveHomeDirectory: func() (string, error) { return home, nil },
		APIServerStarter: func(hostCtx context.Context, request platformhttpserver.StartRequest) error {
			server := httptest.NewServer(gate.wrap(request.Handler))
			defer server.Close()
			ready <- server.URL
			<-hostCtx.Done()
			return hostCtx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	execute := func(callCtx context.Context, output io.Writer, args ...string) error {
		var stderr bytes.Buffer
		err := process.Execute(root.Input{Context: callCtx, Args: append([]string{"you"}, args...), Env: env,
			WorkingDirectory: dir, Stdout: output, Stderr: &stderr})
		if err != nil {
			return fmt.Errorf("%w; stderr=%s", err, stderr.String())
		}
		return nil
	}
	if err := execute(ctx, io.Discard, "run", "--factory", filepath.Join(dir, "missing-eviction-factory.json")); err == nil || !strings.Contains(err.Error(), "missing-eviction-factory.json") {
		t.Fatalf("bootstrap diagnostic=%v", err)
	}
	hostCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- execute(hostCtx, io.Discard, "run", "--dir", dir, "--continuously", "--with-server", "--quiet", "--no-record")
	}()
	t.Cleanup(func() {
		stop()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("host shutdown: %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("host did not join")
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := process.Close(closeCtx); err != nil {
			t.Errorf("process close: %v", err)
		}
	})
	select {
	case endpoint := <-ready:
		return endpoint, execute
	case err := <-done:
		t.Fatalf("host exited before readiness: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	return "", nil
}

func admitEvictionScript(t *testing.T, ctx context.Context, baseURL string, started <-chan struct{}) string {
	t.Helper()
	worker := "worker"
	fleetProfileHTTP(t, ctx, http.MethodPost, baseURL+"/worker-sessions", factoryapi.WorkerSessionStartRequest{
		RequestId: "eviction-request", WorkerSessionId: "eviction-worker",
		Execution: factoryapi.WorkerSessionResolvedExecution{WorkstationName: "process", WorkerType: &worker,
			Dispatch: factoryapi.WorkerSessionResolvedDispatch{DispatchId: "eviction-dispatch", WorkstationName: "process", WorkerType: &worker}},
	})
	waitEvictionSignal(t, ctx, started)
	return "eviction-worker"
}

func waitEvictionSignal(t *testing.T, ctx context.Context, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func waitEvictionCapture(t *testing.T, ctx context.Context, baseURL, id string, position int64) factoryapi.WorkerSessionLogPage {
	t.Helper()
	// Durable commits have no public subscription. Poll only their watermark;
	// the bounded cadence is an observation mechanism, never a readiness sleep.
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		body := fleetProfileHTTP(t, ctx, http.MethodGet, baseURL+"/worker-sessions/"+id+"/logs?limit=1", nil)
		var page factoryapi.WorkerSessionLogPage
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatal(err)
		}
		if page.CommittedPosition >= position {
			return page
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
}

func assertEvictedPublicEvents(t *testing.T, ctx context.Context, baseURL, id string) {
	t.Helper()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/worker-sessions/"+id+"/events?after_position=2", nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusOK || !bytes.Contains(body, []byte(`"delivery":"SOURCE_FAILURE"`)) || !bytes.Contains(body, []byte(`"WORKER_SESSION_STREAM_GAP"`)) {
		t.Fatalf("real Events ring did not report evicted cursor: status=%d body=%s", response.StatusCode, body)
	}
}

func assertEvictionReplay(t *testing.T, ctx context.Context, baseURL, id string, output []byte, token string, execute evictionExecute) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(output))
	var followed []factoryapi.WorkerSessionEvent
	for {
		var event factoryapi.WorkerSessionEvent
		if err := decoder.Decode(&event); errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if event.Event.Position != int64(len(followed)+1) {
			t.Fatalf("eviction follow lost/repeated position: got=%d after=%d", event.Event.Position, len(followed))
		}
		followed = append(followed, event)
	}
	var captured []factoryapi.WorkerSessionEvent
	endpoint := baseURL + "/worker-sessions/" + id + "/logs?limit=1000"
	for {
		body := fleetProfileHTTP(t, ctx, http.MethodGet, endpoint, nil)
		var page factoryapi.WorkerSessionLogPage
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatal(err)
		}
		captured = append(captured, page.Events...)
		if page.NextToken == nil {
			if page.Health != factoryapi.COMPLETE || page.CommittedPosition != int64(len(followed)) {
				t.Fatalf("eviction terminal capture incomplete: health=%s head=%d followed=%d", page.Health, page.CommittedPosition, len(followed))
			}
			break
		}
		endpoint = baseURL + "/worker-sessions/" + id + "/logs?limit=1000&nextToken=" + *page.NextToken
	}
	if len(followed) < 10050 || !reflect.DeepEqual(followed, captured) {
		t.Fatalf("eviction changed committed replay: follow=%d capture=%d", len(followed), len(captured))
	}
	var resumed bytes.Buffer
	if err := execute(ctx, &resumed, "--server", baseURL, "worker-sessions", "read", "--worker-session-id", id, "--view", "logs", "--follow", "--next-token", token, "--output", "json"); err != nil {
		t.Fatal(err)
	}
	// The initial one-record page acknowledged only position 1.
	firstEnd := bytes.IndexByte(output, '\n') + 1
	if !bytes.Equal(resumed.Bytes(), output[firstEnd:]) {
		t.Fatal("acknowledged reconnect changed the committed tail after eviction")
	}
	t.Logf("evicted Events cursor=2; committed once-only follow/reconnect records=%d; batches=157x64; full L1 remains unproved", len(followed))
}
