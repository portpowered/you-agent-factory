package agy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	platformclock "github.com/portpowered/infinite-you/pkg/platform/clock"
	providers "github.com/portpowered/infinite-you/pkg/services/providers"
	providerservice "github.com/portpowered/infinite-you/pkg/services/providers/internal/service"
	execution "github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/adapters/commanddispatch"
	"github.com/portpowered/infinite-you/pkg/services/providers/internal/services/execution/internal/commandenv"
)

const (
	defaultPrintTimeout = providers.DefaultAntigravityPrintTimeout
	outputFormatStream  = "stream-json"
	agyExecutable       = "agy"
)

// NewCommandEffect binds the canonical AGY print-mode invocation to the
// Providers command-runner boundary. The command runner owns process
// creation; this adapter owns only AGY's argv and timeout policy.

func NewCommandEffect(runner providerservice.CommandRunner, clock platformclock.Source, scheduler platformclock.TimerSource) Effect {
	if runner == nil || clock == nil || scheduler == nil {
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
			return EffectResult{}, providers.ExecuteFailure{
				Kind:    providers.ExecuteFailureKindInvalidRequest,
				Message: err.Error(),
			}
		}

		timeout := effectivePrintTimeout(request.PrintTimeout)
		commandContext, cancel := printTimeoutContext(normalizeContext(ctx), scheduler, timeout)
		defer cancel()
		result, runErr := runCommand(commandContext, runner, command, observe)
		effectResult := EffectResult{
			DurationMillis: clock.Now().Sub(started).Milliseconds(),
			Metadata:       map[string]string{"output_format": outputFormatForRequest(request)},
			SessionRef:     sessionRefFromRequest(request.ResumeSession),
			CapturedStdout: append([]byte(nil), result.Stdout...),
		}
		if runErr != nil {
			return effectResult, nativeCommandError(commandContext, runErr)
		}
		if result.ExitCode != 0 {
			return effectResult, commandFailure(result)
		}
		return effectResult, nil
	})
}

// printTimeoutContext keeps the runner's deadline error identity while the
// supplied scheduler owns delivery. Cleanup joins the timer watcher before
// releasing the attempt, including when the command finishes before its limit.
func printTimeoutContext(parent context.Context, scheduler platformclock.TimerSource, timeout time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	deadline := scheduler.Now().Add(timeout)
	if parentDeadline, ok := parent.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	timer := scheduler.NewTimer(timeout)
	joined := make(chan struct{})
	go func() {
		defer close(joined)
		select {
		case <-ctx.Done():
		case <-timer.C():
			cancel(printTimeoutCause{})
		}
	}()
	return printDeadlineContext{Context: ctx, deadline: deadline}, func() {
		cancel(context.Canceled)
		timer.Stop()
		<-joined
	}
}

type printDeadlineContext struct {
	context.Context
	deadline time.Time
}

func (ctx printDeadlineContext) Deadline() (time.Time, bool) { return ctx.deadline, true }

func (ctx printDeadlineContext) Err() error {
	if _, expired := context.Cause(ctx.Context).(printTimeoutCause); expired {
		return context.DeadlineExceeded
	}
	return ctx.Context.Err()
}

type printTimeoutCause struct{}

func (printTimeoutCause) Error() string { return context.DeadlineExceeded.Error() }
func (printTimeoutCause) Unwrap() error { return context.DeadlineExceeded }

func buildCommand(request execution.ContinuationRequest) (providerservice.CommandRequest, error) {
	workDir := strings.TrimSpace(request.WorkingDirectory)
	if workDir == "" {
		workDir = "."
	}
	request.WorkingDirectory = workDir
	args, err := buildAgyArgs(request)
	if err != nil {
		return providerservice.CommandRequest{}, err
	}

	return commanddispatch.Request(request.ExecuteRequest, providerservice.CommandRequest{
		Command: agyExecutable,
		Args:    args,
		Env: commandenv.Build(
			request.ProcessEnvironment,
			request.EnvVars,
		),
		WorkDir: workDir,
	}), nil
}

func buildAgyArgs(request execution.ContinuationRequest) ([]string, error) {
	model := strings.TrimSpace(request.Model)
	if model != "" && !isSupportedModel(model) {
		return nil, fmt.Errorf("agy: unsupported model %q", model)
	}
	effort, ok := providers.ReasoningEffort(request.ReasoningEffort).Canonical()
	if !ok || !isSupportedEffort(effort) {
		return nil, fmt.Errorf("agy: unsupported reasoning effort %q; want low, medium, or high", request.ReasoningEffort)
	}
	if request.PrintTimeout < 0 {
		return nil, fmt.Errorf("agy: print timeout must not be negative")
	}
	workDir := strings.TrimSpace(request.WorkingDirectory)
	if workDir == "" {
		workDir = "."
	}
	args := []string{
		"-p", request.UserMessage,
		"--output-format", outputFormatForRequest(request),
		"--add-dir", workDir,
		"--disable-slash-commands",
	}
	outputSchema := strings.TrimSpace(request.OutputSchema)
	if outputSchema != "" {
		if !json.Valid([]byte(outputSchema)) {
			return nil, fmt.Errorf("agy: output schema must be valid JSON")
		}
		args = append(args, "--json-schema", outputSchema)
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	if effort != "" {
		args = append(args, "--effort", effort)
	}
	if request.SkipPermissions {
		args = append(args, "--dangerously-skip-permissions")
	}
	args = append(args, "--print-timeout", formatPrintTimeout(effectivePrintTimeout(request.PrintTimeout)))
	if request.ResumeSession != nil {
		if sessionID := strings.TrimSpace(request.ResumeSession.ID); sessionID != "" {
			args = append(args, "--session", sessionID)
		}
	}
	return args, nil
}

func outputFormatForRequest(request execution.ContinuationRequest) string {
	if strings.TrimSpace(request.OutputSchema) != "" {
		return agyOutputFormatJSON
	}
	return outputFormatStream
}

func runCommand(
	ctx context.Context,
	runner providerservice.CommandRunner,
	command providerservice.CommandRequest,
	observe func([]byte) error,
) (providerservice.CommandResult, error) {
	if streaming, ok := runner.(providerservice.StreamingCommandRunner); ok {
		return streaming.RunStreaming(ctx, command, func(stream string, chunk []byte) error {
			if strings.TrimSpace(stream) != providerservice.OutputStreamStdout || len(chunk) == 0 {
				return nil
			}
			return observe(chunk)
		})
	}
	result, err := runner.Run(ctx, command)
	if len(result.Stdout) > 0 {
		if observeErr := observe(result.Stdout); err == nil {
			err = observeErr
		}
	}
	return result, err
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

func normalizeContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func effectivePrintTimeout(requested time.Duration) time.Duration {
	if requested <= 0 {
		return defaultPrintTimeout
	}
	return requested
}

func formatPrintTimeout(timeout time.Duration) string {
	if timeout%time.Minute == 0 {
		return fmt.Sprintf("%dm", timeout/time.Minute)
	}
	return timeout.String()
}

func isSupportedEffort(effort string) bool {
	switch effort {
	case "", "low", "medium", "high":
		return true
	default:
		return false
	}
}

func isSupportedModel(model string) bool {
	switch model {
	case
		"gemini-3.6-flash-high",
		"gemini-3.6-flash-medium",
		"gemini-3.6-flash-low",
		"gemini-3.5-flash-high",
		"gemini-3.5-flash-medium",
		"gemini-3.5-flash-low",
		"gemini-3.1-pro-high",
		"gemini-3.1-pro-low",
		"claude-sonnet-4-6",
		"claude-opus-4-6-thinking",
		"gpt-oss-120b-medium":
		return true
	default:
		return false
	}
}
