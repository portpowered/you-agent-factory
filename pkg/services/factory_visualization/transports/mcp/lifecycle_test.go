package factoryvisualization_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	factoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization"
	mcpfactoryvisualization "github.com/portpowered/infinite-you/pkg/services/factory_visualization/transports/mcp"
)

func TestLifecycle_ActivateSuccessReturnsRootLifecycleState(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		activate: func(_ context.Context, request factoryvisualization.ActivateRequest) (factoryvisualization.ActivateResult, error) {
			if request.Mode != factoryvisualization.ActivateModeRetainedThenLive {
				t.Fatalf("mode = %q, want %q", request.Mode, factoryvisualization.ActivateModeRetainedThenLive)
			}
			return factoryvisualization.ActivateResult{State: factoryvisualization.LifecycleStateStarted}, nil
		},
	}
	response := callLifecycleTool(t, fake, mcpfactoryvisualization.ToolActivate, `{"mode":"RETAINED_THEN_LIVE"}`)
	assertLifecycleSuccessState(t, response, `"State":"STARTED"`)
}

func TestLifecycle_JoinSuccessReturnsRootLifecycleState(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		join: func(_ context.Context, _ factoryvisualization.JoinRequest) (factoryvisualization.JoinResult, error) {
			return factoryvisualization.JoinResult{State: factoryvisualization.LifecycleStateStarted}, nil
		},
	}
	response := callLifecycleTool(t, fake, mcpfactoryvisualization.ToolJoin, `{}`)
	assertLifecycleSuccessState(t, response, `"State":"STARTED"`)
}

func TestLifecycle_StopDrainSuccessReturnsRootLifecycleState(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		stopDrain: func(_ context.Context, _ factoryvisualization.StopDrainRequest) (factoryvisualization.StopDrainResult, error) {
			return factoryvisualization.StopDrainResult{State: factoryvisualization.LifecycleStateStopped}, nil
		},
	}
	response := callLifecycleTool(t, fake, mcpfactoryvisualization.ToolStopDrain, `{}`)
	assertLifecycleSuccessState(t, response, `"State":"STOPPED"`)
}

func TestLifecycle_ActivateMissingParametersReturnsTypedEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		activate: func(_ context.Context, request factoryvisualization.ActivateRequest) (factoryvisualization.ActivateResult, error) {
			if request.Mode != factoryvisualization.ActivateModeRetainedThenLive {
				return factoryvisualization.ActivateResult{}, &factoryvisualization.LifecycleError{
					Kind:    factoryvisualization.LifecycleErrorMissingParameters,
					Message: "activate Factory visualization: required request parameters are missing",
				}
			}
			return factoryvisualization.ActivateResult{State: factoryvisualization.LifecycleStateStarted}, nil
		},
	}
	response := callLifecycleTool(t, fake, mcpfactoryvisualization.ToolActivate, `{"mode":"UNSUPPORTED"}`)
	assertLifecycleErrorEnvelope(t, response, "factory_visualization.lifecycle.missing_parameters", false)
}

func TestLifecycle_ActivateAlreadyActivatedReturnsTypedEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		activate: func(context.Context, factoryvisualization.ActivateRequest) (factoryvisualization.ActivateResult, error) {
			return factoryvisualization.ActivateResult{}, &factoryvisualization.LifecycleError{
				Kind:    factoryvisualization.LifecycleErrorAlreadyActivated,
				Message: "Factory visualization is already activated",
			}
		},
	}
	response := callLifecycleTool(t, fake, mcpfactoryvisualization.ToolActivate, `{"mode":"RETAINED_THEN_LIVE"}`)
	assertLifecycleErrorEnvelope(t, response, "factory_visualization.lifecycle.already_activated", false)
}

func TestLifecycle_JoinNotActivatedReturnsTypedEnvelope(t *testing.T) {
	t.Parallel()

	fake := fakeVisualizationRoot{
		join: func(context.Context, factoryvisualization.JoinRequest) (factoryvisualization.JoinResult, error) {
			return factoryvisualization.JoinResult{}, &factoryvisualization.LifecycleError{
				Kind:    factoryvisualization.LifecycleErrorNotActivated,
				Message: "Factory visualization is not activated",
			}
		},
	}
	response := callLifecycleTool(t, fake, mcpfactoryvisualization.ToolJoin, `{}`)
	assertLifecycleErrorEnvelope(t, response, "factory_visualization.lifecycle.not_activated", false)
}

func TestLifecycle_MalformedJSONReturnsDecodeErrorWithoutInvokingRoot(t *testing.T) {
	t.Parallel()

	var invoked bool
	fake := fakeVisualizationRoot{invoked: &invoked}
	operation := mcpfactoryvisualization.Bind(mcpfactoryvisualization.RootDependencies{Root: fake})

	raw, err := operation(context.Background(), mcpfactoryvisualization.ToolActivate, json.RawMessage(`{"mode":`))
	if err != nil {
		t.Fatalf("CallTool(activate) transport error = %v, want JSON response envelope", err)
	}
	if invoked {
		t.Fatal("fake visualization root was invoked for malformed activate input")
	}
	assertLifecycleErrorEnvelope(t, string(raw), "BAD_REQUEST", false)
	if !strings.Contains(string(raw), "decode activate input") {
		t.Fatalf("malformed activate response = %s, want decode error message", raw)
	}
}

func callLifecycleTool(t *testing.T, fake fakeVisualizationRoot, toolName string, input string) string {
	t.Helper()

	operation := mcpfactoryvisualization.Bind(mcpfactoryvisualization.RootDependencies{Root: fake})
	raw, err := operation(context.Background(), toolName, json.RawMessage(input))
	if err != nil {
		t.Fatalf("CallTool(%s) error = %v", toolName, err)
	}
	return string(raw)
}

func assertLifecycleSuccessState(t *testing.T, response string, wantStateFragment string) {
	t.Helper()

	if strings.Contains(response, `"error"`) {
		t.Fatalf("lifecycle success response = %s, want result without error envelope", response)
	}
	if !strings.Contains(response, wantStateFragment) {
		t.Fatalf("lifecycle success response = %s, want state fragment %q", response, wantStateFragment)
	}
}

func assertLifecycleErrorEnvelope(t *testing.T, response string, wantCode string, wantRetryable bool) {
	t.Helper()

	var envelope struct {
		Error *mcpfactoryvisualization.ToolErrorEnvelope `json:"error"`
	}
	if err := json.Unmarshal([]byte(response), &envelope); err != nil {
		t.Fatalf("unmarshal lifecycle response: %v\nresponse=%s", err, response)
	}
	if envelope.Error == nil {
		t.Fatalf("lifecycle response = %s, want error envelope", response)
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
