// Package editable is a transitional compile shim that re-exports the
// snapshots_portability-owned editable snapshot validation implementation.
// Peers should depend on factory_definitions contracts; baseline deletion of
// this path is owned by DEL-DEF.
package editable

import (
	internaleditable "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/editable"
)

// ValidateSnapshot applies the canonical pre-persist rules to one detached
// Factory definition.
var ValidateSnapshot = internaleditable.ValidateSnapshot
