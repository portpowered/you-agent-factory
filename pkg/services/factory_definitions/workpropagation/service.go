// Package workpropagation is a transitional re-export surface for Factory
// Definition work-propagation policy. Implementation is owned by nested
// internal/services/invocation_policy/workpropagation; deletion is deferred to DEL packets.
package workpropagation

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicyworkpropagation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/workpropagation"
)

func NewService() factorydefinitions.WorkPropagationPolicyService {
	return invocationpolicyworkpropagation.NewService()
}
