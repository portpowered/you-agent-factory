package claude

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
		result, runErr := runStreaming(ctx, runner, command, observe)
		effectResult := EffectResult{DurationMillis: clock.Now().Sub(started).Milliseconds()}
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
	)
	command := providerservice.CommandRequest{
		Command: string(providers.IDClaude),
		Env: buildCommandEnv(
			request.ProcessEnvironment,
			request.EnvVars,
		),
		WorkDir: request.WorkingDirectory,
	}
	command.Args, command.Stdin = deliverPrompt(command.Command, args, request.UserMessage)
	return commanddispatch.Request(request.ExecuteRequest, command), nil
}

// promptCommandLineBudget is the composed command-line size at which the
// rendered prompt stops travelling in argv. The bound is the Windows
// CreateProcess limit, applied on every host so one factory definition
// dispatches identically everywhere instead of succeeding on Linux and being
// rejected by the process loader on Windows.
const promptCommandLineBudget = platformprocess.WindowsCommandLineLimit

// deliverPrompt chooses how the rendered prompt reaches the provider. The
// prompt stays in argv while the composed command line fits the budget, which
// keeps the observable command shape unchanged for ordinary work. A prompt
// large enough to push the command line past the budget moves to stdin, which
// print mode accepts and which is already how the codex adapter delivers every
// prompt. Without this the process loader rejects the spawn outright, so the
// payload size, not the request, decided whether work could dispatch at all.
func deliverPrompt(command string, args []string, prompt string) ([]string, []byte) {
	inline := append(append([]string(nil), args...), prompt)
	if platformprocess.ComposedCommandLineLength(command, inline) < promptCommandLineBudget {
		return inline, nil
	}
	return args, []byte(prompt)
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
	if failure, ok := commanddispatch.StartFailure(err); ok {
		return failure
	}
	return err
}
