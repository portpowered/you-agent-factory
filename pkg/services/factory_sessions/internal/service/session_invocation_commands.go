package service

import (
	"context"
	"fmt"
	"strings"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimebinding"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/service/invocation"
	legacyservice "github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/sessionservice"
	"github.com/portpowered/infinite-you/pkg/services/models"
)

// Invoke routes JavaScript Factories through the same durable execution path
// used by one-shot invocation while retaining the selected live session's
// request-scoped worker capabilities.
func (r *Root) Invoke(ctx context.Context, request factorysessions.SessionInvokeRequest) (factorysessions.InvocationResult, error) {
	if r == nil || r.Assembly == nil {
		return factorysessions.InvocationResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	sessionID := strings.TrimSpace(request.SessionID)
	projection, err := r.GetFactorySession(ctx, sessionID)
	if err != nil || !factorydefinitions.IsJavaScriptOrchestratorFactory(projection.Context.FactoryCfg) {
		return r.Assembly.Invoke(ctx, request)
	}
	return r.invokeJavaScriptSession(ctx, sessionID, legacyservice.CanonicalInvocationRequest(request), projection)
}

// InvokeFactorySession is the compatibility invocation boundary consumed by
// HTTP and remote CLI. Resolve JavaScript through the same scoped owner as
// Invoke so it retains the opened session's worker and progress capabilities.
func (r *Root) InvokeFactorySession(ctx context.Context, sessionID string, request factorysessions.InvocationRequest) (factorysessions.InvocationResult, error) {
	if r == nil || r.Assembly == nil {
		return factorysessions.InvocationResult{}, factorysessions.ErrRuntimeNotAvailable
	}
	sessionID = strings.TrimSpace(sessionID)
	projection, err := r.GetFactorySession(ctx, sessionID)
	if err != nil || !factorydefinitions.IsJavaScriptOrchestratorFactory(projection.Context.FactoryCfg) {
		return r.Assembly.InvokeFactorySession(ctx, sessionID, request)
	}
	return r.invokeJavaScriptSession(ctx, sessionID, request, projection)
}

func (r *Root) invokeJavaScriptSession(ctx context.Context, sessionID string, request factorysessions.InvocationRequest, projection factorysessions.SessionProjection) (factorysessions.InvocationResult, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}
	factoryDir := ""
	if projection.Context.Session != nil {
		factoryDir = projection.Context.Session.FactoryDir
	}
	target := factorysessions.InvocationTarget{FactoryDir: factoryDir, MockWorkersConfig: bound.MockWorkersConfig()}
	result, err := sessioninvocation.InvokeJavaScriptFactoryViaSessions(
		ctx, r, r, r.generateSessionID, sessionID, projection.Context, target,
		request,
		func(start *factorysessions.StartRequest) {
			start.MockWorkers = bound.MockWorkersConfig()
			start.WorkerSettings = bound.WorkerSettingsSnapshot()
			start.WorkerAttemptStarter = factorysessions.WorkerAttemptStarter(runtimeWorkerAttemptStarter(bound.Instance))
			start.WorkerProgressPublisher = runtimeProgressPublisher(bound.Instance)
		},
	)
	if err != nil {
		return factorysessions.InvocationResult{}, err
	}
	return factorysessions.InvocationResult{
		RequestID: result.RequestID, TraceID: result.TraceID,
		Status: factorysessions.InvocationTerminalStatus(result.Status), PrimaryResult: result.PrimaryResult,
		ErrorCode: result.ErrorCode, Message: result.Message, FailureReason: result.FailureReason,
		SessionID: result.SessionID, WorkID: result.WorkID, WorkName: result.WorkName, WorkState: result.WorkState,
	}, nil
}

// InvokeModelForSession uses the model invoker retained on the canonical live
// session. No invocation-specific runtime service or registry is allocated.
func (r *Root) InvokeModelForSession(ctx context.Context, sessionID, modelName string, request models.Request) (models.Result, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return models.Result{}, err
	}
	if bound.ModelInvoker == nil {
		return models.Result{}, fmt.Errorf("%w: model invoker for session %q", factorysessions.ErrRuntimeNotAvailable, sessionID)
	}
	return bound.ModelInvoker.InvokeModel(ctx, modelName, request)
}

// ResolveInvocationInputForSession applies the selected Factory signature
// through the input resolver attached to that same live session.
func (r *Root) ResolveInvocationInputForSession(_ context.Context, sessionID string, request factorysessions.InvocationRequest) (factorysessions.ResolvedInvocationInput, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return factorysessions.ResolvedInvocationInput{}, err
	}
	if bound.InputResolver == nil {
		return factorysessions.ResolvedInvocationInput{}, fmt.Errorf("%w: input resolver for session %q", factorysessions.ErrRuntimeNotAvailable, sessionID)
	}
	config, err := runtimebinding.RuntimeConfigForSession(r, sessionID)
	if err != nil {
		return factorysessions.ResolvedInvocationInput{}, err
	}
	return bound.InputResolver.ResolveInvocationInput(config.FactoryConfig(), request)
}

func (r *Root) ModelsScopeForSession(_ context.Context, sessionID string) (models.RuntimeScopeRef, error) {
	bound, err := r.applicationSessionState(sessionID)
	if err != nil {
		return models.RuntimeScopeRef{}, err
	}
	return bound.ModelsScope, nil
}
