package mapping

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	acpsdk "github.com/coder/acp-go-sdk"

	chatsessions "github.com/portpowered/infinite-you/pkg/services/chat_sessions"
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

// ChildProjectionLimits names the bounded amount of content one associated
// Worker Session can add to its ACP tool call. Limits apply independently to
// each stored parent ItemID, never to a connection or a whole Factory turn.
// MaxRecords includes ACP-visible elision notices; MaxSerializedBytes is the
// JSON encoding size of the SessionUpdate delivered to the ACP client.
type ChildProjectionLimits struct {
	MaxRecords         int
	MaxSerializedBytes int
}

const (
	// DefaultChildProjectionMaxRecords leaves enough room for an explicit
	// ordinary-content elision and a later failure/gap indication while still
	// allowing normal Worker activity to remain useful.
	DefaultChildProjectionMaxRecords = 128
	// DefaultChildProjectionMaxSerializedBytes bounds one child's delivered
	// content without allowing a noisy provider payload to dominate siblings.
	DefaultChildProjectionMaxSerializedBytes = 64 * 1024
)

// DefaultChildProjectionLimits returns the production projection bounds.
func DefaultChildProjectionLimits() ChildProjectionLimits {
	return ChildProjectionLimits{
		MaxRecords:         DefaultChildProjectionMaxRecords,
		MaxSerializedBytes: DefaultChildProjectionMaxSerializedBytes,
	}
}

// ChildProjectionBudget is an explicit value-state accumulator for one
// child tool call. Callers retain it with their own delivery state and pass it
// back to BoundChildProjection; the mapper performs no IO, identity lookup,
// cursor mutation, or transport notification.
type ChildProjectionBudget struct {
	limits                 ChildProjectionLimits
	emittedRecords         int
	emittedBytes           int
	ordinaryElisionEmitted bool
	failureElisionEmitted  bool
}

// NewChildProjectionBudget creates empty, validated value state. Invalid
// limits fail closed through BoundChildProjection rather than allowing an
// implicit unlimited path.
func NewChildProjectionBudget(limits ChildProjectionLimits) ChildProjectionBudget {
	return ChildProjectionBudget{limits: limits}
}

// BoundChildProjection decides whether a mapped child update fits the
// supplied per-child budget. It returns a new value-state and either the
// original update, one deterministic ACP-visible elision update, or declared
// no output after a prior elision has stated that later ordinary content is
// omitted. Lifecycle-only updates are never content-budgeted, so terminal
// state remains truthful even when a child is noisy.
//
// Two notice slots are reserved before accepting ordinary content. The first
// makes normal record/byte pressure visible; the second remains available for
// failure or STREAM_GAP evidence that occurs after transient content has
// filled the normal allocation. This gives failure evidence precedence over
// progress without ever exceeding the named record or byte limits.
func BoundChildProjection(
	budget ChildProjectionBudget,
	item chatsessions.SequencedItem,
	update *acpsdk.SessionUpdate,
) (ChildProjectionBudget, *acpsdk.SessionUpdate, error) {
	if update == nil || !hasBoundedChildContent(update) {
		return budget, update, nil
	}
	if err := validateChildProjectionLimits(budget.limits); err != nil {
		return budget, nil, err
	}

	parentItemID, err := childProjectionParentItemID(item, update)
	if err != nil {
		return budget, nil, err
	}
	ordinaryNotice, ordinaryNoticeBytes := childProjectionElision(parentItemID, childProjectionElisionOrdinary)
	failureNotice, failureNoticeBytes := childProjectionElision(parentItemID, childProjectionElisionFailure)
	reservedBytes := ordinaryNoticeBytes + failureNoticeBytes
	if reservedBytes >= budget.limits.MaxSerializedBytes {
		return budget, nil, fmt.Errorf("%w: child projection byte limit %d cannot hold required elision evidence", ErrMalformedRecord, budget.limits.MaxSerializedBytes)
	}

	serializedBytes, err := serializedUpdateBytes(update)
	if err != nil {
		return budget, nil, err
	}
	if childProjectionProtectedEvidence(item) {
		return boundProtectedChildProjection(budget, item, update, serializedBytes, ordinaryNoticeBytes, failureNotice, failureNoticeBytes)
	}
	if budget.ordinaryElisionEmitted {
		return budget, nil, nil
	}
	if budget.emittedRecords < budget.limits.MaxRecords-2 &&
		budget.emittedBytes+serializedBytes <= budget.limits.MaxSerializedBytes-reservedBytes {
		return budget.record(serializedBytes), update, nil
	}
	return budget.recordOrdinaryElision(ordinaryNoticeBytes), ordinaryNotice, nil
}

type childProjectionElisionKind uint8

const (
	childProjectionElisionOrdinary childProjectionElisionKind = iota
	childProjectionElisionFailure
)

func (budget ChildProjectionBudget) record(bytes int) ChildProjectionBudget {
	budget.emittedRecords++
	budget.emittedBytes += bytes
	return budget
}

func (budget ChildProjectionBudget) recordOrdinaryElision(bytes int) ChildProjectionBudget {
	budget = budget.record(bytes)
	budget.ordinaryElisionEmitted = true
	return budget
}

func (budget ChildProjectionBudget) recordFailureElision(bytes int) ChildProjectionBudget {
	budget = budget.record(bytes)
	budget.failureElisionEmitted = true
	return budget
}

func validateChildProjectionLimits(limits ChildProjectionLimits) error {
	if limits.MaxRecords < 2 || limits.MaxSerializedBytes <= 0 {
		return fmt.Errorf("%w: child projection limits require at least two records and a positive byte limit", ErrMalformedRecord)
	}
	return nil
}

func hasBoundedChildContent(update *acpsdk.SessionUpdate) bool {
	if update == nil || update.ToolCallUpdate == nil {
		return false
	}
	tool := update.ToolCallUpdate
	return len(tool.Content) > 0 || tool.RawInput != nil || tool.RawOutput != nil || len(tool.Locations) > 0
}

func childProjectionParentItemID(item chatsessions.SequencedItem, update *acpsdk.SessionUpdate) (string, error) {
	if update.ToolCallUpdate == nil || strings.TrimSpace(string(update.ToolCallUpdate.ToolCallId)) == "" {
		return "", ErrMissingChildParent
	}
	parentItemID := string(update.ToolCallUpdate.ToolCallId)
	if item.ParentItemID != "" && item.ParentItemID != parentItemID {
		return "", fmt.Errorf("%w: child update target does not match stored parent item id", ErrMalformedRecord)
	}
	return parentItemID, nil
}

func childProjectionProtectedEvidence(item chatsessions.SequencedItem) bool {
	return item.Kind == workers.KindError || item.Kind == workers.KindStreamGap
}

func boundProtectedChildProjection(
	budget ChildProjectionBudget,
	item chatsessions.SequencedItem,
	update *acpsdk.SessionUpdate,
	serializedBytes int,
	ordinaryNoticeBytes int,
	failureNotice *acpsdk.SessionUpdate,
	failureNoticeBytes int,
) (ChildProjectionBudget, *acpsdk.SessionUpdate, error) {
	// Keep one ordinary-elision slot available when possible. A later noisy
	// update must still be able to say why it was omitted after error/gap
	// evidence took the protected path.
	if budget.emittedRecords < budget.limits.MaxRecords-1 &&
		budget.emittedBytes+serializedBytes <= budget.limits.MaxSerializedBytes-ordinaryNoticeBytes {
		return budget.record(serializedBytes), update, nil
	}
	if item.Kind == workers.KindStreamGap {
		gapNotice, gapNoticeBytes, err := childStreamGapElision(item, string(update.ToolCallUpdate.ToolCallId))
		if err != nil {
			return budget, nil, err
		}
		if budget.emittedRecords < budget.limits.MaxRecords &&
			budget.emittedBytes+gapNoticeBytes <= budget.limits.MaxSerializedBytes {
			return budget.recordFailureElision(gapNoticeBytes), gapNotice, nil
		}
	}
	if !budget.failureElisionEmitted && budget.emittedRecords < budget.limits.MaxRecords &&
		budget.emittedBytes+failureNoticeBytes <= budget.limits.MaxSerializedBytes {
		return budget.recordFailureElision(failureNoticeBytes), failureNotice, nil
	}
	// An existing ordinary-elision notice already declares that subsequent
	// non-terminal content is unavailable. Avoid duplicate notices once the
	// fixed budget has been exhausted.
	return budget, nil, nil
}

func serializedUpdateBytes(update *acpsdk.SessionUpdate) (int, error) {
	encoded, err := json.Marshal(update)
	if err != nil {
		return 0, fmt.Errorf("%w: serialize child projection: %v", ErrMalformedRecord, err)
	}
	return len(encoded), nil
}

func childProjectionElision(parentItemID string, kind childProjectionElisionKind) (*acpsdk.SessionUpdate, int) {
	text := "Worker child content was elided because its per-child projection limit was reached; subsequent non-terminal content is omitted."
	switch kind {
	case childProjectionElisionFailure:
		text = "Worker error content was elided because its per-child projection limit was reached."
	}
	update := childTextUpdate(parentItemID, text)
	return update, serializedKnownChildUpdateBytes(update)
}

func childStreamGapElision(item chatsessions.SequencedItem, parentItemID string) (*acpsdk.SessionUpdate, int, error) {
	var payload workers.StreamGapPayload
	if err := decodeChildPayload(item.Payload, &payload, "StreamGapPayload"); err != nil {
		return nil, 0, err
	}
	if err := validateStreamGapPayload(payload); err != nil {
		return nil, 0, err
	}
	payload.Reason = ""
	text := gapNoticeText(payload) + " Additional stream-gap detail was elided because its per-child projection limit was reached."
	update := childTextUpdate(parentItemID, text)
	return update, serializedKnownChildUpdateBytes(update), nil
}

// serializedKnownChildUpdateBytes handles updates composed exclusively from
// static string-backed ACP fields. Unlike provider-originated RawInput and
// RawOutput (handled by serializedUpdateBytes), this shape cannot contain an
// unsupported JSON value, so its size is deterministic and cannot fail.
func serializedKnownChildUpdateBytes(update *acpsdk.SessionUpdate) int {
	encoded, _ := json.Marshal(update)
	return len(encoded)
}
