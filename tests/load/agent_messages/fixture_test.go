package agentmessages_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/pkg/initializer/application"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttp "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
)

type latencyCommand struct {
	at  time.Time
	env map[string]string
}
type latencyRunner struct {
	gates     map[string]chan struct{}
	started   chan latencyCommand
	continued chan latencyCommand
}

func (r *latencyRunner) Run(ctx context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	env := map[string]string{}
	for _, item := range req.Env {
		if key, value, ok := strings.Cut(item, "="); ok {
			env[key] = value
		}
	}
	command := latencyCommand{at: time.Now(), env: env}
	gate := r.gates[env["YOU_WORKER_SESSION_ID"]]
	if gate == nil {
		r.continued <- command
		<-ctx.Done()
		return platformprocess.CommandResult{}, ctx.Err()
	}
	r.started <- command
	select {
	case <-gate:
	case <-ctx.Done():
		return platformprocess.CommandResult{}, ctx.Err()
	}
	return platformprocess.CommandResult{Stdout: []byte("{\"type\":\"thread.started\",\"thread_id\":\"load-thread\"}\n{\"type\":\"item.completed\",\"item\":{\"id\":\"answer\",\"type\":\"agent_message\",\"text\":\"COMPLETE\"}}\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n")}, nil
}

type latencyHost struct {
	dir, url string
	runner   *latencyRunner
}

func prepareLatencyHost(t *testing.T, ctx context.Context) *latencyHost {
	t.Helper()
	f := &latencyHost{dir: t.TempDir(), runner: &latencyRunner{gates: map[string]chan struct{}{}, started: make(chan latencyCommand, 40), continued: make(chan latencyCommand, 20)}}
	for _, prefix := range []string{"source-", "lane-"} {
		for i := range 20 {
			f.runner.gates[fmtID(prefix, i)] = make(chan struct{})
		}
	}
	if err := os.WriteFile(filepath.Join(f.dir, "factory.json"), []byte(`{"name":"messaging-latency","workTypes":[],"workers":[],"workstations":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	ready := make(chan *httptest.Server, 1)
	process, err := root.BuildProcess(ctx, edges.Edges{ProviderCommandRunner: f.runner,
		FactorySessionsWorkingDirectory: platformfilesystem.Local{WorkingDirectory: f.dir},
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
	t.Cleanup(func() {
		if err := process.Close(context.Background()); err != nil {
			t.Error(err)
		}
	})
	env := builtcliacceptance.ProcessEnvForIsolatedHome(filepath.Join(f.dir, "home"))
	bootstrapErr := process.Execute(application.Input{Context: ctx, Args: []string{"you", "run", "--factory", filepath.Join(f.dir, "missing.json")},
		Env: env, WorkingDirectory: f.dir, Stdout: io.Discard, Stderr: io.Discard})
	if bootstrapErr == nil || !strings.Contains(bootstrapErr.Error(), "missing.json") {
		t.Fatal("isolated profile bootstrap did not reach missing Factory", bootstrapErr)
	}
	hostCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- process.Execute(application.Input{Context: hostCtx, Args: []string{"you", "run", "--dir", f.dir, "--continuously", "--with-server", "--quiet"},
			Env: env, WorkingDirectory: f.dir, Stdout: io.Discard, Stderr: io.Discard})
	}()
	t.Cleanup(func() {
		cancel()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
			t.Error(err)
		}
	})
	select {
	case server := <-ready:
		f.url = server.URL
	case err := <-done:
		t.Fatal("load host exited", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	return f
}

func (f *latencyHost) request(t *testing.T, ctx context.Context, method, path string, body any, caller map[string]string, status int, out any) {
	t.Helper()
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(ctx, method, f.url+path, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if caller != nil {
		req.Header.Set("X-You-Worker-Session-Id", caller["YOU_WORKER_SESSION_ID"])
		req.Header.Set("Authorization", "Bearer "+caller["YOU_WORKER_SESSION_TOKEN"])
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != status {
		t.Fatalf("%s %s: status=%d expected=%d", method, path, response.StatusCode, status)
	}
	if err := json.NewDecoder(response.Body).Decode(out); err != nil {
		t.Fatal(err)
	}
}
