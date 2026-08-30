package root_composition_test

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

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

type concurrencyInvocationResult struct {
	response factoryapi.InvocationResponse
	err      error
}

func postConcurrencyInvocation(ctx context.Context, baseURL, sessionID, marker string) (factoryapi.InvocationResponse, error) {
	request, err := concurrencyInvocationRequest(marker)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	payload, err := json.Marshal(request)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/invocations"
	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	if response.StatusCode != http.StatusOK {
		return factoryapi.InvocationResponse{}, fmt.Errorf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded factoryapi.InvocationResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return factoryapi.InvocationResponse{}, err
	}
	return decoded, nil
}

func concurrencyInvocationRequest(marker string) (factoryapi.InvocationRequest, error) {
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{Type: factoryapi.WorkContentPartTypeText, Text: marker}); err != nil {
		return factoryapi.InvocationRequest{}, err
	}
	sourceKind := factoryapi.InvocationInputSourceKindText
	content := factoryapi.WorkContent{part}
	return factoryapi.InvocationRequest{SourceKind: &sourceKind, Content: &content}, nil
}

func awaitConcurrencyInvocation(t *testing.T, results <-chan concurrencyInvocationResult) concurrencyInvocationResult {
	t.Helper()
	select {
	case result := <-results:
		return result
	case <-time.After(concurrencySharedProcessTimeout):
		t.Fatal("timed out waiting for Factory Session invocation")
		return concurrencyInvocationResult{}
	}
}

func assertConcurrencyInvocationPrimaryResult(t *testing.T, response factoryapi.InvocationResponse, want string) {
	t.Helper()
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("invocation primary result = %#v, want one text part containing %q", response.PrimaryResult, want)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("invocation primary result part = %#v: %v", (*response.PrimaryResult)[0], err)
	}
	if !strings.Contains(part.Text, want) {
		t.Fatalf("invocation primary result text = %q, want marker %q", part.Text, want)
	}
}

func assertConcurrencyInvocationMarkerIsolation(
	t *testing.T,
	result concurrencyInvocationResult,
	sessionID string,
	wantMarker string,
	foreignMarker string,
) {
	t.Helper()
	if result.err != nil || result.response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("%s CC-03 invocation result = %#v, error=%v, want COMPLETED", sessionID, result.response, result.err)
	}
	if result.response.SessionId != nil && *result.response.SessionId != sessionID {
		t.Fatalf("%s CC-03 invocation response session = %q, want %q", sessionID, *result.response.SessionId, sessionID)
	}
	assertConcurrencyInvocationPrimaryResult(t, result.response, wantMarker)
	part, err := (*result.response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("%s CC-03 invocation result part = %#v: %v", sessionID, (*result.response.PrimaryResult)[0], err)
	}
	if strings.Contains(part.Text, foreignMarker) {
		t.Fatalf("%s CC-03 invocation result text = %q, contains peer marker %q", sessionID, part.Text, foreignMarker)
	}
}

func assertConcurrencyCanceledInvocationResult(t *testing.T, result concurrencyInvocationResult) {
	t.Helper()
	if result.err != nil && !errors.Is(result.err, context.Canceled) &&
		!strings.Contains(result.err.Error(), "status = 404") {
		t.Fatalf("CC-04 canceled invocation error = %v", result.err)
	}
	if result.err == nil && result.response.Status != factoryapi.InvocationTerminalStatusCanceled {
		t.Fatalf("CC-04 canceled invocation response = %#v, want CANCELED", result.response)
	}
}

func cancelConcurrencySession(baseURL, sessionID string) error {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/cancel"
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{}`))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusOK || response.StatusCode == http.StatusAccepted {
		return nil
	}
	body, _ := io.ReadAll(response.Body)
	return fmt.Errorf("POST %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
}

func waitConcurrencyCategories(t *testing.T, baseURL, sessionID string, accept func(factoryapi.StatusResponse) bool) factoryapi.StatusResponse {
	t.Helper()
	result, err := support.WaitForObservation(concurrencySharedProcessTimeout,
		func() (factoryapi.StatusResponse, error) { return readConcurrencyStatus(baseURL, sessionID) },
		accept,
	)
	if err != nil {
		events := concurrencySessionEvents(t, baseURL, sessionID)
		work := listConcurrencyWork(t, baseURL, sessionID)
		t.Fatalf("wait for Factory Session %q categories: %v; events=%v; work=%#v", sessionID, err, concurrencyEventTypeSummary(events), work.Results)
	}
	return result
}

func waitConcurrencyWorkSettled(t *testing.T, baseURL, sessionID string, wantCompleted int) factoryapi.StatusResponse {
	t.Helper()
	return waitConcurrencyCategories(t, baseURL, sessionID, func(status factoryapi.StatusResponse) bool {
		completed := status.Categories.Terminal + status.Categories.Failed
		return completed >= wantCompleted && status.Categories.Initial == 0 && status.Categories.Processing == 0
	})
}

func readConcurrencyStatus(baseURL, sessionID string) (factoryapi.StatusResponse, error) {
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/status"
	response, err := http.Get(endpoint)
	if err != nil {
		return factoryapi.StatusResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		return factoryapi.StatusResponse{}, fmt.Errorf("GET %s status = %d: %s", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.StatusResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return factoryapi.StatusResponse{}, err
	}
	return result, nil
}

func submitConcurrencyWork(t *testing.T, session *concurrencySession, marker string) factoryapi.SubmitWorkResponse {
	t.Helper()
	name := session.name + "-" + marker
	trace := "trace-" + name
	return support.SubmitSessionWorkAt(t, session.fixture.baseURL, session.id, factoryapi.SubmitWorkRequest{
		Name:         &name,
		TraceId:      &trace,
		WorkTypeName: "task",
		Payload:      map[string]string{"marker": marker},
	})
}

func concurrencyWorkRequest(requestID, marker, name string) factoryapi.WorkRequest {
	workType := "task"
	trace := "trace-" + requestID
	works := []factoryapi.Work{{Name: name, Payload: map[string]string{"marker": marker}, TraceId: &trace, WorkTypeName: &workType}}
	return factoryapi.WorkRequest{RequestId: requestID, Type: factoryapi.WorkRequestTypeFactoryRequestBatch, Works: &works}
}

func upsertConcurrencyWorkRequest(t *testing.T, baseURL, sessionID string, request factoryapi.WorkRequest) factoryapi.UpsertWorkRequestResponse {
	t.Helper()
	status, body, response := upsertConcurrencyWorkRequestStatus(t, baseURL, sessionID, request)
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		t.Fatalf("PUT Work request %q status = %d: %s", request.RequestId, status, body)
	}
	return response
}

func upsertConcurrencyWorkRequestStatus(
	t testing.TB,
	baseURL, sessionID string,
	request factoryapi.WorkRequest,
) (int, string, factoryapi.UpsertWorkRequestResponse) {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal Work request %q: %v", request.RequestId, err)
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work-requests/" + url.PathEscape(request.RequestId)
	httpRequest, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build PUT %s: %v", endpoint, err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("PUT %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read PUT %s: %v", endpoint, err)
	}
	var decoded factoryapi.UpsertWorkRequestResponse
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("decode PUT %s: %v", endpoint, err)
		}
	}
	return response.StatusCode, string(body), decoded
}

func listConcurrencyWork(t testing.TB, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

func getConcurrencyFactorySession(t testing.TB, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](t, endpoint)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode Factory Session %q: %v", sessionID, err)
	}
	return session
}

func listConcurrencyWorkerSessions(
	t testing.TB,
	baseURL, sessionID string,
	workID *string,
) factoryapi.ListWorkerSessionsResponse {
	t.Helper()
	work := stringPointerValue(workID)
	if work == "" {
		t.Fatal("Worker Session lookup Work ID is empty")
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/worker-sessions?workId=" + url.QueryEscape(work)
	return support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, endpoint)
}

func concurrencyWorkerSessionForWork(
	t testing.TB,
	session *concurrencySession,
	workID *string,
) factoryapi.WorkerSessionObservation {
	t.Helper()
	listed := listConcurrencyWorkerSessions(t, session.fixture.baseURL, session.id, workID)
	if len(listed.Sessions) != 1 {
		t.Fatalf("%s Worker Sessions for Work %q = %#v, want one public attempt", session.name, stringPointerValue(workID), listed.Sessions)
	}
	observation := listed.Sessions[0]
	if strings.TrimSpace(observation.WorkerSessionId) == "" || strings.TrimSpace(observation.AttemptId) == "" || observation.WorkId == nil || *observation.WorkId != stringPointerValue(workID) {
		t.Fatalf("%s Worker Session for Work %q = %#v, want Worker Session, attempt, and Work identities", session.name, stringPointerValue(workID), observation)
	}
	return observation
}

func cancelConcurrencyWorkerSession(
	t testing.TB,
	baseURL, workerSessionID string,
) (int, string, factoryapi.WorkerSessionControlResponse) {
	t.Helper()
	if strings.TrimSpace(workerSessionID) == "" {
		t.Fatal("Worker Session control ID is empty")
	}
	endpoint := strings.TrimSuffix(baseURL, "/") + "/worker-sessions/" + url.PathEscape(workerSessionID) + "/cancel"
	request, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{}`))
	if err != nil {
		t.Fatalf("build POST %s: %v", endpoint, err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read POST %s: %v", endpoint, err)
	}
	var control factoryapi.WorkerSessionControlResponse
	if response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices {
		if err := json.Unmarshal(body, &control); err != nil {
			t.Fatalf("decode POST %s: %v", endpoint, err)
		}
	}
	return response.StatusCode, string(body), control
}

func concurrencySessionEvents(t testing.TB, baseURL, sessionID string) []factoryapi.FactoryEvent {
	t.Helper()
	return support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)
}

func concurrencyEventTypeSummary(events []factoryapi.FactoryEvent) []string {
	result := make([]string, 0, len(events))
	for _, event := range events {
		result = append(result, string(event.Type))
	}
	return result
}

func assertConcurrencyCompletedWorks(t *testing.T, session *concurrencySession, responses []factoryapi.SubmitWorkResponse) {
	t.Helper()
	if support.CountWorkAtCustomerState(listConcurrencyWork(t, session.fixture.baseURL, session.id), "task:complete") != len(responses) {
		t.Fatalf("%s completed Work count != %d", session.name, len(responses))
	}
	for _, response := range responses {
		assertConcurrencyWorkCompleted(t, session, response.WorkId, "")
	}
	assertConcurrencyCounts(t, session, len(responses), len(responses))
}

func assertConcurrencyWorkCompleted(t *testing.T, session *concurrencySession, workID *string, marker string) {
	t.Helper()
	work := concurrencyWorkByID(t, session, workID)
	if work.State == nil || work.State.Type != factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("%s Work %q state = %#v, want terminal", session.name, stringPointerValue(workID), work.State)
	}
	if marker != "" && !strings.Contains(workContentText(t, work), marker) {
		t.Fatalf("%s Work %q output omitted marker %q: %#v", session.name, stringPointerValue(workID), marker, work.Content)
	}
}

func assertConcurrencyWorkNotCompleted(t *testing.T, session *concurrencySession, workID *string) {
	t.Helper()
	work := concurrencyWorkByID(t, session, workID)
	if work.State != nil && work.State.Type == factoryapi.WorkStateTypeTERMINAL {
		t.Fatalf("%s canceled Work %q unexpectedly completed: %#v", session.name, stringPointerValue(workID), work)
	}
}

func assertConcurrencyWorkFailed(t *testing.T, session *concurrencySession, workID *string, reason factoryapi.WorkFailureType, message string) {
	t.Helper()
	work := concurrencyWorkByID(t, session, workID)
	if work.State == nil || work.State.Type != factoryapi.WorkStateTypeFAILED || work.FailureDetail == nil {
		t.Fatalf("%s Work %q = %#v, want failed Work with detail", session.name, stringPointerValue(workID), work)
	}
	if reason != "" && work.FailureDetail.Reason != reason {
		t.Fatalf("%s Work %q failure reason = %q, want %q", session.name, stringPointerValue(workID), work.FailureDetail.Reason, reason)
	}
	if message != "" && !strings.Contains(strings.ToLower(work.FailureDetail.Message), strings.ToLower(message)) {
		t.Fatalf("%s Work %q failure message = %q, want %q", session.name, stringPointerValue(workID), work.FailureDetail.Message, message)
	}
}

func concurrencyWorkByID(t *testing.T, session *concurrencySession, workID *string) factoryapi.Work {
	t.Helper()
	want := stringPointerValue(workID)
	if want == "" {
		t.Fatal("Work ID is empty")
	}
	listed := listConcurrencyWork(t, session.fixture.baseURL, session.id)
	for _, work := range listed.Results {
		if stringPointerValue(work.WorkId) == want {
			return work
		}
	}
	t.Fatalf("%s Work %q is absent: %#v", session.name, want, listed.Results)
	return factoryapi.Work{}
}

func workContentText(t *testing.T, work factoryapi.Work) string {
	t.Helper()
	if work.Content == nil {
		return ""
	}
	var builder strings.Builder
	for _, part := range *work.Content {
		textPart, err := part.AsWorkTextContentPart()
		if err == nil {
			builder.WriteString(textPart.Text)
		}
	}
	return builder.String()
}

func assertConcurrencyCounts(t *testing.T, session *concurrencySession, workCount, dispatchCount int) {
	t.Helper()
	events := concurrencySessionEvents(t, session.fixture.baseURL, session.id)
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != dispatchCount {
		t.Fatalf("%s dispatch observations = %d, want %d; %#v", session.name, len(dispatches), dispatchCount, dispatches)
	}
	for _, dispatch := range dispatches {
		if dispatch.DispatchID == "" || dispatch.Response == nil {
			t.Fatalf("%s dispatch = %#v, want terminal correlated response", session.name, dispatch)
		}
	}
	if got := len(listConcurrencyWork(t, session.fixture.baseURL, session.id).Results); got != workCount {
		t.Fatalf("%s listed Work count = %d, want %d", session.name, got, workCount)
	}
}

func assertDistinctConcurrencyDispatches(
	t *testing.T,
	first *concurrencySession,
	firstWork factoryapi.SubmitWorkResponse,
	second *concurrencySession,
	secondWork factoryapi.SubmitWorkResponse,
) {
	t.Helper()
	firstEvents := concurrencySessionEvents(t, first.fixture.baseURL, first.id)
	secondEvents := concurrencySessionEvents(t, second.fixture.baseURL, second.id)
	firstDispatches := support.ObserveDispatchEvents(t, firstEvents)
	secondDispatches := support.ObserveDispatchEvents(t, secondEvents)
	if len(firstDispatches) != 1 || len(secondDispatches) != 1 || firstDispatches[0].DispatchID == secondDispatches[0].DispatchID {
		t.Fatalf("CC-03 dispatch identities = %#v/%#v, want distinct one-per-session dispatches", firstDispatches, secondDispatches)
	}
	assertConcurrencySessionMarkerIsolation(t, first, firstEvents, firstWork, second.marker)
	assertConcurrencySessionMarkerIsolation(t, second, secondEvents, secondWork, first.marker)
}

func assertConcurrencySessionOrdering(
	t *testing.T,
	session *concurrencySession,
	responses []factoryapi.SubmitWorkResponse,
) {
	t.Helper()
	events := concurrencySessionEvents(t, session.fixture.baseURL, session.id)
	lastSessionSequence := 0
	haveSessionSequence := false
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != session.id {
			t.Fatalf("%s event escaped session: %#v", session.name, event.Context)
		}
		if event.Context.SessionSequence != nil {
			if haveSessionSequence && *event.Context.SessionSequence <= lastSessionSequence {
				t.Fatalf("%s session sequence regressed at event %q: previous=%d current=%d", session.name, event.Id, lastSessionSequence, *event.Context.SessionSequence)
			}
			lastSessionSequence = *event.Context.SessionSequence
			haveSessionSequence = true
		}
	}
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 2 {
		t.Fatalf("%s ordering dispatches = %d, want two", session.name, len(dispatches))
	}
	if err := concurrencyDispatchStartOrderError(dispatches); err != nil {
		t.Fatalf("%s dispatch start timestamps are not ordered: %v; observations=%#v", session.name, err, dispatches)
	}
	if len(responses) != len(dispatches) {
		t.Fatalf("%s ordering responses = %d, want one per dispatch", session.name, len(responses))
	}
	for _, response := range responses {
		assertConcurrencyRequestDispatchTerminalCorrelation(t, session, events, response)
	}
}

func assertConcurrencySessionMarkerIsolation(
	t *testing.T,
	session *concurrencySession,
	events []factoryapi.FactoryEvent,
	response factoryapi.SubmitWorkResponse,
	foreignMarker string,
) {
	t.Helper()
	workID := stringPointerValue(response.WorkId)
	if workID == "" || strings.TrimSpace(response.RequestId) == "" {
		t.Fatalf("%s CC-03 response = %#v, want Work and request identities", session.name, response)
	}
	work := concurrencyWorkByID(t, session, response.WorkId)
	content := workContentText(t, work)
	if !strings.Contains(content, session.marker) || strings.Contains(content, foreignMarker) {
		t.Fatalf("%s CC-03 Work %q content = %q, want own marker %q and no peer marker %q", session.name, workID, content, session.marker, foreignMarker)
	}

	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 || !support.DispatchObservationIncludesWork(dispatches[0], workID) || dispatches[0].Response == nil {
		t.Fatalf("%s CC-03 dispatch = %#v, want one terminal dispatch for Work %q", session.name, dispatches, workID)
	}
	output := support.StringPointerValue(dispatches[0].Response.Output)
	if !strings.Contains(output, session.marker) || strings.Contains(output, foreignMarker) {
		t.Fatalf("%s CC-03 dispatch output = %q, want own marker %q and no peer marker %q", session.name, output, session.marker, foreignMarker)
	}

	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal %s CC-03 Factory Events: %v", session.name, err)
	}
	if !strings.Contains(string(encoded), session.marker) || strings.Contains(string(encoded), foreignMarker) {
		t.Fatalf("%s CC-03 Factory Events = %s, want own marker %q and no peer marker %q", session.name, encoded, session.marker, foreignMarker)
	}
	hasRequestWorkCorrelation := false
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != session.id {
			t.Fatalf("%s CC-03 event escaped session: %#v", session.name, event.Context)
		}
		if event.Context.RequestId != nil && *event.Context.RequestId == response.RequestId && concurrencyEventHasWork(event, workID) {
			hasRequestWorkCorrelation = true
		}
	}
	if !hasRequestWorkCorrelation {
		t.Fatalf("%s CC-03 Factory Events have no request %q to Work %q correlation", session.name, response.RequestId, workID)
	}
}

func concurrencyEventHasWork(event factoryapi.FactoryEvent, workID string) bool {
	if event.Context.WorkIds != nil {
		for _, candidate := range *event.Context.WorkIds {
			if candidate == workID {
				return true
			}
		}
	}
	switch event.Type {
	case factoryapi.FactoryEventTypeWorkRequest:
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err == nil && payload.Works != nil {
			for _, work := range *payload.Works {
				if stringPointerValue(work.WorkId) == workID {
					return true
				}
			}
		}
	case factoryapi.FactoryEventTypeDispatchRequest:
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err == nil {
			for _, input := range payload.Inputs {
				if input.WorkId == workID {
					return true
				}
			}
		}
	case factoryapi.FactoryEventTypeWorkStateChange:
		payload, err := event.Payload.AsWorkStateChangeEventPayload()
		return err == nil && payload.WorkId == workID
	}
	return false
}

func concurrencyEventSummary(events []factoryapi.FactoryEvent) string {
	result := make([]string, 0, len(events))
	for index, event := range events {
		requestID := ""
		if event.Context.RequestId != nil {
			requestID = *event.Context.RequestId
		}
		dispatchID := ""
		if event.Context.DispatchId != nil {
			dispatchID = *event.Context.DispatchId
		}
		workIDs := []string{}
		if event.Context.WorkIds != nil {
			workIDs = append(workIDs, (*event.Context.WorkIds)...)
		}
		detail := ""
		if event.Type == factoryapi.FactoryEventTypeWorkStateChange {
			if payload, err := event.Payload.AsWorkStateChangeEventPayload(); err == nil {
				detail = fmt.Sprintf("work=%q to=%q", payload.WorkId, payload.ToState)
			}
		}
		result = append(result, fmt.Sprintf("#%d %s seq=%d sessionSeq=%v request=%q dispatch=%q contextWork=%v %s", index, event.Type, event.Context.Sequence, event.Context.SessionSequence, requestID, dispatchID, workIDs, detail))
	}
	return strings.Join(result, "; ")
}

func concurrencyDispatchStartOrderError(dispatches []support.DispatchEventObservation) error {
	for index := 1; index < len(dispatches); index++ {
		previous := dispatches[index-1]
		current := dispatches[index]
		if current.StartedAt.Before(previous.StartedAt) {
			return fmt.Errorf("dispatch %q started at %s before prior dispatch %q at %s", current.DispatchID, current.StartedAt.Format(time.RFC3339Nano), previous.DispatchID, previous.StartedAt.Format(time.RFC3339Nano))
		}
		// Equal wall-clock timestamps are accepted by policy; canonical event
		// order and per-session sequence remain authoritative at this precision.
	}
	return nil
}

func assertConcurrencyRequestDispatchTerminalCorrelation(
	t *testing.T,
	session *concurrencySession,
	events []factoryapi.FactoryEvent,
	response factoryapi.SubmitWorkResponse,
) {
	t.Helper()
	workID := stringPointerValue(response.WorkId)
	requestID := strings.TrimSpace(response.RequestId)
	if workID == "" || requestID == "" {
		t.Fatalf("%s ordering response = %#v, want Work/request identities", session.name, response)
	}
	type eventIndex struct {
		request  int
		dispatch int
		terminal int
	}
	indices := eventIndex{request: -1, dispatch: -1, terminal: -1}
	dispatchID := ""
	for index, event := range events {
		if event.Context.RequestId == nil || *event.Context.RequestId != requestID || !concurrencyEventHasWork(event, workID) {
			continue
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			if indices.request != -1 {
				t.Fatalf("%s Work %q request %q has duplicate WORK_REQUEST events", session.name, workID, requestID)
			}
			indices.request = index
		case factoryapi.FactoryEventTypeDispatchRequest:
			if indices.dispatch != -1 {
				t.Fatalf("%s Work %q request %q has duplicate DISPATCH_REQUEST events", session.name, workID, requestID)
			}
			indices.dispatch = index
			if event.Context.DispatchId == nil || strings.TrimSpace(*event.Context.DispatchId) == "" {
				t.Fatalf("%s Work %q dispatch request has no dispatch identity", session.name, workID)
			}
			dispatchID = *event.Context.DispatchId
		}
	}
	for index, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			if event.Context.DispatchId == nil || *event.Context.DispatchId != dispatchID || !concurrencyEventHasWork(event, workID) || event.Context.SessionId == nil || *event.Context.SessionId != session.id {
				continue
			}
			if indices.terminal != -1 {
				t.Fatalf("%s Work %q request %q has duplicate terminal DISPATCH_RESPONSE events", session.name, workID, requestID)
			}
			indices.terminal = index
		}
	}
	if indices.request == -1 || indices.dispatch == -1 || indices.terminal == -1 {
		t.Fatalf("%s Work %q request %q timeline indexes = %#v, want WORK_REQUEST, DISPATCH_REQUEST, terminal DISPATCH_RESPONSE; events=%s", session.name, workID, requestID, indices, concurrencyEventSummary(events))
	}
	if !(indices.request < indices.dispatch && indices.dispatch < indices.terminal) {
		t.Fatalf("%s Work %q request %q event order = %#v, want request < dispatch < terminal", session.name, workID, requestID, indices)
	}
	for _, dispatch := range support.ObserveDispatchEvents(t, events) {
		if dispatch.DispatchID != dispatchID {
			continue
		}
		if !support.DispatchObservationIncludesWork(dispatch, workID) || dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("%s Work %q dispatch = %#v, want accepted terminal response", session.name, workID, dispatch)
		}
		return
	}
	t.Fatalf("%s Work %q dispatch %q is absent from public dispatch observations", session.name, workID, dispatchID)
}

func assertConcurrencyEventIDsUnchanged(t *testing.T, want, got []factoryapi.FactoryEvent, operation string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("event count after %s = %d, want unchanged %d", operation, len(got), len(want))
	}
	for index := range want {
		if got[index].Id != want[index].Id {
			t.Fatalf("event %d after %s = %q, want %q", index, operation, got[index].Id, want[index].Id)
		}
	}
}

func countConcurrencyEvents(events []factoryapi.FactoryEvent, want factoryapi.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == want {
			count++
		}
	}
	return count
}

func readConcurrencyResponseEventsUntilTerminal(t *testing.T, stream *support.FactoryResponseEventStream, timeout time.Duration) []factoryapi.FactoryResponseEvent {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var events []factoryapi.FactoryResponseEvent
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("timed out waiting for terminal concurrency response event; got %d events", len(events))
		}
		result := stream.TryNextFrameResult(remaining)
		if result.Outcome != support.FactoryResponseEventStreamOutcomeFrame {
			t.Fatalf("concurrency response stream ended before terminal event: %s", result.Diagnostic())
		}
		events = append(events, result.Frame.Event)
		event := result.Frame.Event
		if event.Kind == factoryapi.FactoryResponseEventKindError && (event.Phase == factoryapi.FactoryResponseEventPhaseFailed || event.Phase == factoryapi.FactoryResponseEventPhaseCanceled) {
			return events
		}
		if event.Kind == factoryapi.FactoryResponseEventKindRun && (event.Phase == factoryapi.FactoryResponseEventPhaseCompleted || event.Phase == factoryapi.FactoryResponseEventPhaseFailed || event.Phase == factoryapi.FactoryResponseEventPhaseCanceled) {
			return events
		}
	}
}

func assertConcurrencyCancellationResponseEvents(t *testing.T, events []factoryapi.FactoryResponseEvent, sessionID string) {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("CC-04 cancellation response events are empty")
	}
	for _, event := range events {
		if event.FactorySessionId != sessionID {
			t.Fatalf("CC-04 cancellation response event session = %q, want %q", event.FactorySessionId, sessionID)
		}
	}
	payload, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("marshal CC-04 response events: %v", err)
	}
	if !strings.Contains(string(payload), "stream_canceled") {
		t.Fatalf("CC-04 response events = %s, want stream_canceled diagnostic", payload)
	}
}

func assertConcurrencyResponseStreamIdentity(
	t *testing.T,
	events []factoryapi.FactoryResponseEvent,
	sessionID string,
	dispatchID string,
) string {
	t.Helper()
	if len(events) == 0 {
		t.Fatalf("%s response stream is empty, want public run identity", sessionID)
	}
	runID := ""
	var previousSequence int64
	for _, event := range events {
		if event.FactorySessionId != sessionID || strings.TrimSpace(event.EventId) == "" || strings.TrimSpace(event.RunId) == "" {
			t.Fatalf("%s response event = %#v, want session/event/run identity", sessionID, event)
		}
		if runID == "" {
			runID = event.RunId
		} else if event.RunId != runID {
			t.Fatalf("%s response stream changed run identity from %q to %q", sessionID, runID, event.RunId)
		}
		if event.Sequence <= previousSequence {
			t.Fatalf("%s response stream sequence regressed: previous=%d current=%d event=%#v", sessionID, previousSequence, event.Sequence, event)
		}
		previousSequence = event.Sequence
		if event.DispatchId != nil && *event.DispatchId != dispatchID {
			t.Fatalf("%s response stream dispatch = %q, want %q", sessionID, *event.DispatchId, dispatchID)
		}
	}
	return runID
}

func concurrencyResponseEventIDs(events []factoryapi.FactoryResponseEvent) map[string]struct{} {
	ids := make(map[string]struct{}, len(events))
	for _, event := range events {
		if event.EventId != "" {
			ids[event.EventId] = struct{}{}
		}
	}
	return ids
}

func assertConcurrencySessionDeleted(t testing.TB, baseURL, sessionID string) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID)
	response, err := http.Get(endpoint)
	if err != nil {
		t.Fatalf("GET deleted Factory Session %q: %v", sessionID, err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, response.Body)
	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("GET deleted Factory Session %q status = %d, want 404", sessionID, response.StatusCode)
	}
}

func stringPointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

func stringPointer(value string) *string { return &value }

func TestConcurrencyDispatchStartOrderRejectsDescendingTimestamps(t *testing.T) {
	t.Parallel()
	first := time.Date(2026, time.August, 29, 12, 0, 2, 0, time.UTC)
	second := first.Add(-time.Nanosecond)
	if err := concurrencyDispatchStartOrderError([]support.DispatchEventObservation{
		{DispatchID: "dispatch-first", StartedAt: first},
		{DispatchID: "dispatch-second", StartedAt: second},
	}); err == nil {
		t.Fatal("descending dispatch start timestamps returned nil error, want regression failure")
	}
}
