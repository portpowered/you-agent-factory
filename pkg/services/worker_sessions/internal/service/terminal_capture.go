package service

import (
	"context"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

// A synchronous local CLI exits after observing terminal. Join the invocation
// driver before exposing that record so its durable capture has finished too.
// Runtime-owned attempts have their own completion barrier and no driver here.
func (r *registry) joinTerminalObservation(observerContext context.Context, id string, source workersessions.ObservationSubscription) workersessions.ObservationSubscription {
	return workersessions.ObservationSubscription{
		CloseFunc: source.Close,
		NextFunc: func(ctx context.Context) workersessions.ObservationDelivery {
			delivery := source.Next(ctx)
			if delivery.Kind == workersessions.ObservationDeliveryTerminal || delivery.Kind == workersessions.ObservationDeliveryTerminalReplay {
				if ctx == nil {
					ctx = observerContext
				}
				if err := r.waitForSupervisionDriver(ctx, id); err != nil {
					return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryCanceled, Err: workersessions.ErrObservationCanceled}
				}
			}
			return delivery
		},
	}
}
