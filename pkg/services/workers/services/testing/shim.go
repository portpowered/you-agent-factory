// Package mockworker is a transitional compile shim that re-exports mock-worker
// test helpers from the private runners destination. Peers should construct
// through workers/wire; baseline deletion of this path is owned by DEL-WRK.
package mockworker

import (
	runnermockworker "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/testing"
)

type (
	MockWorkerCommandRunner = runnermockworker.MockWorkerCommandRunner
)
