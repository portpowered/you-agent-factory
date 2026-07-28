// Package persistence is a transitional compile shim that re-exports the
// catalog-owned Factory Definitions persistence implementation. Peers should
// depend on factory_definitions contracts; baseline deletion of this path is
// owned by DEL-DEF.
package persistence

import (
	catalogpersistence "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/persistence"
)

// New constructs the Factory Definitions persistence implementation from flat
// serialization and filesystem capabilities selected by Wire.
var New = catalogpersistence.New
