package http

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

// StartSource decodes one start-source HTTP request, invokes the accepted
// Automations root, and encodes the success response shape.
func (a *Adapter) StartSource(
	ctx context.Context,
	input StartSourceInput,
) (StartSourceResponse, error) {
	request, err := StartSourceRequestFromHTTP(input)
	if err != nil {
		return StartSourceResponse{}, err
	}
	result, err := a.invokeStartSource(ctx, request)
	if err != nil {
		return StartSourceResponse{}, err
	}
	return StartSourceResponseToHTTP(result), nil
}

// StopSource decodes one stop-source HTTP request, invokes the accepted
// Automations root, and encodes the success response shape.
func (a *Adapter) StopSource(
	ctx context.Context,
	input StopSourceInput,
) (StopSourceResponse, error) {
	request, err := StopSourceRequestFromHTTP(input)
	if err != nil {
		return StopSourceResponse{}, err
	}
	result, err := a.invokeStopSource(ctx, request)
	if err != nil {
		return StopSourceResponse{}, err
	}
	return StopSourceResponseToHTTP(result), nil
}

// WaitSource decodes one wait-source HTTP request, invokes the accepted
// Automations root, and encodes the success response shape.
func (a *Adapter) WaitSource(
	ctx context.Context,
	input WaitSourceInput,
) (WaitSourceResponse, error) {
	request, err := WaitSourceRequestFromHTTP(input)
	if err != nil {
		return WaitSourceResponse{}, err
	}
	result, err := a.invokeWaitSource(ctx, request)
	if err != nil {
		return WaitSourceResponse{}, err
	}
	return WaitSourceResponseToHTTP(result), nil
}

// SourceStatus decodes one source-status HTTP request, invokes the accepted
// Automations root, and encodes the success response shape.
func (a *Adapter) SourceStatus(
	ctx context.Context,
	input SourceStatusInput,
) (SourceStatusResponse, error) {
	request, err := SourceStatusRequestFromHTTP(input)
	if err != nil {
		return SourceStatusResponse{}, err
	}
	result, err := a.invokeSourceStatus(ctx, request)
	if err != nil {
		return SourceStatusResponse{}, err
	}
	return SourceStatusResponseToHTTP(result), nil
}

func (a *Adapter) invokeStartSource(
	ctx context.Context,
	request automations.StartSourceRequest,
) (automations.StartSourceResult, error) {
	if a == nil || a.root.Operations == nil {
		return automations.StartSourceResult{}, errors.New("automations root is required")
	}
	return a.root.StartSource(ctx, request)
}

func (a *Adapter) invokeStopSource(
	ctx context.Context,
	request automations.StopSourceRequest,
) (automations.StopSourceResult, error) {
	if a == nil || a.root.Operations == nil {
		return automations.StopSourceResult{}, errors.New("automations root is required")
	}
	return a.root.StopSource(ctx, request)
}

func (a *Adapter) invokeWaitSource(
	ctx context.Context,
	request automations.WaitSourceRequest,
) (automations.WaitSourceResult, error) {
	if a == nil || a.root.Operations == nil {
		return automations.WaitSourceResult{}, errors.New("automations root is required")
	}
	return a.root.WaitSource(ctx, request)
}
