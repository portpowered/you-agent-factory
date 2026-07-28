// Package loading provides the transitional Factory Definitions loading surface
// while effective-source construction is owned by the parent-private compilation
// subservice.
package loading

import internalloading "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/compilation/loading"

// Loader coordinates Factory Definition filesystem loading while representation
// parsing remains an adapter selected by Wire.
type Loader = internalloading.Loader

// New constructs the Factory Definitions loader from flat representation and
// filesystem capabilities.
var New = internalloading.New
