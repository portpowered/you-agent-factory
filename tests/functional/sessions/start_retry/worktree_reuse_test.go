package start_retry_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/google/uuid"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const initialOpeningCheckoutName = "customer-retained"

func newInitialOpeningWorktreeScenario(t *testing.T) initialOpeningScenario {
	t.Helper()
	config := initialOpeningFactoryConfig()
	config["workTypes"].([]map[string]any)[0]["handlingBehavior"] = []string{"DEFAULT"}
	config["workstations"].([]map[string]any)[0]["worktree"] = initialOpeningCheckoutName
	scenario := initialOpeningScenarioWithConfig(t, config)
	for _, dir := range []string{scenario.candidateDir, scenario.peerDir} {
		support.WriteAgentConfig(t, dir, "worker-a", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
		prepareInitialOpeningCheckout(t, filepath.Join(dir, ".worktrees", initialOpeningCheckoutName))
	}
	return scenario
}

func prepareInitialOpeningCheckout(t *testing.T, checkout string) {
	t.Helper()
	if err := os.MkdirAll(checkout, 0o755); err != nil {
		t.Fatalf("prepare customer checkout: %v", err)
	}
	// Git itself is a controlled command edge; the selected checkout and its
	// customer-owned content are real, test-owned filesystem destinations.
	for name, content := range map[string]string{
		".git": "gitdir: controlled-customer-checkout\n", "customer.txt": "keep my uncommitted work\n",
	} {
		if err := os.WriteFile(filepath.Join(checkout, name), []byte(content), 0o644); err != nil {
			t.Fatalf("author customer checkout %s: %v", name, err)
		}
	}
}

func initialOpeningCheckoutPath(scenario initialOpeningScenario) string {
	return filepath.Join(scenario.candidateDir, ".worktrees", initialOpeningCheckoutName)
}

func testInitialOpeningWorktreeReuse(t *testing.T, sessions factorysessions.Service, scenario initialOpeningScenario, effects *initialOpeningEffects) {
	t.Helper()
	peerHistory := scenario.startPeer(t, sessions)
	request := scenario.request()
	checkout := initialOpeningCheckoutPath(scenario)
	// The second session uses the same Factory and checkout destination with
	// fresh session/runtime identities, preserving terminal recording identity.
	for generation := 1; generation <= 2; generation++ {
		startInitialOpeningSession(t, sessions, request)
		assertInitialOpeningInvocation(t, sessions, request.SessionID)
		initialOpeningHistory(t, sessions, request.SessionID)
		calls := effects.forScenario(scenario)
		if calls[checkout+"|git.verify"] != generation || calls[checkout+"|worker.codex"] != generation ||
			calls[filepath.Clean(scenario.candidateDir)+"|worker.codex"] != 0 {
			t.Fatalf("checkout generation %d external effects = %v, want validation and Codex execution in selected checkout", generation, calls)
		}
		closeInitialOpeningSession(t, sessions, request.SessionID)
		assertInitialOpeningCheckoutContent(t, checkout)
		if after := effects.forScenario(scenario); !reflect.DeepEqual(after, calls) {
			t.Fatalf("session close changed customer checkout effects: before %v, after %v", calls, after)
		}
		assertInitialOpeningHistoryPreserved(t, sessions, scenario.peerID, peerHistory)
		assertInitialOpeningInvocation(t, sessions, scenario.peerID)
		request.SessionID = uuid.NewString()
		request.RuntimeSelection.RuntimeInstanceID = uuid.NewString()
	}
}

func assertInitialOpeningCheckoutContent(t *testing.T, checkout string) {
	t.Helper()
	for name, expected := range map[string]string{
		".git": "gitdir: controlled-customer-checkout\n", "customer.txt": "keep my uncommitted work\n",
	} {
		content, err := os.ReadFile(filepath.Join(checkout, name))
		if err != nil || string(content) != expected {
			t.Fatalf("customer checkout %s after session close = %q, %v, want %q", name, content, err, expected)
		}
	}
}

type initialOpeningWorktreeGit struct{ effects *initialOpeningEffects }

func (git initialOpeningWorktreeGit) Run(_ context.Context, dir string, args ...string) (string, string, int, error) {
	if filepath.Base(dir) == initialOpeningCheckoutName && reflect.DeepEqual(args, []string{"rev-parse", "--is-inside-work-tree"}) {
		git.effects.record("git.verify", dir)
		return "true\n", "", 0, nil
	}
	git.effects.record("git.unexpected", dir)
	return "", "", 1, fmt.Errorf("unexpected Git effect in %s: %v", dir, args)
}
