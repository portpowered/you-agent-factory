package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-co-op/gocron/v2"
	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/work/timework"
	"go.uber.org/zap"
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

// StartCronIntervalWatcher supervises one session-scoped cron workstation at a
// validated fixed interval. The caller owns validation and must share the
// runtime sidecar context so stopping the Factory Session stops this watcher.
func (s *Service) StartCronIntervalWatcher(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	workflowIdentity string,
	submitter WorkRequestSubmitter,
	ws interfaces.FactoryWorkstationConfig,
	interval time.Duration,
) error {
	if ctx == nil || sidecars == nil || runtimeCfg == nil || submitter == nil {
		return fmt.Errorf("cron interval watcher dependencies are required")
	}
	if interval <= 0 {
		return fmt.Errorf("cron interval must be positive")
	}
	scheduler, err := gocron.NewScheduler(gocron.WithClock(s.supervisorClock()), gocron.WithLocation(time.UTC))
	if err != nil {
		return fmt.Errorf("create cron interval scheduler: %w", err)
	}
	_, err = scheduler.NewJob(
		gocron.DurationJob(interval),
		gocron.NewTask(func() {
			s.runCronJob(ctx, runtimeCfg, workflowIdentity, ws, s.supervisorClock().Now().UTC(), submitter)
		}),
	)
	if err != nil {
		_ = scheduler.Shutdown()
		return fmt.Errorf("register cron interval %s: %w", interval, err)
	}
	scheduler.Start()
	s.logger().Info("cron interval watcher started", zap.String("workstation", ws.Name), zap.Duration("interval", interval))
	sidecars.Add(1)
	go func() {
		defer sidecars.Done()
		<-ctx.Done()
		if err := scheduler.Shutdown(); err != nil {
			s.logger().Warn("cron interval watcher shutdown failed", zap.Error(err))
		}
	}()
	return nil
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
	if firedAt.IsZero() {
		firedAt = s.supervisorClock().Now().UTC()
	}
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
		schedule, err := cronSchedule(ws)
		if err != nil {
			s.logger().Warn("cron watcher disabled",
				zap.String("workstation", ws.Name),
				zap.Error(err),
			)
			continue
		}

		if err := s.registerCronJob(ctx, scheduler, schedulerClock, runtimeCfg, workflowIdentity, ws, schedule, submitter); err != nil {
			s.logger().Warn("cron watcher disabled",
				zap.String("workstation", ws.Name),
				zap.String("schedule", schedule),
				zap.Error(err),
			)
			continue
		}
		s.triggerCronAtStart(ctx, schedulerClock, runtimeCfg, workflowIdentity, ws, submitter)
		registered++
	}
	return registered
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
	if s != nil && s.cfg.WorkflowID != "" {
		return s.cfg.WorkflowID
	}
	if factoryDir != "" {
		return factoryDir
	}
	if s != nil {
		return s.cfg.DefaultFactoryDir
	}
	return ""
}

func cronSchedule(ws interfaces.FactoryWorkstationConfig) (string, error) {
	if ws.Cron == nil {
		return "", fmt.Errorf("missing cron config")
	}
	schedule := strings.TrimSpace(ws.Cron.Schedule)
	if schedule == "" {
		return "", fmt.Errorf("missing cron schedule")
	}
	if err := timework.ValidateCronSchedule(schedule); err != nil {
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

	workRequest, metadata, err := timework.CronTimeWorkRequest(workflowIdentity, ws, firedAt)
	if err != nil {
		return fmt.Errorf("cron workstation %q time work request: %w", ws.Name, err)
	}
	work := workRequest.Works[0]

	s.logger().Info("cron watcher trigger submitted",
		zap.String("workstation", ws.Name),
		zap.String("work_type", work.WorkTypeID),
		zap.String("state", work.State),
		zap.Time("nominal_at", metadata.NominalAt),
		zap.Time("due_at", metadata.DueAt),
		zap.Time("expires_at", metadata.ExpiresAt),
	)
	if err := submitter(attemptCtx, workRequest); err != nil {
		if errors.Is(attemptCtx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("cron workstation %q %s: %w", ws.Name, cronExecutionError, context.DeadlineExceeded)
		}
		return err
	}
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
	timeout, err := config.WorkstationExecutionTimeout(def)
	if err != nil {
		return 0, fmt.Errorf("cron workstation %q: %w", ws.Name, err)
	}
	if timeout <= 0 {
		return 0, nil
	}
	return timeout, nil
}

// CronTriggerFailure classifies cron tick submit failures for retry policy.
type CronTriggerFailure struct {
	Family    interfaces.WorkFailureFamily
	Type      interfaces.WorkFailureType
	Retryable bool
}

// ClassifyCronTriggerFailure maps cron submit errors to retry policy.
func ClassifyCronTriggerFailure(err error) CronTriggerFailure {
	if errors.Is(err, context.DeadlineExceeded) {
		return CronTriggerFailure{
			Family:    interfaces.WorkFailureFamilyRetryable,
			Type:      interfaces.WorkFailureTypeTimeout,
			Retryable: true,
		}
	}
	if errors.Is(err, context.Canceled) {
		return CronTriggerFailure{
			Family: interfaces.WorkFailureFamilyTerminal,
			Type:   interfaces.WorkFailureTypeUnknown,
		}
	}
	return CronTriggerFailure{
		Family:    interfaces.WorkFailureFamilyRetryable,
		Type:      interfaces.WorkFailureTypeInternalServerError,
		Retryable: true,
	}
}
