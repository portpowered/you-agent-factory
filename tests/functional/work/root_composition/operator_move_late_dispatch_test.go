package root_composition_test

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	operatorMoveLateDispatchRequestID     = "fun-work-operator-move-late-dispatch"
	operatorMoveLateDispatchTargetID      = "fun-work-late-dispatch-target"
	operatorMoveLateDispatchParentID      = "fun-work-late-dispatch-parent"
	operatorMoveLateDispatchDependentID   = "fun-work-late-dispatch-dependent"
	operatorMoveLateDispatchTargetName    = "late-dispatch-target"
	operatorMoveLateDispatchParentName    = "joined-parent"
	operatorMoveLateDispatchDependentName = "healthy-dependent"
)

// TestOperatorMoveTerminalWorkRejectsLateDispatchResult proves the incident
// through one root-built process: a controlled provider call is held, the
// public CLI moves the target to a terminal state, and the released late
// result is retired as one redacted diagnostic without failing or reactivating
// related Work.
func TestOperatorMoveTerminalWorkRejectsLateDispatchResult(t *testing.T) {
	t.Parallel()

	runner := support.NewGatedFailureCommandRunner("controlled late provider failure")
	defer runner.Release()
	dir := support.ScaffoldFactory(t, operatorMoveLateDispatchFactoryConfig())
	support.WriteAgentConfig(
		t,
		dir,
		"processor",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"),
	)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderCommandRunner: runner,
		},
	})
	t.Cleanup(func() { server.Stop(t) })

	stream := support.OpenFactoryEventStreamAt(
		t,
		support.DefaultSessionEventsURL(server.URL()),
	)
	submitOutput := executeOperatorMoveLateDispatchCLI(
		t,
		server,
		"--json", "submit", "batch", operatorMoveLateDispatchBatchJSON(),
	)
	assertOperatorMoveLateDispatchBatchSubmit(t, submitOutput)

	observationContext, cancelObservation := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancelObservation()
	if err := runner.WaitForStart(observationContext); err != nil {
		t.Fatalf("provider command did not reach the controlled edge: %v", err)
	}

	parentMoveOutput := executeOperatorMoveLateDispatchCLI(
		t,
		server,
		"work", "move", operatorMoveLateDispatchParentID, "complete",
	)
	assertOperatorMoveLateDispatchMoveOutput(
		t,
		parentMoveOutput,
		operatorMoveLateDispatchParentID,
		"init",
		"complete",
	)

	targetMoveInputs := operatorMoveLateDispatchCLIInputs(
		t,
		server,
		"work", "move", operatorMoveLateDispatchTargetID, "complete",
	)
	targetMoveDone := make(chan error, 1)
	go func() {
		targetMoveDone <- server.Execute(t, targetMoveInputs.Input)
	}()

	reason, err := runner.WaitForCancellation(observationContext)
	if err != nil {
		t.Fatalf("controlled provider command did not observe Worker cancellation: %v", err)
	}
	if reason != platformprocess.CancellationReasonSuperseded {
		t.Fatalf("provider cancellation reason = %q, want SUPERSEDED at the command edge", reason)
	}
	runner.Release()

	select {
	case err := <-targetMoveDone:
		if err != nil {
			t.Fatalf(
				"Process.Execute(work move) error = %v\nstdout:\n%s\nstderr:\n%s",
				err,
				targetMoveInputs.Stdout(),
				targetMoveInputs.Stderr(),
			)
		}
	case <-observationContext.Done():
		t.Fatalf("public terminal move did not complete after cancellation: %v", observationContext.Err())
	}
	if err := runner.WaitForCompletion(observationContext); err != nil {
		t.Fatalf("controlled provider command did not return its late failure: %v", err)
	}
	if got := runner.CallCount(); got != 1 {
		t.Fatalf("controlled provider call count = %d, want exactly one execution", got)
	}
	assertOperatorMoveLateDispatchMoveOutput(
		t,
		targetMoveInputs.Stdout(),
		operatorMoveLateDispatchTargetID,
		"init",
		"complete",
	)

	listedOutput := executeOperatorMoveLateDispatchCLI(t, server, "--json", "work", "list")
	assertOperatorMoveLateDispatchWorkListing(t, listedOutput)
	assertOperatorMoveLateDispatchEvents(t, server, stream, observationContext)
	session := support.GetDefaultSession(t, server.URL())
	if session.Runtime.Progress.InFlightCount != 0 {
		t.Fatalf("live Factory Session in-flight count = %d, want 0", session.Runtime.Progress.InFlightCount)
	}
	assertOperatorMoveLateDispatchWorkListingResponse(
		t,
		support.ListDefaultSessionWork(t, server.URL()),
	)
}

func operatorMoveLateDispatchFactoryConfig() map[string]any {
	return map[string]any{
		"name": "operator-move-late-dispatch",
		"workTypes": []map[string]any{
			operatorMoveLateDispatchWorkType("target"),
			operatorMoveLateDispatchWorkType("parent"),
			operatorMoveLateDispatchWorkType("dependent"),
		},
		"workers": []map[string]any{{"name": "processor"}},
		"workstations": []map[string]any{{
			"name":      "process-target",
			"worker":    "processor",
			"inputs":    []map[string]any{{"workType": "target", "state": "init"}},
			"outputs":   []map[string]any{{"workType": "target", "state": "complete"}},
			"onFailure": []map[string]any{{"workType": "target", "state": "failed"}},
		}},
	}
}

func operatorMoveLateDispatchWorkType(name string) map[string]any {
	return map[string]any{
		"name": name,
		"states": []map[string]any{
			{"name": "init", "type": "INITIAL"},
			{"name": "complete", "type": "TERMINAL"},
			{"name": "failed", "type": "FAILED"},
		},
	}
}

func operatorMoveLateDispatchBatchJSON() string {
	return fmt.Sprintf(`{
		"requestId": %q,
		"type": "FACTORY_REQUEST_BATCH",
		"works": [
			{"name": %q, "workId": %q, "workTypeName": "target", "payload": {"role": "target"}},
			{"name": %q, "workId": %q, "workTypeName": "parent", "payload": {"role": "parent"}},
			{"name": %q, "workId": %q, "workTypeName": "dependent", "payload": {"role": "dependent"}}
		],
		"relations": [
			{"type": "PARENT_CHILD", "sourceWorkName": %q, "targetWorkName": %q},
			{"type": "DEPENDS_ON", "sourceWorkName": %q, "targetWorkName": %q, "requiredState": "complete"}
		]
	}`, operatorMoveLateDispatchRequestID,
		operatorMoveLateDispatchTargetName, operatorMoveLateDispatchTargetID,
		operatorMoveLateDispatchParentName, operatorMoveLateDispatchParentID,
		operatorMoveLateDispatchDependentName, operatorMoveLateDispatchDependentID,
		operatorMoveLateDispatchTargetName, operatorMoveLateDispatchParentName,
		operatorMoveLateDispatchDependentName, operatorMoveLateDispatchTargetName)
}

func operatorMoveLateDispatchCLIInputs(
	t *testing.T,
	server *support.FunctionalAPIServer,
	args ...string,
) *support.CapturedInputs {
	t.Helper()

	home := t.TempDir()
	inputs := support.FakeInputs(
		t.Context(),
		append([]string{"you", "--server", server.URL()}, args...),
	)
	inputs.Input.Env = operatorMoveLateDispatchHomeEnvironment(home)
	inputs.Input.WorkingDirectory = home
	stdinIsTTY := true
	inputs.Input.StdinIsTTY = &stdinIsTTY
	inputs.Input.Stdin = strings.NewReader("")
	return inputs
}

func executeOperatorMoveLateDispatchCLI(
	t *testing.T,
	server *support.FunctionalAPIServer,
	args ...string,
) string {
	t.Helper()
	inputs := operatorMoveLateDispatchCLIInputs(t, server, args...)
	if err := server.Execute(t, inputs.Input); err != nil {
		t.Fatalf(
			"Process.Execute(%v) error = %v\nstdout:\n%s\nstderr:\n%s",
			args,
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}
	return inputs.Stdout()
}

func operatorMoveLateDispatchHomeEnvironment(home string) []string {
	if runtime.GOOS == "windows" {
		return []string{"USERPROFILE=" + home}
	}
	if runtime.GOOS == "plan9" {
		return []string{"home=" + home}
	}
	return []string{"HOME=" + home}
}

func assertOperatorMoveLateDispatchBatchSubmit(t *testing.T, output string) {
	t.Helper()
	var submitted struct {
		RequestID string `json:"requestId"`
		TraceID   string `json:"traceId"`
		WorkCount int    `json:"workCount"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &submitted); err != nil {
		t.Fatalf("decode late-dispatch batch submit: %v\noutput:\n%s", err, output)
	}
	if submitted.RequestID != operatorMoveLateDispatchRequestID ||
		strings.TrimSpace(submitted.TraceID) == "" || submitted.WorkCount != 3 {
		t.Fatalf("late-dispatch batch submit = %#v, want request, trace, and three works", submitted)
	}
}

func assertOperatorMoveLateDispatchMoveOutput(
	t *testing.T,
	output, workID, previousState, newState string,
) {
	t.Helper()
	for _, marker := range []string{
		"Work ID:\t" + workID,
		"Previous state:\t" + previousState,
		"New state:\t" + newState,
	} {
		if !strings.Contains(output, marker) {
			t.Fatalf("late-dispatch move output missing %q:\n%s", marker, output)
		}
	}
}

func assertOperatorMoveLateDispatchWorkListing(t *testing.T, output string) {
	t.Helper()
	var listed factoryapi.ListWorkResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &listed); err != nil {
		t.Fatalf("decode late-dispatch work list: %v\noutput:\n%s", err, output)
	}
	assertOperatorMoveLateDispatchWorkListingResponse(t, listed)
}

func assertOperatorMoveLateDispatchWorkListingResponse(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
) {
	t.Helper()
	wantStates := map[string]string{
		operatorMoveLateDispatchTargetID:    "complete",
		operatorMoveLateDispatchParentID:    "complete",
		operatorMoveLateDispatchDependentID: "init",
	}
	if len(listed.Results) != len(wantStates) {
		t.Fatalf(
			"work list count = %d, want exactly %d without generated cascade work: %#v",
			len(listed.Results), len(wantStates), listed.Results,
		)
	}
	seen := make(map[string]bool, len(wantStates))
	for _, item := range listed.Results {
		workID := support.StringPointerValue(item.WorkId)
		wantState, ok := wantStates[workID]
		if !ok {
			continue
		}
		if item.State == nil || item.State.Name != wantState {
			t.Fatalf("work %q state = %#v, want %q", workID, item.State, wantState)
		}
		wantType := factoryapi.WorkStateTypeINITIAL
		if workID != operatorMoveLateDispatchDependentID {
			wantType = factoryapi.WorkStateTypeTERMINAL
		}
		if item.State.Type != wantType {
			t.Fatalf("work %q state type = %q, want %q", workID, item.State.Type, wantType)
		}
		if item.State.Type == factoryapi.WorkStateTypeFAILED {
			t.Fatalf("work %q entered FAILED after late dispatch: %#v", workID, item.State)
		}
		seen[workID] = true
	}
	for workID, wantState := range wantStates {
		if !seen[workID] {
			t.Fatalf("work list missing %q at %q: %#v", workID, wantState, listed.Results)
		}
	}
}

func assertOperatorMoveLateDispatchEvents(
	t *testing.T,
	server *support.FunctionalAPIServer,
	stream *support.FactoryEventStream,
	ctx context.Context,
) {
	t.Helper()

	for {
		event := stream.NextEventContext(ctx)
		if event.Type != factoryapi.FactoryEventTypeDispatchResultIgnored ||
			!operatorMoveLateDispatchEventHasWork(event, operatorMoveLateDispatchTargetID) {
			continue
		}

		payload, err := event.Payload.AsDispatchResultIgnoredEventPayload()
		if err != nil {
			t.Fatalf("decode DISPATCH_RESULT_IGNORED event: %v", err)
		}
		if payload.Reason != factoryapi.WORKALREADYTERMINAL {
			t.Fatalf("ignored result reason = %q, want WORK_ALREADY_TERMINAL", payload.Reason)
		}
		if payload.ObservedState.Name != "complete" ||
			payload.ObservedState.Type != factoryapi.WorkStateTypeTERMINAL {
			t.Fatalf("ignored result observed state = %#v, want terminal complete", payload.ObservedState)
		}
		if payload.ResultOutcome != factoryapi.WorkOutcomeCanceled {
			t.Fatalf("ignored result outcome = %q, want CANCELED after superseding cancellation", payload.ResultOutcome)
		}
		if event.Context.DispatchId == nil || strings.TrimSpace(*event.Context.DispatchId) == "" {
			t.Fatal("ignored result event is missing dispatch identity")
		}

		allEvents := support.GetFactoryEventsAt(t, server.URL())
		ignoredCount := 0
		ignoredSequence := -1
		moveSequence := -1
		postMoveSessionStarts := 0
		for _, retained := range allEvents {
			if retained.Type == factoryapi.FactoryEventTypeDispatchResultIgnored &&
				operatorMoveLateDispatchEventHasWork(retained, operatorMoveLateDispatchTargetID) {
				ignoredCount++
				ignoredSequence = retained.Context.Sequence
			}
			if retained.Type != factoryapi.FactoryEventTypeWorkStateChange {
				continue
			}
			movePayload, err := retained.Payload.AsWorkStateChangeEventPayload()
			if err != nil || movePayload.WorkId != operatorMoveLateDispatchTargetID ||
				movePayload.ToState != "complete" {
				continue
			}
			moveSequence = retained.Context.Sequence
		}
		for _, retained := range allEvents {
			if retained.Type == factoryapi.FactoryEventTypeSessionStarted &&
				retained.Context.Sequence > moveSequence {
				postMoveSessionStarts++
			}
		}
		if ignoredCount != 1 {
			t.Fatalf("retained late-dispatch ignored event count = %d, want 1", ignoredCount)
		}
		if moveSequence < 0 || ignoredSequence <= moveSequence {
			t.Fatalf(
				"late-dispatch event ordering = move sequence %d, ignored sequence %d; want move before ignored",
				moveSequence,
				ignoredSequence,
			)
		}
		if postMoveSessionStarts != 0 {
			t.Fatalf("Factory Session start events after operator move = %d, want 0", postMoveSessionStarts)
		}
		return
	}
}

func operatorMoveLateDispatchEventHasWork(event factoryapi.FactoryEvent, workID string) bool {
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
