package poller

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	automationservice "github.com/portpowered/infinite-you/pkg/services/automations"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const btrcHostedServiceEndpoint = "https://linear.characterization.test/graphql"

func TestBTRCP0HostedServiceSuccessCharacterization(t *testing.T) {
	const workID = "linear:issue-characterization"

	clock := clockwork.NewFakeClock()
	httpClient := newBTRCHostedHTTPClient(btrcHostedLinearSuccessBody, false)
	provider := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("Hosted processor completed. COMPLETE"),
	})
	submitted := make(chan work.FactorySubmissionRecord, 1)
	secretCalls := &atomic.Int32{}
	dir := support.ScaffoldFactory(t, btrcHostedServiceFactoryConfig())
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	server := startBTRCHostedService(t, dir, clock, httpClient, provider, submitted, nil, secretCalls)
	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(server.URL()))

	record := waitForBTRCHostedSubmission(t, submitted)
	if record.Request.WorkID != workID {
		t.Fatalf("hosted submission work ID = %q, want %q", record.Request.WorkID, workID)
	}
	if record.Request.TargetState != "init" || record.Request.WorkTypeID != "story" {
		t.Fatalf("hosted submission target = %#v, want story/init", record.Request)
	}
	if record.Request.Tags["external_source"] != "linear" {
		t.Fatalf("hosted submission tags = %#v, want external_source=linear", record.Request.Tags)
	}
	if secretCalls.Load() == 0 || httpClient.callCount() != 1 {
		t.Fatalf("hosted effects calls = secret:%d http:%d, want at least one secret and one HTTP call", secretCalls.Load(), httpClient.callCount())
	}

	waitForBTRCHostedDispatchResponse(t, stream, workID)
	events := server.GetFactoryEvents(t)
	assertBTRCHostedEventOrder(t, events, btrcHostedServiceSuccessEventOrder)
	response := btrcHostedDispatchResponse(t, events, workID)
	if response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("hosted success dispatch outcome = %q, want ACCEPTED", response.Outcome)
	}
	assertBTRCHostedOutputState(t, response, "complete")
	assertBTRCHostedServiceResultReconciliation(t, server.URL(), "complete")
	assertBTRCHostedSingleTerminalWork(t, support.ListDefaultSessionWork(t, server.URL()), workID, "complete")
}

func TestBTRCP0HostedServiceExecutionFailureCharacterization(t *testing.T) {
	const workID = "linear:issue-characterization"

	clock := clockwork.NewFakeClock()
	httpClient := newBTRCHostedHTTPClient(btrcHostedLinearSuccessBody, false)
	provider := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 1,
		Stderr:   []byte("hosted processor unavailable"),
	})
	submitted := make(chan work.FactorySubmissionRecord, 1)
	dir := support.ScaffoldFactory(t, btrcHostedServiceFactoryConfig())
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	server := startBTRCHostedService(t, dir, clock, httpClient, provider, submitted, nil, nil)
	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(server.URL()))

	waitForBTRCHostedSubmission(t, submitted)
	waitForBTRCHostedDispatchResponse(t, stream, workID)
	events := server.GetFactoryEvents(t)
	assertBTRCHostedEventOrder(t, events, btrcHostedServiceFailureEventOrder)
	response := btrcHostedDispatchResponse(t, events, workID)
	if response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("hosted execution-failure outcome = %q, want FAILED", response.Outcome)
	}
	if response.FailureDetail == nil || response.Error == nil {
		t.Fatalf("hosted execution-failure response = %#v, want typed failure detail and error", response)
	}
	assertBTRCHostedOutputState(t, response, "failed")
	if got := provider.CallCount(); got != 1 {
		t.Fatalf("provider command calls = %d, want exactly one dispatch attempt", got)
	}
	assertBTRCHostedServiceResultReconciliation(t, server.URL(), "failed")
	assertBTRCHostedSingleTerminalWork(t, support.ListDefaultSessionWork(t, server.URL()), workID, "failed")
}

func TestBTRCP0HostedServiceSourceFailureStopsAndIsolates(t *testing.T) {
	const failedWorkID = "linear:issue-blocked"

	clock := clockwork.NewFakeClock()
	blockedHTTP := newBTRCHostedHTTPClient(btrcHostedLinearBody("issue-blocked", "ENG-902"), true)
	failedSubmissions := make(chan work.FactorySubmissionRecord, 1)
	failedDispatches := make(chan interfaces.FactoryDispatchRecord, 1)
	secretCalls := &atomic.Int32{}
	dir := support.ScaffoldFactory(t, btrcHostedServiceFactoryConfig())
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	failedServer := startBTRCHostedService(t, dir, clock, blockedHTTP, nil, failedSubmissions, failedDispatches, secretCalls)

	waitForBTRCHostedSignal(t, blockedHTTP.started, "hosted HTTP request")
	if secretCalls.Load() == 0 {
		t.Fatal("hosted source did not resolve its secret before the HTTP request")
	}
	assertBTRCHostedNoSubmission(t, failedSubmissions, failedWorkID)
	assertBTRCHostedNoDispatch(t, failedDispatches)
	assertBTRCHostedEventOrder(t, failedServer.GetFactoryEvents(t), btrcHostedServiceSourceFailureEventOrder)

	failedServer.Stop(t)
	waitForBTRCHostedSignal(t, blockedHTTP.canceled, "hosted HTTP cancellation")

	secondHTTP := newBTRCHostedHTTPClient(btrcHostedLinearBody("issue-isolated", "ENG-903"), false)
	secondProvider := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: support.CodexSuccessStdout("Independent hosted processor completed. COMPLETE"),
	})
	secondSubmissions := make(chan work.FactorySubmissionRecord, 1)
	secondDir := support.ScaffoldFactory(t, btrcHostedServiceFactoryConfig())
	support.WriteAgentConfig(t, secondDir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "gpt-5-codex"))
	secondServer := startBTRCHostedService(t, secondDir, clockwork.NewFakeClock(), secondHTTP, secondProvider, secondSubmissions, nil, nil)
	secondStream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(secondServer.URL()))
	secondRecord := waitForBTRCHostedSubmission(t, secondSubmissions)
	if secondRecord.Request.WorkID != "linear:issue-isolated" {
		t.Fatalf("independent hosted submission work ID = %q, want linear:issue-isolated", secondRecord.Request.WorkID)
	}
	waitForBTRCHostedDispatchResponse(t, secondStream, secondRecord.Request.WorkID)
	secondEvents := secondServer.GetFactoryEvents(t)
	assertBTRCHostedSingleWorkEventScope(t, secondEvents, secondRecord.Request.WorkID)
}

var btrcHostedServiceSuccessEventOrder = []factoryapi.FactoryEventType{
	factoryapi.FactoryEventTypeRunRequest,
	factoryapi.FactoryEventTypeInitialStructureRequest,
	factoryapi.FactoryEventTypeSessionStarted,
	factoryapi.FactoryEventTypeFactoryStateResponse,
	factoryapi.FactoryEventTypeWorkRequest,
	factoryapi.FactoryEventTypeDispatchRequest,
	factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation,
	factoryapi.FactoryEventTypeModelRequest,
	factoryapi.FactoryEventTypeModelResponse,
	factoryapi.FactoryEventTypeAgentRunResponse,
	factoryapi.FactoryEventTypeDispatchResponse,
}

var btrcHostedServiceFailureEventOrder = []factoryapi.FactoryEventType{
	factoryapi.FactoryEventTypeRunRequest,
	factoryapi.FactoryEventTypeInitialStructureRequest,
	factoryapi.FactoryEventTypeSessionStarted,
	factoryapi.FactoryEventTypeFactoryStateResponse,
	factoryapi.FactoryEventTypeWorkRequest,
	factoryapi.FactoryEventTypeDispatchRequest,
	factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation,
	factoryapi.FactoryEventTypeModelRequest,
	factoryapi.FactoryEventTypeModelResponse,
	factoryapi.FactoryEventTypeAgentRunResponse,
	factoryapi.FactoryEventTypeDispatchResponse,
}

var btrcHostedServiceSourceFailureEventOrder = []factoryapi.FactoryEventType{
	factoryapi.FactoryEventTypeRunRequest,
	factoryapi.FactoryEventTypeInitialStructureRequest,
	factoryapi.FactoryEventTypeSessionStarted,
	factoryapi.FactoryEventTypeFactoryStateResponse,
}

func startBTRCHostedService(
	t *testing.T,
	dir string,
	clock automationservice.HostedLinearClock,
	httpClient automationservice.HostedLinearHTTPDoer,
	provider platformprocess.CommandRunner,
	submissions chan<- work.FactorySubmissionRecord,
	dispatches chan<- interfaces.FactoryDispatchRecord,
	secretCalls *atomic.Int32,
) *support.FunctionalAPIServer {
	t.Helper()
	// Hosted service mode is a continuous Process.Execute invocation: after a
	// Work item reaches a terminal state, the live Factory Session intentionally
	// remains IDLE and the process has no finite caller-returned result. These
	// public HTTP reads are therefore the API-owned hosted inspection contract;
	// finite CLI result mapping is characterized by btrc-p0-characterization-005.
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			HostedClock:          clock,
			HostedHTTPClient:     httpClient,
			HostedLinearEndpoint: btrcHostedServiceEndpoint,
			HostedSecretResolver: func(context.Context, automationservice.HostedRuntimePaths, string) (string, error) {
				if secretCalls != nil {
					secretCalls.Add(1)
				}
				return "btrc-secret", nil
			},
			ProviderCommandRunner: provider,
			SubmissionRecorder: func(record work.FactorySubmissionRecord) {
				if submissions != nil {
					submissions <- record
				}
			},
			DispatchRecorder: func(record interfaces.FactoryDispatchRecord) {
				if dispatches != nil {
					dispatches <- record
				}
			},
		},
	})
}

func waitForBTRCHostedSubmission(t *testing.T, submissions <-chan work.FactorySubmissionRecord) work.FactorySubmissionRecord {
	t.Helper()
	select {
	case record := <-submissions:
		return record
	case <-t.Context().Done():
		t.Fatalf("waiting for hosted source submission: %v", t.Context().Err())
		return work.FactorySubmissionRecord{}
	}
}

func waitForBTRCHostedDispatchResponse(t *testing.T, stream *support.FactoryEventStream, workID string) {
	t.Helper()
	for {
		event := stream.NextEventContext(t.Context())
		if event.Type == factoryapi.FactoryEventTypeDispatchResponse && btrcHostedEventIncludesWork(event, workID) {
			return
		}
	}
}

func assertBTRCHostedEventOrder(t *testing.T, events []factoryapi.FactoryEvent, want []factoryapi.FactoryEventType) {
	t.Helper()
	got := make([]factoryapi.FactoryEventType, len(events))
	for index, event := range events {
		got[index] = event.Type
		if event.Context.Sequence != index {
			t.Fatalf("hosted event[%d] sequence = %d, want %d", index, event.Context.Sequence, index)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("hosted canonical event order = %v, want %v", got, want)
	}
}

func btrcHostedDispatchResponse(t *testing.T, events []factoryapi.FactoryEvent, workID string) factoryapi.DispatchResponseEventPayload {
	t.Helper()
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse || !btrcHostedEventIncludesWork(event, workID) {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode hosted dispatch response: %v", err)
		}
		return payload
	}
	t.Fatalf("no hosted dispatch response for work %q", workID)
	return factoryapi.DispatchResponseEventPayload{}
}

func btrcHostedEventIncludesWork(event factoryapi.FactoryEvent, workID string) bool {
	if event.Context.WorkIds == nil {
		return false
	}
	for _, candidate := range *event.Context.WorkIds {
		if candidate == workID {
			return true
		}
	}
	return false
}

func assertBTRCHostedOutputState(t *testing.T, response factoryapi.DispatchResponseEventPayload, state string) {
	t.Helper()
	if response.OutputWork == nil || len(*response.OutputWork) != 1 {
		t.Fatalf("hosted dispatch output work = %#v, want exactly one item", response.OutputWork)
	}
	output := (*response.OutputWork)[0]
	if output.State == nil || output.State.Name != state {
		t.Fatalf("hosted dispatch output state = %#v, want %q", output.State, state)
	}
}

func assertBTRCHostedServiceProgress(t *testing.T, baseURL, terminalState string) {
	t.Helper()
	session := support.GetDefaultSession(t, baseURL)
	if session.Runtime.Status != factoryapi.FactorySessionStatusIDLE {
		t.Fatalf("hosted service runtime status = %q, want IDLE", session.Runtime.Status)
	}
	if session.Runtime.Progress.Categories.Initial != 0 || session.Runtime.Progress.Categories.Processing != 0 {
		t.Fatalf("hosted service progress = %#v, want no initial or processing work", session.Runtime.Progress)
	}
	if terminalState == "complete" && session.Runtime.Progress.Categories.Terminal != 1 {
		t.Fatalf("hosted success terminal count = %d, want 1", session.Runtime.Progress.Categories.Terminal)
	}
	if terminalState == "failed" && session.Runtime.Progress.Categories.Failed != 1 {
		t.Fatalf("hosted failure failed count = %d, want 1", session.Runtime.Progress.Categories.Failed)
	}
}

func assertBTRCHostedServiceResultReconciliation(t *testing.T, baseURL, terminalState string) {
	t.Helper()
	assertBTRCHostedServiceProgress(t, baseURL, terminalState)

	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodGet,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+factorysessions.DefaultSessionID+"/result",
		nil,
	)
	if err != nil {
		t.Fatalf("build hosted live result request: %v", err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("GET hosted live result: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("hosted live result status = %d body=%q, want 404 because continuous Petri service sessions have no terminal live result", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var errorResponse factoryapi.ErrorResponse
	if err := json.NewDecoder(response.Body).Decode(&errorResponse); err != nil {
		t.Fatalf("decode hosted live result absence: %v", err)
	}
	if errorResponse.Code != factoryapi.ErrorResponseCodeNOTFOUND {
		t.Fatalf("hosted live result absence code = %q, want NOT_FOUND", errorResponse.Code)
	}
}

func assertBTRCHostedSingleTerminalWork(t *testing.T, listed factoryapi.ListWorkResponse, workID, state string) {
	t.Helper()
	matched := 0
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) != workID {
			continue
		}
		matched++
		if item.State == nil || item.State.Name != state {
			t.Fatalf("listed hosted work = %#v, want state %q", item, state)
		}
	}
	if matched != 1 {
		t.Fatalf("listed hosted work ID %q count = %d, want exactly one", workID, matched)
	}
}

func assertBTRCHostedNoSubmission(t *testing.T, submissions <-chan work.FactorySubmissionRecord, workID string) {
	t.Helper()
	select {
	case record := <-submissions:
		t.Fatalf("source failure submitted work %q, want no submission for %q", record.Request.WorkID, workID)
	default:
	}
}

func assertBTRCHostedNoDispatch(t *testing.T, dispatches <-chan interfaces.FactoryDispatchRecord) {
	t.Helper()
	select {
	case record := <-dispatches:
		t.Fatalf("source failure dispatched work %v, want no dispatch", record.Dispatch.Execution.WorkIDs)
	default:
	}
}

func assertBTRCHostedSingleWorkEventScope(t *testing.T, events []factoryapi.FactoryEvent, workID string) {
	t.Helper()
	for _, event := range events {
		if event.Context.WorkIds == nil {
			continue
		}
		for _, candidate := range *event.Context.WorkIds {
			if candidate != workID {
				t.Fatalf("independent session event %s references work %q, want only %q", event.Type, candidate, workID)
			}
		}
	}
}

func waitForBTRCHostedSignal(t *testing.T, signal <-chan struct{}, description string) {
	t.Helper()
	select {
	case <-signal:
	case <-t.Context().Done():
		t.Fatalf("waiting for %s: %v", description, t.Context().Err())
	}
}

func btrcHostedServiceFactoryConfig() map[string]any {
	return map[string]any{
		"workTypes": []map[string]any{{
			"name": "story",
			"states": []map[string]string{
				{"name": "source", "type": "PROCESSING"},
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{
			{
				"name":     "linear-poller",
				"type":     "HOSTED_WORKER",
				"provider": "LINEAR",
				"auth":     map[string]string{"secretRef": "secrets/linear-api-key"},
				"linear": map[string]any{
					"pollInterval": "1h",
					"mapping":      map[string]string{"workType": "story", "state": "init"},
				},
			},
			{"name": "processor"},
		},
		"workstations": []map[string]any{
			{
				"name":     "poll-linear",
				"behavior": "POLLER",
				"worker":   "linear-poller",
				"inputs":   []map[string]string{{"workType": "story", "state": "source"}},
				"outputs":  []map[string]string{{"workType": "story", "state": "source"}},
				"onFailure": []map[string]string{{
					"workType": "story",
					"state":    "failed",
				}},
			},
			{
				"name":   "process-story",
				"worker": "processor",
				"inputs": []map[string]string{{"workType": "story", "state": "init"}},
				"outputs": []map[string]string{{
					"workType": "story",
					"state":    "complete",
				}},
				"onFailure": []map[string]string{{
					"workType": "story",
					"state":    "failed",
				}},
			},
		},
	}
}

type btrcHostedHTTPClient struct {
	mu       sync.Mutex
	body     string
	block    bool
	calls    int
	started  chan struct{}
	canceled chan struct{}
	start    sync.Once
	cancel   sync.Once
}

func newBTRCHostedHTTPClient(body string, block bool) *btrcHostedHTTPClient {
	return &btrcHostedHTTPClient{
		body:     body,
		block:    block,
		started:  make(chan struct{}),
		canceled: make(chan struct{}),
	}
}

func (c *btrcHostedHTTPClient) Do(request *http.Request) (*http.Response, error) {
	c.mu.Lock()
	c.calls++
	body := c.body
	block := c.block
	c.mu.Unlock()
	c.start.Do(func() { close(c.started) })
	if block {
		<-request.Context().Done()
		c.cancel.Do(func() { close(c.canceled) })
		return nil, request.Context().Err()
	}
	if body == "" {
		body = btrcHostedLinearSuccessBody
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    request,
	}, nil
}

func (c *btrcHostedHTTPClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func btrcHostedLinearBody(issueID, identifier string) string {
	body := strings.ReplaceAll(btrcHostedLinearSuccessBody, "issue-characterization", issueID)
	return strings.ReplaceAll(body, "ENG-901", identifier)
}

const btrcHostedLinearSuccessBody = `{
	"data": {
		"issues": {
			"nodes": [{
				"id": "issue-characterization",
				"identifier": "ENG-901",
				"title": "Hosted characterization",
				"description": "Freeze hosted service behavior",
				"updatedAt": "2026-05-22T08:10:00Z",
				"url": "https://linear.app/example/issue/ENG-901",
				"team": {"id": "team-characterization", "key": "ENG", "name": "Engineering"},
				"state": {"id": "state-characterization", "name": "Todo", "type": "unstarted"},
				"assignee": null
			}],
			"pageInfo": {"hasNextPage": false, "endCursor": ""}
		}
	}
}`
