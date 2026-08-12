package cli

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPollReturnsTheFirstReadyValue(t *testing.T) {
	reads := 0
	got, err := Poll(
		context.Background(),
		0,
		func(context.Context) (int, error) {
			reads++
			return reads, nil
		},
		func(value int) (bool, error) { return value == 3, nil },
	)
	if err != nil {
		t.Fatalf("Poll() error = %v", err)
	}
	if got != 3 {
		t.Fatalf("Poll() value = %d, want 3", got)
	}
	if reads != 3 {
		t.Fatalf("Poll() reads = %d, want 3", reads)
	}
}

func TestPollPropagatesReadAndClassifyErrors(t *testing.T) {
	readErr := errors.New("read failed")
	if _, err := Poll(
		context.Background(),
		0,
		func(context.Context) (int, error) { return 0, readErr },
		func(int) (bool, error) { return false, nil },
	); !errors.Is(err, readErr) {
		t.Fatalf("Poll() read error = %v, want %v", err, readErr)
	}

	classifyErr := errors.New("classify failed")
	if _, err := Poll(
		context.Background(),
		0,
		func(context.Context) (int, error) { return 1, nil },
		func(int) (bool, error) { return false, classifyErr },
	); !errors.Is(err, classifyErr) {
		t.Fatalf("Poll() classify error = %v, want %v", err, classifyErr)
	}
}

func TestPollRejectsMissingInputs(t *testing.T) {
	tests := []struct {
		name string
		call func() error
	}{
		{
			name: "context",
			call: func() error {
				_, err := Poll[int](nil, 0, func(context.Context) (int, error) { return 0, nil }, func(int) (bool, error) { return true, nil })
				return err
			},
		},
		{
			name: "read",
			call: func() error {
				_, err := Poll[int](context.Background(), 0, nil, func(int) (bool, error) { return true, nil })
				return err
			},
		},
		{
			name: "classify",
			call: func() error {
				_, err := Poll[int](context.Background(), 0, func(context.Context) (int, error) { return 0, nil }, nil)
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, context.Canceled) {
				t.Fatalf("Poll() error = %v, want context.Canceled", err)
			}
		})
	}
}

func TestWaitHonorsImmediateAndTimedCancellation(t *testing.T) {
	if err := Wait(context.Background(), 0); err != nil {
		t.Fatalf("Wait() with no interval error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Wait(ctx, time.Hour); !errors.Is(err, context.Canceled) {
		t.Fatalf("Wait() canceled error = %v, want context.Canceled", err)
	}

	ctx, cancel = context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Wait(ctx, time.Hour) }()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Wait() interrupted error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Wait() did not return after cancellation")
	}
}
