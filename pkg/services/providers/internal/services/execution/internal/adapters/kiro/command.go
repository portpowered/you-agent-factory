package kiro

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

// NewCommandEffect binds one final-only subprocess runner to the Kiro adapter.
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
		effectResult := EffectResult{
			DurationMillis: time.Since(started).Milliseconds(),
			SessionRef:     sessionRefFromOutput(result.Stdout, result.Stderr, request.ResumeSession),
		}
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
	if err := validateKiroOptionalCapabilities(request); err != nil {
		return workers.CommandRequest{}, err
	}
	args := []string{"chat", "--no-interactive"}
	if request.ResumeSession != nil && strings.TrimSpace(request.ResumeSession.ID) != "" {
		args = append(args, "--resume-id", strings.TrimSpace(request.ResumeSession.ID))
	}
	if request.SkipPermissions {
		args = append(args, "--trust-all-tools")
	}
	if prompt := buildPrompt(request); prompt != "" {
		args = append(args, prompt)
	}
	command := commanddispatch.WorkersCommand(request, workers.CommandRequest{
		Command: string(providers.IDKiro),
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

func validateKiroOptionalCapabilities(request providers.ExecuteRequest) error {
	if strings.TrimSpace(request.OutputSchema) != "" {
		return errors.New("structured output is not supported by the kiro runner in v1")
	}
	if strings.TrimSpace(request.Worktree) != "" {
		return errors.New("worktree selection is not supported by the kiro runner in v1")
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
