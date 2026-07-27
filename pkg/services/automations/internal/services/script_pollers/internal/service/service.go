package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"

	automations "github.com/portpowered/infinite-you/pkg/services/automations"
	scriptpollers "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/script_pollers"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const scriptPollerRestartBackoffMax = 250 * time.Millisecond

type service struct {
	dependencies scriptpollers.Dependencies
}

var _ scriptpollers.Service = (*service)(nil)

// New constructs an inert script-poller service with injected runtime
// dependencies. Construction never invokes the supplied functions.
func New(dependencies scriptpollers.Dependencies) scriptpollers.Service {
	return &service{dependencies: dependencies}
}

func (s *service) StartScriptPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeCfg factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	workerDef *factorydefinitions.FactoryWorkerConfig,
	submitter automations.WorkRequestSubmitter,
) {
	if sidecars == nil || submitter == nil {
		return
	}
	sidecars.Add(1)
	go func() {
		defer sidecars.Done()
		s.superviseScriptPoller(ctx, runtimeCfg, workstation, workerDef, submitter)
	}()
}

func (s *service) superviseScriptPoller(
	ctx context.Context,
	runtimeCfg factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	workerDef *factorydefinitions.FactoryWorkerConfig,
	submitter automations.WorkRequestSubmitter,
) {
	logger := s.pollerLogger(workstation.Name, workerDef.Name)
	runner := s.commandRunner()
	backoffClock := s.supervisorClock()
	attempt := 0
	logger.Info("script poller started")
	defer func() {
		logger.Info("script poller stopped", zap.String("reason", scriptPollerStopReason(ctx.Err())))
	}()

	for {
		if ctx.Err() != nil {
			return
		}

		attempt++
		runErr := s.RunScriptPoller(ctx, runner, runtimeCfg, workstation, workerDef, submitter)
		if ctx.Err() != nil {
			return
		}

		backoff := scriptPollerRestartBackoff(attempt)
		logger.Warn("script poller restarting",
			zap.Int("attempt", attempt),
			zap.Duration("backoff", backoff),
			zap.Error(runErr),
		)

		select {
		case <-ctx.Done():
			return
		case <-backoffClock.After(backoff):
		}
	}
}

func (s *service) RunScriptPoller(
	ctx context.Context,
	runner workers.CommandRunner,
	runtimeCfg factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	workerDef *factorydefinitions.FactoryWorkerConfig,
	submitter automations.WorkRequestSubmitter,
) error {
	commandReq, err := scriptpollers.ScriptPollerCommandRequest(
		runtimeCfg,
		workstation,
		workerDef,
		s.dependencies.ResolveTemplates,
	)
	if err != nil {
		return err
	}

	execCtx := ctx
	timeout, err := s.scriptPollerExecutionTimeout(workstation, workerDef)
	if err != nil {
		return err
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	result, runErr := runner.Run(execCtx, commandReq)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if runErr != nil {
		if errors.Is(runErr, context.DeadlineExceeded) {
			return fmt.Errorf("script poller timed out after %s", timeout)
		}
		return fmt.Errorf("script poller execution failed: %w", runErr)
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("script poller exited with code %d", result.ExitCode)
	}
	request, hasOutput, err := scriptpollers.ParseScriptPollerOutput(result.Stdout)
	if err != nil {
		return err
	}
	if hasOutput {
		if submitter == nil {
			return fmt.Errorf("script poller submitter is not available")
		}
		if err := submitter(ctx, request); err != nil {
			return scriptpollers.SubmitFailedError(err)
		}
	}
	return fmt.Errorf("script poller exited unexpectedly")
}

func scriptPollerRestartBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return scriptpollers.ScriptPollerRestartBackoffMin
	}
	backoff := scriptpollers.ScriptPollerRestartBackoffMin
	for i := 1; i < attempt && backoff < scriptPollerRestartBackoffMax; i++ {
		backoff *= 2
		if backoff >= scriptPollerRestartBackoffMax {
			return scriptPollerRestartBackoffMax
		}
	}
	return backoff
}

func scriptPollerStopReason(err error) string {
	switch {
	case errors.Is(err, context.Canceled):
		return "context canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline exceeded"
	case err != nil:
		return err.Error()
	default:
		return "completed"
	}
}

func (s *service) scriptPollerExecutionTimeout(
	workstation factorydefinitions.FactoryWorkstationConfig,
	workerDef *factorydefinitions.FactoryWorkerConfig,
) (time.Duration, error) {
	if s.dependencies.ExecutionPolicy == nil {
		return 0, fmt.Errorf("Factory Definition Workstation execution policy service is required")
	}
	timeout, err := s.dependencies.ExecutionPolicy.ExecutionTimeout(&workstation)
	if err != nil {
		return 0, err
	}
	if timeout > 0 {
		return timeout, nil
	}
	if workerDef != nil && strings.TrimSpace(workerDef.Timeout) != "" {
		parsed, err := time.ParseDuration(workerDef.Timeout)
		if err != nil {
			return 0, fmt.Errorf("invalid worker timeout %q: %w", workerDef.Timeout, err)
		}
		if parsed > 0 {
			return parsed, nil
		}
	}
	return 0, nil
}

func (s *service) pollerLogger(workstationName, workerName string) *zap.Logger {
	if s.dependencies.Logger != nil {
		if logger := s.dependencies.Logger(workstationName, workerName); logger != nil {
			return logger
		}
	}
	return zap.NewNop()
}

func (s *service) commandRunner() workers.CommandRunner {
	if s.dependencies.CommandRunner != nil {
		if runner := s.dependencies.CommandRunner(); runner != nil {
			return runner
		}
	}
	return unavailableCommandRunner{}
}

type unavailableCommandRunner struct{}

func (unavailableCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, errors.New("automation command runner is required")
}

func (s *service) supervisorClock() clockwork.Clock {
	if s.dependencies.Clock != nil {
		if clock := s.dependencies.Clock(); clock != nil {
			return clock
		}
	}
	return clockwork.NewRealClock()
}
