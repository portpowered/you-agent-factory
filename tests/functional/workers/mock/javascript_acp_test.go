package mock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// testJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected keeps the
// MockWorkers ownership proof with the shared Workers mock process. It runs
// after the session-backed rows have stopped the hosted invocation, so the
// same root can execute this one-shot JavaScript compatibility path without a
// second application graph. MockWorkers intercepts the ACP child before any
// provider or process edge is reached.
func testJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected(
	t *testing.T,
	fixture *sharedWorkersMockFixture,
) {
	dir := writeMockJavaScriptACPFactory(t)
	providerRunner := support.NewRecordingCommandRunner("live provider route was unexpectedly invoked")
	fixture.prepareLocalActivation(t)
	fixture.useCommandRunners(providerRunner, nil)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", filepath.Join(dir, "acp.js"), "--with-mock-workers", "--no-record",
	})
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Env = sharedWorkersMockEnvironment(t, writeSharedWorkersMockOperatorHome(t))

	if err := fixture.executeLocal(t, inputs.Input); err != nil {
		t.Fatalf("Process.Execute(mock JavaScript Factory) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if providerRunner.CallCount() != 0 {
		t.Fatalf("mock execution reached the provider runner %d times, want zero live calls", providerRunner.CallCount())
	}
	if !strings.Contains(inputs.Stdout(), " completed (SUCCEEDED).") {
		t.Fatalf("mock JavaScript Factory did not succeed: %s", inputs.Stdout())
	}
}

func writeMockJavaScriptACPFactory(t *testing.T) string {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dynamic"))
	workflow := `return (async function () {
  const child = await agent.run({
    prompt: "complete the JavaScript ACP child",
    label: "javascript-acp",
    executorProvider: "ACP",
    modelProvider: "cursor-acp",
    model: "test-model",
  });
  return child;
})();`
	if err := os.WriteFile(filepath.Join(dir, "acp.js"), []byte(workflow), 0o600); err != nil {
		t.Fatalf("write mock JavaScript ACP Factory: %v", err)
	}
	return dir
}
