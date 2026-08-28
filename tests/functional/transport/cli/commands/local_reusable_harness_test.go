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
	return builtcliacceptance.NewReusableHarness(t, testutil.MustRepoRoot(t))
}
