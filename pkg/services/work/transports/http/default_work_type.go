package http

import (
	"context"
	"errors"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	factoryconfigmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

// NewDefaultWorkTypeResolver binds Work admission's omitted-type policy to the
// current Factory selected for a Factory Session. The policy remains in the
// Work HTTP adapter because it exists solely to prepare Work requests.
func NewDefaultWorkTypeResolver(
	definitions apisurface.FactorySaveAPI,
	invocationWorkType factorydefinitions.InvocationWorkTypeService,
) func(context.Context, string) (string, error) {
	return func(ctx context.Context, sessionID string) (string, error) {
		if definitions == nil || invocationWorkType == nil {
			return "", nil
		}
		namedFactory, err := definitions.GetCurrentFactoryForSession(ctx, sessionID)
		if err != nil {
			if errors.Is(err, apisurface.ErrFactorySessionNotFound) || errors.Is(err, apisurface.ErrCurrentFactoryNotFound) {
				return "", nil
			}
			return "", err
		}
		factoryConfig, err := factoryconfigmapping.FactoryConfigFromOpenAPI(namedFactory)
		if err != nil {
			return "", err
		}
		defaultID, err := invocationWorkType.DefaultWorkType(&factoryConfig)
		if err != nil {
			return "", nil
		}
		return defaultID, nil
	}
}
