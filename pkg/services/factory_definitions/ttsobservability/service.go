// Package ttsobservability is a transitional re-export surface for packaged
// TTS observability policy. Implementation is owned by nested
// internal/services/invocation_policy/ttsobservability; deletion is deferred to DEL packets.
package ttsobservability

import (
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	invocationpolicytts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/invocation_policy/ttsobservability"
)

func NewService() factorydefinitions.TTSObservabilityService {
	return invocationpolicytts.NewService()
}

var (
	IsPackagedTTSFactory       = invocationpolicytts.IsPackagedTTSFactory
	TTSBackendRuntimeLabel     = invocationpolicytts.TTSBackendRuntimeLabel
	ClassifyTTSInvocationWait  = invocationpolicytts.ClassifyTTSInvocationWait
	IsTTSModelNotReadyFailure   = invocationpolicytts.IsTTSModelNotReadyFailure
)
