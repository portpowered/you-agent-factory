package service

import (
	"context"

	modelcatalog "github.com/portpowered/infinite-you/pkg/services/models/internal/catalog"
)

// CatalogAPI is the model-owned discovery seam. Transport composition maps
// these values to generated public contracts at the outward boundary.
type CatalogAPI interface {
	ListModels(context.Context) (modelcatalog.List, error)
	GetModel(context.Context, string) (modelcatalog.Detail, error)
}

var _ CatalogAPI = (*Service)(nil)
