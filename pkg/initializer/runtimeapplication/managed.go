package runtimeapplication

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/portpowered/infinite-you/pkg/initializer"
	"github.com/portpowered/infinite-you/pkg/initializer/lifecycle"
	"github.com/portpowered/infinite-you/pkg/platform/runtimeartifact"
)

// ManagedRunner executes one caller-supplied lifecycle plan through the
// singular manager.
type ManagedRunner struct {
	manager     *lifecycle.Manager
	plan        lifecycle.Plan
	diagnostics runtimeartifact.Diagnostics
	ready       <-chan initializer.RuntimeHostBinding
	readyMu     sync.Mutex
	readyValue  *initializer.RuntimeHostBinding
}

// ManagedRunnerFactory is the injected lifecycle activation constructor used
// by application operations after a session-owned runtime has been opened.
type ManagedRunnerFactory func(lifecycle.Plan, runtimeartifact.Diagnostics) (*ManagedRunner, error)

func NewManagedRunner(
	plan lifecycle.Plan,
	diagnostics runtimeartifact.Diagnostics,
) (*ManagedRunner, error) {
	if err := lifecycle.Validate(plan); err != nil {
		return nil, fmt.Errorf("initialize application: %w", err)
	}
	return &ManagedRunner{
		manager:     lifecycle.NewManager(),
		plan:        plan,
		diagnostics: diagnostics,
	}, nil
}

func (r *ManagedRunner) Run(ctx context.Context) error {
	if r == nil || r.manager == nil {
		return fmt.Errorf("managed application is required")
	}
	return r.manager.Run(ctx, r.plan)
}

// SetRuntimeHostReady supplies the detached readiness stream returned by an
// opened application. It is called by the initializer builder before the
// runner is handed to a transport operation.
func (r *ManagedRunner) SetRuntimeHostReady(ready <-chan initializer.RuntimeHostBinding) {
	if r == nil {
		return
	}
	r.ready = ready
}

// RuntimeHostReadinessConfigured reports whether the opened application has
// an externally hosted endpoint whose readiness can be observed.
func (r *ManagedRunner) RuntimeHostReadinessConfigured() bool {
	return r != nil && r.ready != nil
}

// RuntimeHostBinding waits for and returns the endpoint published by the
// hosted transport. The first observation is retained for later transport
// presentation without exposing the channel to product services.
func (r *ManagedRunner) RuntimeHostBinding(ctx context.Context) (initializer.RuntimeHostBinding, error) {
	if r == nil {
		return initializer.RuntimeHostBinding{}, errors.New("managed application is required")
	}
	r.readyMu.Lock()
	if r.readyValue != nil {
		binding := *r.readyValue
		r.readyMu.Unlock()
		return binding, nil
	}
	ready := r.ready
	r.readyMu.Unlock()
	if ready == nil {
		return initializer.RuntimeHostBinding{}, initializer.ErrRuntimeHostReadinessUnavailable
	}
	select {
	case binding, ok := <-ready:
		if !ok {
			return initializer.RuntimeHostBinding{}, errors.New("managed application host readiness ended without a binding")
		}
		r.readyMu.Lock()
		if r.readyValue == nil {
			copy := binding
			r.readyValue = &copy
		}
		binding = *r.readyValue
		r.readyMu.Unlock()
		return binding, nil
	default:
	}
	select {
	case binding, ok := <-ready:
		if !ok {
			return initializer.RuntimeHostBinding{}, errors.New("managed application host readiness ended without a binding")
		}
		r.readyMu.Lock()
		if r.readyValue == nil {
			copy := binding
			r.readyValue = &copy
		}
		binding = *r.readyValue
		r.readyMu.Unlock()
		return binding, nil
	case <-ctx.Done():
		return initializer.RuntimeHostBinding{}, ctx.Err()
	}
}

// RunWithCompletion owns the hosted one-shot ordering at the initializer
// boundary. The transport starts first, completion waits for the detached
// host binding, and either side finishing cancels and joins the other.
func (r *ManagedRunner) RunWithCompletion(
	ctx context.Context,
	completion initializer.CompletionOperation,
) error {
	if r == nil || r.manager == nil {
		return fmt.Errorf("managed application is required")
	}
	if completion == nil {
		return errors.New("managed application completion is required")
	}
	primaryIndex := -1
	for index, candidate := range r.plan.Components {
		if candidate.Primary {
			primaryIndex = index
			break
		}
	}
	if primaryIndex < 0 {
		return errors.New("managed application primary component is required")
	}
	components := append([]lifecycle.NamedComponent(nil), r.plan.Components...)
	components[primaryIndex].Component = newCompletionTransport(
		components[primaryIndex].Component,
		func(ctx context.Context) error {
			if r.ready != nil {
				if _, err := r.RuntimeHostBinding(ctx); err != nil {
					return err
				}
			}
			return completion(ctx)
		},
	)
	return r.manager.Run(ctx, lifecycle.Plan{
		Components: components,
		Resources:  r.plan.Resources,
	})
}

func (r *ManagedRunner) RuntimeLogDiagnostics() runtimeartifact.Diagnostics {
	if r == nil {
		return runtimeartifact.Diagnostics{}
	}
	return r.diagnostics
}

type completionTransport struct {
	transport  lifecycle.Component
	completion *lifecycle.Runner

	mu     sync.Mutex
	runCtx context.Context
	cancel context.CancelFunc
}

type completionTransportResult struct {
	name string
	err  error
}

func newCompletionTransport(
	transport lifecycle.Component,
	completion func(context.Context) error,
) *completionTransport {
	return &completionTransport{
		transport:  transport,
		completion: lifecycle.NewRunner(completion),
	}
}

func (component *completionTransport) Start(ctx context.Context) error {
	runCtx, cancel := context.WithCancel(ctx)
	component.mu.Lock()
	component.runCtx = runCtx
	component.cancel = cancel
	component.mu.Unlock()
	if err := component.transport.Start(runCtx); err != nil {
		cancel()
		return err
	}
	if err := component.completion.Start(runCtx); err != nil {
		cancel()
		return errors.Join(err, component.transport.Stop(context.Background()))
	}
	return nil
}

func (component *completionTransport) Wait(ctx context.Context) error {
	transport, ok := component.transport.(lifecycle.Waiter)
	if !ok {
		return errors.New("application transport is not waitable")
	}
	component.mu.Lock()
	runCtx := component.runCtx
	component.mu.Unlock()
	if runCtx == nil {
		runCtx = ctx
	}
	results := make(chan completionTransportResult, 2)
	go func() { results <- completionTransportResult{name: "transport", err: transport.Wait(runCtx)} }()
	go func() {
		results <- completionTransportResult{name: "completion", err: component.completion.Wait(runCtx)}
	}()
	first := <-results
	if first.name == "transport" && first.err == nil {
		// A successful transport completion is the normal finite-run signal.
		// Let the completion operation observe the terminal runtime and publish
		// its result before lifecycle shutdown invalidates the bound runtime
		// service. Errors and cancellation still take the existing fast
		// cancellation path below.
		second := <-results
		component.cancelRun()
		return joinCompletionTransportResults(first, second)
	}
	component.cancelRun()
	second := <-results
	return joinCompletionTransportResults(first, second)
}

func (component *completionTransport) Stop(ctx context.Context) error {
	component.cancelRun()
	return errors.Join(component.completion.Stop(ctx), component.transport.Stop(ctx))
}

func (component *completionTransport) cancelRun() {
	component.mu.Lock()
	cancel := component.cancel
	component.runCtx = nil
	component.cancel = nil
	component.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func joinCompletionTransportResults(first, second completionTransportResult) error {
	var errs []error
	for _, candidate := range []completionTransportResult{first, second} {
		if candidate.err != nil && !errors.Is(candidate.err, context.Canceled) {
			errs = append(errs, fmt.Errorf("%s: %w", candidate.name, candidate.err))
		}
	}
	return errors.Join(errs...)
}
