// Package stateaccessrecordings exposes the leased Work state_access
// Recordings-backed read edge for functional verification without importing
// Work internal packages from tests/functional.
package stateaccessrecordings

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
	stateaccesswire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/wire"
)

func newRecordingsRootService(root recordings.Service) stateaccess.Service {
	return stateaccesswire.NewService(
		nilSessionResolver{},
		stateaccesswire.NewRecordingsAdapter(root),
	)
}

// ListWorkFromRecordingsRoot exercises Recordings-backed Work list reads through
// the published Recordings service root when no live session adapter is available.
func ListWorkFromRecordingsRoot(
	ctx context.Context,
	sessionID string,
	root recordings.Service,
	options work.ListOptions,
) (work.ListResult, error) {
	return newRecordingsRootService(root).ListWork(ctx, sessionID, options)
}

// GetWorkFromRecordingsRoot exercises Recordings-backed Work get reads through
// the published Recordings service root when no live session adapter is available.
func GetWorkFromRecordingsRoot(
	ctx context.Context,
	sessionID string,
	workID string,
	root recordings.Service,
) (work.ReadModel, error) {
	return newRecordingsRootService(root).GetWork(ctx, sessionID, workID)
}

type nilSessionResolver struct{}

func (nilSessionResolver) ResolveSessionAdapter(string) (stateaccess.SessionAdapter, error) {
	return nil, nil
}
