// Package assets defines the parent-private Models asset service.
package assets

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// Service resolves scoped asset sources, prepares verified revisions, and
// reports detached cache facts. Pulling, verification, and publication remain
// private implementation details.
type Service interface {
	PrepareModelAssets(
		context.Context,
		models.PrepareModelAssetsRequest,
	) (models.PrepareModelAssetsResult, error)
	InspectModelAssets(
		context.Context,
		models.InspectModelAssetsRequest,
	) (models.InspectModelAssetsResult, error)
}
