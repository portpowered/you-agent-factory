package cli_rest_journeys_test

import "testing"

// TestFactoryYAMLParityJourneys owns its immutable fixture until all customer scenarios finish.
func TestFactoryYAMLParityJourneys(t *testing.T) {
	t.Parallel()
	resetfactorydefinitionstransportscliyamlparity1State()
	initializeFactorydefinitionstransportscliyamlparityFixture(t)
	t.Run("TestCLIFactoryJSONAndYAMLValidateFlattenAndRunParity", testFactorydefinitionstransportscliyamlparityCLIFactoryJSONAndYAMLValidateFlattenAndRunParity)
}
