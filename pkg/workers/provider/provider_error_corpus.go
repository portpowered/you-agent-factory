package provider

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	claudeFailureMessageBytes = 1024
	claudeFailureScanBytes    = 64 * 1024
)

const (
	claudeAuthFailureMessage       = "Claude authentication failed."
	claudeBadRequestFailureMessage = "Claude rejected the request as invalid."
	claudeConfigFailureMessage     = "Claude is not configured correctly."
	claudeServerFailureMessage     = "Claude encountered a temporary server error."
	claudeThrottleFailureMessage   = "Claude is temporarily unavailable due to rate or capacity limits."
	claudeTimeoutFailureMessage    = "Claude request timed out."
)

type claudeStructuredFailure struct {
	Type    string
	Status  int
	Message string
}

// ParseClaudeProviderFailure parses Claude CLI API Error records before any
// text fallback and returns the reason and customer-visible message together.
// Structured scanning covers the complete captured streams while bounding each
// candidate record; text fallback and actionable messages are separately
// bounded so a failed command cannot publish its transcript.
func ParseClaudeProviderFailure(result CommandResult) ProviderFailureResult {
	structuredStreams := [][]byte{result.Stderr, result.Stdout}
	if failure, ok := lastClaudeStructuredFailure(structuredStreams); ok {
		return failure
	}

	streams := []string{
		tailForClaudeFailureScan(result.Stderr),
		tailForClaudeFailureScan(result.Stdout),
	}
	if failure, ok := lastClaudeTextFailure(streams); ok {
		return failure
	}
	if result.ExitCode == 124 {
		return ProviderFailureResult{
			Reason:  interfaces.WorkFailureTypeTimeout,
			Message: claudeTimeoutFailureMessage,
		}
	}
	return ProviderFailureResult{
		Reason:  interfaces.WorkFailureTypeUnknown,
		Message: claudeUnknownFailureMessage(streams, result.ExitCode),
	}
}

// Streams are ordered stderr then stdout, and lines retain their stream order.
// The final recognized record wins; malformed and unrelated later lines do not
// displace it. This matches structured-record selection above.
func lastClaudeStructuredFailure(streams [][]byte) (ProviderFailureResult, bool) {
	var last ProviderFailureResult
	var found bool
	for _, stream := range streams {
		for len(stream) > 0 {
			line := stream
			if newline := bytes.IndexByte(stream, '\n'); newline >= 0 {
				line = stream[:newline]
				stream = stream[newline+1:]
			} else {
				stream = nil
			}
			if len(line) > claudeFailureScanBytes {
				continue
			}
			failure, ok := decodeClaudeAPIError(string(line))
			if !ok {
				continue
			}
			reason, recognized := classifyClaudeStructuredFailure(failure.Type, failure.Status)
			if !recognized {
				continue
			}
			last = ProviderFailureResult{
				Reason:  reason,
				Message: claudeStructuredFailureMessage(failure.Message, reason),
			}
			found = true
		}
	}
	return last, found
}

func decodeClaudeAPIError(line string) (claudeStructuredFailure, bool) {
	const prefix = "API Error:"
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, prefix) {
		return claudeStructuredFailure{}, false
	}
	remainder := strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
	statusText, payload, ok := strings.Cut(remainder, " ")
	if !ok {
		return claudeStructuredFailure{}, false
	}
	status, err := strconv.Atoi(statusText)
	if err != nil || !strings.HasPrefix(strings.TrimSpace(payload), "{") {
		return claudeStructuredFailure{}, false
	}

	var envelope struct {
		Type    string `json:"type"`
		Message string `json:"message"`
		Error   *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(payload)), &envelope); err != nil {
		return claudeStructuredFailure{}, false
	}

	failure := claudeStructuredFailure{Type: envelope.Type, Status: status, Message: envelope.Message}
	if envelope.Error != nil {
		if envelope.Error.Type != "" {
			failure.Type = envelope.Error.Type
		}
		if envelope.Error.Message != "" {
			failure.Message = envelope.Error.Message
		}
	}
	return failure, true
}

func classifyClaudeStructuredFailure(errorType string, status int) (interfaces.WorkFailureType, bool) {
	switch strings.ToLower(strings.TrimSpace(errorType)) {
	case "authentication_error", "permission_error":
		return interfaces.WorkFailureTypeAuthFailure, true
	case "invalid_request_error":
		return interfaces.WorkFailureTypePermanentBadRequest, true
	case "rate_limit_error", "overloaded_error":
		return interfaces.WorkFailureTypeThrottled, true
	case "api_error", "server_error":
		return interfaces.WorkFailureTypeInternalServerError, true
	}

	switch {
	case status == 401 || status == 403:
		return interfaces.WorkFailureTypeAuthFailure, true
	case status == 400 || status == 413 || status == 422:
		return interfaces.WorkFailureTypePermanentBadRequest, true
	case status == 429 || status == 529:
		return interfaces.WorkFailureTypeThrottled, true
	case status >= 500 && status <= 599:
		return interfaces.WorkFailureTypeInternalServerError, true
	default:
		return interfaces.WorkFailureTypeUnknown, false
	}
}

func claudeStructuredFailureMessage(message string, reason interfaces.WorkFailureType) string {
	if reason == interfaces.WorkFailureTypeAuthFailure || reason == interfaces.WorkFailureTypePermanentBadRequest {
		if normalized, ok := safeClaudeActionableMessage(message); ok {
			return normalized
		}
	}
	return claudeFallbackFailureMessage(reason, 0)
}

func safeClaudeActionableMessage(message string) (string, bool) {
	normalized := normalizeClaudeMessage(message)
	lower := strings.ToLower(normalized)
	if normalized == "" ||
		strings.HasPrefix(lower, "api error:") ||
		containsClaudeCredentialSignal(lower) ||
		containsClaudeTranscriptMarker(lower) {
		return "", false
	}
	return boundUTF8Bytes(normalized, claudeFailureMessageBytes), true
}

func containsClaudeCredentialSignal(message string) bool {
	if containsAny(message,
		"authorization:",
		"bearer ",
		"api_key=",
		"api-key:",
		"api key:",
		"sk-ant-",
	) {
		return true
	}
	if containsClaudeCredentialWord(message) || containsClaudeCredentialFieldValue(message) || containsClaudeSensitiveIdentifier(message) {
		return true
	}

	// Keep assignment and header detection aligned with the environment
	// diagnostic policy so supported sensitive names cannot be published just
	// because their exact spelling was absent from the literals above.
	for remainder := message; ; {
		separator := strings.IndexAny(remainder, "=:")
		if separator < 0 {
			break
		}
		prefix := strings.TrimSpace(remainder[:separator])
		if boundary := strings.LastIndexAny(prefix, " \t\r\n"); boundary >= 0 {
			prefix = prefix[boundary+1:]
		}
		name := strings.Trim(prefix, "\"'{}[](),")
		if ClassifyCommandEnvKey(name) == CommandEnvClassificationRedacted {
			return true
		}
		remainder = remainder[separator+1:]
	}
	return false
}

func containsClaudeCredentialFieldValue(message string) bool {
	fields := strings.Fields(message)
	for index, field := range fields {
		normalized := strings.Trim(field, "\"'{}[](),.;:!?=")
		if isClaudeCredentialField(normalized) && index+1 < len(fields) {
			return true
		}
		if normalized == "api" && index+2 < len(fields) &&
			strings.Trim(fields[index+1], "\"'{}[](),.;:!?=") == "key" {
			return true
		}
		if normalized == "auth" && index+2 < len(fields) &&
			strings.Trim(fields[index+1], "\"'{}[](),.;:!?=") == "token" {
			return true
		}
	}
	return false
}

func isClaudeCredentialField(field string) bool {
	if field == "authorization" {
		return true
	}
	identifier := strings.ReplaceAll(field, "-", "_")
	return containsClaudeSensitiveIdentifierPart(identifier) &&
		ClassifyCommandEnvKey(identifier) == CommandEnvClassificationRedacted
}

func containsClaudeCredentialWord(message string) bool {
	for _, field := range strings.Fields(message) {
		switch strings.Trim(field, "\"'{}[](),.;:!?=") {
		case "credential", "credentials", "password", "secret", "token":
			return true
		}
	}
	return false
}

func containsClaudeSensitiveIdentifier(message string) bool {
	for _, field := range strings.Fields(message) {
		identifier := strings.Trim(field, "\"'{}[](),.;:!?=")
		if strings.Contains(identifier, "_") &&
			containsClaudeSensitiveIdentifierPart(identifier) &&
			ClassifyCommandEnvKey(identifier) == CommandEnvClassificationRedacted {
			return true
		}
	}
	return false
}

func containsClaudeSensitiveIdentifierPart(identifier string) bool {
	for _, part := range strings.Split(identifier, "_") {
		switch part {
		case "auth", "credential", "credentials", "key", "pass", "password", "secret", "token":
			return true
		}
	}
	return false
}

func lastClaudeTextFailure(streams []string) (ProviderFailureResult, bool) {
	var last ProviderFailureResult
	var found bool
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			normalized := normalizeClaudeMessage(line)
			if !isClaudeTextDiagnosticRecord(normalized) {
				continue
			}
			reason, ok := claudeTextFailureReason(strings.ToLower(normalized))
			if !ok {
				continue
			}
			last = ProviderFailureResult{
				Reason:  reason,
				Message: claudeTextFailureMessage(normalized, reason),
			}
			found = true
		}
	}
	return last, found
}

// Text fallback accepts only known Claude diagnostic shapes. Transcript and
// prompt markers are rejected wherever they appear so their content cannot
// influence failure policy or become a customer-visible message.
func isClaudeTextDiagnosticRecord(message string) bool {
	lower := strings.ToLower(message)
	if lower == "" || containsClaudeTranscriptMarker(lower) {
		return false
	}
	return hasAnyPrefix(lower,
		"api error:",
		"error:",
		"claude error:",
		"request failed:",
		"configuration error",
		"configuration is invalid",
		"invalid configuration",
		"config file not found",
		"anthropic_api_key is not set",
		"model is not configured",
		"api key",
		"authentication error",
		"permission error",
		"not logged in",
		"login required",
		"unauthorized",
		"forbidden",
		"invalid_request_error",
		"bad request",
		"invalid request",
		"request_too_large",
		"rate limit",
		"too many requests",
		"overloaded",
		"529",
		"internal server error",
		"unexpected status 500",
		"unexpected status 502",
		"unexpected status 503",
		"unexpected status 504",
		"request deadline exceeded",
		"request timed out",
		"request timeout",
		"deadline exceeded",
		"timed out",
		"timeout",
	)
}

func containsClaudeTranscriptMarker(message string) bool {
	return containsAny(message, "user:", "human:", "assistant:", "system:", "prompt:")
}

func claudeTextFailureReason(message string) (interfaces.WorkFailureType, bool) {
	if strings.HasPrefix(message, "cleanup ") ||
		strings.HasPrefix(message, "cleaning up ") ||
		strings.HasPrefix(message, "teardown ") {
		return interfaces.WorkFailureTypeUnknown, false
	}
	switch {
	case containsAny(message, "configuration error", "configuration is invalid", "invalid configuration", "config file not found", "anthropic_api_key is not set", "model is not configured"):
		return interfaces.WorkFailureTypeMisconfigured, true
	case containsAny(message, `"type":"authentication_error"`, `"type":"permission_error"`, "api key", "authentication error", "permission error", "not logged in", "login required", "unauthorized", "forbidden"):
		return interfaces.WorkFailureTypeAuthFailure, true
	case containsAny(message, `"type":"invalid_request_error"`, "invalid_request_error", "bad request", "invalid request", "request_too_large"):
		return interfaces.WorkFailureTypePermanentBadRequest, true
	case containsAny(message, `"type":"rate_limit_error"`, `"type":"overloaded_error"`, "rate limit", "too many requests", "overloaded", "529"):
		return interfaces.WorkFailureTypeThrottled, true
	case containsAny(message, `"type":"api_error"`, "internal server error", "unexpected status 500", "unexpected status 502", "unexpected status 503", "unexpected status 504"):
		return interfaces.WorkFailureTypeInternalServerError, true
	case containsAny(message, "deadline exceeded", "timed out", "timeout"):
		return interfaces.WorkFailureTypeTimeout, true
	default:
		return interfaces.WorkFailureTypeUnknown, false
	}
}

func claudeTextFailureMessage(message string, reason interfaces.WorkFailureType) string {
	if reason == interfaces.WorkFailureTypeAuthFailure ||
		reason == interfaces.WorkFailureTypePermanentBadRequest ||
		reason == interfaces.WorkFailureTypeMisconfigured {
		if actionable, ok := safeClaudeActionableMessage(message); ok {
			return actionable
		}
	}
	return claudeFallbackFailureMessage(reason, 0)
}

func claudeFallbackFailureMessage(reason interfaces.WorkFailureType, exitCode int) string {
	switch reason {
	case interfaces.WorkFailureTypeAuthFailure:
		return claudeAuthFailureMessage
	case interfaces.WorkFailureTypePermanentBadRequest:
		return claudeBadRequestFailureMessage
	case interfaces.WorkFailureTypeMisconfigured:
		return claudeConfigFailureMessage
	case interfaces.WorkFailureTypeThrottled:
		return claudeThrottleFailureMessage
	case interfaces.WorkFailureTypeInternalServerError:
		return claudeServerFailureMessage
	case interfaces.WorkFailureTypeTimeout:
		return claudeTimeoutFailureMessage
	default:
		return fmt.Sprintf("claude exited with code %d", exitCode)
	}
}

func claudeUnknownFailureMessage(streams []string, exitCode int) string {
	var candidate string
	lineCount := 0
	for _, stream := range streams {
		for _, line := range strings.Split(stream, "\n") {
			normalized := normalizeClaudeMessage(line)
			if normalized == "" {
				continue
			}
			lineCount++
			candidate = normalized
		}
	}
	if lineCount == 1 && safeClaudeUnknownExcerpt(candidate) {
		return boundUTF8Bytes("Claude failed: "+candidate, claudeFailureMessageBytes)
	}
	return fmt.Sprintf("claude exited with code %d", exitCode)
}

func safeClaudeUnknownExcerpt(message string) bool {
	lower := strings.ToLower(message)
	return containsAny(lower, "error", "fail") &&
		!containsClaudeCredentialSignal(lower) &&
		!containsClaudeTranscriptMarker(lower) &&
		!strings.HasPrefix(lower, "api error:") &&
		!strings.ContainsAny(message, "{}")
}

func hasAnyPrefix(value string, prefixes ...string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}

func normalizeClaudeMessage(message string) string {
	return strings.Join(strings.Fields(strings.ToValidUTF8(message, "")), " ")
}

func tailForClaudeFailureScan(output []byte) string {
	if len(output) <= claudeFailureScanBytes {
		return string(output)
	}
	return string(output[len(output)-claudeFailureScanBytes:])
}

func boundUTF8Bytes(message string, limit int) string {
	if limit <= 0 || len(message) <= limit {
		return message
	}
	bounded := []byte(message)[:limit]
	for !utf8.Valid(bounded) {
		bounded = bounded[:len(bounded)-1]
	}
	return strings.TrimSpace(string(bounded))

}

// ProviderErrorCorpusEntry is one shared raw provider-failure fixture used by
// worker unit tests and functional smoke coverage.
type ProviderErrorCorpusEntry struct {
	Name                  string                       `json:"name"`
	Provider              interfaces.ModelProvider     `json:"provider"`
	RawProviderFamily     string                       `json:"raw_provider_family"`
	Category              string                       `json:"category"`
	UpstreamSourceCase    string                       `json:"upstream_source_case"`
	ExitCode              int                          `json:"exit_code"`
	Stdout                string                       `json:"stdout"`
	Stderr                string                       `json:"stderr"`
	ExpectedType          interfaces.WorkFailureType   `json:"expected_type"`
	ExpectedFamily        interfaces.WorkFailureFamily `json:"expected_family"`
	ExpectedMessage       string                       `json:"expected_message,omitempty"`
	Retryable             bool                         `json:"retryable"`
	TriggersThrottlePause bool                         `json:"triggers_throttle_pause"`
	Supported             bool                         `json:"supported"`
	RejectMessageContains []string                     `json:"reject_message_contains,omitempty"`
	Notes                 string                       `json:"notes,omitempty"`
}

// CommandResult renders the raw shared fixture into the provider subprocess
// contract used by normalization tests and smoke harnesses.
func (e ProviderErrorCorpusEntry) CommandResult() CommandResult {
	return CommandResult{
		ExitCode: e.ExitCode,
		Stdout:   []byte(e.Stdout),
		Stderr:   []byte(e.Stderr),
	}
}

// RepeatedCommandResults expands one shared failure shape into a fixed number
// of repeated provider command results for bounded retry and throttle tests.
func (e ProviderErrorCorpusEntry) RepeatedCommandResults(count int) []CommandResult {
	results := make([]CommandResult, 0, count)
	for range count {
		results = append(results, e.CommandResult())
	}
	return results
}

type providerErrorCorpusFile struct {
	Entries []ProviderErrorCorpusEntry `json:"entries"`
}

// ProviderErrorCorpus is the cached shared provider-failure fixture set.
type ProviderErrorCorpus struct {
	entriesByName map[string]ProviderErrorCorpusEntry
	allEntries    []ProviderErrorCorpusEntry
}

// Entry returns the named shared fixture.
func (c ProviderErrorCorpus) Entry(name string) (ProviderErrorCorpusEntry, bool) {
	entry, ok := c.entriesByName[name]
	return entry, ok
}

// Entries returns all corpus fixtures in stable order.
func (c ProviderErrorCorpus) Entries() []ProviderErrorCorpusEntry {
	return append([]ProviderErrorCorpusEntry(nil), c.allEntries...)
}

// SupportedEntriesForCategory returns the currently supported fixtures for one
// normalized provider-failure category.
func (c ProviderErrorCorpus) SupportedEntriesForCategory(category string) []ProviderErrorCorpusEntry {
	entries := make([]ProviderErrorCorpusEntry, 0, len(c.allEntries))
	for _, entry := range c.allEntries {
		if entry.Supported && entry.Category == category {
			entries = append(entries, entry)
		}
	}
	return entries
}

//go:embed testdata/provider_error_corpus.json
var providerErrorCorpusJSON []byte

var (
	providerErrorCorpusOnce sync.Once
	providerErrorCorpus     ProviderErrorCorpus
	providerErrorCorpusErr  error
)

// LoadProviderErrorCorpus returns the shared provider-failure fixture corpus.
func LoadProviderErrorCorpus() (ProviderErrorCorpus, error) {
	providerErrorCorpusOnce.Do(func() {
		providerErrorCorpus, providerErrorCorpusErr = loadProviderErrorCorpus()
	})
	return providerErrorCorpus, providerErrorCorpusErr
}

func loadProviderErrorCorpus() (ProviderErrorCorpus, error) {
	var raw providerErrorCorpusFile
	if err := json.Unmarshal(providerErrorCorpusJSON, &raw); err != nil {
		return ProviderErrorCorpus{}, fmt.Errorf("decode provider error corpus: %w", err)
	}
	if len(raw.Entries) == 0 {
		return ProviderErrorCorpus{}, fmt.Errorf("decode provider error corpus: no entries")
	}

	entriesByName := make(map[string]ProviderErrorCorpusEntry, len(raw.Entries))
	for _, entry := range raw.Entries {
		if err := validateProviderErrorCorpusEntry(entry); err != nil {
			return ProviderErrorCorpus{}, err
		}
		if _, exists := entriesByName[entry.Name]; exists {
			return ProviderErrorCorpus{}, fmt.Errorf("decode provider error corpus: duplicate entry %q", entry.Name)
		}
		entriesByName[entry.Name] = entry
	}

	return ProviderErrorCorpus{
		entriesByName: entriesByName,
		allEntries:    append([]ProviderErrorCorpusEntry(nil), raw.Entries...),
	}, nil
}

func validateProviderErrorCorpusEntry(entry ProviderErrorCorpusEntry) error {
	if entry.Name == "" {
		return fmt.Errorf("decode provider error corpus: missing entry name")
	}
	if entry.Provider == "" {
		return fmt.Errorf("decode provider error corpus: entry %q missing provider", entry.Name)
	}
	if entry.RawProviderFamily == "" {
		return fmt.Errorf("decode provider error corpus: entry %q missing raw provider family", entry.Name)
	}
	if entry.Category == "" {
		return fmt.Errorf("decode provider error corpus: entry %q missing category", entry.Name)
	}
	if entry.UpstreamSourceCase == "" {
		return fmt.Errorf("decode provider error corpus: entry %q missing upstream source case", entry.Name)
	}
	if entry.ExpectedType == "" {
		return fmt.Errorf("decode provider error corpus: entry %q missing expected type", entry.Name)
	}
	if entry.ExpectedFamily == "" {
		return fmt.Errorf("decode provider error corpus: entry %q missing expected family", entry.Name)
	}
	if entry.Provider == interfaces.ModelProviderClaude && entry.ExpectedMessage == "" {
		return fmt.Errorf("decode provider error corpus: Claude entry %q missing expected message", entry.Name)
	}
	if entry.ExpectedFamily == interfaces.WorkFailureFamilyThrottle && !entry.TriggersThrottlePause {
		return fmt.Errorf("decode provider error corpus: entry %q throttle family must trigger throttle pause", entry.Name)
	}
	if entry.TriggersThrottlePause && !entry.Retryable {
		return fmt.Errorf("decode provider error corpus: entry %q throttle pause requires retryable=true", entry.Name)
	}
	return nil
}
