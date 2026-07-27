package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	operatorsettings "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

// providerQuery canonicalizes provider identities through the accepted Providers
// root vocabulary at resolve time without caching availability state.
type providerQuery interface {
	CanonicalizeConcreteProvider(raw string) (string, error)
}

type providersRootQuery struct {
	providers providers.Service
}

func newProvidersRootQuery(root providers.Service) (providerQuery, error) {
	if root == nil {
		return nil, fmt.Errorf("providers root is required")
	}
	return providersRootQuery{providers: root}, nil
}

func (query providersRootQuery) CanonicalizeConcreteProvider(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}

	result, err := query.providers.GetProvider(
		context.Background(),
		providers.GetProviderRequest{ID: providers.ID(trimmed)},
	)
	if err != nil {
		return "", mapProviderCatalogError(err, trimmed)
	}

	canonical := interfaces.PublicWorkerModelProviderFromInternalRuntime(result.Provider.ID.String())
	if canonical == "" {
		return "", operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindUnsupportedOverride,
			Message: trimmed,
			Field:   "workerModelProvider",
		}
	}
	return canonical, nil
}

func mapProviderCatalogError(err error, raw string) error {
	switch {
	case errors.Is(err, providers.ErrUnknownProvider), errors.Is(err, providers.ErrInvalidID):
		return operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindUnsupportedOverride,
			Message: raw,
			Field:   "workerModelProvider",
		}
	case errors.Is(err, providers.ErrProviderUnavailable):
		return operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindConflict,
			Message: raw,
			Field:   "workerModelProvider",
		}
	default:
		return operatorsettings.ResolutionFailure{
			Kind:    operatorsettings.ResolutionFailureKindConflict,
			Message: raw,
			Field:   "workerModelProvider",
		}
	}
}
