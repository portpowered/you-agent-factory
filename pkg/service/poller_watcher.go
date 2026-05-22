package service

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/config"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/workers"
	"go.uber.org/zap"
)

const (
	pollerRestartBackoffMin = 25 * time.Millisecond
	pollerRestartBackoffMax = 250 * time.Millisecond
)

func (fs *FactoryService) startPollerWatchersForRuntime(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	factoryCfg *interfaces.FactoryConfig,
	runtimeCfg interfaces.RuntimeConfigLookup,
) {
	if runtimeModeOrDefault(fs.cfg.RuntimeMode) != interfaces.RuntimeModeService || factoryCfg == nil || runtimeCfg == nil || sidecars == nil {
		return
	}

	for _, workstation := range factoryCfg.Workstations {
		ws := workstation
		if ws.Kind != interfaces.WorkstationKindPoller {
			continue
		}

		workerName := strings.TrimSpace(ws.WorkerTypeName)
		if workerName == "" {
			fs.logger.Warn("script poller disabled",
				zap.String("workstation", ws.Name),
				zap.String("reason", "missing worker binding"),
			)
			continue
		}

		workerDef, ok := runtimeCfg.Worker(workerName)
		if !ok || workerDef == nil {
			fs.logger.Warn("script poller disabled",
				zap.String("workstation", ws.Name),
				zap.String("worker", workerName),
				zap.String("reason", "worker config not found"),
			)
			continue
		}
		if workerDef.Type != interfaces.WorkerTypeScript {
			continue
		}

		sidecars.Add(1)
		go func() {
			defer sidecars.Done()
			fs.superviseScriptPoller(ctx, runtimeCfg, ws, workerDef)
		}()
	}
}

func (fs *FactoryService) superviseScriptPoller(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
) {
	logger := fs.pollerLogger(workstation, workerDef)
	runner := fs.pollerCommandRunner()
	backoffClock := fs.pollerSupervisorClock()
	attempt := 0

	for {
		if ctx.Err() != nil {
			return
		}

		attempt++
		runErr := fs.runScriptPoller(ctx, runner, runtimeCfg, workstation, workerDef)
		if ctx.Err() != nil {
			return
		}

		backoff := pollerRestartBackoff(attempt)
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

func (fs *FactoryService) runScriptPoller(
	ctx context.Context,
	runner workers.CommandRunner,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
) error {
	commandReq, err := scriptPollerCommandRequest(runtimeCfg, workstation, workerDef)
	if err != nil {
		return err
	}

	execCtx := ctx
	timeout, err := scriptPollerExecutionTimeout(workstation, workerDef)
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
	if err := validateScriptPollerOutput(result.Stdout); err != nil {
		return err
	}
	return fmt.Errorf("script poller exited unexpectedly")
}

func (fs *FactoryService) pollerCommandRunner() workers.CommandRunner {
	if fs != nil && fs.cfg != nil && fs.cfg.CommandRunnerOverride != nil {
		return fs.cfg.CommandRunnerOverride
	}
	return workers.ExecCommandRunner{}
}

func (fs *FactoryService) pollerSupervisorClock() clockwork.Clock {
	if fs != nil {
		if supervisorClock, ok := fs.clock.(clockwork.Clock); ok && supervisorClock != nil {
			return supervisorClock
		}
	}
	return clockwork.NewRealClock()
}

func (fs *FactoryService) pollerLogger(workstation interfaces.FactoryWorkstationConfig, workerDef *interfaces.WorkerConfig) *zap.Logger {
	if fs == nil || fs.logger == nil {
		return zap.NewNop()
	}
	return fs.logger.With(
		zap.String("workstation", workstation.Name),
		zap.String("worker", workerDef.Name),
	)
}

func pollerRestartBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return pollerRestartBackoffMin
	}
	backoff := pollerRestartBackoffMin
	for i := 1; i < attempt && backoff < pollerRestartBackoffMax; i++ {
		backoff *= 2
		if backoff >= pollerRestartBackoffMax {
			return pollerRestartBackoffMax
		}
	}
	return backoff
}

func scriptPollerCommandRequest(
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.WorkerConfig,
) (workers.CommandRequest, error) {
	if runtimeCfg == nil {
		return workers.CommandRequest{}, fmt.Errorf("runtime config is required")
	}
	if workerDef == nil {
		return workers.CommandRequest{}, fmt.Errorf("script poller worker is required")
	}
	if strings.TrimSpace(workerDef.Command) == "" {
		return workers.CommandRequest{}, fmt.Errorf("script poller worker %q is missing command", workerDef.Name)
	}

	requestContext := &factory_context.FactoryContext{}
	if resolved, err := workers.ResolveTemplateFields(
		workstation.WorkingDirectory,
		workstation.Env,
		nil,
		requestContext,
		workstation.Worktree,
	); err != nil {
		return workers.CommandRequest{}, fmt.Errorf("resolve poller workstation fields: %w", err)
	} else if resolved != nil {
		requestContext.WorkDirectory = resolved.WorkingDirectory
		requestContext.EnvVars = resolved.Env
		if requestContext.WorkDirectory == "" {
			requestContext.WorkDirectory = resolved.Worktree
		}
	}

	workDir := requestContext.WorkDirectory
	if workDir != "" && !filepath.IsAbs(workDir) {
		baseDir := runtimeCfg.RuntimeBaseDir()
		if baseDir == "" {
			baseDir = runtimeCfg.FactoryDir()
		}
		if baseDir != "" {
			workDir = filepath.Clean(filepath.Join(baseDir, filepath.FromSlash(workDir)))
		}
	}

	req := workers.CommandRequest{
		Command:         resolvePortableFactoryScriptReference(runtimeCfg.FactoryDir(), workerDef.Command),
		Args:            resolvePortableFactoryScriptReferences(runtimeCfg.FactoryDir(), workerDef.Args),
		Env:             commandEnvWithResolvedVars(requestContext.EnvVars),
		WorkDir:         workDir,
		WorkerType:      workerDef.Name,
		WorkstationName: workstation.Name,
	}
	return req, nil
}

func scriptPollerExecutionTimeout(workstation interfaces.FactoryWorkstationConfig, workerDef *interfaces.WorkerConfig) (time.Duration, error) {
	timeout, err := config.WorkstationExecutionTimeout(&workstation)
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

func validateScriptPollerOutput(stdout []byte) error {
	if strings.TrimSpace(string(stdout)) == "" {
		return nil
	}

	// Story 003 only supervises the daemon lifecycle. Until story 004 wires the
	// submit-style ingress contract, any non-empty stdout is treated as malformed
	// so the supervisor does not silently drop poller output.
	return fmt.Errorf("script poller emitted malformed stdout")
}

func commandEnvWithResolvedVars(vars map[string]string) []string {
	if len(vars) == 0 {
		return nil
	}

	env := make([]string, 0, len(vars))
	for key, value := range vars {
		env = append(env, key+"="+value)
	}
	return env
}

func resolvePortableFactoryScriptReferences(factoryDir string, args []string) []string {
	if len(args) == 0 {
		return nil
	}
	resolved := make([]string, len(args))
	for i, arg := range args {
		resolved[i] = resolvePortableFactoryScriptReference(factoryDir, arg)
	}
	return resolved
}

func resolvePortableFactoryScriptReference(factoryDir, raw string) string {
	if strings.TrimSpace(factoryDir) == "" {
		return raw
	}

	trimmed := strings.TrimSpace(raw)
	normalized := filepath.ToSlash(trimmed)
	if !strings.HasPrefix(normalized, "factory/scripts/") {
		return raw
	}

	relativePath := strings.TrimPrefix(normalized, "factory/scripts/")
	if relativePath == "" {
		return raw
	}
	return filepath.Join(factoryDir, "scripts", filepath.FromSlash(relativePath))
}
