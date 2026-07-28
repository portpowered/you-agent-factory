// Package invocationworktype is a transitional re-export surface for Factory
// Definition invocation work-type policy. Implementation is owned by nested
// internal/services/invocation_policy/invocationworktype; deletion is deferred to DEL packets.
package invocationworktype

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicyworktype "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationworktype"
)

func NewService() factorydefinitions.InvocationWorkTypeService {
	return invocationpolicyworktype.NewService()
}
