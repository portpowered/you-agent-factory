package service

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	workstations "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations"
)

type lifecycleState uint8

const (
	stateConstructed lifecycleState = iota
	stateRunning
	stateStopped
)

// Pool owns the explicit lifecycle and immutable route snapshot for one
// Workers workstation capability.
type Pool struct {
	mu     sync.RWMutex
	state  lifecycleState
	routes map[string]workstations.Route
}

var _ workstations.Service = (*Pool)(nil)

// New constructs an inert pool without starting background activity.
func New() *Pool {
	return &Pool{state: stateConstructed}
}

// Start atomically activates the supplied route snapshot.
func (p *Pool) Start(
	ctx context.Context,
	routes []workstations.Route,
) (workers.WorkstationPoolLifecycleOutcome, error) {
	if err := contextError(ctx); err != nil {
		return "", err
	}
	snapshot, err := routeSnapshot(routes)
	if err != nil {
		return "", err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	switch p.state {
	case stateConstructed:
		p.routes = snapshot
		p.state = stateRunning
		return workers.WorkstationPoolLifecycleOutcomeStarted, nil
	case stateRunning:
		return workers.WorkstationPoolLifecycleOutcomeAlreadyRunning, nil
	case stateStopped:
		return "", workers.ErrWorkstationPoolStopped
	default:
		return "", workers.ErrWorkstationPoolUnavailable
	}
}

// Stop closes admission and converges concurrent callers on a terminal state.
func (p *Pool) Stop(ctx context.Context) (workers.WorkstationPoolLifecycleOutcome, error) {
	if err := contextError(ctx); err != nil {
		return "", err
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state == stateStopped {
		return workers.WorkstationPoolLifecycleOutcomeAlreadyStopped, nil
	}
	p.routes = nil
	p.state = stateStopped
	return workers.WorkstationPoolLifecycleOutcomeStopped, nil
}

// Route verifies availability against the active immutable snapshot.
func (p *Pool) Route(ctx context.Context, workstationName string) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	name := strings.TrimSpace(workstationName)
	if name == "" {
		return workers.ErrUnknownWorkstationRoute
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	switch p.state {
	case stateConstructed:
		return workers.ErrWorkstationPoolUnavailable
	case stateStopped:
		return workers.ErrWorkstationPoolStopped
	case stateRunning:
		if _, ok := p.routes[name]; !ok {
			return workers.ErrUnknownWorkstationRoute
		}
		return nil
	default:
		return workers.ErrWorkstationPoolUnavailable
	}
}

// Dispatch resolves one immutable route binding before invoking its executor.
func (p *Pool) Dispatch(
	ctx context.Context,
	request workers.WorkstationDispatchRequest,
) (workers.WorkstationDispatchResult, error) {
	if err := contextError(ctx); err != nil {
		return workers.WorkstationDispatchResult{}, err
	}
	name, err := validDispatch(request)
	if err != nil {
		return workers.WorkstationDispatchResult{}, err
	}

	route, release, err := p.acquireBinding(name)
	if err != nil {
		return workers.WorkstationDispatchResult{}, err
	}
	defer release()
	if route.Executor == nil {
		return workers.WorkstationDispatchResult{}, workers.ErrMissingWorkstationBinding
	}

	execution := workers.CloneWorkstationExecutionRequest(request.Execution)
	execution.RunnerID = route.RunnerSelection.RunnerID
	execution.RunnerSelectionSource = route.RunnerSelection.Source
	result, executeErr := route.Executor.Execute(ctx, execution)
	result.DispatchID = execution.Dispatch.DispatchID
	result.TransitionID = execution.Dispatch.TransitionID
	return workers.WorkstationDispatchResult{
		DispatchID:      execution.Dispatch.DispatchID,
		WorkstationName: name,
		Result:          result,
	}, executeErr
}

func (p *Pool) acquireBinding(
	name string,
) (workstations.Route, func(), error) {
	p.mu.RLock()
	switch p.state {
	case stateConstructed:
		p.mu.RUnlock()
		return workstations.Route{}, nil, workers.ErrWorkstationPoolUnavailable
	case stateStopped:
		p.mu.RUnlock()
		return workstations.Route{}, nil, workers.ErrWorkstationPoolStopped
	case stateRunning:
		route, ok := p.routes[name]
		if !ok {
			p.mu.RUnlock()
			return workstations.Route{}, nil, workers.ErrUnknownWorkstationRoute
		}
		return route, p.mu.RUnlock, nil
	default:
		p.mu.RUnlock()
		return workstations.Route{}, nil, workers.ErrWorkstationPoolUnavailable
	}
}

func validDispatch(request workers.WorkstationDispatchRequest) (string, error) {
	name := strings.TrimSpace(request.WorkstationName)
	dispatch := request.Execution.Dispatch
	if name == "" ||
		strings.TrimSpace(dispatch.DispatchID) == "" ||
		strings.TrimSpace(dispatch.WorkstationName) == "" ||
		dispatch.WorkstationName != name {
		return "", workers.ErrInvalidWorkstationDispatch
	}
	return name, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return workers.ErrWorkstationPoolUnavailable
	}
	return ctx.Err()
}

func routeSnapshot(routes []workstations.Route) (map[string]workstations.Route, error) {
	if len(routes) == 0 {
		return nil, workers.ErrInvalidWorkstationPoolStart
	}
	snapshot := make(map[string]workstations.Route, len(routes))
	for _, route := range routes {
		name := strings.TrimSpace(route.WorkstationName)
		if name == "" {
			return nil, workers.ErrInvalidWorkstationPoolStart
		}
		if _, duplicate := snapshot[name]; duplicate {
			return nil, fmt.Errorf(
				"%w: duplicate route %q",
				workers.ErrInvalidWorkstationPoolStart,
				name,
			)
		}
		route.WorkstationName = name
		snapshot[name] = route
	}
	return snapshot, nil
}
