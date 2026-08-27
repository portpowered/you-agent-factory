package claude

import (
	"context"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

const (
	claudeConductorModel          = "claude-sonnet-4-5-20250514"
	claudeConductorRunTimeout     = 20 * time.Second
	claudeConductorProcessCommand = "claude"
	claudeCancellationMessage     = "provider invocation was canceled"
)

// TestClaudeDefaultLaneSharedProcess proves the four ordinary Claude scenarios
// through one root-built process. Each subtest owns a
// separate Factory directory and opens an explicit non-default Factory Session
// so the process is shared while runtime state remains session-scoped.
func TestClaudeDefaultLaneSharedProcess(t *testing.T) {
	fixture := newClaudeDefaultLaneFixture(t)
	t.Cleanup(func() {
		fixture.assertSharedIdentityLedger(t)
	})

	for _, scenario := range fixture.scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			t.Parallel()
			fixture.runScenario(t, scenario)
		})
	}
	t.Cleanup(func() {
		fixture.assertSharedProcessCleanup(t)
	})
}

// TestClaudeCommandRouterFailsClosed proves that the package-local command
// edge cannot silently fall back to another scenario when its immutable
// selector is absent or duplicated.
func TestClaudeCommandRouterFailsClosed(t *testing.T) {
	first := &claudeScenarioCommandRunner{}
	second := &claudeScenarioCommandRunner{}
	duplicate, err := newClaudeCommandRouter([]claudeCommandRoute{
		{selector: "duplicate-selector", runner: first},
		{selector: "duplicate-selector", runner: second},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate Claude scenario selector") {
		t.Fatalf("duplicate route construction error = %v, want fail-closed duplicate selector error", err)
	}
	if duplicate != nil {
		t.Fatal("duplicate route construction returned a usable router")
	}

	router, err := newClaudeCommandRouter([]claudeCommandRoute{
		{selector: "known-selector", runner: first},
	})
	if err != nil {
		t.Fatalf("newClaudeCommandRouter: %v", err)
	}
	_, err = router.Run(context.Background(), platformprocess.CommandRequest{
		Command: claudeConductorProcessCommand,
		WorkDir: "unknown-selector",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown Claude scenario selector") {
		t.Fatalf("unknown route error = %v, want fail-closed selector error", err)
	}
	if got := first.CallCount(); got != 0 {
		t.Fatalf("known route calls after unknown selector = %d, want 0", got)
	}
}
