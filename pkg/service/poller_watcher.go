package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
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
	submitter workRequestSubmitter,
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
		switch workerDef.Type {
		case interfaces.WorkerTypeScript:
			sidecars.Add(1)
			go func() {
				defer sidecars.Done()
				fs.superviseScriptPoller(ctx, runtimeCfg, ws, workerDef)
			}()
		case interfaces.WorkerTypeHosted:
			if workerDef.Provider != interfaces.HostedWorkerProviderLinear {
				fs.logger.Warn("hosted poller disabled",
					zap.String("workstation", ws.Name),
					zap.String("worker", workerName),
					zap.String("provider", workerDef.Provider),
					zap.String("reason", "unsupported hosted provider"),
				)
				continue
			}
			sidecars.Add(1)
			go func() {
				defer sidecars.Done()
				fs.superviseHostedLinearPoller(ctx, runtimeCfg, ws, workerDef, submitter)
			}()
		default:
			continue
		}
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
	logger.Info("script poller started")
	defer func() {
		logger.Info("script poller stopped", zap.String("reason", pollerStopReason(ctx.Err())))
	}()

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
	request, hasOutput, err := parseScriptPollerOutput(result.Stdout)
	if err != nil {
		return err
	}
	if hasOutput {
		if _, err := fs.SubmitWorkRequest(ctx, request); err != nil {
			return fmt.Errorf("script poller submit failed: %w", err)
		}
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

func pollerStopReason(err error) string {
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

func parseScriptPollerOutput(stdout []byte) (interfaces.WorkRequest, bool, error) {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return interfaces.WorkRequest{}, false, nil
	}

	var envelope scriptPollerOutputEnvelope
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return interfaces.WorkRequest{}, true, fmt.Errorf("script poller emitted malformed stdout: %w", err)
	}
	if len(envelope.Events) > 0 {
		return interfaces.WorkRequest{}, true, fmt.Errorf("script poller emitted unsupported raw factory events")
	}
	if len(envelope.Request) > 0 && len(envelope.Submissions) > 0 {
		return interfaces.WorkRequest{}, true, fmt.Errorf("script poller stdout must contain either request or submissions, not both")
	}
	if len(envelope.Request) > 0 {
		request, err := factory.ParseCanonicalWorkRequestJSON(envelope.Request)
		if err != nil {
			return interfaces.WorkRequest{}, true, fmt.Errorf("script poller emitted malformed stdout: %w", err)
		}
		if err := validateScriptPollerWorkRequest(request); err != nil {
			return interfaces.WorkRequest{}, true, err
		}
		return request, true, nil
	}
	if len(envelope.Submissions) > 0 {
		request, err := scriptPollerWorkRequestFromSubmissions(envelope.Submissions)
		if err != nil {
			return interfaces.WorkRequest{}, true, err
		}
		return request, true, nil
	}

	request, err := factory.ParseCanonicalWorkRequestJSON(trimmed)
	if err != nil {
		return interfaces.WorkRequest{}, true, fmt.Errorf("script poller emitted malformed stdout: %w", err)
	}
	if err := validateScriptPollerWorkRequest(request); err != nil {
		return interfaces.WorkRequest{}, true, err
	}
	return request, true, nil
}

type scriptPollerOutputEnvelope struct {
	Request     json.RawMessage `json:"request"`
	Submissions json.RawMessage `json:"submissions"`
	Events      json.RawMessage `json:"events"`
}

func scriptPollerWorkRequestFromSubmissions(data []byte) (interfaces.WorkRequest, error) {
	var submissions []interfaces.SubmitRequest
	if err := json.Unmarshal(data, &submissions); err != nil {
		return interfaces.WorkRequest{}, fmt.Errorf("script poller emitted malformed stdout: decode submissions: %w", err)
	}
	if len(submissions) == 0 {
		return interfaces.WorkRequest{}, fmt.Errorf("script poller emitted malformed stdout: submissions must contain at least one item")
	}

	request := factory.WorkRequestFromSubmitRequests(submissions)
	if strings.TrimSpace(request.RequestID) == "" {
		return interfaces.WorkRequest{}, fmt.Errorf("script poller emitted malformed stdout: submissions must share a non-empty requestId")
	}
	return request, nil
}

func validateScriptPollerWorkRequest(request interfaces.WorkRequest) error {
	if request.Type != interfaces.WorkRequestTypeFactoryRequestBatch {
		return fmt.Errorf("script poller emitted malformed stdout: unsupported work request type %q", request.Type)
	}
	if strings.TrimSpace(request.RequestID) == "" {
		return fmt.Errorf("script poller emitted malformed stdout: work request must set requestId")
	}
	return nil
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
