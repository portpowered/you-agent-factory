// This file holds the orchestrator-kind routing an activated runtime's
// invocation takes, kept beside service.go rather than inside it so that file
// stays within the repository's own size limit.
package ondemandtarget

import (
	"context"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/roles"
	"github.com/portpowered/infinite-you/pkg/services/factory_sessions/internal/runtimeopening/invocation"
)

// invokeOnActivatedRuntime runs one invocation against an activated runtime
// through the path that runtime's own orchestrator requires.
//
// A JavaScript-orchestrator Factory's whole workflow is its program: it
// declares no work types, so the Work-submission path every other Factory uses
// fails while resolving the single handlingBehavior DEFAULT work type, before
// any Worker runs. Routing it to durable workflow execution instead is what
// keeps a Factory invocable through this activation -- the one ACP dispatch
// uses -- and not only through the CLI's one-shot invocation operation, which
// has always made this same distinction.
//
// A projection read that fails is not treated as fatal here. It is only how
// this decides which path to take, and the Work-submission path reports its
// own failure precisely; failing the invocation on the read alone would
// replace a specific error with a vaguer one.
func (s *Service) invokeOnActivatedRuntime(
	ctx context.Context,
	active *activatedRuntime,
	request factorysessions.InvocationRequest,
) (factorysessions.InvocationResult, error) {
	sessionID := active.factorySessionID()
	projection, err := active.opened.Sessions.GetFactorySession(ctx, sessionID)
	if err != nil || !factorydefinitions.IsJavaScriptOrchestratorFactory(projection.Context.FactoryCfg) {
		return active.opened.Sessions.InvokeFactorySession(ctx, sessionID, request)
	}
	// On-demand activation callers observe a running invocation through
	// SubscribeFactoryResponseEvents against the runtime's own response stream,
	// not through a one-shot invocation presentation scope.
	result, err := invocation.InvokeOpenedJavaScriptFactory(
		ctx, active.opened, projection.Context, active.invocationTarget(), request, s.generateID,
	)
	return sessionInvocationResult(result), err
}

// invocationTarget rebuilds the invocation-target view of this activation's
// own opening request. Only the fields the JavaScript workflow path reads are
// populated: the Factory directory a relative workflow sourceRef resolves
// against, and the mock-worker configuration that selects a fake child
// executor.
func (a *activatedRuntime) invocationTarget() roles.InvocationTarget {
	return roles.InvocationTarget{
		FactorySessionID:  a.factorySessionID(),
		FactoryDir:        a.config.FactoryDefinition.Directory,
		FactorySourcePath: a.config.FactoryDefinition.SourcePath,
		ExecutionBaseDir:  a.config.FactoryDefinition.ExecutionBaseDir,
		MockWorkersConfig: a.config.Workers.MockWorkers,
	}
}

// sessionInvocationResult converts the Factory Definitions-owned invocation
// result the workflow path returns into the Factory Sessions-owned result this
// service publishes. The two carry the same fields; they are distinct types
// because they belong to distinct service contracts.
func sessionInvocationResult(
	result factorydefinitions.FactoryInvocationResult,
) factorysessions.InvocationResult {
	return factorysessions.InvocationResult{
		RequestID:     result.RequestID,
		TraceID:       result.TraceID,
		Status:        factorysessions.InvocationTerminalStatus(result.Status),
		PrimaryResult: result.PrimaryResult,
		ErrorCode:     result.ErrorCode,
		Message:       result.Message,
		SessionID:     result.SessionID,
		WorkID:        result.WorkID,
		WorkName:      result.WorkName,
		WorkState:     result.WorkState,
	}
}
