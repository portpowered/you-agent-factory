package customer_commands_test

import "testing"

// TestCLIShellCompletionJourneys owns its immutable fixture until all customer scenarios finish.
func TestCLIShellCompletionJourneys(t *testing.T) {
	t.Parallel()
	initializeShellcompletionFixture(t)
	t.Run("TestCompletionFailureBoundariesAndReuse", testShellcompletionCompletionFailureBoundariesAndReuse)
	t.Run("TestGeneratedCompletionScriptsReachRootProcess", testShellcompletionGeneratedCompletionScriptsReachRootProcess)
}
