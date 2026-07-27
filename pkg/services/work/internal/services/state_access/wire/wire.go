// Package wire constructs the Work state_access nested subservice.
package wire

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/work"
	stateaccess "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access"
	internalservice "github.com/portpowered/infinite-you/pkg/services/work/internal/services/state_access/internal/service"
)

// NewService constructs the private Work state_access subservice from a
// session resolver. The resolver adapts Factory Sessions into the parent-private
// Session adapter port.
func NewService(sessions stateaccess.SessionResolver) stateaccess.Service {
	return internalservice.New(sessions)
}

type runtimeSessionResolver struct {
	runtimes work.RuntimeResolver
}

// NewRuntimeSessionResolver adapts Work's consumer-owned runtime resolver into
// the parent-private Session adapter port used by state_access.
func NewRuntimeSessionResolver(runtimes work.RuntimeResolver) stateaccess.SessionResolver {
	if runtimes == nil {
		return nil
	}
	return runtimeSessionResolver{runtimes: runtimes}
}

func (r runtimeSessionResolver) ResolveSessionAdapter(sessionID string) (stateaccess.SessionAdapter, error) {
	runtime, err := r.runtimes.ResolveWorkRuntime(sessionID)
	if err != nil {
		return nil, err
	}
	if runtime == nil {
		return nil, nil
	}
	return runtimeSessionAdapter{runtime: runtime}, nil
}

type runtimeSessionAdapter struct {
	runtime work.Runtime
}

func (a runtimeSessionAdapter) SubmitWorkRequest(
	ctx context.Context,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	return a.runtime.SubmitWorkRequest(ctx, request)
}

func (a runtimeSessionAdapter) MoveWork(
	ctx context.Context,
	workID string,
	stateName string,
	source work.WorkStateChangeSource,
	requestID string,
) (work.OperatorMoveResult, error) {
	return a.runtime.MoveWork(ctx, workID, stateName, source, requestID)
}

func (a runtimeSessionAdapter) ReadWorkSnapshot(
	ctx context.Context,
) (work.ReadSnapshot, error) {
	return a.runtime.ReadWorkSnapshot(ctx)
}
