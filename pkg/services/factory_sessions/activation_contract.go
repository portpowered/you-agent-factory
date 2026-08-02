package factorysessions

import factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"

// DefinitionActivationGateway is retained as the Sessions implementation's
// view of the gateway contract, whose authoritative definition belongs to
// Factory Definitions. It is a value contract, not a second Sessions service
// authority.
type DefinitionActivationGateway = factorydefinitions.DefinitionActivationGateway
