// Package wire constructs the Recordings artifacts_export subservice.
package wire

import (
	artifactsexport "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export"
	artifactsexportservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/internal/service"
)

// NewService constructs the private artifacts_export capability from the
// lifecycle snapshot seam and publication effect selected by the Recordings root.
func NewService(
	snapshots artifactsexport.SnapshotSource,
	publication artifactsexport.PortableArtifactPublication,
) artifactsexport.Service {
	return artifactsexportservice.New(snapshots, publication)
}

