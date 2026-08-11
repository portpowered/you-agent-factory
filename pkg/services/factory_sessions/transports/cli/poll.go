// Package cli contains Factory Sessions-owned CLI transport helpers.
package cli

import (
	"context"
	"time"
)

// Poll repeatedly reads an owner-defined value until classify reports that it
// is ready. The Factory Sessions transport owns the timing and cancellation
// policy; callers supply only the read and classification functions.
func Poll[T any](
	ctx context.Context,
	interval time.Duration,
	read func(context.Context) (T, error),
	classify func(T) (bool, error),
) (T, error) {
	var zero T
	if ctx == nil {
		return zero, context.Canceled
	}
	if read == nil {
		return zero, context.Canceled
	}
	if classify == nil {
		return zero, context.Canceled
	}
	for {
		value, err := read(ctx)
		if err != nil {
			return zero, err
		}
		ready, err := classify(value)
		if err != nil {
			return zero, err
		}
		if ready {
			return value, nil
		}
		if err := Wait(ctx, interval); err != nil {
			return zero, err
		}
	}
}

// Wait pauses one owner-controlled retry interval without making timing a
// responsibility of a protocol adapter.
func Wait(ctx context.Context, interval time.Duration) error {
	if ctx == nil {
		return context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if interval <= 0 {
		return nil
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
