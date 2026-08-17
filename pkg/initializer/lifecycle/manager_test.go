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

// TestManagerUnwindsStartFailureAndJoinsCleanupErrors proves a failed
// activation unwinds exactly once: every component that actually started is
// stopped exactly once in reverse declaration order, the component whose Start
// failed is never stopped, no component declared after the failure is started,
// the primary component never runs, every acquired resource closes exactly
// once in reverse declaration order, and the primary start failure is retained
// alongside each cleanup failure it did not mask.
func TestManagerUnwindsStartFailureAndJoinsCleanupErrors(t *testing.T) {
	startErr := errors.New("start failed")
	stopErr := errors.New("stop failed")
	closeErr := errors.New("close failed")
	var order []string
	record := func(step string) { order = append(order, step) }
	first := lifecycle.Functions{
		StartFunc: func(context.Context) error { record("start-first"); return nil },
		StopFunc:  func(context.Context) error { record("stop-first"); return nil },
	}
	second := lifecycle.Functions{
		StartFunc: func(context.Context) error { record("start-second"); return nil },
		StopFunc:  func(context.Context) error { record("stop-second"); return stopErr },
	}
	failing := lifecycle.Functions{
		StartFunc: func(context.Context) error { record("start-failing"); return startErr },
		StopFunc:  func(context.Context) error { record("stop-failing"); return nil },
	}
	unreached := lifecycle.Functions{
		StartFunc: func(context.Context) error { record("start-unreached"); return nil },
		StopFunc:  func(context.Context) error { record("stop-unreached"); return nil },
	}
	primary := lifecycle.NewRunner(func(context.Context) error { record("run-primary"); return nil })

	err := lifecycle.NewManager().Run(context.Background(), lifecycle.Plan{
		Components: []lifecycle.NamedComponent{
			{Name: "first", Component: first},
			{Name: "second", Component: second},
			{Name: "failing", Component: failing},
			{Name: "unreached", Component: unreached},
			{Name: "primary", Component: primary, Primary: true},
		},
		Resources: []lifecycle.NamedResource{
			{Name: "outer", Resource: closerFunc(func() error { record("close-outer"); return nil })},
			{Name: "inner", Resource: closerFunc(func() error { record("close-inner"); return closeErr })},
		},
	})

	for _, cause := range []error{startErr, stopErr, closeErr} {
		if !errors.Is(err, cause) {
			t.Fatalf("Run() error = %v, want cause %v", err, cause)
		}
	}
	want := []string{
		"start-first", "start-second", "start-failing",
		"stop-second", "stop-first",
		"close-inner", "close-outer",
	}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("start-failure unwind = %v, want %v", order, want)
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
