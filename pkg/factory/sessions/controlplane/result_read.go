package controlplane

import (
	"context"
	"fmt"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
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
) (factoryapi.FactorySessionLiveResult, error) {
	if host == nil {
		return factoryapi.FactorySessionLiveResult{}, fmt.Errorf("factory session gateway is required")
	}
	session, err := host.RequireSession(sessionID)
	if err != nil {
		return factoryapi.FactorySessionLiveResult{}, err
	}
	projectionCtx, err := host.BuildSessionProjectionContext(ctx, session)
	if err != nil {
		return factoryapi.FactorySessionLiveResult{}, err
	}
	if !interfaces.IsJavaScriptOrchestratorFactory(projectionCtx.FactoryCfg) {
		return factoryapi.FactorySessionLiveResult{}, fmt.Errorf("%w", apisurface.ErrFactorySessionResultUnavailable)
	}
	return factorysessions.ProjectSessionResult(sessionID, projectionCtx, host.JavaScriptCheckpointStore(session)), nil
}

// GetLiveFactorySessionPartialResult returns checkpoint-backed partial JavaScript results.
func GetLiveFactorySessionPartialResult(
	ctx context.Context,
	host ResultReadHost,
	sessionID string,
) (factoryapi.FactorySessionPartialResult, error) {
	if host == nil {
		return factoryapi.FactorySessionPartialResult{}, fmt.Errorf("factory session gateway is required")
	}
	session, err := host.RequireSession(sessionID)
	if err != nil {
		return factoryapi.FactorySessionPartialResult{}, err
	}
	projectionCtx, err := host.BuildSessionProjectionContext(ctx, session)
	if err != nil {
		return factoryapi.FactorySessionPartialResult{}, err
	}
	if !interfaces.IsJavaScriptOrchestratorFactory(projectionCtx.FactoryCfg) {
		return factoryapi.FactorySessionPartialResult{}, fmt.Errorf("%w", apisurface.ErrFactorySessionResultUnavailable)
	}
	return factorysessions.ProjectSessionPartialResult(sessionID, projectionCtx, host.JavaScriptCheckpointStore(session)), nil
}
