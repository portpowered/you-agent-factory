package routing

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const classifierRoutingWorkstation = "classifier"

type classifierRoutingSuccessCase struct {
	name              string
	providerLabels    []string
	reworkResponses   []string
	wantTerminalState string
	wantClassifier    int
	wantRework        int
	wantLabels        []string
	wantFinalPayload  string
}

func classifierRoutingSuccessCases() []classifierRoutingSuccessCase {
	return []classifierRoutingSuccessCase{
		{
			name:              "accepted_completes",
			providerLabels:    []string{"accepted"},
			wantTerminalState: "done",
			wantClassifier:    1,
			wantLabels:        []string{"accepted"},
		},
		{
			name:              "approved_first_try",
			providerLabels:    []string{"approved"},
			wantTerminalState: "done",
			wantClassifier:    1,
			wantLabels:        []string{"approved"},
		},
		{
			name:              "needs_changes_loops_back_then_completes",
			providerLabels:    []string{"needs_changes", "accepted"},
			reworkResponses:   []string{"rework applied COMPLETE"},
			wantTerminalState: "done",
			wantClassifier:    2,
			wantRework:        1,
			wantLabels:        []string{"needs_changes", "accepted"},
			wantFinalPayload:  "rework applied COMPLETE",
		},
		{
			name:              "rejection_path_retries_then_completes",
			providerLabels:    []string{"rejected", "accepted"},
			wantTerminalState: "done",
			wantClassifier:    2,
			wantLabels:        []string{"rejected", "accepted"},
		},
	}
}

// runClassifierRoutesEveryKnownDecision proves that each authored classifier
// label routes Work to its documented public workType:state outcome through the
// customer process boundary, including accepted completion, approve-first
// completion, rework loop-back followed by completion, and rejection-path
// retry followed by completion.
func runClassifierRoutesEveryKnownDecision(
	t *testing.T,
	fixture *workRoutingPackageFixture,
) {
	t.Helper()
	for _, tc := range classifierRoutingSuccessCases() {
		t.Run(tc.name, func(t *testing.T) {
			runClassifierRoutingSuccessCase(t, fixture, tc)
		})
	}
}

func runClassifierRoutingSuccessCase(
	t *testing.T,
	fixture *workRoutingPackageFixture,
	tc classifierRoutingSuccessCase,
) {
	t.Helper()
	workID := "classifier-routing-" + tc.name
	payload := workID + "-payload"
	wantFinalPayload := tc.wantFinalPayload
	if wantFinalPayload == "" {
		wantFinalPayload = payload
	}
	runner := newClassifierRoutingCommandRunner(tc.name, tc.providerLabels, tc.reworkResponses)
	scenario := fixture.newScenario(t, "classifier-success-"+tc.name, "classifier_routing_dir", runner)
	writeLogicalMoveSeedRequest(t, scenario.factoryDir, workID, payload)
	scenario.open(t)

	session, listed, events := scenario.observe(t, 10*time.Second)
	assertClassifierRoutingTerminalState(t, listed, tc.wantTerminalState)
	assertClassifierRoutingSuccessfulSession(t, session, 1)
	assertClassifierRoutingPublicWork(
		t,
		scenario,
		listed,
		events,
		workID,
		payload,
		wantFinalPayload,
	)
	dispatches := support.ObserveDispatchEvents(t, events)
	assertClassifierRoutingWorkstationDispatches(
		t,
		dispatches,
		classifierRoutingWorkstation,
		tc.wantClassifier,
		tc.wantLabels,
	)
	if got := countWorkstationDispatches(dispatches, "rework"); got != tc.wantRework {
		t.Fatalf("rework dispatch count = %d, want %d", got, tc.wantRework)
	}
	assertClassifierRoutingCommandRequests(t, scenario, runner, tc.wantClassifier+tc.wantRework)
}

// runClassifierMultiOutputPreservesPayload proves that when one classifier route
// fans out to multiple authored outputs, every expected branch retains the same
// customer payload on every expected branch at the next observable public Work read
// surface.
func runClassifierMultiOutputPreservesPayload(
	t *testing.T,
	fixture *workRoutingPackageFixture,
) {
	t.Helper()
	const wantPayload = "classifier-multi-output-payload"
	const wantWorkID = "classifier-multi-output-work"
	wantBranches := []string{
		support.WorkCustomerLocation("task", "done"),
		support.WorkCustomerLocation("branch-a", "done"),
		support.WorkCustomerLocation("branch-b", "done"),
	}

	runner := newWorkRoutingScenarioCommandRunner(
		"classifier-multi-output",
		[]platformprocess.CommandResult{
			{Stdout: support.CodexSuccessStdout("fanout")},
			{Stdout: support.CodexSuccessStdout("COMPLETE")},
			{Stdout: support.CodexSuccessStdout("COMPLETE")},
		},
		nil,
	)
	scenario := fixture.newScenario(
		t,
		"classifier-fanout",
		"classifier_multi_output_dir",
		runner,
	)
	writeLogicalMoveSeedRequest(t, scenario.factoryDir, wantWorkID, wantPayload)
	scenario.open(t)

	session, listed, events := scenario.observe(t, 10*time.Second)

	for _, location := range wantBranches {
		if got := support.CountWorkAtCustomerState(listed, location); got != 1 {
			t.Fatalf("%s work count = %d, want 1; listed=%#v", location, got, listed)
		}
	}
	for _, state := range []string{"init", "failed"} {
		location := support.WorkCustomerLocation("task", state)
		if got := support.CountWorkAtCustomerState(listed, location); got != 0 {
			t.Fatalf("%s work count = %d, want 0 after classifier fan-out; listed=%#v", location, got, listed)
		}
	}
	assertClassifierRoutingSuccessfulSession(t, session, 3)
	dispatches := support.ObserveDispatchEvents(t, events)
	if len(dispatches) == 0 {
		t.Fatal("no dispatch events observed")
	}
	assertClassifierRoutingPublicWork(
		t,
		scenario,
		listed,
		events,
		wantWorkID,
		wantPayload,
		wantPayload,
	)
	assertClassifierRoutingBranchProviderPayloads(t, runner, wantPayload, 2)
	assertClassifierRoutingOutputWorkBranches(
		t,
		dispatches,
		[]string{
			support.WorkCustomerLocation("task", "done"),
			support.WorkCustomerLocation("branch-a", "init"),
			support.WorkCustomerLocation("branch-b", "init"),
		},
		wantPayload,
	)
	assertClassifierRoutingWorkstationDispatches(
		t,
		dispatches,
		classifierRoutingWorkstation,
		1,
		[]string{"fanout"},
	)
	assertClassifierRoutingCommandRequests(t, scenario, runner, 3)
}

func runClassifierRoutingSelectorGuard(
	t *testing.T,
	fixture *workRoutingPackageFixture,
) {
	t.Helper()
	const (
		workID  = "classifier-selector-guard-work"
		payload = "classifier-selector-guard-payload"
	)

	runner := newWorkRoutingScenarioCommandRunner(
		"classifier-selector-guard",
		[]platformprocess.CommandResult{{Stdout: support.CodexSuccessStdout("accepted")}},
		nil,
	)
	scenario := fixture.newScenario(
		t,
		"classifier-selector-guard",
		"classifier_routing_dir",
		runner,
	)
	writeLogicalMoveSeedRequest(t, scenario.factoryDir, workID, payload)
	fixture.provider.unregister(scenario.id)
	scenario.open(t)

	session, listed, events := scenario.observe(t, 10*time.Second)
	assertClassifierRoutingFailedTerminal(t, session, listed)
	dispatches := support.ObserveDispatchEvents(t, events)
	dispatch := assertClassifierRoutingFailedDispatch(
		t,
		dispatches,
		"provider execution failed",
	)
	if dispatch.Response == nil {
		t.Fatal("selector guard dispatch response is nil")
	}
	if strings.Contains(classifierRoutingDispatchErrorText(dispatch.Response), payload) {
		t.Fatalf("selector guard failure leaked payload %q", payload)
	}
	assertClassifierRoutingUnmatchedSelector(t, fixture)
	assertClassifierRoutingAmbiguousSelector(t, fixture)
}

func assertClassifierRoutingUnmatchedSelector(
	t *testing.T,
	fixture *workRoutingPackageFixture,
) {
	t.Helper()
	const requestContent = "sensitive-selector-request"
	_, err := fixture.provider.Run(context.Background(), platformprocess.CommandRequest{
		WorkDir: fixture.rootDir,
		Args:    []string{requestContent},
		Stdin:   []byte(requestContent),
	})
	if err == nil {
		t.Fatal("unmatched provider selector unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "matched 0 scenarios") {
		t.Fatalf("unmatched selector error = %q, want matched-0 diagnostic", err)
	}
	if strings.Contains(err.Error(), requestContent) {
		t.Fatalf("unmatched selector error leaked request content: %q", err)
	}
}

func assertClassifierRoutingAmbiguousSelector(
	t *testing.T,
	fixture *workRoutingPackageFixture,
) {
	t.Helper()
	const requestContent = "sensitive-selector-request"
	first := newWorkRoutingScenarioCommandRunner("ambiguous-first", nil, nil)
	second := newWorkRoutingScenarioCommandRunner("ambiguous-second", nil, nil)
	ambiguousWorkDir := filepath.Join(fixture.rootDir, "ambiguous-work")
	if err := fixture.provider.register("ambiguous-first", []string{fixture.rootDir}, first); err != nil {
		t.Fatalf("register first ambiguous selector: %v", err)
	}
	if err := fixture.provider.register("ambiguous-second", []string{ambiguousWorkDir}, second); err != nil {
		fixture.provider.unregister("ambiguous-first")
		t.Fatalf("register second ambiguous selector: %v", err)
	}
	_, err := fixture.provider.Run(context.Background(), platformprocess.CommandRequest{
		WorkDir: ambiguousWorkDir,
		Args:    []string{requestContent},
		Stdin:   []byte(requestContent),
	})
	fixture.provider.unregister("ambiguous-first")
	fixture.provider.unregister("ambiguous-second")
	if err == nil {
		t.Fatal("ambiguous provider selector unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "matched 2 scenarios") {
		t.Fatalf("ambiguous selector error = %q, want matched-2 diagnostic", err)
	}
	if strings.Contains(err.Error(), requestContent) {
		t.Fatalf("ambiguous selector error leaked request content: %q", err)
	}
}

// TestClassifierUnknownAndMalformedDecisionFailDistinctly proves unknown classifier
// labels and malformed decision payloads each fail with distinct customer-visible
// non-success outcomes at task:failed, without routing Work to successful terminal
// completion or reporting classifier routing as accepted success.
func TestClassifierUnknownAndMalformedDecisionFailDistinctly(t *testing.T) {
	cases := []struct {
		name              string
		providerOutput    string
		wantErrorContains string
	}{
		{
			name:              "unknown_classifier_label",
			providerOutput:    "MAYBE",
			wantErrorContains: "did not match any authored classification route",
		},
		{
			name:              "malformed_structured_decision_payload",
			providerOutput:    `{"decision":"MAYBE","feedback":"unknown structured decision"}`,
			wantErrorContains: "classifier output invalid",
		},
		{
			name:              "malformed_json_object_label",
			providerOutput:    `{"label":"accepted"}`,
			wantErrorContains: "classifier output invalid",
		},
	}

	signatures := make(map[string]string, len(cases))
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "classifier_routing_dir"))
			testutil.WriteSeedFile(t, dir, "task", []byte("classifier-failure-payload"))

			runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
				Stdout: support.CodexSuccessStdout(tc.providerOutput),
			})
			session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
				t,
				dir,
				serviceedges.Edges{ProviderCommandRunner: runner},
				20*time.Second,
			)

			assertClassifierRoutingFailedTerminal(t, session, listed)
			dispatch := assertClassifierRoutingFailedDispatch(
				t,
				support.ObserveDispatchEvents(t, events),
				tc.wantErrorContains,
			)
			signatures[tc.name] = classifierRoutingFailureSignature(dispatch)
		})
	}

	for _, left := range cases {
		for _, right := range cases {
			if left.name >= right.name {
				continue
			}
			if signatures[left.name] == signatures[right.name] {
				t.Fatalf(
					"failure signatures for %q and %q are identical (%q); want distinct customer-visible classifier failure markers",
					left.name,
					right.name,
					signatures[left.name],
				)
			}
		}
	}
}

// TestClassifierReworkFailureTerminatesWithoutCompletion proves that when a
// classifier routes Work into rework and the rework workstation fails, Work
// terminates at task:failed without reaching task:done.
func TestClassifierReworkFailureTerminatesWithoutCompletion(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "classifier_routing_dir"))
	testutil.WriteSeedFile(t, dir, "task", []byte("classifier-rework-failure-payload"))

	runner := testutil.NewProviderCommandRunner(
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("needs_changes")},
		platformprocess.CommandResult{ExitCode: 1, Stderr: []byte("rework failed")},
	)
	session, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		20*time.Second,
	)

	assertClassifierRoutingFailedTerminal(t, session, listed)
	assertClassifierRoutingWorkstationDispatches(
		t,
		support.ObserveDispatchEvents(t, events),
		classifierRoutingWorkstation,
		1,
		[]string{"needs_changes"},
	)
}

// TestClassifierRejectionWithoutArcsRoutesToFailedTerminal proves that when a
// worker returns a rejection outcome and the factory has no rejection routing
// arcs, Work terminates at task:failed without reaching task:done.
func TestClassifierRejectionWithoutArcsRoutesToFailedTerminal(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_no_arcs"))
	testutil.WriteSeedFile(t, dir, "task", []byte("work payload"))

	provider := testutil.NewMockProvider(support.RejectedProviderResponse("not good enough"))
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		10*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "failed")); got != 1 {
		t.Fatalf("task:failed work count = %d, want 1; listed=%#v", got, listed)
	}
	for _, state := range []string{"init", "done"} {
		location := support.WorkCustomerLocation("task", state)
		if got := support.CountWorkAtCustomerState(listed, location); got != 0 {
			t.Fatalf("%s work count = %d, want 0 after rejection without arcs; listed=%#v", location, got, listed)
		}
	}
}

// TestClassifierRejectionWithoutArcsRecordsDispatchFeedback proves rejection
// feedback is recorded on the public dispatch response event when no rejection
// routing arcs are configured.
func TestClassifierRejectionWithoutArcsRecordsDispatchFeedback(t *testing.T) {
	const wantFeedback = "missing tests"

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_no_arcs"))
	testutil.WriteSeedFile(t, dir, "task", []byte("work"))

	provider := testutil.NewMockProvider(support.RejectedProviderResponse(wantFeedback))
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: dir,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	support.WaitForTerminalStatus(t, server.URL(), 10*time.Second)
	listed := support.ListDefaultSessionWork(t, server.URL())

	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "failed")); got != 1 {
		t.Fatalf("task:failed work count = %d, want 1; listed=%#v", got, listed)
	}
	for _, event := range server.GetFactoryEvents(t) {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch response: %v", err)
		}
		if payload.Outcome != factoryapi.WorkOutcomeRejected ||
			payload.Output == nil ||
			*payload.Output != wantFeedback {
			t.Fatalf("dispatch response = %#v, want recorded rejection feedback %q", payload, wantFeedback)
		}
		server.Stop(t)
		return
	}
	t.Fatal("Factory Event history has no dispatch response")
}

// TestClassifierRejectionWithoutArcsReleasesResourcesForSubsequentWork proves
// that after a rejection without routing arcs fails one Work item, constrained
// resources are released so a subsequent Work item can complete.
func TestClassifierRejectionWithoutArcsReleasesResourcesForSubsequentWork(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "rejection_no_arcs_resources"))
	testutil.WriteSeedFile(t, dir, "task", []byte("first item"))
	testutil.WriteSeedFile(t, dir, "task", []byte("second item"))

	provider := testutil.NewMockProvider(
		support.RejectedProviderResponse("not good enough"),
		support.AcceptedProviderResponse(),
	)
	_, listed, _ := support.RunFactoryToCompletionWithEdgesAndObservations(
		t,
		dir,
		serviceedges.Edges{ProviderOverride: provider},
		20*time.Second,
	)

	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "failed")); got != 1 {
		t.Fatalf("task:failed work count = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "done")); got != 1 {
		t.Fatalf("task:done work count = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")); got != 0 {
		t.Fatalf("task:init work count = %d, want 0; listed=%#v", got, listed)
	}
}

func newClassifierRoutingCommandRunner(
	name string,
	labels []string,
	reworkResponses []string,
) *workRoutingScenarioCommandRunner {
	results := make([]platformprocess.CommandResult, 0, len(labels)+len(reworkResponses))
	labelIndex := 0
	reworkIndex := 0
	for labelIndex < len(labels) {
		if reworkIndex < len(reworkResponses) && labelIndex > 0 {
			results = append(results, platformprocess.CommandResult{
				Stdout: support.CodexSuccessStdout(reworkResponses[reworkIndex]),
			})
			reworkIndex++
		}
		results = append(results, platformprocess.CommandResult{
			Stdout: support.CodexSuccessStdout(labels[labelIndex]),
		})
		labelIndex++
	}
	for ; reworkIndex < len(reworkResponses); reworkIndex++ {
		results = append(results, platformprocess.CommandResult{
			Stdout: support.CodexSuccessStdout(reworkResponses[reworkIndex]),
		})
	}
	return newWorkRoutingScenarioCommandRunner(name, results, nil)
}

func assertClassifierRoutingTerminalState(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	terminalState string,
) {
	t.Helper()

	location := support.WorkCustomerLocation("task", terminalState)
	if got := support.CountWorkAtCustomerState(listed, location); got != 1 {
		t.Fatalf("%s work count = %d, want 1; listed=%#v", location, got, listed)
	}
	for _, state := range []string{"init", "rework", "failed"} {
		if state == terminalState {
			continue
		}
		other := support.WorkCustomerLocation("task", state)
		if got := support.CountWorkAtCustomerState(listed, other); got != 0 {
			t.Fatalf("%s work count = %d, want 0 while terminal is %s; listed=%#v", other, got, terminalState, listed)
		}
	}
}

func assertClassifierRoutingSuccessfulSession(
	t *testing.T,
	session factoryapi.FactorySession,
	wantTerminal int,
) {
	t.Helper()

	if session.Runtime.Progress.Categories.Terminal != wantTerminal || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf(
			"successful classifier session progress categories = %+v, want %d terminal and zero failed",
			session.Runtime.Progress.Categories,
			wantTerminal,
		)
	}
}

func assertClassifierRoutingCommandRequests(
	t *testing.T,
	scenario *workRoutingScenario,
	runner *workRoutingScenarioCommandRunner,
	wantCalls int,
) {
	t.Helper()

	requests := runner.requestsSnapshot()
	if len(requests) != wantCalls {
		t.Fatalf(
			"scenario %q provider command count = %d, want %d; requests=%#v",
			scenario.id,
			len(requests),
			wantCalls,
			requests,
		)
	}
	for index, request := range requests {
		if !workRoutingPathContains(scenario.rootDir, request.WorkDir) {
			t.Fatalf(
				"scenario %q provider request %d work directory %q escaped scenario root %q",
				scenario.id,
				index,
				request.WorkDir,
				scenario.rootDir,
			)
		}
	}
}

func assertClassifierRoutingPublicWork(
	t *testing.T,
	scenario *workRoutingScenario,
	listed factoryapi.ListWorkResponse,
	events []factoryapi.FactoryEvent,
	workID, wantPayload, wantFinalPayload string,
) {
	t.Helper()

	admitted := workRoutingAdmissionWork(t, events, workID)
	if got := workRoutingPublicWorkText(admitted); got != wantPayload {
		t.Fatalf("WORK_REQUEST payload = %q, want %q", got, wantPayload)
	}
	publicWork := getWorkRoutingWorkByID(t, scenario.fixture.baseURL, scenario.sessionID, workID)
	if got := support.StringPointerValue(publicWork.WorkId); got != workID {
		t.Fatalf("public Work ID = %q, want %q", got, workID)
	}
	if got := support.StringPointerValue(publicWork.RequestId); got != workID+"-request" {
		t.Fatalf("public Work request ID = %q, want %q", got, workID+"-request")
	}
	if got := support.StringPointerValue(publicWork.TraceId); got != workID+"-trace" {
		t.Fatalf("public Work trace ID = %q, want %q", got, workID+"-trace")
	}
	if got := workRoutingPublicWorkText(publicWork); got != wantFinalPayload {
		t.Fatalf("public Work payload = %q, want %q", got, wantFinalPayload)
	}
	if !support.HasWorkAtCustomerState(
		listed,
		workID,
		support.WorkCustomerLocation("task", "done"),
	) {
		t.Fatalf("listed Work %q missing at task:done; listed=%#v", workID, listed)
	}
}

func assertClassifierRoutingWorkstationDispatches(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	workstation string,
	wantCount int,
	wantLabels []string,
) {
	t.Helper()

	classifierDispatches := filterWorkstationDispatches(dispatches, workstation)
	if len(classifierDispatches) != wantCount {
		t.Fatalf(
			"classifier dispatch count = %d, want %d; dispatches=%#v",
			len(classifierDispatches),
			wantCount,
			classifierDispatches,
		)
	}
	if len(wantLabels) != wantCount {
		t.Fatalf("wantLabels length = %d, want %d to match classifier dispatch count", len(wantLabels), wantCount)
	}
	for index, dispatch := range classifierDispatches {
		if dispatch.Response == nil {
			t.Fatalf("classifier dispatch %q missing response payload", dispatch.DispatchID)
		}
		if dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
			t.Fatalf(
				"classifier dispatch %q outcome = %s, want ACCEPTED for label %q",
				dispatch.DispatchID,
				dispatch.Response.Outcome,
				wantLabels[index],
			)
		}
		if dispatch.Response.Output == nil || *dispatch.Response.Output != wantLabels[index] {
			t.Fatalf(
				"classifier dispatch %q output = %#v, want plain label %q",
				dispatch.DispatchID,
				dispatch.Response.Output,
				wantLabels[index],
			)
		}
	}
}

func filterWorkstationDispatches(
	dispatches []support.DispatchEventObservation,
	workstation string,
) []support.DispatchEventObservation {
	filtered := make([]support.DispatchEventObservation, 0, len(dispatches))
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != workstation {
			continue
		}
		filtered = append(filtered, dispatch)
	}
	return filtered
}

func countWorkstationDispatches(
	dispatches []support.DispatchEventObservation,
	workstation string,
) int {
	return len(filterWorkstationDispatches(dispatches, workstation))
}

func assertClassifierRoutingFailedTerminal(
	t *testing.T,
	session factoryapi.FactorySession,
	listed factoryapi.ListWorkResponse,
) {
	t.Helper()

	if session.Runtime.Progress.Categories.Terminal != 0 || session.Runtime.Progress.Categories.Failed != 1 {
		t.Fatalf(
			"session progress categories = %+v, want zero terminal and one failed",
			session.Runtime.Progress.Categories,
		)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "failed")); got != 1 {
		t.Fatalf("task:failed work count = %d, want 1; listed=%#v", got, listed)
	}
	for _, state := range []string{"init", "rework", "done"} {
		location := support.WorkCustomerLocation("task", state)
		if got := support.CountWorkAtCustomerState(listed, location); got != 0 {
			t.Fatalf("%s work count = %d, want 0 after classifier failure; listed=%#v", location, got, listed)
		}
	}
}

func assertClassifierRoutingFailedDispatch(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	wantErrorContains string,
) support.DispatchEventObservation {
	t.Helper()

	classifierDispatches := filterWorkstationDispatches(dispatches, classifierRoutingWorkstation)
	if len(classifierDispatches) != 1 {
		t.Fatalf(
			"classifier dispatch count = %d, want 1; dispatches=%#v",
			len(classifierDispatches),
			classifierDispatches,
		)
	}
	dispatch := classifierDispatches[0]
	if dispatch.Response == nil {
		t.Fatalf("classifier dispatch %q missing response payload", dispatch.DispatchID)
	}
	if dispatch.Response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf(
			"classifier dispatch %q outcome = %s, want FAILED",
			dispatch.DispatchID,
			dispatch.Response.Outcome,
		)
	}
	if dispatch.Response.SelectedClassificationLabel != nil &&
		strings.TrimSpace(*dispatch.Response.SelectedClassificationLabel) != "" {
		t.Fatalf(
			"classifier dispatch %q selectedClassificationLabel = %#v, want empty on failure",
			dispatch.DispatchID,
			dispatch.Response.SelectedClassificationLabel,
		)
	}
	errorText := classifierRoutingDispatchErrorText(dispatch.Response)
	if !strings.Contains(errorText, wantErrorContains) {
		t.Fatalf(
			"classifier dispatch %q error = %q, want substring %q",
			dispatch.DispatchID,
			errorText,
			wantErrorContains,
		)
	}
	return dispatch
}

func classifierRoutingDispatchErrorText(response *factoryapi.DispatchResponseEventPayload) string {
	if response == nil {
		return ""
	}
	if response.Error != nil && strings.TrimSpace(*response.Error) != "" {
		return *response.Error
	}
	if response.FailureDetail != nil && strings.TrimSpace(response.FailureDetail.Message) != "" {
		return response.FailureDetail.Message
	}
	return ""
}

func assertClassifierRoutingBranchProviderPayloads(
	t *testing.T,
	runner *workRoutingScenarioCommandRunner,
	want string,
	wantBranchCalls int,
) {
	t.Helper()

	requests := runner.requestsSnapshot()
	if len(requests) != 1+wantBranchCalls {
		t.Fatalf("provider command count = %d, want %d", len(requests), 1+wantBranchCalls)
	}
	branchRequests := requests[len(requests)-wantBranchCalls:]
	for index, request := range branchRequests {
		if !classifierRoutingProviderRequestIncludesPayload(request, want) {
			t.Fatalf(
				"branch provider request %d missing payload %q; args=%#v stdin=%q workDir=%q",
				index,
				want,
				request.Args,
				string(request.Stdin),
				request.WorkDir,
			)
		}
	}
}

func classifierRoutingProviderRequestIncludesPayload(
	request platformprocess.CommandRequest,
	want string,
) bool {
	if strings.Contains(string(request.Stdin), want) {
		return true
	}
	for _, arg := range request.Args {
		if strings.Contains(arg, want) {
			return true
		}
	}
	return false
}

func assertClassifierRoutingOutputWorkBranches(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	wantBranches []string,
	wantPayload string,
) {
	t.Helper()

	classifierDispatches := filterWorkstationDispatches(dispatches, classifierRoutingWorkstation)
	if len(classifierDispatches) != 1 {
		t.Fatalf("classifier dispatch count = %d, want 1", len(classifierDispatches))
	}
	response := classifierDispatches[0].Response
	if response == nil || response.OutputWork == nil {
		t.Fatalf("classifier dispatch missing outputWork branches: response=%#v", response)
	}
	seen := make(map[string]bool, len(wantBranches))
	for _, item := range *response.OutputWork {
		location := support.WorkItemCustomerLocation(item)
		if location == "" {
			continue
		}
		seen[location] = true
		if got := workRoutingPublicWorkText(item); got != wantPayload {
			t.Fatalf(
				"classifier outputWork branch %s payload = %q, want %q preserved across fan-out",
				location,
				got,
				wantPayload,
			)
		}
	}
	for _, location := range wantBranches {
		if !seen[location] {
			t.Fatalf("classifier outputWork missing branch %s; seen=%#v", location, seen)
		}
	}
}

func classifierRoutingFailureSignature(dispatch support.DispatchEventObservation) string {
	if dispatch.Response == nil {
		return ""
	}
	parts := []string{string(dispatch.Response.Outcome)}
	if dispatch.Response.Error != nil {
		parts = append(parts, *dispatch.Response.Error)
	}
	if dispatch.Response.FailureDetail != nil {
		parts = append(parts, string(dispatch.Response.FailureDetail.Reason))
		parts = append(parts, dispatch.Response.FailureDetail.Message)
	}
	return strings.Join(parts, "|")
}
