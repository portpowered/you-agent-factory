package opencode

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const (
	maxRecordBytes = 1024 * 1024
	maxDetailBytes = 1024
)

type decoder struct {
	mode Mode

	pending     []byte
	discardLine bool
	flushed     bool

	runStarted bool
	sessionID  string

	deltaContent      strings.Builder
	snapshotContent   strings.Builder
	seenSnapshotParts map[string]struct{}
	hasSnapshot       bool
	finalContent      string

	finalOnlyStdout []byte

	progress        []providers.ExecuteProgress
	declaredFailure *providers.ExecuteFailure
	decodeErr       error
}

func newDecoder(mode Mode) *decoder {
	if mode == "" {
		mode = ModeStructured
	}
	return &decoder{
		mode:              mode,
		seenSnapshotParts: make(map[string]struct{}),
	}
}

func (decoder *decoder) observe(chunk []byte) error {
	if decoder.flushed {
		return errors.New("opencode stream received output after finalization")
	}
	if decoder.mode == ModeFinalOnly {
		decoder.finalOnlyStdout = append(decoder.finalOnlyStdout, chunk...)
		return nil
	}
	for len(chunk) > 0 {
		if decoder.discardLine {
			newline := bytes.IndexByte(chunk, '\n')
			if newline < 0 {
				return nil
			}
			decoder.discardLine = false
			chunk = chunk[newline+1:]
			continue
		}
		newline := bytes.IndexByte(chunk, '\n')
		if newline < 0 {
			if len(decoder.pending)+len(chunk) > maxRecordBytes {
				decoder.pending = nil
				decoder.discardLine = true
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
		return errors.New("opencode stream finalized more than once")
	}
	decoder.flushed = true
	if decoder.mode == ModeFinalOnly {
		return decoder.flushFinalOnly()
	}
	if decoder.discardLine {
		decoder.pending = nil
		decoder.discardLine = false
		return nil
	}
	if len(bytes.TrimSpace(decoder.pending)) > 0 {
		decoder.decodeRecord(decoder.pending)
	}
	decoder.pending = nil
	decoder.finalContent = decoder.authoritativeContent()
	if decoder.runStarted {
		decoder.addProgress("run.completed", "completed", nil)
	}
	return nil
}

func (decoder *decoder) flushFinalOnly() error {
	if !utf8.Valid(decoder.finalOnlyStdout) {
		return errors.New("opencode final-only output was not valid utf-8")
	}
	content := strings.TrimSpace(string(decoder.finalOnlyStdout))
	if content == "" {
		return errors.New("opencode final-only output did not contain an authoritative response")
	}
	decoder.finalContent = content
	decoder.addProgress("run.started", "started", nil)
	decoder.addProgress(
		"message.completed",
		boundedDetail(content),
		messageMetadata("final-only"),
	)
	decoder.addProgress("run.completed", "completed", nil)
	return nil
}

func (decoder *decoder) final() (string, *providers.SessionRef, error) {
	if !decoder.flushed {
		return "", nil, errors.New("opencode stream was not finalized")
	}
	content := strings.TrimSpace(decoder.finalContent)
	if content == "" {
		content = strings.TrimSpace(decoder.authoritativeContent())
	}
	if content == "" {
		return "", nil, errMissingAuthoritativeResponse
	}
	var session *providers.SessionRef
	if validCorrelation(decoder.sessionID) {
		session = &providers.SessionRef{
			Provider: providers.IDOpenCode,
			Kind:     providers.SessionIDKind,
			ID:       decoder.sessionID,
		}
	}
	return content, session, nil
}

func (decoder *decoder) failureSessionRef() *providers.SessionRef {
	if !validCorrelation(decoder.sessionID) {
		return nil
	}
	return &providers.SessionRef{
		Provider: providers.IDOpenCode,
		Kind:     providers.SessionIDKind,
		ID:       decoder.sessionID,
	}
}

func (decoder *decoder) authoritativeContent() string {
	if decoder.hasSnapshot {
		return decoder.snapshotContent.String()
	}
	return decoder.deltaContent.String()
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
	record, err := decodeStructuredRecord(raw)
	if err != nil || record.Type == "" {
		decoder.markDecodeFailure("malformed_record")
		return
	}
	if sessionID := recordSessionID(record); sessionID != "" {
		decoder.sessionID = sessionID
	}
	switch record.Type {
	case "step_start":
		decoder.ensureRunStarted("step_start")
	case "text":
		decoder.decodeText(record)
	case "reasoning":
		decoder.decodeReasoning(record)
	case "tool_use":
		decoder.decodeTool(record)
	case "step_finish":
		decoder.decodeUsage(record)
	case "error":
		decoder.decodeError(record)
	default:
		if strings.TrimSpace(record.Type) != "" {
			decoder.addDiagnostic("unknown_record")
		}
	}
}

func (decoder *decoder) decodeText(record structuredRecord) {
	if !validCorrelation(record.Part.ID) || strings.TrimSpace(record.Part.Text) == "" {
		decoder.addDiagnostic("invalid_text_record")
		return
	}
	text := boundedPublishedText(record.Part.Text)
	metadata := messageMetadata(record.Part.ID)
	if validCorrelation(record.Part.MessageID) {
		metadata["parent_item_id"] = record.Part.MessageID
	}
	if record.Part.Time.End != nil {
		decoder.hasSnapshot = true
		partID := strings.TrimSpace(record.Part.ID)
		if partID != "" {
			if _, seen := decoder.seenSnapshotParts[partID]; seen {
				return
			}
			decoder.seenSnapshotParts[partID] = struct{}{}
		}
		decoder.snapshotContent.WriteString(text)
		decoder.ensureRunStarted("text")
		decoder.addProgress("message.completed", boundedDetail(text), metadata)
		return
	}
	if !decoder.hasSnapshot {
		decoder.deltaContent.WriteString(text)
	}
	decoder.ensureRunStarted("text")
	decoder.addProgress("message.delta", boundedDetail(text), metadata)
}

func (decoder *decoder) decodeReasoning(record structuredRecord) {
	if !validCorrelation(record.Part.ID) || strings.TrimSpace(record.Part.Text) == "" {
		decoder.addDiagnostic("invalid_reasoning_record")
		return
	}
	text := boundedPublishedText(record.Part.Text)
	metadata := map[string]string{"item_id": record.Part.ID}
	if record.Part.Time.End != nil {
		decoder.ensureRunStarted("reasoning")
		decoder.addProgress("reasoning.completed", boundedDetail(text), metadata)
		return
	}
	decoder.ensureRunStarted("reasoning")
	decoder.addProgress("reasoning.delta", boundedDetail(text), metadata)
}

func (decoder *decoder) decodeTool(record structuredRecord) {
	if !validCorrelation(record.Part.ID) ||
		!validCorrelation(record.Part.CallID) ||
		strings.TrimSpace(record.Part.Tool) == "" {
		decoder.addDiagnostic("invalid_tool_record")
		return
	}
	status := strings.ToLower(strings.TrimSpace(record.Part.State.Status))
	detail := fmt.Sprintf("%s %s", boundedName(record.Part.Tool), status)
	metadata := toolMetadata(record.Part.CallID, boundedName(record.Part.Tool))
	metadata["item_id"] = record.Part.ID
	if validCorrelation(record.Part.MessageID) {
		metadata["parent_item_id"] = record.Part.MessageID
	}
	decoder.ensureRunStarted("tool_use")
	switch status {
	case "completed":
		decoder.addProgress("tool.started", detail, metadata)
		decoder.addProgress("tool.completed", detail, metadata)
	case "error", "failed":
		decoder.addProgress("tool.started", detail, metadata)
		decoder.addProgress("tool.failed", detail, metadata)
	case "pending", "running", "":
		decoder.addProgress("tool.started", detail, metadata)
	default:
		decoder.addDiagnostic("invalid_tool_status")
	}
}

func (decoder *decoder) decodeUsage(record structuredRecord) {
	usage := record.Part.Tokens
	if usage.Input < 0 || usage.Output < 0 || usage.Reasoning < 0 {
		decoder.addDiagnostic("invalid_usage_record")
		return
	}
	metadata := map[string]string{
		"input_tokens":    fmt.Sprintf("%d", usage.Input),
		"output_tokens":   fmt.Sprintf("%d", usage.Output),
		"reasoning_tokens": fmt.Sprintf("%d", usage.Reasoning),
	}
	if validCorrelation(record.Part.ID) {
		metadata["item_id"] = record.Part.ID
	}
	decoder.ensureRunStarted("step_finish")
	decoder.addProgress("usage.updated", "token usage updated", metadata)
}

func (decoder *decoder) decodeError(record structuredRecord) {
	kind := executeFailureKindFromStructuredError(record.Error)
	message := openCodeDeclaredFailureMessage(kind)
	decoder.declareFailure(providers.ExecuteFailure{
		Kind:    kind,
		Message: message,
	})
	decoder.ensureRunStarted("error")
	decoder.addProgress("error.reported", message, nil)
}

func (decoder *decoder) ensureRunStarted(nativeType string) {
	if decoder.runStarted {
		return
	}
	decoder.runStarted = true
	decoder.addProgress("run.started", "started", map[string]string{
		"native_type": nativeType,
	})
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
	decoder.addProgress("diagnostic", "OpenCode stream record was omitted", map[string]string{
		"code": code,
	})
}

func (decoder *decoder) markDecodeFailure(code string) {
	decoder.addDiagnostic(code)
	if decoder.decodeErr == nil {
		decoder.decodeErr = errors.New("opencode stream could not be decoded safely")
	}
}

func (decoder *decoder) declareFailure(failure providers.ExecuteFailure) {
	clone := failure.Clone()
	decoder.declaredFailure = &clone
}

func messageMetadata(messageID string) map[string]string {
	return map[string]string{"message_id": messageID}
}

func toolMetadata(correlationID, toolName string) map[string]string {
	return map[string]string{
		"correlation_id": correlationID,
		"tool_name":      toolName,
	}
}

func boundedName(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > maxCorrelationLength {
		return value[:maxCorrelationLength]
	}
	return value
}

func boundedDetail(value string) string {
	return boundedText(strings.TrimSpace(value), maxDetailBytes)
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
