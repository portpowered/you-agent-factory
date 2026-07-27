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
	// ResolveRuntimeCache returns the Models-private filesystem layout required
	// by the existing local runtime. Peers outside Models receive only detached
	// root-contract asset facts.
	ResolveRuntimeCache(
		context.Context,
		models.InspectModelAssetsRequest,
	) (RuntimeCacheLayout, error)
	// InspectRuntimeCache returns Models-private cache readiness without
	// contacting the configured source.
	InspectRuntimeCache(
		context.Context,
		models.InspectModelAssetsRequest,
	) (RuntimeCacheInspection, error)
}

// RuntimeCacheLayout is the Models-private bridge from verified scoped assets
// to the existing local runtime process boundary.
type RuntimeCacheLayout struct {
	ModelName string
	CachePath string
	Revision  string
	Files     []string
}

// RuntimeCacheInspection is the Models-private cache projection used by
// catalog and host compatibility behavior during their separate migrations.
type RuntimeCacheInspection struct {
	Supported          bool
	Installed          bool
	Revision           string
	CachePath          string
	InstalledFileCount int
	MissingAssets      []string
	PartialArtifacts   bool
}
