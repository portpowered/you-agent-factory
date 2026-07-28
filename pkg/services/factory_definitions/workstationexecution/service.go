// Package workstationexecution is a transitional re-export surface for
// Workstation execution-limit policy. Implementation is owned by nested
// internal/services/invocation_policy/workstationexecution; deletion is deferred to DEL packets.
package workstationexecution

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicyworkstationexecution "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/workstationexecution"
)

func NewService() factorydefinitions.WorkstationExecutionPolicyService {
	return invocationpolicyworkstationexecution.NewService()
}

var NormalizeExecutionLimit = invocationpolicyworkstationexecution.NormalizeExecutionLimit
