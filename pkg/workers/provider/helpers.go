package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
)

type CommandRunner = workerprocess.CommandRunner
type CommandRequest = workerprocess.CommandRequest
type CommandResult = workerprocess.CommandResult
type ExecCommandRunner = workerprocess.ExecCommandRunner
type LoggingCommandRunner = workerprocess.LoggingCommandRunner

const (
	omitZeroWorkerEventExitCode = false
)

const (
	RedactedCommandEnvValue     = "<redacted>"
	MetadataOnlyCommandEnvValue = "<metadata-only>"
	ProviderDefaultModel        = "provider-default"
	ProviderInvocationPrepared  = "provider.invocation_prepared"
	ProviderFailureNormalized   = "provider.failure_normalized"
	RedactedProviderArgValue    = "<redacted>"
	RedactedProviderPrompt      = "<redacted:prompt>"
)

var providerFlagsWithSafeValues = map[string]struct{}{
	"--approval-mode": {}, "--model": {}, "--output-format": {}, "--sandbox": {},
}

var providerFlagsWithSensitiveValues = map[string]struct{}{
	"--agent": {}, "--cd": {}, "--dir": {}, "--image": {}, "--prompt": {},
	"--resume": {}, "--resume-id": {}, "--session": {}, "--system-prompt": {},
	"--workspace": {}, "--worktree": {},
}

func providerModelForLog(model string) string {
	if normalized := strings.TrimSpace(model); normalized != "" {
		return normalized
	}
	return ProviderDefaultModel
}

func sanitizeProviderArgs(provider string, args []string) []string {
	sanitized := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if providerInlineArgIsSensitive(arg) {
			sanitized = append(sanitized, strings.SplitN(arg, "=", 2)[0]+"="+RedactedProviderArgValue)
			continue
		}
		sanitized = append(sanitized, arg)
		if _, ok := providerFlagsWithSafeValues[arg]; ok && index+1 < len(args) {
			index++
			sanitized = append(sanitized, args[index])
			continue
		}
		if _, ok := providerFlagsWithSensitiveValues[arg]; ok && index+1 < len(args) {
			index++
			sanitized = append(sanitized, RedactedProviderArgValue)
			continue
		}
		if providerArgIsSensitivePositional(provider, args, index) {
			sanitized[len(sanitized)-1] = RedactedProviderPrompt
		}
	}
	return sanitized
}

func providerInlineArgIsSensitive(arg string) bool {
	name, _, ok := strings.Cut(arg, "=")
	if !ok {
		return false
	}
	normalized := strings.ToLower(name)
	return strings.Contains(normalized, "key") || strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") || strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "prompt") || strings.Contains(normalized, "credential")
}

func providerArgIsSensitivePositional(provider string, args []string, index int) bool {
	if strings.HasPrefix(args[index], "-") {
		return false
	}
	if index == 0 && (args[index] == "exec" || args[index] == "chat" || args[index] == "run") {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case string(interfaces.ModelProviderCodex):
		return args[index] != "-"
	default:
		return true
	}
}

func providerPreparedLogFields(ctx context.Context, req interfaces.ProviderInferenceRequest, execReq CommandRequest) []any {
	fields := workLogFields(req.Dispatch.Execution,
		"event_name", ProviderInvocationPrepared,
		"provider", strings.ToLower(strings.TrimSpace(req.ModelProvider)),
		"model", providerModelForLog(req.Model),
		"command", execReq.Command,
		"args", sanitizeProviderArgs(req.ModelProvider, execReq.Args),
		"working_dir", execReq.WorkDir,
		"stdin_bytes", len(execReq.Stdin),
		"stdin_sha256", sha256Hex(execReq.Stdin),
		"dispatch_id", req.Dispatch.DispatchID)
	if deadline, ok := ctx.Deadline(); ok {
		fields = append(fields, "deadline", deadline.UTC().Format(time.RFC3339Nano))
	}
	return fields
}

func providerFailureLogFields(
	req interfaces.ProviderInferenceRequest,
	providerErr *ProviderError,
	result CommandResult,
	duration time.Duration,
) []any {
	decision := WorkFailureDecisionFromProviderError(providerErr)
	fields := workLogFields(req.Dispatch.Execution,
		"event_name", ProviderFailureNormalized,
		"provider", strings.ToLower(strings.TrimSpace(req.ModelProvider)),
		"model", providerModelForLog(req.Model),
		"failure_reason", providerErr.Type,
		"failure_message", safeProviderFailureLogMessage(req.ModelProvider, providerErr),
		"retryable", decision.Retryable,
		"duration_ms", duration.Milliseconds(),
		"dispatch_id", req.Dispatch.DispatchID)
	if result.ExitCode != 0 {
		fields = append(fields, "exit_code", result.ExitCode)
	}
	return fields
}

func safeProviderFailureLogMessage(provider string, providerErr *ProviderError) string {
	// Codex exit failures are parsed into bounded, audited messages. Execution
	// errors retain raw command diagnostics in the returned error, so they must
	// use the same fixed reason-based messages as the other providers.
	if strings.EqualFold(strings.TrimSpace(provider), string(interfaces.ModelProviderCodex)) && providerErr.Cause == nil {
		return providerErr.Message
	}
	switch providerErr.Type {
	case interfaces.WorkFailureTypeAuthFailure:
		return "Provider authentication failed."
	case interfaces.WorkFailureTypePermanentBadRequest:
		return "Provider rejected the request as invalid."
	case interfaces.WorkFailureTypeThrottled:
		return "Provider is temporarily unavailable due to usage or capacity limits."
	case interfaces.WorkFailureTypeInternalServerError:
		return "Provider encountered a temporary server error."
	case interfaces.WorkFailureTypeTimeout:
		return "Provider request timed out."
	case interfaces.WorkFailureTypeMisconfigured:
		return "Provider command could not be started."
	default:
		return "Provider execution failed."
	}
}

// SafeProviderFailureDetail returns the allowlisted public diagnostic for a
// normalized provider failure. It intentionally excludes causes and raw
// provider output so durable projections can persist it safely.
func SafeProviderFailureDetail(providerErr *ProviderError) *interfaces.FailureDetail {
	if providerErr == nil {
		return nil
	}
	return &interfaces.FailureDetail{
		Reason:  providerErr.Type,
		Message: safeProviderFailureLogMessage("", providerErr),
	}
}

func sha256Hex(input []byte) string {
	digest := sha256.Sum256(input)
	return hex.EncodeToString(digest[:])
}

type CommandEnvClassification string

const (
	CommandEnvClassificationSafe         CommandEnvClassification = "safe"
	CommandEnvClassificationRedacted     CommandEnvClassification = "redacted"
	CommandEnvClassificationMetadataOnly CommandEnvClassification = "metadata_only"
)

type CommandEnvDiagnosticProjection struct {
	Count  int               `json:"count"`
	Keys   []string          `json:"keys,omitempty"`
	Values map[string]string `json:"values,omitempty"`
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

func workLogFields(metadata interfaces.ExecutionMetadata, keysAndValues ...any) []any {
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

func firstImageContentPart(rawTokens []any) (int, int, interfaces.WorkContentPart, bool) {
	for tokenIndex, token := range cloneInputTokens(rawTokens) {
		for partIndex, part := range token.Color.Content {
			if part.Type == interfaces.WorkContentPartTypeImage {
				return tokenIndex, partIndex, part, true
			}
		}
	}
	return 0, 0, interfaces.WorkContentPart{}, false
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

func ClassifyCommandEnvKey(name string) CommandEnvClassification {
	normalized := strings.ToUpper(strings.TrimSpace(name))
	if normalized == "" {
		return CommandEnvClassificationMetadataOnly
	}
	for _, fragment := range sensitiveCommandEnvNameFragments {
		if strings.Contains(normalized, fragment) {
			return CommandEnvClassificationRedacted
		}
	}
	if _, ok := safeCommandEnvKeys[normalized]; ok {
		return CommandEnvClassificationSafe
	}
	return CommandEnvClassificationMetadataOnly
}

func ProjectCommandEnvForDiagnostics(env []string) CommandEnvDiagnosticProjection {
	projection := CommandEnvDiagnosticProjection{}
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
		switch ClassifyCommandEnvKey(name) {
		case CommandEnvClassificationSafe:
			values[name] = value
		case CommandEnvClassificationRedacted:
			values[name] = RedactedCommandEnvValue
		default:
			values[name] = MetadataOnlyCommandEnvValue
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

func commandEnvDiagnosticMetadata(projection CommandEnvDiagnosticProjection) map[string]string {
	if projection.Count == 0 && len(projection.Keys) == 0 {
		return nil
	}
	return map[string]string{
		"env_count": fmt.Sprintf("%d", projection.Count),
		"env_keys":  strings.Join(projection.Keys, ","),
	}
}

func workDiagnosticsForInferenceRequest(req interfaces.ProviderInferenceRequest) *interfaces.WorkDiagnostics {
	requestMetadata := map[string]string{
		"worker_type":       firstNonEmpty(req.WorkerType, req.Dispatch.WorkerType),
		"workstation_type":  req.WorkstationType,
		"worktree":          req.Worktree,
		"working_directory": req.WorkingDirectory,
		"session_id":        req.SessionID,
		"output_schema":     req.OutputSchema,
	}
	if req.OpenCodeAgent != "" {
		requestMetadata["opencode_agent"] = req.OpenCodeAgent
	}
	return &interfaces.WorkDiagnostics{
		RenderedPrompt: &interfaces.RenderedPromptDiagnostic{
			SystemPromptHash: hashText(req.SystemPrompt),
			UserMessageHash:  hashText(req.UserMessage),
		},
		Provider: &interfaces.ProviderDiagnostic{
			Provider:        req.ModelProvider,
			Model:           req.Model,
			RequestMetadata: requestMetadata,
		},
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func withInferenceResponseDiagnostics(base *interfaces.WorkDiagnostics, resp interfaces.InferenceResponse, retryCount int) *interfaces.WorkDiagnostics {
	diagnostics := interfaces.CloneWorkDiagnostics(base)
	diagnostics = mergeWorkDiagnostics(diagnostics, resp.Diagnostics)
	if diagnostics == nil {
		diagnostics = &interfaces.WorkDiagnostics{}
	}
	if diagnostics.Provider == nil {
		diagnostics.Provider = &interfaces.ProviderDiagnostic{}
	}
	if diagnostics.Provider.ResponseMetadata == nil {
		diagnostics.Provider.ResponseMetadata = make(map[string]string)
	}
	diagnostics.Provider.ResponseMetadata["content_bytes"] = fmt.Sprintf("%d", len(resp.Content))
	diagnostics.Provider.ResponseMetadata["retry_count"] = fmt.Sprintf("%d", retryCount)
	if resp.ProviderSession != nil {
		diagnostics.Provider.ResponseMetadata["provider_session_provider"] = interfaces.CanonicalProviderSessionProvider(resp.ProviderSession.Provider)
		diagnostics.Provider.ResponseMetadata["provider_session_kind"] = resp.ProviderSession.Kind
		diagnostics.Provider.ResponseMetadata["provider_session_id"] = resp.ProviderSession.ID
	}
	return diagnostics
}

func withInferenceErrorDiagnostics(base *interfaces.WorkDiagnostics, err error, retryCount int) *interfaces.WorkDiagnostics {
	diagnostics := interfaces.CloneWorkDiagnostics(base)
	if diagnostics == nil {
		diagnostics = &interfaces.WorkDiagnostics{}
	}
	if diagnostics.Provider == nil {
		diagnostics.Provider = &interfaces.ProviderDiagnostic{}
	}
	if diagnostics.Provider.ResponseMetadata == nil {
		diagnostics.Provider.ResponseMetadata = make(map[string]string)
	}
	diagnostics.Provider.ResponseMetadata["error"] = err.Error()
	diagnostics.Provider.ResponseMetadata["retry_count"] = fmt.Sprintf("%d", retryCount)
	return diagnostics
}

func commandDiagnostics(req CommandRequest, result CommandResult, duration time.Duration, timedOut bool) *interfaces.WorkDiagnostics {
	envProjection := ProjectCommandEnvForDiagnostics(req.Env)
	return &interfaces.WorkDiagnostics{
		Command: &interfaces.CommandDiagnostic{
			Command:    req.Command,
			Args:       append([]string(nil), req.Args...),
			Stdin:      string(req.Stdin),
			Env:        envProjection.Values,
			Stdout:     string(result.Stdout),
			Stderr:     string(result.Stderr),
			ExitCode:   result.ExitCode,
			TimedOut:   timedOut,
			Duration:   duration,
			WorkingDir: req.WorkDir,
		},
		Metadata: commandEnvDiagnosticMetadata(envProjection),
	}
}

func mergeWorkDiagnostics(base, overlay *interfaces.WorkDiagnostics) *interfaces.WorkDiagnostics {
	if base == nil {
		return interfaces.CloneWorkDiagnostics(overlay)
	}
	if overlay == nil {
		return base
	}
	overlay = interfaces.CloneWorkDiagnostics(overlay)
	if overlay.RenderedPrompt != nil {
		base.RenderedPrompt = overlay.RenderedPrompt
	}
	if overlay.Provider != nil {
		base.Provider = mergeProviderDiagnostic(base.Provider, overlay.Provider)
	}
	if overlay.Invocation != nil {
		base.Invocation = interfaces.CloneWorkDiagnostics(&interfaces.WorkDiagnostics{Invocation: overlay.Invocation}).Invocation
	}
	if overlay.Command != nil {
		base.Command = overlay.Command
	}
	if overlay.Panic != nil {
		base.Panic = overlay.Panic
	}
	if len(overlay.Metadata) > 0 {
		if base.Metadata == nil {
			base.Metadata = make(map[string]string, len(overlay.Metadata))
		}
		for k, v := range overlay.Metadata {
			base.Metadata[k] = v
		}
	}
	return base
}

func mergeProviderDiagnostic(base, overlay *interfaces.ProviderDiagnostic) *interfaces.ProviderDiagnostic {
	if base == nil {
		return overlay
	}
	if overlay.Provider != "" {
		base.Provider = overlay.Provider
	}
	if overlay.Model != "" {
		base.Model = overlay.Model
	}
	if len(overlay.RequestMetadata) > 0 {
		if base.RequestMetadata == nil {
			base.RequestMetadata = make(map[string]string, len(overlay.RequestMetadata))
		}
		for k, v := range overlay.RequestMetadata {
			base.RequestMetadata[k] = v
		}
	}
	if len(overlay.ResponseMetadata) > 0 {
		if base.ResponseMetadata == nil {
			base.ResponseMetadata = make(map[string]string, len(overlay.ResponseMetadata))
		}
		for k, v := range overlay.ResponseMetadata {
			base.ResponseMetadata[k] = v
		}
	}
	return base
}

func hashText(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func looksLikeStructuredCodexPayload(raw string, fallbackEventType string) bool {
	if strings.TrimSpace(fallbackEventType) != "" {
		return true
	}
	trimmed := strings.TrimSpace(raw)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func malformedCodexStructuredEvent(raw string, fallbackEventType string, diagnosticClass string, hasher hash.Hash) InferenceProgressFragment {
	return InferenceProgressFragment{
		Kind:              ProgressFragmentKind,
		Type:              NormalizedEventTypeUnknown,
		Payload:           "codex event omitted",
		ExternalEventType: strings.TrimSpace(fallbackEventType),
		Metadata:          codexRawDiagnosticMetadata(raw, diagnosticClass, hasher),
	}
}

func annotateBoundedPayloadMetadata(metadata map[string]string, original string, bounded string) map[string]string {
	trimmedOriginal := strings.TrimSpace(original)
	if trimmedOriginal == "" {
		return metadata
	}
	annotated := cloneStringMap(metadata)
	if annotated == nil {
		annotated = map[string]string{}
	}
	annotated[codexMetadataTextBytesKey] = strconv.Itoa(len([]byte(trimmedOriginal)))
	if len([]byte(bounded)) < len([]byte(trimmedOriginal)) {
		annotated[codexMetadataTruncatedKey] = "true"
	}
	return annotated
}

func codexRawDiagnosticMetadata(raw string, diagnosticClass string, hasher hash.Hash) map[string]string {
	metadata := map[string]string{
		codexMetadataDiagnosticKey: strings.TrimSpace(diagnosticClass),
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		metadata[codexMetadataRawBytesKey] = strconv.Itoa(len([]byte(trimmed)))
		metadata[codexMetadataRawSHA256Key] = sha256Digest(trimmed, hasher)
	}
	return metadata
}

func sha256Digest(raw string, hasher hash.Hash) string {
	if hasher == nil {
		hasher = sha256.New()
	}
	hasher.Reset()
	_, _ = hasher.Write([]byte(raw))
	return fmt.Sprintf("%x", hasher.Sum(nil))
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

func lastSafeOpenCodeUnknownExcerpt(streams []string) (string, bool) {
	var last string
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			trimmed := strings.TrimSpace(line)
			if failure, ok := decodeOpenCodeStructuredFailure(trimmed); ok {
				if excerpt, safe := safeOpenCodeFailureDetail(failure.Message); safe {
					last = excerpt
				}
				continue
			}
			if !openCodeErrorTextLine(trimmed) {
				continue
			}
			if excerpt, safe := safeOpenCodeFailureDetail(trimmed); safe {
				last = excerpt
			}
		}
	}
	return last, last != ""
}

func lastOpenCodeStructuredFailure(streams []string) (ProviderFailureResult, bool) {
	var last ProviderFailureResult
	var found bool
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			failure, ok := decodeOpenCodeStructuredFailure(strings.TrimSpace(line))
			if !ok {
				continue
			}
			reason, recognized := classifyOpenCodeStructuredFailure(failure)
			if !recognized {
				continue
			}
			last = ProviderFailureResult{
				Reason:  reason,
				Message: openCodeFailureMessage(reason, failure.Message),
			}
			found = true
		}
	}
	return last, found
}

func decodeOpenCodeStructuredFailure(line string) (opencodeStructuredFailure, bool) {
	if !strings.HasPrefix(line, "{") {
		return opencodeStructuredFailure{}, false
	}
	var envelope struct {
		Type       string `json:"type"`
		Name       string `json:"name"`
		Code       string `json:"code"`
		Status     int    `json:"status"`
		StatusCode int    `json:"statusCode"`
		Message    string `json:"message"`
		Error      *struct {
			Type       string `json:"type"`
			Name       string `json:"name"`
			Code       string `json:"code"`
			Status     int    `json:"status"`
			StatusCode int    `json:"statusCode"`
			Message    string `json:"message"`
			Data       *struct {
				Code       string `json:"code"`
				Status     int    `json:"status"`
				StatusCode int    `json:"statusCode"`
				Message    string `json:"message"`
			} `json:"data"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(line), &envelope); err != nil {
		return opencodeStructuredFailure{}, false
	}

	failure := opencodeStructuredFailure{
		Name:       envelope.Name,
		Type:       envelope.Type,
		Code:       envelope.Code,
		StatusCode: firstNonZero(envelope.StatusCode, envelope.Status),
		Message:    envelope.Message,
	}
	if envelope.Error != nil {
		failure.Name = firstNonEmpty(envelope.Error.Name, failure.Name)
		failure.Type = firstNonEmpty(envelope.Error.Type, failure.Type)
		failure.Code = firstNonEmpty(envelope.Error.Code, failure.Code)
		failure.StatusCode = firstNonZero(envelope.Error.StatusCode, envelope.Error.Status, failure.StatusCode)
		failure.Message = firstNonEmpty(envelope.Error.Message, failure.Message)
		if envelope.Error.Data != nil {
			failure.Code = firstNonEmpty(envelope.Error.Data.Code, failure.Code)
			failure.StatusCode = firstNonZero(envelope.Error.Data.StatusCode, envelope.Error.Data.Status, failure.StatusCode)
			failure.Message = firstNonEmpty(envelope.Error.Data.Message, failure.Message)
		}
	}
	if envelope.Error == nil && !strings.EqualFold(envelope.Type, "error") {
		return opencodeStructuredFailure{}, false
	}
	return failure, true
}

func firstNonZero(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func classifyOpenCodeStructuredFailure(failure opencodeStructuredFailure) (interfaces.WorkFailureType, bool) {
	signal := strings.ToLower(strings.Join([]string{failure.Name, failure.Type, failure.Code}, " "))
	switch {
	case containsAny(signal, "providerautherror", "authentication_error", "permission_error", "unauthorized", "forbidden"):
		return interfaces.WorkFailureTypeAuthFailure, true
	case containsAny(signal, "invalid_request_error", "badrequesterror", "invalidrequesterror"):
		return interfaces.WorkFailureTypePermanentBadRequest, true
	case containsAny(signal, "ratelimiterror", "rate_limit_error", "overloaded_error", "quotaexceeded"):
		return interfaces.WorkFailureTypeThrottled, true
	case containsAny(signal, "timeouterror", "timeout_error", "etimedout"):
		return interfaces.WorkFailureTypeTimeout, true
	case containsAny(signal, "server_error", "internalservererror"):
		return interfaces.WorkFailureTypeInternalServerError, true
	}
	switch {
	case failure.StatusCode == 401 || failure.StatusCode == 403:
		return interfaces.WorkFailureTypeAuthFailure, true
	case failure.StatusCode == 400 || failure.StatusCode == 422:
		return interfaces.WorkFailureTypePermanentBadRequest, true
	case failure.StatusCode == 408:
		return interfaces.WorkFailureTypeTimeout, true
	case failure.StatusCode == 429:
		return interfaces.WorkFailureTypeThrottled, true
	case failure.StatusCode >= 500 && failure.StatusCode <= 599:
		return interfaces.WorkFailureTypeInternalServerError, true
	default:
		return interfaces.WorkFailureTypeUnknown, false
	}
}

func lastOpenCodeTextFailure(streams []string, exitCode int) (ProviderFailureResult, bool) {
	var last ProviderFailureResult
	var found bool
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			trimmed := strings.TrimSpace(line)
			if !openCodeErrorTextLine(trimmed) {
				continue
			}
			if failure, ok := recognizedOpenCodeTextFailure(trimmed, exitCode); ok {
				last, found = failure, true
			}
		}
	}
	if found {
		return last, true
	}
	for _, stream := range streams {
		trimmed := strings.TrimSpace(stream)
		if trimmed == "" || strings.Contains(trimmed, "\n") {
			continue
		}
		if failure, ok := recognizedOpenCodeTextFailure(trimmed, exitCode); ok {
			last, found = failure, true
		}
	}
	return last, found
}

func openCodeErrorTextLine(line string) bool {
	normalized := strings.ToLower(line)
	return strings.HasPrefix(normalized, "error:") || strings.HasPrefix(normalized, "api error:")
}

func recognizedOpenCodeTextFailure(message string, exitCode int) (ProviderFailureResult, bool) {
	normalized := strings.ToLower(strings.TrimSpace(message))
	var reason interfaces.WorkFailureType
	switch {
	case exitCode == 124 || containsAny(normalized, "deadline exceeded", "request timed out", "timed out", "timeout"):
		reason = interfaces.WorkFailureTypeTimeout
	case containsAny(normalized, "authentication", "login required", "not authenticated", "unauthorized", "forbidden", "api key"):
		reason = interfaces.WorkFailureTypeAuthFailure
	case containsAny(normalized, "invalid request", "bad request", "invalid argument", "model not found"):
		reason = interfaces.WorkFailureTypePermanentBadRequest
	case containsAny(normalized, "rate limit", "too many requests", "usage limit", "at capacity", "status 429"):
		reason = interfaces.WorkFailureTypeThrottled
	case containsAny(normalized, "internal server error", "server error", "status 500", "status 502", "status 503", "status 504"):
		reason = interfaces.WorkFailureTypeInternalServerError
	default:
		return ProviderFailureResult{}, false
	}
	return ProviderFailureResult{Reason: reason, Message: openCodeFailureMessage(reason, message)}, true
}

func openCodeFailureMessage(reason interfaces.WorkFailureType, detail string) string {
	if reason == interfaces.WorkFailureTypeAuthFailure || reason == interfaces.WorkFailureTypePermanentBadRequest {
		if sanitized, ok := safeOpenCodeFailureDetail(detail); ok {
			return sanitized
		}
	}
	switch reason {
	case interfaces.WorkFailureTypeAuthFailure:
		return opencodeAuthFailureMessage
	case interfaces.WorkFailureTypePermanentBadRequest:
		return opencodeBadRequestFailureMessage
	case interfaces.WorkFailureTypeThrottled:
		return opencodeThrottleFailureMessage
	case interfaces.WorkFailureTypeTimeout:
		return opencodeTimeoutFailureMessage
	case interfaces.WorkFailureTypeInternalServerError:
		return opencodeServerFailureMessage
	default:
		return ""
	}
}

func safeOpenCodeFailureDetail(detail string) (string, bool) {
	detail = strings.ToValidUTF8(strings.Join(strings.Fields(detail), " "), "")
	normalized := strings.ToLower(detail)
	if detail == "" || containsAny(normalized,
		"authorization:", "bearer ", "api_key=", "api-key=", `"token":`, "secret=", "sk-", "prompt:", "transcript:",
	) {
		return "", false
	}
	if len(detail) <= opencodeFailureMessageBytes {
		return detail, true
	}
	end := opencodeFailureMessageBytes
	for end > 0 && detail[end]&0xc0 == 0x80 {
		end--
	}
	return detail[:end], true
}

func safeGeminiTextCandidate(line string) string {
	message := sanitizeGeminiMessage(line)
	if message == "" || strings.HasPrefix(message, "{") {
		return ""
	}
	normalized := strings.ToLower(message)
	if isRejectedGeminiMessage(normalized) || !isGeminiErrorSignal(normalized) {
		return ""
	}
	return message
}

func isGeminiErrorSignal(normalized string) bool {
	if strings.HasPrefix(normalized, "error:") ||
		strings.HasPrefix(normalized, "gemini error:") ||
		strings.HasPrefix(normalized, "fatal") ||
		strings.HasPrefix(normalized, "failed") ||
		strings.HasPrefix(normalized, "failure:") ||
		strings.HasPrefix(normalized, "cannot ") ||
		strings.HasPrefix(normalized, "could not ") {
		return true
	}
	return containsAny(normalized,
		"http 4", "http 5", "status 4", "status 5",
		"unauthenticated", "permission_denied", "resource_exhausted", "resource exhausted",
		"deadline_exceeded", "timed out", "timeout", "permission denied",
		"rate limit exceeded", "quota exceeded", "too many requests",
		"invalid request", "bad request", "service unavailable", "upstream unavailable")
}

func geminiFailureResult(reason interfaces.WorkFailureType, upstreamMessage string) ProviderFailureResult {
	message := geminiFixedFailureMessage(reason)
	if reason == interfaces.WorkFailureTypeAuthFailure || reason == interfaces.WorkFailureTypePermanentBadRequest {
		if safe := safeGeminiStructuredMessage(upstreamMessage); safe != "" {
			message = safe
		}
	}
	return ProviderFailureResult{Reason: reason, Message: message}
}

func geminiFixedFailureMessage(reason interfaces.WorkFailureType) string {
	switch reason {
	case interfaces.WorkFailureTypeAuthFailure:
		return geminiAuthFailureMessage
	case interfaces.WorkFailureTypePermanentBadRequest:
		return geminiBadRequestMessage
	case interfaces.WorkFailureTypeThrottled:
		return geminiThrottleFailureMessage
	case interfaces.WorkFailureTypeTimeout:
		return geminiTimeoutFailureMessage
	case interfaces.WorkFailureTypeInternalServerError:
		return geminiServerFailureMessage
	default:
		return ""
	}
}

func safeGeminiStructuredMessage(message string) string {
	message = sanitizeGeminiMessage(message)
	if message == "" {
		return ""
	}
	normalized := strings.ToLower(message)
	if isRejectedGeminiMessage(normalized) {
		return ""
	}
	return message
}

func sanitizeGeminiMessage(message string) string {
	message = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, message)
	message = strings.Join(strings.Fields(message), " ")
	runes := []rune(message)
	if len(runes) > geminiFailureMessageRunes {
		message = string(runes[:geminiFailureMessageRunes])
	}
	return message
}

func isRejectedGeminiMessage(normalized string) bool {
	if strings.HasPrefix(normalized, "at ") || strings.HasPrefix(normalized, "goroutine ") {
		return true
	}
	return containsAny(normalized,
		"authorization:", "basic ", "bearer ", "api_key=", "api-key=", "password=", "token=", "secret=", "sk-",
		"api key:", "credential=", "aiza", "ya29.", "-----begin private key",
		"customer prompt", "user prompt", "prompt:", "model response", "transcript:",
		"[debug]", "debug:", "[progress]", "progress:", "traceback", "stack trace",
		"error report", "report written", "cleanup", "cleaning up", "/tmp/", "/var/tmp/", ".gemini/tmp/")
}

func tailForGeminiFailureScan(output []byte) string {
	if len(output) <= geminiFailureScanBytes {
		return string(output)
	}
	return string(output[len(output)-geminiFailureScanBytes:])
}
