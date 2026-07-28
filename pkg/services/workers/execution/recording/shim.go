// Package recording is a transitional compile shim that re-exports model
// recording helpers from the private workstations destination. Baseline deletion
// of this path is owned by DEL-WRK.
package recording

import (
	privaterecording "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/execution/recording"
)

type Recorder = privaterecording.Recorder

var (
	NewRunner   = privaterecording.NewRunner
	Hooks       = privaterecording.Hooks
	Event       = privaterecording.Event
	Diagnostics = privaterecording.Diagnostics
)
