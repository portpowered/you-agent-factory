package wire

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
)

// NewSnapshotRootService constructs the private state_access subservice for
// snapshot-backed reads when no live session adapter is available. Composition
// selects which owner supplies the reader.
func NewSnapshotRootService(snapshots stateaccess.SnapshotReader) stateaccess.Service {
	return NewService(nilSessionResolver{}, snapshots)
}

// ListWorkFromSnapshots exercises snapshot-backed Work list reads when no live
// session adapter is available.
func ListWorkFromSnapshots(
	ctx context.Context,
	sessionID string,
	snapshots stateaccess.SnapshotReader,
	options work.ListOptions,
) (work.ListResult, error) {
	return NewSnapshotRootService(snapshots).ListWork(ctx, sessionID, options)
}

// GetWorkFromSnapshots exercises snapshot-backed Work get reads when no live
// session adapter is available.
func GetWorkFromSnapshots(
	ctx context.Context,
	sessionID string,
	workID string,
	snapshots stateaccess.SnapshotReader,
) (work.ReadModel, error) {
	return NewSnapshotRootService(snapshots).GetWork(ctx, sessionID, workID)
}

type nilSessionResolver struct{}

func (nilSessionResolver) ResolveSessionAdapter(string) (stateaccess.SessionAdapter, error) {
	return nil, nil
}
