package internal

import (
	"context"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	artifactsexportwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/wire"
)

// PortableArtifactPublication is the private publication capability selected
// by the application graph for completed artifact bytes.
type PortableArtifactPublication interface {
	Publish(context.Context, string, []byte) error
	Read(context.Context, string) ([]byte, error)
}

// NewPortableArtifactPublication constructs the private publication capability
// from exact filesystem effects supplied by the application graph.
func NewPortableArtifactPublication(
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
	readFile recordings.RecordingReadFile,
) (PortableArtifactPublication, error) {
	return artifactsexportwire.NewPublication(
		makeDirectories,
		createTemporaryFile,
		removePath,
		renamePath,
		readFile,
	)
}
