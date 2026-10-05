package customer_lifecycles_test

import "testing"

// TestWorkRoutingJourneys owns its immutable fixture until all customer scenarios finish.
func TestWorkRoutingJourneys(t *testing.T) {
	t.Parallel()
	resetworkrouting4State()
	initializeWorkroutingFixture(t)
	t.Run("TestSharedProcessWorkRouting", testWorkroutingSharedProcessWorkRouting)
}
