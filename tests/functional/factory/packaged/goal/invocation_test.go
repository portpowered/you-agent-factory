package goal

import (
	"testing"
)

// TestPackagedGoalAcceptCompletesWithSummary proves packaged @you/goal invocation
// completes through the public session invocation API with an explicit summary
// primary result distinct from the submitted goal text.
func TestPackagedGoalAcceptCompletesWithSummary(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
	mockWorkersPath := writePackagedGoalBuiltinMockWorkersConfig(t)

	submitted := "customer goal request text"
	response := postPackagedGoalInvocation(t, dir, mockWorkersPath, submitted)
	assertPackagedGoalCompletedWithSummary(t, response, packagedGoalMockWorkerAcceptedSummary)
	if primaryResultText(t, response) == submitted {
		t.Fatal("primaryResult echoed submitted goal text")
	}
}
