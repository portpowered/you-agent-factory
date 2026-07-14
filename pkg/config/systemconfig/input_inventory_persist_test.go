package systemconfig

import (
	"strings"
	"testing"
)

func TestIndexedPersistScopeCases_MatchProductionLoader(t *testing.T) {
	inventory := ProjectInputInventory()
	seen := make(map[string]struct{}, len(inventory.Cases))
	for _, inputCase := range inventory.Cases {
		if inputCase.Entrypoint != entrypointPersistBackendScopeID {
			continue
		}
		if inputCase.ID == "" {
			t.Fatal("input case missing id")
		}
		if _, exists := seen[inputCase.ID]; exists {
			t.Fatalf("duplicate input case id %q", inputCase.ID)
		}
		seen[inputCase.ID] = struct{}{}

		t.Run(inputCase.ID, func(t *testing.T) {
			runPersistScopeCase(t, inputCase)
		})
	}
}

func runPersistScopeCase(t *testing.T, inputCase InputCase) {
	t.Helper()

	configPath := DefaultConfigPath(t.TempDir())
	err := persistBackendScopeID(configPath, inputCase.PersistScopeID)
	if inputCase.Outcome == outcomeAccept {
		if err != nil {
			t.Fatalf("persistBackendScopeID() error = %v, want accept", err)
		}
		return
	}
	if err == nil {
		t.Fatal("persistBackendScopeID() error = nil, want reject")
	}
	for _, fragment := range inputCase.ErrorFragments {
		if !strings.Contains(err.Error(), fragment) {
			t.Fatalf("error = %q, want fragment %q", err.Error(), fragment)
		}
	}
}
