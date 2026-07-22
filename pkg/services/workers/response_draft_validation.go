package workers

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// ValidationError identifies the response-event field that failed validation.
type ValidationError struct {
	Field   string
	message string
}

func (e *ValidationError) Error() string {
	return e.message
}

func validationError(field, message string) error {
	return &ValidationError{Field: field, message: message}
}

var allowedPhasesByKind = map[Kind][]Phase{
	KindSession:    {PhaseStarted, PhaseCompleted, PhaseFailed, PhaseCanceled},
	KindRun:        {PhaseStarted, PhaseCompleted, PhaseFailed, PhaseCanceled},
	KindTurn:       {PhaseStarted, PhaseCompleted, PhaseFailed, PhaseCanceled},
	KindMessage:    {PhaseStarted, PhaseDelta, PhaseCompleted},
	KindReasoning:  {PhaseStarted, PhaseDelta, PhaseCompleted},
	KindTool:       {PhaseStarted, PhaseDelta, PhaseCompleted, PhaseFailed, PhaseCanceled},
	KindFileChange: {PhaseUpdated},
	KindPlan:       {PhaseUpdated},
	KindProgress:   {PhaseUpdated},
	KindUsage:      {PhaseUpdated},
	KindError:      {PhaseUpdated, PhaseFailed},
	KindStreamGap:  {PhaseUpdated},
}

var knownContentBlockKinds = []ContentBlockKind{
	ContentBlockText,
	ContentBlockReasoningSummary,
	ContentBlockToolRequest,
	ContentBlockImageRef,
	ContentBlockResourceRef,
	ContentBlockStructuredOutput,
}

// ValidateDraft checks a provider draft before it crosses the publication
// boundary. Publication-owned identity, sequence, and time are intentionally
// absent from this validation input.
func ValidateDraft(draft Draft) error {
	if !isKnownKind(draft.Kind) {
		return validationError("kind", fmt.Sprintf("kind %q is not a declared response draft kind", draft.Kind))
	}
	if err := validateKindPhase(draft.Kind, draft.Phase); err != nil {
		return err
	}
	return validatePayload(draft.Kind, draft.Phase, draft.Payload)
}

func validateKindPhase(kind Kind, phase Phase) error {
	allowed, ok := allowedPhasesByKind[kind]
	if !ok {
		return validationError("kind", fmt.Sprintf("kind %q has no declared phase allow-list", kind))
	}
	if !slices.Contains(allowed, phase) {
		return validationError(
			"phase",
			fmt.Sprintf(
				"phase %q is not allowed for kind %q; allowed phases: %s",
				phase,
				kind,
				formatPhaseList(allowed),
			),
		)
	}
	return nil
}

func validatePayload(kind Kind, phase Phase, payload json.RawMessage) error {
	if err := validatePayloadBasics(kind, payload); err != nil {
		return err
	}

	switch kind {
	case KindSession:
		var body SessionPayload
		return decodePayload(payload, &body, "SessionPayload")
	case KindRun:
		var body RunPayload
		return decodePayload(payload, &body, "RunPayload")
	case KindTurn:
		var body TurnPayload
		return decodePayload(payload, &body, "TurnPayload")
	case KindMessage:
		return validateMessageKindPayload(phase, payload)
	case KindReasoning:
		var body ReasoningPayload
		return decodePayload(payload, &body, "ReasoningPayload")
	case KindTool:
		return validateToolKindPayload(phase, payload)
	case KindFileChange:
		return validateFileChangeKindPayload(payload)
	case KindPlan:
		var body PlanPayload
		return decodePayload(payload, &body, "PlanPayload")
	case KindProgress:
		return validateProgressKindPayload(payload)
	case KindUsage:
		var body UsagePayload
		return decodePayload(payload, &body, "UsagePayload")
	case KindError:
		return validateErrorKindPayload(payload)
	case KindStreamGap:
		return validateStreamGapKindPayload(payload)
	default:
		return validationError("kind", fmt.Sprintf("kind %q has no declared payload validator", kind))
	}
}

func validatePayloadBasics(kind Kind, payload json.RawMessage) error {
	if len(payload) == 0 {
		return validationError("payload", fmt.Sprintf("payload is required for kind %q", kind))
	}
	if !json.Valid(payload) {
		return validationError("payload", "payload must contain valid JSON")
	}
	return nil
}

func validateMessageKindPayload(phase Phase, payload json.RawMessage) error {
	if phase == PhaseDelta {
		if err := rejectPayloadKeys(payload, "MessageDeltaPayload", "role", "contentBlocks"); err != nil {
			return err
		}
		var body MessageDeltaPayload
		if err := decodePayload(payload, &body, "MessageDeltaPayload"); err != nil {
			return err
		}
		return validateMessageDeltaPayload(body)
	}
	if err := rejectPayloadKeys(payload, "MessagePayload", "contentBlockIndex", "contentBlockKind", "textDelta"); err != nil {
		return err
	}
	var body MessagePayload
	if err := decodePayload(payload, &body, "MessagePayload"); err != nil {
		return err
	}
	return validateMessagePayload(body)
}

func validateToolKindPayload(phase Phase, payload json.RawMessage) error {
	if phase == PhaseDelta {
		if err := rejectPayloadKeys(payload, "ToolDeltaPayload", "toolName", "status", "argumentsSummary", "resultSummary"); err != nil {
			return err
		}
		var body ToolDeltaPayload
		if err := decodePayload(payload, &body, "ToolDeltaPayload"); err != nil {
			return err
		}
		return validateToolDeltaPayload(body)
	}
	if err := rejectPayloadKeys(payload, "ToolPayload", "outputDelta"); err != nil {
		return err
	}
	var body ToolPayload
	if err := decodePayload(payload, &body, "ToolPayload"); err != nil {
		return err
	}
	return validateToolPayload(body)
}

func validateFileChangeKindPayload(payload json.RawMessage) error {
	var body FileChangePayload
	if err := decodePayload(payload, &body, "FileChangePayload"); err != nil {
		return err
	}
	if strings.TrimSpace(body.Path) == "" {
		return validationError("payload.path", "path is required for FileChangePayload")
	}
	if strings.TrimSpace(body.Operation) == "" {
		return validationError("payload.operation", "operation is required for FileChangePayload")
	}
	return nil
}

func validateProgressKindPayload(payload json.RawMessage) error {
	var body ProgressPayload
	if err := decodePayload(payload, &body, "ProgressPayload"); err != nil {
		return err
	}
	if strings.TrimSpace(body.Label) == "" {
		return validationError("payload.label", "label is required for ProgressPayload")
	}
	return nil
}

func validateErrorKindPayload(payload json.RawMessage) error {
	var body ErrorPayload
	if err := decodePayload(payload, &body, "ErrorPayload"); err != nil {
		return err
	}
	if strings.TrimSpace(body.Code) == "" {
		return validationError("payload.code", "code is required for ErrorPayload")
	}
	if strings.TrimSpace(body.Message) == "" {
		return validationError("payload.message", "message is required for ErrorPayload")
	}
	return nil
}

func validateStreamGapKindPayload(payload json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return validationError("payload", fmt.Sprintf("payload must decode as StreamGapPayload: %v", err))
	}
	var body StreamGapPayload
	if err := decodePayload(payload, &body, "StreamGapPayload"); err != nil {
		return err
	}
	if strings.TrimSpace(body.AffectedItemID) != "" {
		if strings.TrimSpace(body.Reason) == "" {
			return validationError("payload.reason", "reason is required for an item-scoped StreamGapPayload")
		}
		if body.FromSequence != 0 || body.ToSequence != 0 || body.FirstAvailableSequence != 0 {
			return validationError("payload.fromSequence", "sequence fields are not allowed for an item-scoped StreamGapPayload")
		}
		if err := rejectItemGapSequenceFields(fields); err != nil {
			return err
		}
		return nil
	}
	if strings.TrimSpace(body.ToolCallID) != "" {
		return validationError("payload.affectedItemId", "affectedItemId is required when toolCallId is supplied for StreamGapPayload")
	}
	if err := requireRetentionGapFields(fields); err != nil {
		return err
	}
	if body.FromSequence < 0 || body.ToSequence < 0 {
		return validationError("payload.fromSequence", "fromSequence and toSequence must be non-negative for StreamGapPayload")
	}
	if body.FromSequence > body.ToSequence {
		return validationError("payload.toSequence", "toSequence must not precede fromSequence for StreamGapPayload")
	}
	if body.FirstAvailableSequence <= 0 {
		return validationError("payload.firstAvailableSequence", "firstAvailableSequence must be positive for StreamGapPayload")
	}
	return nil
}

func rejectItemGapSequenceFields(fields map[string]json.RawMessage) error {
	for _, key := range []string{"fromSequence", "toSequence", "firstAvailableSequence"} {
		if _, present := fields[key]; present {
			return validationError("payload."+key, "sequence fields are not allowed for an item-scoped StreamGapPayload")
		}
	}
	return nil
}

func requireRetentionGapFields(fields map[string]json.RawMessage) error {
	for _, key := range []string{"fromSequence", "toSequence", "firstAvailableSequence"} {
		if _, present := fields[key]; !present {
			return validationError("payload."+key, key+" is required for a retention StreamGapPayload")
		}
	}
	return nil
}

func decodePayload(payload json.RawMessage, target any, typeName string) error {
	if err := json.Unmarshal(payload, target); err != nil {
		return validationError(
			"payload",
			fmt.Sprintf("payload must decode as %s: %v", typeName, err),
		)
	}
	return nil
}

func rejectPayloadKeys(payload json.RawMessage, typeName string, forbiddenKeys ...string) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return validationError("payload", fmt.Sprintf("payload must decode as %s: %v", typeName, err))
	}
	for _, key := range forbiddenKeys {
		if _, ok := fields[key]; ok {
			return validationError(
				"payload",
				fmt.Sprintf("payload shape is incompatible with %s; remove field %q", typeName, key),
			)
		}
	}
	return nil
}

func validateMessagePayload(payload MessagePayload) error {
	if strings.TrimSpace(payload.Role) == "" {
		return validationError("payload.role", "role is required for MessagePayload")
	}
	if len(payload.ContentBlocks) == 0 {
		return validationError("payload.contentBlocks", "contentBlocks must contain at least one block for MessagePayload")
	}
	for index, block := range payload.ContentBlocks {
		if err := validateContentBlock(block); err != nil {
			if validationErr, ok := err.(*ValidationError); ok {
				return validationError(
					fmt.Sprintf("payload.contentBlocks[%d].%s", index, validationErr.Field),
					validationErr.message,
				)
			}
			return err
		}
	}
	return nil
}

func validateMessageDeltaPayload(payload MessageDeltaPayload) error {
	if payload.ContentBlockIndex < 0 {
		return validationError("payload.contentBlockIndex", "contentBlockIndex must be non-negative for MessageDeltaPayload")
	}
	if err := validateContentBlockKind(payload.ContentBlockKind); err != nil {
		if validationErr, ok := err.(*ValidationError); ok {
			return validationError("payload."+validationErr.Field, validationErr.message)
		}
		return err
	}
	return nil
}

func validateToolPayload(payload ToolPayload) error {
	if strings.TrimSpace(payload.ToolCallID) == "" {
		return validationError("payload.toolCallId", "toolCallId is required for ToolPayload")
	}
	if strings.TrimSpace(payload.ToolName) == "" {
		return validationError("payload.toolName", "toolName is required for ToolPayload")
	}
	return nil
}

func validateToolDeltaPayload(payload ToolDeltaPayload) error {
	if strings.TrimSpace(payload.ToolCallID) == "" {
		return validationError("payload.toolCallId", "toolCallId is required for ToolDeltaPayload")
	}
	if payload.OutputDelta == "" {
		return validationError("payload.outputDelta", "outputDelta is required for ToolDeltaPayload")
	}
	return nil
}

func validateContentBlock(block ContentBlock) error {
	if err := validateContentBlockKind(block.Kind); err != nil {
		return validationError("kind", err.Error())
	}
	switch block.Kind {
	case ContentBlockText:
		// Empty text is a valid authoritative snapshot when the provider
		// explicitly completes a message with an empty final value.
	case ContentBlockReasoningSummary:
		if strings.TrimSpace(block.Text) == "" {
			return validationError("text", fmt.Sprintf("text is required for content block kind %q", block.Kind))
		}
	case ContentBlockToolRequest:
		if strings.TrimSpace(block.ToolCallID) == "" {
			return validationError("toolCallId", "toolCallId is required for TOOL_REQUEST content blocks")
		}
		if strings.TrimSpace(block.ToolName) == "" {
			return validationError("toolName", "toolName is required for TOOL_REQUEST content blocks")
		}
	case ContentBlockImageRef:
		if strings.TrimSpace(block.ImageRef) == "" {
			return validationError("imageRef", "imageRef is required for IMAGE_REF content blocks")
		}
	case ContentBlockResourceRef:
		if strings.TrimSpace(block.ResourceRef) == "" {
			return validationError("resourceRef", "resourceRef is required for RESOURCE_REF content blocks")
		}
	case ContentBlockStructuredOutput:
		if len(block.StructuredOutput) == 0 || !json.Valid(block.StructuredOutput) {
			return validationError("structuredOutput", "structuredOutput must contain valid JSON for STRUCTURED_OUTPUT content blocks")
		}
	}
	return nil
}

func validateContentBlockKind(kind ContentBlockKind) error {
	if !slices.Contains(knownContentBlockKinds, kind) {
		return validationError(
			"contentBlockKind",
			fmt.Sprintf(
				"contentBlockKind %q is not declared; allowed kinds: %s",
				kind,
				formatContentBlockKindList(knownContentBlockKinds),
			),
		)
	}
	return nil
}

func isKnownKind(kind Kind) bool {
	_, ok := allowedPhasesByKind[kind]
	return ok
}

func formatPhaseList(phases []Phase) string {
	values := make([]string, len(phases))
	for index, phase := range phases {
		values[index] = string(phase)
	}
	return strings.Join(values, ", ")
}

func formatContentBlockKindList(kinds []ContentBlockKind) string {
	values := make([]string, len(kinds))
	for index, kind := range kinds {
		values[index] = string(kind)
	}
	return strings.Join(values, ", ")
}
