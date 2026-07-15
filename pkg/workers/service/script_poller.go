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

	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"

	"go.uber.org/zap"

	"github.com/portpowered/infinite-you/pkg/config"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	"github.com/portpowered/infinite-you/pkg/factory/requests"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
)

const (
	// ScriptPollerRestartBackoffMin is the minimum restart delay after an unexpected script poller exit.
	ScriptPollerRestartBackoffMin = 25 * time.Millisecond
	scriptPollerRestartBackoffMax = 250 * time.Millisecond
)

// StartScriptPoller supervises one script poller workstation until ctx is canceled.
func (s *Service) StartScriptPoller(
	ctx context.Context,
	sidecars *sync.WaitGroup,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *workerconfig.Config,
	submitter WorkRequestSubmitter,
) {
	if sidecars == nil || submitter == nil {
		return
	}
	sidecars.Add(1)
	go func() {
		defer sidecars.Done()
		s.superviseScriptPoller(ctx, runtimeCfg, workstation, workerDef, submitter)
	}()
}

func (s *Service) superviseScriptPoller(
	ctx context.Context,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *workerconfig.Config,
	submitter WorkRequestSubmitter,
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
		runErr := s.RunScriptPoller(ctx, runner, runtimeCfg, workstation, workerDef, submitter)
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

// RunScriptPoller executes one script poller command cycle and submits any parsed work request.
func (s *Service) RunScriptPoller(
	ctx context.Context,
	runner workers.CommandRunner,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *workerconfig.Config,
	submitter WorkRequestSubmitter,
) error {
	commandReq, err := ScriptPollerCommandRequest(runtimeCfg, workstation, workerDef)
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
	request, hasOutput, err := ParseScriptPollerOutput(result.Stdout)
	if err != nil {
		return err
	}
	if hasOutput {
		if submitter == nil {
			return fmt.Errorf("script poller submitter is not available")
		}
		if err := submitter(ctx, request); err != nil {
			return fmt.Errorf("script poller submit failed: %w", err)
		}
	}
	return fmt.Errorf("script poller exited unexpectedly")
}

func scriptPollerRestartBackoff(attempt int) time.Duration {
	if attempt <= 1 {
		return ScriptPollerRestartBackoffMin
	}
	backoff := ScriptPollerRestartBackoffMin
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

// ScriptPollerCommandRequest builds the command invocation for a script poller worker.
func ScriptPollerCommandRequest(
	runtimeCfg interfaces.RuntimeConfigLookup,
	workstation interfaces.FactoryWorkstationConfig,
	workerDef *workerconfig.Config,
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
	if resolved, err := workerprompting.ResolveTemplateFields(
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
	if workDir == "" {
		workDir = pollerRuntimeWorkingDirectory(runtimeCfg)
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

func pollerRuntimeWorkingDirectory(runtimeCfg interfaces.RuntimeConfigLookup) string {
	if runtimeCfg == nil {
		return ""
	}
	baseDir := strings.TrimSpace(runtimeCfg.RuntimeBaseDir())
	if baseDir == "" {
		baseDir = strings.TrimSpace(runtimeCfg.FactoryDir())
	}
	if baseDir == "" {
		return ""
	}
	return filepath.Clean(baseDir)
}

func scriptPollerExecutionTimeout(workstation interfaces.FactoryWorkstationConfig, workerDef *workerconfig.Config) (time.Duration, error) {
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

// ParseScriptPollerOutput parses stdout from a script poller into a work request.
func ParseScriptPollerOutput(stdout []byte) (work.WorkRequest, bool, error) {
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return work.WorkRequest{}, false, nil
	}

	var envelope scriptPollerOutputEnvelope
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return work.WorkRequest{}, true, fmt.Errorf("script poller emitted malformed stdout: %w", err)
	}
	if len(envelope.Events) > 0 {
		return work.WorkRequest{}, true, fmt.Errorf("script poller emitted unsupported raw factory events")
	}
	if len(envelope.Request) > 0 && len(envelope.Submissions) > 0 {
		return work.WorkRequest{}, true, fmt.Errorf("script poller stdout must contain either request or submissions, not both")
	}
	if len(envelope.Request) > 0 {
		request, err := requests.ParseCanonicalWorkRequestJSON(envelope.Request)
		if err != nil {
			return work.WorkRequest{}, true, fmt.Errorf("script poller emitted malformed stdout: %w", err)
		}
		if err := validateScriptPollerWorkRequest(request); err != nil {
			return work.WorkRequest{}, true, err
		}
		return request, true, nil
	}
	if len(envelope.Submissions) > 0 {
		request, err := scriptPollerWorkRequestFromSubmissions(envelope.Submissions)
		if err != nil {
			return work.WorkRequest{}, true, err
		}
		return request, true, nil
	}

	request, err := requests.ParseCanonicalWorkRequestJSON(trimmed)
	if err != nil {
		return work.WorkRequest{}, true, fmt.Errorf("script poller emitted malformed stdout: %w", err)
	}
	if err := validateScriptPollerWorkRequest(request); err != nil {
		return work.WorkRequest{}, true, err
	}
	return request, true, nil
}

type scriptPollerOutputEnvelope struct {
	Request     json.RawMessage `json:"request"`
	Submissions json.RawMessage `json:"submissions"`
	Events      json.RawMessage `json:"events"`
}

func scriptPollerWorkRequestFromSubmissions(data []byte) (work.WorkRequest, error) {
	var submissions []work.SubmitRequest
	if err := json.Unmarshal(data, &submissions); err != nil {
		return work.WorkRequest{}, fmt.Errorf("script poller emitted malformed stdout: decode submissions: %w", err)
	}
	if len(submissions) == 0 {
		return work.WorkRequest{}, fmt.Errorf("script poller emitted malformed stdout: submissions must contain at least one item")
	}

	request := requests.WorkRequestFromSubmitRequests(submissions)
	if strings.TrimSpace(request.RequestID) == "" {
		return work.WorkRequest{}, fmt.Errorf("script poller emitted malformed stdout: submissions must share a non-empty requestId")
	}
	return request, nil
}

func validateScriptPollerWorkRequest(request work.WorkRequest) error {
	if request.Type != work.WorkRequestTypeFactoryRequestBatch {
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
