package workers_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// The standalone selector is a JavaScript source file, rather than a JSON
// Factory containing JavaScript orchestration. Observe the actual public route.
func runJavaScriptStandaloneFile(t *testing.T, fixture *javascriptSharedProcessFixture) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	const marker = "standalone file child"
	runner := &runtimeJavaScriptCommandRunner{marker: marker, started: make(chan struct{}), release: make(chan struct{})}
	runner.unblock()
	if err := fixture.router.register(marker, runner); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fixture.router.unregister(marker); err != nil {
			t.Error(err)
		}
	})
	peerMarker := "standalone file live peer"
	peer := &runtimeJavaScriptCommandRunner{marker: peerMarker, started: make(chan struct{}), release: make(chan struct{})}
	if err := fixture.router.register(peerMarker, peer); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		peer.unblock()
		if err := fixture.router.unregister(peerMarker); err != nil {
			t.Error(err)
		}
	})
	results := make(chan concurrentJavaScriptResult, 1)
	go func() {
		workflow := strings.ReplaceAll(liveProviderChildWorkflow, "use the live provider command edge", peerMarker)
		response, err := postOverridesWorkflow(ctx, fixture.baseURL, "standalone-file-peer", workflow)
		results <- concurrentJavaScriptResult{response: response, err: err}
	}()
	select {
	case <-peer.started:
	case result := <-results:
		t.Fatalf("peer returned before admission: %+v", result)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	runStandaloneJavaScriptFile(t, fixture, ctx, marker, runner)
	select {
	case result := <-results:
		t.Fatalf("standalone file completed its gated peer: %+v", result)
	default:
	}
	peer.unblock()
	result, err := awaitConcurrentJavaScriptResult(ctx, results, "standalone file peer")
	if err != nil {
		t.Fatal(err)
	}
	assertRuntimeJavaScriptChild(t, fixture, result, peerMarker, marker)
}

func runStandaloneJavaScriptFile(t *testing.T, fixture *javascriptSharedProcessFixture, ctx context.Context, marker string, runner *runtimeJavaScriptCommandRunner) {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "workflow.js")
	workflow := strings.ReplaceAll(liveProviderChildWorkflow, "use the live provider command edge", marker)
	if err := os.WriteFile(source, []byte(workflow), 0600); err != nil {
		t.Fatal(err)
	}
	inputs := support.FakeInputs(ctx, []string{"you", "--json", "run", "--factory", source})
	inputs.Input.Env = append([]string(nil), fixture.environment...)
	inputs.Input.WorkingDirectory = dir
	if err := fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("standalone file: %v stdout=%s stderr=%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if !strings.Contains(inputs.Stdout(), marker+" output") || runner.calls.Load() != 1 {
		t.Fatalf("standalone output=%s calls=%d", inputs.Stdout(), runner.calls.Load())
	}
	var response factoryapi.FactorySessionSyncExecutionResponse
	if err := json.Unmarshal([]byte(inputs.Stdout()), &response); err != nil {
		t.Fatal(err)
	}
	if response.ResolvedSource.Kind != "WORKFLOW_FILE" || response.ResolvedSource.SourceRef == nil || *response.ResolvedSource.SourceRef != "workflow.js" {
		t.Fatalf("standalone source: %+v", response.ResolvedSource)
	}
	result := concurrentJavaScriptResult{response: response}
	assertRuntimeJavaScriptChild(t, fixture, result, marker, "standalone file live peer")
	t.Logf("F16-09 disposition required: documented --factory workflow.js succeeds with scoped supervised Worker under Factory %s; no bypass PASS", response.SessionId)
}
