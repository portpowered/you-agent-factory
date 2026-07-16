package hostedworkers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	hostedlinear "github.com/portpowered/infinite-you/pkg/workers/hosted/linear"
	workertaxonomy "github.com/portpowered/infinite-you/pkg/workers/taxonomy"
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
	workerDef *workerconfig.Config,
	submitter Submitter,
) error {
	if ctx == nil {
		return fmt.Errorf("start hosted linear poller: context is required")
	}
	if sidecars == nil {
		return fmt.Errorf("start hosted linear poller: sidecar wait group is required")
	}
	poller, err := NewLinearPoller(LinearPollerDependencies{
		Config: cfg, RuntimeConfig: runtimeCfg, Workstation: workstation,
		Worker: workerDef, Submitter: submitter,
	})
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
	logger := pollerLogger(p.config, p.workstation, p.worker).With(zap.String("provider", workertaxonomy.HostedWorkerProviderLinear))
	backoffClock := p.config.Clock
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
		runErr := p.run(ctx, logger)
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

func (p *LinearPoller) run(ctx context.Context, logger *zap.Logger) error {
	interval, err := hostedlinear.PollInterval(p.worker.Linear)
	if err != nil {
		return err
	}

	apiKey, err := p.config.SecretResolver(ctx, p.runtimeConfig, p.worker.Auth.SecretRef)
	if err != nil {
		return fmt.Errorf("resolve hosted linear auth %q: %w", p.worker.Auth.SecretRef, err)
	}

	pollerClient := hostedlinear.Client{
		Endpoint:   p.config.LinearEndpoint,
		HTTPClient: p.config.HTTPClient,
		Logger:     logger,
	}
	checkpointPath := hostedlinear.CheckpointPath(p.runtimeConfig, p.workstation, p.worker)

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		result, err := hostedlinear.RunPollCycle(ctx, pollerClient, p.runtimeConfig, p.workstation, p.worker, hostedlinear.Submitter(p.submitter), checkpointPath, apiKey, logger)
		if err != nil {
			return redactResolvedSecret(err, apiKey)
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
		case <-p.config.Clock.After(interval):
		}
	}
}

func redactResolvedSecret(err error, secret string) error {
	if err == nil || secret == "" || !strings.Contains(err.Error(), secret) {
		return err
	}
	return errors.New(strings.ReplaceAll(err.Error(), secret, "[REDACTED]"))
}

func pollerLogger(cfg Config, workstation interfaces.FactoryWorkstationConfig, workerDef *workerconfig.Config) *zap.Logger {
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
