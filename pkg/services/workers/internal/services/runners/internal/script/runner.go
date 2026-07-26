// Package script implements the Workers-parent-private Script Runner.
package script

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/services/work"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/prompting"
)

const Identity = "script"

const (
	scriptAttempt               = 1
	scriptRequestEventIDPrefix  = "factory-event/script-request"
	scriptResponseEventIDPrefix = "factory-event/script-response"
)

// Config is the immutable script definition captured by one Runner.
type Config struct {
	Command          string
	Args             []string
	FactoryDirectory string
}

// Dependencies are the exact effects used by one Script Runner.
type Dependencies struct {
	CommandRunner workers.CommandRunner
	FactoryDocs   workers.FactoryDocsLoader
	Now           func() time.Time
	Publish       workers.ProgressPublisher
	Record        workers.ScriptEventRecorder
}

type runner struct {
	command          string
	args             []string
	factoryDirectory string
	commandRunner    streamingCommandRunner
	factoryDocs      workers.FactoryDocsLoader
	now              func() time.Time
	publish          workers.ProgressPublisher
	record           workers.ScriptEventRecorder
}

type streamingCommandRunner interface {
	RunStreaming(
		context.Context,
		workers.CommandRequest,
		platformprocess.OutputChunkObserver,
	) (workers.CommandResult, error)
}

var _ workers.Runner = (*runner)(nil)

// New validates and snapshots a Script Runner and its exact execution edges.
func New(config Config, dependencies Dependencies) (workers.Runner, error) {
	if strings.TrimSpace(config.Command) == "" {
		return nil, misconfigured("script command is required", nil)
	}
	if dependencies.CommandRunner == nil {
		return nil, misconfigured("script command runner is required", nil)
	}
	commandRunner, ok := dependencies.CommandRunner.(streamingCommandRunner)
	if !ok {
		return nil, misconfigured("script command runner must support streaming", nil)
	}
	if dependencies.FactoryDocs == nil {
		return nil, misconfigured("script Factory docs loader is required", nil)
	}
	if dependencies.Now == nil {
		return nil, misconfigured("script clock is required", nil)
	}
	if dependencies.Publish == nil {
		return nil, misconfigured("script progress publisher is required", nil)
	}
	if dependencies.Record == nil {
		return nil, misconfigured("script event recorder is required", nil)
	}
	return &runner{
		command:          config.Command,
		args:             append([]string(nil), config.Args...),
		factoryDirectory: strings.TrimSpace(config.FactoryDirectory),
		commandRunner:    commandRunner,
		factoryDocs:      dependencies.FactoryDocs,
		now:              dependencies.Now,
		publish:          dependencies.Publish,
		record:           dependencies.Record,
	}, nil
}

func (r *runner) Execute(
	ctx context.Context,
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionResult, error) {
	if err := ctx.Err(); err != nil {
		return workers.RunnerExecutionResult{}, err
	}
	request, tokens, err := snapshotRequest(request)
	if err != nil {
		return workers.RunnerExecutionResult{}, badRequest("script input is invalid", err)
	}
	if err := validateRequest(request, tokens); err != nil {
		return workers.RunnerExecutionResult{}, err
	}

	commandRequest, err := r.resolveCommandRequest(request, tokens)
	if err != nil {
		return workers.RunnerExecutionResult{}, badRequest("script command template is invalid", err)
	}
	started := r.now()
	requestID := scriptRequestID(commandRequest.DispatchID)
	r.record(scriptRequestEvent(commandRequest, requestID, started))
	observer := r.outputObserver(commandRequest.DispatchID)
	result, err := r.commandRunner.RunStreaming(
		ctx,
		workers.CloneSubprocessExecutionRequest(commandRequest),
		observer,
	)
	finished := r.now()
	duration := finished.Sub(started)
	diagnostics := commandDiagnostics(commandRequest, result, duration)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return workers.RunnerExecutionResult{}, ctxErr
		}
		r.record(scriptFailureEvent(
			commandRequest,
			requestID,
			result,
			duration,
			workers.ScriptExecutionOutcomeProcessError,
			finished,
		))
		return failureResult(result, diagnostics), executionFailure(
			"script command execution failed",
			err,
			diagnostics,
		)
	}
	if result.ExitCode != 0 {
		r.record(scriptFailureEvent(
			commandRequest,
			requestID,
			result,
			duration,
			workers.ScriptExecutionOutcomeFailedExitCode,
			finished,
		))
		return failureResult(result, diagnostics), executionFailure(
			nonZeroExitMessage(result),
			nil,
			diagnostics,
		)
	}
	r.record(scriptSuccessEvent(commandRequest, requestID, result, duration, finished))
	return workers.RunnerExecutionResult{
		Content:     strings.TrimSpace(string(result.Stdout)),
		Diagnostics: workers.CloneWorkDiagnostics(diagnostics),
	}, nil
}

func failureResult(
	result workers.CommandResult,
	diagnostics *workers.WorkDiagnostics,
) workers.RunnerExecutionResult {
	return workers.RunnerExecutionResult{
		Content:     strings.TrimSpace(string(result.Stdout)),
		Diagnostics: workers.CloneWorkDiagnostics(diagnostics),
	}
}

func executionFailure(
	message string,
	cause error,
	diagnostics *workers.WorkDiagnostics,
) error {
	failure := workers.NewProviderError(
		workers.WorkFailureTypeInternalServerError,
		message,
		cause,
	)
	failure.Diagnostics = workers.CloneWorkDiagnostics(diagnostics)
	return failure
}

func nonZeroExitMessage(result workers.CommandResult) string {
	if message := strings.TrimSpace(string(result.Stderr)); message != "" {
		return message
	}
	return fmt.Sprintf("script command exited with status %d", result.ExitCode)
}

func (r *runner) resolveCommandRequest(
	request workers.RunnerExecutionRequest,
	tokens []workers.Token,
) (workers.CommandRequest, error) {
	workDir := effectiveWorkDir(request)
	templateContext := &workers.Context{
		FactoryDirectory: r.factoryDirectory,
		WorkDirectory:    workDir,
		EnvVars:          cloneStringMap(request.EnvVars),
		ProjectID:        request.ProjectID,
		SessionID:        request.SessionID,
	}
	data, err := prompting.BuildPromptDataWithFactoryDocs(
		tokens,
		templateContext,
		r.factoryDocs,
	)
	if err != nil {
		return workers.CommandRequest{}, err
	}
	args, err := resolveArgs(r.args, data)
	if err != nil {
		return workers.CommandRequest{}, err
	}

	dispatch := request.Dispatch
	return workers.CommandRequest{
		Command:                  resolveFactoryScript(r.factoryDirectory, r.command),
		Args:                     resolveFactoryScripts(r.factoryDirectory, args),
		Env:                      mergedEnvironment(request.ProcessEnvironment, request.EnvVars),
		WorkDir:                  workDir,
		DispatchID:               dispatch.DispatchID,
		TransitionID:             dispatch.TransitionID,
		WorkerType:               firstNonEmpty(request.WorkerType, dispatch.WorkerType),
		WorkstationName:          firstNonEmpty(request.WorkstationType, dispatch.WorkstationName),
		ProjectID:                firstNonEmpty(request.ProjectID, dispatch.ProjectID),
		CurrentChainingTraceID:   dispatch.CurrentChainingTraceID,
		PreviousChainingTraceIDs: append([]string(nil), dispatch.PreviousChainingTraceIDs...),
		Execution:                dispatch.Execution,
		InputTokens:              workers.InputTokens(tokens...),
		InputBindings:            cloneStringSliceMap(dispatch.InputBindings),
	}, nil
}

func snapshotRequest(
	request workers.RunnerExecutionRequest,
) (workers.RunnerExecutionRequest, []workers.Token, error) {
	request = workers.CloneProviderInferenceRequest(request)
	tokens, err := decodeTokens(request.InputTokens)
	if err != nil {
		return workers.RunnerExecutionRequest{}, nil, err
	}
	request.InputTokens = workers.InputTokens(tokens...)
	request.Dispatch.InputTokens, err = cloneAnyValues(request.Dispatch.InputTokens)
	if err != nil {
		return workers.RunnerExecutionRequest{}, nil, err
	}
	return request, tokens, nil
}

func decodeTokens(values []any) ([]workers.Token, error) {
	if len(values) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("encode input tokens: %w", err)
	}
	var tokens []workers.Token
	if err := json.Unmarshal(encoded, &tokens); err != nil {
		return nil, fmt.Errorf("decode input tokens: %w", err)
	}
	return tokens, nil
}

func validateRequest(request workers.RunnerExecutionRequest, tokens []workers.Token) error {
	if request.RunnerID != Identity {
		return badRequest(fmt.Sprintf("script runner identity must be %q", Identity), nil)
	}
	for _, capability := range request.RequiredOptionalCapabilities {
		switch capability {
		case workers.RunnerOptionalCapabilityWorkingDirectory,
			workers.RunnerOptionalCapabilityWorktree:
		default:
			return &workers.UnsupportedRunnerCapabilityError{
				RunnerID:   Identity,
				Capability: capability,
			}
		}
	}
	for tokenIndex, token := range tokens {
		for contentIndex, content := range token.Color.Content {
			if content.Type.Normalized() == work.WorkContentPartTypeImage {
				return badRequest(
					fmt.Sprintf(
						"input_tokens[%d].color.content[%d]: image content is not supported by script runner",
						tokenIndex,
						contentIndex,
					),
					nil,
				)
			}
		}
	}
	return nil
}

func resolveArgs(args []string, data prompting.PromptData) ([]string, error) {
	resolved := make([]string, len(args))
	for index, arg := range args {
		if !strings.Contains(arg, "{{") {
			resolved[index] = arg
			continue
		}
		tmpl, err := template.New("script_argument").Option("missingkey=zero").Parse(arg)
		if err != nil {
			return nil, fmt.Errorf("argument %d: %w", index, err)
		}
		var output bytes.Buffer
		if err := tmpl.Execute(&output, data); err != nil {
			return nil, fmt.Errorf("argument %d: %w", index, err)
		}
		resolved[index] = output.String()
	}
	return resolved, nil
}

func effectiveWorkDir(request workers.RunnerExecutionRequest) string {
	if request.WorkingDirectory != "" {
		return request.WorkingDirectory
	}
	return request.Worktree
}

func mergedEnvironment(base []string, overrides map[string]string) []string {
	return platformprocess.MergeCommandEnv(
		base,
		platformprocess.CommandEnvEntriesFromMap(overrides),
	)
}

func resolveFactoryScripts(factoryDirectory string, values []string) []string {
	resolved := make([]string, len(values))
	for index, value := range values {
		resolved[index] = resolveFactoryScript(factoryDirectory, value)
	}
	return resolved
}

func resolveFactoryScript(factoryDirectory, value string) string {
	if factoryDirectory == "" {
		return value
	}
	normalized := filepath.ToSlash(strings.TrimSpace(value))
	relative, ok := strings.CutPrefix(normalized, "scripts/")
	if !ok {
		relative, ok = strings.CutPrefix(normalized, "factory/scripts/")
	}
	if !ok || relative == "" {
		return value
	}
	return filepath.Join(factoryDirectory, "scripts", filepath.FromSlash(relative))
}

func firstNonEmpty(primary, fallback string) string {
	if primary != "" {
		return primary
	}
	return fallback
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func cloneStringSliceMap(values map[string][]string) map[string][]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[string][]string, len(values))
	for key, value := range values {
		cloned[key] = append([]string(nil), value...)
	}
	return cloned
}

func cloneAnyValues(values []any) ([]any, error) {
	if len(values) == 0 {
		return nil, nil
	}
	encoded, err := json.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("encode dispatch input tokens: %w", err)
	}
	var cloned []any
	if err := json.Unmarshal(encoded, &cloned); err != nil {
		return nil, fmt.Errorf("decode dispatch input tokens: %w", err)
	}
	return cloned, nil
}

func badRequest(message string, cause error) error {
	return workers.NewProviderError(
		workers.WorkFailureTypePermanentBadRequest,
		message,
		cause,
	)
}

func misconfigured(message string, cause error) error {
	return workers.NewProviderError(
		workers.WorkFailureTypeMisconfigured,
		message,
		cause,
	)
}
