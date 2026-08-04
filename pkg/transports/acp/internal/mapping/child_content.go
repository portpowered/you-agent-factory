package mapping

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// projectChildRecord gives every legal non-opening Worker record an explicit
// parent-addressed ACP outcome. It is deliberately pure: all ownership and
// identity facts have already been carried by the sequenced envelope.
func projectChildRecord(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	switch draft.Kind {
	case workers.KindSession:
		return ProjectChildLifecycle(draft)
	case workers.KindRun:
		return validateRun(draft)
	case workers.KindTurn:
		return validateTurn(draft)
	case workers.KindMessage:
		return projectChildMessage(draft)
	case workers.KindReasoning:
		return projectChildReasoning(draft)
	case workers.KindTool:
		return projectChildTool(draft)
	case workers.KindFileChange:
		return projectChildFileChange(draft)
	case workers.KindPlan:
		return projectChildPlan(draft)
	case workers.KindProgress:
		return projectChildProgress(draft)
	case workers.KindUsage:
		return projectChildUsage(draft)
	case workers.KindError:
		return projectChildError(draft)
	case workers.KindStreamGap:
		return projectChildStreamGap(draft)
	default:
		return nil, fmt.Errorf("%w: kind %q has no child projection", ErrMalformedRecord, draft.Kind)
	}
}

func validateRun(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	var payload workers.RunPayload
	if err := decodeChildPayload(draft.Payload, &payload, "RunPayload"); err != nil {
		return nil, err
	}
	return nil, nil
}

func validateTurn(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	var payload workers.TurnPayload
	if err := decodeChildPayload(draft.Payload, &payload, "TurnPayload"); err != nil {
		return nil, err
	}
	return nil, nil
}

func projectChildMessage(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	if err := validateChildMessage(draft); err != nil {
		return nil, err
	}
	text, err := messageText(draft.Phase, draft.Payload)
	if err != nil || text == "" {
		return nil, err
	}
	return childTextUpdate(draft.ParentItemID, text), nil
}

func projectChildReasoning(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	text, err := reasoningText(draft.Phase, draft.Payload)
	if err != nil || text == "" {
		return nil, err
	}
	return childTextUpdate(draft.ParentItemID, text), nil
}

// projectChildTool preserves the current workers.ToolPayload and
// workers.ToolDeltaPayload vocabulary in ACP raw fields. The parent Worker
// tool call remains the only ACP identity; provider-native tool IDs are data
// inside that call, never a second transport-local tool-call taxonomy.
func projectChildTool(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	if draft.Phase == workers.PhaseDelta {
		var payload workers.ToolDeltaPayload
		if err := decodeChildPayload(draft.Payload, &payload, "ToolDeltaPayload"); err != nil {
			return nil, err
		}
		if strings.TrimSpace(payload.ToolCallID) == "" || payload.OutputDelta == "" {
			return nil, fmt.Errorf("%w: ToolDeltaPayload requires toolCallId and outputDelta", ErrMalformedRecord)
		}
		return &acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
			ToolCallId: acpsdk.ToolCallId(draft.ParentItemID),
			Content:    childTextContent(payload.OutputDelta),
			RawOutput:  payload,
		}}, nil
	}

	var payload workers.ToolPayload
	if err := decodeChildPayload(draft.Payload, &payload, "ToolPayload"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.ToolCallID) == "" || strings.TrimSpace(payload.ToolName) == "" {
		return nil, fmt.Errorf("%w: ToolPayload requires toolCallId and toolName", ErrMalformedRecord)
	}
	title := "Tool " + payload.ToolName
	text := title
	if payload.Status != "" {
		text += " (" + payload.Status + ")"
	}
	return &acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
		ToolCallId: acpsdk.ToolCallId(draft.ParentItemID),
		Title:      &title,
		Content:    childTextContent(text),
		RawInput:   payload,
	}}, nil
}

func projectChildFileChange(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	var payload workers.FileChangePayload
	if err := decodeChildPayload(draft.Payload, &payload, "FileChangePayload"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Path) == "" || strings.TrimSpace(payload.Operation) == "" {
		return nil, fmt.Errorf("%w: FileChangePayload requires path and operation", ErrMalformedRecord)
	}
	text := "File " + payload.Operation + ": " + payload.Path
	if payload.Summary != "" {
		text += "\n" + payload.Summary
	}
	return &acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
		ToolCallId: acpsdk.ToolCallId(draft.ParentItemID),
		Content:    childTextContent(text),
		Locations:  []acpsdk.ToolCallLocation{{Path: payload.Path}},
	}}, nil
}

func projectChildPlan(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	var payload workers.PlanPayload
	if err := decodeChildPayload(draft.Payload, &payload, "PlanPayload"); err != nil {
		return nil, err
	}
	parts := make([]string, 0, len(payload.Steps)+1)
	if payload.Summary != "" {
		parts = append(parts, payload.Summary)
	}
	for _, step := range payload.Steps {
		if strings.TrimSpace(step.Description) == "" {
			return nil, fmt.Errorf("%w: PlanPayload step description is required", ErrMalformedRecord)
		}
		line := step.Description
		if step.Status != "" {
			line += " (" + step.Status + ")"
		}
		parts = append(parts, line)
	}
	if len(parts) == 0 {
		return nil, nil
	}
	return childTextUpdate(draft.ParentItemID, strings.Join(parts, "\n")), nil
}

func projectChildProgress(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	var payload workers.ProgressPayload
	if err := decodeChildPayload(draft.Payload, &payload, "ProgressPayload"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Label) == "" {
		return nil, fmt.Errorf("%w: ProgressPayload label is required", ErrMalformedRecord)
	}
	text := payload.Label
	if payload.Message != "" {
		text += ": " + payload.Message
	}
	if payload.PercentComplete != nil {
		text += " (" + strconv.FormatFloat(*payload.PercentComplete, 'f', -1, 64) + "%)"
	}
	return childTextUpdate(draft.ParentItemID, text), nil
}

func projectChildUsage(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	var payload workers.UsagePayload
	if err := decodeChildPayload(draft.Payload, &payload, "UsagePayload"); err != nil {
		return nil, err
	}
	text := fmt.Sprintf("Usage: %d total tokens", payload.TotalTokens)
	if payload.Model != "" {
		text += " (" + payload.Model + ")"
	}
	return childTextUpdate(draft.ParentItemID, text), nil
}

func projectChildError(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	var payload workers.ErrorPayload
	if err := decodeChildPayload(draft.Payload, &payload, "ErrorPayload"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Code) == "" || strings.TrimSpace(payload.Message) == "" {
		return nil, fmt.Errorf("%w: ErrorPayload requires code and message", ErrMalformedRecord)
	}
	text := "Worker error [" + payload.Code + "]: " + payload.Message
	if payload.Retryable {
		text += " (retryable)"
	}
	return childTextUpdate(draft.ParentItemID, text), nil
}

func projectChildStreamGap(draft workers.Draft) (*acpsdk.SessionUpdate, error) {
	var payload workers.StreamGapPayload
	if err := decodeChildPayload(draft.Payload, &payload, "StreamGapPayload"); err != nil {
		return nil, err
	}
	if err := validateStreamGapPayload(payload); err != nil {
		return nil, err
	}
	return childTextUpdate(draft.ParentItemID, gapNoticeText(payload)), nil
}

func childTextUpdate(parentItemID, text string) *acpsdk.SessionUpdate {
	return &acpsdk.SessionUpdate{ToolCallUpdate: &acpsdk.SessionToolCallUpdate{
		ToolCallId: acpsdk.ToolCallId(parentItemID),
		Content:    childTextContent(text),
	}}
}

func childTextContent(text string) []acpsdk.ToolCallContent {
	return []acpsdk.ToolCallContent{{Content: &acpsdk.ToolCallContentContent{
		Content: acpsdk.TextBlock(text),
	}}}
}

func decodeChildPayload(payload json.RawMessage, target any, typeName string) error {
	if len(payload) == 0 || !json.Valid(payload) {
		return fmt.Errorf("%w: payload must contain valid JSON for %s", ErrMalformedRecord, typeName)
	}
	if err := json.Unmarshal(payload, target); err != nil {
		return fmt.Errorf("%w: payload must decode as %s: %v", ErrMalformedRecord, typeName, err)
	}
	return nil
}

func validateChildMessage(draft workers.Draft) error {
	if draft.Phase == workers.PhaseDelta {
		var payload workers.MessageDeltaPayload
		if err := decodeChildPayload(draft.Payload, &payload, "MessageDeltaPayload"); err != nil {
			return err
		}
		if payload.ContentBlockIndex < 0 || !knownContentBlockKind(payload.ContentBlockKind) {
			return fmt.Errorf("%w: invalid MessageDeltaPayload", ErrMalformedRecord)
		}
		return nil
	}
	var payload workers.MessagePayload
	if err := decodeChildPayload(draft.Payload, &payload, "MessagePayload"); err != nil {
		return err
	}
	if strings.TrimSpace(payload.Role) == "" || len(payload.ContentBlocks) == 0 {
		return fmt.Errorf("%w: MessagePayload requires role and contentBlocks", ErrMalformedRecord)
	}
	for _, block := range payload.ContentBlocks {
		if !knownContentBlockKind(block.Kind) {
			return fmt.Errorf("%w: unknown MessagePayload content block kind %q", ErrMalformedRecord, block.Kind)
		}
	}
	return nil
}

func knownContentBlockKind(kind workers.ContentBlockKind) bool {
	switch kind {
	case workers.ContentBlockText, workers.ContentBlockReasoningSummary, workers.ContentBlockToolRequest,
		workers.ContentBlockImageRef, workers.ContentBlockResourceRef, workers.ContentBlockStructuredOutput:
		return true
	default:
		return false
	}
}
