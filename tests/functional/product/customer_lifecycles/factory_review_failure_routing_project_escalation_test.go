package customer_lifecycles_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Each cell uses the authored routes in an isolated session on the shared root
// process. Work/Events API reads assert the same public contract as CLI ingress.
type escalationCase struct {
	id, decision, state    string
	commandError, untagged bool
}

func testProjectEscalationTagLineage(t *testing.T) {
	t.Parallel()
	cases := []escalationCase{
		{id: "ESC-F1", decision: "FAILED", state: "failed"},
		{id: "ESC-F2", decision: "ACCEPTED", state: "complete"},
		{id: "ESC-F3", state: "failed", commandError: true},
		{id: "ESC-F4", decision: "ACCEPTED", state: "complete", untagged: true},
	}
	for _, tc := range cases {
		t.Run(tc.id, func(t *testing.T) {
			t.Parallel()
			runProjectEscalation(t, tc)
		})
	}
}

func runProjectEscalation(t *testing.T, tc escalationCase) {
	release := make(chan struct{})
	prompt := make(chan string, 1)
	var releaseOnce sync.Once
	releaseProvider := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(releaseProvider)
	scenario := openReviewFailureScenario(t, reviewFailureRouteConfig{provider: escalationProvider(tc, prompt, release)})
	wake := &projectWakeScenario{reviewFailureScenario: scenario,
		ownerName: scenario.marker + "-zulu", ownerID: scenario.marker + "-zulu-project",
		peerName: scenario.marker + "-alpha", peerID: scenario.marker + "-alpha-project"}
	// Admit the earlier-sorting peer through the same customer CLI.
	wake.escalationCLI(t, "submit", "batch", "--session", wake.sessionID, escalationBatch(t, wake.marker+"-peer-request", map[string]any{
		"name": wake.peerName, "workId": wake.peerID, "workTypeName": "project", "state": "waiting",
		"tags": map[string]string{"project": wake.peerName}, "payload": escalationProjectPayload(t, wake, wake.peerName),
	}))
	stream := wake.eventStream(t)
	tags := map[string]string{"project": wake.ownerName, "scope": wake.marker + "-secondary"}
	if tc.untagged {
		delete(tags, "project")
	}
	payload := escalationProjectPayload(t, wake, wake.ownerName)
	// Structured Project payloads have no legacy text content projection.
	var content any
	wake.escalationCLI(t, "submit", "batch", "--session", wake.sessionID, escalationBatch(t, wake.marker+"-owner-request", map[string]any{
		"name": wake.ownerName, "workId": wake.ownerID, "workTypeName": "project",
		"state":   "needs-supervision",
		"payload": payload, "tags": tags,
	}))
	escalation := decodeReviewFailureDispatchResponse(t, awaitReviewFailureDispatchResponses(t, stream, "escalate-project", 1)[0])
	thoughtsID := assertEscalationOutput(t, escalation, wake.ownerID, tags)
	// The escalation event captures creation before provider output;
	// the Work read independently confirms the durable parent identity.
	thoughts := support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(wake.fixture.baseURL, wake.sessionID, "/work/"+thoughtsID))
	assertEscalationOrigin(t, thoughts, wake.ownerID)
	parent := support.GetJSON[factoryapi.Work](t, support.SessionWorkURL(wake.fixture.baseURL, wake.sessionID, "/work/"+wake.ownerID))
	// Ordinary escalation creates thoughts with the Project's display name.
	// That non-Project peer must not make the verified Project target ambiguous.
	if thoughts.Name != parent.Name || thoughts.WorkTypeName == nil || *thoughts.WorkTypeName != "thoughts" || parent.WorkTypeName == nil || *parent.WorkTypeName != "project" {
		t.Fatalf("same-name Project/thoughts boundary lost: Project=%+v thoughts=%+v", parent, thoughts)
	}
	assertEscalationJSONEqual(t, "Project payload", parent.Payload, payload)
	payloadJSON, _ := json.Marshal(payload)
	ctx, cancel := context.WithTimeout(t.Context(), reviewFailureEventTimeout)
	defer cancel()
	select {
	case rendered := <-prompt:
		if !strings.Contains(rendered, string(payloadJSON)) {
			t.Fatalf("supervision prompt lost originating payload: %s", rendered)
		}
	case <-ctx.Done():
		t.Fatal("supervision prompt not observed")
	}
	assertEscalationJSONEqual(t, "thoughts content", thoughts.Content, content)
	assertEscalationJSONEqual(t, "Project content", parent.Content, content)
	assertReviewFailureWorkStates(t, wake.listWorks(t), map[string]string{wake.ownerID: "blocked", wake.peerID: "waiting"})
	releaseProvider()
	route := "report-thoughts-" + tc.state
	if tc.state == "failed" {
		route = "report-thoughts-failure"
	}
	awaitReviewFailureDispatchResponses(t, stream, route, 1)
	awaitReviewFailureQuiescence(t, wake.reviewFailureScenario)
	assertEscalationPendingReport(t, wake, thoughtsID, tags, tc.state)
	assertEscalationDispatchLineage(t, wake, tags, tc.commandError)
	wake.escalationCLI(t, "work", "move", "--session", wake.sessionID, "--request-id", wake.marker+"-resume", wake.ownerID, "waiting")
	if !tc.untagged {
		assertProjectLeadWakes(t, wake, stream, wake.ownerName)
	}
	awaitReviewFailureQuiescence(t, wake.reviewFailureScenario)
	assertEscalationFinalRouting(t, wake, thoughtsID, tags, tc.untagged)
}

func escalationProvider(tc escalationCase, prompt chan<- string, release <-chan struct{}) reviewFailureCommandResponder {
	return func(ctx context.Context, request platformprocess.CommandRequest, _ int) (platformprocess.CommandResult, error) {
		if strings.Contains(providerCommandPrompt(request), "Project Lead wake") {
			return reviewFailureAccepted("owner reconciled"), nil
		}
		prompt <- providerCommandPrompt(request)
		select {
		case <-release:
		case <-ctx.Done():
			return platformprocess.CommandResult{}, ctx.Err()
		}
		if tc.commandError {
			return platformprocess.CommandResult{}, errors.New("controlled escalation supervision command failure")
		}
		body, _ := json.Marshal(map[string]string{"decision": tc.decision, "feedback": "supervision result", "output": "hold"})
		return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(string(body))}, nil
	}
}

func escalationProjectPayload(t *testing.T, wake *projectWakeScenario, projectName string) map[string]any {
	t.Helper()
	projectRoot := filepath.Join(wake.rootDir, "docs", "temp", "projects", projectName)
	if err := os.MkdirAll(projectRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	sourcePlan := filepath.Join(projectRoot, "source-plan.md")
	if err := os.WriteFile(sourcePlan, []byte("Authorized isolated tag-lineage fixture."), 0o600); err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"projectRoot": projectRoot, "sourcePlan": sourcePlan, "contractRevision": wake.marker + "-v1",
		"request": wake.marker + "-payload-sentinel", "acceptance": []any{"Escalation preserves origin tags."},
	}
}

func escalationBatch(t *testing.T, requestID string, item map[string]any) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"requestId": requestID, "type": "FACTORY_REQUEST_BATCH", "works": []any{item}})
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func (wake *projectWakeScenario) escalationCLI(t *testing.T, args ...string) {
	t.Helper()
	input := support.FakeInputs(t.Context(), append([]string{"you", "--server", wake.fixture.baseURL, "--json"}, args...))
	input.Input.WorkingDirectory = wake.factoryDir
	input.Input.Env = isolatedHomeEnvironment(filepath.Join(wake.rootDir, "home"))
	if err := wake.fixture.process.Execute(input.Input); err != nil {
		t.Fatalf("session CLI %v: %v stdout=%s stderr=%s", args[:2], err, input.Stdout(), input.Stderr())
	}
}

func assertEscalationOutput(t *testing.T, response factoryapi.DispatchResponseEventPayload, ownerID string, tags map[string]string) string {
	t.Helper()
	if response.OutputWork == nil || len(*response.OutputWork) != 2 {
		t.Fatalf("escalation output = %#v, want Project and thoughts", response.OutputWork)
	}
	thoughtsID := ""
	for _, item := range *response.OutputWork {
		assertEscalationTags(t, item.Tags, tags)
		if item.WorkTypeName != nil && *item.WorkTypeName == "thoughts" {
			if item.WorkId == nil || item.State == nil || item.State.Name != "init" {
				t.Fatalf("thoughts output = %#v", item)
			}
			thoughtsID = *item.WorkId
		} else if item.WorkId == nil || *item.WorkId != ownerID || item.State == nil || item.State.Name != "blocked" {
			t.Fatalf("Project identity/state changed: %#v", item)
		}
	}
	if thoughtsID == "" {
		t.Fatal("escalation created no thoughts")
	}
	return thoughtsID
}

func assertEscalationTags(t *testing.T, actual *factoryapi.StringMap, expected map[string]string) {
	t.Helper()
	for key, value := range expected {
		if actual == nil || (*actual)[key] != value {
			t.Fatalf("tags=%v, want %s=%s", actual, key, value)
		}
	}
	if _, tagged := expected["project"]; !tagged && actual != nil && (*actual)["project"] != "" {
		t.Fatalf("invented project tag: %v", *actual)
	}
}

func assertEscalationJSONEqual(t *testing.T, label string, actual, expected any) {
	t.Helper()
	actualJSON, _ := json.Marshal(actual)
	expectedJSON, _ := json.Marshal(expected)
	var left, right any
	_ = json.Unmarshal(actualJSON, &left)
	_ = json.Unmarshal(expectedJSON, &right)
	if !reflect.DeepEqual(left, right) {
		t.Fatalf("%s=%s, want %s", label, actualJSON, expectedJSON)
	}
}

func assertEscalationOrigin(t *testing.T, item factoryapi.Work, origin string) {
	t.Helper()
	if got := reportOriginWorkID(item); got != origin {
		t.Fatalf("Work parent=%q, want exact origin %q", got, origin)
	}
}

func assertEscalationPendingReport(t *testing.T, wake *projectWakeScenario, thoughtsID string, tags map[string]string, state string) {
	t.Helper()
	works := wake.listWorks(t)
	assertReviewFailureWorkStates(t, works, map[string]string{wake.ownerID: "blocked", wake.peerID: "waiting", thoughtsID: state})
	reports := 0
	for _, item := range works {
		assertEscalationTags(t, item.Tags, func() map[string]string {
			if item.WorkId != nil && *item.WorkId == wake.peerID {
				return map[string]string{"project": wake.peerName}
			}
			return tags
		}())
		if item.WorkTypeName != nil && *item.WorkTypeName == "project-report" {
			reports++
			assertEscalationOrigin(t, item, thoughtsID)
			if item.State == nil || item.State.Name != "pending" {
				t.Fatalf("blocked owner's report was consumed: %#v", item)
			}
		}
	}
	if reports != 1 || len(dispatchesWithTransition(reviewFailureDispatches(t, wake.reviewFailureScenario), projectLeadWakeTransition)) != 0 {
		t.Fatalf("blocked owner: reports=%d, want one pending report and no wake", reports)
	}
}

func assertEscalationDispatchLineage(t *testing.T, wake *projectWakeScenario, tags map[string]string, commandError bool) {
	t.Helper()
	dispatches := reviewFailureDispatches(t, wake.reviewFailureScenario)
	for _, dispatch := range dispatches {
		if dispatch.Response == nil || dispatch.Response.OutputWork == nil {
			continue
		}
		for _, output := range *dispatch.Response.OutputWork {
			assertEscalationTags(t, output.Tags, tags)
		}
	}
	supervision := dispatchesWithTransition(dispatches, "ideafy")
	if len(supervision) != 1 || supervision[0].Response == nil {
		t.Fatalf("supervision dispatches=%#v", supervision)
	}
	if commandError && supervision[0].Response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf("command error outcome=%s, want FAILED", supervision[0].Response.Outcome)
	}
}

func assertEscalationFinalRouting(t *testing.T, wake *projectWakeScenario, thoughtsID string, tags map[string]string, untagged bool) {
	t.Helper()
	wakes := dispatchesWithTransition(reviewFailureDispatches(t, wake.reviewFailureScenario), projectLeadWakeTransition)
	want := 1
	if untagged {
		want = 0
	}
	if len(wakes) != want {
		t.Fatalf("lead wakes=%d, want %d", len(wakes), want)
	}
	for _, dispatch := range wakes {
		assertExactReviewFailureInputIDs(t, dispatch, wake.ownerID, escalationReportID(t, wake, thoughtsID))
	}
	works := wake.listWorks(t)
	assertReviewFailureWorkStates(t, works, map[string]string{wake.ownerID: "waiting", wake.peerID: "waiting"})
	report := workByID(t, works, escalationReportID(t, wake, thoughtsID))
	assertEscalationTags(t, report.Tags, tags)
	wantState := "delivered"
	if untagged {
		wantState = "pending"
	}
	if report.State == nil || report.State.Name != wantState {
		t.Fatalf("report state=%v, want %s", report.State, wantState)
	}
}

func escalationReportID(t *testing.T, wake *projectWakeScenario, thoughtsID string) string {
	t.Helper()
	for _, item := range wake.listWorks(t) {
		if item.WorkTypeName != nil && *item.WorkTypeName == "project-report" && reportOriginWorkID(item) == thoughtsID && item.WorkId != nil {
			return *item.WorkId
		}
	}
	t.Fatal("missing escalation report")
	return ""
}
