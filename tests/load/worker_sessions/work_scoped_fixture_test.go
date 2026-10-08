package workersessions_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/initializer/application"
	platformhttp "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

type scopedLatencyFixture struct {
	dir, serverURL, sessionID, workID string
	environment                       []string
}

type scopedLatencyRunner struct {
	mu      sync.Mutex
	command scopedLatencyCommand
}

func (runner *scopedLatencyRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return platformprocess.CommandResult{Stdout: runner.command.stdout()}, nil
}

func newScopedLatencyFixture(t *testing.T, ctx context.Context) scopedLatencyFixture {
	t.Helper()
	dir := t.TempDir()
	home := filepath.Join(dir, "home")
	scopedLatencyFactory(t, dir)
	ready := make(chan *httptest.Server, 1)
	process, err := root.BuildProcess(ctx, edges.Edges{
		ProviderCommandRunner:               &scopedLatencyRunner{},
		ProviderSessionResolveHomeDirectory: func() (string, error) { return home, nil },
		APIServerStarter: func(ctx context.Context, request platformhttp.StartRequest) error {
			server := httptest.NewServer(request.Handler)
			defer server.Close()
			if request.OnBound != nil {
				request.OnBound(platformhttp.Binding{Port: request.Port})
			}
			ready <- server
			<-ctx.Done()
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = process.Close(context.Background()) })
	f := scopedLatencyFixture{dir: dir, environment: builtcliacceptance.ProcessEnvForIsolatedHome(home)}
	server := startScopedLatencyHost(t, ctx, process, f, ready)
	f.serverURL = server.URL
	var opened factoryapi.OpenFactorySessionResponse
	fleetPOST(t, server.Config.Handler, "/factory-sessions", factoryapi.OpenFactorySessionRequest{FolderPath: dir}, http.StatusOK, &opened)
	if opened.Session == nil {
		t.Fatal("Session absent")
	}
	f.sessionID = opened.Session.Id
	var submitted factoryapi.SubmitWorkResponse
	fleetPOST(t, server.Config.Handler, "/factory-sessions/"+f.sessionID+"/work", factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: map[string]string{"title": "scoped latency"}}, http.StatusCreated, &submitted)
	if submitted.WorkId == nil {
		t.Fatal("Work absent")
	}
	f.workID = *submitted.WorkId
	waitScopedLatencyCommit(t, ctx, f)
	return f
}

func startScopedLatencyHost(t *testing.T, ctx context.Context, process *application.Process, f scopedLatencyFixture, ready <-chan *httptest.Server) *httptest.Server {
	t.Helper()
	// Bootstrap the isolated profile through the existing customer boundary.
	_ = process.Execute(application.Input{Args: []string{"you", "run", "--factory", filepath.Join(f.dir, "missing.json")}, Env: f.environment, WorkingDirectory: f.dir, Context: ctx, Stdout: io.Discard, Stderr: io.Discard})
	hostCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	var hostErr error
	go func() {
		hostErr = process.Execute(application.Input{Args: []string{"you", "run", "--dir", f.dir, "--continuously", "--with-server", "--quiet"}, Env: f.environment, WorkingDirectory: f.dir, Context: hostCtx, Stdout: io.Discard, Stderr: io.Discard})
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
		if hostErr != nil && !errors.Is(hostErr, context.Canceled) {
			t.Errorf("owned host cleanup: %v", hostErr)
		}
	})
	var server *httptest.Server
	select {
	case server = <-ready:
	case <-done:
		t.Fatalf("host exited: %v", hostErr)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	return server
}

func waitScopedLatencyCommit(t *testing.T, ctx context.Context, f scopedLatencyFixture) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		raw, _, _ := scopedLatencyHTTP(t, ctx, f.serverURL+"/factory-sessions/"+f.sessionID+"/worker-sessions?workId="+f.workID)
		var listed factoryapi.ListWorkerSessionsResponse
		if err := json.Unmarshal(raw, &listed); err != nil {
			t.Fatal(err)
		}
		complete := len(listed.Sessions) == 200
		for _, row := range listed.Sessions {
			complete = complete && row.State == factoryapi.WorkerSessionObservationStateCompleted && row.TokenUsage != nil
		}
		if complete {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("200 attempts never committed: rows=%d: %v", len(listed.Sessions), ctx.Err())
		case <-ticker.C:
		}
	}
}
