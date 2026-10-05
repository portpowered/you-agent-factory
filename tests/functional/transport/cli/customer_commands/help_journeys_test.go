package customer_commands_test

import "testing"

// TestCLIHelpJourneys owns its immutable fixture until all customer scenarios finish.
func TestCLIHelpJourneys(t *testing.T) {
	t.Parallel()
	initializeHelpFixture(t)
	t.Run("TestCLIRunHelpShowsInvocationSignatureForNamedFactory", testHelpCLIRunHelpShowsInvocationSignatureForNamedFactory)
	t.Run("TestCLIRunHelpDistinguishesRequiredAndOptionalParameters", testHelpCLIRunHelpDistinguishesRequiredAndOptionalParameters)
	t.Run("TestCLIRunHelpDoesNotDispatchExternalWork", testHelpCLIRunHelpDoesNotDispatchExternalWork)
	t.Run("TestCLISessionHelpPublishesRunnablePlacementExamples", testHelpCLISessionHelpPublishesRunnablePlacementExamples)
	t.Run("TestCLIRunHelpCoversGenericAndExplicitFactorySelections", testHelpCLIRunHelpCoversGenericAndExplicitFactorySelections)
	t.Run("TestCLIRunHelpResetsEmptyAndInvalidSelections", testHelpCLIRunHelpResetsEmptyAndInvalidSelections)
}
