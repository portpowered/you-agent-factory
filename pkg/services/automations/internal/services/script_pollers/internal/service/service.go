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

const (
	scriptPollerRestartBackoffMin = 25 * time.Millisecond
	scriptPollerRestartBackoffMax = 250 * time.Millisecond
)

type service struct {
	logger           *zap.Logger
	clock            clockwork.Clock
	commandRunner    workers.CommandRunner
	templateResolver workers.TemplateFieldResolver
	executionPolicy  factorydefinitions.WorkstationExecutionPolicyService
	cursors          cursorRecorder
}

var _ scriptpollers.Service = (*service)(nil)

// New constructs an inert script-poller service from direct collaborators. It
// creates its private in-memory cursor authority without invoking any effect.
func New(
	logger *zap.Logger,
	clock clockwork.Clock,
	commandRunner workers.CommandRunner,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
) scriptpollers.Service {
	return newWithCursorRecorder(
		logger,
		clock,
		commandRunner,
		resolveTemplates,
		executionPolicy,
		newMemoryCursorRecorder(),
	)
}

func newWithCursorRecorder(
	logger *zap.Logger,
	clock clockwork.Clock,
	commandRunner workers.CommandRunner,
	resolveTemplates workers.TemplateFieldResolver,
	executionPolicy factorydefinitions.WorkstationExecutionPolicyService,
	recorder cursorRecorder,
) scriptpollers.Service {
	if logger == nil {
		logger = zap.NewNop()
	}
	if clock == nil {
		clock = clockwork.NewRealClock()
	}
	if commandRunner == nil {
		commandRunner = unavailableCommandRunner{}
	}
	return &service{
		logger:           logger,
		clock:            clock,
		commandRunner:    commandRunner,
		templateResolver: resolveTemplates,
		executionPolicy:  executionPolicy,
		cursors:          recorder,
	}
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
	workflowID string,
	submitter automations.WorkRequestSubmitter,
) {
	if sidecars == nil || submitter == nil {
		return
	}
	sidecars.Add(1)
	supervision := supervisionFor(workflowID, workstation.Name)
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
	supervision scriptPollerSupervision,
	submitter automations.WorkRequestSubmitter,
) {
	logger := s.pollerLogger(workstation.Name, workerDefName(workerDef))
	runner := s.commandRunner
	backoffClock := s.clock
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
		runErr := s.runScriptPoller(ctx, runner, runtimeCfg, workstation, workerDef, supervision, submitter)
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
	workflowID string,
	submitter automations.WorkRequestSubmitter,
) error {
	return s.runScriptPoller(
		ctx,
		runner,
		runtimeCfg,
		workstation,
		workerDef,
		supervisionFor(workflowID, workstation.Name),
		submitter,
	)
}

func (s *service) runScriptPoller(
	ctx context.Context,
	runner workers.CommandRunner,
	runtimeCfg factorydefinitions.RuntimeConfigLookup,
	workstation factorydefinitions.FactoryWorkstationConfig,
	workerDef *factorydefinitions.FactoryWorkerConfig,
	supervision scriptPollerSupervision,
	submitter automations.WorkRequestSubmitter,
) error {
	resume, err := s.resolveScriptPollerResume(ctx, supervision)
	if err != nil {
		return err
	}

	commandReq, err := scriptPollerCommandRequest(
		runtimeCfg,
		workstation,
		workerDef,
		s.templateResolver,
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

	if runner == nil {
		runner = s.commandRunner
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

	parsed, err := parseScriptPollerStdout(result.Stdout)
	if err != nil {
		return err
	}
	if parsed.hasRequest {
		if submitter == nil {
			return fmt.Errorf("script poller submitter is not available")
		}
		if err := submitter(ctx, parsed.request); err != nil {
			return submitFailedError(err)
		}
		if parsed.advancesPosition {
			if err := s.commitScriptPollerRecovery(ctx, supervision, resume, parsed); err != nil {
				return err
			}
		}
	}
	return fmt.Errorf("script poller exited unexpectedly")
}

func (s *service) resolveScriptPollerResume(
	ctx context.Context,
	supervision scriptPollerSupervision,
) (resumeCursor, error) {
	instanceID := strings.TrimSpace(supervision.instanceID)
	if instanceID == "" {
		return resumeCursor{}, nil
	}

	recorder := s.cursorRecorder()
	if recorder == nil {
		return resumeCursor{}, unavailableCursorRecorderError()
	}

	request := automations.GetCursorRequest{InstanceID: instanceID}
	if supervision.expectedCursor != "" {
		request.ExpectedCursor = supervision.expectedCursor
	}
	current, err := recorder.GetCursor(ctx, request)
	if err != nil {
		var typed *automations.Error
		if errors.As(err, &typed) && typed.Code == automations.ErrorCodeNotFound {
			if supervision.expectedCursor != "" {
				return resumeCursor{}, cursorConflictError(getCursorOperation)
			}
			return resumeCursor{}, nil
		}
		return resumeCursor{}, err
	}
	if supervision.automationID != "" &&
		strings.TrimSpace(current.AutomationID) != strings.TrimSpace(supervision.automationID) {
		return resumeCursor{}, cursorConflictError(getCursorOperation)
	}
	return resumeCursor{
		cursor:     current.Cursor,
		checkpoint: current.Checkpoint,
	}, nil
}

func (s *service) commitScriptPollerRecovery(
	ctx context.Context,
	supervision scriptPollerSupervision,
	resume resumeCursor,
	parsed scriptPollerStdout,
) error {
	instanceID := strings.TrimSpace(supervision.instanceID)
	if instanceID == "" {
		return nil
	}
	recorder := s.cursorRecorder()
	if recorder == nil {
		return unavailableCursorRecorderError()
	}
	return cursorPersistError(recorder.CommitCursor(ctx, commitCursorRequest{
		automationID:   supervision.automationID,
		instanceID:     instanceID,
		expectedCursor: resume.cursor,
		cursor:         automations.Cursor(parsed.advancedCursor),
		checkpoint:     parsed.checkpoint,
	}))
}

func scriptPollerRestartBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return scriptPollerRestartBackoffMin
	}
	backoff := scriptPollerRestartBackoffMin
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
	if s.executionPolicy == nil {
		return 0, fmt.Errorf("Factory Definition Workstation execution policy service is required")
	}
	timeout, err := s.executionPolicy.ExecutionTimeout(&workstation)
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
	return s.logger.With(
		zap.String("workstation", workstationName),
		zap.String("worker", workerName),
	)
}

type unavailableCommandRunner struct{}

func (unavailableCommandRunner) Run(context.Context, workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{}, errors.New("automation command runner is required")
}

func (s *service) cursorRecorder() cursorRecorder {
	return s.cursors
}

func unavailableCursorRecorderError() error {
	return &automations.Error{
		Op:   getCursorOperation,
		Code: automations.ErrorCodeNotReady,
		Err:  automations.ErrNotReady,
	}
}

func workerDefName(workerDef *factorydefinitions.FactoryWorkerConfig) string {
	if workerDef == nil {
		return ""
	}
	return workerDef.Name
}
