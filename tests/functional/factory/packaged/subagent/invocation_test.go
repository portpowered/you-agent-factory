package subagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	platformhttpserver "github.com/portpowered/infinite-you/pkg/platform/httpserver"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const packagedSubagentChildPrimaryResult = "mock worker accepted"

// TestPackagedSubagentReturnsChildResult proves that packaged @you/subagent
// invocation through the public CLI completes with the child's authoritative
// primary agent response instead of echoing the submitted request text,
// including hermetic no-server success without starting an HTTP listener.
func TestPackagedSubagentReturnsChildResult(t *testing.T) {
	t.Run("CLI JSON returns authoritative child primary result", func(t *testing.T) {
		requestText := fmt.Sprintf("functional-packaged-subagent-primary-%d", time.Now().UnixNano())
		response := runPackagedSubagentCLIJSONInvocation(t, requestText)

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
		stdout, listenerStarts := runHermeticPackagedSubagentInvocation(t, requestText)

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
func TestPackagedSubagentStreamsChildResponseEvents(t *testing.T) {
	requestText := fmt.Sprintf("functional-packaged-subagent-stream-%d", time.Now().UnixNano())
	response, responseEvents := runPackagedSubagentAPIInvocationWithResponseEvents(t, requestText)

	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if got := invocationPrimaryResultText(t, response); got != packagedSubagentChildPrimaryResult {
		t.Fatalf("primaryResult = %q, want %q", got, packagedSubagentChildPrimaryResult)
	}
	assertPackagedSubagentChildResponseEvents(t, responseEvents)
}

func runPackagedSubagentAPIInvocationWithResponseEvents(
	t *testing.T,
	requestText string,
) (factoryapi.InvocationResponse, []factoryapi.FactoryResponseEvent) {
	t.Helper()

	factoryDir := support.InstallPackagedFactory(t, t.TempDir(), factorydefinitions.PackagedSubagentFactoryName)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:        factoryDir,
		UseMockWorkers:    true,
		MockWorkersConfig: packagedSubagentAcceptMockWorkersConfig(),
	})

	response := postPackagedSubagentInvocation(t, server.URL(), requestText)
	responseEvents := support.GetFactoryResponseEventsAt(t, server.URL(), factorysessions.DefaultSessionID)
	return response, responseEvents
}

func postPackagedSubagentInvocation(
	t *testing.T,
	serverURL string,
	requestText string,
) factoryapi.InvocationResponse {
	t.Helper()

	body, err := json.Marshal(packagedSubagentTextInvocationRequest(requestText))
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") +
		"/factory-sessions/" + factorysessions.DefaultSessionID + "/invocations"
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

func packagedSubagentTextInvocationRequest(requestText string) factoryapi.InvocationRequest {
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: requestText,
	}); err != nil {
		panic(fmt.Sprintf("build invocation text content: %v", err))
	}
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := factoryapi.WorkContent{part}
	return factoryapi.InvocationRequest{
		SourceKind: &sourceKind,
		Content:    &content,
	}
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
		if event.Kind != factoryapi.FactoryResponseEventKindMessage {
			t.Fatalf("response event[%d] kind = %q, want MESSAGE child progress", index, event.Kind)
		}
		if event.Phase != factoryapi.FactoryResponseEventPhaseCompleted {
			t.Fatalf("response event[%d] phase = %q, want COMPLETED child message", index, event.Phase)
		}
		if event.SchemaVersion == "" || event.EventId == "" {
			t.Fatalf("response event[%d] = %#v, want public response-event contract fields", index, event)
		}
	}

	finalEvent := events[len(events)-1]
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

func runPackagedSubagentCLIJSONInvocation(t *testing.T, requestText string) factoryapi.InvocationResponse {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, factorydefinitions.PackagedSubagentFactoryName)
	mockWorkersPath := support.WriteMockWorkersConfig(t, packagedSubagentAcceptMockWorkersConfig())

	args := []string{
		"you", "--json", "run",
		"--named", factorydefinitions.PackagedSubagentFactoryName,
		"--with-mock-workers", mockWorkersPath,
		"--no-record",
		requestText,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = t.TempDir()
	if err := support.BuildProcess(t, serviceedges.Edges{}).Execute(inputs.Input); err != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, err, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("stderr = %q, want empty successful-run stderr", inputs.Stderr())
	}

	var response factoryapi.InvocationResponse
	if decodeErr := json.Unmarshal([]byte(strings.TrimSpace(inputs.Stdout())), &response); decodeErr != nil {
		t.Fatalf("decode invocation JSON stdout: %v\nstdout:\n%s", decodeErr, inputs.Stdout())
	}
	return response
}

func runHermeticPackagedSubagentInvocation(t *testing.T, requestText string) (string, int) {
	t.Helper()

	homeDir := t.TempDir()
	workingDirectory := t.TempDir()
	listenerStarts := &listenerStartObservation{}
	process, err := root.BuildProcess(t.Context(), serviceedges.Edges{
		APIServerStarter: listenerStarts.Start,
	})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}

	environment := append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	mockWorkersPath := support.WriteMockWorkersConfig(t, packagedSubagentAcceptMockWorkersConfig())
	args := []string{
		"you", "run",
		"--named", factorydefinitions.PackagedSubagentFactoryName,
		"--with-mock-workers", "--no-record", "--quiet",
		mockWorkersPath, requestText,
	}
	inputs := support.FakeInputs(t.Context(), args)
	inputs.Input.Env = environment
	inputs.Input.WorkingDirectory = workingDirectory
	if execErr := process.Execute(inputs.Input); execErr != nil {
		t.Fatalf("Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s", args, execErr, inputs.Stdout(), inputs.Stderr())
	}
	if inputs.Stderr() != "" {
		t.Fatalf("named invocation stderr = %q, want empty; stdout=%s", inputs.Stderr(), inputs.Stdout())
	}
	return strings.TrimSpace(inputs.Stdout()), int(listenerStarts.calls.Load())
}

func packagedSubagentAcceptMockWorkersConfig() *workers.MockWorkersConfig {
	return &workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{{
			WorkerName:      factorydefinitions.PackagedSubagentWorkerName,
			WorkstationName: factorydefinitions.PackagedSubagentRunWorkstationName,
			RunType:         workers.MockWorkerRunTypeAccept,
		}},
	}
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

type listenerStartObservation struct {
	calls atomic.Int32
}

func (observation *listenerStartObservation) Start(
	_ context.Context,
	_ platformhttpserver.StartRequest,
) error {
	observation.calls.Add(1)
	return errors.New("hermetic packaged subagent invocation attempted to start an HTTP listener")
}
