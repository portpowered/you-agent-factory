package service

import (
	sessionruntime "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtime"
)

// NewDefinitionActivationGatewayForTest publishes the activation gateway backed by
// the supplied session state for unit tests.
func NewDefinitionActivationGatewayForTest(state *sessionruntime.Service) *definitionActivationGateway {
	return NewDefinitionActivationGateway(&SessionRuntime{sessionState: state})
}
