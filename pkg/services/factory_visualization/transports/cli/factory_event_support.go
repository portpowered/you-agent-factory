package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
	"unicode/utf8"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const (
	factoryEventJSONRecordType                 = "factory_event"
	factoryEventJSONInvocationResultRecordType = "invocation_result"

	responseStreamPrimaryResultHeader     = "--- primary result ---"
	responseStreamInvocationOutcomeHeader = "--- invocation outcome ---"
	maxHumanProgressLineBytes             = 1024
)

type factoryEventJSONRecord struct {
	RecordType string                  `json:"recordType"`
	Event      interfaces.FactoryEvent `json:"event"`
}

type factoryEventJSONInvocationResultRecord struct {
	RecordType string                        `json:"recordType"`
	Response   factoryapi.InvocationResponse `json:"response"`
}

func jsonMarshalFactoryEventRecord(event interfaces.FactoryEvent) ([]byte, error) {
	return json.Marshal(factoryEventJSONRecord{
		RecordType: factoryEventJSONRecordType,
		Event:      event,
	})
}

func writeJSONInvocationResultRecord(
	writer io.Writer,
	result apisurface.FactoryInvocationResult,
) error {
	encoded, encodeErr := json.Marshal(factoryEventJSONInvocationResultRecord{
		RecordType: factoryEventJSONInvocationResultRecordType,
		Response:   apisurface.InvocationResponseFromResult(result),
	})
	if encodeErr != nil {
		return fmt.Errorf("marshal Factory Event terminal record: %w", encodeErr)
	}
	encoded = append(encoded, '\n')
	written, writeErr := writer.Write(encoded)
	if writeErr == nil && written != len(encoded) {
		writeErr = io.ErrShortWrite
	}
	return writeErr
}

func factoryEventForPublicPresentation(event interfaces.FactoryEvent) (interfaces.FactoryEvent, bool) {
	var payload any
	decoder := json.NewDecoder(bytes.NewReader(event.Payload))
	decoder.UseNumber()
	if len(event.Payload) == 0 || decoder.Decode(&payload) != nil {
		event.Payload = json.RawMessage(`{}`)
		return event, true
	}
	payload = redactPrivateFactoryEventPayload(payload)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return interfaces.FactoryEvent{}, false
	}
	event.Payload = encoded
	return event, true
}

func redactPrivateFactoryEventPayload(value any) any {
	if list, ok := value.([]any); ok {
		for index, child := range list {
			list[index] = redactPrivateFactoryEventPayload(child)
		}
		return list
	}
	object, ok := value.(map[string]any)
	if !ok {
		return value
	}
	if object["schemaVersion"] == string(factorysessions.ResponseEventSchemaVersionV1) {
		return map[string]any{}
	}
	for _, key := range []string{"diagnostics", "response", "providerSession", "provider_session", "providerSessionRef", "textDelta", "toolCallId", "toolCalls"} {
		delete(object, key)
	}
	for key, child := range object {
		object[key] = redactPrivateFactoryEventPayload(child)
	}
	return object
}

func invocationPrimaryResultText(parts []work.WorkContentPart) (string, error) {
	if len(parts) == 0 {
		return "", fmt.Errorf("invocation primary result is empty")
	}

	textParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type.Normalized() != work.WorkContentPartTypeText {
			return "", fmt.Errorf("invocation primary result is not plain text; use --json")
		}
		textParts = append(textParts, part.Text)
	}
	return strings.Join(textParts, "\n"), nil
}

func writeHumanInvocationOutcome(
	output io.Writer,
	progressSeen bool,
	result apisurface.FactoryInvocationResult,
) error {
	lines := formatHumanInvocationOutcomeLines(result)

	if progressSeen {
		if _, err := fmt.Fprintln(output); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(output, responseStreamInvocationOutcomeHeader); err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(output, line); err != nil {
			return err
		}
	}
	return nil
}

func formatHumanInvocationOutcomeLines(result apisurface.FactoryInvocationResult) []string {
	lines := []string{
		"status: " + string(result.Status),
	}
	if code := strings.TrimSpace(result.ErrorCode); code != "" {
		lines = append(lines, "error: "+code)
	}
	if message := strings.TrimSpace(result.Message); message != "" {
		lines = append(lines, "message: "+message)
	}
	if sessionID := strings.TrimSpace(result.SessionID); sessionID != "" {
		lines = append(lines, "session: "+sessionID)
	}
	if workID := strings.TrimSpace(result.WorkID); workID != "" {
		lines = append(lines, "workId: "+workID)
	}
	if workName := strings.TrimSpace(result.WorkName); workName != "" {
		lines = append(lines, "workName: "+workName)
	}
	if workState := strings.TrimSpace(result.WorkState); workState != "" {
		lines = append(lines, "workState: "+workState)
	}
	return lines
}

func writeHumanPrimaryResult(output io.Writer, progressSeen bool, text string) error {
	if progressSeen {
		if _, err := fmt.Fprintln(output); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(output, responseStreamPrimaryResultHeader); err != nil {
			return err
		}
	}
	_, err := fmt.Fprint(output, text)
	return err
}

func boundedHumanProgressPayload(payload string) string {
	trimmed := normalizeHumanProgressField(payload)
	if trimmed == "" {
		return ""
	}
	if maxHumanProgressLineBytes <= 0 || len(trimmed) <= maxHumanProgressLineBytes {
		return trimmed
	}
	const omissionMarker = "..."
	budget := maxHumanProgressLineBytes - len(omissionMarker)
	if budget <= 0 {
		return omissionMarker
	}
	end := 0
	for end < len(trimmed) {
		_, size := utf8.DecodeRuneInString(trimmed[end:])
		if end+size > budget {
			break
		}
		end += size
	}
	return strings.TrimSpace(trimmed[:end]) + omissionMarker
}

func normalizeHumanProgressField(value string) string {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return ' '
		}
		return r
	}, value)
	return strings.Join(strings.Fields(normalized), " ")
}

func decodeFactoryEventPayload[T any](event interfaces.FactoryEvent) (T, bool) {
	var payload T
	if len(event.Payload) == 0 || json.Unmarshal(event.Payload, &payload) != nil {
		return payload, false
	}
	return payload, true
}

func firstFactoryEventWorkID(event interfaces.FactoryEvent) string {
	if event.Context.WorkIDs == nil || len(*event.Context.WorkIDs) == 0 {
		return ""
	}
	return (*event.Context.WorkIDs)[0]
}

func withHumanLifecycleSubject(label, subject string) string {
	if subject = boundedHumanProgressPayload(subject); subject != "" {
		return label + ": " + subject
	}
	return label
}

func withHumanLifecycleAttempt(label string, attempt int) string {
	if attempt > 0 {
		return fmt.Sprintf("%s (attempt %d)", label, attempt)
	}
	return label
}

func withHumanLifecycleFailure(message, failure string) string {
	if failure = boundedHumanProgressPayload(failure); failure != "" {
		return message + " — " + failure
	}
	return message
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatHumanFactoryEvent(event interfaces.FactoryEvent) ([]byte, bool) {
	var message string
	switch event.Type {
	case interfaces.FactoryEventTypeWorkRequest:
		message = formatHumanWorkAccepted(event)
	case interfaces.FactoryEventTypeSessionStarted:
		message = "factory started"
	case interfaces.FactoryEventTypeSessionCompleted:
		message = formatHumanSessionCompleted(event)
	case interfaces.FactoryEventTypeDispatchQueued:
		message = formatHumanDispatchQueued(event)
	case interfaces.FactoryEventTypeDispatchRequest:
		message = formatHumanDispatchStarted(event)
	case interfaces.FactoryEventTypeDispatchResponse:
		message = formatHumanDispatchCompleted(event)
	case interfaces.FactoryEventTypeDispatchInterrupted:
		message = formatHumanDispatchInterrupted(event)
	case interfaces.FactoryEventTypeInferenceRequest:
		message = formatHumanInferenceStarted(event)
	case interfaces.FactoryEventTypeInferenceResponse:
		message = formatHumanInferenceCompleted(event)
	case interfaces.FactoryEventTypeOrchestratorPhaseChanged:
		message = formatHumanOrchestratorPhase(event)
	case interfaces.FactoryEventTypeOrchestratorCheckpointWritten:
		message = formatHumanOrchestratorCheckpoint(event)
	case interfaces.FactoryEventTypeSessionResultUpdated:
		message = formatHumanResultUpdated(event)
	default:
		return nil, false
	}
	sequence := event.Context.Sequence
	if event.Context.SessionSequence != nil {
		sequence = *event.Context.SessionSequence
	}
	return []byte(fmt.Sprintf("[%d] %s", sequence, message)), true
}

func formatHumanWorkAccepted(event interfaces.FactoryEvent) string {
	payload, ok := decodeFactoryEventPayload[work.WorkRequestEventPayload](event)
	if !ok || len(payload.Works) == 0 {
		return withHumanLifecycleSubject("work accepted", firstFactoryEventWorkID(event))
	}
	if len(payload.Works) > 1 {
		return fmt.Sprintf("work accepted: %d items", len(payload.Works))
	}
	subject := payload.Works[0].Name
	if content := workContentSummary(payload.Works[0].Content); content != "" &&
		(strings.TrimSpace(subject) == "" || subject == payload.Works[0].WorkID || strings.HasPrefix(subject, "work-")) {
		subject = content
	} else if strings.TrimSpace(subject) == "" {
		subject = payload.Works[0].WorkID
	}
	return withHumanLifecycleSubject("work accepted", subject)
}

func workContentSummary(parts []work.WorkContentPart) string {
	for _, part := range parts {
		if part.Type.Normalized() == work.WorkContentPartTypeText {
			if text := boundedHumanProgressPayload(part.Text); text != "" {
				return text
			}
		}
	}
	return ""
}

func formatHumanSessionCompleted(event interfaces.FactoryEvent) string {
	payload, ok := decodeFactoryEventPayload[interfaces.FactorySessionCompletedEventPayload](event)
	if !ok || payload.FinalStatus == "" {
		return "factory completed"
	}
	message := "factory completed: " + string(payload.FinalStatus)
	if payload.FailureDetail != nil {
		message = withHumanLifecycleFailure(message, payload.FailureDetail.Message)
	}
	return message
}

func formatHumanDispatchQueued(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[interfaces.DispatchQueuedEventPayload](event)
	subject := stringPointerValue(payload.Label)
	if subject == "" {
		subject = stringPointerValue(event.Context.DispatchID)
	}
	return withHumanLifecycleSubject("workstation queued", subject)
}

func formatHumanDispatchStarted(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[interfaces.DispatchRequestEventPayload](event)
	return withHumanLifecycleSubject("workstation started", payload.TransitionID)
}

func formatHumanDispatchCompleted(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[workerexecution.DispatchResponseEventPayload](event)
	label := "workstation completed"
	if payload.Outcome == workerexecution.OutcomeFailed {
		label = "workstation failed"
	}
	message := withHumanLifecycleSubject(label, payload.TransitionID)
	if payload.Outcome != "" && payload.Outcome != workerexecution.OutcomeAccepted && payload.Outcome != workerexecution.OutcomeFailed {
		message += " (" + string(payload.Outcome) + ")"
	}
	if payload.FailureDetail != nil {
		message = withHumanLifecycleFailure(message, payload.FailureDetail.Message)
	}
	return message
}

func formatHumanDispatchInterrupted(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[interfaces.DispatchInterruptedEventPayload](event)
	return withHumanLifecycleFailure(
		withHumanLifecycleSubject("workstation interrupted", stringPointerValue(event.Context.DispatchID)),
		payload.Reason,
	)
}

func formatHumanInferenceStarted(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[workerexecution.InferenceRequestEventPayload](event)
	return withHumanLifecycleAttempt("inference started", payload.Attempt)
}

func formatHumanInferenceCompleted(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[workerexecution.InferenceResponseEventPayload](event)
	label := "inference completed"
	if payload.Outcome == workerexecution.InferenceOutcomeFailed {
		label = "inference failed"
	}
	message := withHumanLifecycleAttempt(label, payload.Attempt)
	if payload.FailureDetail != nil {
		message = withHumanLifecycleFailure(message, payload.FailureDetail.Message)
	}
	return message
}

func formatHumanOrchestratorPhase(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[interfaces.OrchestratorPhaseChangedEventPayload](event)
	message := "workflow phase"
	if phase := boundedHumanProgressPayload(stringPointerValue(event.Context.PhaseName)); phase != "" {
		message += " " + phase
	}
	if payload.PhaseStatus != "" {
		message += ": " + string(payload.PhaseStatus)
	}
	return message
}

func formatHumanOrchestratorCheckpoint(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[interfaces.OrchestratorCheckpointWrittenEventPayload](event)
	message := withHumanLifecycleSubject("workflow checkpoint written", payload.Label)
	if payload.ResumabilityStatus != "" {
		message += " (" + string(payload.ResumabilityStatus) + ")"
	}
	return message
}

func formatHumanResultUpdated(event interfaces.FactoryEvent) string {
	payload, _ := decodeFactoryEventPayload[interfaces.FactorySessionResultUpdatedEventPayload](event)
	message := "final output updated"
	if payload.ResultStatus != "" {
		message += ": " + string(payload.ResultStatus)
	}
	return message
}
