package commands_test

import "testing"

// newLocalReusableProcessHarness gives a command group one reusable
// production root process for serialized functional CLI invocations.
func newLocalReusableProcessHarness(t *testing.T) *commandRuntime {
	t.Helper()
	if commandPackageRuntime == nil {
		t.Fatal("command package runtime was not initialized")
	}
	return commandPackageRuntime
}
