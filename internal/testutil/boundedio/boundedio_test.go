package boundedio

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPendingCanBeAwaitedAfterAnEarlierTimeout(t *testing.T) {
	release := make(chan struct{})
	pending := Start(func() string {
		<-release
		return "complete"
	})

	if _, err := pending.Await(context.Background(), time.Millisecond); !errors.Is(err, ErrTimeout) {
		t.Fatalf("first Await error = %v, want ErrTimeout", err)
	}
	close(release)
	if got, err := pending.Await(context.Background(), time.Second); err != nil || got != "complete" {
		t.Fatalf("second Await = %q, %v; want complete, nil", got, err)
	}
}

func TestPendingDistinguishesCallerCancellation(t *testing.T) {
	release := make(chan struct{})
	pending := Start(func() struct{} {
		<-release
		return struct{}{}
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := pending.Await(ctx, time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("Await error = %v, want context.Canceled", err)
	}
	close(release)
}

func TestDeadlineAndRemainingOwnHostClockMechanics(t *testing.T) {
	deadline := Deadline(time.Second)
	remaining := Remaining(deadline)
	if remaining <= 0 || remaining > time.Second {
		t.Fatalf("Remaining(Deadline(1s)) = %v, want (0, 1s]", remaining)
	}
}
