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
	"github.com/portpowered/infinite-you/pkg/timework"
	"go.uber.org/zap"
)

const (
	cronSourceTag          = interfaces.TimeWorkTagKeySource
	cronWorkstationTag     = interfaces.TimeWorkTagKeyCronWorkstation
	cronSubmissionNamePref = "cron:"

	cronMaxRetries     = 2
	cronRetryBackoff   = 10 * time.Millisecond
	cronExecutionError = "execution timeout"
)

type workRequestSubmitter func(context.Context, interfaces.WorkRequest) error

func (fs *FactoryService) startCronWatchersForRuntime(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
	submitter workRequestSubmitter,
) {
	if runtimeModeOrDefault(fs.cfg.RuntimeMode) != interfaces.RuntimeModeService || factoryCfg == nil || runtimeCfg == nil || submitter == nil {
		return
	}

	schedulerClock := fs.cronSchedulerClock()
	scheduler, err := gocron.NewScheduler(
		gocron.WithClock(schedulerClock),
		gocron.WithLocation(time.UTC),
	)
	if err != nil {
		fs.logger.Error("cron scheduler disabled", zap.Error(err))
		return
	}

	registered := fs.registerCronJobs(ctx, scheduler, schedulerClock, factoryDir, factoryCfg, runtimeCfg, submitter)
	if registered == 0 {
		_ = scheduler.Shutdown()
		return
	}

	scheduler.Start()
	fs.logger.Info("cron scheduler started", zap.Int("jobs", registered))
	sidecars.Add(1)
	go func() {
		defer sidecars.Done()
		<-ctx.Done()
		if err := scheduler.Shutdown(); err != nil {
			fs.logger.Warn("cron scheduler shutdown failed", zap.Error(err))
		}
		fs.logger.Info("cron scheduler stopped")
	}()
}

func (fs *FactoryService) registerCronJobs(
	ctx context.Context,
	scheduler gocron.Scheduler,
	schedulerClock clockwork.Clock,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
	submitter workRequestSubmitter,
) int {
	registered := 0
	workflowIdentity := fs.cronWorkflowIdentity(factoryDir)
	for _, workstation := range factoryCfg.Workstations {
		ws := workstation
		if ws.Kind != interfaces.WorkstationKindCron {
			continue
		}
		schedule, err := cronSchedule(ws)
		if err != nil {
			fs.logger.Warn("cron watcher disabled",
				zap.String("workstation", ws.Name),
				zap.Error(err),
			)
			continue
		}

		if err := fs.registerCronJob(ctx, scheduler, schedulerClock, runtimeCfg, workflowIdentity, ws, schedule, submitter); err != nil {
			fs.logger.Warn("cron watcher disabled",
				zap.String("workstation", ws.Name),
				zap.String("schedule", schedule),
				zap.Error(err),
			)
			continue
		}
		fs.triggerCronAtStart(ctx, schedulerClock, runtimeCfg, workflowIdentity, ws, submitter)
		registered++
	}
	return registered
}

func (fs *FactoryService) registerCronJob(
	ctx context.Context,
	scheduler gocron.Scheduler,
	schedulerClock clockwork.Clock,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workflowIdentity string,
	ws interfaces.FactoryWorkstationConfig,
	schedule string,
	submitter workRequestSubmitter,
) error {
	_, err := scheduler.NewJob(
		gocron.CronJob(schedule, false),
		gocron.NewTask(func() {
			fs.runCronJob(ctx, runtimeCfg, workflowIdentity, ws, schedulerClock.Now().UTC(), submitter)
		}),
	)
	if err != nil {
		return fmt.Errorf("register schedule %q: %w", schedule, err)
	}
	fs.logger.Info("cron watcher registered",
		zap.String("workstation", ws.Name),
		zap.String("schedule", schedule),
	)
	return nil
}

func (fs *FactoryService) triggerCronAtStart(
	ctx context.Context,
	schedulerClock clockwork.Clock,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workflowIdentity string,
	ws interfaces.FactoryWorkstationConfig,
	submitter workRequestSubmitter,
) {
	if ws.Cron == nil || !ws.Cron.TriggerAtStart {
		return
	}
	fs.runCronJob(ctx, runtimeCfg, workflowIdentity, ws, schedulerClock.Now().UTC(), submitter)
}

func (fs *FactoryService) runCronJob(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workflowIdentity string,
	ws interfaces.FactoryWorkstationConfig,
	firedAt time.Time,
	submitter workRequestSubmitter,
) {
	if err := fs.submitCronTickForRuntime(ctx, runtimeCfg, workflowIdentity, submitter, ws, firedAt); err != nil {
		if ctx.Err() != nil {
			return
		}
		fs.logger.Error("cron watcher trigger failed",
			zap.String("workstation", ws.Name),
			zap.Error(err),
		)
	}
}

func (fs *FactoryService) cronSchedulerClock() clockwork.Clock {
	if fs != nil {
		if schedulerClock, ok := fs.clock.(clockwork.Clock); ok && schedulerClock != nil {
			return schedulerClock
		}
	}
	return clockwork.NewRealClock()
}

func cronSchedule(ws interfaces.FactoryWorkstationConfig) (string, error) {
	if ws.Cron == nil {
		return "", fmt.Errorf("missing cron config")
	}
	schedule := strings.TrimSpace(ws.Cron.Schedule)
	if schedule == "" {
		return "", fmt.Errorf("missing cron schedule")
	}
	return schedule, nil
}

func (fs *FactoryService) submitCronTick(
	ctx context.Context,
	ws interfaces.FactoryWorkstationConfig,
	firedAt time.Time,
) error {
	runtimeCfg := fs.currentRuntimeConfig()
	if runtimeCfg == nil {
		return fs.submitCronTickForRuntime(ctx, nil, "", fs.currentRuntimeSubmitter(), ws, firedAt)
	}
	return fs.submitCronTickForRuntime(
		ctx,
		runtimeCfg,
		fs.cronWorkflowIdentity(runtimeCfg.FactoryDir()),
		fs.currentRuntimeSubmitter(),
		ws,
		firedAt,
	)
}

func (fs *FactoryService) submitCronTickForRuntime(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workflowIdentity string,
	submitter workRequestSubmitter,
	ws interfaces.FactoryWorkstationConfig,
	firedAt time.Time,
) error {
	attempts := cronMaxRetries + 1
	for attempt := 1; attempt <= attempts; attempt++ {
		err := fs.submitCronTickAttempt(ctx, runtimeCfg, workflowIdentity, submitter, ws, firedAt)
		if err == nil {
			return nil
		}
		if ctx.Err() != nil {
			return err
		}

		failure := classifyCronTriggerFailure(err)
		fields := []zap.Field{
			zap.String("workstation", ws.Name),
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", attempts),
			zap.String("failure_family", string(failure.Family)),
			zap.String("failure_type", string(failure.Type)),
			zap.Error(err),
		}
		if !failure.retryable || attempt == attempts {
			fs.logger.Error("cron watcher trigger exhausted", fields...)
			return err
		}

		fs.logger.Warn("cron watcher trigger retrying", fields...)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cronRetryBackoff):
		}
	}
	return nil
}

func (fs *FactoryService) submitCronTickAttempt(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workflowIdentity string,
	submitter workRequestSubmitter,
	ws interfaces.FactoryWorkstationConfig,
	firedAt time.Time,
) error {
	if submitter == nil {
		return fmt.Errorf("cron submitter is required")
	}
	attemptCtx, cancel, err := fs.cronAttemptContext(ctx, runtimeCfg, ws)
	if err != nil {
		return err
	}
	defer cancel()

	workRequest, metadata, err := timework.CronTimeWorkRequest(workflowIdentity, ws, firedAt)
	if err != nil {
		return fmt.Errorf("cron workstation %q time work request: %w", ws.Name, err)
	}
	work := workRequest.Works[0]

	fs.logger.Info("cron watcher trigger submitted",
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

func (fs *FactoryService) cronWorkflowIdentity(factoryDir string) string {
	if fs == nil {
		return ""
	}
	if fs.cfg != nil && fs.cfg.WorkflowID != "" {
		return fs.cfg.WorkflowID
	}
	if factoryDir != "" {
		return factoryDir
	}
	if fs.cfg != nil {
		return fs.cfg.Dir
	}
	return ""
}

func (fs *FactoryService) cronAttemptContext(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeWorkstationLookup,
	ws interfaces.FactoryWorkstationConfig,
) (context.Context, context.CancelFunc, error) {
	timeout, err := fs.cronExecutionTimeout(runtimeCfg, ws)
	if err != nil {
		return nil, nil, err
	}
	if timeout <= 0 {
		return ctx, func() {}, nil
	}
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	return attemptCtx, cancel, nil
}

func (fs *FactoryService) cronExecutionTimeout(
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

type cronTriggerFailure struct {
	Family    interfaces.ProviderErrorFamily
	Type      interfaces.ProviderErrorType
	retryable bool
}

func classifyCronTriggerFailure(err error) cronTriggerFailure {
	if errors.Is(err, context.DeadlineExceeded) {
		return cronTriggerFailure{
			Family:    interfaces.ProviderErrorFamilyRetryable,
			Type:      interfaces.ProviderErrorTypeTimeout,
			retryable: true,
		}
	}
	if errors.Is(err, context.Canceled) {
		return cronTriggerFailure{
			Family: interfaces.ProviderErrorFamilyTerminal,
			Type:   interfaces.ProviderErrorTypeUnknown,
		}
	}
	return cronTriggerFailure{
		Family:    interfaces.ProviderErrorFamilyRetryable,
		Type:      interfaces.ProviderErrorTypeInternalServerError,
		retryable: true,
	}
}
