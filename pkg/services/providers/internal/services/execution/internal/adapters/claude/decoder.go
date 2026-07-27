package claude

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const (
	maxRecordBytes    = 256 * 1024
	maxToolInputBytes = 64 * 1024
	maxDetailBytes    = 1024
	maxSummaryBytes   = 256
)

type decoder struct {
	attemptID string
	pending   []byte
	flushed   bool

	runStarted        bool
	messageID         string
	parentItemID      string
	sessionID         string
	blocks            map[int]*contentBlock
	pendingCompletion *messageCompletion
	completedMessages map[string]string
	completedTools    map[string]string

	finalContent   string
	finalSessionID string
	hasResult      bool

	progress        []providers.ExecuteProgress
	declaredFailure *providers.ExecuteFailure
	declaredKnown   bool
	decodeErr       error
}

type messageCompletion struct {
	messageID    string
	parentItemID string
	nativeType   string
	textBlocks   []string
	toolIndexes  []int
}

type contentBlock struct {
	kind        string
	text        strings.Builder
	toolID      string
	toolName    string
	toolInput   []byte
	lastSummary string
}

type nativeEnvelope struct {
	Type            string          `json:"type"`
	Subtype         string          `json:"subtype"`
	SessionID       string          `json:"session_id"`
	IsError         bool            `json:"is_error"`
	Result          string          `json:"result"`
	ParentToolUseID json.RawMessage `json:"parent_tool_use_id"`
	Event           json.RawMessage `json:"event"`
	Message         *nativeMessage  `json:"message"`
}

type nativeEvent struct {
	Type         string         `json:"type"`
	Index        int            `json:"index"`
	Message      *nativeMessage `json:"message"`
	ContentBlock *nativeBlock   `json:"content_block"`
	Delta        *nativeDelta   `json:"delta"`
}

type nativeMessage struct {
	ID      string        `json:"id"`
	Role    string        `json:"role"`
	Content []nativeBlock `json:"content"`
}

type nativeBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

type nativeDelta struct {
	Type        string `json:"type"`
	Text        string `json:"text"`
	PartialJSON string `json:"partial_json"`
}

func newDecoder(attemptID string) *decoder {
	return &decoder{
		attemptID:         strings.TrimSpace(attemptID),
		blocks:            make(map[int]*contentBlock),
		completedMessages: make(map[string]string),
		completedTools:    make(map[string]string),
	}
}

func (decoder *decoder) observe(chunk []byte) error {
	if decoder.flushed {
		return errors.New("claude stream received output after finalization")
	}
	for len(chunk) > 0 {
		newline := bytes.IndexByte(chunk, '\n')
		if newline < 0 {
			if len(decoder.pending)+len(chunk) > maxRecordBytes {
				decoder.pending = nil
				decoder.markDecodeFailure("oversized_record")
				return nil
			}
			decoder.pending = append(decoder.pending, chunk...)
			return nil
		}
		if len(decoder.pending)+newline > maxRecordBytes {
			decoder.markDecodeFailure("oversized_record")
		} else {
			decoder.pending = append(decoder.pending, chunk[:newline]...)
			decoder.decodeRecord(decoder.pending)
		}
		decoder.pending = decoder.pending[:0]
		chunk = chunk[newline+1:]
	}
	return nil
}

func (decoder *decoder) flush() error {
	if decoder.flushed {
		return errors.New("claude stream finalized more than once")
	}
	decoder.flushed = true
	if len(bytes.TrimSpace(decoder.pending)) > 0 {
		decoder.decodeRecord(decoder.pending)
	}
	decoder.pending = nil
	decoder.publishPendingCompletion()
	return nil
}

func (decoder *decoder) final() (string, *providers.SessionRef, error) {
	if !decoder.flushed {
		return "", nil, errors.New("claude stream was not finalized")
	}
	if !decoder.hasResult || decoder.finalContent == "" {
		return "", nil, errors.New("claude stream did not contain a terminal result")
	}
	sessionID := strings.TrimSpace(decoder.finalSessionID)
	if sessionID == "" {
		sessionID = strings.TrimSpace(decoder.sessionID)
	}
	var session *providers.SessionRef
	if sessionID != "" {
		session = &providers.SessionRef{
			Provider: providers.IDClaude,
			Kind:     providers.SessionIDKind,
			ID:       sessionID,
		}
	}
	return decoder.finalContent, session, nil
}

func (decoder *decoder) progressFacts() []providers.ExecuteProgress {
	progress := make([]providers.ExecuteProgress, len(decoder.progress))
	for index := range decoder.progress {
		progress[index] = decoder.progress[index].Clone()
	}
	return progress
}

func (decoder *decoder) decodeRecord(raw []byte) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return
	}
	var envelope nativeEnvelope
	if json.Unmarshal(raw, &envelope) != nil {
		decoder.markDecodeFailure("malformed_json")
		return
	}
	if session := strings.TrimSpace(envelope.SessionID); session != "" {
		if decoder.sessionID == "" {
			decoder.sessionID = session
			decoder.addProgress("session.started", "started", nil)
		}
	}
	switch envelope.Type {
	case "system":
		decoder.decodeSystemRecord(envelope)
	case "assistant":
		if envelope.Message != nil {
			decoder.completeNativeMessage(*envelope.Message, optionalString(envelope.ParentToolUseID))
		}
	case "stream_event":
		if len(envelope.Event) == 0 {
			decoder.addDiagnostic("malformed_event")
			return
		}
		decoder.decodeStreamEvent(envelope.Event, optionalString(envelope.ParentToolUseID))
	case "result":
		decoder.decodeResultRecord(envelope)
	case "user":
		return
	default:
		decoder.addDiagnostic("unsupported_record")
	}
}

func (decoder *decoder) decodeSystemRecord(envelope nativeEnvelope) {
	switch envelope.Subtype {
	case "init", "status":
		return
	case "api_retry":
		decoder.addProgress("retry.updated", "Claude reported a provider API retry", map[string]string{
			"code": "api_retry",
		})
	case "compact_boundary":
		decoder.addProgress("context.compacted", "Claude compacted the conversation context", map[string]string{
			"code": "compact_boundary",
		})
	default:
		decoder.addDiagnostic("unsupported_system_record")
	}
}

func (decoder *decoder) decodeResultRecord(envelope nativeEnvelope) {
	if envelope.Subtype != "success" || envelope.IsError {
		decoder.declareFailure(classifyResultFailure(envelope))
		return
	}
	decoder.finalContent = strings.TrimSpace(envelope.Result)
	decoder.finalSessionID = strings.TrimSpace(envelope.SessionID)
	decoder.hasResult = true
}

func (decoder *decoder) decodeStreamEvent(raw json.RawMessage, parentItemID string) {
	var event nativeEvent
	if json.Unmarshal(raw, &event) != nil {
		decoder.addDiagnostic("malformed_event")
		return
	}
	switch event.Type {
	case "message_start":
		decoder.publishPendingCompletion()
		if event.Message != nil {
			decoder.messageID = strings.TrimSpace(event.Message.ID)
		}
		if decoder.messageID == "" {
			decoder.messageID = decoder.fallbackMessageID()
		}
		decoder.parentItemID = parentItemID
		decoder.blocks = make(map[int]*contentBlock)
		decoder.ensureRunStarted()
		decoder.addMessageProgress("started", "", event.Index)
	case "content_block_start":
		decoder.retainExplicitParent(parentItemID)
		decoder.startBlock(event)
	case "content_block_delta":
		decoder.retainExplicitParent(parentItemID)
		decoder.updateBlock(event)
	case "content_block_stop":
		decoder.retainExplicitParent(parentItemID)
		decoder.stopBlock(event)
	case "message_stop":
		decoder.retainExplicitParent(parentItemID)
		decoder.deferAccumulatedMessage()
	default:
		decoder.addDiagnostic("unsupported_stream_event")
	}
}

func (decoder *decoder) startBlock(event nativeEvent) {
	if event.ContentBlock == nil || event.Index < 0 {
		decoder.addDiagnostic("invalid_block_start")
		return
	}
	block := &contentBlock{
		kind:     event.ContentBlock.Type,
		toolID:   strings.TrimSpace(event.ContentBlock.ID),
		toolName: strings.TrimSpace(event.ContentBlock.Name),
	}
	decoder.blocks[event.Index] = block
	switch block.kind {
	case "text":
		if event.ContentBlock.Text != "" {
			block.text.WriteString(event.ContentBlock.Text)
			decoder.addMessageProgress("started", boundedDetail(event.ContentBlock.Text), event.Index)
		} else {
			decoder.addMessageProgress("started", "", event.Index)
		}
	case "tool_use":
		if block.toolID == "" || block.toolName == "" {
			decoder.addDiagnostic("invalid_tool_start")
			return
		}
		if len(event.ContentBlock.Input) > 0 && string(event.ContentBlock.Input) != "{}" {
			block.toolInput = append(block.toolInput, event.ContentBlock.Input...)
		}
		decoder.addToolProgress("started", boundedDetail(block.toolName), event.Index, block)
	default:
		decoder.addDiagnostic("unsupported_block")
	}
}

func (decoder *decoder) updateBlock(event nativeEvent) {
	block := decoder.blocks[event.Index]
	if block == nil || event.Delta == nil {
		decoder.addDiagnostic("unmatched_block_delta")
		return
	}
	switch event.Delta.Type {
	case "text_delta":
		if block.kind != "text" || event.Delta.Text == "" {
			return
		}
		block.text.WriteString(event.Delta.Text)
		decoder.addMessageProgress("delta", boundedDetail(event.Delta.Text), event.Index)
	case "input_json_delta":
		if block.kind != "tool_use" || event.Delta.PartialJSON == "" {
			return
		}
		if len(block.toolInput)+len(event.Delta.PartialJSON) > maxToolInputBytes {
			block.toolInput = nil
			decoder.addDiagnostic("tool_input_too_large")
			return
		}
		block.toolInput = append(block.toolInput, event.Delta.PartialJSON...)
		summary, ok := safeJSONSummary(block.toolInput)
		if !ok || summary == block.lastSummary {
			return
		}
		block.lastSummary = summary
		decoder.addToolProgress("updated", summary, event.Index, block)
	default:
		decoder.addDiagnostic("unsupported_block_delta")
	}
}

func (decoder *decoder) stopBlock(event nativeEvent) {
	block := decoder.blocks[event.Index]
	if block == nil {
		return
	}
	switch block.kind {
	case "text":
		decoder.addMessageProgress("completed", boundedDetail(block.text.String()), event.Index)
	case "tool_use":
		decoder.addToolProgress("completed", block.lastSummary, event.Index, block)
		if len(block.toolInput) > 0 {
			if _, ok := safeJSONSummary(block.toolInput); !ok {
				decoder.addDiagnostic("invalid_tool_input")
			}
		}
	}
}

func (decoder *decoder) deferAccumulatedMessage() {
	indexes := make([]int, 0, len(decoder.blocks))
	for index := range decoder.blocks {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	textBlocks := make([]string, 0, len(indexes))
	toolIndexes := make([]int, 0, len(indexes))
	for _, index := range indexes {
		block := decoder.blocks[index]
		switch block.kind {
		case "text":
			if block.text.Len() > 0 {
				textBlocks = append(textBlocks, block.text.String())
			}
		case "tool_use":
			toolIndexes = append(toolIndexes, index)
		}
	}
	if len(textBlocks) > 0 || len(toolIndexes) > 0 {
		decoder.pendingCompletion = &messageCompletion{
			messageID:    decoder.stableMessageID(),
			parentItemID: decoder.parentItemID,
			nativeType:   "message_stop",
			textBlocks:   textBlocks,
			toolIndexes:  toolIndexes,
		}
	}
}

func (decoder *decoder) completeNativeMessage(message nativeMessage, parentItemID string) {
	if strings.TrimSpace(message.Role) != "assistant" {
		return
	}
	messageID := strings.TrimSpace(message.ID)
	if messageID == "" {
		messageID = decoder.stableMessageID()
	}
	if decoder.pendingCompletion != nil && decoder.pendingCompletion.messageID != messageID {
		decoder.publishPendingCompletion()
	}
	textBlocks, toolIndexes, diagnostics := canonicalNativeBlocks(message.Content)
	for _, code := range diagnostics {
		decoder.addDiagnostic(code)
	}
	if len(textBlocks) == 0 && len(toolIndexes) == 0 {
		return
	}
	if decoder.pendingCompletion != nil && decoder.pendingCompletion.messageID == messageID {
		decoder.pendingCompletion = nil
	}
	decoder.messageID = messageID
	decoder.parentItemID = parentItemID
	decoder.publishCompletion(messageCompletion{
		messageID:    messageID,
		parentItemID: parentItemID,
		nativeType:   "assistant",
		textBlocks:   textBlocks,
		toolIndexes:  toolIndexes,
	})
}

func canonicalNativeBlocks(native []nativeBlock) ([]string, []int, []string) {
	textBlocks := make([]string, 0, len(native))
	toolIndexes := make([]int, 0, len(native))
	var diagnostics []string
	for index, block := range native {
		switch block.Type {
		case "text":
			if block.Text != "" {
				textBlocks = append(textBlocks, block.Text)
			}
		case "tool_use":
			toolID, toolName := strings.TrimSpace(block.ID), strings.TrimSpace(block.Name)
			if toolID == "" || toolName == "" {
				diagnostics = append(diagnostics, "invalid_tool_snapshot")
				continue
			}
			if len(block.Input) > maxToolInputBytes {
				diagnostics = append(diagnostics, "tool_input_too_large")
			} else if len(block.Input) > 0 {
				if _, ok := safeJSONSummary(block.Input); !ok {
					diagnostics = append(diagnostics, "invalid_tool_input")
				}
			}
			toolIndexes = append(toolIndexes, index)
		}
	}
	return textBlocks, toolIndexes, diagnostics
}

func (decoder *decoder) publishPendingCompletion() {
	if decoder.pendingCompletion == nil {
		return
	}
	completion := *decoder.pendingCompletion
	decoder.pendingCompletion = nil
	decoder.publishCompletion(completion)
}

func (decoder *decoder) publishCompletion(completion messageCompletion) {
	fingerprint := completion.parentItemID + "\x00" + strings.Join(completion.textBlocks, "\x00")
	if decoder.completedMessages[completion.messageID] == fingerprint {
		return
	}
	decoder.completedMessages[completion.messageID] = fingerprint
	decoder.ensureRunStarted()
	if len(completion.textBlocks) > 0 {
		decoder.addProgress(
			"message.completed",
			boundedDetail(strings.Join(completion.textBlocks, "\n")),
			messageMetadata(completion.messageID, completion.parentItemID, completion.nativeType),
		)
	}
	for _, index := range completion.toolIndexes {
		block := decoder.blocks[index]
		if block == nil || block.kind != "tool_use" {
			continue
		}
		summary := block.lastSummary
		if summary == "" {
			if parsed, ok := safeJSONSummary(block.toolInput); ok {
				summary = parsed
			}
		}
		decoder.addToolProgress("completed", summary, index, block)
	}
}

func (decoder *decoder) addMessageProgress(phase, detail string, blockIndex int) {
	decoder.ensureRunStarted()
	metadata := messageMetadata(decoder.stableMessageID(), decoder.parentItemID, "content_block_"+phase)
	metadata["content_block_index"] = strconv.Itoa(blockIndex)
	decoder.addProgress("message."+phase, detail, metadata)
}

func (decoder *decoder) addToolProgress(phase, detail string, blockIndex int, block *contentBlock) {
	decoder.ensureRunStarted()
	metadata := toolMetadata(decoder.stableMessageID(), blockIndex, block.toolID, block.toolName)
	decoder.addProgress("tool."+phase, detail, metadata)
}

func (decoder *decoder) ensureRunStarted() {
	if decoder.runStarted {
		return
	}
	decoder.runStarted = true
	decoder.addProgress("run.started", "started", nil)
}

func (decoder *decoder) addProgress(
	phase string,
	detail string,
	metadata map[string]string,
) {
	decoder.progress = append(decoder.progress, providers.ExecuteProgress{
		Phase:    phase,
		Detail:   detail,
		Metadata: cloneMetadata(metadata),
	})
}

func (decoder *decoder) addDiagnostic(code string) {
	decoder.addProgress("diagnostic", "Claude stream record was omitted", map[string]string{
		"code": code,
	})
}

func (decoder *decoder) markDecodeFailure(code string) {
	decoder.addDiagnostic(code)
	if decoder.decodeErr == nil {
		decoder.decodeErr = errors.New("Claude stream could not be decoded safely")
	}
}

func (decoder *decoder) declareFailure(failure providers.ExecuteFailure) {
	known := failure.Kind != providers.ExecuteFailureKindUnknown
	if decoder.declaredFailure == nil || known || !decoder.declaredKnown {
		decoder.declaredFailure = &failure
	}
	decoder.declaredKnown = decoder.declaredKnown || known
}

func classifyResultFailure(envelope nativeEnvelope) providers.ExecuteFailure {
	subtype := strings.ToLower(strings.TrimSpace(envelope.Subtype))
	message := strings.ToLower(strings.TrimSpace(envelope.Result))
	kind := providers.ExecuteFailureKindUnknown
	switch subtype {
	case "authentication_error", "permission_error":
		kind = providers.ExecuteFailureKindAuthentication
	case "invalid_request_error":
		kind = providers.ExecuteFailureKindInvalidRequest
	case "rate_limit_error", "overloaded_error":
		kind = providers.ExecuteFailureKindThrottled
	case "api_error", "server_error":
		kind = providers.ExecuteFailureKindDependency
	case "cancel", "canceled", "cancelled":
		kind = providers.ExecuteFailureKindCanceled
	}
	if kind == providers.ExecuteFailureKindUnknown {
		switch {
		case strings.Contains(message, "request timed out"),
			strings.Contains(message, "request timeout"),
			strings.Contains(message, "deadline exceeded"),
			strings.Contains(message, "timed out"):
			kind = providers.ExecuteFailureKindTimeout
		}
	}
	return providers.ExecuteFailure{
		Kind:    kind,
		Message: claudeDeclaredFailureMessage(kind),
	}
}

func claudeDeclaredFailureMessage(kind providers.ExecuteFailureKind) string {
	switch kind {
	case providers.ExecuteFailureKindAuthentication:
		return "Claude authentication failed"
	case providers.ExecuteFailureKindInvalidRequest:
		return "Claude rejected the request as invalid"
	case providers.ExecuteFailureKindThrottled:
		return "Claude is temporarily unavailable due to usage or capacity limits"
	case providers.ExecuteFailureKindTimeout:
		return "Claude request timed out"
	case providers.ExecuteFailureKindDependency:
		return "Claude encountered a temporary server error"
	case providers.ExecuteFailureKindCanceled:
		return "Claude execution was canceled"
	default:
		return "Claude returned a terminal failure"
	}
}

func (decoder *decoder) stableMessageID() string {
	if strings.TrimSpace(decoder.messageID) != "" {
		return decoder.messageID
	}
	decoder.messageID = decoder.fallbackMessageID()
	return decoder.messageID
}

func (decoder *decoder) fallbackMessageID() string {
	identity := decoder.attemptID
	if identity == "" {
		identity = "invocation"
	}
	return "claude-message/" + identity
}

func (decoder *decoder) retainExplicitParent(parentItemID string) {
	if parentItemID != "" {
		decoder.parentItemID = parentItemID
	}
}

func messageMetadata(messageID, parentItemID, nativeEvent string) map[string]string {
	metadata := map[string]string{
		"message_id":   messageID,
		"native_event": nativeEvent,
	}
	if parentItemID != "" {
		metadata["parent_item_id"] = parentItemID
	}
	return metadata
}

func toolMetadata(messageID string, blockIndex int, toolID, toolName string) map[string]string {
	return map[string]string{
		"message_id":          messageID,
		"correlation_id":      toolID,
		"tool_name":           boundedDetail(toolName),
		"content_block_index": strconv.Itoa(blockIndex),
		"item_id":             toolItemID(messageID, blockIndex),
	}
}

func toolItemID(messageID string, index int) string {
	return messageID + "/content-block/" + strconv.Itoa(index)
}

func optionalString(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func boundedDetail(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxDetailBytes {
		return value
	}
	return strings.TrimSpace(value[:maxDetailBytes])
}

func safeJSONSummary(raw []byte) (string, bool) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return "", false
	}
	summarized := summarizeJSON(value, 0)
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(summarized); err != nil {
		return "", false
	}
	encoded := bytes.TrimSuffix(buffer.Bytes(), []byte("\n"))
	if len(encoded) > maxSummaryBytes {
		encoded = boundedJSONShapeSummary(value)
	}
	return string(encoded), true
}

func boundedJSONShapeSummary(value any) []byte {
	switch typed := value.(type) {
	case map[string]any:
		return []byte(fmt.Sprintf(`{"summary":"valid tool input omitted","fieldCount":%d}`, len(typed)))
	case []any:
		return []byte(fmt.Sprintf(`{"summary":"valid tool input omitted","itemCount":%d}`, len(typed)))
	default:
		return []byte(`{"summary":"valid tool input omitted"}`)
	}
}

func summarizeJSON(value any, depth int) any {
	if depth >= 3 {
		return "<omitted>"
	}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		if len(keys) > 12 {
			keys = keys[:12]
		}
		result := make(map[string]any, len(keys))
		for _, key := range keys {
			if sensitiveKey(key) {
				result[key] = "<redacted>"
			} else {
				result[key] = summarizeJSON(typed[key], depth+1)
			}
		}
		return result
	case []any:
		if len(typed) > 8 {
			typed = typed[:8]
		}
		result := make([]any, len(typed))
		for index := range typed {
			result[index] = summarizeJSON(typed[index], depth+1)
		}
		return result
	case string:
		runes := []rune(typed)
		if len(runes) > 48 {
			return string(runes[:48]) + "…"
		}
		return typed
	default:
		return typed
	}
}

func sensitiveKey(key string) bool {
	normalized := strings.ToLower(key)
	for _, marker := range []string{"token", "secret", "password", "credential", "api_key", "apikey", "authorization"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
