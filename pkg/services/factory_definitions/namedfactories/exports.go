// Package namedfactories is a transitional compile shim that re-exports the
// catalog-owned named-factory catalog implementation. Peers should depend on
// factory_definitions contracts; baseline deletion of this path is owned by
// DEL-DEF.
package namedfactories

import (
	internalnamedfactories "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/namedfactories"
)

// New constructs the stateless persisted named-Factory catalog.
var New = internalnamedfactories.New

// ResolveCurrent resolves the active Factory definition under rootDir.
var ResolveCurrent = internalnamedfactories.ResolveCurrent
