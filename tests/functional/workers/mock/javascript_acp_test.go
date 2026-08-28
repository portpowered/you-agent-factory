package mock

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected keeps the
// MockWorkers ownership proof with the Workers mock cell. The counted ACP
// command edge and provider command runner make a regression observable while
// ensuring this compatibility route never reaches a live provider.
func TestJavaScriptMockWorkersRemainFakeWhenACPProviderIsSelected(t *testing.T) {
	dir := writeMockJavaScriptACPFactory(t)
	support.SetWorkingDirectory(t, dir)

	var acpStarts atomic.Int32
	providerRunner := support.NewRecordingCommandRunner("live provider route was unexpectedly invoked")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", "./acp.js", "--with-mock-workers", "--no-record",
	})
	inputs.Input.WorkingDirectory = dir
	inputs.Input.Env = os.Environ()
	process := support.BuildProcess(t, serviceedges.Edges{
		PlatformProcessCommandFactory: mockACPCommandFactory(&acpStarts),
		ProvidersExecutableLocator:    mockACPExecutableLocator{},
		ProviderCommandRunner:         providerRunner,
	})
	support.CleanupProcess(t, process)

	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(mock JavaScript Factory) error = %v\nstdout:\n%s\nstderr:\n%s", err, inputs.Stdout(), inputs.Stderr())
	}
	if acpStarts.Load() != 0 || providerRunner.CallCount() != 0 {
		t.Fatalf("mock execution started live providers ACP=%d providerRunner=%d, want zero live calls", acpStarts.Load(), providerRunner.CallCount())
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

func mockACPCommandFactory(starts *atomic.Int32) platformprocess.CommandFactory {
	return func(string, ...string) *exec.Cmd {
		starts.Add(1)
		// If MockWorkers stops intercepting the ACP child, the no-test command
		// exits immediately and the counted edge makes the failure explicit.
		return exec.Command(os.Args[0], "-test.run=^$")
	}
}

type mockACPExecutableLocator struct{}

func (mockACPExecutableLocator) LookPath(file string) (string, error) {
	return file, nil
}
