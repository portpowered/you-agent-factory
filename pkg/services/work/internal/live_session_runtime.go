package internal

import (
	"context"
	"fmt"
	"strings"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
)

type liveSessionRuntimeResolver struct {
	sessions RuntimeResolver
}

func (r liveSessionRuntimeResolver) ResolveWorkRuntime(sessionID string) (work.Runtime, error) {
	if r.sessions == nil {
		return nil, fmt.Errorf("Factory Session runtime service is required")
	}
	runtime := r.sessions.Resolve(sessionID)
	if runtime == nil || runtime.Factory == nil {
		return nil, fmt.Errorf("%w: %s", factorysessions.ErrSessionNotFound, strings.TrimSpace(sessionID))
	}
	return liveSessionRuntimeAdapter{factory: runtime.Factory}, nil
}

type liveSessionRuntimeAdapter struct {
	factory any
}

func (a liveSessionRuntimeAdapter) SubmitWorkRequest(
	ctx context.Context,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	legacyRuntime, ok := a.factory.(factory.APIFactory)
	if !ok {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("legacy Factory Runtime submission is required")
	}
	return legacyRuntime.SubmitWorkRequest(ctx, request)
}

func (a liveSessionRuntimeAdapter) MoveWork(
	ctx context.Context,
	workID string,
	stateName string,
	source work.WorkStateChangeSource,
	requestID string,
) (work.OperatorMoveResult, error) {
	mover, ok := a.factory.(factory.WorkMover)
	if !ok {
		return work.OperatorMoveResult{}, fmt.Errorf("legacy Factory Runtime work move is required")
	}
	return mover.MoveWork(ctx, workID, stateName, source, requestID)
}

func (a liveSessionRuntimeAdapter) ReadWorkSnapshot(context.Context) (work.ReadSnapshot, error) {
	return work.ReadSnapshot{}, nil
}
