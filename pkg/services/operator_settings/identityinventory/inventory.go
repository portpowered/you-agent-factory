// Package identityinventory is a transitional shim over document-owned identity
// input inventory. Implementation lives under internal/services/document.
package identityinventory

import (
	documentidentityinventory "github.com/portpowered/infinite-you/pkg/services/operator_settings/internal/services/document/identityinventory"
)

// InputInventoryFormatVersion identifies the system config input inventory shape.
const InputInventoryFormatVersion = documentidentityinventory.InputInventoryFormatVersion

// InputIndexBaselineRelativePath is the committed system config input index fixture.
const InputIndexBaselineRelativePath = documentidentityinventory.InputIndexBaselineRelativePath

// InputInventory indexes deterministic system-config inputs and expected loader outcomes.
type InputInventory = documentidentityinventory.InputInventory

// InputCase records one indexed input and the production loader outcome it documents.
type InputCase = documentidentityinventory.InputCase

// ScopeExpectation records expected EnsureLocalBackendScope outputs for accepted cases.
type ScopeExpectation = documentidentityinventory.ScopeExpectation

// PersistedFileExpectation records on-disk expectations after ensure or persist.
type PersistedFileExpectation = documentidentityinventory.PersistedFileExpectation
