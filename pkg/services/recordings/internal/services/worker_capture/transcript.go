package worker_capture

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// WorkerTranscriptEntry is a presentation of captured public Worker content.
// It has no native file, private reasoning, or execution authority fields.
type WorkerTranscriptEntry struct {
	Type                                                   string
	Order                                                  int
	Text, Summary, Name, CallID, Arguments, Output, Status *string
	Timestamp                                              *time.Time
	TurnIndex                                              *int
}

type transcriptItem struct {
	sourceType, source, run, turn, attempt, item, part string
	anonymous                                          uint64
}

func (key transcriptItem) withPart(part string) transcriptItem { key.part = part; return key }

type transcriptSlot struct {
	entry WorkerTranscriptEntry
}

// ProjectTranscript reduces the page's captured snapshots and deltas.
// Snapshot replacement preserves the item's first-observed chronological order.
// This pure projection depends only on detached capture data, never a registry.
// Callers collect all committed pages before projecting a complete transcript.
func (page WorkerCapturedActivityPage) ProjectTranscript() ([]WorkerTranscriptEntry, error) {
	projection := transcriptProjection{slots: make(map[transcriptItem]int)}
	for _, record := range page.Records {
		var draft workers.Draft
		if json.Unmarshal(record.Record.Payload, &draft) != nil {
			return nil, fmt.Errorf("decode captured Worker draft")
		}
		if err := projection.apply(draft, record); err != nil {
			return nil, err
		}
	}
	entries := make([]WorkerTranscriptEntry, 0, len(projection.entries))
	for _, slot := range projection.entries {
		entry := slot.entry
		if entry.Text == nil && entry.Summary == nil && entry.Output == nil && entry.Name == nil {
			continue
		}
		entry.Order = len(entries) + 1
		entries = append(entries, entry)
	}
	return entries, nil
}

type transcriptProjection struct {
	entries []transcriptSlot
	slots   map[transcriptItem]int
	turn    *int
}

func (p *transcriptProjection) slot(key transcriptItem, kind string, record WorkerCapturedRecord) *WorkerTranscriptEntry {
	if index, ok := p.slots[key]; ok {
		p.entries[index].entry.Type = kind
		return &p.entries[index].entry
	}
	p.slots[key] = len(p.entries)
	entry := WorkerTranscriptEntry{Type: kind}
	if record.CapturedAt != nil {
		stamp := *record.CapturedAt
		entry.Timestamp = &stamp
	}
	if p.turn != nil {
		turn := *p.turn
		entry.TurnIndex = &turn
	}
	p.entries = append(p.entries, transcriptSlot{entry: entry})
	return &p.entries[len(p.entries)-1].entry
}

func (p *transcriptProjection) apply(draft workers.Draft, record WorkerCapturedRecord) error {
	// Item IDs are provider/source and turn scoped. Anonymous snapshots must
	// remain separate; there is no identity on which to merge their deltas.
	key := transcriptItem{sourceType: string(record.Record.SourceType), source: string(record.Record.SourceID), run: draft.RunID, turn: draft.TurnID, attempt: draft.DispatchID, item: draft.ItemID}
	if draft.ItemID == "" {
		key.anonymous = uint64(record.Record.ID.Position)
	}
	switch draft.Kind {
	case workers.KindTurn:
		var payload workers.TurnPayload
		if err := json.Unmarshal(draft.Payload, &payload); err != nil {
			return err
		}
		p.turn = &payload.TurnIndex
	case workers.KindMessage:
		return p.message(draft, record, key)
	case workers.KindReasoning:
		var payload workers.ReasoningPayload
		if err := json.Unmarshal(draft.Payload, &payload); err != nil {
			return err
		}
		entry := p.slot(key.withPart("reasoning"), "reasoning", record)
		if draft.Phase == workers.PhaseDelta {
			appendTranscriptText(&entry.Summary, payload.SummaryDelta)
		} else if payload.Summary != "" {
			entry.Summary = transcriptText(payload.Summary)
		}
	case workers.KindTool:
		return p.tool(draft, record, key)
	case workers.KindStreamGap:
		return fmt.Errorf("captured Worker stream contains a gap")
	}
	return nil
}

func (p *transcriptProjection) message(draft workers.Draft, record WorkerCapturedRecord, key transcriptItem) error {
	if draft.Phase == workers.PhaseDelta {
		var delta workers.MessageDeltaPayload
		if err := json.Unmarshal(draft.Payload, &delta); err != nil {
			return err
		}
		kind := transcriptBlockType(delta.ContentBlockKind, "assistant")
		if kind == "" {
			return nil
		}
		entry := p.slot(key.withPart("block/"+strconv.Itoa(delta.ContentBlockIndex)), kind, record)
		if kind == "reasoning" {
			appendTranscriptText(&entry.Summary, delta.TextDelta)
		} else {
			appendTranscriptText(&entry.Text, delta.TextDelta)
		}
		return nil
	}
	var payload workers.MessagePayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		return err
	}
	for index, block := range payload.ContentBlocks {
		kind := transcriptBlockType(block.Kind, payload.Role)
		if kind == "" {
			continue
		}
		entry := p.slot(key.withPart("block/"+strconv.Itoa(index)), kind, record)
		if kind == "reasoning" {
			entry.Summary = transcriptText(block.Text)
		} else {
			entry.Text = transcriptText(block.Text)
		}
	}
	return nil
}

func transcriptBlockType(kind workers.ContentBlockKind, role string) string {
	if kind == workers.ContentBlockReasoningSummary {
		return "reasoning"
	}
	if kind != workers.ContentBlockText {
		return ""
	}
	switch role {
	case "user":
		return "user_message"
	case "assistant":
		return "assistant_message"
	case "system":
		return "system_event"
	default:
		return ""
	}
}

func (p *transcriptProjection) tool(draft workers.Draft, record WorkerCapturedRecord, key transcriptItem) error {
	if draft.Phase == workers.PhaseDelta {
		var payload workers.ToolDeltaPayload
		if err := json.Unmarshal(draft.Payload, &payload); err != nil {
			return err
		}
		if key.item == "" && payload.ToolCallID != "" {
			key.item = payload.ToolCallID
			key.anonymous = 0
		}
		entry := p.slot(key.withPart("output"), "tool_output", record)
		entry.CallID = transcriptText(payload.ToolCallID)
		appendTranscriptText(&entry.Output, payload.OutputDelta)
		return nil
	}
	var payload workers.ToolPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		return err
	}
	if key.item == "" && payload.ToolCallID != "" {
		key.item = payload.ToolCallID
		key.anonymous = 0
	}
	entry := p.slot(key.withPart("call"), "tool_call", record)
	entry.Name, entry.CallID, entry.Status = transcriptText(payload.ToolName), transcriptText(payload.ToolCallID), transcriptText(payload.Status)
	if len(payload.ArgumentsSummary) > 0 {
		entry.Arguments = transcriptText(string(payload.ArgumentsSummary))
	}
	if len(payload.ResultSummary) > 0 {
		output := p.slot(key.withPart("output"), "tool_output", record)
		output.CallID, output.Output, output.Status = transcriptText(payload.ToolCallID), transcriptSummary(payload.ResultSummary), transcriptText(payload.Status)
	}
	return nil
}

func transcriptText(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}
func appendTranscriptText(target **string, delta string) {
	if delta == "" {
		return
	}
	text := delta
	if *target != nil {
		text = **target + delta
	}
	*target = &text
}

func transcriptSummary(raw json.RawMessage) *string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return transcriptText(text)
	}
	return transcriptText(string(raw))
}
