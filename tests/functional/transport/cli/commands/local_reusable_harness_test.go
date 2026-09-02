package commands_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

// newLocalReusableProcessHarness gives a command group one reusable
// production root process for serialized functional CLI invocations.
func newLocalReusableProcessHarness(t *testing.T) *builtcliacceptance.Harness {
	t.Helper()
	harness := builtcliacceptance.NewReusableHarness(t, testutil.MustRepoRoot(t))
	harness.DefaultEnv = builtcliacceptance.ProcessEnvForIsolatedHome(t.TempDir())
	return harness
}

// newLocalConcurrentProcessHarness gives independent command scenarios one
// reusable production root process without imposing a package-wide command
// gate. Each invocation must select isolated state through its own inputs or
// explicit Factory Session.
func newLocalConcurrentProcessHarness(t *testing.T) *builtcliacceptance.Harness {
	t.Helper()
	harness := builtcliacceptance.NewConcurrentReusableHarness(t, testutil.MustRepoRoot(t))
	harness.DefaultEnv = builtcliacceptance.ProcessEnvForIsolatedHome(t.TempDir())
	return harness
}
