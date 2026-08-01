package resource

import factorycontracts "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/contracts"

type Config = factorycontracts.ResourceConfig

const (
	TypeModel          = factorycontracts.ResourceTypeModel
	TypeProviderQuota  = factorycontracts.ResourceTypeProviderQuota
	TypeInvocationSlot = factorycontracts.ResourceTypeInvocationSlot
)
