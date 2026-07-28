// Package invocationinterpolation is a transitional re-export surface for Factory
// invocation interpolation policy. Implementation is owned by nested
// internal/services/invocation_policy/invocationinterpolation; deletion is deferred to DEL packets.
package invocationinterpolation

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicyinterpolation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/invocationinterpolation"
)

func NewService() factorydefinitions.InvocationInterpolationService {
	return invocationpolicyinterpolation.NewService()
}

var (
	ValidateInvocationInterpolation = invocationpolicyinterpolation.ValidateInvocationInterpolation
	InterpolateWorkerConfig         = invocationpolicyinterpolation.InterpolateWorkerConfig
	InterpolateWorkstationConfig    = invocationpolicyinterpolation.InterpolateWorkstationConfig
)
