package codex

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
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
func NewCommandEffect(runner platformprocess.CommandRunner) Effect {
	if runner == nil {
		return nil
	}
	return EffectFunc(func(
		ctx context.Context,
		request providers.ExecuteRequest,
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
			return effectResult, fmt.Errorf("codex exited with code %d", result.ExitCode)
		}
		return effectResult, nil
	})
}

func buildCommand(request providers.ExecuteRequest) (platformprocess.CommandRequest, error) {
	if err := validateCodexOptionalCapabilities(request); err != nil {
		return platformprocess.CommandRequest{}, err
	}
	args := []string{"exec", "--json"}
	if request.SkipPermissions {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	}
	if strings.TrimSpace(request.Model) != "" {
		args = append(args, "--model", strings.TrimSpace(request.Model))
	}
	args = append(args, "-")
	return platformprocess.CommandRequest{
		Command: string(providers.IDCodex),
		Args:    args,
		Stdin:   []byte(request.UserMessage),
		Env: buildCommandEnv(
			request.ProcessEnvironment,
			request.EnvVars,
		),
		WorkDir: request.WorkingDirectory,
	}, nil
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
	runner platformprocess.CommandRunner,
	command platformprocess.CommandRequest,
	observe func([]byte) error,
) (platformprocess.CommandResult, error) {
	if streaming, ok := runner.(interface {
		RunStreaming(context.Context, platformprocess.CommandRequest, platformprocess.OutputChunkObserver) (platformprocess.CommandResult, error)
	}); ok {
		return streaming.RunStreaming(ctx, command, func(stream string, chunk []byte) {
			if strings.TrimSpace(stream) == platformprocess.OutputStreamStdout {
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
