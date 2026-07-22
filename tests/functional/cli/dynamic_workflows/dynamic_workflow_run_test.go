package session

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	"github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// THIS IS THE CANONICAL ROUGH SHAPE OF HOW INJECTION AND SERVICE RUNNING IS INTENDED TO BE SHAPED.
func TestRunJavaScriptFactoryWithMockWorkersUsesFakeChildExecutor(t *testing.T) {
	// Arrange
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dynamic"))
	support.SetWorkingDirectory(t, dir)

	runner := support.NewRecordingCommandRunner("unexpected live provider execution")
	fakeEnv := support.FakeInputs(t.Context(), []string{
		"you", "run", "--factory", "./basic.js", "--with-mock-workers",
	})

	// Act
	process, err := root.BuildProcess(t.Context(), edges.Edges{
		ProviderCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	err = process.Execute(fakeEnv.Input)

	// Assert
	if err != nil {
		t.Fatalf("Process.Execute() error = %v; stdout=%q stderr=%q", err, fakeEnv.Stdout(), fakeEnv.Stderr())
	}
	if !strings.Contains(fakeEnv.Stdout(), " completed (SUCCEEDED).") {
		t.Fatalf("stdout = %q, want successful Factory Session", fakeEnv.Stdout())
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for fake child execution", runner.CallCount())
	}
}
