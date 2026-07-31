package internal

import (
	"context"

	artifactsexportwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/wire"
)

type portableArtifactPublication interface {
	Publish(context.Context, string, []byte) error
	Read(context.Context, string) ([]byte, error)
}

// NewPortableArtifactPublication constructs the Wire-selected host publication
// effect for portable artifact export and read.
func NewPortableArtifactPublication() (portableArtifactPublication, error) {
	return artifactsexportwire.NewOSPublication()
}
