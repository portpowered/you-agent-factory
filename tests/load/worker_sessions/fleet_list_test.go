package workersessions_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
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

// Dedicated performance workload: one root process, 200 associated sessions,
// controlled Codex command/files, no subprocesses or paid providers. Run with
// -benchtime=1s -p 1 -timeout=2m. Includes real Work attribution and serialization.
func BenchmarkFleetList(b *testing.B) {
	handler, files := fleetFixture(b)
	for _, size := range []int{10, 50, 200} {
		b.Run(fmt.Sprintf("rows-%d", size), func(b *testing.B) {
			files.opens.Store(0)
			request := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/worker-sessions?maxResults=%d", size), nil)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				response := httptest.NewRecorder()
				handler.ServeHTTP(response, request)
				if response.Code != http.StatusOK {
					b.Fatalf("fleet HTTP %d: %s", response.Code, response.Body.String())
				}
			}
			b.StopTimer()
			b.ReportMetric(float64(files.opens.Load())/float64(b.N), "native-opens/op")
			if files.opens.Load() != 0 {
				b.Fatal("fleet opened native provider files")
			}
		})
	}
}

type nativeFiles struct{ opens atomic.Int64 }

func (f *nativeFiles) Open(path string) (io.ReadCloser, error) {
	f.opens.Add(1)
	return os.Open(path)
}

func (*nativeFiles) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

type codexCommand struct{ stdout []byte }

func (c codexCommand) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: append([]byte(nil), c.stdout...)}, nil
}

func fleetFixture(b *testing.B) (http.Handler, *nativeFiles) {
	b.Helper()
	dir := b.TempDir()
	home := filepath.Join(dir, "home")
	writeFleetFactory(b, dir)
	stdout := prepareNativeFixture(b, home)
	ready := make(chan http.Handler, 1)
	files := &nativeFiles{}
	process, err := root.BuildProcess(b.Context(), edges.Edges{
		ProviderCommandRunner:               codexCommand{stdout: stdout},
		ProviderSessionFileSystem:           files,
		ProviderSessionResolveHomeDirectory: func() (string, error) { return home, nil },
		APIServerStarter: func(ctx context.Context, req platformhttp.StartRequest) error {
			ready <- req.Handler
			if req.OnBound != nil {
				req.OnBound(platformhttp.Binding{Port: req.Port})
			}
			<-ctx.Done()
			return nil
		},
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = process.Close(context.Background()) })
	handler := startFleetHost(b, process, dir, home, ready)
	var opened factoryapi.OpenFactorySessionResponse
	fleetPOST(b, handler, "/factory-sessions", factoryapi.OpenFactorySessionRequest{FolderPath: dir}, http.StatusOK, &opened)
	if opened.Session == nil {
		b.Fatal("session absent")
	}
	for range 200 {
		var submitted factoryapi.SubmitWorkResponse
		fleetPOST(b, handler, "/factory-sessions/"+opened.Session.Id+"/work", factoryapi.SubmitWorkRequest{WorkTypeName: "task", Payload: map[string]string{"title": "fleet fixture"}}, http.StatusCreated, &submitted)
	}
	rows := waitFleet(b, handler)
	// Positive control: the assembled selected detail still opens native files.
	before := files.opens.Load()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/factory-sessions/"+opened.Session.Id+"/worker-sessions/"+rows[0].WorkerSessionId, nil))
	if response.Code != http.StatusOK || files.opens.Load() <= before {
		b.Fatalf("detail did not read native files: %d %s", response.Code, response.Body.String())
	}
	return handler, files
}

func prepareNativeFixture(b *testing.B, home string) []byte {
	b.Helper()
	stdout, err := os.ReadFile(filepath.Join("..", "..", "functional", "internal", "support", "testdata", "provider-sessions", "codex", "success", "stdout.jsonl"))
	if err != nil {
		b.Fatal(err)
	}
	rollout, err := os.ReadFile(filepath.Join("..", "..", "functional", "internal", "support", "testdata", "provider-sessions", "codex", "success", "rollout.jsonl"))
	if err != nil {
		b.Fatal(err)
	}
	nativeDir := filepath.Join(home, ".codex", "sessions", "2026", "07", "27")
	if err := os.MkdirAll(nativeDir, 0700); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nativeDir, "rollout-session_fixture_codex_success.jsonl"), rollout, 0600); err != nil {
		b.Fatal(err)
	}
	return stdout
}

func startFleetHost(b *testing.B, process *application.Process, dir, home string, ready <-chan http.Handler) http.Handler {
	b.Helper()
	env := builtcliacceptance.ProcessEnvForIsolatedHome(home)
	// Bootstrap the isolated profile through the customer process before hosting.
	_ = process.Execute(application.Input{Args: []string{"you", "run", "--factory", filepath.Join(dir, "missing.json")}, Env: env, WorkingDirectory: dir, Context: b.Context(), Stdout: io.Discard, Stderr: io.Discard})
	ctx, cancel := context.WithCancel(b.Context())
	done := make(chan struct{})
	var hostErr error
	go func() {
		hostErr = process.Execute(application.Input{Args: []string{"you", "run", "--dir", dir, "--continuously", "--with-server", "--quiet"}, Env: env, WorkingDirectory: dir, Context: ctx, Stdout: io.Discard, Stderr: io.Discard})
		close(done)
	}()
	b.Cleanup(func() {
		cancel()
		<-done
		if hostErr != nil {
			b.Errorf("host cleanup: %v", hostErr)
		}
	})
	var handler http.Handler
	select {
	case handler = <-ready:
	case <-done:
		b.Fatalf("host exited: %v", hostErr)
	case <-b.Context().Done():
		b.Fatal("host unavailable")
	}
	return handler
}

func fleetPOST(b *testing.B, handler http.Handler, path string, body any, status int, out any) {
	b.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		b.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != status {
		b.Fatalf("POST %s: %d %s", path, response.Code, response.Body.String())
	}
	if err := json.Unmarshal(response.Body.Bytes(), out); err != nil {
		b.Fatal(err)
	}
}

func waitFleet(b *testing.B, handler http.Handler) []factoryapi.WorkerSessionObservation {
	b.Helper()
	deadline := time.NewTimer(time.Minute)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/worker-sessions?maxResults=200&state=COMPLETED", nil))
		var page factoryapi.ListWorkerSessionsResponse
		if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
			b.Fatal(err)
		}
		if len(page.Sessions) == 200 {
			return page.Sessions
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			b.Fatalf("completed fleet rows=%d", len(page.Sessions))
		case <-b.Context().Done():
			b.Fatal("fixture canceled")
		}
	}
}

func writeFleetFactory(b *testing.B, dir string) {
	b.Helper()
	files := map[string]string{
		"factory.json":                   `{"name":"fleet-load","workTypes":[{"name":"task","states":[{"name":"ready","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],"workers":[{"name":"processor"}],"workstations":[{"name":"process","worker":"processor","inputs":[{"workType":"task","state":"ready"}],"outputs":[{"workType":"task","state":"complete"}],"onFailure":[{"workType":"task","state":"failed"}]}]}`,
		"workers/processor/AGENTS.md":    "---\ntype: MODEL_WORKER\nmodel: gpt-5-codex\nmodelProvider: CODEX\nexecutorProvider: CODEX\nstopToken: COMPLETE\n---\nComplete the fixture Work.\n",
		"workstations/process/AGENTS.md": "---\ntype: MODEL_WORKSTATION\n---\nComplete the fixture Work.\n",
	}
	for name, contents := range files {
		path := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			b.Fatal(err)
		}
	}
}
