package subagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const packagedSubagentChildPrimaryResult = "mock worker accepted"

// TestPackagedSubagentReturnsChildResult proves that packaged @you/subagent
// invocation through the public CLI completes with the child's authoritative
// primary agent response instead of echoing the submitted request text,
// including hermetic no-server success without starting an HTTP listener.
func TestPackagedSubagent(t *testing.T) {
	fixture := newSubagentSharedFixture(t)
	t.Run("TestPackagedSubagentReturnsChildResult", func(t *testing.T) {
		testPackagedSubagentReturnsChildResult(t, fixture)
	})
	t.Run("TestPackagedSubagentChildFailureReturnsStableFailure", func(t *testing.T) {
		testPackagedSubagentChildFailureReturnsStableFailure(t, fixture)
	})
	t.Run("TestPackagedSubagentStreamsChildResponseEvents", func(t *testing.T) {
		testPackagedSubagentStreamsChildResponseEvents(t, fixture)
	})
	t.Run("TestPackagedSubagentPropagatesLunaXHighReasoningEffort", func(t *testing.T) {
		testPackagedSubagentPropagatesLunaXHighReasoningEffort(t, fixture)
	})
	t.Run("TestPackagedSubagentOmittedReasoningEffortPreservesProviderDefault", func(t *testing.T) {
		testPackagedSubagentOmittedReasoningEffortPreservesProviderDefault(t, fixture)
	})
}

func testPackagedSubagentReturnsChildResult(t *testing.T, fixture *subagentSharedFixture) {
	t.Run("CLI JSON returns authoritative child primary result", func(t *testing.T) {
		requestText := fmt.Sprintf("functional-packaged-subagent-primary-%d", time.Now().UnixNano())
		runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
			Stdout: support.CodexSuccessStdout(packagedSubagentChildPrimaryResult),
		})
		scenario := fixture.newScenario(t, runner)
		response := runPackagedSubagentCLIJSONInvocation(t, scenario, requestText)

		if response.Status != factoryapi.InvocationTerminalStatusCompleted {
			t.Fatalf("status = %q, want COMPLETED; response = %#v", response.Status, response)
		}
		if got := invocationPrimaryResultText(t, response); got != packagedSubagentChildPrimaryResult {
			t.Fatalf("primaryResult = %q, want %q", got, packagedSubagentChildPrimaryResult)
		}
		if strings.Contains(invocationResponseJSON(t, response), requestText) {
			t.Fatalf("invocation JSON echoed submitted request text %q", requestText)
		}
	})

	t.Run("hermetic named invocation succeeds without listening server", func(t *testing.T) {
		requestText := "hermetic no-server packaged subagent prompt"
		runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
			Stdout: support.CodexSuccessStdout(packagedSubagentChildPrimaryResult),
		})
		scenario := fixture.newScenario(t, runner)
		stdout, listenerStarts := runHermeticPackagedSubagentInvocation(t, scenario, requestText)

		if stdout != packagedSubagentChildPrimaryResult {
			t.Fatalf("stdout = %q, want child primary result %q", stdout, packagedSubagentChildPrimaryResult)
		}
		if stdout == requestText {
			t.Fatal("stdout echoed submitted request text instead of child agent response")
		}
		if listenerStarts != 0 {
			t.Fatalf("HTTP listener start calls = %d, want 0", listenerStarts)
		}
	})
}

// TestPackagedSubagentStreamsChildResponseEvents proves that packaged
// @you/subagent invocation publishes child Factory Response Events on the public
// Factory Session response-event stream, not only the terminal invocation
// primary result returned by the invocation API.
func testPackagedSubagentStreamsChildResponseEvents(t *testing.T, fixture *subagentSharedFixture) {
	requestText := fmt.Sprintf("functional-packaged-subagent-stream-%d", time.Now().UnixNano())
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout(packagedSubagentChildPrimaryResult),
	})
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	response, responseEvents := runPackagedSubagentAPIInvocationWithResponseEvents(t, scenario, requestText)

	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if got := invocationPrimaryResultText(t, response); got != packagedSubagentChildPrimaryResult {
		t.Fatalf("primaryResult = %q, want %q", got, packagedSubagentChildPrimaryResult)
	}
	assertPackagedSubagentChildResponseEvents(t, responseEvents)
}

func testPackagedSubagentPropagatesLunaXHighReasoningEffort(t *testing.T, fixture *subagentSharedFixture) {
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("review complete"),
	})
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	response := postPackagedSubagentArgs(t, scenario, map[string]interface{}{
		"input": "review the implementation", "workerProvider": "CODEX",
		"workerModel": "gpt-5.6-luna", "workerReasoningEffort": "xhigh",
	})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response = %#v, want completed", response)
	}
	want := []string{
		"exec", "--json", "--dangerously-bypass-approvals-and-sandbox",
		"--model", "gpt-5.6-luna",
		"--config", `model_reasoning_effort="xhigh"`,
		"-",
	}
	if got := runner.LastRequest().Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("subagent command args = %#v, want %#v", got, want)
	}
}

func testPackagedSubagentOmittedReasoningEffortPreservesProviderDefault(t *testing.T, fixture *subagentSharedFixture) {
	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("review complete"),
	})
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	response := postPackagedSubagentArgs(t, scenario, map[string]interface{}{
		"input": "review the implementation", "workerProvider": "CODEX",
		"workerModel": "gpt-5.6-luna",
	})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response = %#v, want completed", response)
	}
	want := []string{
		"exec", "--json", "--dangerously-bypass-approvals-and-sandbox",
		"--model", "gpt-5.6-luna",
		"-",
	}
	if got := runner.LastRequest().Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("subagent command args = %#v, want provider default without effort config %#v", got, want)
	}
}

// TestPackagedSubagentChildFailureReturnsStableFailure proves that packaged
// @you/subagent invocation returns a stable failed public terminal outcome when
// the child worker rejects, without emitting a success primary result for the
// failing run.
func testPackagedSubagentChildFailureReturnsStableFailure(t *testing.T, fixture *subagentSharedFixture) {
	t.Run("CLI JSON returns stable failed terminal outcome", func(t *testing.T) {
		requestText := fmt.Sprintf("functional-packaged-subagent-failure-%d", time.Now().UnixNano())
		runner := newPackagedSubagentRejectingRunner()
		scenario := fixture.newScenario(t, runner)
		response, stderr, execErr := runPackagedSubagentCLIJSONFailureInvocation(t, scenario, requestText)

		if execErr == nil {
			t.Fatal("Process.Execute error = nil, want terminal packaged-subagent child failure")
		}
		assertPackagedSubagentStableFailureInvocation(t, response)
		assertPackagedSubagentStableFailureErrorResponse(t, response, stderr)
		if invocationPrimaryResultPresent(response) {
			t.Fatalf("primaryResult = %#v, want no success primary result after child worker failure", response.PrimaryResult)
		}
		if strings.Contains(invocationResponseJSON(t, response), packagedSubagentChildPrimaryResult) {
			t.Fatal("failed invocation JSON contained child success primary result text")
		}
		if strings.Contains(invocationResponseJSON(t, response), requestText) {
			t.Fatalf("failed invocation JSON echoed submitted request text %q", requestText)
		}
	})
	fixture.openPendingScenarioSessions(t)

	t.Run("API returns stable failed terminal outcome", func(t *testing.T) {
		requestText := fmt.Sprintf("functional-packaged-subagent-api-failure-%d", time.Now().UnixNano())
		runner := newPackagedSubagentRejectingRunner()
		scenario := fixture.newScenario(t, runner)
		scenario.open(t)
		response := runPackagedSubagentAPIFailureInvocation(t, scenario, requestText)

		assertPackagedSubagentStableFailureInvocation(t, response)
		if invocationPrimaryResultPresent(response) {
			t.Fatalf("primaryResult = %#v, want no success primary result after child worker failure", response.PrimaryResult)
		}
	})
}

func runPackagedSubagentAPIInvocationWithResponseEvents(
	t *testing.T,
	scenario *subagentScenario,
	requestText string,
) (factoryapi.InvocationResponse, []factoryapi.FactoryResponseEvent) {
	t.Helper()

	response := postPackagedSubagentInvocation(t, scenario, requestText)
	responseEvents := support.GetFactoryResponseEventsAt(t, scenario.fixture.baseURL, scenario.sessionID)
	return response, responseEvents
}

func postPackagedSubagentInvocation(
	t *testing.T,
	scenario *subagentScenario,
	requestText string,
) factoryapi.InvocationResponse {
	t.Helper()
	return postPackagedSubagentArgs(t, scenario, map[string]interface{}{"input": requestText})
}

func postPackagedSubagentArgs(
	t *testing.T,
	scenario *subagentScenario,
	args map[string]interface{},
) factoryapi.InvocationResponse {
	t.Helper()
	body, err := json.Marshal(factoryapi.InvocationRequest{Args: &args})
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	endpoint := strings.TrimSuffix(scenario.fixture.baseURL, "/") +
		"/factory-sessions/" + scenario.sessionID + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	payload, readErr := io.ReadAll(response.Body)
	if readErr != nil {
		t.Fatalf("read invocation response: %v", readErr)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, string(payload))
	}

	var decoded factoryapi.InvocationResponse
	if decodeErr := json.Unmarshal(payload, &decoded); decodeErr != nil {
		t.Fatalf("decode invocation response: %v\npayload:\n%s", decodeErr, string(payload))
	}
	return decoded
}

func assertPackagedSubagentChildResponseEvents(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
) {
	t.Helper()

	if len(events) == 0 {
		t.Fatal("response events are empty, want child Factory Response Events on the public stream surface")
	}

	var dispatchID string
	for index, event := range events {
		if event.DispatchId == nil || strings.TrimSpace(*event.DispatchId) == "" {
			t.Fatalf("response event[%d] = %#v, want child dispatch correlation", index, event)
		}
		if dispatchID == "" {
			dispatchID = *event.DispatchId
		}
		if *event.DispatchId != dispatchID {
			t.Fatalf("response event[%d] dispatch = %q, want %q", index, *event.DispatchId, dispatchID)
		}
		if event.SchemaVersion == "" || event.EventId == "" {
			t.Fatalf("response event[%d] = %#v, want public response-event contract fields", index, event)
		}
	}

	finalEvent := events[len(events)-1]
	if finalEvent.Kind != factoryapi.FactoryResponseEventKindMessage {
		t.Fatalf("final response event kind = %q, want MESSAGE", finalEvent.Kind)
	}
	if finalEvent.Phase != factoryapi.FactoryResponseEventPhaseCompleted {
		t.Fatalf("final response event phase = %q, want COMPLETED", finalEvent.Phase)
	}
	payload, err := finalEvent.Payload.AsFactoryResponseEventMessagePayload()
	if err != nil {
		t.Fatalf("decode final child response event payload: %v", err)
	}
	if got := responseEventMessageText(payload); got != packagedSubagentChildPrimaryResult {
		t.Fatalf("final child response event text = %q, want %q", got, packagedSubagentChildPrimaryResult)
	}
}

func responseEventMessageText(payload factoryapi.FactoryResponseEventMessagePayload) string {
	for _, block := range payload.ContentBlocks {
		text, err := block.AsFactoryResponseEventTextContentBlock()
		if err != nil {
			continue
		}
		if text.Text != "" {
			return text.Text
		}
	}
	return ""
}

func runPackagedSubagentCLIJSONInvocation(
	t *testing.T,
	scenario *subagentScenario,
	requestText string,
) factoryapi.InvocationResponse {
	t.Helper()

	args := []string{
		"you", "--json", "run",
		"--named", factorydefinitions.PackagedSubagentFactoryName,
		"--no-record",
		requestText,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = scenario.environment
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	if err := scenario.fixture.process.Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", inputs.Stderr())
	}

	return support.DecodeInvocationResponseJSON(t, inputs.Stdout())
}

func runHermeticPackagedSubagentInvocation(
	t *testing.T,
	scenario *subagentScenario,
	requestText string,
) (string, int) {
	t.Helper()

	args := []string{
		"you", "run",
		"--named", factorydefinitions.PackagedSubagentFactoryName,
		"--no-record", "--quiet",
		requestText,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = scenario.environment
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	if execErr := scenario.fixture.process.Execute(inputs.Input); execErr != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, execErr, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("named invocation stderr = %q, want empty; stdout=%s", inputs.Stderr(), inputs.Stdout())
	}
	return strings.TrimSpace(inputs.Stdout()), int(scenario.fixture.apiStarter.calls.Load())
}

func newPackagedSubagentRejectingRunner() *testutil.ProviderCommandRunner {
	return testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stderr:   []byte("packaged subagent mock worker failure"),
		ExitCode: 7,
	})
}

func runPackagedSubagentCLIJSONFailureInvocation(
	t *testing.T,
	scenario *subagentScenario,
	requestText string,
) (factoryapi.InvocationResponse, string, error) {
	t.Helper()

	args := []string{
		"you", "--json", "run",
		"--named", factorydefinitions.PackagedSubagentFactoryName,
		"--no-record",
		requestText,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = scenario.environment
	inputs.Input.WorkingDirectory = scenario.workingDirectory
	execErr := scenario.fixture.process.Execute(inputs.Input)

	response := support.DecodeInvocationResponseJSON(t, inputs.Stdout())
	return response, inputs.Stderr(), execErr
}

func runPackagedSubagentAPIFailureInvocation(
	t *testing.T,
	scenario *subagentScenario,
	requestText string,
) factoryapi.InvocationResponse {
	t.Helper()
	return postPackagedSubagentInvocation(t, scenario, requestText)
}

func assertPackagedSubagentStableFailureInvocation(
	t *testing.T,
	response factoryapi.InvocationResponse,
) {
	t.Helper()

	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("status = %q, want FAILED; response = %#v", response.Status, response)
	}
	if response.ErrorCode == nil || strings.TrimSpace(string(*response.ErrorCode)) == "" {
		t.Fatalf("errorCode = %#v, want stable public failure code", response.ErrorCode)
	}
	if response.Message == nil || strings.TrimSpace(*response.Message) == "" {
		t.Fatalf("message = %#v, want stable public failure message", response.Message)
	}
}

func assertPackagedSubagentStableFailureErrorResponse(
	t *testing.T,
	response factoryapi.InvocationResponse,
	stderr string,
) {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(stderr))
	var errorResponse factoryapi.ErrorResponse
	if err := decoder.Decode(&errorResponse); err != nil {
		t.Fatalf("decode ErrorResponse: %v\nstderr:\n%s", err, stderr)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("stderr contains data after ErrorResponse: %v\nstderr:\n%s", err, stderr)
	}
	if errorResponse.Code != factoryapi.ErrorResponseCode(*response.ErrorCode) {
		t.Fatalf("ErrorResponse code = %q, want %q", errorResponse.Code, *response.ErrorCode)
	}
	if errorResponse.Family != factoryapi.ErrorFamilyInternalServerError {
		t.Fatalf("ErrorResponse family = %q, want INTERNAL_SERVER_ERROR", errorResponse.Family)
	}
	if !strings.HasPrefix(errorResponse.Message, *response.Message) {
		t.Fatalf("ErrorResponse message = %q, want prefix %q", errorResponse.Message, *response.Message)
	}
}

func invocationPrimaryResultPresent(response factoryapi.InvocationResponse) bool {
	return response.PrimaryResult != nil && len(*response.PrimaryResult) > 0
}

func invocationPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}

func invocationResponseJSON(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal invocation response: %v", err)
	}
	return string(encoded)
}
