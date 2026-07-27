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

func (s *service) GetCursor(
	ctx context.Context,
	request automations.GetCursorRequest,
) (automations.GetCursorResult, error) {
	recorder := s.cursorRecorder()
	if recorder == nil {
		return automations.GetCursorResult{}, unavailableCursorRecorderError()
	}
	return recorder.GetCursor(ctx, request)
}

func (s *service) StartScriptPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeCfg factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	workerDef *factorydefinitions.FactoryWorkerConfig,
	supervision scriptpollers.ScriptPollerSupervision,
	submitter automations.WorkRequestSubmitter,
) {
	if sidecars == nil || submitter == nil {
		return
	}
	sidecars.Add(1)
	go func() {
		defer sidecars.Done()
		s.superviseScriptPoller(ctx, runtimeCfg, workstation, workerDef, supervision, submitter)
	}()
}

func (s *service) superviseScriptPoller(
	ctx context.Context,
	runtimeCfg factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	workerDef *factorydefinitions.FactoryWorkerConfig,
	supervision scriptpollers.ScriptPollerSupervision,
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
		runErr := s.RunScriptPoller(ctx, runner, runtimeCfg, workstation, workerDef, supervision, submitter)
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
	supervision scriptpollers.ScriptPollerSupervision,
	submitter automations.WorkRequestSubmitter,
) error {
	resume, err := s.resolveScriptPollerResume(ctx, supervision)
	if err != nil {
		return err
	}

	commandReq, err := scriptpollers.ScriptPollerCommandRequest(
		runtimeCfg,
		workstation,
		workerDef,
		s.dependencies.ResolveTemplates,
		resume,
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

	parsed, err := scriptpollers.ParseScriptPollerStdout(result.Stdout)
	if err != nil {
		return err
	}
	if parsed.HasRequest {
		if submitter == nil {
			return fmt.Errorf("script poller submitter is not available")
		}
		if err := submitter(ctx, parsed.Request); err != nil {
			return scriptpollers.SubmitFailedError(err)
		}
		if parsed.AdvancesPosition {
			if err := s.commitScriptPollerRecovery(ctx, supervision, resume, parsed); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("script poller exited unexpectedly")
}

func (s *service) resolveScriptPollerResume(
	ctx context.Context,
	supervision scriptpollers.ScriptPollerSupervision,
) (scriptpollers.ResumeCursor, error) {
	instanceID := strings.TrimSpace(supervision.InstanceID)
	if instanceID == "" {
		return scriptpollers.ResumeCursor{}, nil
	}

	recorder := s.cursorRecorder()
	if recorder == nil {
		return scriptpollers.ResumeCursor{}, unavailableCursorRecorderError()
	}

	request := automations.GetCursorRequest{InstanceID: instanceID}
	if supervision.ExpectedCursor != "" {
		request.ExpectedCursor = supervision.ExpectedCursor
	}
	current, err := recorder.GetCursor(ctx, request)
	if err != nil {
		var typed *automations.Error
		if errors.As(err, &typed) && typed.Code == automations.ErrorCodeNotFound {
			if supervision.ExpectedCursor != "" {
				return scriptpollers.ResumeCursor{}, scriptpollers.CursorConflictError(scriptpollers.GetCursorOperation)
			}
			return scriptpollers.ResumeCursor{}, nil
		}
		return scriptpollers.ResumeCursor{}, err
	}
	if supervision.AutomationID != "" &&
		strings.TrimSpace(current.AutomationID) != strings.TrimSpace(supervision.AutomationID) {
		return scriptpollers.ResumeCursor{}, scriptpollers.CursorConflictError(scriptpollers.GetCursorOperation)
	}
	return scriptpollers.ResumeCursor{
		Cursor:     current.Cursor,
		Checkpoint: current.Checkpoint,
	}, nil
}

func (s *service) commitScriptPollerRecovery(
	ctx context.Context,
	supervision scriptpollers.ScriptPollerSupervision,
	resume scriptpollers.ResumeCursor,
	parsed scriptpollers.ScriptPollerStdout,
) error {
	instanceID := strings.TrimSpace(supervision.InstanceID)
	if instanceID == "" {
		return nil
	}
	recorder := s.cursorRecorder()
	if recorder == nil {
		return unavailableCursorRecorderError()
	}
	return scriptpollers.CursorPersistError(recorder.CommitCursor(ctx, scriptpollers.CommitCursorRequest{
		AutomationID:   supervision.AutomationID,
		InstanceID:     instanceID,
		ExpectedCursor: resume.Cursor,
		Cursor:         automations.Cursor(parsed.AdvancedCursor),
		Checkpoint:     parsed.Checkpoint,
	}))
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

func (s *service) cursorRecorder() scriptpollers.CursorRecorder {
	return s.dependencies.CursorRecorder
}

func unavailableCursorRecorderError() error {
	return &automations.Error{
		Op:   scriptpollers.GetCursorOperation,
		Code: automations.ErrorCodeNotReady,
		Err:  automations.ErrNotReady,
	}
}
