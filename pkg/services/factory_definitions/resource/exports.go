// Package resource is a transitional compile shim that re-exports the
// catalog-owned Factory resource capacity contracts. Peers should depend on
// factory_definitions contracts; baseline deletion of this path is owned by
// DEL-DEF.
package resource

import (
	catalogresource "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal/services/catalog/resource"
)

type Config = catalogresource.Config

const (
	TypeModel          = catalogresource.TypeModel
	TypeProviderQuota  = catalogresource.TypeProviderQuota
	TypeInvocationSlot = catalogresource.TypeInvocationSlot
)
