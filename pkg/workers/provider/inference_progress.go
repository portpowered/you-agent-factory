package provider

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"hash"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	cursorpkg "github.com/portpowered/infinite-you/pkg/workers/provider/cursor"
)

const (
	ProgressFragmentKind         = "PROGRESS_FRAGMENT"
	ResponseFragmentKind         = "RESPONSE_FRAGMENT"
	CompletedFragmentKind        = "STREAM_COMPLETED"
	FailedFragmentKind           = "STREAM_FAILED"
	NormalizedEventTypeUnknown   = "UNKNOWN"
	NormalizedEventTypeStarted   = "STARTED"
	NormalizedEventTypeProgress  = "PROGRESS"
	NormalizedEventTypeTextDelta = "TEXT_DELTA"
	NormalizedEventTypeFinalText = "FINAL_TEXT"
	NormalizedEventTypeFailed    = "FAILED"
	NormalizedEventTypeCanceled  = "CANCELED"
)

const (
	codexRetainedTextBytes      = 4096
	codexRetainedProgressBytes  = 1024
	codexMetadataRunnerIDKey    = "runner_id"
	codexMetadataWorkIDKey      = "work_id"
	codexMetadataWorkstationKey = "workstation_name"
	codexMetadataTextBytesKey   = "text_bytes"
	codexMetadataTruncatedKey   = "payload_truncated"
	codexMetadataRawBytesKey    = "raw_bytes"
	codexMetadataRawSHA256Key   = "raw_sha256"
	codexMetadataDiagnosticKey  = "diagnostic_class"
)

const (
	codexDiagnosticUnknownEvent  = "unknown_event"
	codexDiagnosticMalformedJSON = "malformed_json"
	codexDiagnosticIncompleteSSE = "incomplete_event_stream"
)

// InferenceProgressFragment is the provider-boundary shape for transient internal
// session progress that must not enter canonical factory event history.
type InferenceProgressFragment struct {
	DispatchID         string
	Kind               string
	Type               string
	Payload            string
	ProviderSessionRef *interfaces.ProviderSessionMetadata
	ExternalEventType  string
	Metadata           map[string]string
}

// InferenceProgressPublisher receives provider progress fragments for one live
// Factory Session internal response stream.
type InferenceProgressPublisher func(fragment InferenceProgressFragment)

// ProgressFragment builds one ordered progress fragment for a dispatch.
func ProgressFragment(dispatchID string, providerSession *interfaces.ProviderSessionMetadata, payload string) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               ProgressFragmentKind,
		Type:               NormalizedEventTypeProgress,
		Payload:            payload,
		ProviderSessionRef: interfaces.CloneProviderSessionMetadata(providerSession),
	}
}

// ResponseFragment builds one ordered response fragment for a dispatch.
func ResponseFragment(dispatchID string, providerSession *interfaces.ProviderSessionMetadata, payload string) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               ResponseFragmentKind,
		Type:               NormalizedEventTypeTextDelta,
		Payload:            payload,
		ProviderSessionRef: interfaces.CloneProviderSessionMetadata(providerSession),
	}
}

// CompletedFragment builds one terminal completion marker for a dispatch.
func CompletedFragment(dispatchID string, providerSession *interfaces.ProviderSessionMetadata) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               CompletedFragmentKind,
		ProviderSessionRef: interfaces.CloneProviderSessionMetadata(providerSession),
	}
}

// FailedFragment builds one terminal failure marker for a dispatch.
func FailedFragment(dispatchID string, providerSession *interfaces.ProviderSessionMetadata, payload string) InferenceProgressFragment {
	return InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(dispatchID),
		Kind:               FailedFragmentKind,
		Payload:            payload,
		ProviderSessionRef: interfaces.CloneProviderSessionMetadata(providerSession),
	}
}

// InferenceProgressPublishingCommandRunner publishes internal response-stream
// fragments while provider subprocess stdout/stderr grow.
type InferenceProgressPublishingCommandRunner struct {
	Publisher InferenceProgressPublisher
	Logger    logging.Logger
}

// Run executes the provider subprocess and publishes incremental stdout/stderr
// fragments into the configured internal session response stream.
func (r InferenceProgressPublishingCommandRunner) Run(ctx context.Context, req CommandRequest) (CommandResult, error) {
	if r.Publisher == nil {
		return workerprocess.ExecCommandRunner{}.Run(ctx, req)
	}
	dispatchID := strings.TrimSpace(req.DispatchID)
	var cursorStream *cursorpkg.StreamParser
	if strings.TrimSpace(req.Command) == string(interfaces.ModelProviderCursor) {
		cursorStream = cursorpkg.NewStreamParser(req.Command, func(fragment cursorpkg.StreamFragment) {
			switch fragment.Kind {
			case cursorpkg.StreamFragmentKindResponse:
				r.Publisher(ResponseFragment(dispatchID, fragment.ProviderSession, fragment.Payload))
			case cursorpkg.StreamFragmentKindProgress:
				r.Publisher(ProgressFragment(dispatchID, fragment.ProviderSession, fragment.Payload))
			}
		})
	}
	normalizer := newCommandOutputNormalizer(req, r.Publisher)
	observer := func(stream string, chunk []byte) {
		if len(chunk) == 0 {
			return
		}
		if cursorStream != nil && stream == workerprocess.OutputStreamStdout {
			cursorStream.Consume(chunk)
			return
		}
		if normalizer != nil && normalizer.Observe(stream, chunk) {
			return
		}
		payload := string(chunk)
		switch stream {
		case workerprocess.OutputStreamStdout:
			r.Publisher(ResponseFragment(dispatchID, nil, payload))
		case workerprocess.OutputStreamStderr:
			r.Publisher(ProgressFragment(dispatchID, nil, payload))
		}
	}
	result, err := workerprocess.StreamingExecCommandRunner{
		Observer: observer,
		Logger:   logging.EnsureLogger(r.Logger),
	}.Run(ctx, req)
	if cursorStream != nil {
		cursorStream.Flush()
	}
	if normalizer != nil {
		normalizer.Flush()
	}
	return result, err
}

type commandOutputNormalizer interface {
	Observe(stream string, chunk []byte) bool
	Flush()
}

func newCommandOutputNormalizer(req CommandRequest, publisher InferenceProgressPublisher) commandOutputNormalizer {
	if !isCodexCommand(req.Command) {
		return nil
	}
	return &codexCommandOutputNormalizer{
		req:       interfaces.CloneSubprocessExecutionRequest(req),
		publisher: publisher,
		hasher:    sha256.New(),
	}
}

type codexCommandOutputNormalizer struct {
	req       CommandRequest
	publisher InferenceProgressPublisher

	stdoutBuffer string
	stderrBuffer string

	pendingSSEEvent string
	pendingSSEData  []string
	providerSession *interfaces.ProviderSessionMetadata
	hasher          hash.Hash
}

func (n *codexCommandOutputNormalizer) Observe(stream string, chunk []byte) bool {
	switch stream {
	case workerprocess.OutputStreamStdout:
		n.stdoutBuffer += string(chunk)
		n.drainLines(&n.stdoutBuffer, n.handleStdoutLine)
	case workerprocess.OutputStreamStderr:
		n.stderrBuffer += string(chunk)
		n.drainLines(&n.stderrBuffer, n.handleStderrLine)
	default:
		return false
	}
	return true
}

func (n *codexCommandOutputNormalizer) Flush() {
	if trimmed := strings.TrimSpace(n.stdoutBuffer); trimmed != "" {
		n.handleStdoutLine(trimmed)
	}
	if trimmed := strings.TrimSpace(n.stderrBuffer); trimmed != "" {
		n.handleStderrLine(trimmed)
	}
	n.flushPendingSSEEvent()
}

func (n *codexCommandOutputNormalizer) drainLines(buffer *string, handle func(string)) {
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

func (n *codexCommandOutputNormalizer) handleStdoutLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		n.flushPendingSSEEvent()
		return
	}
	if strings.HasPrefix(trimmed, "event:") {
		n.flushPendingSSEEvent()
		n.pendingSSEEvent = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
		return
	}
	if strings.HasPrefix(trimmed, "data:") {
		n.pendingSSEData = append(n.pendingSSEData, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
		return
	}
	n.flushPendingSSEEvent()
	if n.publishStructuredCodexEvent(trimmed, "") {
		return
	}
	n.publishResponseEvent(NormalizedEventTypeTextDelta, "", trimmed, nil)
}

func (n *codexCommandOutputNormalizer) handleStderrLine(line string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}
	if n.publishStructuredCodexEvent(trimmed, "") {
		return
	}
	n.publishProgressEvent(NormalizedEventTypeProgress, "", trimmed, nil)
}

func (n *codexCommandOutputNormalizer) publishStructuredCodexEvent(raw string, fallbackEventType string) bool {
	normalized, status := normalizeCodexStructuredEvent(raw, fallbackEventType, n.hasher)
	if status == codexStructuredEventStatusNotStructured {
		return false
	}
	if normalized.ProviderSessionRef != nil {
		n.providerSession = interfaces.CloneProviderSessionMetadata(normalized.ProviderSessionRef)
	}
	if normalized.ProviderSessionRef == nil {
		normalized.ProviderSessionRef = interfaces.CloneProviderSessionMetadata(n.providerSession)
	}
	normalized.DispatchID = strings.TrimSpace(n.req.DispatchID)
	normalized.Metadata = mergeStringMaps(baseFragmentMetadata(n.req), normalized.Metadata)
	n.publisher(normalized)
	return true
}

func (n *codexCommandOutputNormalizer) flushPendingSSEEvent() {
	if n.pendingSSEEvent == "" && len(n.pendingSSEData) == 0 {
		return
	}
	raw := strings.Join(n.pendingSSEData, "\n")
	n.publishStructuredCodexEvent(raw, strings.TrimSpace(n.pendingSSEEvent))
	n.pendingSSEEvent = ""
	n.pendingSSEData = nil
}

type codexStructuredEventStatus string

const (
	codexStructuredEventStatusNotStructured codexStructuredEventStatus = "NOT_STRUCTURED"
	codexStructuredEventStatusNormalized    codexStructuredEventStatus = "NORMALIZED"
	codexStructuredEventStatusMalformed     codexStructuredEventStatus = "MALFORMED"
)

func normalizeCodexStructuredEvent(raw string, fallbackEventType string, hasher hash.Hash) (InferenceProgressFragment, codexStructuredEventStatus) {
	trimmedRaw := strings.TrimSpace(raw)
	trimmedFallback := strings.TrimSpace(fallbackEventType)
	if trimmedRaw == "" && trimmedFallback == "" {
		return InferenceProgressFragment{}, codexStructuredEventStatusNotStructured
	}
	if !looksLikeStructuredCodexPayload(trimmedRaw, trimmedFallback) {
		return InferenceProgressFragment{}, codexStructuredEventStatusNotStructured
	}
	var payload map[string]any
	if trimmedRaw == "" {
		return malformedCodexStructuredEvent(trimmedRaw, trimmedFallback, codexDiagnosticIncompleteSSE, hasher), codexStructuredEventStatusMalformed
	}
	if err := json.Unmarshal([]byte(trimmedRaw), &payload); err != nil {
		return malformedCodexStructuredEvent(trimmedRaw, trimmedFallback, codexDiagnosticMalformedJSON, hasher), codexStructuredEventStatusMalformed
	}
	externalEventType := firstNonEmptyString(
		stringValue(payload["event"]),
		stringValue(payload["type"]),
		trimmedFallback,
	)
	normalizedType, kind := classifyCodexStructuredEvent(externalEventType)
	fragment := InferenceProgressFragment{
		Kind:               kind,
		Type:               normalizedType,
		ExternalEventType:  externalEventType,
		ProviderSessionRef: providerSessionFromStructuredCodexEvent(payload),
		Metadata:           codexStructuredEventMetadata(payload),
	}
	switch normalizedType {
	case NormalizedEventTypeStarted:
		original := firstNonEmptyString(
			stringValue(payload["message"]),
			externalEventType,
		)
		fragment.Payload = boundedCodexPayload(original, codexRetainedProgressBytes)
		fragment.Metadata = annotateBoundedPayloadMetadata(fragment.Metadata, original, fragment.Payload)
	case NormalizedEventTypeProgress:
		original := firstNonEmptyString(
			stringValue(payload["message"]),
			stringValue(payload["status"]),
			externalEventType,
		)
		fragment.Payload = boundedCodexPayload(original, codexRetainedProgressBytes)
		fragment.Metadata = annotateBoundedPayloadMetadata(fragment.Metadata, original, fragment.Payload)
	case NormalizedEventTypeTextDelta, NormalizedEventTypeFinalText:
		original := extractCodexEventText(payload)
		fragment.Payload = boundedCodexPayload(original, codexRetainedTextBytes)
		fragment.Metadata = annotateBoundedPayloadMetadata(fragment.Metadata, original, fragment.Payload)
	case NormalizedEventTypeFailed, NormalizedEventTypeCanceled:
		original := firstNonEmptyString(
			stringValue(payload["message"]),
			stringValue(payload["error"]),
			stringValue(payload["status"]),
			externalEventType,
		)
		fragment.Payload = boundedCodexPayload(original, codexRetainedProgressBytes)
		fragment.Metadata = annotateBoundedPayloadMetadata(fragment.Metadata, original, fragment.Payload)
	default:
		fragment.Payload = "codex event omitted"
		fragment.Metadata = mergeStringMaps(fragment.Metadata, codexRawDiagnosticMetadata(trimmedRaw, codexDiagnosticUnknownEvent, hasher))
	}
	return fragment, codexStructuredEventStatusNormalized
}

func classifyCodexStructuredEvent(externalEventType string) (string, string) {
	normalized := strings.ToLower(strings.TrimSpace(externalEventType))
	switch normalized {
	case "session.created", "response.created", "response.started", "turn.started":
		return NormalizedEventTypeStarted, ProgressFragmentKind
	case "response.output_text.delta", "response.delta", "message.delta", "output_text.delta":
		return NormalizedEventTypeTextDelta, ResponseFragmentKind
	case "response.completed", "response.output_text.done", "message.completed", "output.completed":
		return NormalizedEventTypeFinalText, ResponseFragmentKind
	case "response.failed", "response.error", "error":
		return NormalizedEventTypeFailed, ProgressFragmentKind
	case "response.canceled", "response.cancelled", "session.canceled", "session.cancelled":
		return NormalizedEventTypeCanceled, ProgressFragmentKind
	case "response.progress", "progress", "response.updated":
		return NormalizedEventTypeProgress, ProgressFragmentKind
	}
	switch {
	case strings.Contains(normalized, "cancel"):
		return NormalizedEventTypeCanceled, ProgressFragmentKind
	case strings.Contains(normalized, "fail"), strings.Contains(normalized, "error"):
		return NormalizedEventTypeFailed, ProgressFragmentKind
	case strings.Contains(normalized, "delta"):
		return NormalizedEventTypeTextDelta, ResponseFragmentKind
	case strings.Contains(normalized, "complete"), strings.Contains(normalized, "final"), strings.Contains(normalized, "done"):
		return NormalizedEventTypeFinalText, ResponseFragmentKind
	case strings.Contains(normalized, "start"), strings.Contains(normalized, "created"), strings.Contains(normalized, "begin"):
		return NormalizedEventTypeStarted, ProgressFragmentKind
	case strings.Contains(normalized, "progress"), strings.Contains(normalized, "update"):
		return NormalizedEventTypeProgress, ProgressFragmentKind
	default:
		return NormalizedEventTypeUnknown, ProgressFragmentKind
	}
}

func providerSessionFromStructuredCodexEvent(payload map[string]any) *interfaces.ProviderSessionMetadata {
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
		return &interfaces.ProviderSessionMetadata{
			Provider: interfaces.CanonicalProviderSessionProvider(string(interfaces.ModelProviderCodex)),
			Kind:     candidate.kind,
			ID:       strings.TrimSpace(candidate.value),
		}
	}
	return nil
}

func codexStructuredEventMetadata(payload map[string]any) map[string]string {
	metadata := map[string]string{}
	for _, key := range []string{"session_id", "response_id", "conversation_id"} {
		if value := stringValue(payload[key]); value != "" {
			metadata[key] = value
		}
	}
	if text := extractCodexEventText(payload); text != "" {
		metadata[codexMetadataTextBytesKey] = strconv.Itoa(len([]byte(text)))
	}
	if len(metadata) == 0 {
		return nil
	}
	return metadata
}

func extractCodexEventText(payload map[string]any) string {
	if delta := stringValue(payload["delta"]); strings.TrimSpace(delta) != "" {
		return delta
	}
	if text := stringValue(payload["text"]); strings.TrimSpace(text) != "" {
		return text
	}
	return strings.TrimSpace(collectCodexText(payload))
}

func collectCodexText(value any) string {
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
				if text := collectCodexText(nested); text != "" {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := collectCodexText(item); text != "" {
				parts = append(parts, text)
			}
		}
		return strings.Join(parts, "")
	default:
		return ""
	}
}

func boundedCodexPayload(payload string, limit int) string {
	trimmed := strings.TrimSpace(payload)
	if limit <= 0 || len([]byte(trimmed)) <= limit {
		return trimmed
	}
	bytes := []byte(trimmed)
	return strings.TrimSpace(string(bytes[:limit]))
}

func baseFragmentMetadata(req CommandRequest) map[string]string {
	metadata := map[string]string{
		codexMetadataRunnerIDKey: string(interfaces.ModelProviderCodex),
	}
	if workID := primaryWorkID(req.Execution.WorkIDs); workID != "" {
		metadata[codexMetadataWorkIDKey] = workID
	}
	if workstation := strings.TrimSpace(req.WorkstationName); workstation != "" {
		metadata[codexMetadataWorkstationKey] = workstation
	}
	return metadata
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

func (n *codexCommandOutputNormalizer) publishResponseEvent(eventType string, externalEventType string, payload string, metadata map[string]string) {
	n.publisher(InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(n.req.DispatchID),
		Kind:               ResponseFragmentKind,
		Type:               eventType,
		Payload:            boundedCodexPayload(payload, codexRetainedTextBytes),
		ProviderSessionRef: interfaces.CloneProviderSessionMetadata(n.providerSession),
		ExternalEventType:  strings.TrimSpace(externalEventType),
		Metadata:           mergeStringMaps(baseFragmentMetadata(n.req), metadata),
	})
}

func (n *codexCommandOutputNormalizer) publishProgressEvent(eventType string, externalEventType string, payload string, metadata map[string]string) {
	n.publisher(InferenceProgressFragment{
		DispatchID:         strings.TrimSpace(n.req.DispatchID),
		Kind:               ProgressFragmentKind,
		Type:               eventType,
		Payload:            boundedCodexPayload(payload, codexRetainedProgressBytes),
		ProviderSessionRef: interfaces.CloneProviderSessionMetadata(n.providerSession),
		ExternalEventType:  strings.TrimSpace(externalEventType),
		Metadata:           mergeStringMaps(baseFragmentMetadata(n.req), metadata),
	})
}

// NewInferenceProgressPublishingCommandRunner constructs a provider command
// runner that publishes internal response-stream fragments during subprocess IO.
func NewInferenceProgressPublishingCommandRunner(
	publisher InferenceProgressPublisher,
	logger logging.Logger,
) CommandRunner {
	if publisher == nil {
		return workerprocess.ExecCommandRunner{}
	}
	return InferenceProgressPublishingCommandRunner{
		Publisher: publisher,
		Logger:    logger,
	}
}

func isCodexCommand(command string) bool {
	base := filepath.Base(strings.ReplaceAll(strings.TrimSpace(command), `\`, "/"))
	extension := strings.ToLower(filepath.Ext(base))
	if extension == ".exe" || extension == ".cmd" || extension == ".bat" {
		base = strings.TrimSuffix(base, filepath.Ext(base))
	}
	return strings.EqualFold(base, string(interfaces.ModelProviderCodex))
}
