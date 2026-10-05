package cli_rest_journeys_test

import "testing"

// TestSessionCLIJourneys owns its immutable fixture until all customer scenarios finish.
func TestSessionCLIJourneys(t *testing.T) {
	t.Parallel()
	resetsessionscli0State()
	initializeSessionscliFixture(t)
	t.Run("TestRemotePauseRejectionRetainsSelectedTargetAndError", testSessionscliRemotePauseRejectionRetainsSelectedTargetAndError)
	t.Run("TestBuildProcessRoutesEverySessionLeafThroughResolvedProductionComposition", testSessionscliBuildProcessRoutesEverySessionLeafThroughResolvedProductionComposition)
	t.Run("TestBuildProcessRejectsDeprecatedPortBeforeSubmitDispatch", testSessionscliBuildProcessRejectsDeprecatedPortBeforeSubmitDispatch)
}
