package routing

import (
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

// TestClassifierRoutesEveryKnownDecision proves that each authored classifier
// label routes Work to its documented public workType:state outcome through the
// customer process boundary, including accepted completion, approve-first
// completion, rework loop-back followed by completion, and rejection-path
// retry followed by completion.
func TestClassifierRoutesEveryKnownDecision(t *testing.T) {
	cases := []struct {
		name              string
		providerLabels    []string
		reworkResponses   []string
		wantTerminalState string
		wantClassifier    int
		wantRework        int
		wantLabels        []string
	}{
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
		},
		{
			name:              "rejection_path_retries_then_completes",
			providerLabels:    []string{"rejected", "accepted"},
			wantTerminalState: "done",
			wantClassifier:    2,
			wantLabels:        []string{"rejected", "accepted"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "classifier_routing_dir"))
			testutil.WriteSeedFile(t, dir, "task", []byte("classifier-routing-payload"))

			runner := newClassifierRoutingCommandRunner(tc.providerLabels, tc.reworkResponses)
			_, listed, events := support.RunFactoryToCompletionWithEdgesAndObservations(
				t,
				dir,
				serviceedges.Edges{ProviderCommandRunner: runner},
				20*time.Second,
			)

			assertClassifierRoutingTerminalState(t, listed, tc.wantTerminalState)
			assertClassifierRoutingWorkstationDispatches(
				t,
				support.ObserveDispatchEvents(t, events),
				classifierRoutingWorkstation,
				tc.wantClassifier,
				tc.wantLabels,
			)
			if got := countWorkstationDispatches(
				support.ObserveDispatchEvents(t, events),
				"rework",
			); got != tc.wantRework {
				t.Fatalf("rework dispatch count = %d, want %d", got, tc.wantRework)
			}
		})
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

func newClassifierRoutingCommandRunner(
	labels []string,
	reworkResponses []string,
) *testutil.ProviderCommandRunner {
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
	return testutil.NewProviderCommandRunner(results...)
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
