package hostedworkers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	hostedlinear "github.com/portpowered/infinite-you/pkg/hostedworkers/linear"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

const (
	restartBackoffMin = 25 * time.Millisecond
	restartBackoffMax = 250 * time.Millisecond
)

// StartLinearPoller supervises a hosted Linear poller for one poller workstation.
// Unsupported providers must be filtered by the caller before invoking this function.
func StartLinearPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	cfg Config,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	submitter Submitter,
) {
	if sidecars == nil || submitter == nil {
		return
	}
	sidecars.Add(1)
	go func() {
		defer sidecars.Done()
		superviseLinearPoller(ctx, cfg, runtimeCfg, workstation, workerDef, submitter)
	}()
}

func superviseLinearPoller(
	ctx context.Context,
	cfg Config,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	submitter Submitter,
) {
	logger := pollerLogger(cfg, workstation, workerDef).With(zap.String("provider", interfaces.HostedWorkerProviderLinear))
	backoffClock := cfg.supervisorClock()
	attempt := 0
	logger.Info("hosted linear poller started")
	defer func() {
		logger.Info("hosted linear poller stopped", zap.String("reason", stopReason(ctx.Err())))
	}()

	for {
		if ctx.Err() != nil {
			return
		}

		attempt++
		runErr := runLinearPoller(ctx, cfg, runtimeCfg, workstation, workerDef, submitter, logger, backoffClock)
		if ctx.Err() != nil {
			return
		}

		backoff := restartBackoff(attempt)
		logger.Warn("hosted linear poller restarting",
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

func runLinearPoller(
	ctx context.Context,
	cfg Config,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
	submitter Submitter,
	logger *zap.Logger,
	clock clockwork.Clock,
) error {
	if runtimeCfg == nil {
		return fmt.Errorf("runtime config is required")
	}
	if workerDef == nil {
		return fmt.Errorf("hosted linear poller worker is required")
	}
	if workerDef.Auth == nil || strings.TrimSpace(workerDef.Auth.SecretRef) == "" {
		return fmt.Errorf("hosted linear poller worker %q is missing auth.secretRef", workerDef.Name)
	}
	if workerDef.Linear == nil {
		return fmt.Errorf("hosted linear poller worker %q is missing linear config", workerDef.Name)
	}
	if submitter == nil {
		return fmt.Errorf("hosted linear poller submitter is required")
	}
	interval, err := hostedlinear.PollInterval(workerDef.Linear)
	if err != nil {
		return err
	}

	resolver := cfg.secretResolver()
	apiKey, err := resolver(ctx, runtimeCfg, workerDef.Auth.SecretRef)
	if err != nil {
		return fmt.Errorf("resolve hosted linear auth %q: %w", workerDef.Auth.SecretRef, err)
	}

	pollerClient := hostedlinear.Client{
		Endpoint:   cfg.linearEndpoint(),
		HTTPClient: cfg.httpClient(),
		Logger:     logger,
	}
	checkpointPath := hostedlinear.CheckpointPath(runtimeCfg, workstation, workerDef)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		result, err := hostedlinear.RunPollCycle(ctx, pollerClient, runtimeCfg, workstation, workerDef, hostedlinear.Submitter(submitter), checkpointPath, apiKey, logger)
		if err != nil {
			return err
		}
		if result.FoundNewer {
			logger.Info("hosted linear poller cycle completed",
				zap.Int("submissions", len(result.Submissions)),
				zap.String("checkpoint_issue_id", result.Checkpoint.IssueID),
				zap.String("checkpoint_updated_at", result.Checkpoint.UpdatedAt),
			)
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-clock.After(interval):
		}
	}
}

func pollerLogger(cfg Config, workstation interfaces.FactoryWorkstationConfig, workerDef *interfaces.WorkerConfig) *zap.Logger {
	return cfg.logger().With(
		zap.String("workstation", workstation.Name),
		zap.String("worker", workerDef.Name),
	)
}

func restartBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return restartBackoffMin
	}
	backoff := restartBackoffMin
	for i := 1; i < attempt && backoff < restartBackoffMax; i++ {
		backoff *= 2
		if backoff >= restartBackoffMax {
			return restartBackoffMax
		}
	}
	return backoff
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
