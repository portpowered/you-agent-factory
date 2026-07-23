package controlplane

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workflowresult "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessionprojection "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionprojection"
)

// ResultReadHost exposes live session projection and checkpoint seams for result reads.
type ResultReadHost interface {
	LiveReadHost
	JavaScriptCheckpointStore(session *factorysessions.LiveSession) workflowresult.JavaScriptCheckpointStore
}

// GetLiveFactorySessionResult returns the terminal JavaScript session result read shape.
func GetLiveFactorySessionResult(
	ctx context.Context,
	host ResultReadHost,
	projection workflowresult.SessionResultProjectionOperation,
	sessionID string,
) (workflowresult.LiveSessionResult, error) {
	if host == nil {
		return workflowresult.LiveSessionResult{}, fmt.Errorf("factory session gateway is required")
	}
	session, err := host.RequireSession(sessionID)
	if err != nil {
		return workflowresult.LiveSessionResult{}, err
	}
	projectionCtx, err := host.BuildSessionProjectionContext(ctx, session)
	if err != nil {
		return workflowresult.LiveSessionResult{}, err
	}
	if !interfaces.IsJavaScriptOrchestratorFactory(projectionCtx.FactoryCfg) {
		return workflowresult.LiveSessionResult{}, fmt.Errorf("%w", factorysessions.ErrResultUnavailable)
	}
	if projection == nil {
		return workflowresult.LiveSessionResult{}, fmt.Errorf("Factory Runtime session result projection is required")
	}
	result := sessionprojection.ProjectSessionResult(
		sessionID,
		projectionCtx,
		host.JavaScriptCheckpointStore(session),
		projection,
	)
	return result, nil
}

// GetLiveFactorySessionPartialResult returns checkpoint-backed partial JavaScript results.
func GetLiveFactorySessionPartialResult(
	ctx context.Context,
	host ResultReadHost,
	sessionID string,
) (workflowresult.PartialSessionResult, error) {
	if host == nil {
		return workflowresult.PartialSessionResult{}, fmt.Errorf("factory session gateway is required")
	}
	session, err := host.RequireSession(sessionID)
	if err != nil {
		return workflowresult.PartialSessionResult{}, err
	}
	projectionCtx, err := host.BuildSessionProjectionContext(ctx, session)
	if err != nil {
		return workflowresult.PartialSessionResult{}, err
	}
	if !interfaces.IsJavaScriptOrchestratorFactory(projectionCtx.FactoryCfg) {
		return workflowresult.PartialSessionResult{}, fmt.Errorf("%w", factorysessions.ErrResultUnavailable)
	}
	result := sessionprojection.ProjectSessionPartialResult(sessionID, projectionCtx, host.JavaScriptCheckpointStore(session))
	return result, nil
}
