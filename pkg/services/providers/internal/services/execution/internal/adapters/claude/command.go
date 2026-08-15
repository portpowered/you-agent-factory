package claude

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/commanddispatch"
)

const (
	outputFormatStreamJSON = "stream-json"
)

var commandAutomationDefaults = []platformprocess.CommandEnvEntry{
	{Name: "GIT_EDITOR", Value: "true"},
	{Name: "GIT_SEQUENCE_EDITOR", Value: "true"},
	{Name: "GIT_MERGE_AUTOEDIT", Value: "no"},
	{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
	{Name: "EDITOR", Value: "true"},
	{Name: "VISUAL", Value: "true"},
}

// NewCommandEffect binds one streaming subprocess runner to the Claude adapter.

func NewCommandEffect(candidate any) Effect {
	runner := providerservice.AdaptCommandRunner(candidate)
	if runner == nil {
		return nil
	}
	return EffectFunc(func(
		ctx context.Context,
		request execution.ContinuationRequest,
		observe func([]byte) error,
	) (EffectResult, error) {
		started := time.Now()
		command, err := buildCommand(request)
		if err != nil {
			return EffectResult{}, execution.AttemptFailure{NativeError: err}
		}
		result, runErr := runStreaming(ctx, runner, command, observe)
		effectResult := EffectResult{DurationMillis: time.Since(started).Milliseconds()}
		if runErr != nil {
			return effectResult, nativeCommandError(ctx, runErr)
		}
		if result.ExitCode != 0 {
			return effectResult, exitFailureFromCommandResult(result)
		}
		return effectResult, nil
	})
}

func buildCommand(request execution.ContinuationRequest) (providerservice.CommandRequest, error) {
	args := []string{"-p"}
	if request.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	if strings.TrimSpace(request.Worktree) != "" {
		args = append(args, "--worktree", strings.TrimSpace(request.Worktree))
	}
	if strings.TrimSpace(request.SystemPrompt) != "" {
		args = append(args, "--system-prompt", strings.TrimSpace(request.SystemPrompt))
	}
	if strings.TrimSpace(request.Model) != "" {
		args = append(args, "--model", strings.TrimSpace(request.Model))
	}
	effort, ok := providers.ReasoningEffort(request.ReasoningEffort).Canonical()
	if !ok {
		return providerservice.CommandRequest{}, fmt.Errorf("unsupported reasoning effort %q", request.ReasoningEffort)
	}
	if effort != "" {
		if effort == "minimal" {
			return providerservice.CommandRequest{}, fmt.Errorf(`Claude does not support reasoning effort "minimal"`)
		}
		args = append(args, "--effort", effort)
	}
	if request.ResumeSession != nil && strings.TrimSpace(request.ResumeSession.ID) != "" {
		args = append(args, "--resume", strings.TrimSpace(request.ResumeSession.ID))
	}
	args = append(
		args,
		"--verbose",
		"--output-format",
		outputFormatStreamJSON,
		"--include-partial-messages",
		request.UserMessage,
	)
	return commanddispatch.Request(request.ExecuteRequest, providerservice.CommandRequest{
		Command: string(providers.IDClaude),
		Args:    args,
		Env: buildCommandEnv(
			request.ProcessEnvironment,
			request.EnvVars,
		),
		WorkDir: request.WorkingDirectory,
	}), nil
}

func buildCommandEnv(processEnvironment []string, envVars map[string]string) []string {
	return platformprocess.MergeCommandEnv(
		processEnvironment,
		platformprocess.CommandEnvEntriesFromMap(envVars),
		commandAutomationDefaults,
	)
}

func runStreaming(
	ctx context.Context,
	runner providerservice.CommandRunner,
	command providerservice.CommandRequest,
	observe func([]byte) error,
) (providerservice.CommandResult, error) {
	if streaming, ok := runner.(interface {
		RunStreaming(context.Context, providerservice.CommandRequest, providerservice.OutputChunkObserver) (providerservice.CommandResult, error)
	}); ok {
		return streaming.RunStreaming(ctx, command, func(stream string, chunk []byte) error {
			if strings.TrimSpace(stream) != providerservice.OutputStreamStdout {
				return nil
			}
			return observe(chunk)
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
