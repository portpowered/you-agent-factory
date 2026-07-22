package inferencecontract

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const maxEventPayloadBytes = 64 * 1024

// ProtocolError identifies a provider violation of the invocation lifecycle.
type ProtocolError struct {
	Rule    string
	message string
}

func (e *ProtocolError) Error() string { return e.message }

func protocolError(rule, message string) error {
	return &ProtocolError{Rule: rule, message: message}
}

// ExecuteInvocation invokes an integration through a validating response
// writer. The destination receives only ordered, correlated drafts followed by
// at most one authoritative completion.
func ExecuteInvocation(
	ctx context.Context,
	integration Integration,
	request InvocationRequest,
	destination ResponseWriter,
) error {
	if integration == nil {
		return protocolError("integration", "provider integration is required")
	}
	if destination == nil {
		return protocolError("destination", "response destination is required")
	}
	writer := newProtocolWriter(request.InvocationID(), integration.Identity(), destination)
	invokeErr := integration.Invoke(ctx, request, writer)
	return writer.finish(ctx, invokeErr)
}

type lifecycleState struct {
	started  bool
	terminal bool
	toolName string
}

type protocolWriter struct {
	mu                 sync.Mutex
	destination        ResponseWriter
	invocationID       string
	provider           Identity
	closed             bool
	terminalBuffer     []EventDraft
	bufferTerminalTail bool
	lifecycles         map[string]lifecycleState
	finalMessage       string
	hasFinalMessage    bool
	terminalErr        error
}

func newProtocolWriter(invocationID string, provider Identity, destination ResponseWriter) *protocolWriter {
	return &protocolWriter{
		destination:  destination,
		invocationID: invocationID,
		provider:     provider,
		lifecycles:   make(map[string]lifecycleState),
	}
}

func (w *protocolWriter) WriteEvent(ctx context.Context, event EventDraft) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		err := protocolError("write_after_close", "response event cannot be written after completion")
		w.terminalErr = errors.Join(w.terminalErr, err)
		return err
	}
	draft := event.Draft()
	if err := w.validateDraft(draft); err != nil {
		return err
	}
	if w.bufferTerminalTail || beginsTerminalTail(draft) {
		w.bufferTerminalTail = true
		w.terminalBuffer = append(w.terminalBuffer, event)
		return nil
	}
	if err := w.destination.WriteEvent(ctx, event); err != nil {
		w.closed = true
		w.terminalErr = err
		return err
	}
	return nil
}

func (w *protocolWriter) Close(ctx context.Context, completion Completion) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.closeLocked(ctx, completion)
}

func (w *protocolWriter) closeLocked(ctx context.Context, completion Completion) error {
	if w.closed {
		err := protocolError("duplicate_close", "response completion may be closed exactly once")
		w.terminalErr = errors.Join(w.terminalErr, err)
		return err
	}
	w.closed = true
	if err := w.validateCompletion(completion); err != nil {
		w.terminalErr = err
		return err
	}
	for _, event := range w.terminalBuffer {
		if err := w.destination.WriteEvent(ctx, event); err != nil {
			w.terminalErr = err
			return err
		}
	}
	if err := w.destination.Close(ctx, completion); err != nil {
		w.terminalErr = err
		return err
	}
	return nil
}

func (w *protocolWriter) finish(ctx context.Context, invokeErr error) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		if invokeErr == nil || errors.Is(invokeErr, w.terminalErr) {
			return w.terminalErr
		}
		return errors.Join(invokeErr, w.terminalErr)
	}
	if invokeErr != nil {
		closeErr := w.closeLocked(ctx, FailedCompletion(normalizeInvocationError(ctx, invokeErr)))
		return errors.Join(invokeErr, closeErr)
	}
	missingClose := protocolError("missing_close", "provider invocation returned without closing the response")
	closeErr := w.closeLocked(ctx, FailedCompletion(NewFailure(FailureInput{
		Kind:    FailureMalformedOutput,
		Message: "provider invocation completed without a response",
	})))
	return errors.Join(missingClose, closeErr)
}

func (w *protocolWriter) validateDraft(draft workers.Draft) error {
	if len(draft.Payload) > maxEventPayloadBytes {
		return protocolError("payload_bound", fmt.Sprintf("response event payload exceeds %d bytes", maxEventPayloadBytes))
	}
	if err := workers.ValidateDraft(draft); err != nil {
		return protocolError("draft", fmt.Sprintf("invalid response event draft: %v", err))
	}
	if draft.Kind == workers.KindStreamGap {
		return protocolError("draft_kind", "STREAM_GAP is reserved for Factory Session publication")
	}
	if draft.RunID != w.invocationID {
		return protocolError("invocation_correlation", "response event run ID must match the invocation ID")
	}
	if err := w.validateProvenance(draft.Provenance); err != nil {
		return err
	}
	key, toolName, err := lifecycleKey(draft)
	if err != nil {
		return err
	}
	if key != "" {
		if err := w.advanceLifecycle(key, toolName, draft.Kind, draft.Phase); err != nil {
			return err
		}
	}
	if draft.Kind == workers.KindMessage && draft.Phase == workers.PhaseCompleted {
		content, represented, err := authoritativeMessageContent(draft.Payload)
		if err != nil {
			return err
		}
		if represented {
			w.finalMessage = content
			w.hasFinalMessage = true
		}
	}
	return nil
}

func (w *protocolWriter) validateProvenance(provenance workers.Provenance) error {
	if provenance.Provider != string(w.provider) {
		return protocolError("provenance_provider", "response event provider must match the integration identity")
	}
	if provenance.Delivery != workers.DeliveryNativeStream && provenance.Delivery != workers.DeliveryNativeFinal &&
		provenance.Delivery != workers.DeliverySynthesized {
		return protocolError("provenance_delivery", "response event has an invalid provider-writable delivery")
	}
	if provenance.Fidelity != workers.FidelityLossless && provenance.Fidelity != workers.FidelityNormalized &&
		provenance.Fidelity != workers.FidelityLossy && provenance.Fidelity != workers.FidelityFinalOnly &&
		provenance.Fidelity != workers.FidelityLifecycleOnly {
		return protocolError("provenance_fidelity", "response event has an invalid fidelity")
	}
	if provenance.Representation != workers.RepresentationDelta && provenance.Representation != workers.RepresentationSnapshot &&
		provenance.Representation != workers.RepresentationNotification {
		return protocolError("provenance_representation", "response event has an invalid representation")
	}
	if err := validatePublicText("event.provenance.nativeEventType", provenance.NativeEventType, maxDiagnosticValueLength); err != nil {
		return err
	}
	return nil
}

func lifecycleKey(draft workers.Draft) (string, string, error) {
	switch draft.Kind {
	case workers.KindSession:
		return requiredCorrelation("session", draft.ProviderSessionRef)
	case workers.KindRun:
		return "run:" + draft.RunID, "", nil
	case workers.KindTurn:
		return requiredCorrelation("turn", draft.TurnID)
	case workers.KindMessage, workers.KindReasoning:
		return requiredCorrelation(strings.ToLower(string(draft.Kind)), draft.ItemID)
	case workers.KindTool:
		return toolCorrelation(draft)
	default:
		return "", "", nil
	}
}

func requiredCorrelation(kind, value string) (string, string, error) {
	if strings.TrimSpace(value) == "" {
		return "", "", protocolError("event_correlation", kind+" lifecycle requires stable correlation")
	}
	return kind + ":" + value, "", nil
}

func toolCorrelation(draft workers.Draft) (string, string, error) {
	if draft.Phase == workers.PhaseDelta {
		var payload workers.ToolDeltaPayload
		if err := json.Unmarshal(draft.Payload, &payload); err != nil {
			return "", "", protocolError("tool_payload", "tool delta payload could not be decoded")
		}
		return "tool:" + payload.ToolCallID, "", nil
	}
	var payload workers.ToolPayload
	if err := json.Unmarshal(draft.Payload, &payload); err != nil {
		return "", "", protocolError("tool_payload", "tool payload could not be decoded")
	}
	return "tool:" + payload.ToolCallID, payload.ToolName, nil
}

func (w *protocolWriter) advanceLifecycle(key, toolName string, kind workers.Kind, phase workers.Phase) error {
	state := w.lifecycles[key]
	switch phase {
	case workers.PhaseStarted:
		if state.started || state.terminal {
			return protocolError("duplicate_start", "response event lifecycle may start only once")
		}
		state.started = true
		state.toolName = toolName
	case workers.PhaseDelta:
		if !state.started || state.terminal {
			return protocolError("update_before_start", "response event update requires an active lifecycle")
		}
	case workers.PhaseCompleted, workers.PhaseFailed, workers.PhaseCanceled:
		if state.terminal {
			return protocolError("duplicate_terminal", "response event lifecycle may terminate only once")
		}
		if !state.started && !(kind == workers.KindMessage && phase == workers.PhaseCompleted) {
			return protocolError("terminal_before_start", "response event terminal phase requires a start")
		}
		if kind == workers.KindTool && state.toolName != toolName {
			return protocolError("tool_correlation", "tool name must remain stable for one tool correlation")
		}
		state.terminal = true
	}
	w.lifecycles[key] = state
	return nil
}

func (w *protocolWriter) validateCompletion(completion Completion) error {
	response, failure := completion.Response(), completion.Failure()
	if (response == nil) == (failure == nil) {
		return protocolError("completion_outcome", "completion requires exactly one response or failure")
	}
	if failure != nil {
		return ValidateFailure(*failure)
	}
	for _, state := range w.lifecycles {
		if state.started && !state.terminal {
			return protocolError("incomplete_lifecycle", "completion requires every started event lifecycle to terminate")
		}
	}
	if strings.TrimSpace(response.Content()) == "" {
		return protocolError("response_content", "authoritative response content is required")
	}
	if w.hasFinalMessage && w.finalMessage != response.Content() {
		return protocolError("final_result_agreement", "completed message content must agree with the authoritative response")
	}
	return nil
}

func authoritativeMessageContent(payload []byte) (string, bool, error) {
	var message workers.MessagePayload
	if err := json.Unmarshal(payload, &message); err != nil {
		return "", false, protocolError("message_payload", "completed message payload could not be decoded")
	}
	if !workers.IsAuthoritativeMessageSnapshot(message) {
		return "", false, nil
	}
	var content strings.Builder
	represented := false
	for _, block := range message.ContentBlocks {
		if block.Kind == workers.ContentBlockText {
			content.WriteString(block.Text)
			represented = true
		}
	}
	return content.String(), represented, nil
}

func beginsTerminalTail(draft workers.Draft) bool {
	return draft.Kind == workers.KindMessage && draft.Phase == workers.PhaseCompleted
}

func normalizeInvocationError(ctx context.Context, invokeErr error) Failure {
	kind := FailureUnknown
	message := "provider invocation failed"
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(invokeErr, context.DeadlineExceeded) {
		kind = FailureTimeout
		message = "provider invocation timed out"
	} else if errors.Is(ctx.Err(), context.Canceled) || errors.Is(invokeErr, context.Canceled) {
		kind = FailureCanceled
		message = "provider invocation was canceled"
	}
	return NewFailure(FailureInput{Kind: kind, Message: message})
}
