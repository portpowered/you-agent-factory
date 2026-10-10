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
	"sync"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type l1Script struct {
	entered chan *l1Stream
	release chan struct{}
	finish  chan struct{}
}

type l1Stream struct {
	id      string
	mu      sync.Mutex
	emitted []time.Time
}

func (runner *l1Script) Run(ctx context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return runner.RunStreaming(ctx, request, nil)
}

func (runner *l1Script) RunStreaming(ctx context.Context, _ platformprocess.CommandRequest, observe platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error) {
	if observe == nil {
		return platformprocess.CommandResult{}, errors.New("L1 requires streaming output")
	}
	stream := &l1Stream{}
	observe(platformprocess.OutputStreamStdout, fmt.Appendf(nil, "initial stream=%p\n", stream))
	runner.entered <- stream
	select {
	case <-runner.release:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	// This clock is the declared load generator (11/s/session), not a readiness
	// sleep. Record every actual emission; missed ticks cannot count as workload.
	// Slight headroom permits >=1,000 acknowledged commits/s in the fixed window;
	// generator ticks and post-window drain never establish committed throughput.
	ticker := time.NewTicker(time.Second / 11)
	defer ticker.Stop()
	for {
		select {
		case <-runner.finish:
			return platformprocess.CommandResult{}, nil
		case <-ticker.C:
			stream.mu.Lock()
			stream.emitted = append(stream.emitted, time.Now())
			sequence := len(stream.emitted)
			stream.mu.Unlock()
			observe(platformprocess.OutputStreamStdout, fmt.Appendf(nil, "l1 sequence=%06d\n", sequence))
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
	}
}

func startL1Host(t *testing.T, ctx context.Context, dir string, runner platformprocess.CommandRunner, writers ...recordings.WorkerRecordingWriter) (string, recordings.WorkerRecordingStore) {
	t.Helper()
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	ready := make(chan string, 1)
	var store recordings.WorkerRecordingStore
	var writer recordings.WorkerRecordingWriter
	if len(writers) > 0 {
		writer = writers[0]
	}
	process, err := root.BuildProcess(ctx, edges.Edges{
		WorkerRecordingWriter: writer,
		ScriptCommandRunner:   runner, FactorySessionsWorkingDirectory: evictionWorkingDirectory(dir),
		WorkerRecordingStoreObserver: func(value recordings.WorkerRecordingStore) {
			store = value
			if pressure, ok := writer.(*pressureWriter); ok {
				pressure.WorkerRecordingStore = value
			}
		},
		ProviderCommandRunner: l1ForbiddenProvider{},
		APIServerStarter: func(hostCtx context.Context, request platformhttpserver.StartRequest) error {
			server := httptest.NewServer(request.Handler)
			defer server.Close()
			ready <- server.URL
			<-hostCtx.Done()
			return hostCtx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	execute := func(callCtx context.Context, args ...string) error {
		var stderr bytes.Buffer
		err := process.Execute(root.Input{Context: callCtx, Args: append([]string{"you"}, args...), Env: env, WorkingDirectory: dir, Stdout: io.Discard, Stderr: &stderr})
		if err != nil {
			return fmt.Errorf("%w; stderr=%s", err, stderr.String())
		}
		return nil
	}
	if err := execute(ctx, "run", "--factory", filepath.Join(dir, "missing-l1-factory.json")); err == nil {
		t.Fatal("bootstrap expected missing Factory")
	}
	hostCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- execute(hostCtx, "run", "--dir", dir, "--continuously", "--with-server", "--quiet", "--no-record")
	}()
	t.Cleanup(func() {
		stop()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("host shutdown: %v", err)
			}
		case <-time.After(30 * time.Second):
			t.Error("L1 host did not join")
		}
		closeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := process.Close(closeCtx); err != nil {
			t.Errorf("L1 close: %v", err)
		}
	})
	select {
	case endpoint := <-ready:
		return endpoint, store
	case err := <-done:
		t.Fatalf("host exited: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	return "", nil
}

// Factory admission lets Runtime resolve the authored SCRIPT_WORKER command.
// The public direct-start envelope intentionally has no script command field.
func admitL1FactoryScript(t *testing.T, ctx context.Context, endpoint, dir string, count int) []string {
	t.Helper()
	body := fleetProfileHTTP(t, ctx, http.MethodPost, endpoint+"/factory-sessions", map[string]any{"folderPath": dir})
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(body, &opened); err != nil || opened.Session == nil {
		t.Fatalf("open L1 Factory: %v %s", err, body)
	}
	scope := opened.Session.Id
	t.Logf("Factory fixture scope=%s", scope)
	works := make([]map[string]any, count)
	for index := range works {
		works[index] = map[string]any{"name": fmt.Sprintf("l1-work-%03d", index), "workTypeName": "task", "payload": map[string]any{"title": "controlled L1 script"}}
	}
	fleetProfileHTTP(t, ctx, http.MethodPut, endpoint+"/factory-sessions/"+scope+"/work-requests/l1", map[string]any{"requestId": "l1", "type": "FACTORY_REQUEST_BATCH", "works": works})
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	logged := false
	for {
		body = fleetProfileHTTP(t, ctx, http.MethodGet, endpoint+"/worker-sessions?history=active&maxResults=100", nil)
		var fleet factoryapi.ListWorkerSessionsResponse
		if err := json.Unmarshal(body, &fleet); err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, count)
		for _, row := range fleet.Sessions {
			if row.FactorySessionId != nil && *row.FactorySessionId == scope {
				ids = append(ids, row.WorkerSessionId)
			}
		}
		if !logged {
			logged = true
			t.Logf("waiting for script admissions: observed=%d want=%d", len(ids), count)
		}
		if len(ids) == count {
			return ids
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("INCONCLUSIVE: Factory admitted %d/%d scripts: %v", len(ids), count, ctx.Err())
		}
	}
}

type l1ForbiddenProvider struct{}

func (l1ForbiddenProvider) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{}, errors.New("L1 forbids provider execution")
}
