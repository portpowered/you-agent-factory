package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

var (
	// ErrAttemptCapacityExceeded reports that Runtime cannot admit another
	// Execute call until an active attempt reaches a terminal boundary.
	ErrAttemptCapacityExceeded = errors.New("Factory Runtime worker-attempt capacity is full")
	// ErrAttemptLifecycleUnavailable reports an attempt lifecycle that was not
	// constructed with the required stateless execution capability.
	ErrAttemptLifecycleUnavailable = errors.New("Factory Runtime worker-attempt lifecycle is unavailable")
)

const defaultRuntimeAttemptCapacity = 64

type attemptTerminalFunc func(context.Context, workers.ExecuteRequest, workers.ExecuteResult, error)

type activeAttempt struct {
	dispatchID string
	attemptID  string
	cancel     context.CancelFunc
	done       chan struct{}
	canceled   bool
}

// attemptLifecycle is the Runtime-owned lifecycle boundary for stateless
// Worker execution. It deliberately retains only cancellation and correlation
// state; Workers receives a detached ExecuteRequest and returns a detached
// ExecuteResult.
type attemptLifecycle struct {
	mu       sync.Mutex
	service  workers.ExecuteService
	newID    factory.IDGenerator
	capacity int
	active   map[string]*activeAttempt
	terminal map[string]string
}

func newAttemptLifecycle(service workers.ExecuteService, newID factory.IDGenerator, capacity int) *attemptLifecycle {
	if capacity <= 0 {
		capacity = defaultRuntimeAttemptCapacity
	}
	return &attemptLifecycle{
		service:  service,
		newID:    newID,
		capacity: capacity,
		active:   make(map[string]*activeAttempt),
		terminal: make(map[string]string),
	}
}

// start admits one request and invokes its terminal callback exactly once.
// A full lifecycle is removed from active state before the callback is
// delivered, so result application cannot hold the admission lock.
func (l *attemptLifecycle) start(
	ctx context.Context,
	request workers.ExecuteRequest,
	async bool,
	terminal attemptTerminalFunc,
) error {
	return l.startWithRetry(ctx, request, async, terminal, false)
}

// startRetry admits a new Attempt ID for an already-terminal dispatch. The
// Runtime uses this only for an explicit caller-requested retry; ordinary
// duplicate dispatch publication remains idempotent through start.
func (l *attemptLifecycle) startRetry(
	ctx context.Context,
	request workers.ExecuteRequest,
	async bool,
	terminal attemptTerminalFunc,
) error {
	return l.startWithRetry(ctx, request, async, terminal, true)
}

func (l *attemptLifecycle) startWithRetry(
	ctx context.Context,
	request workers.ExecuteRequest,
	async bool,
	terminal attemptTerminalFunc,
	allowRetry bool,
) error {
	if l == nil || l.service == nil {
		return ErrAttemptLifecycleUnavailable
	}
	if ctx == nil {
		return fmt.Errorf("start worker attempt: context is required")
	}
	if terminal == nil {
		return fmt.Errorf("start worker attempt: terminal callback is required")
	}
	request = request.Clone()
	request.Correlation.DispatchID = strings.TrimSpace(request.Correlation.DispatchID)
	request.Correlation.AttemptID = strings.TrimSpace(request.Correlation.AttemptID)
	if request.Correlation.AttemptID == "" {
		if l.newID == nil {
			return fmt.Errorf("start worker attempt: Attempt ID generator is required")
		}
		request.Correlation.AttemptID = strings.TrimSpace(l.newID())
		if request.Correlation.AttemptID == "" {
			return fmt.Errorf("start worker attempt: Attempt ID generator returned an empty ID")
		}
	}
	if err := request.Validate(); err != nil {
		return err
	}
	if request.Attempt.Number <= 0 {
		request.Attempt.Number = 1
	}

	execCtx, cancel := context.WithCancel(ctx)
	attempt := &activeAttempt{
		dispatchID: request.Correlation.DispatchID,
		attemptID:  request.Correlation.AttemptID,
		cancel:     cancel,
		done:       make(chan struct{}),
	}
	l.mu.Lock()
	if _, exists := l.active[attempt.dispatchID]; exists {
		l.mu.Unlock()
		cancel()
		return nil
	}
	terminalAttemptID, terminalAlreadyApplied := l.terminal[attempt.dispatchID]
	if terminalAlreadyApplied &&
		(!allowRetry || terminalAttemptID == attempt.attemptID) {
		l.mu.Unlock()
		cancel()
		return nil
	}
	if len(l.active) >= l.capacity {
		l.mu.Unlock()
		cancel()
		return ErrAttemptCapacityExceeded
	}
	l.active[attempt.dispatchID] = attempt
	l.mu.Unlock()

	run := func() {
		result, err := l.executeSafely(execCtx, request)
		applied, canceled := l.finish(attempt)
		if !applied {
			close(attempt.done)
			return
		}
		defer close(attempt.done)
		if canceled {
			result = canceledAttemptResult(request, result)
			err = nil
		}
		terminal(context.Background(), request, result, err)
	}
	if async {
		go run()
		return nil
	}
	run()
	return nil
}

func (l *attemptLifecycle) executeSafely(
	ctx context.Context,
	request workers.ExecuteRequest,
) (result workers.ExecuteResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			result = workers.ExecuteResult{
				Correlation: request.Correlation,
				Outcome:     workers.ExecutionOutcomeFailed,
				Failure: &workers.ExecutionFailure{
					Type:    workers.WorkFailureTypeUnknown,
					Family:  workers.WorkFailureFamilyTerminal,
					Message: "worker execution panicked",
				},
			}
			// Keep panic detail out of the detached customer-facing result.
			err = nil
		}
		result = normalizeAttemptResult(request, result, err)
		if result.Outcome == workers.ExecutionOutcomeCanceled {
			err = nil
		}
	}()
	result, err = l.service.Execute(ctx, request)
	return result, err
}

func normalizeAttemptResult(
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
	executeErr error,
) workers.ExecuteResult {
	if result.Correlation.DispatchID == "" {
		result.Correlation = request.Correlation
	}
	if result.Correlation.AttemptID == "" {
		result.Correlation.AttemptID = request.Correlation.AttemptID
	}
	if result.Correlation.FactorySessionID == "" {
		result.Correlation.FactorySessionID = request.Correlation.FactorySessionID
	}
	if result.Correlation.RuntimeID == "" {
		result.Correlation.RuntimeID = request.Correlation.RuntimeID
	}
	if result.Correlation.RequestID == "" {
		result.Correlation.RequestID = request.Correlation.RequestID
	}
	if result.Correlation.TraceID == "" {
		result.Correlation.TraceID = request.Correlation.TraceID
	}
	if result.Correlation.DispatchID != request.Correlation.DispatchID ||
		result.Correlation.AttemptID != request.Correlation.AttemptID ||
		correlationValueConflicts(result.Correlation.FactorySessionID, request.Correlation.FactorySessionID) ||
		correlationValueConflicts(result.Correlation.RuntimeID, request.Correlation.RuntimeID) ||
		correlationValueConflicts(result.Correlation.RequestID, request.Correlation.RequestID) ||
		correlationValueConflicts(result.Correlation.TraceID, request.Correlation.TraceID) {
		result = workers.ExecuteResult{
			Correlation: request.Correlation,
			Outcome:     workers.ExecutionOutcomeFailed,
			Failure: &workers.ExecutionFailure{
				Type:    workers.WorkFailureTypeUnknown,
				Family:  workers.WorkFailureFamilyTerminal,
				Message: "worker execution returned conflicting correlation",
			},
		}
		return result
	}
	if executeErr == nil && result.Outcome != "" {
		return result
	}
	if errors.Is(executeErr, context.Canceled) ||
		errors.Is(executeErr, workers.ErrWorkstationDispatchCanceled) ||
		result.Outcome == workers.ExecutionOutcomeCanceled {
		result.Correlation = request.Correlation
		result.Outcome = workers.ExecutionOutcomeCanceled
		if result.Failure == nil {
			result.Failure = &workers.ExecutionFailure{
				Type:    workers.WorkFailureTypeUnknown,
				Family:  workers.WorkFailureFamilyTerminal,
				Message: "execution canceled",
			}
		}
		return result
	}
	if executeErr != nil {
		result.Correlation = request.Correlation
		result.Outcome = workers.ExecutionOutcomeFailed
		if result.Failure == nil {
			result.Failure = &workers.ExecutionFailure{
				Type:    workers.WorkFailureTypeUnknown,
				Family:  workers.WorkFailureFamilyTerminal,
				Message: executeErr.Error(),
			}
		}
	} else if result.Outcome == "" {
		result.Correlation = request.Correlation
		result.Outcome = workers.ExecutionOutcomeFailed
		result.Failure = &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypeUnknown,
			Family:  workers.WorkFailureFamilyTerminal,
			Message: "worker execution returned no terminal outcome",
		}
	}
	return result
}

func canceledAttemptResult(
	request workers.ExecuteRequest,
	result workers.ExecuteResult,
) workers.ExecuteResult {
	result.Correlation = request.Correlation
	result.Outcome = workers.ExecutionOutcomeCanceled
	if result.Failure == nil {
		result.Failure = &workers.ExecutionFailure{
			Type:    workers.WorkFailureTypeUnknown,
			Family:  workers.WorkFailureFamilyTerminal,
			Message: "execution canceled",
		}
	}
	return result
}

func correlationValueConflicts(actual, expected string) bool {
	return strings.TrimSpace(actual) != "" &&
		strings.TrimSpace(expected) != "" &&
		strings.TrimSpace(actual) != strings.TrimSpace(expected)
}

func (l *attemptLifecycle) finish(attempt *activeAttempt) (bool, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	current, exists := l.active[attempt.dispatchID]
	if !exists || current != attempt {
		return false, false
	}
	delete(l.active, attempt.dispatchID)
	l.terminal[attempt.dispatchID] = attempt.attemptID
	return true, attempt.canceled
}

func (l *attemptLifecycle) cancel(
	ctx context.Context,
	dispatchID string,
) (workers.WorkstationDispatchCancelOutcome, error) {
	if l == nil {
		return "", ErrAttemptLifecycleUnavailable
	}
	if ctx == nil {
		return "", fmt.Errorf("cancel worker attempt: context is required")
	}
	dispatchID = strings.TrimSpace(dispatchID)
	if dispatchID == "" {
		return "", fmt.Errorf("cancel worker attempt: dispatch ID is required")
	}
	l.mu.Lock()
	attempt := l.active[dispatchID]
	_, wasTerminal := l.terminal[dispatchID]
	if attempt != nil {
		if attempt.canceled {
			l.mu.Unlock()
			return workers.WorkstationDispatchCancelOutcomeAlreadyCanceled, nil
		}
		attempt.canceled = true
	}
	l.mu.Unlock()
	if attempt == nil {
		if wasTerminal {
			return workers.WorkstationDispatchCancelOutcomeAlreadyTerminal, nil
		}
		return "", fmt.Errorf("cancel worker attempt %q: dispatch is unknown", dispatchID)
	}
	attempt.cancel()
	return workers.WorkstationDispatchCancelOutcomeCanceled, nil
}

func (l *attemptLifecycle) stop(ctx context.Context) error {
	if l == nil {
		return nil
	}
	if ctx == nil {
		return fmt.Errorf("stop worker attempts: context is required")
	}
	l.mu.Lock()
	attempts := make([]*activeAttempt, 0, len(l.active))
	for _, attempt := range l.active {
		attempt.canceled = true
		attempts = append(attempts, attempt)
	}
	l.mu.Unlock()
	for _, attempt := range attempts {
		attempt.cancel()
	}
	for _, attempt := range attempts {
		select {
		case <-attempt.done:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

func (l *attemptLifecycle) activeCount() int {
	if l == nil {
		return 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.active)
}

func (l *attemptLifecycle) terminalAttemptID(dispatchID string) (string, bool) {
	if l == nil {
		return "", false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	attemptID, ok := l.terminal[strings.TrimSpace(dispatchID)]
	return attemptID, ok
}
