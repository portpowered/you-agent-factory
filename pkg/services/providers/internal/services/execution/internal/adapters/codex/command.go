package codex

import (
	"context"
	"errors"
	"fmt"
	"strings"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/commanddispatch"
)

var commandAutomationDefaults = []platformprocess.CommandEnvEntry{
	{Name: "GIT_EDITOR", Value: "true"},
	{Name: "GIT_SEQUENCE_EDITOR", Value: "true"},
	{Name: "GIT_MERGE_AUTOEDIT", Value: "no"},
	{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
	{Name: "EDITOR", Value: "true"},
	{Name: "VISUAL", Value: "true"},
}

// NewCommandEffect binds one streaming subprocess runner to the Codex adapter.

func NewCommandEffect(candidate any, clock platformclock.Source) Effect {
	runner := providerservice.AdaptCommandRunner(candidate)
	if runner == nil || clock == nil {
		return nil
	}
	return EffectFunc(func(
		ctx context.Context,
		request execution.ContinuationRequest,
		observe func([]byte) error,
	) (EffectResult, error) {
		started := clock.Now()
		command, err := buildCommand(request)
		if err != nil {
			return EffectResult{}, execution.AttemptFailure{NativeError: err}
		}
		result, runErr := runStreaming(ctx, runner, request.ExecuteRequest, command, observe)
		effectResult := EffectResult{DurationMillis: clock.Now().Sub(started).Milliseconds()}
		if runErr != nil {
			return effectResult, nativeCommandError(ctx, runErr)
		}
		if result.ExitCode != 0 {
			return effectResult, exitFailureFromCommandResult(result, request.WorkingDirectory)
		}
		return effectResult, nil
	})
}

func buildCommand(request execution.ContinuationRequest) (providerservice.CommandRequest, error) {
	if err := validateCodexOptionalCapabilities(request.ExecuteRequest); err != nil {
		return providerservice.CommandRequest{}, err
	}
	args := []string{"exec", "--json"}
	if request.SkipPermissions {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	}
	if strings.TrimSpace(request.Model) != "" {
		args = append(args, "--model", strings.TrimSpace(request.Model))
	}
	effort, ok := providers.ReasoningEffort(request.ReasoningEffort).Canonical()
	if !ok {
		return providerservice.CommandRequest{}, fmt.Errorf("unsupported reasoning effort %q", request.ReasoningEffort)
	}
	if effort != "" {
		args = append(args, "--config", `model_reasoning_effort="`+effort+`"`)
	}
	if request.ResumeSession != nil {
		if sessionID := strings.TrimSpace(request.ResumeSession.ID); sessionID != "" {
			args = append(args, "resume", sessionID)
		}
	}
	args = append(args, "-")
	return commanddispatch.Request(request.ExecuteRequest, providerservice.CommandRequest{
		Command: string(providers.IDCodex),
		Args:    args,
		Stdin:   []byte(request.UserMessage),
		Env: buildCommandEnv(
			request.ProcessEnvironment,
			request.EnvVars,
		),
		WorkDir: request.WorkingDirectory,
	}), nil
}

func validateCodexOptionalCapabilities(request providers.ExecuteRequest) error {
	if request.Worktree != "" && request.WorkingDirectory == "" {
		return errors.New("worktree selection is not supported by the codex runner in v1")
	}
	return nil
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
	request providers.ExecuteRequest,
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
	if failure, ok := commanddispatch.StartFailure(err); ok {
		return failure
	}
	return err
}
