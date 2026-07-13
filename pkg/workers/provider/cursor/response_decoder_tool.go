package cursor

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/factorysessions/responseevents"
	"github.com/portpowered/infinite-you/pkg/workers/provider/adapter"
)

const (
	cursorToolFallbackName       = "tool_call"
	cursorToolSummaryMaxDepth    = 3
	cursorToolSummaryMaxEntries  = 16
	cursorToolSummaryStringLimit = 256
	cursorToolRedactedValue      = "<redacted>"
	cursorToolTruncatedValue     = "<truncated>"
	cursorToolGapReconnect       = "provider_reconnect"
	cursorToolGapFlush           = "decoder_flush"
	cursorToolGapTerminated      = "provider_terminated"
	cursorToolGapTerminal        = "terminal_result_missing_completion"
	cursorToolGapFailure         = "provider_terminal_failure"
)

type cursorToolState struct {
	name             string
	argumentsSummary json.RawMessage
	gapReason        string
}

type cursorToolCloseOutcome struct {
	reason        string
	canceled      bool
	nativeType    string
	nativeSubtype string
}

type cursorToolEnvelope struct {
	Function *cursorFunctionTool `json:"function"`
}

type cursorFunctionTool struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
	Result    json.RawMessage `json:"result"`
}

func (d *ResponseEventDecoder) decodeToolCall(record cursorStreamRecord) (adapter.DecodeResult, error) {
	callID := strings.TrimSpace(record.CallID)
	if callID == "" {
		return cursorDiagnostic(cursorDiagnosticInvalidToolCall, "Cursor tool record omitted a valid call identifier"), nil
	}

	name, arguments, result, malformed := decodeCursorToolDetails(record.ToolCall)
	if malformed {
		return cursorDiagnostic(cursorDiagnosticMalformedRecord, "Cursor stream ignored a malformed tool record"), nil
	}
	state, known := d.tools[callID]
	if known {
		name = state.name
	}
	name = cursorSafeToolName(name)

	switch strings.ToLower(strings.TrimSpace(record.Subtype)) {
	case "started":
		argumentSummary := cursorSafeToolSummary(arguments)
		d.tools[callID] = cursorToolState{name: name, argumentsSummary: argumentSummary}
		return d.cursorToolDraft(record, callID, name, responseevents.PhaseStarted, "started", argumentSummary, nil)
	case "completed", "failed", "canceled", "cancelled":
		phase, status := cursorToolTerminalPhase(record.Subtype, result)
		delete(d.tools, callID)
		return d.cursorToolDraft(record, callID, name, phase, status, state.argumentsSummary, cursorSafeToolSummary(result))
	default:
		return cursorDiagnostic(cursorDiagnosticUnknownRecord, "Cursor stream ignored an unsupported tool lifecycle record"), nil
	}
}

func (d *ResponseEventDecoder) cursorToolDraft(record cursorStreamRecord, callID, name string, phase responseevents.Phase, status string, arguments, result json.RawMessage) (adapter.DecodeResult, error) {
	return d.cursorToolDraftWithProviderRef(record, callID, name, phase, status, arguments, result, d.providerRef(record.SessionID))
}

func (d *ResponseEventDecoder) cursorToolDraftWithProviderRef(record cursorStreamRecord, callID, name string, phase responseevents.Phase, status string, arguments, result json.RawMessage, providerRef string) (adapter.DecodeResult, error) {
	payload, err := json.Marshal(responseevents.ToolPayload{
		ToolCallID: callID, ToolName: name, Status: status,
		ArgumentsSummary: arguments, ResultSummary: result,
	})
	if err != nil {
		return adapter.DecodeResult{}, fmt.Errorf("marshal Cursor tool payload: %w", err)
	}
	return adapter.DecodeResult{Drafts: []responseevents.Draft{{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID,
		Kind: responseevents.KindTool, Phase: phase,
		ItemID: "cursor-tool/" + callID, ProviderSessionRef: providerRef,
		Provenance: cursorResponseProvenance("tool_call", record.Subtype, responseevents.RepresentationNotification, responseevents.FidelityNormalized),
		Payload:    payload,
	}}}, nil
}

func (d *ResponseEventDecoder) markToolsInterrupted(reason string) {
	for callID, state := range d.tools {
		if state.gapReason == "" {
			state.gapReason = reason
			d.tools[callID] = state
		}
	}
}

func (d *ResponseEventDecoder) closeUnresolvedTools(outcome cursorToolCloseOutcome) (adapter.DecodeResult, error) {
	callIDs := make([]string, 0, len(d.tools))
	for callID := range d.tools {
		callIDs = append(callIDs, callID)
	}
	sort.Strings(callIDs)

	var result adapter.DecodeResult
	for _, callID := range callIDs {
		state := d.tools[callID]
		delete(d.tools, callID)
		if outcome.canceled {
			closed, err := d.cursorCanceledToolDraft(callID, state, outcome)
			result = appendCursorDecodeResult(result, closed)
			if err != nil {
				return result, err
			}
			continue
		}
		reason := state.gapReason
		if reason == "" {
			reason = outcome.reason
		}
		closed, err := d.cursorToolGapDraft(callID, reason)
		result = appendCursorDecodeResult(result, closed)
		if err != nil {
			return result, err
		}
	}
	return result, nil
}

func (d *ResponseEventDecoder) cursorCanceledToolDraft(callID string, state cursorToolState, outcome cursorToolCloseOutcome) (adapter.DecodeResult, error) {
	payload, err := json.Marshal(responseevents.ToolPayload{
		ToolCallID: callID, ToolName: state.name, Status: "canceled",
		ArgumentsSummary: state.argumentsSummary,
	})
	if err != nil {
		return adapter.DecodeResult{}, fmt.Errorf("marshal Cursor synthesized tool cancellation payload: %w", err)
	}
	return adapter.DecodeResult{Drafts: []responseevents.Draft{{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID,
		Kind: responseevents.KindTool, Phase: responseevents.PhaseCanceled,
		ItemID: "cursor-tool/" + callID, ProviderSessionRef: d.providerSessionRef,
		Provenance: responseevents.Provenance{
			Provider:        cursorResponseProvenance(outcome.nativeType, outcome.nativeSubtype, responseevents.RepresentationNotification, responseevents.FidelityNormalized).Provider,
			NativeEventType: outcome.nativeType, NativeEventSubtype: outcome.nativeSubtype,
			Delivery: responseevents.DeliverySynthesized, Representation: responseevents.RepresentationNotification,
			Fidelity: responseevents.FidelityNormalized,
		},
		Payload: payload,
	}}}, nil
}

func (d *ResponseEventDecoder) cursorToolGapDraft(callID, reason string) (adapter.DecodeResult, error) {
	itemID := "cursor-tool/" + callID
	payload, err := json.Marshal(responseevents.StreamGapPayload{
		AffectedItemID: itemID,
		ToolCallID:     callID,
		Reason:         reason,
	})
	if err != nil {
		return adapter.DecodeResult{}, fmt.Errorf("marshal Cursor tool gap payload: %w", err)
	}
	return adapter.DecodeResult{Drafts: []responseevents.Draft{{
		RunID: d.context.RunID, DispatchID: d.context.DispatchID,
		Kind: responseevents.KindStreamGap, Phase: responseevents.PhaseUpdated,
		ItemID: itemID, ProviderSessionRef: d.providerSessionRef,
		Provenance: responseevents.Provenance{
			Provider:        cursorResponseProvenance("tool_call", "missing_completion", responseevents.RepresentationNotification, responseevents.FidelityLossy).Provider,
			NativeEventType: "tool_call", NativeEventSubtype: "missing_completion",
			Delivery: responseevents.DeliverySynthesized, Representation: responseevents.RepresentationNotification,
			Fidelity: responseevents.FidelityLossy,
		},
		Payload: payload,
	}}}, nil
}

func cursorToolFlushOutcome(reason adapter.FlushReason) cursorToolCloseOutcome {
	switch reason {
	case adapter.FlushReasonCanceled:
		return cursorToolCloseOutcome{canceled: true, nativeType: "provider_boundary", nativeSubtype: "canceled"}
	case adapter.FlushReasonTerminated:
		return cursorToolCloseOutcome{reason: cursorToolGapTerminated}
	default:
		return cursorToolCloseOutcome{reason: cursorToolGapFlush}
	}
}

func decodeCursorToolDetails(raw json.RawMessage) (name string, arguments, result json.RawMessage, malformed bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil, nil, false
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return "", nil, nil, true
	}
	if functionRaw, ok := fields["function"]; ok {
		var function cursorFunctionTool
		if err := json.Unmarshal(functionRaw, &function); err != nil {
			return "", nil, nil, true
		}
		return strings.TrimSpace(function.Name), function.Arguments, function.Result, false
	}

	keys := make([]string, 0, len(fields))
	for key := range fields {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return "", nil, nil, false
	}
	sort.Strings(keys)
	name = keys[0]
	var details struct {
		Args      json.RawMessage `json:"args"`
		Arguments json.RawMessage `json:"arguments"`
		Result    json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(fields[name], &details); err != nil {
		return "", nil, nil, true
	}
	arguments = details.Args
	if len(arguments) == 0 {
		arguments = details.Arguments
	}
	return name, arguments, details.Result, false
}

func cursorToolTerminalPhase(subtype string, result json.RawMessage) (responseevents.Phase, string) {
	switch strings.ToLower(strings.TrimSpace(subtype)) {
	case "failed":
		return responseevents.PhaseFailed, "failed"
	case "canceled", "cancelled":
		return responseevents.PhaseCanceled, "canceled"
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(result, &fields) == nil {
		if _, ok := fields["canceled"]; ok {
			return responseevents.PhaseCanceled, "canceled"
		}
		if _, ok := fields["cancelled"]; ok {
			return responseevents.PhaseCanceled, "canceled"
		}
		if _, ok := fields["error"]; ok {
			return responseevents.PhaseFailed, "failed"
		}
		if _, ok := fields["failure"]; ok {
			return responseevents.PhaseFailed, "failed"
		}
	}
	return responseevents.PhaseCompleted, "completed"
}

type cursorToolSummaryBudget struct {
	remaining int
}

func cursorSafeToolSummary(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil
	}
	if encoded, ok := value.(string); ok && json.Valid([]byte(encoded)) {
		if err := json.Unmarshal([]byte(encoded), &value); err != nil {
			return nil
		}
	}
	budget := cursorToolSummaryBudget{remaining: cursorToolSummaryMaxEntries}
	safe := sanitizeCursorToolValue("", value, 0, &budget)
	object, ok := safe.(map[string]any)
	if !ok || len(object) == 0 {
		return nil
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return nil
	}
	return encoded
}

func cursorSafeToolName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return cursorToolFallbackName
	}
	for _, character := range trimmed {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || strings.ContainsRune("_.-", character) {
			continue
		}
		return cursorToolFallbackName
	}
	return boundedTrimmedText(trimmed, PublishedDiagnosticLimit)
}

func sanitizeCursorToolValue(key string, value any, depth int, budget *cursorToolSummaryBudget) any {
	if cursorSensitiveToolKey(key) {
		return cursorToolRedactedValue
	}
	if depth >= cursorToolSummaryMaxDepth || budget.remaining <= 0 {
		return cursorToolTruncatedValue
	}
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for field := range typed {
			keys = append(keys, field)
		}
		sort.Strings(keys)
		result := make(map[string]any)
		for _, field := range keys {
			if budget.remaining <= 0 {
				result["summary"] = cursorToolTruncatedValue
				break
			}
			budget.remaining--
			result[field] = sanitizeCursorToolValue(field, typed[field], depth+1, budget)
		}
		return result
	case []any:
		result := make([]any, 0, len(typed))
		for _, item := range typed {
			if budget.remaining <= 0 {
				result = append(result, cursorToolTruncatedValue)
				break
			}
			budget.remaining--
			result = append(result, sanitizeCursorToolValue(key, item, depth+1, budget))
		}
		return result
	case string:
		if cursorSensitiveToolValue(typed) {
			return cursorToolRedactedValue
		}
		return boundedText(typed, cursorToolSummaryStringLimit)
	case nil, bool, float64:
		return typed
	default:
		return cursorToolRedactedValue
	}
}

func cursorSensitiveToolKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
	for _, marker := range []string{"api_key", "apikey", "authorization", "credential", "password", "passwd", "private_key", "prompt", "secret", "token"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}

func cursorSensitiveToolValue(value string) bool {
	normalized := strings.ToLower(value)
	for _, marker := range []string{"authorization:", "bearer ", "api_key=", "apikey=", "password=", "private key", "secret=", "token="} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
