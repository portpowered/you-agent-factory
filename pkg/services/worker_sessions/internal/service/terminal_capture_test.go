package service

import (
	"context"
	"errors"
	"testing"
	"time"

	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
)

func TestT7TerminalObserverJoinsCaptureDriverAndHonorsCancellation(t *testing.T) {
	t.Parallel()
	for _, canceled := range []bool{false, true} {
		t.Run(map[bool]string{false: "joined", true: "canceled"}[canceled], func(t *testing.T) {
			t.Parallel()
			joined := make(chan struct{})
			r := &registry{supervisions: map[string]*supervision{"worker": {driverDone: joined}}}
			delivered := make(chan struct{})
			source := workersessions.ObservationSubscription{NextFunc: func(context.Context) workersessions.ObservationDelivery {
				close(delivered)
				return workersessions.ObservationDelivery{Kind: workersessions.ObservationDeliveryTerminal}
			}}
			stream := r.joinTerminalObservation(t.Context(), "worker", source)
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			result := make(chan workersessions.ObservationDelivery, 1)
			go func() { result <- stream.Next(ctx) }()
			<-delivered
			select {
			case <-result:
				t.Fatal("terminal returned before capture joined")
			default:
			}
			if canceled {
				cancel()
			} else {
				close(joined)
			}
			select {
			case got := <-result:
				if canceled {
					if got.Kind != workersessions.ObservationDeliveryCanceled || !errors.Is(got.Err, workersessions.ErrObservationCanceled) {
						t.Fatalf("canceled terminal = %+v", got)
					}
				} else if got.Kind != workersessions.ObservationDeliveryTerminal {
					t.Fatalf("joined terminal = %+v", got)
				}
			case <-ctx.Done():
				if !canceled {
					t.Fatal(ctx.Err())
				}
				// Cancellation is immediate; wait for the owned goroutine to join.
				got := <-result
				if !errors.Is(got.Err, workersessions.ErrObservationCanceled) {
					t.Fatalf("canceled terminal = %+v", got)
				}
			}
		})
	}
}
