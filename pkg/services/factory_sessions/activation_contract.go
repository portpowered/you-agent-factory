package factorysessions

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

// DefinitionActivationGateway is the Factory Sessions-owned activation edge for
// definition save, activate, and swap paths. The canonical interface definition
// lives in factory_definitions so Definitions peers can consume the gateway
// without importing Factory Sessions.
type DefinitionActivationGateway = factorydefinitions.DefinitionActivationGateway

// DefinitionActivationGatewayProvider exposes the Sessions-owned activation
// gateway for Factory Definitions construction without the attach-capable
// SessionHost bundle.
type DefinitionActivationGatewayProvider interface {
	DefinitionActivationGateway() DefinitionActivationGateway
}
