package claude

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Claude timeout retries are retryable; queue one transcript result per provider
// invocation so the scenario route cannot fall back to an unrelated result.
const claudeGoldenTimeoutCommandInvocations = 9

func loadClaudeGoldenCase(t *testing.T, caseName string) support.ProviderSessionCase {
	t.Helper()

	repoRoot := testutil.MustRepoRoot(t)
	caseDir := filepath.Join(repoRoot, filepath.FromSlash(support.ProviderSessionFixturePath("claude", caseName)))
	loaded, err := support.LoadProviderSessionCase(caseDir)
	if err != nil {
		t.Fatalf("LoadProviderSessionCase(%q): %v", caseName, err)
	}
	return loaded
}

func marshalProviderSessionGoldenJSON(value any) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage("null"), nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func assertProviderSessionGoldensMatch(
	t *testing.T,
	loaded support.ProviderSessionCase,
	observed support.ProviderSessionObservedGoldens,
) {
	t.Helper()

	if err := support.CompareOrUpdateProviderSessionGoldens(loaded, observed); err != nil {
		t.Fatalf("CompareOrUpdateProviderSessionGoldens: %v", err)
	}
}
