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

// TestPackagedGoalRejectRepeatsThenCompletes proves a packaged @you/goal reject
// decision feeds back through the public session invocation API, triggers another
// executor dispatch on the built-in repeater workstation, and then completes with
// the post-reject primary result.
func TestPackagedGoalRejectRepeatsThenCompletes(t *testing.T) {
	dir := scaffoldPackagedGoalBuiltInFactory(t)
	mockWorkersPath, executorCounterPath := writePackagedGoalRejectThenAcceptMockWorkersConfig(t)

	_, response := invokePackagedGoal(t, dir, mockWorkersPath, "invoke packaged goal after reject")
	assertPackagedGoalCompletedWithSummary(t, response, packagedGoalRejectThenCompleteSummary)

	executorInvocations := readPackagedGoalExecutorInvocationCount(t, executorCounterPath)
	if executorInvocations < 2 {
		t.Fatalf("executor invocation count = %d, want at least 2 after reject-then-complete", executorInvocations)
	}
}
