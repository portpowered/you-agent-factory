package wire

import (
	"os"

	artifactsexport "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export"
	artifactsexportservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/internal/service"
)

// NewOSPublication constructs the default host publication effect for portable
// artifact export and read.
func NewOSPublication() (artifactsexport.PortableArtifactPublication, error) {
	return artifactsexportservice.NewPublication(
		os.MkdirAll,
		func(dir, pattern string) (artifactsexportservice.PublicationTemporaryFile, error) {
			return os.CreateTemp(dir, pattern)
		},
		os.Remove,
		os.Rename,
		os.ReadFile,
	)
}
