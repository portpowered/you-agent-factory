package pi

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

// CommandFacts retain bounded subprocess facts for one native attempt.
type CommandFacts struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	RunError error
}

var commandAutomationDefaults = []workers.CommandEnvEntry{
	{Name: "GIT_EDITOR", Value: "true"},
	{Name: "GIT_SEQUENCE_EDITOR", Value: "true"},
	{Name: "GIT_MERGE_AUTOEDIT", Value: "no"},
	{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
	{Name: "EDITOR", Value: "true"},
	{Name: "VISUAL", Value: "true"},
}

// NewCommandEffect binds one streaming subprocess runner to the Pi adapter.
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
		result, runErr := runStreaming(ctx, runner, command, observe)
		effectResult := EffectResult{
			DurationMillis: time.Since(started).Milliseconds(),
			Metadata:       map[string]string{"output_mode": "json"},
			Command:        commandFactsFromResult(result, runErr),
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

func commandFactsFromResult(result workers.CommandResult, runErr error) CommandFacts {
	return CommandFacts{
		ExitCode: result.ExitCode,
		Stdout:   append([]byte(nil), result.Stdout...),
		Stderr:   append([]byte(nil), result.Stderr...),
		RunError: runErr,
	}
}

func buildCommand(request providers.ExecuteRequest) (workers.CommandRequest, error) {
	if err := validatePiOptionalCapabilities(request); err != nil {
		return workers.CommandRequest{}, err
	}
	args := []string{"--print", "--mode", "json", "--approve"}
	if strings.TrimSpace(request.Model) != "" {
		args = append(args, "--model", strings.TrimSpace(request.Model))
	}
	if request.ResumeSession != nil && strings.TrimSpace(request.ResumeSession.ID) != "" {
		args = append(args, "--session", strings.TrimSpace(request.ResumeSession.ID))
	}
	if strings.TrimSpace(request.SystemPrompt) != "" {
		args = append(args, "--system-prompt", strings.TrimSpace(request.SystemPrompt))
	}
	args = append(args, request.UserMessage)
	command := commanddispatch.WorkersCommand(request, workers.CommandRequest{
		Command: string(providers.IDPi),
		Args:    args,
		Env: buildCommandEnv(
			request.ProcessEnvironment,
			request.EnvVars,
		),
		WorkDir: request.WorkingDirectory,
	})
	if len(request.InputTokens) > 0 {
		command.InputTokens = append([]any(nil), request.InputTokens...)
	}
	return command, nil
}

func validatePiOptionalCapabilities(request providers.ExecuteRequest) error {
	if strings.TrimSpace(request.OutputSchema) != "" {
		return errors.New("structured output is not supported by the pi runner in v1")
	}
	if strings.TrimSpace(request.Worktree) != "" {
		return errors.New("worktree selection is not supported by the pi runner in v1")
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

func runStreaming(
	ctx context.Context,
	runner workers.CommandRunner,
	command workers.CommandRequest,
	observe func([]byte) error,
) (workers.CommandResult, error) {
	if streaming, ok := runner.(interface {
		RunStreaming(context.Context, workers.CommandRequest, workers.OutputChunkObserver) (workers.CommandResult, error)
	}); ok {
		return streaming.RunStreaming(ctx, command, func(stream string, chunk []byte) {
			if strings.TrimSpace(stream) == workers.OutputStreamStdout {
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
