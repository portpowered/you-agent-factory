package customer_lifecycles_test

import "testing"

// TestPackagedClassificationJourneys owns its immutable fixture until all customer scenarios finish.
func TestPackagedClassificationJourneys(t *testing.T) {
	t.Parallel()
	resetfactorypackagedclassify2State()
	initializeFactorypackagedclassifyFixture(t)
	t.Run("TestPackagedClassifyRoutesSmallMediumAndLarge", testFactorypackagedclassifyPackagedClassifyRoutesSmallMediumAndLarge)
	t.Run("TestPackagedClassifyInvalidLabelFailsWithoutExecutingLane", testFactorypackagedclassifyPackagedClassifyInvalidLabelFailsWithoutExecutingLane)
	t.Run("TestPackagedClassifyRoleProviderAndModelSelections", testFactorypackagedclassifyPackagedClassifyRoleProviderAndModelSelections)
}
