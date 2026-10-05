package cli_rest_journeys_test

import "testing"

// TestWorkCLIJourneys owns its immutable fixture until all customer scenarios finish.
func TestWorkCLIJourneys(t *testing.T) {
	t.Parallel()
	resetworktransportscli1State()
	initializeWorktransportscliFixture(t)
	t.Run("TestWorkListFiltersAndCounts", testWorktransportscliWorkListFiltersAndCounts)
	t.Run("TestWorkListPublicCLITraversesThreeRESTPages", testWorktransportscliWorkListPublicCLITraversesThreeRESTPages)
	t.Run("TestWorkShowPublicCLILooksUpWorkBeyondFirstRESTPage", testWorktransportscliWorkShowPublicCLILooksUpWorkBeyondFirstRESTPage)
	t.Run("TestWorkListAllAnnotatesSupersededSameName", testWorktransportscliWorkListAllAnnotatesSupersededSameName)
}
