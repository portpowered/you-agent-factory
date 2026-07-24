// Package catalog defines the Factory Definitions-owned private catalog
// capability for list/get/resolve/delete and current-pointer read/write.
// Consumers outside Factory Definitions use the outer Factory Definitions
// root Service instead of this private subservice contract.
package catalog

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

// Service owns deterministic named-Factory catalog operations behind the
// CTR-DEF root catalog slice.
type Service interface {
	ListNamedFactories(context.Context, factorydefinitions.ListNamedFactoriesRequest) (factorydefinitions.ListNamedFactoriesResult, error)
	GetNamedFactory(context.Context, factorydefinitions.GetNamedFactoryRequest) (factorydefinitions.GetNamedFactoryResult, error)
	ResolveNamedFactory(context.Context, factorydefinitions.ResolveNamedFactoryRequest) (factorydefinitions.ResolveNamedFactoryResult, error)
	DeleteNamedFactory(context.Context, factorydefinitions.DeleteNamedFactoryRequest) (factorydefinitions.DeleteNamedFactoryResult, error)
	GetCurrentFactoryPointer(context.Context, factorydefinitions.GetCurrentFactoryPointerRequest) (factorydefinitions.GetCurrentFactoryPointerResult, error)
	SetCurrentFactoryPointer(context.Context, factorydefinitions.SetCurrentFactoryPointerRequest) (factorydefinitions.SetCurrentFactoryPointerResult, error)
}
