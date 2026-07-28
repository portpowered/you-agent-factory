// Package replayconfig is a transitional compile shim that re-exports the
// snapshots_portability-owned replay runtime-config reconstruction
// implementation. Peers should depend on factory_definitions contracts;
// baseline deletion of this path is owned by DEL-DEF.
package replayconfig

import (
	internalreplayconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/snapshots_portability/replayconfig"
)

// Decode reconstructs a detached runtime lookup using the representation
// decoder selected by Wire.
var Decode = internalreplayconfig.Decode
