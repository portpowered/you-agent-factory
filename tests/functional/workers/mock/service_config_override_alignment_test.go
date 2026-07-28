package mock

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestServiceConfigOverrideAlignment_CustomerProcessSharesScriptAndProviderCommandRunner
// proves customer-process script and provider dispatches route through one shared
// replaced command-runner edge with correct per-workstation env and work dirs.
func TestServiceConfigOverrideAlignment_CustomerProcessSharesScriptAndProviderCommandRunner(t *testing.T) {
	dir := scaffoldSharedCommandRunnerFactory(t)
	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: []byte("script-output")},
		platformprocess.CommandResult{
			Stdout: []byte("provider-output COMPLETE"),
			Stderr: []byte(`{"event":"session.created","session_id":"sess_mixed_command"}`),
		},
	)
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "--json", "run", "--dir", dir, "--no-record",
	})
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		ProviderCommandRunner: runner,
		ScriptCommandRunner:   runner,
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

	requests := runner.Requests()
	if len(requests) != 2 {
		t.Fatalf("shared command runner request count = %d, want 2", len(requests))
	}

	assertSharedCommandRunnerScriptRequest(t, dir, requests[0])
	assertSharedCommandRunnerProviderRequest(t, dir, requests[1])
}
