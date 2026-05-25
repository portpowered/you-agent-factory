package executor

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	workerprompting "github.com/portpowered/infinite-you/pkg/workers/prompting"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
)

type CommandRunner = workerprocess.CommandRunner
type CommandRequest = workerprocess.CommandRequest
type CommandResult = workerprocess.CommandResult
type ExecCommandRunner = workerprocess.ExecCommandRunner
type LoggingCommandRunner = workerprocess.LoggingCommandRunner

type Provider = workerprovider.Provider
type ProviderError = workerprovider.ProviderError

const (
	ModelProviderClaude   = workerprovider.ModelProviderClaude
	ModelProviderCodex    = workerprovider.ModelProviderCodex
	ModelProviderGemini   = workerprovider.ModelProviderGemini
	ModelProviderKiro     = workerprovider.ModelProviderKiro
	ModelProviderCursor   = workerprovider.ModelProviderCursor
	ModelProviderOpenCode = workerprovider.ModelProviderOpenCode

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

func cloneInputTokens(rawTokens []any) []interfaces.Token {
	if len(rawTokens) == 0 {
		return nil
	}

	out := make([]interfaces.Token, 0, len(rawTokens))
	for _, raw := range rawTokens {
		token, ok := decodeToken(raw)
		if !ok {
			continue
		}
		out = append(out, token)
	}
	return out
}

func clonePetriInputTokens(inputTokens []interfaces.Token) []any {
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

func decodeToken(raw any) (interfaces.Token, bool) {
	if token, ok := raw.(interfaces.Token); ok {
		return token, true
	}

	encoded, err := json.Marshal(raw)
	if err != nil {
		return interfaces.Token{}, false
	}
	var token interfaces.Token
	if err := json.Unmarshal(encoded, &token); err != nil {
		return interfaces.Token{}, false
	}
	return token, true
}

func InputTokens(tokens ...interfaces.Token) []any {
	return clonePetriInputTokens(tokens)
}

func WorkDispatchInputTokens(dispatch interfaces.WorkDispatch) []interfaces.Token {
	return cloneInputTokens(dispatch.InputTokens)
}

func CommandRequestInputTokens(request CommandRequest) []interfaces.Token {
	return cloneInputTokens(request.InputTokens)
}

func workDispatchNonResourceTokensForWorkstation(dispatch interfaces.WorkDispatch, workstationDef *interfaces.FactoryWorkstationConfig) []interfaces.Token {
	var tokens []interfaces.Token
	for _, token := range orderedWorkDispatchTokensForWorkstation(dispatch, workstationDef) {
		if token.Color.DataType != interfaces.DataTypeResource {
			tokens = append(tokens, token)
		}
	}
	return tokens
}

func orderedWorkDispatchTokensForWorkstation(dispatch interfaces.WorkDispatch, workstationDef *interfaces.FactoryWorkstationConfig) []interfaces.Token {
	tokens := WorkDispatchInputTokens(dispatch)
	if workstationDef == nil || len(tokens) < 2 {
		return tokens
	}

	byPlace := make(map[string][]int)
	for i, token := range tokens {
		byPlace[token.PlaceID] = append(byPlace[token.PlaceID], i)
	}

	ordered := make([]interfaces.Token, 0, len(tokens))
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

func stringPtrIfNotEmpty(value string) *string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func stringSlicePtr(values ...string) *[]string {
	if len(values) == 0 {
		return nil
	}
	cloned := append([]string(nil), values...)
	return &cloned
}
