package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

const diagnosticMessage = "codex JSONL record could not be decoded"

type Decoder struct {
	context      adapter.DecoderContext
	stdout       []byte
	threadID     string
	turnID       string
	turnSequence int
}

func NewDecoder(input adapter.DecoderContext) *Decoder { return &Decoder{context: input} }

func (d *Decoder) Observe(_ context.Context, observation adapter.Observation) (adapter.DecodeResult, error) {
	if observation.Stream != adapter.OutputStreamStdout || len(observation.Chunk) == 0 {
		return adapter.DecodeResult{}, nil
	}
	d.stdout = append(d.stdout, observation.Chunk...)
	var result adapter.DecodeResult
	for {
		newline := bytes.IndexByte(d.stdout, '\n')
		if newline < 0 {
			break
		}
		result = appendResult(result, d.decodeRecord(d.stdout[:newline]))
		d.stdout = d.stdout[newline+1:]
	}
	return result, nil
}

func (d *Decoder) Flush(_ context.Context, _ adapter.FlushContext) (adapter.DecodeResult, error) {
	if len(bytes.TrimSpace(d.stdout)) == 0 {
		d.stdout = nil
		return adapter.DecodeResult{}, nil
	}
	result := d.decodeRecord(d.stdout)
	d.stdout = nil
	return result, nil
}

type recordEnvelope struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id"`
	Item     json.RawMessage `json:"item"`
}

type itemEnvelope struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Text string `json:"text"`
}

func (d *Decoder) decodeRecord(raw []byte) adapter.DecodeResult {
	if len(bytes.TrimSpace(raw)) == 0 {
		return adapter.DecodeResult{}
	}
	var record recordEnvelope
	if err := json.Unmarshal(raw, &record); err != nil {
		return diagnostic("codex_malformed_json", diagnosticMessage)
	}
	switch record.Type {
	case "thread.started":
		d.threadID = strings.TrimSpace(record.ThreadID)
		if d.threadID == "" {
			return diagnostic("codex_malformed_thread_started", diagnosticMessage)
		}
		return oneDraft(d.lifecycleDraft(responseevents.KindSession, responseevents.PhaseStarted, "started", "thread.started"))
	case "turn.started":
		d.turnSequence++
		d.turnID = fmt.Sprintf("codex-turn-%d", d.turnSequence)
		return oneDraft(d.lifecycleDraft(responseevents.KindTurn, responseevents.PhaseStarted, "started", "turn.started"))
	case "turn.completed":
		return oneDraft(d.lifecycleDraft(responseevents.KindTurn, responseevents.PhaseCompleted, "completed", "turn.completed"))
	case "item.completed":
		return d.decodeCompletedItem(record.Item)
	default:
		return diagnostic("codex_unknown_event", "codex JSONL event type is not supported: "+safeDiscriminator(record.Type))
	}
}

func (d *Decoder) decodeCompletedItem(raw json.RawMessage) adapter.DecodeResult {
	var item itemEnvelope
	if err := json.Unmarshal(raw, &item); err != nil || strings.TrimSpace(item.ID) == "" {
		return diagnostic("codex_malformed_item", diagnosticMessage)
	}
	if item.Type != "agent_message" {
		return diagnostic("codex_unknown_item", "codex JSONL item type is not supported: "+safeDiscriminator(item.Type))
	}
	payload, _ := json.Marshal(responseevents.MessagePayload{Role: "assistant", ContentBlocks: []responseevents.ContentBlock{{Kind: responseevents.ContentBlockText, Text: item.Text}}})
	return oneDraft(responseevents.Draft{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID,
		ItemID: strings.TrimSpace(item.ID), ProviderSessionRef: d.threadID,
		Kind: responseevents.KindMessage, Phase: responseevents.PhaseCompleted,
		Provenance: provenance("item.completed", responseevents.RepresentationSnapshot), Payload: payload,
	})
}

func (d *Decoder) lifecycleDraft(kind responseevents.Kind, phase responseevents.Phase, status, nativeType string) responseevents.Draft {
	var payload []byte
	if kind == responseevents.KindSession {
		payload, _ = json.Marshal(responseevents.SessionPayload{Status: status})
	} else {
		payload, _ = json.Marshal(responseevents.TurnPayload{TurnIndex: d.turnSequence, Status: status})
	}
	return responseevents.Draft{RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID,
		ProviderSessionRef: d.threadID, Kind: kind, Phase: phase,
		Provenance: provenance(nativeType, responseevents.RepresentationNotification), Payload: payload}
}

func provenance(nativeType string, representation responseevents.Representation) responseevents.Provenance {
	return responseevents.Provenance{Provider: "codex", NativeEventType: nativeType,
		Delivery: responseevents.DeliveryNativeStream, Representation: representation, Fidelity: responseevents.FidelityNormalized}
}

func safeDiscriminator(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}

func diagnostic(code, message string) adapter.DecodeResult {
	return adapter.DecodeResult{Diagnostics: []adapter.Diagnostic{{Code: code, Message: message}}}
}

func oneDraft(draft responseevents.Draft) adapter.DecodeResult {
	if err := responseevents.ValidateDraft(draft); err != nil {
		return diagnostic("codex_invalid_draft", diagnosticMessage)
	}
	return adapter.DecodeResult{Drafts: []responseevents.Draft{draft}}
}

func appendResult(target, next adapter.DecodeResult) adapter.DecodeResult {
	target.Drafts = append(target.Drafts, next.Drafts...)
	target.Diagnostics = append(target.Diagnostics, next.Diagnostics...)
	return target
}

var _ adapter.Decoder = (*Decoder)(nil)
