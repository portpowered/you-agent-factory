package fusion

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestPackagedFusionRequiredInputCompletes proves that invoking the packaged
// @you/fusion Factory with only the required input completes under mock
// workers, runs the drafter then refiner dispatch sequence, and returns a
// primary result reflecting the refined fusion outcome for the submitted request.
func TestPackagedFusionRequiredInputCompletes(t *testing.T) {
	input := fmt.Sprintf(
		"functional packaged fusion required input %d",
		time.Now().UnixNano(),
	)

	factoryDir := support.InstallPackagedFactory(
		t,
		t.TempDir(),
		factorydefinitions.PackagedFusionFactoryName,
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})

	response := startPackagedFusionInvocation(
		t,
		server,
		"packaged-fusion-required-input",
		map[string]any{"input": input},
	)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one refined result part", response.PrimaryResult)
	}
	primaryText := invocationPrimaryResultText(t, response)
	if !strings.Contains(primaryText, "mock worker accepted") {
		t.Fatalf("primary result = %q, want refined mock-worker outcome", primaryText)
	}
	if strings.Contains(primaryText, input) {
		t.Fatalf("primary result = %q, want refined output rather than raw submitted input echo", primaryText)
	}

	dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
	if len(dispatches) != 2 {
		t.Fatalf(
			"dispatch count = %d, want drafter and refiner dispatches",
			len(dispatches),
		)
	}
	wantWorkstations := []string{"draft-fusion", "refine-fusion"}
	for index, dispatch := range dispatches {
		if dispatch.Response == nil {
			t.Fatalf("dispatch[%d] = %#v, want completed public dispatch response", index, dispatch)
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("dispatch[%d] outcome = %q, want ACCEPTED", index, dispatch.Response.Outcome)
		}
		if dispatch.Request.TransitionId != wantWorkstations[index] {
			t.Fatalf(
				"dispatch[%d] transition = %q, want %q in documented order",
				index,
				dispatch.Request.TransitionId,
				wantWorkstations[index],
			)
		}
	}
}

// TestPackagedFusionOptionalInputsReachWorkers proves that optional fusion
// provider, model, and effort overrides reach the drafter and refiner workers
// and are observable on public dispatch execution selection and agent-run
// diagnostics when invoking @you/fusion through the packaged invocation API.
func TestPackagedFusionOptionalInputsReachWorkers(t *testing.T) {
	input := fmt.Sprintf(
		"functional packaged fusion optional overrides %d",
		time.Now().UnixNano(),
	)

	factoryDir := support.InstallPackagedFactory(
		t,
		t.TempDir(),
		factorydefinitions.PackagedFusionFactoryName,
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
	})

	args := map[string]any{
		"input":           input,
		"firstProvider":   "CLAUDE",
		"secondProvider":  "CODEX",
		"firstModel":      "claude-sonnet-4-20250514",
		"secondModel":     "gpt-5",
		"firstEffort":     "low",
		"secondEffort":    "high",
		"output":          "fusion-optional-overrides.md",
	}
	response := startPackagedFusionInvocation(
		t,
		server,
		"packaged-fusion-optional-inputs",
		args,
	)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one refined result part", response.PrimaryResult)
	}

	dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
	if len(dispatches) != 2 {
		t.Fatalf("dispatch count = %d, want drafter and refiner dispatches", len(dispatches))
	}
	wantWorkstations := []string{"draft-fusion", "refine-fusion"}
	for index, dispatch := range dispatches {
		if dispatch.Request.TransitionId != wantWorkstations[index] {
			t.Fatalf(
				"dispatch[%d] transition = %q, want %q in documented order",
				index,
				dispatch.Request.TransitionId,
				wantWorkstations[index],
			)
		}
	}

	events := server.GetFactoryEvents(t)
	modelRequests := modelRequestsByWorker(t, events)
	assertFusionModelRequest(
		t,
		modelRequests,
		"fusion-drafter",
		"claude-sonnet-4-20250514",
	)
	assertFusionModelRequest(t, modelRequests, "fusion-refiner", "gpt-5")

	providersByDispatch := inferenceProvidersByDispatchID(t, events)
	assertFusionInferenceProvider(
		t,
		providersByDispatch,
		dispatches[0].DispatchID,
		"draft-fusion",
		"claude",
	)
	assertFusionInferenceProvider(
		t,
		providersByDispatch,
		dispatches[1].DispatchID,
		"refine-fusion",
		"codex",
	)

	effortByDispatch := agentRunSystemSummariesByDispatchID(t, events)
	if !strings.Contains(effortByDispatch[dispatches[0].DispatchID], "Reasoning effort: low") {
		t.Fatalf(
			"drafter agent-run system summary = %q, want firstEffort override",
			effortByDispatch[dispatches[0].DispatchID],
		)
	}
	if !strings.Contains(effortByDispatch[dispatches[1].DispatchID], "Reasoning effort: high") {
		t.Fatalf(
			"refiner agent-run system summary = %q, want secondEffort override",
			effortByDispatch[dispatches[1].DispatchID],
		)
	}
}

// TestPackagedFusionPartialWorkerFailureUsesDocumentedOutcome proves that a
// configured mock-worker rejection during one fusion stage returns a failed
// public terminal invocation outcome without a completed success primary
// result attributable to the failing run.
func TestPackagedFusionPartialWorkerFailureUsesDocumentedOutcome(t *testing.T) {
	input := fmt.Sprintf(
		"functional packaged fusion partial worker failure %d",
		time.Now().UnixNano(),
	)

	factoryDir := support.InstallPackagedFactory(
		t,
		t.TempDir(),
		factorydefinitions.PackagedFusionFactoryName,
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		UseMockWorkers:            true,
		MockWorkersConfig:         packagedFusionRejectingDrafterMockWorkersConfig(),
		WaitForServiceModeRuntime: true,
	})

	response := startPackagedFusionInvocation(
		t,
		server,
		"packaged-fusion-partial-worker-failure",
		map[string]any{"input": input},
	)
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED; response = %#v", response.Status, response)
	}
	if response.PrimaryResult != nil {
		t.Fatalf(
			"primary result = %#v, want no completed success primary result after worker failure",
			response.PrimaryResult,
		)
	}
	if response.WorkState == nil || *response.WorkState != "task:failed" {
		t.Fatalf("invocation workState = %#v, want task:failed", response.WorkState)
	}

	dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
	if len(dispatches) == 0 {
		t.Fatal("dispatch observations missing, want at least one draft-fusion failure")
	}
	var failedDraftDispatches int
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId == "refine-fusion" {
			t.Fatalf(
				"dispatch transition = %q, want no refiner dispatch after draft-stage failure",
				dispatch.Request.TransitionId,
			)
		}
		if dispatch.Request.TransitionId != "draft-fusion" {
			t.Fatalf(
				"dispatch transition = %q, want only draft-fusion dispatches",
				dispatch.Request.TransitionId,
			)
		}
		if dispatch.Response == nil {
			t.Fatalf("draft-fusion dispatch response missing: %#v", dispatch)
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeFailed {
			t.Fatalf(
				"draft-fusion outcome = %q, want FAILED",
				dispatch.Response.Outcome,
			)
		}
		if dispatch.Response.Error == nil || strings.TrimSpace(*dispatch.Response.Error) == "" {
			t.Fatalf("dispatch error = %#v, want stable public failure record", dispatch.Response.Error)
		}
		failedDraftDispatches++
	}
	if failedDraftDispatches == 0 {
		t.Fatal("failed draft-fusion dispatch count = 0, want at least one")
	}
}

func packagedFusionRejectingDrafterMockWorkersConfig() *workers.MockWorkersConfig {
	exitCode := 7
	return &workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      "fusion-drafter",
			WorkstationName: "draft-fusion",
			RunType:         workers.MockWorkerRunTypeReject,
			RejectConfig: &workers.MockWorkerRejectConfig{
				Stderr:   "packaged fusion mock worker failure",
				ExitCode: &exitCode,
			},
		}},
	}
}

func modelRequestsByWorker(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) map[string]factoryapi.ModelRequestEventPayload {
	t.Helper()

	requests := make(map[string]factoryapi.ModelRequestEventPayload)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeModelRequest {
			continue
		}
		payload, err := event.Payload.AsModelRequestEventPayload()
		if err != nil {
			t.Fatalf("decode MODEL_REQUEST: %v", err)
		}
		requests[payload.Worker] = payload
	}
	return requests
}

func inferenceProvidersByDispatchID(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) map[string]string {
	t.Helper()

	providers := make(map[string]string)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		if event.Context.DispatchId == nil || *event.Context.DispatchId == "" {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode INFERENCE_RESPONSE: %v", err)
		}
		if payload.Diagnostics == nil || payload.Diagnostics.Provider == nil ||
			payload.Diagnostics.Provider.Provider == nil {
			t.Fatalf(
				"INFERENCE_RESPONSE for dispatch %q missing provider diagnostics: %#v",
				*event.Context.DispatchId,
				payload,
			)
		}
		providers[*event.Context.DispatchId] = *payload.Diagnostics.Provider.Provider
	}
	return providers
}

func agentRunSystemSummariesByDispatchID(
	t *testing.T,
	events []factoryapi.FactoryEvent,
) map[string]string {
	t.Helper()

	summaries := make(map[string]string)
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeAgentRunResponse {
			continue
		}
		if event.Context.DispatchId == nil || *event.Context.DispatchId == "" {
			continue
		}
		payload, err := event.Payload.AsAgentRunResponseEventPayload()
		if err != nil {
			t.Fatalf("decode AGENT_RUN_RESPONSE: %v", err)
		}
		if payload.Diagnostics == nil || payload.Diagnostics.AgentRun == nil ||
			payload.Diagnostics.AgentRun.Transcript == nil {
			t.Fatalf(
				"AGENT_RUN_RESPONSE for dispatch %q missing transcript diagnostics: %#v",
				*event.Context.DispatchId,
				payload,
			)
		}
		for _, entry := range *payload.Diagnostics.AgentRun.Transcript {
			if entry.Role != nil && *entry.Role == "system" && entry.Summary != nil {
				summaries[*event.Context.DispatchId] = *entry.Summary
				break
			}
		}
	}
	return summaries
}

func assertFusionModelRequest(
	t *testing.T,
	requests map[string]factoryapi.ModelRequestEventPayload,
	workerName, wantModel string,
) {
	t.Helper()

	payload, ok := requests[workerName]
	if !ok {
		t.Fatalf("model requests = %#v, want %q MODEL_REQUEST", requests, workerName)
	}
	if payload.Model != wantModel {
		t.Fatalf(
			"%s model request model = %q, want override %q",
			workerName,
			payload.Model,
			wantModel,
		)
	}
}

func assertFusionInferenceProvider(
	t *testing.T,
	providersByDispatch map[string]string,
	dispatchID, workstationName, wantProvider string,
) {
	t.Helper()

	gotProvider, ok := providersByDispatch[dispatchID]
	if !ok {
		t.Fatalf(
			"inference providers = %#v, want provider diagnostics for %q dispatch %q",
			providersByDispatch,
			workstationName,
			dispatchID,
		)
	}
	if gotProvider != wantProvider {
		t.Fatalf(
			"%s inference provider = %q, want override %q",
			workstationName,
			gotProvider,
			wantProvider,
		)
	}
}

func startPackagedFusionInvocation(
	t *testing.T,
	server *support.FunctionalAPIServer,
	requestID string,
	args map[string]any,
) factoryapi.InvocationResponse {
	t.Helper()

	return postJSON[factoryapi.InvocationResponse](
		t,
		server.URL()+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		factoryapi.InvocationRequest{
			RequestId: &requestID,
			Args:      &args,
		},
		"start packaged fusion invocation",
	)
}

func invocationPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}

func postJSON[T any](t *testing.T, endpoint string, request any, failurePrefix string) T {
	t.Helper()
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("%s: marshal request: %v", failurePrefix, err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("%s: POST %s: %v", failurePrefix, endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("%s: POST %s status = %d, want success: %s", failurePrefix, endpoint, response.StatusCode, string(payload))
	}
	var decoded T
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("%s: decode %s response: %v", failurePrefix, endpoint, err)
	}
	return decoded
}
