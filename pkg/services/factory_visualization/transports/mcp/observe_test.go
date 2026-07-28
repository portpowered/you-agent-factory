package factoryvisualization_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	mcpfactoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/mcp"
)

func TestObserve_SuccessReturnsRootProjectedView(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, 7, 28, 2, 0, 0, 0, time.UTC)
	afterSequence := 7
	fake := fakeVisualizationRoot{
		observe: func(_ context.Context, request factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error) {
			if request.Mode != factoryvisualization.ObserveModeRetainedThenLive {
				t.Fatalf("mode = %q, want %q", request.Mode, factoryvisualization.ObserveModeRetainedThenLive)
			}
			if request.Reconnect == nil || request.Reconnect.AfterEventID != "evt-1" || request.Reconnect.AfterSequence == nil || *request.Reconnect.AfterSequence != afterSequence {
				t.Fatalf("reconnect = %#v, want afterEventId evt-1 and afterSequence 7", request.Reconnect)
			}
			return factoryvisualization.ObserveResult{
				View: factoryvisualization.ProjectedView{
					TickCount:          9,
					RetainedEventCount: 1,
					ObservedAt:         observedAt,
				},
			}, nil
		},
	}
	response := callObserveTool(t, fake, `{"mode":"RETAINED_THEN_LIVE","reconnect":{"afterEventId":"evt-1","afterSequence":7}}`)
	assertObserveSuccessView(t, response, `"TickCount":9`, `"RetainedEventCount":1`, `"ObservedAt":"2026-07-28T02:00:00Z"`)
}

func TestObserve_InvalidInputReturnsTypedProjectionEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		observe: func(context.Context, factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error) {
			return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
				Kind:    factoryvisualization.ProjectionErrorInvalidInput,
				Message: "observe Factory visualization: required request parameters are missing",
			}
		},
	}
	response := callObserveTool(t, fake, `{"mode":"RETAINED_THEN_LIVE"}`)
	assertObserveErrorEnvelope(t, response, "factory_visualization.projection.invalid_input", false)
}

func TestObserve_SnapshotUnavailableReturnsTypedProjectionEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		observe: func(context.Context, factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error) {
			return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
				Kind:    factoryvisualization.ProjectionErrorSnapshotUnavailable,
				Message: "observe Factory visualization: retained snapshot is unavailable",
			}
		},
	}
	response := callObserveTool(t, fake, `{"mode":"RETAINED_THEN_LIVE"}`)
	assertObserveErrorEnvelope(t, response, "factory_visualization.projection.snapshot_unavailable", false)
}

func TestObserve_ReconstructionFailedReturnsTypedProjectionEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		observe: func(context.Context, factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error) {
			return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
				Kind:    factoryvisualization.ProjectionErrorReconstructionFailed,
				Message: "observe Factory visualization: retained history reconstruction failed",
			}
		},
	}
	response := callObserveTool(t, fake, `{"mode":"RETAINED_THEN_LIVE"}`)
	assertObserveErrorEnvelope(t, response, "factory_visualization.projection.reconstruction_failed", false)
}

func TestObserve_ProjectionErrorsAreDistinguishableFromLifecycleErrors(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		observe: func(context.Context, factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error) {
			return factoryvisualization.ObserveResult{}, &factoryvisualization.ProjectionError{
				Kind:    factoryvisualization.ProjectionErrorSnapshotUnavailable,
				Message: "observe Factory visualization: retained snapshot is unavailable",
			}
		},
	}
	response := callObserveTool(t, fake, `{"mode":"RETAINED_THEN_LIVE"}`)
	if strings.Contains(response, "factory_visualization.lifecycle.") {
		t.Fatalf("projection failure response = %s, want projection error code prefix", response)
	}
	assertObserveErrorEnvelope(t, response, "factory_visualization.projection.snapshot_unavailable", false)
}

func TestObserve_MalformedJSONReturnsDecodeErrorWithoutInvokingRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeVisualizationRoot{invoked: &invoked}
	operation := mcpfactoryvisualization.Bind(mcpfactoryvisualization.RootDependencies{Root: fake})

	raw, err := operation(context.Background(), mcpfactoryvisualization.ToolObserve, json.RawMessage(`{"mode":`))
	if err != nil {
		t.Fatalf("CallTool(observe) transport error = %v, want JSON response envelope", err)
	}
	if invoked {
		t.Fatal("fake visualization root was invoked for malformed observe input")
	}
	assertObserveErrorEnvelope(t, string(raw), "BAD_REQUEST", false)
	if !strings.Contains(string(raw), "decode observe input") {
		t.Fatalf("malformed observe response = %s, want decode error message", raw)
	}
}

func TestObserve_SuccessEncodesAsSerializedJSONCallToolResult(t *testing.T) {
	t.Parallel()

	observedAt := time.Date(2026, 7, 28, 2, 0, 0, 0, time.UTC)
	fake := fakeVisualizationRoot{
		observe: func(context.Context, factoryvisualization.ObserveRequest) (factoryvisualization.ObserveResult, error) {
			return factoryvisualization.ObserveResult{
				View: factoryvisualization.ProjectedView{
					TickCount:          3,
					RetainedEventCount: 2,
					ObservedAt:         observedAt,
				},
			}, nil
		},
	}
	toolResponse := json.RawMessage(callObserveTool(t, fake, `{"mode":"RETAINED_THEN_LIVE"}`))
	callToolResult, err := mcpfactoryvisualization.MarshalSuccessCallToolResultJSON(toolResponse)
	if err != nil {
		t.Fatalf("MarshalSuccessCallToolResultJSON() error = %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(callToolResult, &decoded); err != nil {
		t.Fatalf("unmarshal callToolResult: %v", err)
	}
	content, ok := decoded["content"].([]any)
	if !ok || len(content) != 1 {
		t.Fatalf("callToolResult content = %#v, want one text item", decoded["content"])
	}
	item, ok := content[0].(map[string]any)
	if !ok {
		t.Fatalf("content[0] = %#v, want object", content[0])
	}
	if item["type"] != "text" {
		t.Fatalf("content type = %v, want text", item["type"])
	}
	text, ok := item["text"].(string)
	if !ok {
		t.Fatalf("content text = %#v, want string", item["text"])
	}
	if text != string(toolResponse) {
		t.Fatalf("content text = %s, want serialized tool response %s", text, toolResponse)
	}
	if _, hasError := decoded["isError"]; hasError {
		t.Fatalf("callToolResult = %s, want success envelope without isError", callToolResult)
	}
}

func callObserveTool(t *testing.T, fake fakeVisualizationRoot, input string) string {
	t.Helper()

	operation := mcpfactoryvisualization.Bind(mcpfactoryvisualization.RootDependencies{Root: fake})
	raw, err := operation(context.Background(), mcpfactoryvisualization.ToolObserve, json.RawMessage(input))
	if err != nil {
		t.Fatalf("CallTool(observe) error = %v", err)
	}
	return string(raw)
}

func assertObserveSuccessView(t *testing.T, response string, wantFragments ...string) {
	t.Helper()

	if strings.Contains(response, `"error"`) {
		t.Fatalf("observe success response = %s, want result without error envelope", response)
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(response, fragment) {
			t.Fatalf("observe success response = %s, want fragment %q", response, fragment)
		}
	}
}

func assertObserveErrorEnvelope(t *testing.T, response string, wantCode string, wantRetryable bool) {
	t.Helper()

	var envelope struct {
		Error *mcpfactoryvisualization.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal([]byte(response), &envelope); err != nil {
		t.Fatalf("unmarshal observe response: %v\nresponse=%s", err, response)
	}
	if envelope.Error == nil {
		t.Fatalf("observe response = %s, want error envelope", response)
	}
	if envelope.Error.Code != wantCode {
		t.Fatalf("error code = %q, want %q", envelope.Error.Code, wantCode)
	}
	if envelope.Error.Retryable != wantRetryable {
		t.Fatalf("error retryable = %v, want %v", envelope.Error.Retryable, wantRetryable)
	}
	if strings.TrimSpace(envelope.Error.Message) == "" {
		t.Fatal("error message is required")
	}
}
