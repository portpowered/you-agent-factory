// Package assets defines the parent-private Models asset service.
package assets

import (
	"context"

	models "github.com/portpowered/infinite-you/pkg/services/models"
)

// Service resolves scoped asset sources and reports detached cache facts.
// Pulling, verification, and removal remain private implementation details as
// those operations are added to this service.
type Service interface {
	InspectModelAssets(
		context.Context,
		models.InspectModelAssetsRequest,
	) (models.InspectModelAssetsResult, error)
}
