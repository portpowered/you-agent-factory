package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	hostedsources "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources"
	hostedlinear "github.com/portpowered/infinite-you/pkg/services/automations/internal/services/hosted_sources/internal/linear"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"go.uber.org/zap"
)

// StartLinearPoller supervises a hosted Linear poller for one poller workstation.
// Unsupported providers must be filtered by the caller before invoking this function.
func StartLinearPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	logger *zap.Logger,
	clock hostedsources.Clock,
	httpClient hostedlinear.HTTPDoer,
	secretResolver hostedlinear.SecretResolver,
	checkpoints hostedlinear.CheckpointStore,
	linearEndpoint string,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.FactoryWorkerConfig,
	jitter retryJitter,
	submitter Submitter,
) error {
	if ctx == nil {
		return fmt.Errorf("start hosted linear poller: context is required")
	}
	if sidecars == nil {
		return fmt.Errorf("start hosted linear poller: sidecar wait group is required")
	}
	poller, err := NewLinearPoller(
		logger, clock, httpClient, secretResolver, checkpoints, linearEndpoint,
		runtimeCfg, workstation, workerDef, jitter, submitter,
	)
	if err != nil {
		return err
	}
	sidecars.Add(1)
	go func() {
		defer sidecars.Done()
		poller.supervise(ctx)
	}()
	return nil
}

func (p *LinearPoller) supervise(ctx context.Context) {
	logger := pollerLogger(p.logger, p.workstation, p.worker).With(zap.String("provider", interfaces.HostedWorkerProviderLinear))
	backoffClock := p.clock
	consecutiveFailures := 0
	logger.Info("hosted linear poller started")
	defer func() {
		logger.Info("hosted linear poller stopped", zap.String("reason", stopReason(ctx.Err())))
	}()

	for {
		if ctx.Err() != nil {
			return
		}

		runErr, completedCycle := p.run(ctx, logger)
		if ctx.Err() != nil {
			return
		}

		if completedCycle {
			consecutiveFailures = 0
		}
		consecutiveFailures++
		backoff, delaySource := retryDelay(consecutiveFailures, runErr, p.jitter)
		logger.Warn("hosted linear poller restarting",
			zap.Int("attempt", consecutiveFailures),
			zap.Int("consecutive_failures", consecutiveFailures),
			zap.Duration("backoff", backoff),
			zap.Duration("selected_delay", backoff),
			zap.String("delay_source", delaySource),
			zap.Error(runErr),
		)

		select {
		case <-ctx.Done():
			return
		case <-backoffClock.After(backoff):
		}
	}
}

func (p *LinearPoller) run(ctx context.Context, logger *zap.Logger) (error, bool) {
	interval, err := hostedlinear.PollInterval(p.worker.Linear)
	if err != nil {
		return err, false
	}

	apiKey, err := p.secretResolver(ctx, p.runtimeConfig, p.worker.Auth.SecretRef)
	if err != nil {
		return fmt.Errorf(
			"resolve hosted linear auth %q: %w: %w",
			p.worker.Auth.SecretRef,
			hostedlinear.ErrSecretResolution,
			err,
		), false
	}

	pollerClient := hostedlinear.Client{
		Endpoint:   p.linearEndpoint,
		HTTPClient: p.httpClient,
		Clock:      p.clock,
		Logger:     logger,
	}
	checkpointPath := hostedlinear.CheckpointPath(p.runtimeConfig, p.workstation, p.worker)
	completedCycle := false

	for {
		if ctx.Err() != nil {
			return ctx.Err(), completedCycle
		}
		result, err := hostedlinear.RunPollCycle(ctx, pollerClient, p.runtimeConfig, p.workstation, p.worker, hostedlinear.Submitter(p.submitter), p.checkpoints, checkpointPath, apiKey, logger)
		if err != nil {
			return redactResolvedSecret(err, apiKey), completedCycle
		}
		completedCycle = true
		if result.FoundNewer {
			logger.Info("hosted linear poller cycle completed",
				zap.Int("submissions", len(result.Submissions)),
				zap.String("checkpoint_issue_id", result.Checkpoint.IssueID),
				zap.String("checkpoint_updated_at", result.Checkpoint.UpdatedAt),
			)
		}

		select {
		case <-ctx.Done():
			return ctx.Err(), completedCycle
		case <-p.clock.After(interval):
		}
	}
}

func redactResolvedSecret(err error, secret string) error {
	if err == nil || secret == "" || !strings.Contains(err.Error(), secret) {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), secret, "[REDACTED]"))
}

func pollerLogger(logger *zap.Logger, workstation interfaces.FactoryWorkstationConfig, workerDef *interfaces.FactoryWorkerConfig) *zap.Logger {
	return defaultLogger(logger).With(
		zap.String("workstation", workstation.Name),
		zap.String("worker", workerDef.Name),
	)
}

func stopReason(err error) string {
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
