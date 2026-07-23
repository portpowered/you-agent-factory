package acceptance

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
)

// acceptanceScenarioTests lists the focused customer-outcome tests that prove
// the S24 matrix. Keep this map aligned with builtcliacceptance.S24Scenarios().
var acceptanceScenarioTests = map[string]struct{}{
	"TestFreshInstall_EmptyHomeProducesDocumentedCustomerOutcome":                           {},
	"TestMigratedInstall_ExistingConfigIsPreservedWithoutRewrite":                           {},
	"TestProviderPosture_Absent_UnresolvedDefaultRejectsWithDocumentedGuidance":             {},
	"TestProviderPosture_Configured_ExplicitHomeConfigEnablesNamedGoalSuccessPath":          {},
	"TestProviderPosture_Discovered_EnvDefaultResolvesWithoutFileProvider":                  {},
	"TestInvalidGoal_OutputModesExitNonZero":                                                {},
	"TestInvocationOutput_TerminalFailureExitsNonZero":                                      {},
	"TestLocalModelInvoke_MissingReadiness_FailsWithDocumentedBootstrapGuidance":            {},
	"TestGoalRepeat_RepeatedNamedRunsAssignDistinctInvocationIdentityAndReuseInstalledCopy": {},
}

func TestS24ScenarioMatrix_EveryDocumentedScenarioHasFocusedAcceptanceTest(t *testing.T) {
	for _, scenario := range builtcliacceptance.S24Scenarios() {
		scenario := scenario
		t.Run(scenario.ID, func(t *testing.T) {
			t.Parallel()

			if _, ok := acceptanceScenarioTests[scenario.TestName]; !ok {
				t.Fatalf("scenario %s (%s) maps to missing focused acceptance test %q; update acceptanceScenarioTests when adding coverage",
					scenario.ID, scenario.Title, scenario.TestName)
			}
		})
	}
}

func TestS24ScenarioMatrix_FocusedTestsMapOnlyToRegisteredScenarios(t *testing.T) {
	t.Parallel()

	registered := make(map[string]struct{}, len(builtcliacceptance.S24Scenarios()))
	for _, scenario := range builtcliacceptance.S24Scenarios() {
		registered[scenario.TestName] = struct{}{}
	}

	for testName := range acceptanceScenarioTests {
		if _, ok := registered[testName]; !ok {
			t.Fatalf("acceptanceScenarioTests includes unregistered test %q", testName)
		}
	}
	if len(acceptanceScenarioTests) != len(builtcliacceptance.S24Scenarios()) {
		t.Fatalf("acceptanceScenarioTests count = %d, want %d registered S24 scenarios",
			len(acceptanceScenarioTests), len(builtcliacceptance.S24Scenarios()))
	}
}
