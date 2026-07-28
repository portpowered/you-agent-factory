package factoryvisualization_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	mcpfactoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/mcp"
)

func TestPresentation_OpenPresentationSuccessReturnsSessionIdentity(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		openPresentation: func(_ context.Context, request factoryvisualization.OpenPresentationRequest) (factoryvisualization.OpenPresentationResult, error) {
			if request.Mode != factoryvisualization.PresentationDeliveryBestEffort {
				t.Fatalf("mode = %q, want %q", request.Mode, factoryvisualization.PresentationDeliveryBestEffort)
			}
			return factoryvisualization.OpenPresentationResult{
				SessionID: "presentation-1",
				Mode:      factoryvisualization.PresentationDeliveryBestEffort,
			}, nil
		},
	}
	response := callPresentationTool(t, fake, mcpfactoryvisualization.ToolOpenPresentation, `{"mode":"BEST_EFFORT"}`)
	assertPresentationSuccess(t, response, `"SessionID":"presentation-1"`, `"Mode":"BEST_EFFORT"`)
}

func TestPresentation_PresentProgressSuccessReturnsAcceptedCount(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		presentProgress: func(_ context.Context, request factoryvisualization.PresentProgressRequest) (factoryvisualization.PresentProgressResult, error) {
			if request.SessionID != "presentation-1" {
				t.Fatalf("session id = %q, want presentation-1", request.SessionID)
			}
			if len(request.Records) != 2 || string(request.Records[0].Payload) != "one" || string(request.Records[1].Payload) != "two" {
				t.Fatalf("records = %#v, want decoded one and two payloads", request.Records)
			}
			return factoryvisualization.PresentProgressResult{AcceptedCount: 2}, nil
		},
	}
	input := `{"sessionId":"presentation-1","records":[{"payload":"` + base64.StdEncoding.EncodeToString([]byte("one")) + `"},{"payload":"` + base64.StdEncoding.EncodeToString([]byte("two")) + `"}]}`
	response := callPresentationTool(t, fake, mcpfactoryvisualization.ToolPresentProgress, input)
	assertPresentationSuccess(t, response, `"AcceptedCount":2`)
}

func TestPresentation_FinalizePresentationSuccessReturnsOutcome(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		finalizePresentation: func(_ context.Context, request factoryvisualization.FinalizePresentationRequest) (factoryvisualization.FinalizePresentationResult, error) {
			if request.SessionID != "presentation-1" {
				t.Fatalf("session id = %q, want presentation-1", request.SessionID)
			}
			if request.Terminal == nil || string(request.Terminal.Payload) != "done" {
				t.Fatalf("terminal = %#v, want done payload", request.Terminal)
			}
			return factoryvisualization.FinalizePresentationResult{
				Finalized:    true,
				ProgressSeen: true,
			}, nil
		},
	}
	input := `{"sessionId":"presentation-1","terminal":{"payload":"` + base64.StdEncoding.EncodeToString([]byte("done")) + `"}}`
	response := callPresentationTool(t, fake, mcpfactoryvisualization.ToolFinalizePresentation, input)
	assertPresentationSuccess(t, response, `"Finalized":true`, `"ProgressSeen":true`)
}

func TestPresentation_ClosePresentationSuccessReturnsDroppedCount(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		closePresentation: func(_ context.Context, request factoryvisualization.ClosePresentationRequest) (factoryvisualization.ClosePresentationResult, error) {
			if request.SessionID != "presentation-1" {
				t.Fatalf("session id = %q, want presentation-1", request.SessionID)
			}
			return factoryvisualization.ClosePresentationResult{DroppedCount: 3}, nil
		},
	}
	response := callPresentationTool(t, fake, mcpfactoryvisualization.ToolClosePresentation, `{"sessionId":"presentation-1"}`)
	assertPresentationSuccess(t, response, `"DroppedCount":3`)
}

func TestPresentation_EnqueueAfterCloseReturnsTypedEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		presentProgress: func(context.Context, factoryvisualization.PresentProgressRequest) (factoryvisualization.PresentProgressResult, error) {
			return factoryvisualization.PresentProgressResult{}, &factoryvisualization.PresentationError{
				Kind:    factoryvisualization.PresentationErrorEnqueueAfterClose,
				Message: "present Factory visualization progress: presentation output is closed",
			}
		},
	}
	input := `{"sessionId":"presentation-1","records":[{"payload":"` + base64.StdEncoding.EncodeToString([]byte("late")) + `"}]}`
	response := callPresentationTool(t, fake, mcpfactoryvisualization.ToolPresentProgress, input)
	assertPresentationErrorEnvelope(t, response, "factory_visualization.presentation.enqueue_after_close", false)
}

func TestPresentation_FinalizeWithoutWriterReturnsTypedEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		finalizePresentation: func(context.Context, factoryvisualization.FinalizePresentationRequest) (factoryvisualization.FinalizePresentationResult, error) {
			return factoryvisualization.FinalizePresentationResult{}, &factoryvisualization.PresentationError{
				Kind:    factoryvisualization.PresentationErrorFinalizeWithoutWriter,
				Message: "finalize Factory visualization presentation: terminal writer is required",
			}
		},
	}
	response := callPresentationTool(t, fake, mcpfactoryvisualization.ToolFinalizePresentation, `{"sessionId":"presentation-1"}`)
	assertPresentationErrorEnvelope(t, response, "factory_visualization.presentation.finalize_without_writer", false)
}

func TestPresentation_BackpressureRejectedReturnsTypedEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		presentProgress: func(context.Context, factoryvisualization.PresentProgressRequest) (factoryvisualization.PresentProgressResult, error) {
			return factoryvisualization.PresentProgressResult{AcceptedCount: 1}, &factoryvisualization.PresentationError{
				Kind:    factoryvisualization.PresentationErrorBackpressureRejected,
				Message: "present Factory visualization progress: best-effort backlog rejected record",
			}
		},
	}
	input := `{"sessionId":"presentation-1","records":[{"payload":"` + base64.StdEncoding.EncodeToString([]byte("overflow")) + `"}]}`
	response := callPresentationTool(t, fake, mcpfactoryvisualization.ToolPresentProgress, input)
	assertPresentationErrorEnvelope(t, response, "factory_visualization.presentation.backpressure_rejected", false)
}

func TestPresentation_ErrorsAreDistinguishableFromLifecycleAndProjectionFailures(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		presentProgress: func(context.Context, factoryvisualization.PresentProgressRequest) (factoryvisualization.PresentProgressResult, error) {
			return factoryvisualization.PresentProgressResult{}, &factoryvisualization.PresentationError{
				Kind:    factoryvisualization.PresentationErrorEnqueueAfterClose,
				Message: "present Factory visualization progress: presentation output is closed",
			}
		},
	}
	input := `{"sessionId":"presentation-1","records":[{"payload":"` + base64.StdEncoding.EncodeToString([]byte("late")) + `"}]}`
	response := callPresentationTool(t, fake, mcpfactoryvisualization.ToolPresentProgress, input)
	if strings.Contains(response, "factory_visualization.lifecycle.") || strings.Contains(response, "factory_visualization.projection.") {
		t.Fatalf("presentation failure response = %s, want presentation error code prefix", response)
	}
	assertPresentationErrorEnvelope(t, response, "factory_visualization.presentation.enqueue_after_close", false)
}

func TestPresentation_MalformedJSONReturnsDecodeErrorWithoutInvokingRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeVisualizationRoot{invoked: &invoked}
	operation := mcpfactoryvisualization.Bind(mcpfactoryvisualization.RootDependencies{Root: fake})

	raw, err := operation(context.Background(), mcpfactoryvisualization.ToolOpenPresentation, json.RawMessage(`{"mode":`))
	if err != nil {
		t.Fatalf("CallTool(open_presentation) transport error = %v, want JSON response envelope", err)
	}
	if invoked {
		t.Fatal("fake visualization root was invoked for malformed open_presentation input")
	}
	assertPresentationErrorEnvelope(t, string(raw), "BAD_REQUEST", false)
	if !strings.Contains(string(raw), "decode open_presentation input") {
		t.Fatalf("malformed open_presentation response = %s, want decode error message", raw)
	}
}

func TestPresentation_InvalidBase64PayloadReturnsDecodeErrorWithoutInvokingRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeVisualizationRoot{
		invoked: &invoked,
		presentProgress: func(context.Context, factoryvisualization.PresentProgressRequest) (factoryvisualization.PresentProgressResult, error) {
			t.Fatal("fake visualization root should not be invoked for invalid base64 payload")
			return factoryvisualization.PresentProgressResult{}, nil
		},
	}
	response := callPresentationTool(t, fake, mcpfactoryvisualization.ToolPresentProgress, `{"sessionId":"presentation-1","records":[{"payload":"not-base64!!!"}]}`)
	if invoked {
		t.Fatal("fake visualization root was invoked for invalid base64 payload")
	}
	assertPresentationErrorEnvelope(t, response, "BAD_REQUEST", false)
	if !strings.Contains(response, "decode present progress payload") {
		t.Fatalf("invalid base64 response = %s, want decode error message", response)
	}
}

func callPresentationTool(t *testing.T, fake fakeVisualizationRoot, toolName string, input string) string {
	t.Helper()

	operation := mcpfactoryvisualization.Bind(mcpfactoryvisualization.RootDependencies{Root: fake})
	raw, err := operation(context.Background(), toolName, json.RawMessage(input))
	if err != nil {
		t.Fatalf("CallTool(%s) error = %v", toolName, err)
	}
	return string(raw)
}

func assertPresentationSuccess(t *testing.T, response string, wantFragments ...string) {
	t.Helper()

	if strings.Contains(response, `"error"`) {
		t.Fatalf("presentation success response = %s, want result without error envelope", response)
	}
	for _, fragment := range wantFragments {
		if !strings.Contains(response, fragment) {
			t.Fatalf("presentation success response = %s, want fragment %q", response, fragment)
		}
	}
}

func assertPresentationErrorEnvelope(t *testing.T, response string, wantCode string, wantRetryable bool) {
	t.Helper()

	var envelope struct {
		Error *mcpfactoryvisualization.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal([]byte(response), &envelope); err != nil {
		t.Fatalf("unmarshal presentation response: %v\nresponse=%s", err, response)
	}
	if envelope.Error == nil {
		t.Fatalf("presentation response = %s, want error envelope", response)
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
