package cursor

import (
	"context"
	"errors"
	"strings"
	"time"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/commanddispatch"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// CommandEffectOptions are platform facts required for oversized Windows prompt
// materialization. Omitted dependencies are valid for direct prompts; oversized
// Windows prompts fail closed without their required temporary-file effect.
type CommandEffectOptions struct {
	OperatingSystem string
	TemporaryDir    string
	TemporaryFiles  platformfilesystem.TemporaryFileSystem
}

const (
	cursorAgentCommand       = "cursor"
	cursorOutputFormatStream = "stream-json"
)

var commandAutomationDefaults = []workers.CommandEnvEntry{
	{Name: "GIT_EDITOR", Value: "true"},
	{Name: "GIT_SEQUENCE_EDITOR", Value: "true"},
	{Name: "GIT_MERGE_AUTOEDIT", Value: "no"},
	{Name: "GIT_TERMINAL_PROMPT", Value: "0"},
	{Name: "EDITOR", Value: "true"},
	{Name: "VISUAL", Value: "true"},
}

// NewCommandEffect binds one streaming subprocess runner to the Cursor adapter.
func NewCommandEffect(
	runner workers.CommandRunner,
	options ...CommandEffectOptions,
) Effect {
	if runner == nil {
		return nil
	}
	var platform CommandEffectOptions
	if len(options) > 0 {
		platform = options[0]
	}
	return EffectFunc(func(
		ctx context.Context,
		request providers.ExecuteRequest,
		observe func([]byte) error,
	) (EffectResult, error) {
		started := time.Now()
		command, cleanup, err := buildCommand(ctx, request, platform)
		if cleanup != nil {
			defer cleanup()
		}
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

func buildCommand(
	ctx context.Context,
	request providers.ExecuteRequest,
	platform CommandEffectOptions,
) (workers.CommandRequest, func(), error) {
	if err := validateCursorOptionalCapabilities(request); err != nil {
		return workers.CommandRequest{}, nil, err
	}
	args := make([]string, 0, 14)
	if request.SkipPermissions {
		args = append(args, "-f")
	}
	args = append(args, "-p")
	if strings.TrimSpace(request.Model) != "" {
		args = append(args, "--model", strings.TrimSpace(request.Model))
	}
	if request.ResumeSession != nil && strings.TrimSpace(request.ResumeSession.ID) != "" {
		args = append(args, "--resume", strings.TrimSpace(request.ResumeSession.ID))
	}
	if strings.TrimSpace(request.WorkingDirectory) != "" {
		args = append(args, "--workspace", strings.TrimSpace(request.WorkingDirectory))
	}
	if strings.TrimSpace(request.Worktree) != "" {
		args = append(args, "--worktree", strings.TrimSpace(request.Worktree))
	}
	args = append(
		args,
		"--output-format", cursorOutputFormatStream,
		"--stream-partial-output",
		buildPrompt(request),
	)
	args, cleanup, err := materializePrompt(ctx, request, platform, args)
	if err != nil {
		if cleanup != nil {
			cleanup()
		}
		return workers.CommandRequest{}, nil, err
	}
	return commanddispatch.WorkersCommand(request, workers.CommandRequest{
		Command: cursorAgentCommand,
		Args:    args,
		Env: buildCommandEnv(
			request.ProcessEnvironment,
			request.EnvVars,
		),
		WorkDir: request.WorkingDirectory,
	}), cleanup, nil
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

func validateCursorOptionalCapabilities(request providers.ExecuteRequest) error {
	if strings.TrimSpace(request.OutputSchema) != "" {
		return errors.New("structured output is not supported by the cursor-cli runner in v1")
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

func nativeCommandError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindTimeout}
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return providers.ExecuteFailure{Kind: providers.ExecuteFailureKindCanceled}
	}
	return err
}
