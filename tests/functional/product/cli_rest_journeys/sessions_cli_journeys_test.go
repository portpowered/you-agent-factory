package cli_rest_journeys_test

import "testing"

// Session commands preserve their selected customer target.
func runSessionCLIJourneys(t *testing.T) {
	t.Parallel()
	t.Run("TestRemotePauseRejectionRetainsSelectedTargetAndError", testSessionscliRemotePauseRejectionRetainsSelectedTargetAndError)
	t.Run("TestSessionCommandsPreserveSelectedTargets", testSessionscliCommandsPreserveSelectedTargets)
}
