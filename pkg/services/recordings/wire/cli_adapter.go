package wire

import (
	recordingscli "github.com/portpowered/infinite-you/pkg/services/recordings/transports/cli"
)

// NewCLIAdapter constructs the production Recordings CLI adapter for process
// composition.
func NewCLIAdapter() recordingscli.Adapter {
	return recordingscli.New()
}
