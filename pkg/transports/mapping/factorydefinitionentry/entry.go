// Package factorydefinitionentry publishes the composition-root entry points
// for Factory Definitions representation mapping that non-owning transports
// consume.
//
// The canonical implementations are owned by the Factory Definitions service
// under pkg/services/factory_definitions/transports/mapping. Composition-root
// transports and repository-wide test support consume this package instead of
// reaching into that service's transport subpackages directly.
package factorydefinitionentry

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/factorysnapshot"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/validationentry"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

// ValidateFactoryAPI maps one public validation request and invokes the exact
// Factory Definitions validation operation. The operation owns its fixed
// topology profile; HTTP and CLI callers cannot select validation phases.
func ValidateFactoryAPI(
	ctx context.Context,
	factory factoryapi.Factory,
	operation interfaces.SubmittedDefinitionValidationOperation,
) (interfaces.ValidationResult, error) {
	return validationentry.ValidateFactoryAPI(ctx, factory, operation)
}

// ObjectFromFactoryConfig maps an authored Factory Definition through the
// generated public contract at the transport boundary.
func ObjectFromFactoryConfig(
	factoryConfig *interfaces.FactoryConfig,
) (map[string]any, error) {
	return factorysnapshot.ObjectFromFactoryConfig(factoryConfig)
}
