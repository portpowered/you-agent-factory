package wire

import (
	"context"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimewire "github.com/portpowered/infinite-you/pkg/services/factory_runtime/wire"
	factorysessionwire "github.com/portpowered/infinite-you/pkg/services/factory_sessions/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// provideFactoryRuntimeRootFactory composes the singular process-scoped
// Runtime root once. Factory Sessions supplies the explicit activation
// operation when it constructs its opening capability; no late delegate
// binder is exposed.
func provideFactoryRuntimeRootFactory(
	newID factoryruntime.IDGenerator,
	workflows factoryruntime.JavaScriptWorkflowDefinitions,
	clock factoryruntime.Clock,
) factorysessionwire.RuntimeRootFactory {
	return func(activation factoryruntime.RuntimeActivationOperation) (factoryruntime.Root, error) {
		return factoryruntimewire.NewService(
			newID,
			workflows,
			nil,
			clock,
			func(context.Context, workers.WorkstationDispatchRequest) error {
				return factoryruntime.ErrNotRunning
			},
			nil,
			activation,
		)
	}
}
