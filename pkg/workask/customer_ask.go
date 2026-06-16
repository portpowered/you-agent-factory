package workask

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

const (
	// WorkTypeIdea is the canonical work type id for customer ideation intake.
	WorkTypeIdea = "idea"
)

// CustomerAskRequiredMessage is the stable customer-readable validation copy for
// empty idea asks surfaced by API and CLI submission paths.
const CustomerAskRequiredMessage = "requires a customer ask for idea work"

// CustomerAskText returns authored text from canonical content parts and any
// legacy text payload projection.
func CustomerAskText(content []interfaces.WorkContentPart, rawPayload []byte) string {
	var builder strings.Builder
	for _, part := range content {
		if part.Type.Normalized() == interfaces.WorkContentPartTypeText {
			builder.WriteString(part.Text)
		}
	}
	if text, ok := textPayloadBytes(rawPayload); ok {
		builder.WriteString(text)
	}
	return builder.String()
}

// IsEmptyCustomerAsk reports whether idea work is missing a customer ask after
// trimming surrounding whitespace from authored text. Non-text payloads such as
// structured JSON objects are treated as present asks.
func IsEmptyCustomerAsk(content []interfaces.WorkContentPart, rawPayload []byte) bool {
	if hasNonTextAskPayload(rawPayload) {
		return false
	}
	return strings.TrimSpace(CustomerAskText(content, rawPayload)) == ""
}

// ValidateIdeaCustomerAsk rejects blank or whitespace-only customer asks for
// idea work using the canonical empty-ask rule shared by intake and execution.
func ValidateIdeaCustomerAsk(workIndex int, workName string, content []interfaces.WorkContentPart, rawPayload []byte) error {
	if !IsEmptyCustomerAsk(content, rawPayload) {
		return nil
	}
	return fmt.Errorf("work_request: works[%d] (%q) %s", workIndex, workName, CustomerAskRequiredMessage)
}

// ValidateIdeaWorkInBatch validates one batch work item when it targets idea
// work. Callers pass the public batch index and work record before queueing.
func ValidateIdeaWorkInBatch(workIndex int, work interfaces.Work) error {
	if work.WorkTypeID != WorkTypeIdea {
		return nil
	}
	rawPayload, err := payloadBytes(work.Payload)
	if err != nil {
		return fmt.Errorf("work_request: works[%d] (%q) has invalid payload: %w", workIndex, work.Name, err)
	}
	return ValidateIdeaCustomerAsk(workIndex, work.Name, work.Content, rawPayload)
}

func hasNonTextAskPayload(rawPayload []byte) bool {
	if len(rawPayload) == 0 {
		return false
	}
	if _, ok := textPayloadBytes(rawPayload); ok {
		return false
	}
	return strings.TrimSpace(string(rawPayload)) != ""
}

func textPayloadBytes(rawPayload []byte) (string, bool) {
	if len(rawPayload) == 0 {
		return "", false
	}

	var text string
	if err := json.Unmarshal(rawPayload, &text); err == nil {
		return text, true
	}
	if json.Valid(rawPayload) {
		return "", false
	}
	return string(rawPayload), true
}

func payloadBytes(payload any) ([]byte, error) {
	switch value := payload.(type) {
	case nil:
		return nil, nil
	case []byte:
		return append([]byte(nil), value...), nil
	case json.RawMessage:
		return append([]byte(nil), value...), nil
	case string:
		return []byte(value), nil
	default:
		return json.Marshal(value)
	}
}
