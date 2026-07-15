package progress

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"hash"
	"path/filepath"
	"strconv"
	"strings"

	modelprovider "github.com/portpowered/infinite-you/pkg/models/provider"

	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"

	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
)

const (
	ProgressRetainedTextBytes      = 4096
	ProgressRetainedProgressBytes  = 1024
	ProgressMetadataRunnerIDKey    = "runner_id"
	ProgressMetadataWorkIDKey      = "work_id"
	ProgressMetadataWorkstationKey = "workstation_name"
	ProgressMetadataTextBytesKey   = "text_bytes"
	ProgressMetadataTruncatedKey   = "payload_truncated"
	ProgressMetadataRawBytesKey    = "raw_bytes"
	ProgressMetadataRawSHA256Key   = "raw_sha256"
	ProgressMetadataDiagnosticKey  = "diagnostic_class"
)

const (
	ProgressDiagnosticUnknownEvent  = "unknown_event"
	ProgressDiagnosticMalformedJSON = "malformed_json"
	ProgressDiagnosticIncompleteSSE = "incomplete_event_stream"
)

const (
	progressFragmentKind    = "PROGRESS_FRAGMENT"
	responseFragmentKind    = "RESPONSE_FRAGMENT"
	normalizedTypeUnknown   = "UNKNOWN"
	normalizedTypeStarted   = "STARTED"
	normalizedTypeProgress  = "PROGRESS"
	normalizedTypeTextDelta = "TEXT_DELTA"
	normalizedTypeFinalText = "FINAL_TEXT"
	normalizedTypeFailed    = "FAILED"
	normalizedTypeCanceled  = "CANCELED"
)

const (
	providerSessionKindSessionID      = "session_id"
	providerSessionKindConversationID = "conversation_id"
	providerSessionKindResponseID     = "response_id"
)

// ProgressFragment is the Codex-owned progress-stream payload published at the
// provider boundary before shared orchestration maps it into session fragments.
type ProgressFragment struct {
	DispatchID         string
	Kind               string
	Type               string
	Payload            string
	ProviderSessionRef *workerexecution.ProviderSessionMetadata
	ExternalEventType  string
	Metadata           map[string]string
}

// ProgressPublisher receives Codex-owned progress fragments for one invocation.
type ProgressPublisher func(fragment ProgressFragment)

// IsCommand reports whether command names the Codex CLI executable.
func IsCommand(command string) bool {
	base := filepath.Base(strings.ReplaceAll(strings.TrimSpace(command), `\`, "/"))
	extension := strings.ToLower(filepath.Ext(base))
	if extension == ".exe" || extension == ".cmd" || extension == ".bat" {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return strings.EqualFold(base, string(modelprovider.Codex))
}

// ProgressStream observes Codex subprocess stdout/stderr and publishes
// provider-neutral progress fragments for legacy SSE/JSON response events.
type ProgressStream struct {
	req       workerprocess.CommandRequest
	publisher ProgressPublisher

	stdoutBuffer string
	stderrBuffer string

	pendingSSEEvent string
	pendingSSEData  []string
	providerSession *workerexecution.ProviderSessionMetadata
	hasher          hash.Hash
}

// NewProgressStream constructs a Codex-owned stdout/stderr observer for one invocation.
func NewProgressStream(req workerprocess.CommandRequest, publisher ProgressPublisher) *ProgressStream {
	return &ProgressStream{
		req:       workerexecution.CloneSubprocessExecutionRequest(req),
		publisher: publisher,
		hasher:    sha256.New(),
	}
}

func (s *ProgressStream) Observe(stream string, chunk []byte) bool {
	switch stream {
	case workerprocess.OutputStreamStdout:
		s.stdoutBuffer += string(chunk)
		s.drainLines(&s.stdoutBuffer, s.handleStdoutLine)
	case workerprocess.OutputStreamStderr:
		s.stderrBuffer += string(chunk)
		s.drainLines(&s.stderrBuffer, s.handleStderrLine)
	default:
		return false
	}
	return true
}

func (s *ProgressStream) Flush() {
	if trimmed := strings.TrimSpace(s.stdoutBuffer); trimmed != "" {
		s.handleStdoutLine(trimmed)
	}
	if trimmed := strings.TrimSpace(s.stderrBuffer); trimmed != "" {
		s.handleStderrLine(trimmed)
	}
	s.flushPendingSSEEvent()
}

func (s *ProgressStream) drainLines(buffer *string, handle func(string)) {
	for {
		index := strings.IndexByte(*buffer, '\n')
		if index < 0 {
			return
		}
		line := strings.TrimRight((*buffer)[:index], "\r")
		*buffer = (*buffer)[index+1:]
		handle(line)
	}
}

func (s *ProgressStream) handleStdoutLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		s.flushPendingSSEEvent()
		return
	}
	if strings.HasPrefix(trimmed, "event:") {
		s.flushPendingSSEEvent()
		s.pendingSSEEvent = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
		return
	}
	if strings.HasPrefix(trimmed, "data:") {
		s.pendingSSEData = append(s.pendingSSEData, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		return
	}
	s.flushPendingSSEEvent()
	if s.publishStructuredEvent(trimmed, "") {
		return
	}
	s.publishResponseEvent(normalizedTypeTextDelta, "", trimmed, nil)
}

func (s *ProgressStream) handleStderrLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}
	if s.publishStructuredEvent(trimmed, "") {
		return
	}
	s.publishProgressEvent(normalizedTypeProgress, "", trimmed, nil)
}

func (s *ProgressStream) publishStructuredEvent(raw string, fallbackEventType string) bool {
	normalized, status := normalizeStructuredEvent(raw, fallbackEventType, s.hasher)
	if status == structuredEventStatusNotStructured {
		return false
	}
	if normalized.ProviderSessionRef != nil {
		s.providerSession = workerexecution.CloneProviderSessionMetadata(normalized.ProviderSessionRef)
	}
	if normalized.ProviderSessionRef == nil {
		normalized.ProviderSessionRef = workerexecution.CloneProviderSessionMetadata(s.providerSession)
	}
	normalized.DispatchID = strings.TrimSpace(s.req.DispatchID)
	normalized.Metadata = mergeStringMaps(baseFragmentMetadata(s.req), normalized.Metadata)
	s.publisher(normalized)
	return true
}

func (s *ProgressStream) flushPendingSSEEvent() {
	if s.pendingSSEEvent == "" && len(s.pendingSSEData) == 0 {
		return
	}
	raw := strings.Join(s.pendingSSEData, "\n")
	s.publishStructuredEvent(raw, strings.TrimSpace(s.pendingSSEEvent))
	s.pendingSSEEvent = ""
	s.pendingSSEData = nil
}

type structuredEventStatus string

const (
	structuredEventStatusNotStructured structuredEventStatus = "NOT_STRUCTURED"
	structuredEventStatusNormalized    structuredEventStatus = "NORMALIZED"
	structuredEventStatusMalformed     structuredEventStatus = "MALFORMED"
)

func normalizeStructuredEvent(raw string, fallbackEventType string, hasher hash.Hash) (ProgressFragment, structuredEventStatus) {
	trimmedRaw := strings.TrimSpace(raw)
	trimmedFallback := strings.TrimSpace(fallbackEventType)
	if trimmedRaw == "" && trimmedFallback == "" {
		return ProgressFragment{}, structuredEventStatusNotStructured
	}
	if !looksLikeStructuredPayload(trimmedRaw, trimmedFallback) {
		return ProgressFragment{}, structuredEventStatusNotStructured
	}
	var payload map[string]any
	if trimmedRaw == "" {
		return malformedStructuredEvent(trimmedRaw, trimmedFallback, ProgressDiagnosticIncompleteSSE, hasher), structuredEventStatusMalformed
	}
	if err := json.Unmarshal([]byte(trimmedRaw), &payload); err != nil {
		return malformedStructuredEvent(trimmedRaw, trimmedFallback, ProgressDiagnosticMalformedJSON, hasher), structuredEventStatusMalformed
	}
	externalEventType := firstNonEmptyString(
		stringValue(payload["event"]),
		stringValue(payload["type"]),
		trimmedFallback,
	)
	normalizedType, kind := classifyStructuredEvent(externalEventType)
	fragment := ProgressFragment{
		Kind:               kind,
		Type:               normalizedType,
		ExternalEventType:  externalEventType,
		ProviderSessionRef: providerSessionFromStructuredEvent(payload),
		Metadata:           structuredEventMetadata(payload),
	}
	switch normalizedType {
	case normalizedTypeStarted:
		original := firstNonEmptyString(
			stringValue(payload["message"]),
			externalEventType,
		)
		fragment.Payload = boundedPayload(original, ProgressRetainedProgressBytes)
		fragment.Metadata = annotateBoundedPayloadMetadata(fragment.Metadata, original, fragment.Payload)
	case normalizedTypeProgress:
		original := firstNonEmptyString(
			stringValue(payload["message"]),
			stringValue(payload["status"]),
			externalEventType,
		)
		fragment.Payload = boundedPayload(original, ProgressRetainedProgressBytes)
		fragment.Metadata = annotateBoundedPayloadMetadata(fragment.Metadata, original, fragment.Payload)
	case normalizedTypeTextDelta, normalizedTypeFinalText:
		original := extractEventText(payload)
		fragment.Payload = boundedPayload(original, ProgressRetainedTextBytes)
		fragment.Metadata = annotateBoundedPayloadMetadata(fragment.Metadata, original, fragment.Payload)
	case normalizedTypeFailed, normalizedTypeCanceled:
		original := firstNonEmptyString(
			stringValue(payload["message"]),
			stringValue(payload["error"]),
			stringValue(payload["status"]),
			externalEventType,
		)
		fragment.Payload = boundedPayload(original, ProgressRetainedProgressBytes)
		fragment.Metadata = annotateBoundedPayloadMetadata(fragment.Metadata, original, fragment.Payload)
	default:
		fragment.Payload = "codex event omitted"
		fragment.Metadata = mergeStringMaps(fragment.Metadata, rawDiagnosticMetadata(trimmedRaw, ProgressDiagnosticUnknownEvent, hasher))
	}
	return fragment, structuredEventStatusNormalized
}

func classifyStructuredEvent(externalEventType string) (string, string) {
	normalized := strings.ToLower(strings.TrimSpace(externalEventType))
	switch normalized {
	case "session.created", "response.created", "response.started", "turn.started":
		return normalizedTypeStarted, progressFragmentKind
	case "response.output_text.delta", "response.delta", "message.delta", "output_text.delta":
		return normalizedTypeTextDelta, responseFragmentKind
	case "response.completed", "response.output_text.done", "message.completed", "output.completed":
		return normalizedTypeFinalText, responseFragmentKind
	case "response.failed", "response.error", "error":
		return normalizedTypeFailed, progressFragmentKind
	case "response.canceled", "response.cancelled", "session.canceled", "session.cancelled":
		return normalizedTypeCanceled, progressFragmentKind
	case "response.progress", "progress", "response.updated":
		return normalizedTypeProgress, progressFragmentKind
	}
	switch {
	case strings.Contains(normalized, "cancel"):
		return normalizedTypeCanceled, progressFragmentKind
	case strings.Contains(normalized, "fail"), strings.Contains(normalized, "error"):
		return normalizedTypeFailed, progressFragmentKind
	case strings.Contains(normalized, "delta"):
		return normalizedTypeTextDelta, responseFragmentKind
	case strings.Contains(normalized, "complete"), strings.Contains(normalized, "final"), strings.Contains(normalized, "done"):
		return normalizedTypeFinalText, responseFragmentKind
	case strings.Contains(normalized, "start"), strings.Contains(normalized, "created"), strings.Contains(normalized, "begin"):
		return normalizedTypeStarted, progressFragmentKind
	case strings.Contains(normalized, "progress"), strings.Contains(normalized, "update"):
		return normalizedTypeProgress, progressFragmentKind
	default:
		return normalizedTypeUnknown, progressFragmentKind
	}
}

func providerSessionFromStructuredEvent(payload map[string]any) *workerexecution.ProviderSessionMetadata {
	candidates := []struct {
		kind  string
		value string
	}{
		{kind: providerSessionKindSessionID, value: firstNonEmptyString(stringValue(payload["session_id"]), stringValue(payload["sessionId"]))},
		{kind: providerSessionKindResponseID, value: firstNonEmptyString(stringValue(payload["response_id"]), stringValue(payload["responseId"]))},
		{kind: providerSessionKindConversationID, value: firstNonEmptyString(stringValue(payload["conversation_id"]), stringValue(payload["conversationId"]))},
	}
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.value) == "" {
			continue
		}
		return &workerexecution.ProviderSessionMetadata{
			Provider: workerexecution.CanonicalProviderSessionProvider(string(modelprovider.Codex)),
			Kind:     candidate.kind,
			ID:       strings.TrimSpace(candidate.value),
		}
	}
	return nil
}

func structuredEventMetadata(payload map[string]any) map[string]string {
	metadata := map[string]string{}
	for _, key := range []string{"session_id", "response_id", "conversation_id"} {
		if value := stringValue(payload[key]); value != "" {
			metadata[key] = value
		}
	}
	if text := extractEventText(payload); text != "" {
		metadata[ProgressMetadataTextBytesKey] = strconv.Itoa(len([]byte(text)))
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func extractEventText(payload map[string]any) string {
	if delta := stringValue(payload["delta"]); strings.TrimSpace(delta) != "" {
		return delta
	}
	if text := stringValue(payload["text"]); strings.TrimSpace(text) != "" {
		return text
	}
	return strings.TrimSpace(collectEventText(payload))
}

func collectEventText(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		if strings.EqualFold(stringValue(typed["type"]), "output_text") {
			if text := stringValue(typed["text"]); text != "" {
				return text
			}
		}
		parts := make([]string, 0, 4)
		for _, nestedKey := range []string{"response", "output", "content", "item", "message"} {
			if nested, ok := typed[nestedKey]; ok {
				if text := collectEventText(nested); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := collectEventText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func boundedPayload(payload string, limit int) string {
	trimmed := strings.TrimSpace(payload)
	if limit <= 0 || len([]byte(trimmed)) <= limit {
		return trimmed
	}
	bytes := []byte(trimmed)
	return strings.TrimSpace(string(bytes[:limit]))
}

func baseFragmentMetadata(req workerprocess.CommandRequest) map[string]string {
	metadata := map[string]string{
		ProgressMetadataRunnerIDKey: string(modelprovider.Codex),
	}
	if workID := primaryWorkID(req.Execution.WorkIDs); workID != "" {
		metadata[ProgressMetadataWorkIDKey] = workID
	}
	if workstation := strings.TrimSpace(req.WorkstationName); workstation != "" {
		metadata[ProgressMetadataWorkstationKey] = workstation
	}
	return metadata
}

func primaryWorkID(workIDs []string) string {
	for _, workID := range workIDs {
		if workID != "" {
			return workID
		}
	}
	return ""
}

func mergeStringMaps(base, overlay map[string]string) map[string]string {
	switch {
	case len(base) == 0 && len(overlay) == 0:
		return nil
	case len(base) == 0:
		return cloneStringMap(overlay)
	case len(overlay) == 0:
		return cloneStringMap(base)
	}
	merged := cloneStringMap(base)
	for key, value := range overlay {
		merged[key] = value
	}
	return merged
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func looksLikeStructuredPayload(raw string, fallbackEventType string) bool {
	if strings.TrimSpace(fallbackEventType) != "" {
		return true
	}
	trimmed := strings.TrimSpace(raw)
	return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[")
}

func malformedStructuredEvent(raw string, fallbackEventType string, diagnosticClass string, hasher hash.Hash) ProgressFragment {
	return ProgressFragment{
		Kind:              progressFragmentKind,
		Type:              normalizedTypeUnknown,
		Payload:           "codex event omitted",
		ExternalEventType: strings.TrimSpace(fallbackEventType),
		Metadata:          rawDiagnosticMetadata(raw, diagnosticClass, hasher),
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
	annotated[ProgressMetadataTextBytesKey] = strconv.Itoa(len([]byte(trimmedOriginal)))
	if len([]byte(bounded)) < len([]byte(trimmedOriginal)) {
		annotated[ProgressMetadataTruncatedKey] = "true"
	}
	return annotated
}

func rawDiagnosticMetadata(raw string, diagnosticClass string, hasher hash.Hash) map[string]string {
	metadata := map[string]string{
		ProgressMetadataDiagnosticKey: strings.TrimSpace(diagnosticClass),
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		metadata[ProgressMetadataRawBytesKey] = strconv.Itoa(len([]byte(trimmed)))
		metadata[ProgressMetadataRawSHA256Key] = sha256Digest(trimmed, hasher)
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

func (s *ProgressStream) publishResponseEvent(eventType string, externalEventType string, payload string, metadata map[string]string) {
	s.publisher(ProgressFragment{
		DispatchID:         strings.TrimSpace(s.req.DispatchID),
		Kind:               responseFragmentKind,
		Type:               eventType,
		Payload:            boundedPayload(payload, ProgressRetainedTextBytes),
		ProviderSessionRef: workerexecution.CloneProviderSessionMetadata(s.providerSession),
		ExternalEventType:  strings.TrimSpace(externalEventType),
		Metadata:           mergeStringMaps(baseFragmentMetadata(s.req), metadata),
	})
}

func (s *ProgressStream) publishProgressEvent(eventType string, externalEventType string, payload string, metadata map[string]string) {
	s.publisher(ProgressFragment{
		DispatchID:         strings.TrimSpace(s.req.DispatchID),
		Kind:               progressFragmentKind,
		Type:               eventType,
		Payload:            boundedPayload(payload, ProgressRetainedProgressBytes),
		ProviderSessionRef: workerexecution.CloneProviderSessionMetadata(s.providerSession),
		ExternalEventType:  strings.TrimSpace(externalEventType),
		Metadata:           mergeStringMaps(baseFragmentMetadata(s.req), metadata),
	})
}
