//go:build functionallong

package mock

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestServiceConfigOverrideAlignment_CustomerProcessScriptCommandRunner proves
// customer-process script execution routes through the replaced ScriptCommandRunner
// edge for legacy fixture script workers.
func TestServiceConfigOverrideAlignment_CustomerProcessScriptCommandRunner(t *testing.T) {
	support.SkipLongFunctional(t, "slow service-harness script command-runner alignment sweep")

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "script_executor_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("script harness alignment"))

	runner := support.NewRecordingCommandRunner("script alignment output")
	support.SetWorkingDirectory(t, dir)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--dir", dir, "--no-record",
	})
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		ScriptCommandRunner: runner,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	if err := process.Execute(inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute() error = %v; stdout=%q stderr=%q",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("script command runner calls = %d, want 1", got)
	}
}
