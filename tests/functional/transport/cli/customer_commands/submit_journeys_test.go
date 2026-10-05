package customer_commands_test

import "testing"

// TestCLISubmitJourneys owns its immutable fixture until all customer scenarios finish.
func TestCLISubmitJourneys(t *testing.T) {
	t.Parallel()
	initializeSubmitFixture(t)
	t.Run("TestSubmitInputDependencyAndCleanupMatrix", testSubmitSubmitInputDependencyAndCleanupMatrix)
	t.Run("TestSubmitFamilyExecutesThroughRootBuiltProcess", testSubmitSubmitFamilyExecutesThroughRootBuiltProcess)
	t.Run("TestSubmitFamilyEnqueuesWorkBeforeDownstreamStructuredOutputFailure", testSubmitSubmitFamilyEnqueuesWorkBeforeDownstreamStructuredOutputFailure)
}
