// Package catalog defines the parent-private Models catalog service.
package catalog

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// Service serves detached, deterministically ordered discovery values for
// runtime configuration held by the Models Runtime Scopes authority.
type Service interface {
	ListCatalog(context.Context, models.ListModelsRequest) (models.ListModelsResult, error)
	GetCatalogModel(context.Context, models.GetModelRequest) (models.GetModelResult, error)
	GetModelReadiness(context.Context, models.GetModelReadinessRequest) (models.GetModelReadinessResult, error)
}

// ReadinessQuery reads current Models-owned readiness facts for one validated
// catalog model. Inputs and outputs are detached values; implementations must
// honor context cancellation and must not expose runtime or cache handles.
type ReadinessQuery func(
	context.Context,
	models.RuntimeScopeRef,
	models.RuntimeScopeConfig,
	models.Detail,
) (models.Runtime, error)
