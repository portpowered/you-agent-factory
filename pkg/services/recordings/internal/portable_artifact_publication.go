package internal

import (
	"context"

	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	artifactsexportwire "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/wire"
)

type portableArtifactPublication interface {
	Publish(context.Context, string, []byte) error
	Read(context.Context, string) ([]byte, error)
}

// NewPortableArtifactPublication constructs the private publication effect
// from exact filesystem operations selected by the application graph.
func NewPortableArtifactPublication(
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
	readFile recordings.RecordingReadFile,
) (portableArtifactPublication, error) {
	return artifactsexportwire.NewPublication(
		makeDirectories,
		createTemporaryFile,
		removePath,
		renamePath,
		readFile,
	)
}
