package support_test

import (
	"context"
	"net/http"
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
