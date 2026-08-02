package internal

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/jonboulle/clockwork"
	"go.uber.org/zap"

	cron "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/cron"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	// CronMaxRetries is the number of retry attempts after the initial cron tick submit.
	CronMaxRetries = 2

	cronRetryBackoff   = 10 * time.Millisecond
	cronExecutionError = "execution timeout"
)

// StartCronWatchersForRuntime supervises cron workstations until ctx is canceled.
func (s *Service) StartCronWatchersForRuntime(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	submitter WorkRequestSubmitter,
) {
	if factoryCfg == nil || runtimeCfg == nil || sidecars == nil || submitter == nil {
		return
	}

	schedulerClock := s.supervisorClock()
	scheduler, err := gocron.NewScheduler(
		gocron.WithClock(schedulerClock),
		gocron.WithLocation(time.UTC),
	)
	if err != nil {
		s.logger().Error("cron scheduler disabled", zap.Error(err))
		return
	}

	registered := s.registerCronJobs(ctx, scheduler, schedulerClock, factoryDir, factoryCfg, runtimeCfg, submitter)
	if registered == 0 {
		_ = scheduler.Shutdown()
		return
	}

	scheduler.Start()
	s.logger().Info("cron scheduler started", zap.Int("jobs", registered))
	sidecars.Add(1)
	go func() {
		defer sidecars.Done()
		<-ctx.Done()
		if err := scheduler.Shutdown(); err != nil {
			s.logger().Warn("cron scheduler shutdown failed", zap.Error(err))
		}
		s.logger().Info("cron scheduler stopped")
	}()
}

// SubmitCronTick submits one cron workstation tick through the injected runtime submitter.
func (s *Service) SubmitCronTick(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	workflowIdentity string,
	submitter WorkRequestSubmitter,
	ws interfaces.FactoryWorkstationConfig,
	firedAt time.Time,
) error {
	return s.submitCronTickForRuntime(ctx, runtimeCfg, workflowIdentity, submitter, ws, firedAt)
}

func (s *Service) registerCronJobs(
	ctx context.Context,
	scheduler gocron.Scheduler,
	schedulerClock clockwork.Clock,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	submitter WorkRequestSubmitter,
) int {
	registered := 0
	workflowIdentity := s.workflowIdentity(factoryDir)
	for _, workstation := range factoryCfg.Workstations {
		ws := workstation
		if ws.Kind != interfaces.WorkstationKindCron {
			continue
		}
		if ws.Cron == nil {
			s.logger().Warn("cron watcher disabled", zap.String("workstation", ws.Name), zap.String("reason", "missing cron configuration"))
			continue
		}
		var err error
		if strings.TrimSpace(ws.Cron.Every) != "" {
			if _, invocationScoped := invocationParameterReference(ws.Cron.Every); invocationScoped {
				s.logger().Info("invocation interval watcher awaiting controller Work",
					zap.String("workstation", ws.Name),
					zap.String("every", ws.Cron.Every),
				)
				continue
			}
			err = s.registerIntervalJob(ctx, scheduler, schedulerClock, runtimeCfg, workflowIdentity, ws, submitter)
		} else {
			var schedule string
			schedule, err = s.cronSchedule(ws)
			if err == nil {
				err = s.registerCronJob(ctx, scheduler, schedulerClock, runtimeCfg, workflowIdentity, ws, schedule, submitter)
			}
		}
		if err != nil {
			s.logger().Warn("cron watcher disabled",
				zap.String("workstation", ws.Name),
				zap.Error(err),
			)
			continue
		}
		s.triggerCronAtStart(ctx, schedulerClock, runtimeCfg, workflowIdentity, ws, submitter)
		registered++
	}
	return registered
}

func (s *Service) registerIntervalJob(
	ctx context.Context,
	scheduler gocron.Scheduler,
	schedulerClock clockwork.Clock,
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	workflowIdentity string,
	ws interfaces.FactoryWorkstationConfig,
	submitter WorkRequestSubmitter,
) error {
	every, err := time.ParseDuration(strings.TrimSpace(ws.Cron.Every))
	if err != nil || every < time.Second || every > 7*24*time.Hour {
		return fmt.Errorf("interval %q must be from 1s through 168h", ws.Cron.Every)
	}
	_, err = scheduler.NewJob(
		gocron.DurationJob(every),
		gocron.NewTask(func() {
			s.runCronJob(ctx, runtimeCfg, workflowIdentity, ws, schedulerClock.Now().UTC(), submitter)
		}),
		gocron.WithSingletonMode(gocron.LimitModeReschedule),
	)
	if err != nil {
		return fmt.Errorf("register interval %q: %w", ws.Cron.Every, err)
	}
	s.logger().Info("interval watcher registered", zap.String("workstation", ws.Name), zap.String("every", ws.Cron.Every))
	return nil
}

func (s *Service) registerCronJob(
	ctx context.Context,
	scheduler gocron.Scheduler,
	schedulerClock clockwork.Clock,
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	workflowIdentity string,
	ws interfaces.FactoryWorkstationConfig,
	schedule string,
	submitter WorkRequestSubmitter,
) error {
	_, err := scheduler.NewJob(
		gocron.CronJob(schedule, false),
		gocron.NewTask(func() {
			s.runCronJob(ctx, runtimeCfg, workflowIdentity, ws, schedulerClock.Now().UTC(), submitter)
		}),
	)
	if err != nil {
		return fmt.Errorf("register schedule %q: %w", schedule, err)
	}
	s.logger().Info("cron watcher registered",
		zap.String("workstation", ws.Name),
		zap.String("schedule", schedule),
	)
	return nil
}

func (s *Service) triggerCronAtStart(
	ctx context.Context,
	schedulerClock clockwork.Clock,
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	workflowIdentity string,
	ws interfaces.FactoryWorkstationConfig,
	submitter WorkRequestSubmitter,
) {
	if ws.Cron == nil || !ws.Cron.TriggerAtStart {
		return
	}
	s.runCronJob(ctx, runtimeCfg, workflowIdentity, ws, schedulerClock.Now().UTC(), submitter)
}

func (s *Service) runCronJob(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	workflowIdentity string,
	ws interfaces.FactoryWorkstationConfig,
	firedAt time.Time,
	submitter WorkRequestSubmitter,
) {
	if err := s.submitCronTickForRuntime(ctx, runtimeCfg, workflowIdentity, submitter, ws, firedAt); err != nil {
		if ctx.Err() != nil {
			return
		}
		s.logger().Error("cron watcher trigger failed",
			zap.String("workstation", ws.Name),
			zap.Error(err),
		)
	}
}

// WorkflowIdentityForFactoryDir resolves cron workflow identity from runtime factory directory.
func (s *Service) WorkflowIdentityForFactoryDir(factoryDir string) string {
	return s.workflowIdentity(factoryDir)
}

func (s *Service) workflowIdentity(factoryDir string) string {
	if s != nil && s.workflowID != "" {
		return s.workflowID
	}
	if factoryDir != "" {
		return factoryDir
	}
	if s != nil {
		return s.defaultFactoryDir
	}
	return ""
}

func (s *Service) cronSchedule(ws interfaces.FactoryWorkstationConfig) (string, error) {
	if ws.Cron == nil {
		return "", fmt.Errorf("%w: missing cron config", cron.ErrInvalidSchedule)
	}
	schedule := strings.TrimSpace(ws.Cron.Schedule)
	if schedule == "" {
		return "", fmt.Errorf("%w: schedule is required", cron.ErrInvalidSchedule)
	}
	if err := s.cron.ValidateCronSchedule(schedule); err != nil {
		return "", err
	}
	return schedule, nil
}

func (s *Service) submitCronTickForRuntime(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	workflowIdentity string,
	submitter WorkRequestSubmitter,
	ws interfaces.FactoryWorkstationConfig,
	firedAt time.Time,
) error {
	attempts := CronMaxRetries + 1
	for attempt := 1; attempt <= attempts; attempt++ {
		err := s.submitCronTickAttempt(ctx, runtimeCfg, workflowIdentity, submitter, ws, firedAt)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return err
		}

		failure := ClassifyCronTriggerFailure(err)
		fields := []zap.Field{
			zap.String("workstation", ws.Name),
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", attempts),
			zap.String("failure_family", string(failure.Family)),
			zap.String("failure_type", string(failure.Type)),
			zap.Error(err),
		}
		if !failure.Retryable || attempt == attempts {
			s.logger().Error("cron watcher trigger exhausted", fields...)
			return err
		}

		s.logger().Warn("cron watcher trigger retrying", fields...)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cronRetryBackoff):
		}
	}
	return nil
}

func (s *Service) submitCronTickAttempt(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	workflowIdentity string,
	submitter WorkRequestSubmitter,
	ws interfaces.FactoryWorkstationConfig,
	firedAt time.Time,
) error {
	if submitter == nil {
		return fmt.Errorf("cron submitter is required")
	}
	attemptCtx, cancel, err := s.cronAttemptContext(ctx, runtimeCfg, ws)
	if err != nil {
		return err
	}
	defer cancel()

	submission, err := s.cron.SubmitCronTick(
		attemptCtx,
		cron.WorkRequestSubmitter(submitter),
		workflowIdentity,
		ws,
		firedAt,
	)
	if err != nil {
		if errors.Is(attemptCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("cron workstation %q %s: %w", ws.Name, cronExecutionError, context.DeadlineExceeded)
		}
		return fmt.Errorf("cron workstation %q time work request: %w", ws.Name, err)
	}
	if !submission.Submitted {
		return fmt.Errorf("cron workstation %q: expected submitted tick at %s", ws.Name, firedAt.Format(time.RFC3339Nano))
	}
	metadata := submission.Metadata
	s.logger().Info("cron watcher trigger submitted",
		zap.String("workstation", ws.Name),
		zap.String("work_type", interfaces.SystemTimeWorkTypeID),
		zap.String("state", interfaces.SystemTimePendingState),
		zap.Time("nominal_at", metadata.NominalAt),
		zap.Time("due_at", metadata.DueAt),
		zap.Time("expires_at", metadata.ExpiresAt),
	)
	return nil
}

func (s *Service) cronAttemptContext(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	ws interfaces.FactoryWorkstationConfig,
) (context.Context, context.CancelFunc, error) {
	timeout, err := s.cronExecutionTimeout(runtimeCfg, ws)
	if err != nil {
		return nil, nil, err
	}
	if timeout <= 0 {
		return ctx, func() {}, nil
	}
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	return attemptCtx, cancel, nil
}

// CronExecutionTimeout resolves the execution timeout for a cron workstation from runtime config.
func (s *Service) CronExecutionTimeout(
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	ws interfaces.FactoryWorkstationConfig,
) (time.Duration, error) {
	return s.cronExecutionTimeout(runtimeCfg, ws)
}

func (s *Service) cronExecutionTimeout(
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	ws interfaces.FactoryWorkstationConfig,
) (time.Duration, error) {
	if runtimeCfg == nil {
		return 0, nil
	}
	def, ok := runtimeCfg.Workstation(ws.Name)
	if !ok || def == nil {
		return 0, nil
	}
	authoredTimeout := strings.TrimSpace(def.Limits.MaxExecutionTime)
	if authoredTimeout == "" {
		authoredTimeout = strings.TrimSpace(def.Timeout)
	}
	if authoredTimeout == "" {
		return 0, nil
	}
	timeout, err := time.ParseDuration(authoredTimeout)
	if err != nil {
		return 0, fmt.Errorf("cron workstation %q: invalid execution timeout %q: %w", ws.Name, authoredTimeout, err)
	}
	if timeout <= 0 {
		return 0, nil
	}
	return timeout, nil
}

// CronTriggerFailure classifies cron tick submit failures for retry policy.
type CronTriggerFailure struct {
	Family    workerexecution.WorkFailureFamily
	Type      workerexecution.WorkFailureType
	Retryable bool
}

// ClassifyCronTriggerFailure maps cron submit errors to retry policy.
func ClassifyCronTriggerFailure(err error) CronTriggerFailure {
	if errors.Is(err, context.DeadlineExceeded) {
		return CronTriggerFailure{
			Family:    workerexecution.WorkFailureFamilyRetryable,
			Type:      workerexecution.WorkFailureTypeTimeout,
			Retryable: true,
		}
	}
	if errors.Is(err, context.Canceled) {
		return CronTriggerFailure{
			Family: workerexecution.WorkFailureFamilyTerminal,
			Type:   workerexecution.WorkFailureTypeUnknown,
		}
	}
	return CronTriggerFailure{
		Family:    workerexecution.WorkFailureFamilyRetryable,
		Type:      workerexecution.WorkFailureTypeInternalServerError,
		Retryable: true,
	}
}
