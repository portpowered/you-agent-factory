package lifecycle_test

import (
	"context"
	"errors"
	"io"
	"reflect"
	"testing"

	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
)

func TestManagerRunsAndClosesInReverseOrder(t *testing.T) {
	var order []string
	primary := lifecycle.NewRunner(func(context.Context) error {
		order = append(order, "run")
		return nil
	})
	secondary := lifecycle.Functions{
		StartFunc: func(context.Context) error { order = append(order, "start-secondary"); return nil },
		StopFunc:  func(context.Context) error { order = append(order, "stop-secondary"); return nil },
	}
	resource := closerFunc(func() error { order = append(order, "close"); return nil })

	err := lifecycle.NewManager().Run(context.Background(), lifecycle.Plan{
		Components: []lifecycle.NamedComponent{
			{Name: "secondary", Component: secondary},
			{Name: "primary", Component: primary, Primary: true},
		},
		Resources: []lifecycle.NamedResource{{Name: "owned", Resource: resource}},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := []string{"start-secondary", "run", "stop-secondary", "close"}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
}

func TestManagerUnwindsStartFailureAndJoinsCleanupErrors(t *testing.T) {
	startErr := errors.New("start failed")
	stopErr := errors.New("stop failed")
	closeErr := errors.New("close failed")
	started := lifecycle.Functions{
		StartFunc: func(context.Context) error { return nil },
		StopFunc:  func(context.Context) error { return stopErr },
	}
	failing := lifecycle.Functions{StartFunc: func(context.Context) error { return startErr }}
	primary := lifecycle.NewRunner(func(context.Context) error { return nil })

	err := lifecycle.NewManager().Run(context.Background(), lifecycle.Plan{
		Components: []lifecycle.NamedComponent{
			{Name: "started", Component: started},
			{Name: "failing", Component: failing},
			{Name: "primary", Component: primary, Primary: true},
		},
		Resources: []lifecycle.NamedResource{{
			Name: "owned", Resource: closerFunc(func() error { return closeErr }),
		}},
	})
	for _, cause := range []error{startErr, stopErr, closeErr} {
		if !errors.Is(err, cause) {
			t.Fatalf("Run() error = %v, want cause %v", err, cause)
		}
	}
}

func TestManagerUsesSameShutdownPathForCancellationAndRunnerFailure(t *testing.T) {
	runnerErr := errors.New("runner failed")
	tests := []struct {
		name       string
		run        func(context.Context) error
		cancelWhen func(context.CancelFunc) func()
		wantErr    error
	}{
		{
			name: "cancellation",
			run: func(ctx context.Context) error {
				<-ctx.Done()
				return ctx.Err()
			},
			cancelWhen: func(cancel context.CancelFunc) func() { return cancel },
		},
		{
			name: "runner failure",
			run:  func(context.Context) error { return runnerErr },
			cancelWhen: func(context.CancelFunc) func() {
				return func() {}
			},
			wantErr: runnerErr,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var stopped, closed int
			secondary := lifecycle.Functions{
				StopFunc: func(context.Context) error { stopped++; return nil },
			}
			plan := lifecycle.Plan{
				Components: []lifecycle.NamedComponent{
					{Name: "secondary", Component: secondary},
					{Name: "primary", Component: lifecycle.NewRunner(test.run), Primary: true},
				},
				Resources: []lifecycle.NamedResource{{
					Name: "owned",
					Resource: closerFunc(func() error {
						closed++
						return nil
					}),
				}},
			}
			err := lifecycle.NewManager().RunWithReady(ctx, plan, test.cancelWhen(cancel))
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("RunWithReady() error = %v, want %v", err, test.wantErr)
			}
			if stopped != 1 || closed != 1 {
				t.Fatalf("shutdown counts = stop %d close %d, want 1 and 1", stopped, closed)
			}
		})
	}
}

type closerFunc func() error

func (fn closerFunc) Close() error { return fn() }

var _ io.Closer = closerFunc(nil)
