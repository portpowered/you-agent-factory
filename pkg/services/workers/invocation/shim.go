// Package invocation is a transitional compile shim that re-exports the
// workstations invocation adapter from the private destination. Peers should
// construct through workers/wire; baseline deletion of this path is owned by
// DEL-WRK.
package invocation

import (
	private "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/invocation"
)

type Executor = private.Executor

var (
	NewExecutor         = private.NewExecutor
	NewProviderExecutor = private.NewProviderExecutor
)
