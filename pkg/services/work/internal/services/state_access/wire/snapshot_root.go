package wire

import (
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
)

// NewSnapshotRootService constructs the private state_access subservice for
// snapshot-backed reads when no live session adapter is available. Composition
// selects which owner supplies the reader.
func NewSnapshotRootService(snapshots stateaccess.SnapshotReader) stateaccess.Service {
	return NewService(nilSessionResolver{}, snapshots)
}

type nilSessionResolver struct{}

func (nilSessionResolver) ResolveSessionAdapter(string) (stateaccess.SessionAdapter, error) {
	return nil, nil
}
