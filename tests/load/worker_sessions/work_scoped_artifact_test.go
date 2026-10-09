package workersessions_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	platformhttp "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// Dedicated performance lane: <=553 Work, 3,304 retained attempts, one seed
// process with controlled Codex and one delivered server. The invoking build
// supplies the artifact; setup never reads the scoped list. Six measured reads
// cover first/subsequent HTTP plus real CLI across default and resolved scopes.
func TestWorkScopedRetainedSessionsLatency(t *testing.T) {
	binary := os.Getenv("INFINITE_YOU_INTEGRATION_BINARY")
	if binary == "" {
		if os.Getenv("INFINITE_YOU_REQUIRE_PREBUILT_ARTIFACT") == "1" {
			t.Fatal("invoking build must supply INFINITE_YOU_INTEGRATION_BINARY")
		}
		t.Skip("requires prebuilt CLI/server artifact")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("artifact path must be absolute")
	}
	artifact, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("artifact=%s sha256=%x", binary, sha256.Sum256(artifact))
	ctx, cancel := context.WithTimeout(t.Context(), 9*time.Minute)
	defer cancel()
	f := scopedLatencyFixture{dir: t.TempDir()}
	f.environment = builtcliacceptance.ProcessEnvForIsolatedHome(filepath.Join(f.dir, "home"))
	record := filepath.Join(f.dir, "retained.json")
	started := time.Now()
	if !t.Run("prepare", func(t *testing.T) { seedRetainedArtifact(t, ctx, &f, record) }) {
		t.Fatal("retained preparation failed")
	}
	t.Logf("setup=%s Work=553 retained=3304 matching=4", time.Since(started))
	started = time.Now()
	f.serverURL = startRetainedArtifact(t, ctx, binary, f, record)
	t.Logf("delivered startup=%s endpoint=%s", time.Since(started), f.serverURL)
	var session factoryapi.FactorySession
	raw, _, _ := scopedLatencyHTTP(t, ctx, f.serverURL+"/factory-sessions/~default")
	if err := json.Unmarshal(raw, &session); err != nil || session.Id == "" {
		t.Fatalf("resolved session: %v %s", err, raw)
	}
	f.sessionID = session.Id
	var expected any
	for _, scope := range []string{"~default", f.sessionID} {
		for sample := range 2 {
			path := "/factory-sessions/" + scope + "/worker-sessions?workId=" + url.QueryEscape(f.workID)
			raw, elapsed, headers := scopedLatencyHTTP(t, ctx, f.serverURL+path)
			value, ids := retainedArtifactRows(t, raw, f.workID)
			if expected != nil && !reflect.DeepEqual(expected, value) {
				t.Fatal("ordered rows changed across reads/scopes")
			}
			expected = value
			t.Logf("HTTP request=GET %s scope=%s sample=%d first=%t status=200 bytes=%d IDs=%v elapsed=%s headers=%s", path, scope, sample, scope == "~default" && sample == 0, len(raw), ids, elapsed, headers)
			if elapsed >= 2*time.Second {
				t.Errorf("scoped HTTP exceeded strict 2s contract: %s", elapsed)
			}
		}
		assertRetainedArtifactCLI(t, ctx, binary, f, scope, expected)
	}
}

func seedRetainedArtifact(t *testing.T, ctx context.Context, f *scopedLatencyFixture, record string) {
	t.Helper()
	writeRetainedFactory(t, f.dir)
	ready := make(chan *httptest.Server, 1)
	var store recordings.WorkerRecordingStore
	process, err := root.BuildProcess(ctx, edges.Edges{
		ProviderCommandRunner:           &scopedLatencyRunner{},
		FactorySessionsWorkingDirectory: platformfilesystem.Local{WorkingDirectory: f.dir},
		WorkerRecordingStoreObserver:    func(value recordings.WorkerRecordingStore) { store = value },
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
	// Retain a concrete source scope in canonical events. The delivered runtime
	// binds this history to its own default scope; a historical literal ~default
	// cannot identify the original capture owner after that binding changes.
	f.sessionID = uuid.NewString()
	server := startScopedLatencyHost(t, ctx, process, *f, ready, "--session", f.sessionID, "--record", record)
	var session factoryapi.FactorySession
	raw, _, _ := scopedLatencyHTTP(t, ctx, server.URL+"/factory-sessions/"+f.sessionID)
	if err := json.Unmarshal(raw, &session); err != nil || session.Id == "" {
		t.Fatalf("seed session: %v %s", err, raw)
	}
	f.sessionID = session.Id
	f.workID = submitRetainedWork(t, server.Config.Handler, f.sessionID, "target")
	var pending []string
	pending = append(pending, f.workID)
	for range 550 {
		pending = append(pending, submitRetainedWork(t, server.Config.Handler, f.sessionID, "task"))
	}
	// Two known empty Works complete the representative board without adding
	// attempts. Neither readiness nor inventory warms the measured list.
	for range 2 {
		submitRetainedWork(t, server.Config.Handler, f.sessionID, "idle")
	}
	waitRetainedWorks(t, ctx, server.Config.Handler, f.sessionID, pending)
	assertRetainedInventory(t, ctx, store, f.sessionID, 3304)
}

func startRetainedArtifact(t *testing.T, ctx context.Context, binary string, f scopedLatencyFixture, record string) string {
	t.Helper()
	port, err := builtcliacceptance.ReserveLocalTCPPort()
	if err != nil || port == 7437 {
		t.Fatalf("owned non-7437 port: %d %v", port, err)
	}
	address := fmt.Sprintf("127.0.0.1:%d", port)
	baseURL := "http://" + address
	command := exec.CommandContext(ctx, binary, "run", "--dir", f.dir, "--resume", record, "--record", filepath.Join(f.dir, "successor.json"), "--continuously", "--with-server", "--listen", address, "--quiet")
	command.Dir, command.Env = f.dir, f.environment
	logPath := filepath.Join(f.dir, "delivered.log")
	log, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	command.Stdout, command.Stderr = log, log
	if err := command.Start(); err != nil {
		_ = log.Close()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	t.Cleanup(func() {
		_ = command.Process.Kill()
		select {
		case <-done:
		case <-time.After(5 * time.Second): //nolint:testsleep // Failure ceiling; actual OS exit comes from Wait.
			t.Error("delivered server failed to join")
		}
		_ = log.Close()
	})
	waitRetainedArtifactReady(t, ctx, baseURL, logPath, done)
	return baseURL
}

func waitRetainedArtifactReady(t *testing.T, ctx context.Context, baseURL, logPath string, done chan error) {
	t.Helper()
	client := &http.Client{Timeout: 250 * time.Millisecond}
	defer client.CloseIdleConnections()
	// OS startup has no injected bound callback. Observe public Factory
	// readiness only; polling must never warm the scoped Worker Session route.
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/factory-sessions/~default/factory", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := client.Do(request)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		select {
		case err := <-done:
			done <- err
			data, _ := os.ReadFile(logPath)
			t.Fatalf("delivered server exited before readiness: %v %s", err, data)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-ticker.C:
		}
	}
}

func retainedArtifactRows(t *testing.T, raw []byte, workID string) (any, []string) {
	t.Helper()
	response := httptest.NewRecorder()
	response.Code = http.StatusOK
	_, _ = response.Body.Write(raw)
	ids := assertRetainedRead(t, response, workID, 4)
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return withoutScopedNulls(value), ids
}

func assertRetainedArtifactCLI(t *testing.T, ctx context.Context, binary string, f scopedLatencyFixture, scope string, expected any) {
	t.Helper()
	command := exec.CommandContext(ctx, binary, "--server", f.serverURL, "worker-sessions", "list", "--session", scope, "--work-id", f.workID, "--output", "json")
	command.Dir, command.Env = f.dir, f.environment
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	started := time.Now()
	if err := command.Run(); err != nil {
		t.Fatalf("compiled scoped CLI: %v stderr=%s", err, stderr.String())
	}
	elapsed := time.Since(started)
	value, ids := retainedArtifactRows(t, stdout.Bytes(), f.workID)
	if !reflect.DeepEqual(expected, value) {
		t.Fatal("compiled CLI/HTTP ordered rows differ")
	}
	t.Logf("CLI scope=%s exit=0 bytes=%d IDs=%v elapsed=%s", scope, stdout.Len(), ids, elapsed)
}
