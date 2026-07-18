package support_test

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestRootRunFunctionalHostStartsThroughCustomerRESTAndSSE(t *testing.T) {
	t.Parallel()

	factoryRoot := writeRootRunHostFactory(t)
	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot: factoryRoot,
		SystemRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}

	status, err := host.REST().GetStatus(context.Background())
	if err != nil {
		t.Fatalf("generated GetStatus() error = %v", err)
	}
	if status.StatusCode() != http.StatusOK || status.JSON200 == nil {
		t.Fatalf("generated GetStatus() response = %#v, want typed 200", status)
	}

	streamCtx, cancelStream := context.WithCancel(context.Background())
	stream, err := host.OpenFactoryEvents(streamCtx, factorysessions.DefaultSessionID, nil)
	if err != nil {
		cancelStream()
		t.Fatalf("OpenFactoryEvents() error = %v", err)
	}
	if got := stream.Header.Get("Content-Type"); !strings.HasPrefix(got, "text/event-stream") {
		cancelStream()
		_ = stream.Body.Close()
		t.Fatalf("Factory Event stream content type = %q, want text/event-stream", got)
	}
	cancelStream()
	_ = stream.Body.Close()

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	result, err := host.Shutdown(shutdownCtx)
	if err != nil {
		t.Fatalf("Shutdown() error = %v", err)
	}
	if result.ExitCode != root.ExitSuccess {
		t.Fatalf("canceled root.Run exit code = %d, want %d", result.ExitCode, root.ExitSuccess)
	}
	if result.Outcome != support.RootRunProcessStopped {
		t.Fatalf("canceled root.Run outcome = %q, want %q", result.Outcome, support.RootRunProcessStopped)
	}
}

func TestRootRunFunctionalHostContextCancellationCompletesAndReleasesListener(t *testing.T) {
	t.Parallel()

	hostCtx, cancelHost := context.WithCancel(context.Background())
	host, err := support.StartRootRunFunctionalHost(hostCtx, support.RootRunFunctionalHostConfig{
		FactoryRoot: writeRootRunHostFactory(t),
		SystemRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}
	endpoint, err := url.Parse(host.Endpoint())
	if err != nil {
		t.Fatalf("parse host endpoint: %v", err)
	}

	cancelHost()
	waitCtx, cancelWait := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelWait()
	select {
	case <-host.Done():
	case <-waitCtx.Done():
		t.Fatalf("root.Run process completion: %v", waitCtx.Err())
	}
	result, finished := host.Result()
	if !finished {
		t.Fatal("Result() reports unfinished after Done() closed")
	}
	if result.ExitCode != root.ExitSuccess || result.Outcome != support.RootRunProcessStopped {
		t.Fatalf("Result() = %#v, want clean stopped outcome", result)
	}

	listener, err := net.Listen("tcp", endpoint.Host)
	if err != nil {
		t.Fatalf("listen on released host address %s: %v", endpoint.Host, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close replacement listener: %v", err)
	}
}

func TestRootRunFunctionalHostShutdownIsBoundedAndIdempotent(t *testing.T) {
	t.Parallel()

	host, err := support.StartRootRunFunctionalHost(context.Background(), support.RootRunFunctionalHostConfig{
		FactoryRoot: writeRootRunHostFactory(t),
		SystemRoot:  t.TempDir(),
	})
	if err != nil {
		t.Fatalf("StartRootRunFunctionalHost() error = %v", err)
	}

	expiredCtx, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancelExpired()
	_, err = host.Shutdown(expiredCtx)
	if err == nil {
		t.Fatal("Shutdown(expired context) error = nil")
	}
	for _, diagnostic := range []string{
		host.Endpoint(),
		"context deadline exceeded",
		"last readiness=\"generated GET /status succeeded with HTTP 200\"",
		"process=",
	} {
		if !strings.Contains(err.Error(), diagnostic) {
			t.Fatalf("Shutdown(expired context) error = %q, want %q", err, diagnostic)
		}
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	first, err := host.Shutdown(shutdownCtx)
	cancelShutdown()
	if err != nil {
		t.Fatalf("first joined Shutdown() error = %v", err)
	}
	repeatCtx, cancelRepeat := context.WithTimeout(context.Background(), 5*time.Second)
	second, err := host.Shutdown(repeatCtx)
	cancelRepeat()
	if err != nil {
		t.Fatalf("repeated Shutdown() error = %v", err)
	}
	if first != second {
		t.Fatalf("repeated Shutdown() result = %#v, want %#v", second, first)
	}
	if first.ExitCode != root.ExitSuccess || first.Outcome != support.RootRunProcessStopped {
		t.Fatalf("Shutdown() result = %#v, want clean stopped outcome", first)
	}
}

func writeRootRunHostFactory(t *testing.T) string {
	t.Helper()
	rootDir := t.TempDir()
	factoryJSON := `{
  "name": "root-run-host",
  "workTypes": [{
    "name": "task",
    "states": [
      {"name": "init", "type": "INITIAL"},
      {"name": "complete", "type": "TERMINAL"},
      {"name": "failed", "type": "FAILED"}
    ]
  }],
  "workers": [{"name": "worker"}],
  "workstations": [{
    "name": "process",
    "worker": "worker",
    "inputs": [{"workType": "task", "state": "init"}],
    "outputs": [{"workType": "task", "state": "complete"}],
    "onFailure": [{"workType": "task", "state": "failed"}]
  }]
}`
	writeRootRunHostFile(t, filepath.Join(rootDir, "factory.json"), factoryJSON)
	workerAgents := `---
type: AGENT_WORKER
model: fixture-model
modelProvider: CLAUDE
executorProvider: SCRIPT_WRAP
---
`
	workstationAgents := `---
type: AGENT_RUN
limits:
  maxExecutionTime: 1m
stopWords:
  - DONE
---

Return DONE.
`
	writeRootRunHostFile(t, filepath.Join(rootDir, "workers", "worker", "AGENTS.md"), workerAgents)
	writeRootRunHostFile(t, filepath.Join(rootDir, "workstations", "process", "AGENTS.md"), workstationAgents)
	return rootDir
}

func writeRootRunHostFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
}
