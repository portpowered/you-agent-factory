package opencode

import (
	"context"
	"errors"
	"strings"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/commanddispatch"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
)

// CommandEffectOptions configure one built-in OpenCode command effect.
type CommandEffectOptions struct {
	Mode Mode
}

var commandAutomationDefaults = []workerprocess.CommandEnvEntry{
	{Name: "GIT_EDITOR", Value: "true"},
	{Name: "GIT_SEQUENCE_EDITOR", Value: "true"},
	{Name: "GIT_MERGE_AUTOEDIT", Value: "no"},
	{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
	{Name: "EDITOR", Value: "true"},
	{Name: "VISUAL", Value: "true"},
}

// NewCommandEffect binds one streaming subprocess runner to the OpenCode adapter.
func NewCommandEffect(
	runner workers.CommandRunner,
	options ...CommandEffectOptions,
) Effect {
	if runner == nil {
		return nil
	}
	var config CommandEffectOptions
	if len(options) > 0 {
		config = options[0]
	}
	if config.Mode == "" {
		config.Mode = ModeStructured
	}
	return EffectFunc(func(
		ctx context.Context,
		request providers.ExecuteRequest,
		observe func([]byte) error,
	) (EffectResult, error) {
		started := time.Now()
		command, err := buildCommand(request, config.Mode)
		if err != nil {
			return EffectResult{}, execution.AttemptFailure{NativeError: err}
		}
		result, runErr := runStreaming(ctx, runner, command, observe)
		effectResult := EffectResult{
			DurationMillis: time.Since(started).Milliseconds(),
			Metadata: map[string]string{
				"output_mode": string(config.Mode),
			},
		}
		if runErr != nil {
			return effectResult, nativeCommandError(ctx, runErr)
		}
		if result.ExitCode != 0 {
			return effectResult, exitFailureFromCommandResult(result)
		}
		return effectResult, nil
	})
}

func buildCommand(
	request providers.ExecuteRequest,
	mode Mode,
) (workers.CommandRequest, error) {
	if err := validateOpenCodeOptionalCapabilities(request); err != nil {
		return workers.CommandRequest{}, err
	}
	args := []string{"run"}
	if mode == ModeStructured {
		args = append(args, "--format", "json")
	}
	if strings.TrimSpace(request.Model) != "" {
		args = append(args, "--model", strings.TrimSpace(request.Model))
	}
	if request.ResumeSession != nil && strings.TrimSpace(request.ResumeSession.ID) != "" {
		args = append(args, "--session", strings.TrimSpace(request.ResumeSession.ID))
	}
	if strings.TrimSpace(request.WorkingDirectory) != "" {
		args = append(args, "--dir", strings.TrimSpace(request.WorkingDirectory))
	}
	if request.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	args = append(args, buildPrompt(request))
	return commanddispatch.WorkersCommand(request, workers.CommandRequest{
		Command: string(providers.IDOpenCode),
		Args:    args,
		Env: buildCommandEnv(
			request.ProcessEnvironment,
			request.EnvVars,
		),
		WorkDir: request.WorkingDirectory,
	}), nil
}

func buildPrompt(request providers.ExecuteRequest) string {
	systemPrompt := strings.TrimSpace(request.SystemPrompt)
	userMessage := strings.TrimSpace(request.UserMessage)
	switch {
	case systemPrompt == "":
		return userMessage
	case userMessage == "":
		return systemPrompt
	default:
		return "System instructions:\n" + systemPrompt + "\n\nUser request:\n" + userMessage
	}
}

func validateOpenCodeOptionalCapabilities(request providers.ExecuteRequest) error {
	if request.Worktree != "" {
		return errors.New("worktree selection is not supported by the opencode runner in v1")
	}
	return nil
}

func buildCommandEnv(processEnvironment []string, envVars map[string]string) []string {
	return workerprocess.MergeCommandEnv(
		processEnvironment,
		workerprocess.CommandEnvEntriesFromMap(envVars),
		commandAutomationDefaults,
	)
}

func runStreaming(
	ctx context.Context,
	runner workers.CommandRunner,
	command workers.CommandRequest,
	observe func([]byte) error,
) (workers.CommandResult, error) {
	if streaming, ok := runner.(interface {
		RunStreaming(context.Context, workers.CommandRequest, workerprocess.OutputChunkObserver) (workers.CommandResult, error)
	}); ok {
		return streaming.RunStreaming(ctx, command, func(stream string, chunk []byte) {
			if strings.TrimSpace(stream) == workerprocess.OutputStreamStdout {
				_ = observe(chunk)
			}
		})
	}
	result, err := runner.Run(ctx, command)
	if len(result.Stdout) > 0 {
		observeErr := observe(result.Stdout)
		if err == nil {
			err = observeErr
		}
	}
	return result, err
}

func nativeCommandError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindTimeout}
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled}
	}
	return err
}
