package service

import (
	"context"
	"fmt"

	dispatchplanning "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/dispatch_planning"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// Pause keeps accepted intents pending without making them visible to Workers.
func (p *Planner) Pause(ctx context.Context) error {
	if err := validateLifecycleContext(ctx); err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.mode == dispatchplanning.RuntimeOutboxModeStopped {
		return fmt.Errorf(
			"%w: stopped by %s",
			dispatchplanning.ErrDispatchRuntimeStopped,
			p.stopReason,
		)
	}
	p.mode = dispatchplanning.RuntimeOutboxModePaused
	return nil
}

// Resume drains accepted pending intents in their original order. A failed
// publication returns the outbox to PAUSED so later intents cannot overtake it.
func (p *Planner) Resume(ctx context.Context) error {
	if err := validateLifecycleContext(ctx); err != nil {
		return err
	}
	p.mu.Lock()
	if p.mode == dispatchplanning.RuntimeOutboxModeStopped {
		reason := p.stopReason
		p.mu.Unlock()
		return fmt.Errorf("%w: stopped by %s", dispatchplanning.ErrDispatchRuntimeStopped, reason)
	}
	p.mode = dispatchplanning.RuntimeOutboxModeActive
	ordered := append([]*intentRecord(nil), p.ordered...)
	p.mu.Unlock()

	for _, record := range ordered {
		if err := p.publish(ctx, record); err != nil {
			p.mu.Lock()
			if p.mode == dispatchplanning.RuntimeOutboxModeActive {
				p.mode = dispatchplanning.RuntimeOutboxModePaused
			}
			p.mu.Unlock()
			return err
		}
	}
	return nil
}

// Stop permanently blocks new publication and forwards cancellation for every
// request already made visible to Workers. Repeated calls retry only failed
// cancellation propagation.
func (p *Planner) Stop(ctx context.Context, reason dispatchplanning.RuntimeStopReason) error {
	if err := validateLifecycleContext(ctx); err != nil {
		return err
	}
	if reason != dispatchplanning.RuntimeStopReasonCancelled &&
		reason != dispatchplanning.RuntimeStopReasonTerminated {
		return fmt.Errorf("%w: invalid stop reason %q", dispatchplanning.ErrInvalidRunnableDecision, reason)
	}

	p.mu.Lock()
	if p.mode != dispatchplanning.RuntimeOutboxModeStopped {
		p.mode = dispatchplanning.RuntimeOutboxModeStopped
		p.stopReason = reason
	}
	ordered := append([]*intentRecord(nil), p.ordered...)
	p.mu.Unlock()

	for _, record := range ordered {
		if err := p.cancelIfPublished(ctx, record); err != nil {
			return err
		}
	}
	return nil
}

// State returns a detached lifecycle observation.
func (p *Planner) State() dispatchplanning.RuntimeOutboxState {
	p.mu.Lock()
	defer p.mu.Unlock()
	return dispatchplanning.RuntimeOutboxState{
		Mode:       p.mode,
		StopReason: p.stopReason,
	}
}

func (p *Planner) cancelPublished(ctx context.Context, record *intentRecord) error {
	return p.cancelPublishedWithReason(ctx, record, workers.WorkstationDispatchCancelRequest{})
}

func (p *Planner) cancelPublishedWithReason(
	ctx context.Context,
	record *intentRecord,
	request workers.WorkstationDispatchCancelRequest,
) error {
	p.mu.Lock()
	if record.cancelled || record.cancelling || record.result != nil {
		p.mu.Unlock()
		return nil
	}
	dispatchID := record.action.Request.Execution.Dispatch.DispatchID
	canceler := p.canceler
	request.DispatchID = dispatchID
	record.cancelling = true
	p.mu.Unlock()

	if canceler == nil {
		p.mu.Lock()
		record.cancelling = false
		p.mu.Unlock()
		return fmt.Errorf("cancel dispatch intent %q: Workers canceler is unavailable", dispatchID)
	}
	if _, err := canceler(ctx, request); err != nil {
		p.mu.Lock()
		record.cancelling = false
		p.mu.Unlock()
		return err
	}

	p.mu.Lock()
	record.cancelling = false
	record.cancelled = true
	p.mu.Unlock()
	return nil
}

// cancelInvalidated forwards a superseding cancellation only after an
// invalidated intent has actually crossed the Workers publication boundary.
// A publication that races the invalidation sets published before its
// completion signal, so this remains safe for the PUBLISHING state.
func (p *Planner) cancelInvalidated(ctx context.Context, record *intentRecord) error {
	p.mu.Lock()
	status := record.status
	published := record.published
	p.mu.Unlock()
	if status != dispatchplanning.OutboxIntentStatusInvalidated || !published {
		return nil
	}
	return p.cancelPublishedWithReason(ctx, record, workers.WorkstationDispatchCancelRequest{
		Reason: workers.WorkstationDispatchCancelReasonSuperseded,
	})
}

func (p *Planner) cancelIfPublished(ctx context.Context, record *intentRecord) error {
	p.mu.Lock()
	status := record.status
	publishDone := record.publishDone
	p.mu.Unlock()
	if status == dispatchplanning.OutboxIntentStatusPublishing {
		select {
		case <-publishDone:
		case <-ctx.Done():
			return ctx.Err()
		}
		p.mu.Lock()
		status = record.status
		p.mu.Unlock()
	}
	if status != dispatchplanning.OutboxIntentStatusPublished &&
		status != dispatchplanning.OutboxIntentStatusInvalidated {
		return nil
	}
	if status == dispatchplanning.OutboxIntentStatusInvalidated {
		return p.cancelInvalidated(ctx, record)
	}
	return p.cancelPublished(ctx, record)
}

func validateLifecycleContext(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("%w: context is required", dispatchplanning.ErrInvalidRunnableDecision)
	}
	return ctx.Err()
}
