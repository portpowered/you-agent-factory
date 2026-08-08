package executor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	runnerinference "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/inference"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/process"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/prompting"
	"github.com/santhosh-tekuri/jsonschema/v6"
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

func parseOutputAgainstSchema(content string, schemaPayload []byte) (any, error) {
	if strings.TrimSpace(content) == "" {
		return nil, fmt.Errorf("response is empty")
	}
	if len(bytes.TrimSpace(schemaPayload)) == 0 {
		return nil, fmt.Errorf("output schema is empty")
	}

	schemaDocument, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaPayload))
	if err != nil {
		return nil, fmt.Errorf("output schema is malformed: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	const schemaID = "worker-output-schema.json"
	if err := compiler.AddResource(schemaID, schemaDocument); err != nil {
		return nil, fmt.Errorf("output schema is invalid: %w", err)
	}
	compiled, err := compiler.Compile(schemaID)
	if err != nil {
		return nil, fmt.Errorf("output schema is invalid: %w", err)
	}

	document, err := jsonschema.UnmarshalJSON(bytes.NewReader([]byte(content)))
	if err != nil {
		return nil, fmt.Errorf("response is not valid JSON: %w", err)
	}
	if err := compiled.Validate(document); err != nil {
		return nil, fmt.Errorf("response does not satisfy output schema: %w", err)
	}
	return document, nil
}

func structuredOutputFailure(content string) string {
	var object map[string]any
	if err := json.Unmarshal([]byte(content), &object); err != nil || object == nil {
		return ""
	}

	for _, key := range []string{"verdict", "status", "outcome"} {
		value, ok := object[key].(string)
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(value)) {
		case "fail", "failed", "failure", "error", "reroll", "reject", "rejected":
			return fmt.Sprintf("%s=%s", key, strings.TrimSpace(value))
		}
	}
	for _, key := range []string{"action_completed", "success", "completed"} {
		value, ok := object[key].(bool)
		if ok && !value {
			return fmt.Sprintf("%s=false", key)
		}
	}
	return ""
}

func structuredOutputFailureMetadata() *workerexecution.WorkFailureMetadata {
	return &workerexecution.WorkFailureMetadata{
		Family: workerexecution.WorkFailureFamilyTerminal,
		Type:   workerexecution.WorkFailureTypePermanentBadRequest,
	}
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
