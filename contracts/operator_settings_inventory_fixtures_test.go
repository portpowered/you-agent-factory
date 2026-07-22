package contracts_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
)

const (
	globalConfigTopologyBaselinePath = "pkg/services/operator_settings/globalconfiginventory/testdata/baseline/global-config-topology.json"
	identityInputBaselinePath        = "pkg/services/operator_settings/identityinventory/testdata/baseline/system-config-input-index.json"
	identityFixtureDirectory         = "pkg/services/operator_settings/identityinventory/testdata/fixtures"
)

// These representation-only structures deliberately belong to the schema
// contract tests. Operator Settings owns projection and canonical encoding;
// contracts consume only the committed JSON artifact that owner tests guard.
type committedGlobalConfigInventoryDocument struct {
	PrecedenceChain    string                               `json:"precedenceChain"`
	SharedFileSplit    committedGlobalConfigSharedFileSplit `json:"sharedFileSplit"`
	UnknownFieldPolicy []committedGlobalConfigUnknownPolicy `json:"unknownFieldPolicy"`
	Fields             []committedGlobalConfigField         `json:"fields"`
}

type committedGlobalConfigSharedFileSplit struct {
	Summary string                           `json:"summary"`
	Owners  []committedGlobalConfigFileOwner `json:"owners"`
}

type committedGlobalConfigFileOwner struct {
	Package    string   `json:"package"`
	Owns       []string `json:"owns"`
	Tolerates  []string `json:"tolerates,omitempty"`
	DoesNotOwn []string `json:"doesNotOwn,omitempty"`
}

type committedGlobalConfigUnknownPolicy struct {
	Package string `json:"package"`
	Policy  string `json:"policy"`
}

type committedGlobalConfigField struct {
	ID                   string   `json:"id"`
	JSONPath             string   `json:"jsonPath"`
	DefaultEmptyBehavior string   `json:"defaultEmptyBehavior"`
	Strictness           string   `json:"strictness"`
	PersistenceOwner     string   `json:"persistenceOwner"`
	ParseOwner           string   `json:"parseOwner"`
	PrecedenceLayers     []string `json:"precedenceLayers,omitempty"`
	EnvironmentVariable  string   `json:"environmentVariable,omitempty"`
	FlagName             string   `json:"flagName,omitempty"`
	Notes                string   `json:"notes,omitempty"`
}

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

func committedGlobalConfigInventory(t *testing.T) committedGlobalConfigInventoryDocument {
	t.Helper()
	return readCommittedOperatorInventory[committedGlobalConfigInventoryDocument](
		t,
		globalConfigTopologyBaselinePath,
	)
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
