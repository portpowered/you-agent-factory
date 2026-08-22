package wire

import (
	recordings "github.com/portpowered/infinite-you/pkg/services/recordings"
	artifactsexportservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/artifacts_export/internal/service"
)

// NewPublication constructs the private publication capability from exact
// filesystem effects selected by the application graph.
func NewPublication(
	makeDirectories recordings.RecordingMakeDirectories,
	createTemporaryFile recordings.RecordingCreateTemporaryFile,
	removePath recordings.RecordingRemovePath,
	renamePath recordings.RecordingRenamePath,
	readFile recordings.RecordingReadFile,
) (artifactsexportservice.PortableArtifactPublication, error) {
	var createFile func(string, string) (artifactsexportservice.PublicationTemporaryFile, error)
	if createTemporaryFile != nil {
		createFile = func(dir, pattern string) (artifactsexportservice.PublicationTemporaryFile, error) {
			file, err := createTemporaryFile(dir, pattern)
			return file, err
		}
	}
	publication, err := artifactsexportservice.NewPublication(
		makeDirectories,
		createFile,
		removePath,
		renamePath,
		readFile,
	)
	if err != nil {
		return nil, err
	}
	return publication, nil
}
