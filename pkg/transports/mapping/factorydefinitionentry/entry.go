// Package factorydefinitionentry publishes the composition-root entry points
// for Factory Definitions representation mapping that non-owning transports
// consume.
//
// The canonical implementations are owned by the Factory Definitions service
// under pkg/services/factory_definitions/transports/mapping. Composition-root
// transports and peer-service transports consume this package instead of
// reaching into that service's transport subpackages directly.
package factorydefinitionentry

import (
	"context"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/validationentry"
	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/transports/mapping/workerinference"
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

// OperationBindingsFromGenerated maps generated workstation operation bindings
// onto the Factory Definitions model operation bindings that worker inference
// reads.
func OperationBindingsFromGenerated(
	values *[]factoryapi.WorkstationOperationBinding,
) []interfaces.ModelOperationBinding {
	return workerinference.OperationBindingsFromGenerated(values)
}
