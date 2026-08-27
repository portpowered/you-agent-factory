package routing

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
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

// classifierRoutingFailureCase describes one invalid classifier result and the
// public failure marker it must produce.
type classifierRoutingFailureCase struct {
	name              string
	providerOutput    string
	wantErrorContains string
}

func classifierRoutingFailureCases() []classifierRoutingFailureCase {
	return []classifierRoutingFailureCase{
		{
			name:              "unknown_classifier_label",
			providerOutput:    "MAYBE",
			wantErrorContains: "did not match any authored classification route",
		},
		{
			name:              "malformed_structured_decision_payload",
			providerOutput:    "{\"decision\":\"MAYBE\",\"feedback\":\"unknown structured decision\"}",
			wantErrorContains: "classifier output invalid",
		},
		{
			name:              "malformed_json_object_label",
			providerOutput:    "{\"label\":\"accepted\"}",
			wantErrorContains: "classifier output invalid",
		},
	}
}

// runClassifierUnknownAndMalformedDecisionFailures proves the three invalid
// classifier forms through unique explicit sessions on the shared process.
func runClassifierUnknownAndMalformedDecisionFailures(
	t *testing.T,
	fixture *workRoutingPackageFixture,
) {
	t.Helper()
	cases := classifierRoutingFailureCases()
	signatures := make(map[string]string, len(cases))
	for _, tc := range cases {
		if !t.Run(tc.name, func(t *testing.T) {
			workID := "classifier-failure-" + tc.name
			payload := workID + "-payload"
			runner := newWorkRoutingScenarioCommandRunner(
				workID,
				[]platformprocess.CommandResult{{Stdout: support.CodexSuccessStdout(tc.providerOutput)}},
				nil,
			)
			scenario := fixture.newScenario(t, workID, "classifier_routing_dir", runner)
			writeLogicalMoveSeedRequest(t, scenario.factoryDir, workID, payload)
			scenario.open(t)

			session, listed, events := scenario.observe(t, 20*time.Second)
			assertClassifierRoutingFailedTerminal(t, session, listed)
			assertClassifierRoutingFailedPublicWork(t, scenario, events, workID, payload)
			dispatch := assertClassifierRoutingFailedDispatch(
				t,
				support.ObserveDispatchEvents(t, events),
				tc.wantErrorContains,
			)
			signatures[tc.name] = classifierRoutingFailureSignature(dispatch)
			assertClassifierRoutingCommandRequests(t, scenario, runner, 1)
		}) {
			continue
		}
	}

	for _, left := range cases {
		for _, right := range cases {
			if left.name >= right.name {
				continue
			}
			leftSignature, leftRan := signatures[left.name]
			rightSignature, rightRan := signatures[right.name]
			if !leftRan || !rightRan {
				continue
			}
			if leftSignature == rightSignature {
				t.Fatalf(
					"failure signatures for %q and %q are identical (%q); want distinct customer-visible classifier failure markers",
					left.name,
					right.name,
					leftSignature,
				)
			}
		}
	}
}

// runClassifierReworkFailureTerminatesWithoutCompletion proves that a
// needs_changes route followed by a failed rework command leaves Work failed.
func runClassifierReworkFailureTerminatesWithoutCompletion(
	t *testing.T,
	fixture *workRoutingPackageFixture,
) {
	t.Helper()
	const (
		workID  = "classifier-rework-failure-work"
		payload = "classifier-rework-failure-payload"
	)
	runner := newWorkRoutingScenarioCommandRunner(
		"classifier-rework-failure",
		[]platformprocess.CommandResult{
			{Stdout: support.CodexSuccessStdout("needs_changes")},
			{ExitCode: 1, Stderr: []byte("rework failed")},
		},
		nil,
	)
	scenario := fixture.newScenario(t, "classifier-rework-failure", "classifier_routing_dir", runner)
	writeLogicalMoveSeedRequest(t, scenario.factoryDir, workID, payload)
	scenario.open(t)

	session, listed, events := scenario.observe(t, 20*time.Second)
	assertClassifierRoutingFailedTerminal(t, session, listed)
	assertClassifierRoutingFailedPublicWork(t, scenario, events, workID, payload)
	dispatches := support.ObserveDispatchEvents(t, events)
	assertClassifierRoutingWorkstationDispatches(
		t,
		dispatches,
		classifierRoutingWorkstation,
		1,
		[]string{"needs_changes"},
	)
	assertClassifierRoutingFailedWorkstationDispatch(t, dispatches, "rework")
	assertClassifierRoutingCommandRequests(t, scenario, runner, 2)
}

// runClassifierRejectionWithoutArcsRoutesToFailedTerminal proves that a
// rejected command result with no rejection arcs leaves Work failed.
func runClassifierRejectionWithoutArcsRoutesToFailedTerminal(
	t *testing.T,
	fixture *workRoutingPackageFixture,
) {
	t.Helper()
	const (
		workID  = "classifier-rejection-terminal-work"
		payload = "classifier-rejection-terminal-payload"
	)
	runner := newWorkRoutingScenarioCommandRunner(
		"classifier-rejection-terminal",
		[]platformprocess.CommandResult{{Stdout: support.CodexSuccessStdout("not good enough")}},
		nil,
	)
	scenario := fixture.newScenario(t, "classifier-rejection-terminal", "rejection_no_arcs", runner)
	configureCommandEdgeWorker(t, scenario.factoryDir, "worker")
	writeLogicalMoveSeedRequest(t, scenario.factoryDir, workID, payload)
	scenario.open(t)

	session, listed, events := scenario.observe(t, 20*time.Second)
	assertClassifierRoutingFailedTerminal(t, session, listed)
	assertClassifierRoutingFailedPublicWork(t, scenario, events, workID, payload)
	assertClassifierRoutingRejectedDispatch(
		t,
		support.ObserveDispatchEvents(t, events),
		workID,
		"not good enough",
	)
	assertClassifierRoutingCommandRequests(t, scenario, runner, 1)
}

// runClassifierRejectionWithoutArcsRecordsDispatchFeedback proves that the
// rejected provider output is retained as public dispatch feedback.
func runClassifierRejectionWithoutArcsRecordsDispatchFeedback(
	t *testing.T,
	fixture *workRoutingPackageFixture,
) {
	t.Helper()
	const (
		workID       = "classifier-rejection-feedback-work"
		payload      = "classifier-rejection-feedback-payload"
		wantFeedback = "missing tests"
	)
	runner := newWorkRoutingScenarioCommandRunner(
		"classifier-rejection-feedback",
		[]platformprocess.CommandResult{{Stdout: support.CodexSuccessStdout(wantFeedback)}},
		nil,
	)
	scenario := fixture.newScenario(t, "classifier-rejection-feedback", "rejection_no_arcs", runner)
	configureCommandEdgeWorker(t, scenario.factoryDir, "worker")
	writeLogicalMoveSeedRequest(t, scenario.factoryDir, workID, payload)
	scenario.open(t)

	session, listed, events := scenario.observe(t, 20*time.Second)
	assertClassifierRoutingFailedTerminal(t, session, listed)
	assertClassifierRoutingFailedPublicWork(t, scenario, events, workID, payload)
	assertClassifierRoutingRejectedDispatch(
		t,
		support.ObserveDispatchEvents(t, events),
		workID,
		wantFeedback,
	)
	assertClassifierRoutingCommandRequests(t, scenario, runner, 1)
}

// runClassifierRejectionWithoutArcsReleasesResourcesForSubsequentWork proves
// that a rejected Work releases a capacity-one resource for the next Work.
func runClassifierRejectionWithoutArcsReleasesResourcesForSubsequentWork(
	t *testing.T,
	fixture *workRoutingPackageFixture,
) {
	t.Helper()
	const (
		firstWorkID   = "classifier-rejection-resource-first-work"
		secondWorkID  = "classifier-rejection-resource-second-work"
		firstPayload  = "classifier-rejection-resource-first-payload"
		secondPayload = "classifier-rejection-resource-second-payload"
	)
	runner := newWorkRoutingScenarioCommandRunner(
		"classifier-rejection-resources",
		[]platformprocess.CommandResult{
			{Stdout: support.CodexSuccessStdout("not good enough")},
			{Stdout: support.CodexSuccessStdout("accepted COMPLETE")},
		},
		nil,
	)
	scenario := fixture.newScenario(t, "classifier-rejection-resources", "rejection_no_arcs_resources", runner)
	configureCommandEdgeWorker(t, scenario.factoryDir, "worker")
	writeLogicalMoveSeedRequest(t, scenario.factoryDir, firstWorkID, firstPayload)
	writeLogicalMoveSeedRequest(t, scenario.factoryDir, secondWorkID, secondPayload)
	scenario.open(t)

	session, listed, events := scenario.observe(t, 20*time.Second)
	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 1 {
		t.Fatalf(
			"resource rejection session progress categories = %+v, want one terminal and one failed Work",
			session.Runtime.Progress.Categories,
		)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "failed")); got != 1 {
		t.Fatalf("task:failed work count = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "done")); got != 1 {
		t.Fatalf("task:done work count = %d, want 1 after resource release; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "init")); got != 0 {
		t.Fatalf("task:init work count = %d, want 0; listed=%#v", got, listed)
	}
	assertClassifierRoutingFailedPublicWork(t, scenario, events, firstWorkID, firstPayload)
	assertClassifierRoutingPublicWork(t, scenario, listed, events, secondWorkID, secondPayload, "accepted COMPLETE")
	dispatches := support.ObserveDispatchEvents(t, events)
	assertClassifierRoutingRejectedDispatch(t, dispatches, firstWorkID, "not good enough")
	assertClassifierRoutingAcceptedWorkDispatch(t, dispatches, secondWorkID)
	assertClassifierRoutingCommandRequests(t, scenario, runner, 2)
}

func configureCommandEdgeWorker(t *testing.T, dir, workerName string) {
	t.Helper()
	support.WriteAgentConfig(t, dir, workerName,
		"---\n"+
			"type: MODEL_WORKER\n"+
			"model: gpt-5-codex\n"+
			"modelProvider: codex\n"+
			"stopToken: COMPLETE\n"+
			"---\n\n"+
			"Process the task.\n",
	)
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

func assertClassifierRoutingFailedPublicWork(
	t *testing.T,
	scenario *workRoutingScenario,
	events []factoryapi.FactoryEvent,
	workID, wantPayload string,
) {
	t.Helper()

	admitted := workRoutingAdmissionWork(t, events, workID)
	if got := workRoutingPublicWorkText(admitted); got != wantPayload {
		t.Fatalf("failed WORK_REQUEST payload = %q, want %q", got, wantPayload)
	}
	publicWork := getWorkRoutingWorkByID(t, scenario.fixture.baseURL, scenario.sessionID, workID)
	if got := support.StringPointerValue(publicWork.WorkId); got != workID {
		t.Fatalf("failed public Work ID = %q, want %q", got, workID)
	}
	if got := support.StringPointerValue(publicWork.RequestId); got != workID+"-request" {
		t.Fatalf("failed public Work request ID = %q, want %q", got, workID+"-request")
	}
	if got := support.StringPointerValue(publicWork.TraceId); got != workID+"-trace" {
		t.Fatalf("failed public Work trace ID = %q, want %q", got, workID+"-trace")
	}
	if got := workRoutingPublicWorkText(publicWork); got != wantPayload {
		t.Fatalf("failed public Work payload = %q, want %q", got, wantPayload)
	}
}

func assertClassifierRoutingFailedWorkstationDispatch(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	workstation string,
) support.DispatchEventObservation {
	t.Helper()

	failed := filterWorkstationDispatches(dispatches, workstation)
	if len(failed) != 1 {
		t.Fatalf(
			"%s dispatch count = %d, want 1; dispatches=%#v",
			workstation,
			len(failed),
			failed,
		)
	}
	dispatch := failed[0]
	if dispatch.Response == nil {
		t.Fatalf("%s dispatch %q missing response payload", workstation, dispatch.DispatchID)
	}
	if dispatch.Response.Outcome != factoryapi.WorkOutcomeFailed {
		t.Fatalf(
			"%s dispatch %q outcome = %s, want FAILED",
			workstation,
			dispatch.DispatchID,
			dispatch.Response.Outcome,
		)
	}
	return dispatch
}

func assertClassifierRoutingRejectedDispatch(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	workID, wantFeedback string,
) support.DispatchEventObservation {
	t.Helper()

	processDispatches := filterWorkstationDispatches(dispatches, "process")
	matching := make([]support.DispatchEventObservation, 0, len(processDispatches))
	for _, dispatch := range processDispatches {
		if workID == "" || support.DispatchObservationIncludesWork(dispatch, workID) {
			matching = append(matching, dispatch)
		}
	}
	if len(matching) != 1 {
		t.Fatalf(
			"rejected process dispatch count for Work %q = %d, want 1; dispatches=%#v",
			workID,
			len(matching),
			processDispatches,
		)
	}
	dispatch := matching[0]
	if dispatch.Response == nil {
		t.Fatalf("rejected process dispatch %q missing response payload", dispatch.DispatchID)
	}
	if dispatch.Response.Outcome != factoryapi.WorkOutcomeRejected {
		t.Fatalf(
			"rejected process dispatch %q outcome = %s, want REJECTED",
			dispatch.DispatchID,
			dispatch.Response.Outcome,
		)
	}
	if dispatch.Response.Output == nil || *dispatch.Response.Output != wantFeedback {
		t.Fatalf(
			"rejected process dispatch %q output = %#v, want feedback %q",
			dispatch.DispatchID,
			dispatch.Response.Output,
			wantFeedback,
		)
	}
	return dispatch
}

func assertClassifierRoutingAcceptedWorkDispatch(
	t *testing.T,
	dispatches []support.DispatchEventObservation,
	workID string,
) support.DispatchEventObservation {
	t.Helper()

	processDispatches := filterWorkstationDispatches(dispatches, "process")
	matching := make([]support.DispatchEventObservation, 0, len(processDispatches))
	for _, dispatch := range processDispatches {
		if support.DispatchObservationIncludesWork(dispatch, workID) {
			matching = append(matching, dispatch)
		}
	}
	if len(matching) != 1 {
		t.Fatalf(
			"accepted process dispatch count for Work %q = %d, want 1; dispatches=%#v",
			workID,
			len(matching),
			processDispatches,
		)
	}
	dispatch := matching[0]
	if dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted {
		var outcome factoryapi.WorkOutcome
		if dispatch.Response != nil {
			outcome = dispatch.Response.Outcome
		}
		t.Fatalf(
			"accepted process dispatch %q outcome = %s, want ACCEPTED",
			dispatch.DispatchID,
			outcome,
		)
	}
	return dispatch
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
