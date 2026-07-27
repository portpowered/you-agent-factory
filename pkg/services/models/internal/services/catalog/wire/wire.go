// Package wire constructs the private Models Catalog subservice.
package wire

import (
	"context"
	"fmt"

	models "github.com/portpowered/infinite-you/pkg/services/models"
	catalog "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog"
	internalservice "github.com/portpowered/infinite-you/pkg/services/models/internal/services/catalog/internal/service"
	runtimescopes "github.com/portpowered/infinite-you/pkg/services/models/internal/services/runtime_scopes"
)

// NewService constructs an inert catalog over the supplied Runtime Scopes
// authority.
func NewService(
	scopes runtimescopes.Service,
	readinessQueries ...catalog.ReadinessQuery,
) (catalog.Service, error) {
	if scopes == nil {
		return nil, fmt.Errorf("Models Catalog runtime scopes service is required")
	}
	readiness := catalog.ReadinessQuery(catalogReadiness)
	if len(readinessQueries) > 0 {
		readiness = readinessQueries[0]
	}
	if readiness == nil {
		return nil, fmt.Errorf("Models Catalog readiness query is required")
	}
	return internalservice.New(scopes, readiness), nil
}

func catalogReadiness(
	ctx context.Context,
	_ models.RuntimeScopeRef,
	_ models.RuntimeScopeConfig,
	detail models.Detail,
) (models.Runtime, error) {
	if err := ctx.Err(); err != nil {
		return models.Runtime{}, err
	}
	return detail.ManagedRuntime.Clone(), nil
}
