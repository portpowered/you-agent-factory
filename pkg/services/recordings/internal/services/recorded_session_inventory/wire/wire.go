package wire

import (
	"io/fs"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordedsessioninventoryservice "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recorded_session_inventory/internal/service"
)

// New constructs the private history-inventory implementation for the
// Recordings composition boundary.
func New(
	readDir func(string) ([]fs.DirEntry, error),
	replayInputs recordings.ReplayInputLoader,
	logger logging.Logger,
) recordings.RecordedSessionInventory {
	return recordedsessioninventoryservice.New(readDir, replayInputs, logger)
}
