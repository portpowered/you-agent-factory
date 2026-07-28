package http

import (
	"context"
	"errors"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

// Reconcile decodes one reconcile HTTP request, invokes the accepted Automations
// root, and encodes the success response shape.
func (a *Adapter) Reconcile(
	ctx context.Context,
	input ReconcileInput,
) (ReconcileResponse, error) {
	request, err := ReconcileRequestFromHTTP(input)
	if err != nil {
		return ReconcileResponse{}, err
	}
	if err := guardRequestContext(ctx); err != nil {
		return ReconcileResponse{}, err
	}
	result, err := a.invokeReconcile(ctx, request)
	if err != nil {
		return ReconcileResponse{}, err
	}
	return ReconcileResponseToHTTP(result), nil
}

// GetStatus decodes one instance-status HTTP request, invokes the accepted
// Automations root, and encodes the success response shape.
func (a *Adapter) GetStatus(
	ctx context.Context,
	input GetStatusInput,
) (GetStatusResponse, error) {
	request, err := GetStatusRequestFromHTTP(input)
	if err != nil {
		return GetStatusResponse{}, err
	}
	if err := guardRequestContext(ctx); err != nil {
		return GetStatusResponse{}, err
	}
	result, err := a.invokeGetStatus(ctx, request)
	if err != nil {
		return GetStatusResponse{}, err
	}
	return GetStatusResponseToHTTP(result), nil
}

// GetCursor decodes one cursor HTTP request, invokes the accepted Automations
// root, and encodes the success response shape.
func (a *Adapter) GetCursor(
	ctx context.Context,
	input GetCursorInput,
) (GetCursorResponse, error) {
	request, err := GetCursorRequestFromHTTP(input)
	if err != nil {
		return GetCursorResponse{}, err
	}
	if err := guardRequestContext(ctx); err != nil {
		return GetCursorResponse{}, err
	}
	result, err := a.invokeGetCursor(ctx, request)
	if err != nil {
		return GetCursorResponse{}, err
	}
	return GetCursorResponseToHTTP(result), nil
}

func (a *Adapter) invokeReconcile(
	ctx context.Context,
	request automations.ReconcileRequest,
) (automations.ReconcileResult, error) {
	if a == nil || a.root.Operations == nil {
		return automations.ReconcileResult{}, errors.New("automations root is required")
	}
	return a.root.Reconcile(ctx, request)
}

func (a *Adapter) invokeGetStatus(
	ctx context.Context,
	request automations.GetStatusRequest,
) (automations.GetStatusResult, error) {
	if a == nil || a.root.Operations == nil {
		return automations.GetStatusResult{}, errors.New("automations root is required")
	}
	return a.root.GetStatus(ctx, request)
}

func (a *Adapter) invokeGetCursor(
	ctx context.Context,
	request automations.GetCursorRequest,
) (automations.GetCursorResult, error) {
	if a == nil || a.root.Operations == nil {
		return automations.GetCursorResult{}, errors.New("automations root is required")
	}
	return a.root.GetCursor(ctx, request)
}
