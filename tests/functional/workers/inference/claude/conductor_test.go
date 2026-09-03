package claude

import (
	"testing"
	"time"
)

const (
	claudeConductorModel          = "claude-sonnet-4-5-20250514"
	claudeConductorRunTimeout     = 20 * time.Second
	claudeConductorProcessCommand = "claude"
	claudeCancellationMessage     = "provider invocation was canceled"
)

// TestClaudeDefaultLaneSharedProcess proves the ordinary Claude scenarios and
// same-process recovery through one root-built process. Each subtest owns a
// separate Factory directory and opens an explicit non-default Factory Session
// so the process is shared while runtime state remains session-scoped.
func TestClaudeDefaultLaneSharedProcess(t *testing.T) {
	t.Parallel()
	fixture := newClaudeDefaultLaneFixture(t)
	t.Cleanup(func() {
		fixture.assertSharedIdentityLedger(t)
	})

	t.Run("ConcurrentDefaultScenarios", func(t *testing.T) {
		for _, scenario := range fixture.defaultScenarios {
			scenario := scenario
			t.Run(scenario.name, func(t *testing.T) {
				t.Parallel()
				fixture.runScenario(t, scenario)
			})
		}
	})
	t.Run("SameProcessRecoveryAfterAdverseSession", func(t *testing.T) {
		runClaudeSameProcessRecoveryAfterAdverseSession(t, fixture)
	})
	t.Cleanup(func() {
		fixture.assertSharedProcessCleanup(t)
	})
}

// runClaudeSameProcessRecoveryAfterAdverseSession proves that a canceled
// explicit Factory Session can be deleted before a fresh explicit session
// succeeds on the same root-built process. Its scenarios use distinct routes
// so the concurrent default cases and this ordered probe remain independently
// observable while sharing the production process.
func runClaudeSameProcessRecoveryAfterAdverseSession(t *testing.T, fixture *claudeDefaultLaneFixture) {
	t.Helper()
	if len(fixture.recoveryScenarios) != 2 {
		t.Fatalf("recovery scenarios = %d, want cancellation and fresh success", len(fixture.recoveryScenarios))
	}
	if !t.Run("Cancellation", func(t *testing.T) {
		fixture.runScenario(t, fixture.recoveryScenarios[0])
	}) {
		t.Fatal("cancellation recovery prerequisite failed")
	}
	if !t.Run("FreshSuccess", func(t *testing.T) {
		fixture.runScenario(t, fixture.recoveryScenarios[1])
	}) {
		t.Fatal("fresh success recovery probe failed")
	}

	if got := fixture.apiStarts.Load(); got != 1 {
		t.Fatalf("recovery API server starts = %d, want exactly one shared process server", got)
	}
}

func claudeScenarioNamed(t *testing.T, scenarios []claudeScenario, name string) claudeScenario {
	t.Helper()
	for _, scenario := range scenarios {
		if scenario.name == name {
			return scenario
		}
	}
	t.Fatalf("Claude scenario %q is not configured", name)
	return claudeScenario{}
}
