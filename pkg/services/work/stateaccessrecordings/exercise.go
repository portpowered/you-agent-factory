// Package stateaccessrecordings is a transitional compile shim that re-exports
// the Recordings-backed state_access exercise helpers from
// work/internal/services/state_access/wire. Peer and functional callers will
// retarget to the Work root contract; baseline deletion of this path is owned
// by DEL-WORK.
package stateaccessrecordings

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/recordings"
	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccesswire "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/wire"
)

// ListWorkFromRecordingsRoot delegates to the owner-private state_access wire helper.
func ListWorkFromRecordingsRoot(
	ctx context.Context,
	sessionID string,
	root recordings.Service,
	options work.ListOptions,
) (work.ListResult, error) {
	return stateaccesswire.ListWorkFromRecordingsRoot(ctx, sessionID, root, options)
}

// GetWorkFromRecordingsRoot delegates to the owner-private state_access wire helper.
func GetWorkFromRecordingsRoot(
	ctx context.Context,
	sessionID string,
	workID string,
	root recordings.Service,
) (work.ReadModel, error) {
	return stateaccesswire.GetWorkFromRecordingsRoot(ctx, sessionID, workID, root)
}
