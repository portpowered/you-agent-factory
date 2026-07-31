package service

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
)

// Execute validates and clones the request, runs one private runner attempt,
// normalizes terminal outcomes, delivers safe observations, and releases
// request-scoped resources before returning.
func (s *Service) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (result workers.ExecuteResult, err error) {
	if s == nil || s.runners == nil {
		return workers.ExecuteResult{}, workers.ErrExecuteUnavailable
	}
	if err := ctx.Err(); err != nil {
		return workers.ExecuteResult{}, err
	}
	if err := request.Validate(); err != nil {
		return workers.ExecuteResult{}, err
	}
	request = request.Clone()
	correlation := request.Correlation

	cleanup := newCleanupRegistry()
	defer cleanup.run(s.logger)

	if err := s.prepareWorkspace(ctx, &request, cleanup); err != nil {
		return workers.ExecuteResult{}, err
	}
	identity := resolveRunnerIdentity(request.Target)
	if err := s.authorizeProviderTarget(ctx, &request, identity); err != nil {
		return workers.ExecuteResult{}, err
	}
	// Selection is validated before the attempt starts so missing/unknown
	// identities remain pre-start errors. Resolve performs no execution effect.
	if _, err := s.runners.Resolve(runners.ResolutionRequest{
		Identity:             identity,
		RequiredCapabilities: request.Target.Tools.RequiredOptionalCapabilities,
	}); err != nil {
		return workers.ExecuteResult{}, fmt.Errorf(
			"%w: resolve runner %q: %v",
			workers.ErrInvalidExecuteRequest,
			identity,
			err,
		)
	}

	startedAt := s.clock()
	sequence := atomic.Int64{}
	s.emit(ctx, &sequence, workers.ExecutionObservation{
		Correlation: correlation,
		Kind:        workers.ExecutionObservationKindStarted,
		Timestamp:   startedAt,
		Phase:       "execute.started",
	})
	s.logger.Info(
		"workers execute started",
		"factory_session_id", correlation.FactorySessionID,
		"runtime_id", correlation.RuntimeID,
		"dispatch_id", correlation.DispatchID,
		"attempt_id", correlation.AttemptID,
		"runner_id", request.Target.RunnerID,
	)

	execCtx, cancel := s.withTimeout(ctx, request.Target.Timeout)
	defer cancel()

	var runnerResult workers.RunnerExecutionResult
	var runErr error
	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				runErr = panicFailure(recovered, debug.Stack())
			}
		}()
		adapted := adaptRunnerRequest(request, identity)
		runnerResult, runErr = s.runners.Execute(execCtx, runners.ExecuteRequest{
			Identity:             identity,
			RequiredCapabilities: request.Target.Tools.RequiredOptionalCapabilities,
			Attempt:              adapted,
		})
	}()

	finishedAt := s.clock()
	result = s.normalizeResult(correlation, request, runnerResult, runErr, finishedAt.Sub(startedAt))
	s.emitTerminal(ctx, &sequence, result, finishedAt)
	s.logger.Info(
		"workers execute finished",
		"factory_session_id", correlation.FactorySessionID,
		"runtime_id", correlation.RuntimeID,
		"dispatch_id", correlation.DispatchID,
		"attempt_id", correlation.AttemptID,
		"outcome", string(result.Outcome),
		"duration_ms", finishedAt.Sub(startedAt).Milliseconds(),
	)
	return result.Clone(), nil
}

func (s *Service) withTimeout(
	ctx context.Context,
	timeout time.Duration,
) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

func (s *Service) prepareWorkspace(
	ctx context.Context,
	request *workers.ExecuteRequest,
	cleanup *cleanupRegistry,
) error {
	workspace := request.Target.Workspace
	if !workspace.PrepareWorktree || s.worktree == nil {
		return nil
	}
	factoryDirectory := strings.TrimSpace(workspace.FactoryDirectory)
	checkout := strings.TrimSpace(workspace.CheckoutIdentifier)
	if factoryDirectory == "" || checkout == "" {
		return fmt.Errorf(
			"%w: worktree preparation requires factory directory and checkout identity",
			workers.ErrInvalidExecuteRequest,
		)
	}
	preparation, err := s.worktree.Prepare(ctx, factoryDirectory, checkout)
	if err != nil {
		return fmt.Errorf("%w: prepare worktree: %v", workers.ErrInvalidExecuteRequest, err)
	}
	if path := strings.TrimSpace(preparation.CheckoutPath); path != "" {
		request.Target.Workspace.Worktree = path
		if strings.TrimSpace(request.Target.Workspace.WorkingDirectory) == "" {
			request.Target.Workspace.WorkingDirectory = path
		}
		if strings.TrimSpace(request.Target.Environment.WorkingDirectory) == "" {
			request.Target.Environment.WorkingDirectory = path
			request.Target.Environment.WorkingDirectorySet = true
		}
		cleanup.add(func() error {
			// Worktree leases are released by dropping request-scoped ownership.
			// Concrete checkout deletion remains owned by the Worktree preparer
			// implementation; Execute only guarantees request-end cleanup hooks run.
			return nil
		})
	}
	return nil
}

func (s *Service) emitTerminal(
	ctx context.Context,
	sequence *atomic.Int64,
	result workers.ExecuteResult,
	timestamp time.Time,
) {
	kind := workers.ExecutionObservationKindCompleted
	phase := "execute.completed"
	switch result.Outcome {
	case workers.ExecutionOutcomeFailed:
		kind = workers.ExecutionObservationKindFailed
		phase = "execute.failed"
	case workers.ExecutionOutcomeCanceled:
		kind = workers.ExecutionObservationKindCanceled
		phase = "execute.canceled"
	}
	metadata := map[string]string{"outcome": string(result.Outcome)}
	if result.Failure != nil {
		metadata["failure_type"] = string(result.Failure.Type)
	}
	s.emit(ctx, sequence, workers.ExecutionObservation{
		Correlation: result.Correlation,
		Kind:        kind,
		Timestamp:   timestamp,
		Phase:       phase,
		Metadata:    metadata,
	})
}

func (s *Service) emit(
	ctx context.Context,
	sequence *atomic.Int64,
	observation workers.ExecutionObservation,
) {
	if s == nil || s.observe == nil {
		return
	}
	observation.Sequence = sequence.Add(1)
	observation = observation.Clone()
	if err := s.observe(ctx, observation); err != nil {
		s.logger.Warn(
			"workers observation delivery failed",
			"dispatch_id", observation.Correlation.DispatchID,
			"attempt_id", observation.Correlation.AttemptID,
			"kind", string(observation.Kind),
			"error", err.Error(),
		)
	}
}

func panicFailure(recovered any, stack []byte) error {
	message := fmt.Sprintf("workers execute panic: %v", recovered)
	failure := workers.NewProviderError(
		workers.WorkFailureTypeUnknown,
		message,
		nil,
	)
	failure.Diagnostics = &workers.WorkDiagnostics{
		Panic: &workers.PanicDiagnostic{
			Message: message,
			Stack:   string(stack),
		},
	}
	return failure
}

func errMisconfigured(message string) error {
	return fmt.Errorf("%w: %s", workers.ErrExecuteUnavailable, message)
}
