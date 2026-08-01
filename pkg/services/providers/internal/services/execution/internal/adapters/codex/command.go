package codex

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	effects "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
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
func NewCommandEffect(runner effects.CommandRunner, clocks ...platformclock.Source) Effect {
	if runner == nil {
		return nil
	}
	var clock platformclock.Source
	if len(clocks) > 0 {
		clock = clocks[0]
	}
	return EffectFunc(func(
		ctx context.Context,
		request providers.ExecuteRequest,
		observe func([]byte) error,
	) (EffectResult, error) {
		var started time.Time
		if clock != nil {
			started = clock.Now()
		}
		command, err := buildCommand(request)
		if err != nil {
			return EffectResult{}, execution.AttemptFailure{NativeError: err}
		}
		result, runErr := runStreaming(ctx, runner, request, command, observe)
		durationMillis := int64(0)
		if clock != nil {
			durationMillis = clock.Now().Sub(started).Milliseconds()
		}
		effectResult := EffectResult{DurationMillis: durationMillis}
		if runErr != nil {
			return effectResult, nativeCommandError(ctx, runErr)
		}
		if result.ExitCode != 0 {
			return effectResult, exitFailureFromCommandResult(result)
		}
		return effectResult, nil
	})
}

func buildCommand(request providers.ExecuteRequest) (effects.CommandRequest, error) {
	if err := validateCodexOptionalCapabilities(request); err != nil {
		return effects.CommandRequest{}, err
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
		return effects.CommandRequest{}, fmt.Errorf("unsupported reasoning effort %q", request.ReasoningEffort)
	}
	if effort != "" {
		args = append(args, "--config", `model_reasoning_effort="`+effort+`"`)
	}
	args = append(args, "-")
	return commanddispatch.Request(request, effects.CommandRequest{
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
	runner effects.CommandRunner,
	request providers.ExecuteRequest,
	command effects.CommandRequest,
	observe func([]byte) error,
) (effects.CommandResult, error) {
	if streaming, ok := runner.(effects.StreamingCommandRunner); ok {
		return streaming.RunStreaming(ctx, command, func(stream string, chunk []byte) error {
			if strings.TrimSpace(stream) == effects.OutputStreamStdout {
				return observe(chunk)
			}
			return nil
		})
	}
	result, err := runner.Run(ctx, command)
	if len(result.Stdout) > 0 {
		observeErr := observe(result.Stdout)
		err = errors.Join(err, observeErr)
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
