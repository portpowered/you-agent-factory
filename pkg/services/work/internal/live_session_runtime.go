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
	return liveSessionRuntimeAdapter{
		runtime: runtime.Factory,
		ingress: runtime.WorkAndEventIngress,
	}, nil
}

type liveSessionRuntimeAdapter struct {
	runtime factory.Service
	// ingress is the Work-submission boundary Factory Sessions declares on the
	// live runtime. Work reads the declared capability rather than recovering
	// one from the runtime value.
	ingress factory.APIFactory
}

func (a liveSessionRuntimeAdapter) SubmitWorkRequest(
	ctx context.Context,
	request work.WorkRequest,
) (work.WorkRequestSubmitResult, error) {
	if a.ingress == nil {
		return work.WorkRequestSubmitResult{}, fmt.Errorf("Factory Runtime work submission is required")
	}
	return a.ingress.SubmitWorkRequest(ctx, request)
}

func (a liveSessionRuntimeAdapter) MoveWork(
	ctx context.Context,
	workID string,
	stateName string,
	source work.WorkStateChangeSource,
	requestID string,
) (work.OperatorMoveResult, error) {
	if a.runtime == nil {
		return work.OperatorMoveResult{}, fmt.Errorf("Factory Runtime work move is required")
	}
	result, err := a.runtime.ControlMoveWork(ctx, factory.MoveWorkRequest{
		WorkID: workID, StateName: stateName,
		Source: factory.WorkMoveSource(source), RequestID: requestID,
	})
	if err != nil {
		return work.OperatorMoveResult{}, err
	}
	return work.OperatorMoveResult{
		WorkID: result.WorkID, WorkTypeID: result.WorkTypeID,
		FromState: result.FromState, ToState: result.ToState,
	}, nil
}

func (a liveSessionRuntimeAdapter) ReadWorkSnapshot(context.Context) (work.ReadSnapshot, error) {
	return work.ReadSnapshot{}, nil
}
