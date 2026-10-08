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
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// This bounded incident-size profile is deliberately separate from functional
// coverage. It measures the unchanged customer client deadline on a 70-Work
// board; it does not claim Project L1 throughput or production capacity.
func TestFleetActiveListSeventyWorkProfile(t *testing.T) {
	if testing.Short() {
		t.Skip("incident-sized active fleet latency profile")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 110*time.Second)
	defer cancel()
	dir := testutil.CopyFixtureDir(t, filepath.Join(testutil.MustRepoRoot(t), "tests", "functional_test", "testdata", "executor_success"))
	if err := os.RemoveAll(filepath.Join(dir, "inputs")); err != nil {
		t.Fatal(err)
	}
	worker := filepath.Join(dir, "workers", "worker", "AGENTS.md")
	if err := os.WriteFile(worker, []byte("---\ntype: MODEL_WORKER\nmodel: fixture-model\nmodelProvider: codex\nstopToken: COMPLETE\n---\nProcess input.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	env := append(os.Environ(), "HOME="+home, "USERPROFILE="+home)
	ready := make(chan *httptest.Server, 1)
	runner := &heldFleetCommand{entered: make(chan struct{}), release: make(chan struct{})}
	process, err := root.BuildProcess(ctx, edges.Edges{
		ProviderCommandRunner: runner,
		APIServerStarter: func(hostCtx context.Context, request platformhttpserver.StartRequest) error {
			server := httptest.NewServer(request.Handler)
			defer server.Close()
			ready <- server
			<-hostCtx.Done()
			return hostCtx.Err()
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	execute := func(args ...string) ([]byte, error) {
		var stdout, stderr bytes.Buffer
		err := process.Execute(root.Input{Context: ctx, Args: append([]string{"you"}, args...), Env: env,
			WorkingDirectory: dir, Stdout: &stdout, Stderr: &stderr})
		if err != nil {
			return stdout.Bytes(), fmt.Errorf("%w; stderr=%s", err, stderr.String())
		}
		return stdout.Bytes(), nil
	}
	// First-run bootstrap belongs outside the measured query window.
	if _, err := execute("run", "--factory", filepath.Join(dir, "missing-profile-factory.json")); err == nil || !strings.Contains(err.Error(), "missing-profile-factory.json") {
		t.Fatalf("bootstrap missing Factory diagnostic=%v", err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := execute("run", "--dir", dir, "--continuously", "--with-server", "--quiet", "--no-record")
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		close(runner.release)
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("host shutdown: %v", err)
			}
		case <-time.After(15 * time.Second):
			t.Error("host did not join")
		}
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer closeCancel()
		if err := process.Close(closeCtx); err != nil {
			t.Errorf("process close: %v", err)
		}
	})
	var server *httptest.Server
	select {
	case server = <-ready:
	case err := <-done:
		t.Fatalf("host exited before readiness: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	assertFleetProfileReads(t, ctx, server.URL, dir, execute, runner.entered)
}

func assertFleetProfileReads(t *testing.T, ctx context.Context, baseURL, dir string, execute func(...string) ([]byte, error), admitted <-chan struct{}) {
	t.Helper()
	// Explicit session owns the board and all admitted workers.
	openedBody := fleetProfileHTTP(t, ctx, http.MethodPost, baseURL+"/factory-sessions", map[string]any{"folderPath": dir})
	var opened factoryapi.OpenFactorySessionResponse
	if err := json.Unmarshal(openedBody, &opened); err != nil || opened.Session == nil {
		t.Fatalf("open explicit session: %v %s", err, openedBody)
	}
	sessionID := opened.Session.Id
	works := make([]map[string]any, 70)
	for index := range works {
		works[index] = map[string]any{"name": fmt.Sprintf("fleet-profile-%02d", index), "workTypeName": "task", "payload": map[string]any{"title": "controlled fleet profile"}}
		// Board size and worker concurrency are different dimensions. Keep three
		// held active attempts; admit terminal neighbors without dispatching them.
		if index >= 3 {
			works[index]["state"] = "done"
		}
	}
	request := map[string]any{"requestId": "fleet-seventy-profile", "type": "FACTORY_REQUEST_BATCH", "works": works}
	fleetProfileHTTP(t, ctx, http.MethodPut, baseURL+"/factory-sessions/"+sessionID+"/work-requests/fleet-seventy-profile", request)
	select {
	case <-admitted:
	case <-ctx.Done():
		t.Fatal("no controlled worker admission")
	}
	boardBody := fleetProfileHTTP(t, ctx, http.MethodGet, baseURL+"/factory-sessions/"+sessionID+"/work?maxResults=100&counts=true", nil)
	var board factoryapi.ListWorkResponse
	if err := json.Unmarshal(boardBody, &board); err != nil || len(board.Results) != 70 {
		t.Fatalf("board count=%d, want 70: err=%v body=%s", len(board.Results), err, boardBody)
	}
	for _, scope := range []string{"all", "factory"} {
		started := time.Now()
		body, err := execute("--server", baseURL, "worker-sessions", "list", "--scope", scope,
			"--state", "RUNNING", "--state", "STARTING", "--max-results", "25", "--output", "json")
		elapsed := time.Since(started)
		t.Logf("board=70 scope=%s elapsed=%s bytes=%d hostOS=%s hostArch=%s go=%s CPUs=%d error=%v", scope, elapsed, len(body), runtime.GOOS, runtime.GOARCH, runtime.Version(), runtime.NumCPU(), err)
		if err != nil {
			t.Fatal(err)
		}
		var fleet factoryapi.ListWorkerSessionsResponse
		if err := json.Unmarshal(body, &fleet); err != nil || len(fleet.Sessions) == 0 || len(fleet.Sessions) > 25 {
			t.Fatalf("active fleet count=%d: err=%v body=%s", len(fleet.Sessions), err, body)
		}
		for _, row := range fleet.Sessions {
			if row.WorkId == nil || row.FactorySessionId == nil || *row.FactorySessionId != sessionID {
				t.Fatalf("active attribution=%#v", row)
			}
			fleetProfileHTTP(t, ctx, http.MethodGet, baseURL+"/factory-sessions/"+sessionID+"/worker-sessions?workId="+*row.WorkId, nil)
		}
	}
}

type heldFleetCommand struct {
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (r *heldFleetCommand) Run(ctx context.Context, _ platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.once.Do(func() { close(r.entered) })
	select {
	case <-r.release:
		return platformprocess.CommandResult{}, nil
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
}

func fleetProfileHTTP(t *testing.T, ctx context.Context, method, endpoint string, payload any) []byte {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	result, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode < 200 || response.StatusCode >= 300 {
		t.Fatalf("%s %s status=%d: %v %s", method, endpoint, response.StatusCode, err, result)
	}
	return result
}
