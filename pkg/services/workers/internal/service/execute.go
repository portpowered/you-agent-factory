package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/execution"
	workerrecording "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/execution/recording"
	workerexecutor "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/executor"
)

const (
	detachedProviderMaxRetries     = 2
	detachedProviderInitialBackoff = 100 * time.Millisecond
)

// Execute validates and clones the request, runs one private runner attempt,
// normalizes terminal outcomes, delivers safe observations, and releases
// request-scoped resources before returning.
func (s *Service) Execute(
	ctx context.Context,
	request workers.ExecuteRequest,
) (workers.ExecuteResult, error) {
	if s == nil || s.runners == nil {
		return workers.ExecuteResult{}, workers.ErrExecuteUnavailable
	}
	if ctx == nil {
		return workers.ExecuteResult{}, fmt.Errorf(
			"%w: context is required",
			workers.ErrInvalidExecuteRequest,
		)
	}
	if err := ctx.Err(); err != nil {
		return workers.ExecuteResult{}, err
	}
	// Detach all caller-owned mutable data before validation or any downstream
	// selection. The request snapshot is the only input the attempt may use.
	request = request.Clone()
	if err := request.Validate(); err != nil {
		return workers.ExecuteResult{}, err
	}
	if !request.Target.Environment.SkipProcessInheritance &&
		len(request.Target.Environment.ProcessEnvironment) == 0 {
		request.Target.Environment.ProcessEnvironment = os.Environ()
	}
	correlation := request.Correlation
	if request.Target.Noop {
		return workers.ExecuteResult{
			Correlation: correlation,
			Outcome:     workers.ExecutionOutcomeAccepted,
		}, nil
	}

	cleanup := newCleanupRegistry()
	temporaryFiles := newTrackedTemporaryFiles(s.temporaryFiles)
	if temporaryFiles != nil {
		cleanup.add(temporaryFiles.Cleanup)
	}

	identity, err := s.prepareAttempt(ctx, &request, cleanup)
	if err != nil {
		return workers.ExecuteResult{}, s.preStartError(ctx, cleanup, err)
	}
	if request.Input.PreparedRequestObserver != nil {
		request.Input.PreparedRequestObserver(request)
	}
	return s.executeStarted(ctx, request, identity, correlation, cleanup, temporaryFiles)
}

func (s *Service) prepareAttempt(
	ctx context.Context,
	request *workers.ExecuteRequest,
	cleanup *cleanupRegistry,
) (string, error) {
	if err := s.prepareWorkspace(ctx, request, cleanup); err != nil {
		return "", err
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return "", contextErr
	}
	identity := resolveRunnerIdentity(request.Target)
	request.Target.Tools.RequiredOptionalCapabilities = requiredOptionalCapabilities(*request, identity)
	if err := s.authorizeProviderTarget(ctx, request, identity); err != nil {
		return "", err
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return "", contextErr
	}
	// Selection is validated before the attempt starts so missing/unknown
	// identities remain pre-start errors. Resolve performs no execution effect.
	if _, err := s.runners.Resolve(runners.ResolutionRequest{
		Identity: identity,
		RequiredCapabilities: runnerResolutionCapabilities(
			request.Target.Tools.RequiredOptionalCapabilities,
			identity,
		),
	}); err != nil {
		return "", fmt.Errorf(
			"%w: resolve runner %q: %v",
			workers.ErrInvalidExecuteRequest,
			identity,
			err,
		)
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return "", contextErr
	}
	return identity, nil
}

func (s *Service) preStartError(
	ctx context.Context,
	cleanup *cleanupRegistry,
	executeErr error,
) error {
	cleanupErr := cleanup.run(s.logger)
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	return errors.Join(executeErr, cleanupErr)
}

func (s *Service) executeStarted(
	ctx context.Context,
	request workers.ExecuteRequest,
	identity string,
	correlation workers.ExecutionCorrelation,
	cleanup *cleanupRegistry,
	temporaryFiles workers.TemporaryFileSystem,
) (workers.ExecuteResult, error) {
	defer cleanup.run(s.logger)

	startedAt := s.clock()
	sequence := atomic.Int64{}
	observationContext := context.WithoutCancel(ctx)
	s.emit(observationContext, &sequence, workers.ExecutionObservation{
		Correlation: correlation,
		Kind:        workers.ExecutionObservationKindStarted,
		Timestamp:   startedAt,
		Phase:       "execute.started",
	})
	s.logger.Info(
		"workers execute started",
		"factory_session_id", correlation.FactorySessionID,
		"runtime_id", correlation.RuntimeID,
		"generation_id", correlation.GenerationID,
		"dispatch_id", correlation.DispatchID,
		"attempt_id", correlation.AttemptID,
		"runner_id", request.Target.RunnerID,
	)

	execCtx, cancel := s.withTimeout(ctx, request.Target.Timeout)
	defer cancel()

	runnerResult, runErr := s.runRunner(execCtx, request, identity, temporaryFiles)

	if contextErr := execCtx.Err(); contextErr != nil {
		var providerErr *workers.ProviderError
		// Runner-owned cancellation errors carry the canonical provider
		// message and should survive the context deadline/cancellation check.
		// A raw runner error is still normalized to the authoritative context
		// error so cancellation cannot be reported as an arbitrary process
		// failure.
		if !errors.As(runErr, &providerErr) || providerErr == nil {
			runErr = contextErr
		}
	}
	cleanupErr := cleanup.run(s.logger)
	if cleanupErr != nil {
		runErr = errors.Join(runErr, cleanupFailure(cleanupErr))
	}
	finishedAt := s.clock()
	result := s.normalizeResult(correlation, request, runnerResult, runErr, finishedAt.Sub(startedAt))
	s.emitTerminal(observationContext, &sequence, result, finishedAt)
	s.logger.Info(
		"workers execute finished",
		"factory_session_id", correlation.FactorySessionID,
		"runtime_id", correlation.RuntimeID,
		"generation_id", correlation.GenerationID,
		"dispatch_id", correlation.DispatchID,
		"attempt_id", correlation.AttemptID,
		"outcome", string(result.Outcome),
		"duration_ms", finishedAt.Sub(startedAt).Milliseconds(),
	)
	return result.Clone(), nil
}

func (s *Service) runRunner(
	ctx context.Context,
	request workers.ExecuteRequest,
	identity string,
	temporaryFiles workers.TemporaryFileSystem,
) (runnerResult workers.RunnerExecutionResult, runErr error) {
	providerOverride := request.Input.ProviderOverride
	if providerOverride == nil {
		providerOverride = s.providerOverride
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			runErr = panicFailure(
				recovered,
				debug.Stack(),
				providerOverride != nil && identity == runners.AgentIdentity,
			)
		}
	}()
	if request.Input.MockWorkers != nil {
		ctx = workerexecution.WithMockWorkersConfig(ctx, request.Input.MockWorkers)
		ctx = workerexecution.WithMockWorkerOutputPolicy(ctx, request.Target.Output)
	}
	if request.Input.ProgressPublisher != nil {
		ctx = workerexecution.WithProgressPublisher(ctx, request.Input.ProgressPublisher)
	}
	if request.Input.ScriptEventRecorder != nil {
		ctx = workerexecution.WithScriptEventRecorder(ctx, request.Input.ScriptEventRecorder)
	}
	if request.Input.CommandRunnerOverride != nil {
		ctx = workerexecution.WithCommandRunnerOverride(ctx, request.Input.CommandRunnerOverride)
	}
	runnerRequest := adaptRunnerRequest(request, identity, temporaryFiles)
	if request.Target.Tools.AgentLoop {
		return s.runAgentLoop(ctx, request, identity, runnerRequest, providerOverride)
	}
	if providerOverride != nil && identity == runners.AgentIdentity &&
		!usesACPProvider(runnerRequest.ExecutorProvider) {
		providerRunner := workerrecording.NewProviderRunner(
			workerexecutor.RunnerFromProvider(providerOverride),
			request.Input.InferenceEventRecorder,
			s.clock,
		)
		return s.executeProviderWithRetry(ctx, runnerRequest, func(
			attempt workers.RunnerExecutionRequest,
		) (workers.RunnerExecutionResult, error) {
			result, err := providerRunner.Execute(ctx, attempt)
			return normalizeProviderOverrideResult(result, attempt), err
		})
	}
	if identity != runners.AgentIdentity {
		return s.runners.Execute(ctx, runners.ExecuteRequest{
			Identity:             identity,
			RequiredCapabilities: request.Target.Tools.RequiredOptionalCapabilities,
			Attempt:              runnerRequest,
		})
	}
	return s.executeProviderWithRetry(ctx, runnerRequest, func(
		attempt workers.RunnerExecutionRequest,
	) (workers.RunnerExecutionResult, error) {
		return s.runners.Execute(ctx, runners.ExecuteRequest{
			Identity: identity,
			RequiredCapabilities: runnerResolutionCapabilities(
				attempt.RequiredOptionalCapabilities,
				identity,
			),
			Attempt: attempt,
		})
	})
}

// executeProviderWithRetry preserves the provider-attempt policy at the
// stateless Workers boundary. Runtime still owns Work-level attempts; this
// loop only retries provider failures that the shared failure classifier
// declares retryable, and carries the exact opaque Provider Session into the
// next provider call.
//
// The classifier is the single authority for retryability. A provider process
// that failed or exited non-zero without a retryable classification is a
// terminal external effect: retrying it burns the Work's attempt budget and
// re-runs a real provider command that already reported a definitive failure.
func (s *Service) executeProviderWithRetry(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
	execute func(workers.RunnerExecutionRequest) (workers.RunnerExecutionResult, error),
) (workers.RunnerExecutionResult, error) {
	for retryCount := 0; ; retryCount++ {
		result, err := execute(request)
		if err == nil {
			return result, nil
		}

		providerErr := workers.NormalizeProviderExecutionError(err)
		if providerErr == nil || !retryableProviderFailure(providerErr) || retryCount >= detachedProviderMaxRetries {
			return result, err
		}

		if session := providerSessionForRetry(providerErr, result); session != nil {
			request.SessionID = strings.TrimSpace(session.ID)
			request.RequiredOptionalCapabilities = appendRunnerCapabilityIfMissing(
				request.RequiredOptionalCapabilities,
				workers.RunnerOptionalCapabilitySessionResume,
			)
		}
		if err := sleepForDetachedProviderRetry(ctx, detachedProviderInitialBackoff<<retryCount); err != nil {
			return result, err
		}
	}
}

func retryableProviderFailure(providerErr *workers.ProviderError) bool {
	if providerErr == nil {
		return false
	}
	return workers.WorkFailureDecisionFromProviderError(providerErr).Retryable
}

func providerSessionForRetry(
	providerErr *workers.ProviderError,
	result workers.RunnerExecutionResult,
) *workers.ProviderSessionMetadata {
	if providerErr != nil && providerErr.ProviderSession != nil &&
		strings.TrimSpace(providerErr.ProviderSession.ID) != "" {
		return workers.CloneProviderSessionMetadata(providerErr.ProviderSession)
	}
	if result.ProviderSession != nil && strings.TrimSpace(result.ProviderSession.ID) != "" {
		return workers.CloneProviderSessionMetadata(result.ProviderSession)
	}
	return nil
}

func sleepForDetachedProviderRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
	if !workspace.PrepareWorktree {
		return nil
	}
	if s.worktree == nil || s.worktreeRelease == nil {
		return fmt.Errorf(
			"%w: worktree preparer and releaser are required when worktree preparation is enabled",
			workers.ErrInvalidExecuteRequest,
		)
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
		if strings.TrimSpace(request.Target.Workspace.WorkingDirectory) == "" {
			request.Target.Workspace.WorkingDirectory = path
		}
		if strings.TrimSpace(request.Target.Environment.WorkingDirectory) == "" {
			request.Target.Environment.WorkingDirectory = path
			request.Target.Environment.WorkingDirectorySet = true
		}
		if !preparation.Reused && !workspace.RetainWorktree {
			preparation := preparation
			cleanup.add(func() error {
				return s.worktreeRelease(context.WithoutCancel(ctx), preparation)
			})
		}
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
	if err := deliverObservation(s.observe, context.WithoutCancel(ctx), observation); err != nil {
		s.logger.Warn(
			"workers observation delivery failed",
			"dispatch_id", observation.Correlation.DispatchID,
			"attempt_id", observation.Correlation.AttemptID,
			"kind", string(observation.Kind),
			"error", err.Error(),
		)
	}
}

func deliverObservation(
	sink workers.ObservationSink,
	ctx context.Context,
	observation workers.ExecutionObservation,
) (err error) {
	if sink == nil {
		return nil
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New("Workers observation sink panicked")
		}
	}()
	return sink(ctx, observation)
}

func panicFailure(recovered any, stack []byte, preserveCompatibilityCause bool) error {
	message := "worker runner panicked"
	var cause error
	if preserveCompatibilityCause {
		cause = &workers.WorkerExecutorPanicError{Cause: recovered}
	}
	failure := workers.NewProviderError(
		workers.WorkFailureTypeUnknown,
		message,
		cause,
	)
	failure.Diagnostics = &workers.WorkDiagnostics{
		Panic: &workers.PanicDiagnostic{
			Message: message,
			Stack:   boundedPanicStack(stack),
		},
	}
	return failure
}

func boundedPanicStack(stack []byte) string {
	const maxPanicStackBytes = 4096
	if len(stack) > maxPanicStackBytes {
		stack = stack[:maxPanicStackBytes]
	}
	return string(stack)
}

func cleanupFailure(err error) error {
	return workers.NewProviderError(
		workers.WorkFailureTypeInternalServerError,
		"execution cleanup failed",
		errors.Join(workers.ErrExecuteCleanupFailed, err),
	)
}

func errMisconfigured(message string) error {
	return fmt.Errorf("%w: %s", workers.ErrExecuteUnavailable, message)
}
