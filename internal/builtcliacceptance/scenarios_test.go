package builtcliacceptance

import (
	"slices"
	"strings"
	"testing"
)

func TestS24Scenarios_RegistryIsCompleteAndUnique(t *testing.T) {
	t.Parallel()

	scenarios := S24Scenarios()
	if len(scenarios) != 9 {
		t.Fatalf("scenario count = %d, want 9 S24 matrix entries", len(scenarios))
	}

	seenIDs := make(map[string]struct{}, len(scenarios))
	seenTests := make(map[string]string, len(scenarios))
	for _, scenario := range scenarios {
		if strings.TrimSpace(scenario.ID) == "" {
			t.Fatalf("scenario missing id: %+v", scenario)
		}
		if strings.TrimSpace(scenario.Title) == "" {
			t.Fatalf("scenario %s missing title", scenario.ID)
		}
		if strings.TrimSpace(scenario.DocumentedOutcome) == "" {
			t.Fatalf("scenario %s missing documented outcome", scenario.ID)
		}
		if strings.TrimSpace(scenario.TestName) == "" {
			t.Fatalf("scenario %s missing test name", scenario.ID)
		}
		if !strings.HasPrefix(scenario.TestName, "Test") {
			t.Fatalf("scenario %s test name %q must start with Test", scenario.ID, scenario.TestName)
		}
		if _, exists := seenIDs[scenario.ID]; exists {
			t.Fatalf("duplicate scenario id %q", scenario.ID)
		}
		seenIDs[scenario.ID] = struct{}{}
		if prior, exists := seenTests[scenario.TestName]; exists {
			t.Fatalf("duplicate test mapping %q for scenarios %q and %q", scenario.TestName, prior, scenario.ID)
		}
		seenTests[scenario.TestName] = scenario.ID
	}

	wantIDs := []string{
		"s24-fresh-install",
		"s24-migrated-install",
		"s24-absent-provider",
		"s24-configured-provider",
		"s24-discovered-provider",
		"s24-invalid-goal",
		"s24-terminal-failure-exit",
		"s24-local-model-invoke",
		"s24-goal-repeat",
	}
	for _, id := range wantIDs {
		if _, ok := seenIDs[id]; !ok {
			t.Fatalf("missing required scenario id %q", id)
		}
	}
}

func TestScenarioByID_LooksUpRegisteredScenario(t *testing.T) {
	t.Parallel()

	scenario := ScenarioByID("s24-fresh-install")
	if scenario == nil {
		t.Fatal("ScenarioByID(s24-fresh-install) = nil, want registered scenario")
	}
	if scenario.TestName != "TestFreshInstall_EmptyHomeProducesDocumentedCustomerOutcome" {
		t.Fatalf("TestName = %q, want fresh-install mapping", scenario.TestName)
	}
	if ScenarioByID("s24-not-real") != nil {
		t.Fatal("ScenarioByID(s24-not-real) should return nil")
	}
}

func TestS24Scenarios_OrderMatchesPriorityMatrix(t *testing.T) {
	t.Parallel()

	ids := make([]string, 0, len(S24Scenarios()))
	for _, scenario := range S24Scenarios() {
		ids = append(ids, scenario.ID)
	}
	wantOrder := []string{
		"s24-fresh-install",
		"s24-migrated-install",
		"s24-absent-provider",
		"s24-configured-provider",
		"s24-discovered-provider",
		"s24-invalid-goal",
		"s24-terminal-failure-exit",
		"s24-local-model-invoke",
		"s24-goal-repeat",
	}
	if !slices.Equal(ids, wantOrder) {
		t.Fatalf("scenario order = %v, want %v", ids, wantOrder)
	}
}
