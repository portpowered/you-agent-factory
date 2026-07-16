package controlplane

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	workflowresult "github.com/portpowered/infinite-you/pkg/orchestrators/javascript/result"
)

// ResultReadHost exposes live session projection and checkpoint seams for result reads.
type ResultReadHost interface {
	LiveReadHost
	JavaScriptCheckpointStore(session *factorysessions.LiveSession) *factorysessions.JavaScriptCheckpointStore
}

// GetLiveFactorySessionResult returns the terminal JavaScript session result read shape.
func GetLiveFactorySessionResult(
	ctx context.Context,
	host ResultReadHost,
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
	result := factorysessions.ProjectSessionResult(sessionID, projectionCtx, host.JavaScriptCheckpointStore(session))
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
	result := factorysessions.ProjectSessionPartialResult(sessionID, projectionCtx, host.JavaScriptCheckpointStore(session))
	return result, nil
}
