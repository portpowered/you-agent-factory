package commands_test

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
)

// newLocalReusableProcessHarness gives local and self-hosted command groups a
// single production root process while leaving the remote command helper
// owned by the later shared-session story.
func newLocalReusableProcessHarness(t *testing.T) *builtcliacceptance.Harness {
	t.Helper()
	return builtcliacceptance.NewReusableHarness(t, testutil.MustRepoRoot(t))
}
