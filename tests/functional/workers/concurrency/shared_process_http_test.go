package root_composition_test

import (
	"bytes"
	"context"
	"encoding/json"
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

func assertDistinctConcurrencyDispatches(t *testing.T, first, second *concurrencySession) {
	t.Helper()
	firstDispatches := support.ObserveDispatchEvents(t, concurrencySessionEvents(t, first.fixture.baseURL, first.id))
	secondDispatches := support.ObserveDispatchEvents(t, concurrencySessionEvents(t, second.fixture.baseURL, second.id))
	if len(firstDispatches) != 1 || len(secondDispatches) != 1 || firstDispatches[0].DispatchID == secondDispatches[0].DispatchID {
		t.Fatalf("CC-03 dispatch identities = %#v/%#v, want distinct one-per-session dispatches", firstDispatches, secondDispatches)
	}
	for _, event := range concurrencySessionEvents(t, first.fixture.baseURL, first.id) {
		if event.Context.SessionId != nil && *event.Context.SessionId != first.id {
			t.Fatalf("CC-03 first event escaped session: %#v", event.Context)
		}
	}
	for _, event := range concurrencySessionEvents(t, second.fixture.baseURL, second.id) {
		if event.Context.SessionId != nil && *event.Context.SessionId != second.id {
			t.Fatalf("CC-03 second event escaped session: %#v", event.Context)
		}
	}
}

func assertConcurrencySessionOrdering(t *testing.T, session *concurrencySession) {
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
	if !dispatches[0].StartedAt.Before(dispatches[1].StartedAt) && dispatches[0].StartedAt.Equal(dispatches[1].StartedAt) {
		t.Fatalf("%s dispatch start timestamps are not ordered: %#v", session.name, dispatches)
	}
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
