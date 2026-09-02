package run

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

func RunRemoteInvocation(
	ctx context.Context,
	cfg RunConfig,
	server string,
	remote RemoteInvocationOperation,
	presentations ...factoryvisualization.ResponsePresentation,
) error {
	return runRemoteInvocation(ctx, cfg, server, remote, nil, presentations...)
}

// RunRemoteInvocationWithWorkTarget runs a remote invocation with the
// Work-owned single-target preparation role supplied by the composition root.
func RunRemoteInvocationWithWorkTarget(
	ctx context.Context,
	cfg RunConfig,
	server string,
	remote RemoteInvocationOperation,
	prepareWorkTarget work.SingleWorkTargetPreparation,
	presentations ...factoryvisualization.ResponsePresentation,
) error {
	return runRemoteInvocation(ctx, cfg, server, remote, prepareWorkTarget, presentations...)
}

func runRemoteInvocation(
	ctx context.Context,
	cfg RunConfig,
	server string,
	remote RemoteInvocationOperation,
	prepareWorkTarget work.SingleWorkTargetPreparation,
	presentations ...factoryvisualization.ResponsePresentation,
) error {
	if remote == nil {
		return fmt.Errorf("run remote durable start: operation is required")
	}
	request, invocationMode, err := resolveFactoryInvocationRequestForRun(cfg, prepareWorkTarget)
	if err != nil {
		return err
	}
	if !invocationMode || request == nil {
		return &InvocationError{
			Code:    RemoteInvocationInputRequiredCode,
			Message: "--remote run requires invocation input; provide a normalized Factory target and invocation arguments",
		}
	}
	if sessionID := strings.TrimSpace(cfg.FactorySessionID); sessionID != "" {
		return runRemoteExistingSessionInvocation(ctx, cfg, server, sessionID, *request, remote, presentations...)
	}
	executionRequest, err := remoteDurableRequestFromRunConfig(cfg, *request)
	if err != nil {
		return err
	}
	response, err := remote.StartFactorySession(ctx, RemoteInvocationRequest{
		Server: server, Request: executionRequest, Diagnostics: cfg.Diagnostics, Verbose: cfg.Verbose,
	})
	if err != nil {
		return err
	}
	if strings.TrimSpace(response.SessionId) == "" || strings.TrimSpace(string(response.Status)) == "" {
		return &InvocationError{
			Code:    RemoteDurableResponseInvalidCode,
			Message: "remote durable start returned no canonical session identity",
		}
	}
	presentation := firstResponsePresentation(presentations)
	resultOperation, ok := remote.(RemoteInvocationResultOperation)
	if !ok {
		// Keep the narrow injected start seam usable for placement-only callers.
		return writeRemoteDurableStartResult(cfg, response)
	}
	if isResponseStreamOutputMode(cfg.InvocationOutputMode) {
		return runRemoteResponseStreamInvocation(
			ctx, cfg, server, response, executionRequest.RequestId, remote, resultOperation, presentation,
		)
	}
	result, err := waitForRemoteInvocationResult(
		ctx, cfg, server, response, executionRequest.RequestId, resultOperation,
	)
	if err != nil {
		return err
	}
	return writeRemoteInvocationResult(cfg, result)
}

func runRemoteExistingSessionInvocation(
	ctx context.Context,
	cfg RunConfig,
	server, sessionID string,
	request factoryapi.InvocationRequest,
	remote RemoteInvocationOperation,
	presentations ...factoryvisualization.ResponsePresentation,
) error {
	invoker, ok := remote.(RemoteExistingSessionInvocationOperation)
	if !ok {
		return &InvocationError{Code: RemoteDurableResponseInvalidCode, Message: "remote explicit-session invocation requires the Factory Session invocation operation"}
	}
	response, err := invoker.InvokeFactorySession(ctx, RemoteExistingSessionInvocationRequest{
		Server: server, SessionID: sessionID, Request: request,
		Diagnostics: cfg.Diagnostics, Verbose: cfg.Verbose,
	})
	if err != nil {
		return err
	}
	result := apisurface.FactoryInvocationResultFromResponse(response)
	if strings.TrimSpace(result.SessionID) == "" {
		result.SessionID = sessionID
	}
	if !isResponseStreamOutputMode(cfg.InvocationOutputMode) {
		return writeRemoteInvocationResult(cfg, result)
	}
	return writeRemoteExistingSessionStream(
		ctx, cfg, server, sessionID, result, remote, firstResponsePresentation(presentations),
	)
}

func writeRemoteExistingSessionStream(
	ctx context.Context,
	cfg RunConfig,
	server, sessionID string,
	result apisurface.FactoryInvocationResult,
	remote RemoteInvocationOperation,
	presentation factoryvisualization.ResponsePresentation,
) error {
	renderer, err := invocationFactoryEventRenderer(cfg, presentation)
	if err != nil {
		return err
	}
	if renderer == nil {
		return &InvocationError{Code: RemoteDurableResponseInvalidCode, Message: "remote explicit-session response stream renderer is required"}
	}
	defer renderer.StopProgressRendering()
	if ctx.Err() != nil && isCanceledInvocationResult(result) {
		if err := renderer.WriteFinalInvocationResult(result); err != nil {
			return err
		}
		return invocationResultFailure(result)
	}
	events, ok := remote.(RemoteInvocationEventOperation)
	if !ok {
		return &InvocationError{Code: RemoteDurableResponseInvalidCode, Message: "remote explicit-session response stream requires the canonical Factory Event operation"}
	}
	stream, err := events.OpenFactorySessionEvents(ctx, RemoteInvocationEventRequest{
		Server: server, SessionID: sessionID, ReplayOnly: true,
		Diagnostics: cfg.Diagnostics, Verbose: cfg.Verbose,
	})
	if err != nil {
		return err
	}
	defer stream.Close()
	cursor := RemoteInvocationEventCursor{}
	if err := consumeRemoteFactoryEventReplay(ctx, stream, renderer, &cursor); err != nil {
		return err
	}
	if err := renderer.WriteFinalInvocationResult(result); err != nil {
		return err
	}
	if result.Status != interfaces.InvocationTerminalStatusCompleted {
		return invocationResultFailure(result)
	}
	return nil
}

func isCanceledInvocationResult(result apisurface.FactoryInvocationResult) bool {
	return result.Status == interfaces.InvocationTerminalStatusTimedOut ||
		result.Status == interfaces.InvocationTerminalStatusCanceled
}

func runRemoteResponseStreamInvocation(
	ctx context.Context,
	cfg RunConfig,
	server string,
	response factoryapi.FactorySessionExecutionResponse,
	requestID string,
	remote RemoteInvocationOperation,
	results RemoteInvocationResultOperation,
	presentation factoryvisualization.ResponsePresentation,
) error {
	events, ok := remote.(RemoteInvocationEventOperation)
	if !ok {
		return &InvocationError{
			Code:    RemoteDurableResponseInvalidCode,
			Message: "remote response-stream output requires the canonical Factory Event operation",
		}
	}
	result, renderer, streamErr := runRemoteResponseStream(
		ctx, cfg, server, response, requestID, events, results, presentation,
	)
	if renderer == nil {
		return streamErr
	}
	defer renderer.StopProgressRendering()
	if streamErr != nil {
		failure := remoteResponseStreamFailureResult(requestID, response.SessionId, streamErr)
		writeErr := renderer.WriteFinalInvocationResult(failure)
		if writeErr != nil {
			return errors.Join(streamErr, writeErr)
		}
		return streamErr
	}
	if err := renderer.WriteFinalInvocationResult(result); err != nil {
		return err
	}
	if result.Status != interfaces.InvocationTerminalStatusCompleted {
		return invocationResultFailure(result)
	}
	return nil
}

func firstResponsePresentation(presentations []factoryvisualization.ResponsePresentation) factoryvisualization.ResponsePresentation {
	if len(presentations) == 0 {
		return nil
	}
	return presentations[0]
}

func safeRemoteEndpoint(raw string) string {
	trimmed := strings.TrimSpace(raw)
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return invalidRemoteEndpointLabel
	}
	parsed.User = nil
	base, err := cliserver.ResolveBase(parsed.String())
	if err != nil {
		return invalidRemoteEndpointLabel
	}
	return base.String()
}
