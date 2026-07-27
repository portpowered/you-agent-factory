package service

import (
	artifactsexport "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export"
	artifactsexportwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/wire"
)

// NewPortableArtifactPublication constructs the Wire-selected host publication
// effect for portable artifact export and read.
func NewPortableArtifactPublication() (artifactsexport.PortableArtifactPublication, error) {
	return artifactsexportwire.NewOSPublication()
}
