package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const routeCharacterizationTimeout = 15 * time.Second

// TestWorkerSessionRouteCharacterization_FreshAndExactly32Dispatches records
// the public dispatch-to-Worker Session association before exercising the
// Work-scoped route. The first Work is checked before the bounded accumulation
// and the first, middle, and last Work are checked after exactly 32 sequential
// controlled dispatches.
func TestWorkerSessionRouteCharacterization_FreshAndExactly32Dispatches(t *testing.T) {
	dir := support.ScaffoldSingleStepFactory(t, "worker-sessions-route-characterization")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))

	gate := make(chan struct{})
	close(gate)
	runner := newFunctionalWorkerGate(gate)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(server.URL()))
	observed := make([]routeCharacterizationDispatch, 0, 32)
	for index := 0; index < 32; index++ {
		name := fmt.Sprintf("route-characterization-work-%02d", index+1)
		submitted := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
			Name:         &name,
			WorkTypeName: "task",
			Payload:      map[string]string{"title": name},
		})
		workID := support.StringPointerValue(submitted.WorkId)
		if workID == "" {
			t.Fatalf("dispatch %d submission = %#v, want Work ID", index+1, submitted)
		}
		observed = append(observed, waitForRouteCharacterizationDispatch(t, stream, workID))

		if index == 0 {
			assertRouteCharacterizationRead(t, server, observed[index], true)
		}
	}

	if runner.callCount() != 32 {
		t.Fatalf("controlled provider calls = %d, want exactly 32", runner.callCount())
	}

	for _, index := range []int{0, 15, 31} {
		assertRouteCharacterizationRead(t, server, observed[index], false)
	}
	t.Logf(
		"public dispatch associations: first=%s/%s/%s middle=%s/%s/%s final=%s/%s/%s",
		observed[0].workID, observed[0].dispatchID, observed[0].workerSessionID,
		observed[15].workID, observed[15].dispatchID, observed[15].workerSessionID,
		observed[31].workID, observed[31].dispatchID, observed[31].workerSessionID,
	)

	events := support.GetFactoryEventsAt(t, server.URL())
	associationCount := 0
	responseCount := 0
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation:
			associationCount++
		case factoryapi.FactoryEventTypeDispatchResponse:
			responseCount++
		}
	}
	if associationCount != 32 || responseCount != 32 {
		t.Fatalf("public Factory Event counts = association:%d response:%d, want 32 each", associationCount, responseCount)
	}
}

// TestWorkerSessionRouteCharacterization_AfterDefaultPauseResume records one
// association before the supported default-session pause/resume cycle and one
// after it. Both route reads continue to use the public ~default selector.
func TestWorkerSessionRouteCharacterization_AfterDefaultPauseResume(t *testing.T) {
	dir := support.ScaffoldSingleStepFactory(t, "worker-sessions-route-resume-characterization")
	support.WriteAgentConfig(t, dir, "processor", support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"))

	gate := make(chan struct{})
	close(gate)
	runner := newFunctionalWorkerGate(gate)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     serviceedges.Edges{ProviderCommandRunner: runner},
	})
	t.Cleanup(func() { server.Stop(t) })

	stream := support.OpenFactoryEventStreamAt(t, support.DefaultSessionEventsURL(server.URL()))
	preResume := submitAndObserveRouteCharacterizationWork(t, server, stream, "route-characterization-pre-resume")

	pause := postRouteCharacterizationLifecycleControl(t, server.URL(), "pause")
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}
	resume := postRouteCharacterizationLifecycleControl(t, server.URL(), "resume")
	if resume.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume response = %#v, want accepted resume", resume)
	}

	postResume := submitAndObserveRouteCharacterizationWork(t, server, stream, "route-characterization-post-resume")
	assertRouteCharacterizationRead(t, server, preResume, false)
	assertRouteCharacterizationRead(t, server, postResume, false)
	t.Logf(
		"public dispatch associations across default pause/resume: pre=%s/%s/%s post=%s/%s/%s",
		preResume.workID, preResume.dispatchID, preResume.workerSessionID,
		postResume.workID, postResume.dispatchID, postResume.workerSessionID,
	)

	if runner.callCount() != 2 {
		t.Fatalf("controlled provider calls across pause/resume = %d, want exactly 2", runner.callCount())
	}
}

type routeCharacterizationDispatch struct {
	workID          string
	dispatchID      string
	workerSessionID string
}

type routeCharacterizationDispatchEvents struct {
	workIDs         map[string]bool
	workerSessionID string
}

func submitAndObserveRouteCharacterizationWork(
	t *testing.T,
	server *support.FunctionalAPIServer,
	stream *support.FactoryEventStream,
	name string,
) routeCharacterizationDispatch {
	t.Helper()
	submitted := support.SubmitDefaultSessionWork(t, server.URL(), factoryapi.SubmitWorkRequest{
		Name:         &name,
		WorkTypeName: "task",
		Payload:      map[string]string{"title": name},
	})
	workID := support.StringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("submission %q = %#v, want Work ID", name, submitted)
	}
	return waitForRouteCharacterizationDispatch(t, stream, workID)
}

func waitForRouteCharacterizationDispatch(
	t *testing.T,
	stream *support.FactoryEventStream,
	workID string,
) routeCharacterizationDispatch {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), routeCharacterizationTimeout)
	defer cancel()

	byDispatch := make(map[string]*routeCharacterizationDispatchEvents)
	for {
		event := stream.NextEventContext(ctx)
		if event.Context.DispatchId == nil || strings.TrimSpace(*event.Context.DispatchId) == "" {
			continue
		}
		dispatchID := strings.TrimSpace(*event.Context.DispatchId)
		entry := byDispatch[dispatchID]
		if entry == nil {
			entry = &routeCharacterizationDispatchEvents{workIDs: make(map[string]bool)}
			byDispatch[dispatchID] = entry
		}

		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			if event.Context.WorkIds != nil {
				for _, candidate := range *event.Context.WorkIds {
					entry.workIDs[candidate] = true
				}
			}
			request, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				t.Fatalf("decode public dispatch request for Work %q: %v", workID, err)
			}
			for _, input := range request.Inputs {
				entry.workIDs[input.WorkId] = true
			}
		case factoryapi.FactoryEventTypeDispatchWorkerSessionAssociation:
			association, err := event.Payload.AsDispatchWorkerSessionAssociationEventPayload()
			if err != nil {
				t.Fatalf("decode public Worker Session association for Work %q: %v", workID, err)
			}
			entry.workerSessionID = strings.TrimSpace(association.WorkerSessionId)
		case factoryapi.FactoryEventTypeDispatchResponse:
			if !entry.workIDs[workID] {
				continue
			}
			if entry.workerSessionID == "" {
				t.Fatalf("public dispatch %q completed for Work %q without a Worker Session association", dispatchID, workID)
			}
			return routeCharacterizationDispatch{
				workID:          workID,
				dispatchID:      dispatchID,
				workerSessionID: entry.workerSessionID,
			}
		}
	}
}

func assertRouteCharacterizationRead(
	t *testing.T,
	server *support.FunctionalAPIServer,
	expected routeCharacterizationDispatch,
	includeHumanOutput bool,
) {
	t.Helper()
	if includeHumanOutput {
		inputs := support.FakeInputs(t.Context(), []string{
			"you", "worker-sessions", "list", "--work-id", expected.workID,
			"--server", server.URL(),
		})
		if err := server.Execute(t, inputs.Input); err != nil {
			t.Fatalf("human Worker Sessions list for Work %q: %v\nstderr:\n%s", expected.workID, err, inputs.Stderr())
		}
		if !strings.Contains(inputs.Stdout(), expected.workerSessionID) {
			t.Fatalf("human Worker Sessions list for Work %q = %q, want Worker Session %q", expected.workID, inputs.Stdout(), expected.workerSessionID)
		}
	}

	cliInputs := support.FakeInputs(t.Context(), []string{
		"you", "worker-sessions", "list", "--work-id", expected.workID,
		"--server", server.URL(), "--output", "json",
	})
	if err := server.Execute(t, cliInputs.Input); err != nil {
		t.Fatalf("JSON Worker Sessions list for Work %q: %v\nstderr:\n%s", expected.workID, err, cliInputs.Stderr())
	}
	var cliResponse factoryapi.ListWorkerSessionsResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(cliInputs.Stdout())), &cliResponse); err != nil {
		t.Fatalf("decode JSON Worker Sessions list for Work %q: %v\nstdout:\n%s", expected.workID, err, cliInputs.Stdout())
	}

	restResponse := support.ListDefaultSessionWorkerSessions(t, server.URL(), expected.workID)
	assertRouteCharacterizationObservation(t, "CLI", cliResponse, expected)
	assertRouteCharacterizationObservation(t, "REST", restResponse, expected)
	if !reflect.DeepEqual(cliResponse, restResponse) {
		t.Fatalf("CLI and REST Worker Session responses for Work %q differ:\nCLI: %#v\nREST: %#v", expected.workID, cliResponse, restResponse)
	}
}

func assertRouteCharacterizationObservation(
	t *testing.T,
	transport string,
	response factoryapi.ListWorkerSessionsResponse,
	expected routeCharacterizationDispatch,
) {
	t.Helper()
	if len(response.Sessions) != 1 {
		t.Fatalf("%s Worker Sessions response for Work %q = %#v, want one observation", transport, expected.workID, response.Sessions)
	}
	observation := response.Sessions[0]
	if observation.WorkerSessionId != expected.workerSessionID ||
		observation.AttemptId != expected.dispatchID ||
		observation.State != factoryapi.WorkerSessionObservationStateCompleted {
		t.Fatalf("%s Worker Session identity for Work %q = %#v, want worker=%q attempt=%q state=COMPLETED", transport, expected.workID, observation, expected.workerSessionID, expected.dispatchID)
	}
	if observation.FactorySessionId == nil || *observation.FactorySessionId != factorysessions.DefaultSessionID {
		t.Fatalf("%s Factory Session identity for Work %q = %#v, want %q", transport, expected.workID, observation.FactorySessionId, factorysessions.DefaultSessionID)
	}
	if observation.WorkId == nil || *observation.WorkId != expected.workID ||
		!reflect.DeepEqual(observation.WorkIds, []string{expected.workID}) {
		t.Fatalf("%s Work identity for Worker Session %q = workId:%v workIds:%v, want %q", transport, observation.WorkerSessionId, observation.WorkId, observation.WorkIds, expected.workID)
	}
}

func postRouteCharacterizationLifecycleControl(
	t *testing.T,
	baseURL string,
	operation string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()
	payload, err := json.Marshal(factoryapi.FactorySessionLifecycleControlRequest{})
	if err != nil {
		t.Fatalf("marshal %s request: %v", operation, err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + factorysessions.DefaultSessionID + "/" + operation
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build %s request: %v", operation, err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode %s response: %v", operation, err)
	}
	return result
}
