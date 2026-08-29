package agent_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func listAgentSessionWork(t testing.TB, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work"
	return support.GetJSON[factoryapi.ListWorkResponse](t, endpoint)
}

func agentTextContent(t *testing.T, text string) factoryapi.WorkContent {
	t.Helper()
	part := factoryapi.WorkContentPart{}
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Text: text,
		Type: factoryapi.WorkContentPartTypeText,
	}); err != nil {
		t.Fatalf("encode text Work content: %v", err)
	}
	return factoryapi.WorkContent{part}
}

func assertAgentWork(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	workID string,
	wantOutput string,
) {
	t.Helper()
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 1 {
		t.Fatalf("completed Work = %d, want one; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("failed Work = %d, want zero; listed=%#v", got, listed)
	}
	var found int
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) != workID {
			continue
		}
		found++
		if item.Content == nil || len(*item.Content) != 1 {
			t.Fatalf("Work %q content = %#v, want one text part", workID, item.Content)
		}
		textPart, err := (*item.Content)[0].AsWorkTextContentPart()
		if err != nil {
			t.Fatalf("decode Work %q text content: %v", workID, err)
		}
		if !strings.Contains(textPart.Text, wantOutput) {
			t.Fatalf("Work %q text = %q, want content %q", workID, textPart.Text, wantOutput)
		}
	}
	if found != 1 {
		t.Fatalf("Work identity count = %d, want exactly one %q; listed=%#v", found, workID, listed)
	}
}

func assertAgentScenarioWork(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	workID string,
	scenario agentSharedScenario,
) {
	t.Helper()
	if scenario.behavior == agentSharedCancel {
		if len(listed.Results) != 1 {
			t.Fatalf("%s cancellation Work results = %#v, want one processing Work", scenario.name, listed.Results)
		}
		item := listed.Results[0]
		if support.StringPointerValue(item.WorkId) != workID || item.State == nil || item.State.Name != "init" || item.State.Type != factoryapi.WorkStateTypePROCESSING {
			t.Fatalf("%s cancellation Work = %#v, want Work %q in init/PROCESSING", scenario.name, item, workID)
		}
		if item.FailureDetail != nil {
			t.Fatalf("%s cancellation Work failure detail = %#v, want none", scenario.name, item.FailureDetail)
		}
		return
	}
	if scenario.wantOutcome == factoryapi.WorkOutcomeAccepted {
		assertAgentWork(t, listed, workID, scenario.output)
		return
	}
	if got := support.CountWorkAtCustomerState(listed, "task:done"); got != 0 {
		t.Fatalf("%s completed Work = %d, want zero; listed=%#v", scenario.name, got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 1 {
		t.Fatalf("%s failed Work = %d, want one; listed=%#v", scenario.name, got, listed)
	}
	var found int
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) != workID {
			continue
		}
		found++
		if item.FailureDetail == nil {
			t.Fatalf("%s Work %q has no failure detail", scenario.name, workID)
		}
		if scenario.wantFailure != "" && item.FailureDetail.Reason != scenario.wantFailure {
			t.Fatalf("%s Work failure reason = %q, want %q", scenario.name, item.FailureDetail.Reason, scenario.wantFailure)
		}
		if scenario.wantMessage != "" && !strings.Contains(item.FailureDetail.Message, scenario.wantMessage) {
			t.Fatalf("%s Work failure message = %q, want %q", scenario.name, item.FailureDetail.Message, scenario.wantMessage)
		}
	}
	if found != 1 {
		t.Fatalf("%s Work identity count = %d, want exactly one %q; listed=%#v", scenario.name, found, workID, listed)
	}
}

func assertAgentEmptyScenarioWork(
	t *testing.T,
	baseURL string,
	sessionID string,
	listed factoryapi.ListWorkResponse,
	emptyWorkID string,
	validWorkID string,
	wantOutput string,
) {
	t.Helper()
	if emptyWorkID == "" || validWorkID == "" || emptyWorkID == validWorkID {
		t.Fatalf("empty/valid Work identities = %q/%q, want distinct non-empty identities", emptyWorkID, validWorkID)
	}
	if len(listed.Results) == 0 {
		t.Fatalf("empty characterization Work list = %#v, want at least one visible result", listed)
	}
	emptyWork := getAgentSessionWorkByID(t, baseURL, sessionID, emptyWorkID)
	validWork := getAgentSessionWorkByID(t, baseURL, sessionID, validWorkID)
	assertAgentCompletedWork(t, emptyWork, emptyWorkID, "")
	assertAgentCompletedWork(t, validWork, validWorkID, wantOutput)
	if got := support.CountWorkAtCustomerState(listed, "task:failed"); got != 0 {
		t.Fatalf("empty characterization visible failed Work = %d, want zero; listed=%#v", got, listed)
	}
	if support.StringPointerValue(emptyWork.WorkId) == support.StringPointerValue(validWork.WorkId) {
		t.Fatalf("empty and valid Work detail identities = %q, want distinct", support.StringPointerValue(emptyWork.WorkId))
	}
}

func getAgentSessionWorkByID(
	t testing.TB,
	baseURL string,
	sessionID string,
	workID string,
) factoryapi.Work {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/work/" + url.PathEscape(workID)
	return support.GetJSON[factoryapi.Work](t, endpoint)
}

func assertAgentCompletedWork(t testing.TB, item factoryapi.Work, workID, wantOutput string) {
	t.Helper()
	if support.StringPointerValue(item.WorkId) != workID || item.State == nil || item.State.Name != "done" {
		t.Fatalf("Work detail = %#v, want Work %q in done state", item, workID)
	}
	if item.FailureDetail != nil {
		t.Fatalf("Work %q failure detail = %#v, want none", workID, item.FailureDetail)
	}
	if wantOutput == "" {
		return
	}
	if item.Content == nil || len(*item.Content) != 1 {
		t.Fatalf("Work %q content = %#v, want one text part", workID, item.Content)
	}
	textPart, err := (*item.Content)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode Work %q text content: %v", workID, err)
	}
	if !strings.Contains(textPart.Text, wantOutput) {
		t.Fatalf("Work %q text = %q, want content %q", workID, textPart.Text, wantOutput)
	}
}

func assertAgentEmptyScenarioDispatch(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	sessionID string,
	emptyRequestID string,
	emptyWorkID string,
	validRequestID string,
	validWorkID string,
	wantOutput string,
) {
	t.Helper()
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 2 {
		t.Fatalf("empty characterization dispatch observations = %#v, want one per accepted Work", dispatches)
	}
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("empty characterization Factory Event %q escaped Factory Session %q", event.Id, sessionID)
		}
	}
	matched := map[string]bool{}
	for _, dispatch := range dispatches {
		if dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf("empty characterization dispatch = %#v, want accepted response", dispatch)
		}
		var workID, requestID string
		switch {
		case support.DispatchObservationIncludesWork(dispatch, emptyWorkID):
			workID, requestID = emptyWorkID, emptyRequestID
		case support.DispatchObservationIncludesWork(dispatch, validWorkID):
			workID, requestID = validWorkID, validRequestID
		default:
			t.Fatalf("empty characterization dispatch = %#v, want Work %q or %q", dispatch, emptyWorkID, validWorkID)
		}
		if dispatch.DispatchID == "" || dispatch.Request.TransitionId != "process" {
			t.Fatalf("empty characterization dispatch = %#v, want process identity for Work %q", dispatch, workID)
		}
		if matched[workID] {
			t.Fatalf("empty characterization has duplicate dispatch for Work %q", workID)
		}
		matched[workID] = true
		if workID == validWorkID && !strings.Contains(support.StringPointerValue(dispatch.Response.Output), wantOutput) {
			t.Fatalf("valid follow-up dispatch output = %q, want content %q", support.StringPointerValue(dispatch.Response.Output), wantOutput)
		}
		correlated := false
		for _, event := range events {
			if event.Context.RequestId != nil && *event.Context.RequestId == requestID {
				correlated = true
			}
		}
		if !correlated {
			t.Fatalf("empty characterization events contain no request correlation for %q", requestID)
		}
	}
	if !matched[emptyWorkID] || !matched[validWorkID] {
		t.Fatalf("empty characterization dispatch Work matches = %#v, want %q and %q", matched, emptyWorkID, validWorkID)
	}
}

func assertAgentDispatch(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	sessionID string,
	requestID string,
	workID string,
	wantOutput string,
) {
	t.Helper()
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != 1 {
		t.Fatalf("agent dispatch observations = %#v, want one", dispatches)
	}
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("Factory Event %q escaped Factory Session %q", event.Id, sessionID)
		}
	}
	dispatch := dispatches[0]
	if dispatch.DispatchID == "" || !support.DispatchObservationIncludesWork(dispatch, workID) {
		t.Fatalf("agent dispatch = %#v, want non-empty identity correlated to Work %q", dispatch, workID)
	}
	if dispatch.Request.TransitionId != "process" || dispatch.Response == nil {
		t.Fatalf("agent dispatch = %#v, want process request and response", dispatch)
	}
	if dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
		t.Fatalf("agent dispatch outcome = %s, want ACCEPTED", dispatch.Response.Outcome)
	}
	if got := support.StringPointerValue(dispatch.Response.Output); !strings.Contains(got, wantOutput) {
		t.Fatalf("agent dispatch output = %q, want content %q", got, wantOutput)
	}
	correlated := false
	for _, event := range events {
		if event.Context.RequestId != nil && *event.Context.RequestId == requestID {
			correlated = true
		}
	}
	if !correlated {
		t.Fatalf("Factory Events contain no request correlation for %q", requestID)
	}
}

func assertAgentScenarioDispatch(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	sessionID string,
	requestID string,
	workID string,
	scenario agentSharedScenario,
) {
	t.Helper()
	if scenario.behavior == agentSharedCancel {
		dispatches := support.ObserveDispatchEvents(t, events)
		if len(dispatches) != 1 {
			t.Fatalf("%s cancellation dispatch observations = %#v, want one in-flight dispatch", scenario.name, dispatches)
		}
		dispatch := dispatches[0]
		if dispatch.DispatchID == "" || !support.DispatchObservationIncludesWork(dispatch, workID) || dispatch.Request.TransitionId != "process" {
			t.Fatalf("%s cancellation dispatch = %#v, want process dispatch correlated to Work %q", scenario.name, dispatch, workID)
		}
		if dispatch.Response != nil {
			t.Fatalf("%s cancellation dispatch response = %#v, want no business response after session cancel", scenario.name, dispatch.Response)
		}
		correlated := false
		for _, event := range events {
			if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
				t.Fatalf("%s Factory Event %q escaped Factory Session %q", scenario.name, event.Id, sessionID)
			}
			if event.Context.RequestId != nil && *event.Context.RequestId == requestID {
				correlated = true
			}
		}
		if !correlated {
			t.Fatalf("%s Factory Events contain no request correlation for %q", scenario.name, requestID)
		}
		return
	}
	if scenario.wantOutcome == factoryapi.WorkOutcomeAccepted {
		assertAgentDispatch(t, events, sessionID, requestID, workID, scenario.output)
		return
	}
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) != scenario.wantDispatches {
		t.Fatalf("%s dispatch observations = %#v, want %d", scenario.name, dispatches, scenario.wantDispatches)
	}
	for _, event := range events {
		if event.Context.SessionId != nil && *event.Context.SessionId != sessionID {
			t.Fatalf("%s Factory Event %q escaped Factory Session %q", scenario.name, event.Id, sessionID)
		}
	}
	for index, dispatch := range dispatches {
		if dispatch.DispatchID == "" || !support.DispatchObservationIncludesWork(dispatch, workID) {
			t.Fatalf("%s dispatch[%d] = %#v, want non-empty identity correlated to Work %q", scenario.name, index, dispatch, workID)
		}
		if dispatch.Request.TransitionId != "process" || dispatch.Response == nil {
			t.Fatalf("%s dispatch[%d] = %#v, want process request and response", scenario.name, index, dispatch)
		}
		if dispatch.Response.Outcome != scenario.wantOutcome {
			t.Fatalf("%s dispatch[%d] outcome = %q, want %q", scenario.name, index, dispatch.Response.Outcome, scenario.wantOutcome)
		}
		if dispatch.Response.FailureDetail == nil {
			t.Fatalf("%s dispatch[%d] response has no failure detail", scenario.name, index)
		}
		if scenario.wantFailure != "" && dispatch.Response.FailureDetail.Reason != scenario.wantFailure {
			t.Fatalf("%s dispatch[%d] failure reason = %q, want %q", scenario.name, index, dispatch.Response.FailureDetail.Reason, scenario.wantFailure)
		}
		if scenario.wantMessage != "" && !strings.Contains(dispatch.Response.FailureDetail.Message, scenario.wantMessage) {
			t.Fatalf("%s dispatch[%d] failure message = %q, want %q", scenario.name, index, dispatch.Response.FailureDetail.Message, scenario.wantMessage)
		}
	}
	correlated := false
	for _, event := range events {
		if event.Context.RequestId != nil && *event.Context.RequestId == requestID {
			correlated = true
		}
	}
	if !correlated {
		t.Fatalf("%s Factory Events contain no request correlation for %q", scenario.name, requestID)
	}
}

func assertAgentWorkerSession(
	t *testing.T,
	baseURL string,
	sessionID string,
	workID string,
	scenario agentSharedScenario,
) {
	t.Helper()
	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + url.PathEscape(sessionID) + "/worker-sessions?workId=" + url.QueryEscape(workID)
	listed := support.GetJSON[factoryapi.ListWorkerSessionsResponse](t, endpoint)
	if scenario.behavior == agentSharedCancel {
		if len(listed.Sessions) != 1 {
			t.Fatalf("%s cancellation Worker Sessions = %#v, want one", scenario.name, listed.Sessions)
		}
		workerSession := listed.Sessions[0]
		if strings.TrimSpace(workerSession.WorkerSessionId) == "" || workerSession.WorkId == nil || *workerSession.WorkId != workID {
			t.Fatalf("%s cancellation Worker Session = %#v, want Work correlation %q", scenario.name, workerSession, workID)
		}
		return
	}
	if len(listed.Sessions) != scenario.wantDispatches {
		t.Fatalf("%s Worker Sessions = %#v, want %d for Work %q", scenario.name, listed.Sessions, scenario.wantDispatches, workID)
	}
	wantState := factoryapi.WorkerSessionObservationStateCompleted
	if scenario.wantOutcome != factoryapi.WorkOutcomeAccepted {
		wantState = factoryapi.WorkerSessionObservationStateFailed
	}
	for index, workerSession := range listed.Sessions {
		if strings.TrimSpace(workerSession.WorkerSessionId) == "" || workerSession.WorkId == nil || *workerSession.WorkId != workID {
			t.Fatalf("%s Worker Session[%d] = %#v, want Work correlation %q", scenario.name, index, workerSession, workID)
		}
		if workerSession.State != wantState {
			t.Fatalf("%s Worker Session[%d] state = %q, want %q", scenario.name, index, workerSession.State, wantState)
		}
	}
}

func assertAgentSessionDeleted(t *testing.T, baseURL, sessionID string) {
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

func agentCommandRequestContains(request platformprocess.CommandRequest, marker string) bool {
	if strings.Contains(string(request.Stdin), marker) {
		return true
	}
	for _, arg := range request.Args {
		if strings.Contains(arg, marker) {
			return true
		}
	}
	return false
}

func containsAgentArgumentPair(args []string, name, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name && args[index+1] == value {
			return true
		}
	}
	return false
}

func cloneAgentCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}

var _ platformprocess.CommandRunner = (*agentSharedCommandRouter)(nil)
