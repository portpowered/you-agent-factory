package http

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/automations"
)

// rootFake is a focused Automations root fake for adapter-edge tests. It avoids
// constructing reconciliation, cron, script-poller, filesystem-watcher, hosted-source,
// or service-local Wire graphs.
type rootFake struct {
	reconcile    func(context.Context, automations.ReconcileRequest) (automations.ReconcileResult, error)
	startSource  func(context.Context, automations.StartSourceRequest) (automations.StartSourceResult, error)
	stopSource   func(context.Context, automations.StopSourceRequest) (automations.StopSourceResult, error)
	waitSource   func(context.Context, automations.WaitSourceRequest) (automations.WaitSourceResult, error)
	sourceStatus func(context.Context, automations.SourceStatusRequest) (automations.SourceStatusResult, error)
	getStatus    func(context.Context, automations.GetStatusRequest) (automations.GetStatusResult, error)
	getCursor    func(context.Context, automations.GetCursorRequest) (automations.GetCursorResult, error)
}

func (fake *rootFake) Reconcile(
	ctx context.Context,
	request automations.ReconcileRequest,
) (automations.ReconcileResult, error) {
	if fake.reconcile != nil {
		return fake.reconcile(ctx, request)
	}
	return automations.ReconcileResult{}, automations.ErrNotReady
}

func (fake *rootFake) StartSource(
	ctx context.Context,
	request automations.StartSourceRequest,
) (automations.StartSourceResult, error) {
	if fake.startSource != nil {
		return fake.startSource(ctx, request)
	}
	return automations.StartSourceResult{}, automations.ErrNotReady
}

func (fake *rootFake) StopSource(
	ctx context.Context,
	request automations.StopSourceRequest,
) (automations.StopSourceResult, error) {
	if fake.stopSource != nil {
		return fake.stopSource(ctx, request)
	}
	return automations.StopSourceResult{}, automations.ErrNotReady
}

func (fake *rootFake) WaitSource(
	ctx context.Context,
	request automations.WaitSourceRequest,
) (automations.WaitSourceResult, error) {
	if fake.waitSource != nil {
		return fake.waitSource(ctx, request)
	}
	return automations.WaitSourceResult{}, automations.ErrNotReady
}

func (fake *rootFake) SourceStatus(
	ctx context.Context,
	request automations.SourceStatusRequest,
) (automations.SourceStatusResult, error) {
	if fake.sourceStatus != nil {
		return fake.sourceStatus(ctx, request)
	}
	return automations.SourceStatusResult{}, automations.ErrNotFound
}

func (fake *rootFake) GetStatus(
	ctx context.Context,
	request automations.GetStatusRequest,
) (automations.GetStatusResult, error) {
	if fake.getStatus != nil {
		return fake.getStatus(ctx, request)
	}
	return automations.GetStatusResult{}, automations.ErrNotFound
}

func (fake *rootFake) GetCursor(
	ctx context.Context,
	request automations.GetCursorRequest,
) (automations.GetCursorResult, error) {
	if fake.getCursor != nil {
		return fake.getCursor(ctx, request)
	}
	return automations.GetCursorResult{}, automations.ErrNotFound
}
