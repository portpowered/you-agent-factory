package wire

import (
	"io/fs"

	"github.com/portpowered/infinite-you/pkg/platform/logging"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordedsessioninventory "github.com/portpowered/infinite-you/pkg/services/recordings/internal/services/recorded_session_inventory"
)

// NewRecordedSessionInventory constructs the Recordings-owned, read-only
// dated-history inventory from the exact directory reader and dual-version
// replay-input capability selected by process composition.
func NewRecordedSessionInventory(
	readDir func(string) ([]fs.DirEntry, error),
	replayInputs recordings.ReplayInputLoader,
	logger logging.Logger,
) recordings.RecordedSessionInventory {
	return recordedsessioninventory.New(readDir, replayInputs, logger)
}
