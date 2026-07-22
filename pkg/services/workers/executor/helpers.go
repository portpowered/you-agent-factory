package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerprocess "github.com/portpowered/infinite-you/pkg/services/workers/process"
	workerprompting "github.com/portpowered/infinite-you/pkg/services/workers/prompting"
	workerprovider "github.com/portpowered/infinite-you/pkg/services/workers/provider"
)

type CommandRunner = workerprocess.CommandRunner
type CommandRequest = workerprocess.CommandRequest
type CommandResult = workerprocess.CommandResult
type ExecCommandRunner = workerprocess.ExecCommandRunner
type LoggingCommandRunner = workerprocess.LoggingCommandRunner

type ProviderError = workerprovider.ProviderError

const (
	providerSessionKindSessionID       = "session_id"
	codexWindowsProcessFailureExitCode = 4294967295

	omitZeroWorkerEventExitCode    = false
	includeZeroWorkerEventExitCode = true

	redactedCommandEnvValue     = "<redacted>"
	metadataOnlyCommandEnvValue = "<metadata-only>"
)

type DefaultPromptRenderer = workerprompting.DefaultPromptRenderer

const (
	RedactedCommandEnvValue     = redactedCommandEnvValue
	MetadataOnlyCommandEnvValue = metadataOnlyCommandEnvValue
)

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

func CommandRequestInputTokens(request CommandRequest) []workerexecution.Token {
	return cloneInputTokens(request.InputTokens)
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

type commandEnvDiagnosticProjection struct {
	Count  int
	Keys   []string
	Values map[string]string
}

func projectCommandEnvForDiagnostics(env []string) commandEnvDiagnosticProjection {
	projection := commandEnvDiagnosticProjection{}
	if len(env) == 0 {
		return projection
	}

	seenKeys := make(map[string]struct{}, len(env))
	values := make(map[string]string, len(env))
	for _, entry := range env {
		name, value, ok := strings.Cut(entry, "=")
		if !ok || name == "" {
			continue
		}
		projection.Count++
		seenKeys[name] = struct{}{}
		switch classifyCommandEnvKey(name) {
		case "safe":
			values[name] = value
		case "redacted":
			values[name] = redactedCommandEnvValue
		default:
			values[name] = metadataOnlyCommandEnvValue
		}
	}

	if len(seenKeys) > 0 {
		projection.Keys = make([]string, 0, len(seenKeys))
		for name := range seenKeys {
			projection.Keys = append(projection.Keys, name)
		}
		sort.Strings(projection.Keys)
	}
	if len(values) > 0 {
		projection.Values = values
	}
	return projection
}

var safeCommandEnvKeys = map[string]struct{}{
	"CI":                  {},
	"CGO_ENABLED":         {},
	"EDITOR":              {},
	"FORCE_COLOR":         {},
	"GIT_EDITOR":          {},
	"GIT_MERGE_AUTOEDIT":  {},
	"GIT_SEQUENCE_EDITOR": {},
	"GIT_TERMINAL_PROMPT": {},
	"GOARCH":              {},
	"GOOS":                {},
	"NO_COLOR":            {},
	"OS":                  {},
	"RUNNER_OS":           {},
	"TERM":                {},
	"VISUAL":              {},
}

var sensitiveCommandEnvNameFragments = []string{
	"TOKEN",
	"SECRET",
	"PASSWORD",
	"PASS",
	"KEY",
	"CREDENTIAL",
	"CREDENTIALS",
	"AUTH",
	"ANTHROPIC",
	"OPENAI",
	"GEMINI",
	"GOOGLE_APPLICATION_CREDENTIALS",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
}

func classifyCommandEnvKey(name string) string {
	normalized := strings.ToUpper(strings.TrimSpace(name))
	if normalized == "" {
		return "metadata_only"
	}
	for _, fragment := range sensitiveCommandEnvNameFragments {
		if strings.Contains(normalized, fragment) {
			return "redacted"
		}
	}
	if _, ok := safeCommandEnvKeys[normalized]; ok {
		return "safe"
	}
	return "metadata_only"
}

func commandEnvDiagnosticMetadata(projection commandEnvDiagnosticProjection) map[string]string {
	if projection.Count == 0 && len(projection.Keys) == 0 {
		return nil
	}
	return map[string]string{
		"env_count": fmt.Sprintf("%d", projection.Count),
		"env_keys":  strings.Join(projection.Keys, ","),
	}
}

func workerEventExitCode(exitCode int, present bool, includeZero bool) *int {
	if !present {
		return nil
	}
	if exitCode == 0 && !includeZero {
		return nil
	}
	exitCodeCopy := exitCode
	return &exitCodeCopy
}

func firstImageContentPart(rawTokens []any) (int, int, work.WorkContentPart, bool) {
	for tokenIndex, token := range cloneInputTokens(rawTokens) {
		for partIndex, part := range token.Color.Content {
			if part.Type == work.WorkContentPartTypeImage {
				return tokenIndex, partIndex, part, true
			}
		}
	}
	return 0, 0, work.WorkContentPart{}, false
}

func unsupportedImageContentError(rawTokens []any, executionPath string) error {
	tokenIndex, partIndex, part, ok := firstImageContentPart(rawTokens)
	if !ok {
		return nil
	}
	if part.File == "" {
		return fmt.Errorf("input_tokens[%d].color.content[%d]: image content is not supported by %s; configure modelProvider codex for image-capable execution", tokenIndex, partIndex, executionPath)
	}
	return fmt.Errorf("input_tokens[%d].color.content[%d].file: image content %q is not supported by %s; configure modelProvider codex for image-capable execution", tokenIndex, partIndex, part.File, executionPath)
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
