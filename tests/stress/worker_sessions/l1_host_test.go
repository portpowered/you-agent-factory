package workersessions_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
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
)

type l1Script struct {
	entered chan *l1Stream
	release chan struct{}
	finish  chan struct{}
}

type l1Stream struct {
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
	observe(platformprocess.OutputStreamStdout, []byte("initial committed progress\n"))
	runner.entered <- stream
	select {
	case <-runner.release:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	// This clock is the declared load generator (11/s/session), not a readiness
	// sleep. Record every actual emission; missed ticks cannot count as workload.
	// Slight headroom ensures the measured achieved rate, including query drain,
	// must still meet 1,000/s rather than rounding a nominal 10/s into a pass.
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

func startL1Host(t *testing.T, ctx context.Context, dir string, runner *l1Script) (string, recordings.WorkerRecordingStore) {
	t.Helper()
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	ready := make(chan string, 1)
	var store recordings.WorkerRecordingStore
	process, err := root.BuildProcess(ctx, edges.Edges{
		ScriptCommandRunner: runner, FactorySessionsWorkingDirectory: evictionWorkingDirectory(dir),
		WorkerRecordingStoreObserver:        func(value recordings.WorkerRecordingStore) { store = value },
		ProviderSessionResolveHomeDirectory: func() (string, error) { return home, nil },
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
