package codex

import (
	"encoding/json"
	"fmt"
	"strings"

	responseevents "github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/provider/adapter"
)

const itemSummaryLimit = 1024

type itemEnvelope struct {
	ID               string                `json:"id"`
	Type             string                `json:"type"`
	Text             string                `json:"text"`
	Message          string                `json:"message"`
	Command          string                `json:"command"`
	AggregatedOutput string                `json:"aggregated_output"`
	ExitCode         *int                  `json:"exit_code"`
	Status           string                `json:"status"`
	Changes          []fileUpdateChange    `json:"changes"`
	Server           string                `json:"server"`
	Tool             string                `json:"tool"`
	Result           json.RawMessage       `json:"result"`
	Error            *toolError            `json:"error"`
	ReceiverThreads  []string              `json:"receiver_thread_ids"`
	AgentStates      map[string]agentState `json:"agents_states"`
	Query            string                `json:"query"`
	Action           json.RawMessage       `json:"action"`
	Items            []todoItem            `json:"items"`
}

type fileUpdateChange struct {
	Path string `json:"path"`
	Kind string `json:"kind"`
}

type toolError struct {
	Message string `json:"message"`
}

type agentState struct {
	Status string `json:"status"`
}

type todoItem struct {
	Text      string `json:"text"`
	Completed bool   `json:"completed"`
}

func (d *Decoder) decodeItem(nativeEventType string, raw json.RawMessage) adapter.DecodeResult {
	var item itemEnvelope
	if err := json.Unmarshal(raw, &item); err != nil || strings.TrimSpace(item.ID) == "" {
		return diagnostic("codex_malformed_item", diagnosticMessage)
	}
	item.ID = strings.TrimSpace(item.ID)
	var draft responseevents.Draft
	var ok bool
	switch item.Type {
	case "agent_message":
		draft, ok = d.messageDraft(nativeEventType, item)
	case "reasoning":
		draft, ok = d.reasoningDraft(nativeEventType, item)
	case "command_execution":
		draft, ok = d.commandDraft(nativeEventType, item)
	case "file_change":
		draft, ok = d.fileChangeDraft(nativeEventType, item)
	case "mcp_tool_call":
		draft, ok = d.mcpToolDraft(nativeEventType, item)
	case "collab_tool_call":
		draft, ok = d.collaborationDraft(nativeEventType, item)
	case "web_search":
		draft, ok = d.webSearchDraft(nativeEventType, item)
	case "todo_list":
		draft, ok = d.planDraft(nativeEventType, item)
	case "error":
		draft, ok = d.itemErrorDraft(nativeEventType, item)
	default:
		return diagnostic("codex_unknown_item", "codex JSONL item type is not supported: unknown")
	}
	if !ok {
		return diagnostic("codex_malformed_"+safeDiscriminator(item.Type), diagnosticMessage)
	}
	return oneDraft(draft)
}

func (d *Decoder) itemErrorDraft(nativeType string, item itemEnvelope) (responseevents.Draft, bool) {
	message := boundedItemText(item.Message)
	if message == "" {
		return responseevents.Draft{}, false
	}
	payload := mustJSON(responseevents.ErrorPayload{Code: "codex_item_error", Message: message})
	return d.itemDraft(item, nativeType, responseevents.KindError, responseevents.PhaseUpdated, payload), true
}

func (d *Decoder) itemDraft(item itemEnvelope, nativeType string, kind responseevents.Kind, phase responseevents.Phase, payload []byte) responseevents.Draft {
	return responseevents.Draft{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID, TurnID: d.turnID,
		ItemID: item.ID, ProviderSessionRef: d.threadID, Kind: kind, Phase: phase,
		Provenance: provenance(nativeType, responseevents.RepresentationSnapshot), Payload: payload,
	}
}

func (d *Decoder) messageDraft(nativeType string, item itemEnvelope) (responseevents.Draft, bool) {
	text := boundedItemText(item.Text)
	if text == "" {
		return responseevents.Draft{}, false
	}
	payload := mustJSON(responseevents.MessagePayload{Role: "assistant", ContentBlocks: []responseevents.ContentBlock{{Kind: responseevents.ContentBlockText, Text: text}}})
	return d.itemDraft(item, nativeType, responseevents.KindMessage, lifecyclePhase(nativeType, item.Status), payload), true
}

func (d *Decoder) reasoningDraft(nativeType string, item itemEnvelope) (responseevents.Draft, bool) {
	payload := mustJSON(responseevents.ReasoningPayload{Summary: boundedItemText(item.Text)})
	return d.itemDraft(item, nativeType, responseevents.KindReasoning, lifecyclePhase(nativeType, item.Status), payload), true
}

func (d *Decoder) commandDraft(nativeType string, item itemEnvelope) (responseevents.Draft, bool) {
	if nativeType == "item.updated" {
		if strings.TrimSpace(item.AggregatedOutput) == "" {
			return responseevents.Draft{}, false
		}
		payload := mustJSON(responseevents.ToolDeltaPayload{ToolCallID: item.ID, OutputDelta: boundedItemText(item.AggregatedOutput)})
		return d.itemDraft(item, nativeType, responseevents.KindTool, responseevents.PhaseDelta, payload), true
	}
	arguments := mustJSON(map[string]string{"category": "command", "command": boundedItemText(item.Command)})
	result := map[string]any{"status": safeDiscriminator(item.Status), "output": boundedItemText(item.AggregatedOutput)}
	if item.ExitCode != nil {
		result["exitCode"] = *item.ExitCode
	}
	payload := mustJSON(responseevents.ToolPayload{ToolCallID: item.ID, ToolName: "command_execution", Status: safeDiscriminator(item.Status), ArgumentsSummary: arguments, ResultSummary: mustJSON(result)})
	return d.itemDraft(item, nativeType, responseevents.KindTool, lifecyclePhase(nativeType, item.Status), payload), true
}

func (d *Decoder) fileChangeDraft(nativeType string, item itemEnvelope) (responseevents.Draft, bool) {
	if len(item.Changes) == 0 {
		return responseevents.Draft{}, false
	}
	first := item.Changes[0]
	if strings.TrimSpace(first.Path) == "" || strings.TrimSpace(first.Kind) == "" {
		return responseevents.Draft{}, false
	}
	summaries := make([]string, 0, len(item.Changes))
	for _, change := range item.Changes {
		if strings.TrimSpace(change.Path) != "" && strings.TrimSpace(change.Kind) != "" {
			summaries = append(summaries, change.Kind+" "+change.Path)
		}
	}
	payload := mustJSON(responseevents.FileChangePayload{Path: boundedItemText(first.Path), Operation: safeDiscriminator(first.Kind), Summary: boundedItemText(strings.Join(summaries, "; "))})
	return d.itemDraft(item, nativeType, responseevents.KindFileChange, responseevents.PhaseUpdated, payload), true
}

func (d *Decoder) mcpToolDraft(nativeType string, item itemEnvelope) (responseevents.Draft, bool) {
	name := strings.Trim(strings.TrimSpace(item.Server)+"/"+strings.TrimSpace(item.Tool), "/")
	if name == "" {
		return responseevents.Draft{}, false
	}
	summary := map[string]any{"category": "mcp", "server": safeDiscriminator(item.Server), "tool": safeDiscriminator(item.Tool), "status": safeDiscriminator(item.Status)}
	if item.Error != nil {
		summary["error"] = boundedItemText(item.Error.Message)
	} else if len(item.Result) > 0 && string(item.Result) != "null" {
		summary["resultAvailable"] = true
	}
	return d.toolSnapshotDraft(nativeType, item, "mcp:"+name, summary), true
}

func (d *Decoder) collaborationDraft(nativeType string, item itemEnvelope) (responseevents.Draft, bool) {
	if strings.TrimSpace(item.Tool) == "" {
		return responseevents.Draft{}, false
	}
	summary := map[string]any{"category": "collaboration", "tool": safeDiscriminator(item.Tool), "status": safeDiscriminator(item.Status), "receiverCount": len(item.ReceiverThreads), "agentCount": len(item.AgentStates)}
	return d.toolSnapshotDraft(nativeType, item, "collaboration:"+item.Tool, summary), true
}

func (d *Decoder) webSearchDraft(nativeType string, item itemEnvelope) (responseevents.Draft, bool) {
	summary := map[string]any{"category": "web_search", "query": boundedItemText(item.Query)}
	if actionType := jsonObjectString(item.Action, "type"); actionType != "" {
		summary["action"] = safeDiscriminator(actionType)
	}
	item.Status = nativeStatus(nativeType)
	return d.toolSnapshotDraft(nativeType, item, "web_search", summary), true
}

func (d *Decoder) toolSnapshotDraft(nativeType string, item itemEnvelope, name string, summary map[string]any) responseevents.Draft {
	payload := mustJSON(responseevents.ToolPayload{ToolCallID: item.ID, ToolName: boundedItemText(name), Status: safeDiscriminator(item.Status), ResultSummary: mustJSON(summary)})
	return d.itemDraft(item, nativeType, responseevents.KindTool, lifecyclePhase(nativeType, item.Status), payload)
}

func (d *Decoder) planDraft(nativeType string, item itemEnvelope) (responseevents.Draft, bool) {
	steps := make([]responseevents.PlanStep, 0, len(item.Items))
	for _, todo := range item.Items {
		status := "pending"
		if todo.Completed {
			status = "completed"
		}
		steps = append(steps, responseevents.PlanStep{Description: boundedItemText(todo.Text), Status: status})
	}
	payload := mustJSON(responseevents.PlanPayload{Steps: steps, Summary: nativeStatus(nativeType)})
	return d.itemDraft(item, nativeType, responseevents.KindPlan, responseevents.PhaseUpdated, payload), true
}

func lifecyclePhase(nativeType, status string) responseevents.Phase {
	switch nativeType {
	case "item.started":
		return responseevents.PhaseStarted
	case "item.updated":
		return responseevents.PhaseDelta
	}
	switch status {
	case "failed":
		return responseevents.PhaseFailed
	case "declined":
		return responseevents.PhaseCanceled
	default:
		return responseevents.PhaseCompleted
	}
}

func nativeStatus(nativeType string) string {
	return strings.TrimPrefix(nativeType, "item.")
}

func boundedItemText(value string) string {
	return boundedText(strings.TrimSpace(value), itemSummaryLimit)
}

func boundedText(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}

func mustJSON(value any) []byte {
	payload, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshal codex canonical payload: %v", err))
	}
	return payload
}

func jsonObjectString(raw json.RawMessage, key string) string {
	var object map[string]json.RawMessage
	if json.Unmarshal(raw, &object) != nil {
		return ""
	}
	var value string
	_ = json.Unmarshal(object[key], &value)
	return value
}
