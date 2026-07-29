package gemini

import (
	"context"
	"errors"
	"strings"
	"time"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/commanddispatch"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

var commandAutomationDefaults = []workers.CommandEnvEntry{
	{Name: "GIT_EDITOR", Value: "true"},
	{Name: "GIT_SEQUENCE_EDITOR", Value: "true"},
	{Name: "GIT_MERGE_AUTOEDIT", Value: "no"},
	{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
	{Name: "EDITOR", Value: "true"},
	{Name: "VISUAL", Value: "true"},
}

// NewCommandEffect binds one final-only subprocess runner to the Gemini adapter.
func NewCommandEffect(runner workers.CommandRunner) Effect {
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
		result, runErr := runner.Run(ctx, command)
		effectResult := EffectResult{DurationMillis: time.Since(started).Milliseconds()}
		if runErr != nil {
			return effectResult, nativeCommandError(ctx, runErr)
		}
		if result.ExitCode != 0 {
			return effectResult, exitFailureFromCommandResult(result)
		}
		if len(result.Stdout) > 0 {
			if observeErr := observe(result.Stdout); observeErr != nil {
				return effectResult, observeErr
			}
		}
		return effectResult, nil
	})
}

func buildCommand(request providers.ExecuteRequest) (workers.CommandRequest, error) {
	if err := validateGeminiOptionalCapabilities(request); err != nil {
		return workers.CommandRequest{}, err
	}
	args := []string{"--prompt", request.UserMessage}
	if strings.TrimSpace(request.Model) != "" {
		args = append(args, "--model", strings.TrimSpace(request.Model))
	}
	if request.SkipPermissions {
		args = append(args, "--approval-mode", "yolo", "--sandbox", "false")
	}
	return commanddispatch.WorkersCommand(request, workers.CommandRequest{
		Command: string(providers.IDGemini),
		Args:    args,
		Env: buildCommandEnv(
			request.ProcessEnvironment,
			request.EnvVars,
		),
		WorkDir: request.WorkingDirectory,
	}), nil
}

func validateGeminiOptionalCapabilities(request providers.ExecuteRequest) error {
	if strings.TrimSpace(request.OutputSchema) != "" {
		return errors.New("structured output is not supported by the gemini runner in v1")
	}
	if request.ResumeSession != nil && strings.TrimSpace(request.ResumeSession.ID) != "" {
		return errors.New("session resume is not supported by the gemini runner in v1")
	}
	if strings.TrimSpace(request.Worktree) != "" {
		return errors.New("worktree selection is not supported by the gemini runner in v1")
	}
	return nil
}

func buildCommandEnv(processEnvironment []string, envVars map[string]string) []string {
	return workers.MergeCommandEnv(
		processEnvironment,
		workers.CommandEnvEntriesFromMap(envVars),
		commandAutomationDefaults,
	)
}

func nativeCommandError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return providers.ExecuteFailure{
			Kind:    providers.ExecuteFailureKindTimeout,
			Message: TimeoutFailureMessage,
		}
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled}
	}
	return err
}
