// Package skippermissions is a transitional compile shim that re-exports
// skip-permissions policy helpers from the private workstations destination.
// Peers should construct through workers/wire; baseline deletion of this path
// is owned by DEL-WRK.
package skippermissions

import (
	private "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/skippermissions"
)

var (
	EffectiveSkipPermissions                = private.EffectiveSkipPermissions
	AgentWorkerSupportsSkipPermissions      = private.AgentWorkerSupportsSkipPermissions
	ValidateInvocationSkipPermissionsForWorker = private.ValidateInvocationSkipPermissionsForWorker
	ValidateInvocationSkipPermissionsWorkers  = private.ValidateInvocationSkipPermissionsWorkers
)
