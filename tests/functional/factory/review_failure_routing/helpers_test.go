package review_failure_routing

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const reviewFailureEventTimeout = 30 * time.Second

type reviewFailureScenario struct {
	fixture    *reviewFailureProcessFixture
	rootDir    string
	factoryDir string
	marker     string
	sessionID  string
}

type reviewFailureSeed struct {
	Name      string
	WorkID    string
	WorkType  string
	State     string
	TraceID   string
	Payload   string
	StateType factoryapi.WorkStateType
}

func openReviewFailureScenario(
	t *testing.T,
	config reviewFailureRouteConfig,
) *reviewFailureScenario {
	t.Helper()
	fixture := sharedReviewFailureFixture(t)
	marker := fixture.nextScenarioID()
	config.marker = marker

	rootDir, err := os.MkdirTemp(filepath.Join(fixture.rootDir, "scenarios"), marker+"-")
	if err != nil {
		t.Fatalf("create review-failure scenario root: %v", err)
	}
	factoryDir := filepath.Join(rootDir, "factory")
	if err := os.CopyFS(factoryDir, os.DirFS(fixture.sourceFactory)); err != nil {
		t.Fatalf("copy authored Factory for review-failure scenario: %v", err)
	}
	if err := fixture.router.register(factoryDir, config); err != nil {
		_ = os.RemoveAll(rootDir)
		t.Fatalf("register review-failure route: %v", err)
	}

	opened := support.OpenFactorySessionAt(t, fixture.baseURL, factoryDir)
	if opened.Session == nil || opened.Session.Id == "" {
		fixture.router.unregister(factoryDir)
		_ = os.RemoveAll(rootDir)
		t.Fatal("opened review-failure Factory Session has no ID")
	}
	if opened.Session.Id == factorysessions.DefaultSessionID {
		fixture.router.unregister(factoryDir)
		_ = os.RemoveAll(rootDir)
		t.Fatalf("opened review-failure Factory Session = %q, want explicit session", opened.Session.Id)
	}
	sessionID := opened.Session.Id
	if err := fixture.sessionOpened(sessionID); err != nil {
		support.CloseFactorySessionAt(t, fixture.baseURL, sessionID)
		fixture.router.unregister(factoryDir)
		_ = os.RemoveAll(rootDir)
		t.Fatal(err)
	}
	scenario := &reviewFailureScenario{
		fixture:    fixture,
		rootDir:    rootDir,
		factoryDir: factoryDir,
		marker:     marker,
		sessionID:  sessionID,
	}
	t.Cleanup(func() { scenario.close(t) })
	return scenario
}

func (scenario *reviewFailureScenario) close(t testing.TB) {
	if scenario == nil {
		return
	}
	support.CloseFactorySessionAt(t, scenario.fixture.baseURL, scenario.sessionID)
	scenario.fixture.router.unregister(scenario.factoryDir)
	scenario.fixture.sessionClosed(scenario.sessionID)
	if err := os.RemoveAll(scenario.rootDir); err != nil {
		t.Errorf("remove review-failure scenario root: %v", err)
	}
	if _, err := os.Stat(scenario.rootDir); !os.IsNotExist(err) {
		t.Errorf("review-failure scenario root remains after cleanup: %v", err)
	}
}

func (scenario *reviewFailureScenario) submit(t *testing.T, requestID string, seeds ...reviewFailureSeed) {
	t.Helper()
	works := make([]factoryapi.Work, 0, len(seeds))
	for _, seed := range seeds {
		if seed.StateType == "" {
			seed.StateType = reviewFailureStateType(seed.State)
		}
		state := &factoryapi.WorkState{Name: seed.State, Type: seed.StateType}
		workID := seed.WorkID
		traceID := seed.TraceID
		workType := seed.WorkType
		works = append(works, factoryapi.Work{
			Name:                   seed.Name,
			WorkId:                 &workID,
			WorkTypeName:           &workType,
			State:                  state,
			CurrentChainingTraceId: &traceID,
			TraceId:                &traceID,
			Payload:                seed.Payload,
		})
	}
	request := factoryapi.WorkRequest{
		RequestId: requestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works:     &works,
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal review-failure Work request: %v", err)
	}
	endpoint := support.SessionWorkURL(
		scenario.fixture.baseURL,
		scenario.sessionID,
		"/work-requests/"+url.PathEscape(requestID),
	)
	httpRequest, err := http.NewRequest(http.MethodPut, endpoint, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build review-failure Work request: %v", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(httpRequest)
	if err != nil {
		t.Fatalf("PUT review-failure Work request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("PUT review-failure Work request status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.UpsertWorkRequestResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode review-failure Work request response: %v", err)
	}
	if result.RequestId != requestID || len(result.Works) != len(seeds) {
		t.Fatalf("review-failure Work request result = %#v, want request %q and %d works", result, requestID, len(seeds))
	}
}

func (scenario *reviewFailureScenario) listWorks(t *testing.T) []factoryapi.Work {
	t.Helper()
	response := support.GetJSON[factoryapi.ListWorkResponse](
		t,
		support.SessionWorkURL(scenario.fixture.baseURL, scenario.sessionID, "/work?includeSuperseded=true"),
	)
	return response.Results
}

func (scenario *reviewFailureScenario) eventStream(t *testing.T) *support.FactoryEventStream {
	t.Helper()
	stream := support.OpenFactoryEventStreamAt(
		t,
		support.SessionEventsURL(scenario.fixture.baseURL, scenario.sessionID),
	)
	awaitReviewFailureEvents(t, stream, func(event factoryapi.FactoryEvent) bool {
		return event.Type == factoryapi.FactoryEventTypeFactoryStateResponse
	})
	return stream
}

func awaitReviewFailureEvents(
	t *testing.T,
	stream *support.FactoryEventStream,
	predicate func(factoryapi.FactoryEvent) bool,
) factoryapi.FactoryEvent {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), reviewFailureEventTimeout)
	defer cancel()
	for {
		event := stream.NextEventContext(ctx)
		if predicate(event) {
			return event
		}
	}
}

func awaitReviewFailureDispatchResponses(
	t *testing.T,
	stream *support.FactoryEventStream,
	transition string,
	count int,
) []factoryapi.FactoryEvent {
	t.Helper()
	responses := make([]factoryapi.FactoryEvent, 0, count)
	ctx, cancel := context.WithTimeout(context.Background(), reviewFailureEventTimeout)
	defer cancel()
	for len(responses) < count {
		event := stream.NextEventContext(ctx)
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode review-failure DISPATCH_RESPONSE %q: %v", event.Id, err)
		}
		if payload.TransitionId == transition {
			responses = append(responses, event)
		}
	}
	return responses
}

func decodeReviewFailureDispatchResponse(t *testing.T, event factoryapi.FactoryEvent) factoryapi.DispatchResponseEventPayload {
	t.Helper()
	payload, err := event.Payload.AsDispatchResponseEventPayload()
	if err != nil {
		t.Fatalf("decode review-failure dispatch response %q: %v", event.Id, err)
	}
	return payload
}

func assertReviewFailureDispatchOutputStates(
	t *testing.T,
	payload factoryapi.DispatchResponseEventPayload,
	want map[string]string,
) {
	t.Helper()
	if payload.OutputWork == nil {
		t.Fatalf("dispatch outputWork = nil, want states for %v", want)
	}
	byID := make(map[string]factoryapi.Work, len(*payload.OutputWork))
	for _, work := range *payload.OutputWork {
		if work.WorkId != nil {
			byID[*work.WorkId] = work
		}
	}
	for workID, wantState := range want {
		work, ok := byID[workID]
		if !ok {
			t.Fatalf("dispatch outputWork is missing Work ID %q; outputWork = %#v", workID, *payload.OutputWork)
		}
		if work.State == nil || work.State.Name != wantState {
			t.Fatalf("dispatch outputWork %q state = %#v, want %q", workID, work.State, wantState)
		}
	}
}

func reviewFailureStateType(state string) factoryapi.WorkStateType {
	switch state {
	case "init":
		return factoryapi.WorkStateTypeINITIAL
	case "complete":
		return factoryapi.WorkStateTypeTERMINAL
	case "failed", "fin":
		return factoryapi.WorkStateTypeFAILED
	default:
		return factoryapi.WorkStateTypePROCESSING
	}
}

func assertReviewFailureWorkStates(
	t *testing.T,
	works []factoryapi.Work,
	want map[string]string,
) {
	t.Helper()
	byID := make(map[string]factoryapi.Work, len(works))
	for _, work := range works {
		if work.WorkId != nil {
			byID[*work.WorkId] = work
		}
	}
	for workID, wantState := range want {
		work, ok := byID[workID]
		if !ok {
			t.Fatalf("public Work list is missing Work ID %q; works = %#v", workID, works)
		}
		if work.State == nil || work.State.Name != wantState {
			t.Fatalf("public Work %q state = %#v, want %q", workID, work.State, wantState)
		}
	}
}

func reviewFailureDispatches(t *testing.T, scenario *reviewFailureScenario) []support.DispatchEventObservation {
	t.Helper()
	return support.ObserveDispatchEvents(
		t,
		support.GetFactoryEventsForSessionAt(t, scenario.fixture.baseURL, scenario.sessionID),
	)
}

func dispatchInputIDs(observation support.DispatchEventObservation) []string {
	ids := make([]string, 0, len(observation.Request.Inputs))
	for _, input := range observation.Request.Inputs {
		ids = append(ids, input.WorkId)
	}
	return ids
}

func assertExactReviewFailureInputIDs(t *testing.T, observation support.DispatchEventObservation, want ...string) {
	t.Helper()
	got := dispatchInputIDs(observation)
	if len(got) != len(want) {
		t.Fatalf("dispatch %q input IDs = %v, want %v", observation.DispatchID, got, want)
	}
	wantCounts := make(map[string]int, len(want))
	for _, workID := range want {
		wantCounts[workID]++
	}
	for _, workID := range got {
		if wantCounts[workID] == 0 {
			t.Fatalf("dispatch %q input IDs = %v, want exact IDs %v", observation.DispatchID, got, want)
		}
		wantCounts[workID]--
	}
	for workID, count := range wantCounts {
		if count != 0 {
			t.Fatalf("dispatch %q input IDs = %v, want exact IDs %v (missing %q)", observation.DispatchID, got, want, workID)
		}
	}
}

func providerCommandPrompt(request platformprocess.CommandRequest) string {
	if len(request.Stdin) > 0 {
		return string(request.Stdin)
	}
	return strings.Join(request.Args, " ")
}

func reviewFailureAccepted(output string) platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(fmt.Sprintf(
		`{"decision":"ACCEPTED","output":%q}`,
		output,
	))}
}

func reviewFailureRejected(feedback string) platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(fmt.Sprintf(
		`{"decision":"REJECTED","feedback":%q}`,
		feedback,
	))}
}
