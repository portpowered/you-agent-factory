package wire

import (
	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
)

// NewCLIAdapter constructs the production Recordings CLI adapter for process
// composition.
func NewCLIAdapter(paths platformfilesystem.PathInspector) recordingscli.Adapter {
	return recordingscli.New(paths)
}
