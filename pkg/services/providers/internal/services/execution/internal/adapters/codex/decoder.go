package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	providers "github.com/portpowered/infinite-you/pkg/services/providers"
)

const (
	maxRecordBytes = 1024 * 1024
	maxDetailBytes = 1024
)

type decoder struct {
	pending      []byte
	discardLine  bool
	flushed      bool
	sessionID    string
	finalContent string
	progress     []providers.ExecuteProgress
}

type recordEnvelope struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id"`
	Item     json.RawMessage `json:"item"`
	Usage    *usageRecord    `json:"usage"`
}

type usageRecord struct {
	InputTokens           int64 `json:"input_tokens"`
	CachedInputTokens     int64 `json:"cached_input_tokens"`
	OutputTokens          int64 `json:"output_tokens"`
	ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
}

type itemEnvelope struct {
	ID               string             `json:"id"`
	Type             string             `json:"type"`
	Text             string             `json:"text"`
	Message          string             `json:"message"`
	Command          string             `json:"command"`
	AggregatedOutput string             `json:"aggregated_output"`
	Status           string             `json:"status"`
	Changes          []fileChangeRecord `json:"changes"`
	Server           string             `json:"server"`
	Tool             string             `json:"tool"`
	Query            string             `json:"query"`
	Items            []planItem         `json:"items"`
}

type fileChangeRecord struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type planItem struct {
	Text      string `json:"text"`
	Completed bool   `json:"completed"`
}

func newDecoder() *decoder {
	return &decoder{}
}

func (decoder *decoder) observe(chunk []byte) error {
	if decoder.flushed {
		return errors.New("codex stream received output after finalization")
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
				decoder.addDiagnostic("oversized_record")
				return nil
			}
			decoder.pending = append(decoder.pending, chunk...)
			return nil
		}
		if len(decoder.pending)+newline > maxRecordBytes {
			decoder.addDiagnostic("oversized_record")
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
		return errors.New("codex stream finalized more than once")
	}
	decoder.flushed = true
	if decoder.discardLine {
		decoder.pending = nil
		decoder.discardLine = false
		return nil
	}
	if len(bytes.TrimSpace(decoder.pending)) > 0 {
		decoder.decodeRecord(decoder.pending)
	}
	decoder.pending = nil
	return nil
}

func (decoder *decoder) final() (string, *providers.SessionRef, error) {
	if !decoder.flushed {
		return "", nil, errors.New("codex stream was not finalized")
	}
	if decoder.finalContent == "" {
		return "", nil, errors.New("codex stream did not contain a completed agent message")
	}
	var session *providers.SessionRef
	if decoder.sessionID != "" {
		session = &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       decoder.sessionID,
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
	var record recordEnvelope
	if json.Unmarshal(raw, &record) != nil {
		decoder.addDiagnostic("malformed_json")
		return
	}
	switch record.Type {
	case "thread.started":
		decoder.sessionID = strings.TrimSpace(record.ThreadID)
		if decoder.sessionID == "" {
			decoder.addDiagnostic("malformed_thread")
			return
		}
		decoder.addProgress("session.started", "started", nil)
	case "turn.started":
		decoder.addProgress("turn.started", "started", nil)
	case "turn.completed":
		if record.Usage == nil {
			decoder.addDiagnostic("malformed_usage")
			return
		}
		detail, _ := json.Marshal(record.Usage)
		decoder.addProgress("usage.updated", string(detail), nil)
		decoder.addProgress("turn.completed", "completed", nil)
	case "item.started", "item.updated", "item.completed":
		decoder.decodeItem(record.Type, record.Item)
	default:
		decoder.addDiagnostic("unsupported_event")
	}
}

func (decoder *decoder) decodeItem(nativeType string, raw json.RawMessage) {
	var item itemEnvelope
	if json.Unmarshal(raw, &item) != nil || strings.TrimSpace(item.ID) == "" {
		decoder.addDiagnostic("malformed_item")
		return
	}
	item.ID = strings.TrimSpace(item.ID)
	phase := itemPhase(nativeType, item.Status)
	metadata := map[string]string{
		"item_id":      item.ID,
		"native_event": nativeType,
	}
	switch item.Type {
	case "agent_message":
		phase = messagePhase(phase)
		detail := boundedDetail(item.Text)
		if detail == "" {
			decoder.addDiagnostic("malformed_message")
			return
		}
		decoder.addProgress("message."+phase, detail, metadata)
		if nativeType == "item.completed" {
			decoder.finalContent = strings.TrimSpace(item.Text)
		}
	case "reasoning":
		decoder.addProgress(
			"reasoning."+messagePhase(phase),
			boundedDetail(item.Text),
			metadata,
		)
	case "todo_list":
		decoder.addProgress("plan."+phase, planDetail(item.Items), metadata)
	case "file_change":
		decoder.addProgress("file_change."+phase, fileChangeDetail(item.Changes), metadata)
	case "command_execution":
		decoder.addProgress("tool."+phase, boundedDetail(
			firstNonEmpty(item.AggregatedOutput, item.Command),
		), toolMetadata(metadata, item.ID, "command_execution"))
	case "mcp_tool_call", "collab_tool_call":
		name := strings.Trim(strings.TrimSpace(item.Server)+"/"+strings.TrimSpace(item.Tool), "/")
		decoder.addProgress("tool."+phase, boundedDetail(item.Message),
			toolMetadata(metadata, item.ID, firstNonEmpty(name, item.Type)))
	case "web_search":
		decoder.addProgress("tool."+phase, boundedDetail(item.Query),
			toolMetadata(metadata, item.ID, "web_search"))
	default:
		decoder.addDiagnostic("unsupported_item")
	}
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
	decoder.addProgress("diagnostic", "Codex stream record was omitted", map[string]string{
		"code": code,
	})
}

func itemPhase(nativeType string, status string) string {
	switch nativeType {
	case "item.started":
		return "started"
	case "item.updated":
		return "updated"
	}
	switch strings.TrimSpace(status) {
	case "failed":
		return "failed"
	case "declined":
		return "canceled"
	default:
		return "completed"
	}
}

func messagePhase(phase string) string {
	if phase == "updated" {
		return "delta"
	}
	return phase
}

func toolMetadata(base map[string]string, id string, name string) map[string]string {
	metadata := cloneMetadata(base)
	metadata["correlation_id"] = id
	metadata["tool_name"] = boundedDetail(name)
	return metadata
}

func planDetail(items []planItem) string {
	parts := make([]string, 0, len(items))
	for _, item := range items {
		status := "pending"
		if item.Completed {
			status = "completed"
		}
		parts = append(parts, status+":"+strings.TrimSpace(item.Text))
	}
	return boundedDetail(strings.Join(parts, "; "))
}

func fileChangeDetail(changes []fileChangeRecord) string {
	parts := make([]string, 0, len(changes))
	for _, change := range changes {
		if change.Path != "" || change.Kind != "" {
			parts = append(parts, strings.TrimSpace(change.Kind)+" "+strings.TrimSpace(change.Path))
		}
	}
	return boundedDetail(strings.Join(parts, "; "))
}

func boundedDetail(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= maxDetailBytes {
		return value
	}
	return strings.TrimSpace(value[:maxDetailBytes])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
