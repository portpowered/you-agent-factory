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
}
