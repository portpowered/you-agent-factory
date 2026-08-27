package fusion

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPackagedFusion(t *testing.T) {
	fixture := newFusionSharedFixture(t)
	t.Run("TestPackagedFusionRequiredInputCompletes", func(t *testing.T) {
		testPackagedFusionRequiredInputCompletes(t, fixture)
	})
	t.Run("TestPackagedFusionOptionalInputsReachWorkers", func(t *testing.T) {
		testPackagedFusionOptionalInputsReachWorkers(t, fixture)
	})
	t.Run("TestPackagedFusionPartialWorkerFailureUsesDocumentedOutcome", func(t *testing.T) {
		testPackagedFusionPartialWorkerFailureUsesDocumentedOutcome(t, fixture)
	})
}

// testPackagedFusionRequiredInputCompletes proves that invoking the packaged
// @you/fusion Factory with only the required input completes through the
// controlled provider command boundary, runs the drafter then refiner dispatch
// sequence, and returns a primary result reflecting the refined fusion outcome.
func testPackagedFusionRequiredInputCompletes(
	t *testing.T,
	fixture *fusionSharedFixture,
) {
	input := fmt.Sprintf(
		"functional packaged fusion required input %d",
		time.Now().UnixNano(),
	)

	scenario := fixture.newScenario(t, support.NewRecordingCommandRunner("mock worker accepted"))
	scenario.open(t)

	response := startPackagedFusionInvocation(
		t,
		scenario,
		"fusion-required-input",
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

	dispatches := support.ObserveDispatchEvents(t, support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID))
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
func testPackagedFusionOptionalInputsReachWorkers(
	t *testing.T,
	fixture *fusionSharedFixture,
) {
	input := fmt.Sprintf(
		"functional packaged fusion optional overrides %d",
		time.Now().UnixNano(),
	)

	runner := support.NewRecordingCommandRunner("mock worker accepted")
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)

	args := map[string]any{
		"input":          input,
		"firstProvider":  "CLAUDE",
		"secondProvider": "CODEX",
		"firstModel":     "claude-sonnet-4-20250514",
		"secondModel":    "gpt-5",
		"firstEffort":    "low",
		"secondEffort":   "high",
		"resultFile":     "fusion-optional-overrides.md",
	}
	response := startPackagedFusionInvocation(
		t,
		scenario,
		"fusion-optional-inputs",
		args,
	)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one refined result part", response.PrimaryResult)
	}

	events := support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
	dispatches := support.ObserveDispatchEvents(t, events)
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

	modelRequests := modelRequestsByWorker(t, events)
	assertFusionModelRequest(
		t,
		modelRequests,
		"fusion-drafter",
		"claude-sonnet-4-20250514",
	)
	assertFusionModelRequest(t, modelRequests, "fusion-refiner", "gpt-5")

	providerRequests := runner.Requests()
	if len(providerRequests) != 2 {
		t.Fatalf("provider requests = %#v, want drafter and refiner commands", providerRequests)
	}
	assertFusionProviderCommand(t, providerRequests[0], "claude", "claude-sonnet-4-20250514")
	assertFusionProviderCommand(t, providerRequests[1], "codex", "gpt-5")

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
// provider-command failure during one fusion stage returns a failed
// public terminal invocation outcome without a completed success primary
// result attributable to the failing run.
func testPackagedFusionPartialWorkerFailureUsesDocumentedOutcome(
	t *testing.T,
	fixture *fusionSharedFixture,
) {
	input := fmt.Sprintf(
		"functional packaged fusion partial worker failure %d",
		time.Now().UnixNano(),
	)

	runner := packagedFusionFailingCommandRunner{}
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)

	response := startPackagedFusionInvocation(
		t,
		scenario,
		"fusion-partial-worker-failure",
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

	// The invocation response is terminal before the retained event projection
	// necessarily includes the worker's terminal dispatch response. Wait for the
	// public runtime status to become stably terminal before asserting that
	// customer-visible event history.
	support.WaitForSessionTerminalStatus(t, scenario.fixture.baseURL, scenario.sessionID, 10*time.Second)

	dispatches := support.ObserveDispatchEvents(t, support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID))
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

// packagedFusionFailingCommandRunner is the ProviderCommandRunner edge mock
// for this customer-visible failure path. It models a provider subprocess
// failure without selecting the workers/mock feature owned elsewhere.
type packagedFusionFailingCommandRunner struct{}

func (packagedFusionFailingCommandRunner) Run(
	_ context.Context,
	_ platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{
		Stderr:   []byte("packaged fusion provider command failure"),
		ExitCode: 7,
	}, errors.New("packaged fusion provider command failure")
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

func assertFusionProviderCommand(
	t *testing.T,
	request platformprocess.CommandRequest,
	wantProvider, wantModel string,
) {
	t.Helper()

	if request.Command != wantProvider {
		t.Fatalf("provider command = %q, want %q", request.Command, wantProvider)
	}
	for index := 0; index+1 < len(request.Args); index++ {
		if request.Args[index] == "--model" && request.Args[index+1] == wantModel {
			return
		}
	}
	t.Fatalf("provider args = %#v, want --model %q", request.Args, wantModel)
}

func startPackagedFusionInvocation(
	t *testing.T,
	scenario *fusionScenario,
	requestID string,
	args map[string]any,
) factoryapi.InvocationResponse {
	t.Helper()

	return postJSON[factoryapi.InvocationResponse](
		t,
		strings.TrimSuffix(scenario.fixture.baseURL, "/")+
			"/factory-sessions/"+url.PathEscape(scenario.sessionID)+"/invocations",
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
