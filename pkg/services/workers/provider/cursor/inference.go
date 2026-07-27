package cursor

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

const (
	ResultTypeResult = "result"

	ResultSubtypeSuccess = "success"

	CommandOutputExcerptLimit    = 2048
	CommandOutputLogPreviewLimit = 200
	PublishedTextLimit           = 2048
	PublishedDiagnosticLimit     = 96

	ResponseMetadataStdoutExcerpt = "stdout_excerpt"
	ResponseMetadataStderrExcerpt = "stderr_excerpt"

	ResponseMetadataRequestID        = "request_id"
	ResponseMetadataDurationMS       = workerexecution.ProviderResponseMetadataDurationMS
	ResponseMetadataDurationAPIMS    = workerexecution.ProviderResponseMetadataDurationAPIMS
	ResponseMetadataInputTokens      = workerexecution.ProviderResponseMetadataInputTokens
	ResponseMetadataOutputTokens     = workerexecution.ProviderResponseMetadataOutputTokens
	ResponseMetadataCacheReadTokens  = "cache_read_tokens"
	ResponseMetadataCacheWriteTokens = "cache_write_tokens"

	ProviderSessionKindSessionID = "session_id"

	cursorSensitiveOutputMessage = "Cursor output was omitted because it contained sensitive details."
)

var (
	safeCursorProviderSessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	cursorWindowsAbsolutePathPattern   = regexp.MustCompile(`(?i)(?:^|[\s"'(])(?:[a-z]:[\\/]|\\\\)`)
)

type resultPayload struct {
	Type          string       `json:"type"`
	Subtype       string       `json:"subtype"`
	IsError       bool         `json:"is_error"`
	DurationMS    int64        `json:"duration_ms"`
	DurationAPIMS int64        `json:"duration_api_ms"`
	Result        string       `json:"result"`
	SessionID     string       `json:"session_id"`
	RequestID     string       `json:"request_id"`
	Usage         *resultUsage `json:"usage,omitempty"`
}

type resultUsage struct {
	InputTokens      *int `json:"inputTokens,omitempty"`
	OutputTokens     *int `json:"outputTokens,omitempty"`
	CacheReadTokens  *int `json:"cacheReadTokens,omitempty"`
	CacheWriteTokens *int `json:"cacheWriteTokens,omitempty"`
}

// InferenceResult is the parsed Cursor CLI success payload.
type InferenceResult struct {
	Content          string
	ProviderSession  *workerexecution.ProviderSessionMetadata
	ResponseMetadata map[string]string
}

// ParseFailure classifies Cursor JSON parse failures for the provider layer.
type ParseFailure struct {
	Type            workerexecution.WorkFailureType
	Message         string
	ProviderSession *workerexecution.ProviderSessionMetadata
	Cause           error
	canonicalResult *FailureResult
}

// CanonicalResult returns the terminal failure result already produced while
// parsing stdout, so provider boundaries do not classify the same record again.
func (f *ParseFailure) CanonicalResult() (FailureResult, bool) {
	if f == nil || f.canonicalResult == nil {
		return FailureResult{}, false
	}
	return *f.canonicalResult, true
}

func (f *ParseFailure) Error() string {
	if f == nil {
		return ""
	}
	return f.Message
}

// ParseInferenceResult parses Cursor success stdout from either terminal json
// or stream-json output.
func ParseInferenceResult(provider string, stdout []byte) (*InferenceResult, *ParseFailure) {
	return parseInferenceResult(provider, stdout, nil)
}

func parseInferenceResult(
	provider string,
	stdout []byte,
	requestedSession *workerexecution.ProviderSessionMetadata,
) (*InferenceResult, *ParseFailure) {
	trimmed := strings.TrimSpace(string(stdout))
	if trimmed == "" {
		return nil, resultParseFailureWithSession(
			provider, "cursor JSON output was empty", nil, requestedSession,
		)
	}

	var payload resultPayload
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		if strings.Contains(trimmed, "\n") {
			return parseInferenceStreamResult(provider, stdout, requestedSession)
		}
		return nil, resultParseFailureWithSession(
			provider, fmt.Sprintf("cursor JSON output was not valid JSON: %v", err), err, requestedSession,
		)
	}

	if payload.Type != ResultTypeResult {
		if strings.Contains(trimmed, "\n") {
			return parseInferenceStreamResult(provider, stdout, requestedSession)
		}
		return nil, resultParseFailureWithSession(
			provider,
			fmt.Sprintf("cursor JSON output had unexpected type %q, want %q", payload.Type, ResultTypeResult),
			nil,
			requestedSession,
		)
	}
	if payload.Subtype != ResultSubtypeSuccess {
		return nil, resultErrorSubtypeWithSession(provider, payload, requestedSession)
	}
	if payload.IsError {
		return nil, resultErrorSubtypeWithSession(provider, payload, requestedSession)
	}

	session := canonicalProviderSession(provider, payload.SessionID)
	if session == nil {
		session = cloneCursorProviderSession(requestedSession)
	}
	if session == nil {
		return nil, resultParseFailure(
			provider, "cursor JSON success result is missing or invalid session_id", nil,
		)
	}

	return &InferenceResult{
		Content:          safeCursorPublishedText(payload.Result),
		ProviderSession:  session,
		ResponseMetadata: responseMetadataFromPayload(payload),
	}, nil
}

func resultErrorSubtypeWithSession(
	provider string,
	payload resultPayload,
	requestedSession *workerexecution.ProviderSessionMetadata,
) *ParseFailure {
	failure := failureResultFromPayload(payload)
	if failure.ProviderSession == nil {
		failure.ProviderSession = cloneCursorProviderSession(requestedSession)
	}
	return &ParseFailure{
		Type:            failure.Reason,
		Message:         failure.Message,
		ProviderSession: failure.ProviderSession,
		canonicalResult: &failure,
	}
}

func resultParseFailure(provider, message string, cause error) *ParseFailure {
	return resultParseFailureWithSession(provider, message, cause, nil)
}

func resultParseFailureWithSession(
	provider, message string,
	cause error,
	session *workerexecution.ProviderSessionMetadata,
) *ParseFailure {
	_ = provider
	return &ParseFailure{
		Type:            workerexecution.WorkFailureTypeUnknown,
		Message:         message,
		ProviderSession: cloneCursorProviderSession(session),
		Cause:           cause,
	}
}

func canonicalProviderSession(provider, sessionID string) *workerexecution.ProviderSessionMetadata {
	normalized := strings.TrimSpace(sessionID)
	if normalized == "" || !safeCursorProviderSessionIDPattern.MatchString(normalized) {
		return nil
	}
	return &workerexecution.ProviderSessionMetadata{
		Provider: workerexecution.CanonicalProviderSessionProvider(provider),
		Kind:     ProviderSessionKindSessionID,
		ID:       normalized,
	}
}

func cloneCursorProviderSession(
	session *workerexecution.ProviderSessionMetadata,
) *workerexecution.ProviderSessionMetadata {
	if session == nil ||
		workerexecution.CanonicalProviderSessionProvider(session.Provider) != "cursor" ||
		strings.TrimSpace(session.Kind) != ProviderSessionKindSessionID {
		return nil
	}
	return canonicalProviderSession("cursor", session.ID)
}

func responseMetadataFromPayload(payload resultPayload) map[string]string {
	metadata := make(map[string]string)
	if requestID := strings.TrimSpace(payload.RequestID); requestID != "" {
		metadata[ResponseMetadataRequestID] = requestID
	}
	if payload.DurationMS > 0 {
		metadata[ResponseMetadataDurationMS] = strconv.FormatInt(payload.DurationMS, 10)
	}
	if payload.DurationAPIMS > 0 {
		metadata[ResponseMetadataDurationAPIMS] = strconv.FormatInt(payload.DurationAPIMS, 10)
	}
	if payload.Usage == nil {
		return metadata
	}
	if payload.Usage.InputTokens != nil {
		metadata[ResponseMetadataInputTokens] = strconv.Itoa(*payload.Usage.InputTokens)
	}
	if payload.Usage.OutputTokens != nil {
		metadata[ResponseMetadataOutputTokens] = strconv.Itoa(*payload.Usage.OutputTokens)
	}
	if payload.Usage.CacheReadTokens != nil {
		metadata[ResponseMetadataCacheReadTokens] = strconv.Itoa(*payload.Usage.CacheReadTokens)
	}
	if payload.Usage.CacheWriteTokens != nil {
		metadata[ResponseMetadataCacheWriteTokens] = strconv.Itoa(*payload.Usage.CacheWriteTokens)
	}
	return metadata
}

// BoundedCommandOutputExcerpt returns a bounded excerpt of command output.
func BoundedCommandOutputExcerpt(output []byte, limit int) string {
	return boundedTrimmedText(string(output), limit)
}

// BoundedPublishedText trims and truncates provider-derived text before it is
// surfaced through internal progress or safe diagnostic channels.
func boundedTrimmedText(value string, limit int) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || limit <= 0 {
		return ""
	}
	if len(trimmed) <= limit {
		return trimmed
	}
	return trimmed[:limit] + "..."
}

func boundedText(value string, limit int) string {
	if value == "" || limit <= 0 {
		return ""
	}
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

// safeCursorPublishedText preserves ordinary assistant output while replacing
// provider text that carries credential, prompt, transcript, control-character,
// or machine-local path signals with stable customer-safe guidance.
func safeCursorPublishedText(value string) string {
	value = boundedText(value, PublishedTextLimit)
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, "\x00\r") || cursorPublishedTextIsSensitive(value) {
		return cursorSensitiveOutputMessage
	}
	return value
}

func cursorPublishedTextIsSensitive(value string) bool {
	normalized := strings.ToLower(value)
	markers := []string{
		"authorization:", "bearer ", "api_key=", "api_key:", "api-key=", "api-key:",
		"apikey=", "apikey:", "token=", "token:", "secret=", "secret:", "password=",
		"password:", "credential=", "credential:", "-----begin", "sk-", "ghp_", "aiza",
		"ya29.", "private prompt", "customer prompt", "user prompt", "secret request",
		"prompt:", "transcript:", "/home/", "/users/", "$home", "${", "%appdata%",
	}
	for _, marker := range markers {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return cursorWindowsAbsolutePathPattern.MatchString(value)
}

// WithCommandOutputExcerpts attaches bounded stdout/stderr excerpts to provider diagnostics.
func WithCommandOutputExcerpts(diagnostics *workerexecution.WorkDiagnostics, stdout, stderr []byte) *workerexecution.WorkDiagnostics {
	excerpts := make(map[string]string, 2)
	if excerpt := BoundedCommandOutputExcerpt(stdout, CommandOutputExcerptLimit); excerpt != "" {
		excerpts[ResponseMetadataStdoutExcerpt] = excerpt
	}
	if excerpt := BoundedCommandOutputExcerpt(stderr, CommandOutputExcerptLimit); excerpt != "" {
		excerpts[ResponseMetadataStderrExcerpt] = excerpt
	}
	if len(excerpts) == 0 {
		return diagnostics
	}
	return WithResponseMetadata(diagnostics, excerpts)
}

// WithResponseMetadata merges Cursor response metadata into provider diagnostics.
func WithResponseMetadata(diagnostics *workerexecution.WorkDiagnostics, metadata map[string]string) *workerexecution.WorkDiagnostics {
	if len(metadata) == 0 {
		return diagnostics
	}
	diagnostics = workerexecution.CloneWorkDiagnostics(diagnostics)
	if diagnostics == nil {
		diagnostics = &workerexecution.WorkDiagnostics{}
	}
	if diagnostics.Provider == nil {
		diagnostics.Provider = &workerexecution.ProviderDiagnostic{}
	}
	if diagnostics.Provider.ResponseMetadata == nil {
		diagnostics.Provider.ResponseMetadata = make(map[string]string, len(metadata))
	}
	for key, value := range metadata {
		diagnostics.Provider.ResponseMetadata[key] = value
	}
	return diagnostics
}

// SuccessStdoutJSON builds representative Cursor success JSON for tests.
func SuccessStdoutJSON(result, sessionID string) []byte {
	if result == "" {
		result = "Done. COMPLETE"
	}
	if sessionID == "" {
		sessionID = "cursor-test-session"
	}
	payload := resultPayload{
		Type:      ResultTypeResult,
		Subtype:   ResultSubtypeSuccess,
		SessionID: sessionID,
		Result:    result,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	return encoded
}
