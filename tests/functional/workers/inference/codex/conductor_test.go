package codex

import (
	"context"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
)

const (
	codexConductorModel          = "gpt-5.3-codex-spark"
	codexConductorProcessCommand = "codex"
	codexConductorRunTimeout     = 20 * time.Second
	codexCancellationMessage     = "provider invocation was canceled"
)

// TestCodexDefaultLaneSharedProcess proves conductor success and cancellation
// through one root-built process. Each subtest owns a separate Factory
// directory and opens an explicit non-default Factory Session so process
// wiring is shared while runtime state remains session-scoped.
func TestCodexDefaultLaneSharedProcess(t *testing.T) {
	fixture := newCodexConductorFixture(t)
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

// TestCodexCommandRouterFailsClosed proves that the package-local command edge
// cannot silently fall back to another scenario when its immutable selector is
// absent or duplicated.
func TestCodexCommandRouterFailsClosed(t *testing.T) {
	first := &codexScenarioCommandRunner{}
	second := &codexScenarioCommandRunner{}
	duplicate, err := newCodexCommandRouter([]codexCommandRoute{
		{selector: "duplicate-selector", runner: first},
		{selector: "duplicate-selector", runner: second},
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate Codex scenario selector") {
		t.Fatalf("duplicate route construction error = %v, want fail-closed duplicate selector error", err)
	}
	if duplicate != nil {
		t.Fatal("duplicate route construction returned a usable router")
	}

	router, err := newCodexCommandRouter([]codexCommandRoute{
		{selector: "known-selector", runner: first},
	})
	if err != nil {
		t.Fatalf("newCodexCommandRouter: %v", err)
	}
	_, err = router.Run(context.Background(), platformprocess.CommandRequest{
		Command: codexConductorProcessCommand,
		WorkDir: "unknown-selector",
	})
	if err == nil || !strings.Contains(err.Error(), "unknown Codex scenario selector") {
		t.Fatalf("unknown route error = %v, want fail-closed selector error", err)
	}
	if got := first.CallCount(); got != 0 {
		t.Fatalf("known route calls after unknown selector = %d, want 0", got)
	}
}

func containsArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}

func containsArgPair(args []string, flag, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == flag && args[index+1] == value {
			return true
		}
	}
	return false
}
