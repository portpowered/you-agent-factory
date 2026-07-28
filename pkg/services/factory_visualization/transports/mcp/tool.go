package factoryvisualization

import (
	"context"
	"encoding/base64"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
)

// Tool names use Factory Visualization vocabulary.
const (
	ToolActivate             = "you.factory_visualization.activate"
	ToolJoin                 = "you.factory_visualization.join"
	ToolStopDrain            = "you.factory_visualization.stop_drain"
	ToolObserve              = "you.factory_visualization.observe"
	ToolOpenPresentation     = "you.factory_visualization.open_presentation"
	ToolPresentProgress      = "you.factory_visualization.present_progress"
	ToolFinalizePresentation = "you.factory_visualization.finalize_presentation"
	ToolClosePresentation    = "you.factory_visualization.close_presentation"
)

// ToolErrorEnvelope is the stable MCP failure shape for Visualization tools.
type ToolErrorEnvelope struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Retryable bool           `json:"retryable"`
	Details   map[string]any `json:"details,omitempty"`
}

// ToolResponse wraps one tool outcome with either a typed result or a stable error.
type ToolResponse[T any] struct {
	Result *T                 `json:"result,omitempty"`
	Error  *ToolErrorEnvelope `json:"error,omitempty"`
}

// ActivateInput is the MCP request shape for you.factory_visualization.activate.
type ActivateInput struct {
	Mode string `json:"mode"`
}

// Activate runs the Visualization root Activate contract for the activate MCP tool.
func Activate(
	ctx context.Context,
	root factoryvisualization.Root,
	input ActivateInput,
) ToolResponse[factoryvisualization.ActivateResult] {
	if ctx == nil {
		envelope := missingContextErrorEnvelope()
		return ToolResponse[factoryvisualization.ActivateResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryvisualization.ActivateResult](ctx); done {
		return response
	}
	if root == nil {
		envelope := serviceUnavailableErrorEnvelope()
		return ToolResponse[factoryvisualization.ActivateResult]{Error: &envelope}
	}
	if input.Mode == "" {
		envelope := requestValidationErrorEnvelope("activate Factory visualization: required request parameters are missing")
		return ToolResponse[factoryvisualization.ActivateResult]{Error: &envelope}
	}
	result, err := root.Activate(ctx, factoryvisualization.ActivateRequest{
		Mode: factoryvisualization.ActivateMode(input.Mode),
	})
	if err != nil {
		envelope := mapRootError(err)
		return ToolResponse[factoryvisualization.ActivateResult]{Error: &envelope}
	}
	return ToolResponse[factoryvisualization.ActivateResult]{Result: &result}
}

// JoinInput is the MCP request shape for you.factory_visualization.join.
type JoinInput struct{}

// Join runs the Visualization root Join contract for the join MCP tool.
func Join(
	ctx context.Context,
	root factoryvisualization.Root,
	input JoinInput,
) ToolResponse[factoryvisualization.JoinResult] {
	if ctx == nil {
		envelope := missingContextErrorEnvelope()
		return ToolResponse[factoryvisualization.JoinResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryvisualization.JoinResult](ctx); done {
		return response
	}
	if root == nil {
		envelope := serviceUnavailableErrorEnvelope()
		return ToolResponse[factoryvisualization.JoinResult]{Error: &envelope}
	}
	result, err := root.Join(ctx, factoryvisualization.JoinRequest{})
	if err != nil {
		envelope := mapRootError(err)
		return ToolResponse[factoryvisualization.JoinResult]{Error: &envelope}
	}
	return ToolResponse[factoryvisualization.JoinResult]{Result: &result}
}

// StopDrainInput is the MCP request shape for you.factory_visualization.stop_drain.
type StopDrainInput struct{}

// StopDrain runs the Visualization root StopDrain contract for the stop_drain MCP tool.
func StopDrain(
	ctx context.Context,
	root factoryvisualization.Root,
	input StopDrainInput,
) ToolResponse[factoryvisualization.StopDrainResult] {
	if ctx == nil {
		envelope := missingContextErrorEnvelope()
		return ToolResponse[factoryvisualization.StopDrainResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryvisualization.StopDrainResult](ctx); done {
		return response
	}
	if root == nil {
		envelope := serviceUnavailableErrorEnvelope()
		return ToolResponse[factoryvisualization.StopDrainResult]{Error: &envelope}
	}
	result, err := root.StopDrain(ctx, factoryvisualization.StopDrainRequest{})
	if err != nil {
		envelope := mapRootError(err)
		return ToolResponse[factoryvisualization.StopDrainResult]{Error: &envelope}
	}
	return ToolResponse[factoryvisualization.StopDrainResult]{Result: &result}
}

// ObserveReconnectInput is the MCP reconnect cursor for you.factory_visualization.observe.
type ObserveReconnectInput struct {
	AfterEventID  string `json:"afterEventId,omitempty"`
	AfterSequence *int   `json:"afterSequence,omitempty"`
}

// ObserveInput is the MCP request shape for you.factory_visualization.observe.
type ObserveInput struct {
	Mode      string                 `json:"mode"`
	Reconnect *ObserveReconnectInput `json:"reconnect,omitempty"`
}

// Observe runs the Visualization root Observe contract for the observe MCP tool.
func Observe(
	ctx context.Context,
	root factoryvisualization.Root,
	input ObserveInput,
) ToolResponse[factoryvisualization.ObserveResult] {
	if ctx == nil {
		envelope := missingContextErrorEnvelope()
		return ToolResponse[factoryvisualization.ObserveResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryvisualization.ObserveResult](ctx); done {
		return response
	}
	if root == nil {
		envelope := serviceUnavailableErrorEnvelope()
		return ToolResponse[factoryvisualization.ObserveResult]{Error: &envelope}
	}
	if input.Mode == "" {
		envelope := requestValidationErrorEnvelope("observe Factory visualization: required request parameters are missing")
		return ToolResponse[factoryvisualization.ObserveResult]{Error: &envelope}
	}
	request := factoryvisualization.ObserveRequest{
		Mode: factoryvisualization.ObserveMode(input.Mode),
	}
	if input.Reconnect != nil {
		request.Reconnect = &factoryvisualization.ObserveReconnectCursor{
			AfterEventID:  input.Reconnect.AfterEventID,
			AfterSequence: input.Reconnect.AfterSequence,
		}
	}
	result, err := root.Observe(ctx, request)
	if err != nil {
		envelope := mapRootError(err)
		return ToolResponse[factoryvisualization.ObserveResult]{Error: &envelope}
	}
	return ToolResponse[factoryvisualization.ObserveResult]{Result: &result}
}

// OpenPresentationInput is the MCP request shape for you.factory_visualization.open_presentation.
type OpenPresentationInput struct {
	Mode string `json:"mode"`
}

// OpenPresentation runs the Visualization root OpenPresentation contract for the open_presentation MCP tool.
func OpenPresentation(
	ctx context.Context,
	root factoryvisualization.Root,
	input OpenPresentationInput,
) ToolResponse[factoryvisualization.OpenPresentationResult] {
	if ctx == nil {
		envelope := missingContextErrorEnvelope()
		return ToolResponse[factoryvisualization.OpenPresentationResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryvisualization.OpenPresentationResult](ctx); done {
		return response
	}
	if root == nil {
		envelope := serviceUnavailableErrorEnvelope()
		return ToolResponse[factoryvisualization.OpenPresentationResult]{Error: &envelope}
	}
	if input.Mode == "" {
		envelope := requestValidationErrorEnvelope("open Factory visualization presentation: required request parameters are missing")
		return ToolResponse[factoryvisualization.OpenPresentationResult]{Error: &envelope}
	}
	result, err := root.OpenPresentation(ctx, factoryvisualization.OpenPresentationRequest{
		Mode: factoryvisualization.PresentationDeliveryMode(input.Mode),
	})
	if err != nil {
		envelope := mapRootError(err)
		return ToolResponse[factoryvisualization.OpenPresentationResult]{Error: &envelope}
	}
	return ToolResponse[factoryvisualization.OpenPresentationResult]{Result: &result}
}

// ProgressRecordInput is one MCP progress record with a base64-encoded payload.
type ProgressRecordInput struct {
	Payload string `json:"payload"`
}

// PresentProgressInput is the MCP request shape for you.factory_visualization.present_progress.
type PresentProgressInput struct {
	SessionID string                `json:"sessionId"`
	Records   []ProgressRecordInput `json:"records"`
}

// PresentProgress runs the Visualization root PresentProgress contract for the present_progress MCP tool.
func PresentProgress(
	ctx context.Context,
	root factoryvisualization.Root,
	input PresentProgressInput,
) ToolResponse[factoryvisualization.PresentProgressResult] {
	if ctx == nil {
		envelope := missingContextErrorEnvelope()
		return ToolResponse[factoryvisualization.PresentProgressResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryvisualization.PresentProgressResult](ctx); done {
		return response
	}
	if root == nil {
		envelope := serviceUnavailableErrorEnvelope()
		return ToolResponse[factoryvisualization.PresentProgressResult]{Error: &envelope}
	}
	if input.SessionID == "" {
		envelope := requestValidationErrorEnvelope("present Factory visualization progress: required request parameters are missing")
		return ToolResponse[factoryvisualization.PresentProgressResult]{Error: &envelope}
	}
	records, envelope, ok := decodeProgressRecords(input.Records)
	if !ok {
		return ToolResponse[factoryvisualization.PresentProgressResult]{Error: &envelope}
	}
	result, err := root.PresentProgress(ctx, factoryvisualization.PresentProgressRequest{
		SessionID: factoryvisualization.PresentationSessionID(input.SessionID),
		Records:   records,
	})
	if err != nil {
		envelope := mapRootError(err)
		return ToolResponse[factoryvisualization.PresentProgressResult]{Error: &envelope}
	}
	return ToolResponse[factoryvisualization.PresentProgressResult]{Result: &result}
}

// TerminalWriteInput is the MCP terminal write with a base64-encoded payload.
type TerminalWriteInput struct {
	Payload string `json:"payload"`
}

// FinalizePresentationInput is the MCP request shape for you.factory_visualization.finalize_presentation.
type FinalizePresentationInput struct {
	SessionID string              `json:"sessionId"`
	Terminal  *TerminalWriteInput `json:"terminal,omitempty"`
}

// FinalizePresentation runs the Visualization root FinalizePresentation contract for the finalize_presentation MCP tool.
func FinalizePresentation(
	ctx context.Context,
	root factoryvisualization.Root,
	input FinalizePresentationInput,
) ToolResponse[factoryvisualization.FinalizePresentationResult] {
	if ctx == nil {
		envelope := missingContextErrorEnvelope()
		return ToolResponse[factoryvisualization.FinalizePresentationResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryvisualization.FinalizePresentationResult](ctx); done {
		return response
	}
	if root == nil {
		envelope := serviceUnavailableErrorEnvelope()
		return ToolResponse[factoryvisualization.FinalizePresentationResult]{Error: &envelope}
	}
	if input.SessionID == "" {
		envelope := requestValidationErrorEnvelope("finalize Factory visualization presentation: required request parameters are missing")
		return ToolResponse[factoryvisualization.FinalizePresentationResult]{Error: &envelope}
	}
	request := factoryvisualization.FinalizePresentationRequest{
		SessionID: factoryvisualization.PresentationSessionID(input.SessionID),
	}
	if input.Terminal != nil {
		payload, envelope, ok := decodeBase64Payload("decode finalize terminal payload", input.Terminal.Payload)
		if !ok {
			return ToolResponse[factoryvisualization.FinalizePresentationResult]{Error: &envelope}
		}
		request.Terminal = &factoryvisualization.TerminalWrite{Payload: payload}
	}
	result, err := root.FinalizePresentation(ctx, request)
	if err != nil {
		envelope := mapRootError(err)
		return ToolResponse[factoryvisualization.FinalizePresentationResult]{Error: &envelope}
	}
	return ToolResponse[factoryvisualization.FinalizePresentationResult]{Result: &result}
}

// ClosePresentationInput is the MCP request shape for you.factory_visualization.close_presentation.
type ClosePresentationInput struct {
	SessionID string `json:"sessionId"`
}

// ClosePresentation runs the Visualization root ClosePresentation contract for the close_presentation MCP tool.
func ClosePresentation(
	ctx context.Context,
	root factoryvisualization.Root,
	input ClosePresentationInput,
) ToolResponse[factoryvisualization.ClosePresentationResult] {
	if ctx == nil {
		envelope := missingContextErrorEnvelope()
		return ToolResponse[factoryvisualization.ClosePresentationResult]{Error: &envelope}
	}
	if response, done := requestContextErrorResponse[factoryvisualization.ClosePresentationResult](ctx); done {
		return response
	}
	if root == nil {
		envelope := serviceUnavailableErrorEnvelope()
		return ToolResponse[factoryvisualization.ClosePresentationResult]{Error: &envelope}
	}
	if input.SessionID == "" {
		envelope := requestValidationErrorEnvelope("close Factory visualization presentation: required request parameters are missing")
		return ToolResponse[factoryvisualization.ClosePresentationResult]{Error: &envelope}
	}
	result, err := root.ClosePresentation(ctx, factoryvisualization.ClosePresentationRequest{
		SessionID: factoryvisualization.PresentationSessionID(input.SessionID),
	})
	if err != nil {
		envelope := mapRootError(err)
		return ToolResponse[factoryvisualization.ClosePresentationResult]{Error: &envelope}
	}
	return ToolResponse[factoryvisualization.ClosePresentationResult]{Result: &result}
}

func decodeProgressRecords(records []ProgressRecordInput) ([]factoryvisualization.ProgressRecord, ToolErrorEnvelope, bool) {
	decoded := make([]factoryvisualization.ProgressRecord, 0, len(records))
	for _, record := range records {
		payload, envelope, ok := decodeBase64Payload("decode present progress payload", record.Payload)
		if !ok {
			return nil, envelope, false
		}
		decoded = append(decoded, factoryvisualization.ProgressRecord{Payload: payload})
	}
	return decoded, ToolErrorEnvelope{}, true
}

func decodeBase64Payload(context string, encoded string) ([]byte, ToolErrorEnvelope, bool) {
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, decodeInputErrorEnvelope(context, err), false
	}
	return payload, ToolErrorEnvelope{}, true
}
