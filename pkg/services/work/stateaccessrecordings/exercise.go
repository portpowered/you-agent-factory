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

// ListWorkFromRecordingsRoot exercises Recordings-backed Work list reads through
// the published Recordings service root when no live session adapter is available.
func ListWorkFromRecordingsRoot(
	ctx context.Context,
	sessionID string,
	root recordings.Service,
	options work.ListOptions,
) (work.ListResult, error) {
	svc := stateaccesswire.NewService(
		nilSessionResolver{},
		stateaccesswire.NewRecordingsAdapter(root),
	)
	return svc.ListWork(ctx, sessionID, options)
}

type nilSessionResolver struct{}

func (nilSessionResolver) ResolveSessionAdapter(string) (stateaccess.SessionAdapter, error) {
	return nil, nil
}
