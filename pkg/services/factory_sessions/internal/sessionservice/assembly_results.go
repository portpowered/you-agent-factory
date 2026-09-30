package service

import (
	"context"
	"strings"

	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/controlplane"
	factorysessionexecution "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/execution"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/livesession"
)

var _ controlplane.ResultReadHost = (*Assembly)(nil)

func (a *Assembly) JavaScriptCheckpointStore(session *livesession.LiveSession) factoryruntime.JavaScriptCheckpointStore {
	if session == nil {
		return nil
	}
	return session.JavaScriptCheckpoints
}

func (a *Assembly) ReadResult(ctx context.Context, request factorysessions.SessionResultReadRequest) (factorysessions.SessionResultReadResult, error) {
	if err := validateCanonicalSessionID(request.SessionID); err != nil {
		return factorysessions.SessionResultReadResult{}, err
	}
	if request.Mode != factorysessions.SessionOperationModeLive {
		return a.SessionGateway.ReadResult(ctx, request)
	}
	normalized, err := factorysessionexecution.NormalizeResultRequest(request.Request)
	if err != nil {
		return factorysessions.SessionResultReadResult{}, err
	}
	return readCanonicalLiveResult(ctx, a, a.sessionResultProjection, strings.TrimSpace(request.SessionID), normalized)
}
