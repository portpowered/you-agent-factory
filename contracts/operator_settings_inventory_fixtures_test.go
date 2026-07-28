package contracts_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	identityInputBaselinePath = "pkg/services/operator_settings/internal/services/document/identityinventory/testdata/baseline/system-config-input-index.json"
	identityFixtureDirectory  = "pkg/services/operator_settings/internal/services/document/identityinventory/testdata/fixtures"
)

type committedIdentityInputInventoryDocument struct {
	SiblingPreservation string                       `json:"siblingPreservation"`
	Cases               []committedIdentityInputCase `json:"cases"`
}

type committedIdentityInputCase struct {
	ID                       string                                     `json:"id"`
	Entrypoint               string                                     `json:"entrypoint"`
	Outcome                  string                                     `json:"outcome"`
	Fixture                  string                                     `json:"fixture,omitempty"`
	ExpectedScope            *committedIdentityScopeExpectation         `json:"expectedScope,omitempty"`
	PersistedFileExpectation *committedIdentityPersistedFileExpectation `json:"persistedFileExpectation,omitempty"`
	ErrorFragments           []string                                   `json:"errorFragments,omitempty"`
}

type committedIdentityScopeExpectation struct {
	BackendScopeID   string `json:"backendScopeID,omitempty"`
	Outcome          string `json:"outcome,omitempty"`
	RequireLocalUUID bool   `json:"requireLocalUUID,omitempty"`
}

type committedIdentityPersistedFileExpectation struct {
	PreservesDefaults    bool     `json:"preservesDefaults,omitempty"`
	PreservesSiblingKeys []string `json:"preservesSiblingKeys,omitempty"`
}

func committedIdentityInputInventory(t *testing.T) committedIdentityInputInventoryDocument {
	t.Helper()
	return readCommittedOperatorInventory[committedIdentityInputInventoryDocument](
		t,
		identityInputBaselinePath,
	)
}

func readCommittedOperatorInventory[T any](t *testing.T, relativePath string) T {
	t.Helper()
	path := testutil.MustRepoPath(t, relativePath)
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read committed Operator Settings inventory %s: %v", path, err)
	}
	var document T
	if err := json.Unmarshal(payload, &document); err != nil {
		t.Fatalf("decode committed Operator Settings inventory %s: %v", path, err)
	}
	return document
}
