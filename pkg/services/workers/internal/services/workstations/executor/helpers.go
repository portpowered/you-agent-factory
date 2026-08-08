package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	runnerinference "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/inference"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/prompting"
)

type CommandRunner = workerexecution.CommandRunner
type CommandRequest = workerexecution.CommandRequest
type CommandResult = workerexecution.CommandResult
type ExecCommandRunner = workerprocess.ExecCommandRunner
type LoggingCommandRunner = workerprocess.LoggingCommandRunner

type ProviderError = workerexecution.ProviderError

const (
	providerSessionKindSessionID       = "session_id"
	codexWindowsProcessFailureExitCode = 4294967295
)

type DefaultPromptRenderer = workerprompting.DefaultPromptRenderer

func cloneInputTokens(rawTokens []any) []workerexecution.Token {
	if len(rawTokens) == 0 {
		return nil
	}

	out := make([]workerexecution.Token, 0, len(rawTokens))
	for _, raw := range rawTokens {
		token, ok := decodeToken(raw)
		if !ok {
			continue
		}
		out = append(out, token)
	}
	return out
}

func clonePetriInputTokens(inputTokens []workerexecution.Token) []any {
	if len(inputTokens) == 0 {
		return nil
	}

	out := make([]any, 0, len(inputTokens))
	for _, token := range inputTokens {
		out = append(out, token)
	}
	return out
}

func cloneRawInputTokens(inputTokens []any) []any {
	if len(inputTokens) == 0 {
		return nil
	}
	return append([]any(nil), inputTokens...)
}

func decodeToken(raw any) (workerexecution.Token, bool) {
	if token, ok := raw.(workerexecution.Token); ok {
		return token, true
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return workerexecution.Token{}, false
	}
	var token workerexecution.Token
	if err := json.Unmarshal(encoded, &token); err != nil {
		return workerexecution.Token{}, false
	}
	return token, true
}

func InputTokens(tokens ...workerexecution.Token) []any {
	return clonePetriInputTokens(tokens)
}

func WorkDispatchInputTokens(dispatch work.WorkDispatch) []workerexecution.Token {
	return cloneInputTokens(dispatch.InputTokens)
}

func workDispatchNonResourceTokensForWorkstation(dispatch work.WorkDispatch, workstationDef *interfaces.FactoryWorkstationConfig) []workerexecution.Token {
	var tokens []workerexecution.Token
	for _, token := range orderedWorkDispatchTokensForWorkstation(dispatch, workstationDef) {
		if token.Color.DataType != workerexecution.DataTypeResource {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func orderedWorkDispatchTokensForWorkstation(dispatch work.WorkDispatch, workstationDef *interfaces.FactoryWorkstationConfig) []workerexecution.Token {
	tokens := WorkDispatchInputTokens(dispatch)
	if workstationDef == nil || len(tokens) < 2 {
		return tokens
	}

	byPlace := make(map[string][]int)
	for i, token := range tokens {
		byPlace[token.PlaceID] = append(byPlace[token.PlaceID], i)
	}

	ordered := make([]workerexecution.Token, 0, len(tokens))
	used := make([]bool, len(tokens))
	appendPlaceTokens := func(placeID string) {
		for _, index := range byPlace[placeID] {
			used[index] = true
			ordered = append(ordered, tokens[index])
		}
	}

	for _, input := range workstationDef.Inputs {
		appendPlaceTokens(fmt.Sprintf("%s:%s", input.WorkTypeName, input.StateName))
	}
	for _, resource := range workstationDef.Resources {
		appendPlaceTokens(fmt.Sprintf("%s:%s", resource.Name, interfaces.ResourceStateAvailable))
	}
	for i, token := range tokens {
		if used[i] {
			continue
		}
		ordered = append(ordered, token)
	}

	return ordered
}

func cloneEnvVars(envVars map[string]string) map[string]string {
	if len(envVars) == 0 {
		return nil
	}
	clone := make(map[string]string, len(envVars))
	for key, value := range envVars {
		clone[key] = value
	}
	return clone
}

const (
	WorkLogEventCommandRunnerRequested      = "command_runner.requested"
	WorkLogEventCommandRunnerCompleted      = "command_runner.completed"
	WorkLogEventCommandRunnerRequestDetails = "command_runner.request_details"
	WorkLogEventCommandRunnerOutputDetails  = "command_runner.output_details"
)

// WorkLogFields returns stable structured log fields for work-scoped runtime
// records. Empty strings are intentional so unavailable IDs remain explicit.
func WorkLogFields(metadata work.ExecutionMetadata, keysAndValues ...any) []any {
	fields := []any{
		"request_id", metadata.RequestID,
		"trace_id", metadata.TraceID,
		"work_id", primaryWorkID(metadata.WorkIDs),
		"work_ids", cloneWorkIDs(metadata.WorkIDs),
	}
	return append(fields, keysAndValues...)
}

func primaryWorkID(workIDs []string) string {
	for _, workID := range workIDs {
		if workID != "" {
			return workID
		}
	}
	return ""
}

func cloneWorkIDs(workIDs []string) []string {
	if workIDs == nil {
		return []string{}
	}
	return append([]string(nil), workIDs...)
}

// NoopExecutor is a WorkerExecutor that always returns OutcomeAccepted
// without calling any LLM or script. It is used as a fallback when no
// AGENTS.md is configured for a worker, allowing tests to exercise the
// petri-net topology without providing real worker configuration.
//
// Hosted/poller Worker shapes must not use this type: Automations owns those
// ingress sources and they are omitted from Workers executor construction.
type NoopExecutor struct{}

// Execute implements WorkerExecutor. It propagates the first input token's
// color and returns OutcomeAccepted immediately.
func (n *NoopExecutor) Execute(_ context.Context, d work.WorkDispatch) (workerexecution.WorkResult, error) {
	return workerexecution.WorkResult{
		DispatchID:   d.DispatchID,
		TransitionID: d.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
	}, nil
}

// Compile-time check.
var _ WorkerExecutor = (*NoopExecutor)(nil)

func resolveModelOperationBindings(
	workstationDef *interfaces.FactoryWorkstationConfig,
	workerDef *interfaces.FactoryWorkerConfig,
	inputTokens []workerexecution.Token,
) ([]workerexecution.ResolvedModelOperationBinding, error) {
	return runnerinference.ResolveInferenceOperationBindings(workstationDef, workerDef, inputTokens)
}

func (we *WorkstationExecutor) promptFileReader() interfaces.FileReader {
	if we.FileSystem == nil {
		return nil
	}
	return we.FileSystem.ReadFile
}

func (we *WorkstationExecutor) prepareWorkstationDefinition(
	dispatch work.WorkDispatch,
	workstationName string,
	workstationDef *interfaces.FactoryWorkstationConfig,
	invocationArgs *work.InvocationArguments,
	readFile interfaces.FileReader,
	diagnostics *workerexecution.WorkDiagnostics,
	start time.Time,
) (*interfaces.FactoryWorkstationConfig, *workerexecution.WorkResult) {
	snapshot, promptPath, err := we.workstationPromptSnapshot(workstationName, workstationDef)
	if err != nil {
		result := promptSourceFailureResult(
			dispatch,
			"workstation",
			workstationName,
			promptPath,
			err,
			diagnostics,
			we.Now().Sub(start),
		)
		return nil, &result
	}
	if we.Interpolation == nil {
		return snapshot, nil
	}
	interpolated, err := we.Interpolation.InterpolateWorkstationConfig(*snapshot, invocationArgs, readFile)
	if err != nil {
		return nil, promptPreparationFailureResult(dispatch, err, diagnostics, we.Now().Sub(start))
	}
	return &interpolated, nil
}

func (we *WorkstationExecutor) prepareWorkerDefinition(
	dispatch work.WorkDispatch,
	workerName string,
	workerDef *interfaces.FactoryWorkerConfig,
	invocationArgs *work.InvocationArguments,
	readFile interfaces.FileReader,
	diagnostics *workerexecution.WorkDiagnostics,
	start time.Time,
) (*interfaces.FactoryWorkerConfig, *workerexecution.WorkResult) {
	snapshot, promptPath, err := we.workerPromptSnapshot(workerName, workerDef)
	if err != nil {
		result := promptSourceFailureResult(
			dispatch,
			"worker",
			workerName,
			promptPath,
			err,
			diagnostics,
			we.Now().Sub(start),
		)
		return nil, &result
	}
	if we.Interpolation == nil {
		return snapshot, nil
	}
	interpolated, err := we.Interpolation.InterpolateWorkerConfig(*snapshot, invocationArgs, readFile)
	if err != nil {
		return nil, promptPreparationFailureResult(dispatch, err, diagnostics, we.Now().Sub(start))
	}
	if strings.TrimSpace(interpolated.ModelProvider) == "" {
		interpolated.ModelProvider = interpolated.RuntimeDefaultModelProvider
	}
	if strings.TrimSpace(interpolated.Model) == "" {
		interpolated.Model = interpolated.RuntimeDefaultModel
	}
	if failed := we.resolveInvocationProvider(dispatch, &interpolated, diagnostics, start); failed != nil {
		return nil, failed
	}
	return &interpolated, nil
}

func promptPreparationFailureResult(
	dispatch work.WorkDispatch,
	err error,
	diagnostics *workerexecution.WorkDiagnostics,
	duration time.Duration,
) *workerexecution.WorkResult {
	return &workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error:        err.Error(),
		Diagnostics:  diagnostics,
		Metrics:      workerexecution.WorkMetrics{Duration: duration},
	}
}

func (we *WorkstationExecutor) workerPromptSnapshot(
	workerName string,
	workerDef *interfaces.FactoryWorkerConfig,
) (*interfaces.FactoryWorkerConfig, string, error) {
	snapshot := interfaces.CloneWorkerConfig(*workerDef)
	source := we.workerPromptSource(workerName, workerDef)
	snapshot.PromptSourcePath = source.Path
	if err := we.refreshWorkerPrompt(&snapshot); err != nil {
		return nil, snapshot.PromptSourcePath, err
	}
	return &snapshot, snapshot.PromptSourcePath, nil
}

func (we *WorkstationExecutor) refreshWorkerPrompt(
	workerDef *interfaces.FactoryWorkerConfig,
) error {
	if workerDef == nil || workerDef.PromptSourcePath == "" {
		return nil
	}
	body, err := workerprompting.ResolveAuthoredPromptSource(
		we.FileSystem,
		workerDef.PromptSourcePath,
		true,
	)
	if err != nil {
		return err
	}
	workerDef.Body = body
	return nil
}

func (we *WorkstationExecutor) workerPromptSource(
	workerName string,
	workerDef *interfaces.FactoryWorkerConfig,
) interfaces.PromptSource {
	if lookup, ok := we.RuntimeConfig.(interfaces.RuntimePromptSourceLookup); ok {
		if source, found := lookup.WorkerPromptSource(workerName); found {
			return source
		}
	}
	if workerDef == nil {
		return interfaces.PromptSource{}
	}
	return interfaces.PromptSource{Path: workerDef.PromptSourcePath}
}

func (we *WorkstationExecutor) workstationPromptSnapshot(
	workstationName string,
	workstationDef *interfaces.FactoryWorkstationConfig,
) (*interfaces.FactoryWorkstationConfig, string, error) {
	snapshot := interfaces.CloneWorkstationConfig(*workstationDef)
	source := we.workstationPromptSource(workstationName, workstationDef)
	snapshot.PromptSourcePath = source.Path
	snapshot.PromptSourceIsTemplate = source.IsTemplate
	if err := we.refreshWorkstationPrompt(&snapshot); err != nil {
		return nil, snapshot.PromptSourcePath, err
	}
	return &snapshot, snapshot.PromptSourcePath, nil
}

func (we *WorkstationExecutor) refreshWorkstationPrompt(
	workstationDef *interfaces.FactoryWorkstationConfig,
) error {
	if workstationDef == nil || workstationDef.PromptSourcePath == "" {
		return nil
	}
	prompt, err := workerprompting.ResolveAuthoredPromptSource(
		we.FileSystem,
		workstationDef.PromptSourcePath,
		!workstationDef.PromptSourceIsTemplate,
	)
	if err != nil {
		return err
	}
	if workstationDef.PromptSourceIsTemplate {
		workstationDef.PromptTemplate = prompt
		return nil
	}
	workstationDef.Body = prompt
	workstationDef.PromptTemplate = prompt
	return nil
}

func (we *WorkstationExecutor) workstationPromptSource(
	workstationName string,
	workstationDef *interfaces.FactoryWorkstationConfig,
) interfaces.PromptSource {
	if lookup, ok := we.RuntimeConfig.(interfaces.RuntimePromptSourceLookup); ok {
		if source, found := lookup.WorkstationPromptSource(workstationName); found {
			return source
		}
	}
	if workstationDef == nil {
		return interfaces.PromptSource{}
	}
	return interfaces.PromptSource{
		Path:       workstationDef.PromptSourcePath,
		IsTemplate: workstationDef.PromptSourceIsTemplate,
	}
}

func promptSourceFailureResult(
	dispatch work.WorkDispatch,
	role string,
	name string,
	path string,
	err error,
	diagnostics *workerexecution.WorkDiagnostics,
	duration time.Duration,
) workerexecution.WorkResult {
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeFailed,
		Error: fmt.Sprintf(
			"%s %q prompt source %s: %v",
			role,
			name,
			path,
			err,
		),
		Diagnostics: diagnostics,
		Metrics: workerexecution.WorkMetrics{
			Duration: duration,
		},
	}
}
