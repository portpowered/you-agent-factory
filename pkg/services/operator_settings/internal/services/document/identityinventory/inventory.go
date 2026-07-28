// Package identityinventory documents deterministic inputs for Operator Settings
// document-owned backend identity behavior.
package identityinventory

import operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"

// InputInventoryFormatVersion identifies the system config input inventory shape.
const InputInventoryFormatVersion = "system-config-input/v1"

// InputIndexBaselineRelativePath is the committed system config input index fixture.
const InputIndexBaselineRelativePath = "pkg/services/operator_settings/internal/services/document/identityinventory/testdata/baseline/system-config-input-index.json"

const (
	outcomeAccept = "accept"
	outcomeReject = "reject"

	entrypointEnsureLocalBackendScope = "EnsureLocalBackendScope"
	entrypointPersistBackendScopeID   = "persistBackendScopeID"

	categoryEnsureScope  = "ensure-scope"
	categoryPersistScope = "persist-scope"
)

// InputInventory indexes deterministic system-config inputs and expected loader outcomes.
type InputInventory struct {
	FormatVersion       string      `json:"formatVersion"`
	UnknownFieldPolicy  string      `json:"unknownFieldPolicy"`
	SiblingPreservation string      `json:"siblingPreservation"`
	Cases               []InputCase `json:"cases"`
}

// InputCase records one indexed input and the production loader outcome it documents.
type InputCase struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Entrypoint  string `json:"entrypoint"`
	Outcome     string `json:"outcome"`
	Fixture     string `json:"fixture,omitempty"`
	Description string `json:"description"`

	ExpectedScope            *ScopeExpectation         `json:"expectedScope,omitempty"`
	PersistedFileExpectation *PersistedFileExpectation `json:"persistedFileExpectation,omitempty"`
	PersistScopeID           string                    `json:"persistScopeID,omitempty"`
	ErrorFragments           []string                  `json:"errorFragments,omitempty"`
}

// ScopeExpectation records expected EnsureLocalBackendScope outputs for accepted cases.
type ScopeExpectation struct {
	BackendScopeID   string                               `json:"backendScopeID,omitempty"`
	Outcome          operatorsettings.BackendScopeOutcome `json:"outcome,omitempty"`
	RequireLocalUUID bool                                 `json:"requireLocalUUID,omitempty"`
}

// PersistedFileExpectation records on-disk expectations after ensure or persist.
type PersistedFileExpectation struct {
	BackendScopeIDMatchesResolved bool     `json:"backendScopeIDMatchesResolved,omitempty"`
	PreservesDefaults             bool     `json:"preservesDefaults,omitempty"`
	PreservesSiblingKeys          []string `json:"preservesSiblingKeys,omitempty"`
}
