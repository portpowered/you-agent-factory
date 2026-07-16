package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factory/sessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

const (
	maximumBufferedRecordBytes = 256 * 1024
	maximumToolInputBytes      = 64 * 1024
	maximumSummaryBytes        = 256
)

type decoder struct {
	context adapter.DecoderContext
	pending []byte

	runStarted        bool
	messageID         string
	parentItemID      string
	sessionRef        string
	blocks            map[int]*contentBlock
	pendingCompletion *messageCompletion
	completedMessages map[string]string
	completedTools    map[string]string
}

type messageCompletion struct {
	messageID     string
	parentItemID  string
	nativeType    string
	contentBlocks []responseevents.ContentBlock
	toolSnapshots []indexedToolSnapshot
}

type indexedToolSnapshot struct {
	index int
	block responseevents.ContentBlock
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
	ParentToolUseID json.RawMessage `json:"parent_tool_use_id"`
	Event           json.RawMessage `json:"event"`
	Message         *nativeMessage  `json:"message"`
	Attempt         *int            `json:"attempt"`
	RetryDelayMS    *int64          `json:"retry_delay_ms"`
	ErrorStatus     json.RawMessage `json:"error_status"`
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

func newDecoder(input adapter.DecoderContext) *decoder {
	return &decoder{
		context: input, blocks: make(map[int]*contentBlock),
		completedMessages: make(map[string]string), completedTools: make(map[string]string),
	}
}

func (d *decoder) Observe(_ context.Context, observation adapter.Observation) (adapter.DecodeResult, error) {
	if observation.Stream == adapter.OutputStreamStderr {
		return adapter.DecodeResult{Diagnostics: []adapter.Diagnostic{{Code: "claude_stderr_ignored", Message: "Claude stderr output was omitted"}}}, nil
	}
	if observation.Stream != adapter.OutputStreamStdout || len(observation.Chunk) == 0 {
		return adapter.DecodeResult{}, nil
	}
	if len(d.pending)+len(observation.Chunk) > maximumBufferedRecordBytes {
		d.pending = nil
		return safeDiagnostic("claude_record_too_large", "Claude emitted an oversized structured record"), nil
	}
	d.pending = append(d.pending, observation.Chunk...)
	return d.drain(false)
}

func (d *decoder) Flush(_ context.Context, _ adapter.FlushContext) (adapter.DecodeResult, error) {
	result, err := d.drain(true)
	if err != nil {
		return result, err
	}
	completed, completeErr := d.publishPendingCompletion()
	return appendResult(result, completed), completeErr
}

func (d *decoder) drain(flush bool) (adapter.DecodeResult, error) {
	var result adapter.DecodeResult
	for {
		newline := bytes.IndexByte(d.pending, '\n')
		if newline < 0 {
			if !flush || len(bytes.TrimSpace(d.pending)) == 0 {
				return result, nil
			}
			record := append([]byte(nil), d.pending...)
			d.pending = nil
			decoded, err := d.decodeRecord(record)
			return appendResult(result, decoded), err
		}
		record := append([]byte(nil), d.pending[:newline]...)
		d.pending = d.pending[newline+1:]
		if len(bytes.TrimSpace(record)) == 0 {
			continue
		}
		decoded, err := d.decodeRecord(record)
		result = appendResult(result, decoded)
		if err != nil {
			return result, err
		}
	}
}

func (d *decoder) decodeRecord(raw []byte) (adapter.DecodeResult, error) {
	var envelope nativeEnvelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return safeDiagnostic("claude_malformed_record", "Claude emitted a malformed structured record"), nil
	}
	if session := providerSession(envelope.SessionID); session != nil {
		d.sessionRef = session.ID
	}
	if envelope.Type == "assistant" && envelope.Message != nil {
		return d.completeNativeMessage(*envelope.Message, optionalString(envelope.ParentToolUseID))
	}
	if envelope.Type == "system" {
		return d.decodeSystemRecord(envelope)
	}
	if envelope.Type != "stream_event" {
		if envelope.Type == "result" || envelope.Type == "user" {
			return adapter.DecodeResult{}, nil
		}
		return safeDiagnostic("claude_unknown_record", "Claude emitted an unsupported additive record"), nil
	}
	if len(envelope.Event) == 0 {
		return safeDiagnostic("claude_malformed_event", "Claude emitted a malformed stream event"), nil
	}
	return d.decodeStreamEvent(envelope.Event, optionalString(envelope.ParentToolUseID))
}

func (d *decoder) decodeSystemRecord(envelope nativeEnvelope) (adapter.DecodeResult, error) {
	switch envelope.Subtype {
	case "init", "status":
		return adapter.DecodeResult{}, nil
	case "api_retry":
		return d.retryObservation(envelope)
	case "compact_boundary":
		return d.compactionObservation()
	default:
		return safeDiagnostic("claude_unknown_system_record", "Claude emitted an unsupported additive system record"), nil
	}
}

func (d *decoder) decodeStreamEvent(raw json.RawMessage, parentItemID string) (adapter.DecodeResult, error) {
	var event nativeEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		return safeDiagnostic("claude_malformed_event", "Claude emitted a malformed stream event"), nil
	}
	switch event.Type {
	case "message_start":
		result, err := d.publishPendingCompletion()
		if err != nil {
			return result, err
		}
		if event.Message != nil {
			d.messageID = strings.TrimSpace(event.Message.ID)
		}
		if d.messageID == "" {
			d.messageID = fallbackMessageID(d.context)
		}
		d.parentItemID = parentItemID
		d.blocks = make(map[int]*contentBlock)
		return appendResult(result, d.withRunStart(adapter.DecodeResult{})), nil
	case "content_block_start":
		d.retainExplicitParent(parentItemID)
		return d.startBlock(event)
	case "content_block_delta":
		d.retainExplicitParent(parentItemID)
		return d.updateBlock(event)
	case "content_block_stop":
		d.retainExplicitParent(parentItemID)
		return d.stopBlock(event)
	case "message_stop":
		d.retainExplicitParent(parentItemID)
		return d.deferAccumulatedMessage()
	default:
		return safeDiagnostic("claude_unknown_stream_event", "Claude emitted an unsupported additive stream event"), nil
	}
}

func (d *decoder) startBlock(event nativeEvent) (adapter.DecodeResult, error) {
	if event.ContentBlock == nil || event.Index < 0 {
		return safeDiagnostic("claude_invalid_block_start", "Claude emitted an invalid content-block start"), nil
	}
	block := &contentBlock{kind: event.ContentBlock.Type, toolID: strings.TrimSpace(event.ContentBlock.ID), toolName: strings.TrimSpace(event.ContentBlock.Name)}
	d.blocks[event.Index] = block
	switch block.kind {
	case "text":
		if event.ContentBlock.Text == "" {
			return adapter.DecodeResult{}, nil
		}
		block.text.WriteString(event.ContentBlock.Text)
		return d.messageDelta(event.Index, event.ContentBlock.Text)
	case "tool_use":
		if block.toolID == "" || block.toolName == "" {
			return safeDiagnostic("claude_invalid_tool_start", "Claude emitted a tool block without stable identity"), nil
		}
		if len(event.ContentBlock.Input) > 0 && string(event.ContentBlock.Input) != "{}" {
			block.toolInput = append(block.toolInput, event.ContentBlock.Input...)
		}
		return d.toolSnapshot(event.Index, responseevents.PhaseStarted)
	default:
		return safeDiagnostic("claude_unknown_block", "Claude emitted an unsupported additive content block"), nil
	}
}

func (d *decoder) updateBlock(event nativeEvent) (adapter.DecodeResult, error) {
	block := d.blocks[event.Index]
	if block == nil || event.Delta == nil {
		return safeDiagnostic("claude_unmatched_block_delta", "Claude emitted an update for an unopened content block"), nil
	}
	switch event.Delta.Type {
	case "text_delta":
		if block.kind != "text" || event.Delta.Text == "" {
			return adapter.DecodeResult{}, nil
		}
		block.text.WriteString(event.Delta.Text)
		return d.messageDelta(event.Index, event.Delta.Text)
	case "input_json_delta":
		if block.kind != "tool_use" || event.Delta.PartialJSON == "" {
			return adapter.DecodeResult{}, nil
		}
		if len(block.toolInput)+len(event.Delta.PartialJSON) > maximumToolInputBytes {
			block.toolInput = nil
			return safeDiagnostic("claude_tool_input_too_large", "Claude tool input exceeded the safe summary boundary"), nil
		}
		block.toolInput = append(block.toolInput, event.Delta.PartialJSON...)
		summary, ok := safeJSONSummary(block.toolInput)
		if !ok || summary == block.lastSummary {
			return adapter.DecodeResult{}, nil
		}
		block.lastSummary = summary
		return d.toolDelta(event.Index, summary)
	default:
		return safeDiagnostic("claude_unknown_block_delta", "Claude emitted an unsupported additive content-block update"), nil
	}
}

func (d *decoder) stopBlock(event nativeEvent) (adapter.DecodeResult, error) {
	block := d.blocks[event.Index]
	if block == nil || block.kind != "tool_use" {
		return adapter.DecodeResult{}, nil
	}
	result, err := d.toolSnapshot(event.Index, responseevents.PhaseCompleted)
	if err != nil {
		return result, err
	}
	if len(block.toolInput) > 0 {
		if _, ok := safeJSONSummary(block.toolInput); !ok {
			result.Diagnostics = append(result.Diagnostics, adapter.Diagnostic{Code: "claude_invalid_tool_input", Message: "Claude tool input did not form a valid safe summary"})
		}
	}
	return result, nil
}

func (d *decoder) deferAccumulatedMessage() (adapter.DecodeResult, error) {
	indexes := make([]int, 0, len(d.blocks))
	for index := range d.blocks {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	blocks := make([]responseevents.ContentBlock, 0, len(indexes))
	for _, index := range indexes {
		block := d.blocks[index]
		switch block.kind {
		case "text":
			if block.text.Len() > 0 {
				blocks = append(blocks, responseevents.ContentBlock{Kind: responseevents.ContentBlockText, Text: block.text.String()})
			}
		case "tool_use":
			content := responseevents.ContentBlock{Kind: responseevents.ContentBlockToolRequest, ToolCallID: block.toolID, ToolName: block.toolName}
			if summary, ok := safeJSONSummary(block.toolInput); ok {
				content.ArgumentsSummary = json.RawMessage(summary)
			}
			blocks = append(blocks, content)
		}
	}
	if len(blocks) > 0 {
		d.pendingCompletion = &messageCompletion{messageID: d.stableMessageID(), parentItemID: d.parentItemID, nativeType: "message_stop", contentBlocks: blocks}
	}
	return adapter.DecodeResult{}, nil
}

func (d *decoder) completeNativeMessage(message nativeMessage, parentItemID string) (adapter.DecodeResult, error) {
	if strings.TrimSpace(message.Role) != "assistant" {
		return adapter.DecodeResult{}, nil
	}
	messageID := strings.TrimSpace(message.ID)
	if messageID == "" {
		messageID = d.stableMessageID()
	}
	var result adapter.DecodeResult
	if d.pendingCompletion != nil && d.pendingCompletion.messageID != messageID {
		pending, err := d.publishPendingCompletion()
		if err != nil {
			return pending, err
		}
		result = appendResult(result, pending)
	}
	blocks, tools, diagnostics := canonicalNativeBlocks(message.Content)
	if len(blocks) == 0 {
		result.Diagnostics = append(result.Diagnostics, diagnostics...)
		return result, nil
	}
	if d.pendingCompletion != nil && d.pendingCompletion.messageID == messageID {
		d.pendingCompletion = nil
	}
	d.messageID = messageID
	d.parentItemID = parentItemID
	completed, err := d.publishCompletion(messageCompletion{
		messageID: messageID, parentItemID: parentItemID, nativeType: "assistant", contentBlocks: blocks, toolSnapshots: tools,
	})
	completed.Diagnostics = append(completed.Diagnostics, diagnostics...)
	return appendResult(result, completed), err
}

func canonicalNativeBlocks(native []nativeBlock) ([]responseevents.ContentBlock, []indexedToolSnapshot, []adapter.Diagnostic) {
	blocks := make([]responseevents.ContentBlock, 0, len(native))
	tools := make([]indexedToolSnapshot, 0, len(native))
	var diagnostics []adapter.Diagnostic
	for index, block := range native {
		switch block.Type {
		case "text":
			if block.Text != "" {
				blocks = append(blocks, responseevents.ContentBlock{Kind: responseevents.ContentBlockText, Text: block.Text})
			}
		case "tool_use":
			canonical, diagnostic := canonicalNativeTool(block)
			if diagnostic != nil {
				diagnostics = append(diagnostics, *diagnostic)
				continue
			}
			blocks = append(blocks, canonical)
			tools = append(tools, indexedToolSnapshot{index: index, block: canonical})
		}
	}
	return blocks, tools, diagnostics
}

func canonicalNativeTool(block nativeBlock) (responseevents.ContentBlock, *adapter.Diagnostic) {
	toolID, toolName := strings.TrimSpace(block.ID), strings.TrimSpace(block.Name)
	if toolID == "" || toolName == "" {
		return responseevents.ContentBlock{}, &adapter.Diagnostic{Code: "claude_invalid_tool_snapshot", Message: "Claude emitted a complete tool block without stable identity"}
	}
	canonical := responseevents.ContentBlock{Kind: responseevents.ContentBlockToolRequest, ToolCallID: toolID, ToolName: toolName}
	if len(block.Input) == 0 {
		return canonical, nil
	}
	if len(block.Input) > maximumToolInputBytes {
		return canonical, &adapter.Diagnostic{Code: "claude_tool_input_too_large", Message: "Claude tool input exceeded the safe summary boundary"}
	}
	if summary, ok := safeJSONSummary(block.Input); ok {
		canonical.ArgumentsSummary = json.RawMessage(summary)
		return canonical, nil
	}
	return canonical, &adapter.Diagnostic{Code: "claude_invalid_tool_input", Message: "Claude tool input did not form a valid safe summary"}
}

func (d *decoder) publishPendingCompletion() (adapter.DecodeResult, error) {
	if d.pendingCompletion == nil {
		return adapter.DecodeResult{}, nil
	}
	completion := *d.pendingCompletion
	d.pendingCompletion = nil
	return d.publishCompletion(completion)
}

func (d *decoder) publishCompletion(completion messageCompletion) (adapter.DecodeResult, error) {
	payload, err := marshalPayload(responseevents.MessagePayload{Role: "assistant", ContentBlocks: completion.contentBlocks})
	if err != nil {
		return adapter.DecodeResult{}, err
	}
	fingerprint := completion.parentItemID + "\x00" + string(payload)
	if d.completedMessages[completion.messageID] == fingerprint {
		return adapter.DecodeResult{}, nil
	}
	result, err := d.publishToolSnapshots(completion)
	if err != nil {
		return result, err
	}
	d.completedMessages[completion.messageID] = fingerprint
	result.Drafts = append(result.Drafts, responseevents.Draft{
		Kind: responseevents.KindMessage, Phase: responseevents.PhaseCompleted, ItemID: completion.messageID, ParentItemID: completion.parentItemID, ProviderSessionRef: d.sessionRef,
		Provenance: provenance(completion.nativeType, responseevents.RepresentationSnapshot), Payload: payload,
	})
	return d.withRunStart(result), nil
}

func (d *decoder) publishToolSnapshots(completion messageCompletion) (adapter.DecodeResult, error) {
	var result adapter.DecodeResult
	for _, tool := range completion.toolSnapshots {
		body := responseevents.ToolPayload{ToolCallID: tool.block.ToolCallID, ToolName: tool.block.ToolName, Status: "completed", ArgumentsSummary: tool.block.ArgumentsSummary}
		payload, err := marshalPayload(body)
		if err != nil {
			return result, err
		}
		itemID := toolItemID(completion.messageID, tool.index)
		if d.completedTools[itemID] == string(payload) {
			continue
		}
		d.completedTools[itemID] = string(payload)
		result.Drafts = append(result.Drafts, responseevents.Draft{
			Kind: responseevents.KindTool, Phase: responseevents.PhaseCompleted, ItemID: itemID, ParentItemID: completion.messageID, ProviderSessionRef: d.sessionRef,
			Provenance: provenance(completion.nativeType, responseevents.RepresentationSnapshot), Payload: payload,
		})
	}
	return result, nil
}

func (d *decoder) messageDelta(index int, text string) (adapter.DecodeResult, error) {
	payload, err := marshalPayload(responseevents.MessageDeltaPayload{ContentBlockIndex: index, ContentBlockKind: responseevents.ContentBlockText, TextDelta: text})
	if err != nil {
		return adapter.DecodeResult{}, err
	}
	return d.withRunStart(adapter.DecodeResult{Drafts: []responseevents.Draft{{
		Kind: responseevents.KindMessage, Phase: responseevents.PhaseDelta, ItemID: d.stableMessageID(), ParentItemID: d.parentItemID, ProviderSessionRef: d.sessionRef,
		Provenance: provenance("content_block_delta", responseevents.RepresentationDelta), Payload: payload,
	}}}), nil
}

func (d *decoder) toolDelta(index int, summary string) (adapter.DecodeResult, error) {
	block := d.blocks[index]
	payload, err := marshalPayload(responseevents.ToolDeltaPayload{ToolCallID: block.toolID, OutputDelta: summary})
	if err != nil {
		return adapter.DecodeResult{}, err
	}
	return d.withRunStart(adapter.DecodeResult{Drafts: []responseevents.Draft{{
		Kind: responseevents.KindTool, Phase: responseevents.PhaseDelta, ItemID: d.toolItemID(index), ParentItemID: d.stableMessageID(), ProviderSessionRef: d.sessionRef,
		Provenance: provenance("input_json_delta", responseevents.RepresentationDelta), Payload: payload,
	}}}), nil
}

func (d *decoder) toolSnapshot(index int, phase responseevents.Phase) (adapter.DecodeResult, error) {
	block := d.blocks[index]
	body := responseevents.ToolPayload{ToolCallID: block.toolID, ToolName: block.toolName, Status: "running"}
	if phase == responseevents.PhaseCompleted {
		body.Status = "completed"
		if summary, ok := safeJSONSummary(block.toolInput); ok {
			body.ArgumentsSummary = json.RawMessage(summary)
		}
	}
	payload, err := marshalPayload(body)
	if err != nil {
		return adapter.DecodeResult{}, err
	}
	if phase == responseevents.PhaseCompleted {
		d.completedTools[d.toolItemID(index)] = string(payload)
	}
	return d.withRunStart(adapter.DecodeResult{Drafts: []responseevents.Draft{{
		Kind: responseevents.KindTool, Phase: phase, ItemID: d.toolItemID(index), ParentItemID: d.stableMessageID(), ProviderSessionRef: d.sessionRef,
		Provenance: provenance("content_block_"+phaseName(phase), responseevents.RepresentationSnapshot), Payload: payload,
	}}}), nil
}

func (d *decoder) withRunStart(result adapter.DecodeResult) adapter.DecodeResult {
	if d.runStarted {
		return result
	}
	payload, err := marshalPayload(responseevents.RunPayload{Status: "started"})
	if err != nil {
		result.Diagnostics = append(result.Diagnostics, adapter.Diagnostic{Code: "claude_run_start_failed", Message: "Claude run metadata could not be normalized"})
		return result
	}
	d.runStarted = true
	started := responseevents.Draft{RunID: d.context.RunID, DispatchID: d.context.DispatchID, Kind: responseevents.KindRun, Phase: responseevents.PhaseStarted,
		Provenance: responseevents.Provenance{Provider: "claude", NativeEventType: "message_start", Delivery: responseevents.DeliverySynthesized, Representation: responseevents.RepresentationNotification, Fidelity: responseevents.FidelityNormalized}, Payload: payload}
	result.Drafts = append([]responseevents.Draft{started}, result.Drafts...)
	return result
}

func (d *decoder) stableMessageID() string {
	if strings.TrimSpace(d.messageID) != "" {
		return d.messageID
	}
	d.messageID = fallbackMessageID(d.context)
	return d.messageID
}

func (d *decoder) retainExplicitParent(parentItemID string) {
	if parentItemID != "" {
		d.parentItemID = parentItemID
	}
}

func (d *decoder) toolItemID(index int) string {
	return toolItemID(d.stableMessageID(), index)
}

func toolItemID(messageID string, index int) string {
	return messageID + "/content-block/" + strconv.Itoa(index)
}

func fallbackMessageID(input adapter.DecoderContext) string {
	identity := strings.TrimSpace(input.DispatchID)
	if identity == "" {
		identity = strings.TrimSpace(input.RunID)
	}
	if identity == "" {
		identity = "invocation"
	}
	return "claude-message/" + identity
}

func provenance(nativeType string, representation responseevents.Representation) responseevents.Provenance {
	return responseevents.Provenance{Provider: "claude", NativeEventType: nativeType, Delivery: responseevents.DeliveryNativeStream, Representation: representation, Fidelity: responseevents.FidelityNormalized}
}

func phaseName(phase responseevents.Phase) string {
	if phase == responseevents.PhaseStarted {
		return "start"
	}
	return "stop"
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
	err := encoder.Encode(summarized)
	encoded := bytes.TrimSuffix(buffer.Bytes(), []byte("\n"))
	if err != nil {
		return "", false
	}
	if len(encoded) > maximumSummaryBytes {
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

func safeDiagnostic(code, message string) adapter.DecodeResult {
	return adapter.DecodeResult{Diagnostics: []adapter.Diagnostic{{Code: code, Message: message}}}
}

func appendResult(target, next adapter.DecodeResult) adapter.DecodeResult {
	target.Drafts = append(target.Drafts, next.Drafts...)
	target.Diagnostics = append(target.Diagnostics, next.Diagnostics...)
	return target
}

var _ adapter.Decoder = (*decoder)(nil)
