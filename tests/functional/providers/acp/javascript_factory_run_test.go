package acp_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestJavaScriptFactoryAgentRunRoutesExecutorProviderThroughACP(t *testing.T) {
	dir := writeACPJavaScriptFactory(t)
	support.SetWorkingDirectory(t, dir)
	t.Setenv(acpHelperEnvironment, "1")

	var starts atomic.Int32
	legacyRunner := support.NewRecordingCommandRunner("legacy provider route was unexpectedly invoked")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--factory", "./acp.js", "--no-record",
	})
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Env = os.Environ()
	err := support.BuildProcess(t, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts),
		ProvidersExecutableLocator:    availableExecutableLocator{},
		ProviderCommandRunner:         legacyRunner,
	}).Execute(inputs.Input)
	if err != nil {
		t.Fatalf("Process.Execute(JavaScript ACP Factory) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if starts.Load() != 1 {
		t.Fatalf("ACP process starts = %d, want 1", starts.Load())
	}
	if legacyRunner.CallCount() != 0 {
		t.Fatalf("legacy provider runner calls = %d, want 0", legacyRunner.CallCount())
	}
	if !strings.Contains(inputs.Stdout(), "ACP root execution COMPLETE") || !strings.Contains(inputs.Stdout(), `"providerSessionRef":"acp-session-functional-1"`) {
		t.Fatalf("JavaScript ACP result omitted content/session evidence: %s", inputs.Stdout())
	}
}

func TestJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected(t *testing.T) {
	dir := writeACPJavaScriptFactory(t)
	support.SetWorkingDirectory(t, dir)

	var starts atomic.Int32
	legacyRunner := support.NewRecordingCommandRunner("live provider route was unexpectedly invoked")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", "./acp.js", "--with-mock-workers", "--no-record",
	})
	inputs.Input.WorkingDirectory = dir
	err := support.BuildProcess(t, serviceedges.Edges{
		PlatformProcessCommandFactory: acpHelperCommandFactory(&starts),
		ProvidersExecutableLocator:    availableExecutableLocator{},
		ProviderCommandRunner:         legacyRunner,
	}).Execute(inputs.Input)
	if err != nil {
		t.Fatalf("Process.Execute(mock JavaScript Factory) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if starts.Load() != 0 || legacyRunner.CallCount() != 0 {
		t.Fatalf("mock execution started ACP=%d legacy=%d, want zero live calls", starts.Load(), legacyRunner.CallCount())
	}
	if !strings.Contains(inputs.Stdout(), " completed (SUCCEEDED).") {
		t.Fatalf("mock JavaScript Factory did not succeed: %s", inputs.Stdout())
	}
}

func writeACPJavaScriptFactory(t *testing.T) string {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dynamic"))
	workflow := `return (async function () {
  const child = await agent.run({
    prompt: "complete the JavaScript ACP child",
    label: "javascript-acp",
    executorProvider: "cursor-acp",
    model: "test-model",
  });
  return child;
})();`
	if err := os.WriteFile(filepath.Join(dir, "acp.js"), []byte(workflow), 0o600); err != nil {
		t.Fatalf("write ACP JavaScript Factory: %v", err)
	}
	return dir
}
