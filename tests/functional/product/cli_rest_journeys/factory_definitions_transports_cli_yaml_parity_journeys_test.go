package cli_rest_journeys_test

import "testing"

// runFactoryYAMLParityJourneys owns its immutable fixture until all customer scenarios finish.
func runFactoryYAMLParityJourneys(t *testing.T) {
	t.Parallel()
	t.Run("TestCLIFactoryJSONAndYAMLValidateFlattenAndRunParity", testFactorydefinitionstransportscliyamlparityCLIFactoryJSONAndYAMLValidateFlattenAndRunParity)
}
