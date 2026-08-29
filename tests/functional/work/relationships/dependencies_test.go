package relationships

import (
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	dependencyStartWorkstation   = "start"
	dependencyFinishWorkstation  = "finish"
	dependencyExecuteWorkstation = "execute"
	dependencyReviewWorkstation  = "review"
	dependencyRequiredState      = "complete"
	dependencyArchivedState      = "archived"
)

// TestDependentWorkWaitsForPrerequisiteTargetState proves through public Work
// listings and Factory Event dispatch observations that a DEPENDS_ON dependent
// stays undispatched at its initial state until the prerequisite reaches the
// declared requiredState, then proceeds through the public work session once
// that prerequisite target state is satisfied.
func testDependentWorkWaitsForPrerequisiteTargetState(t *testing.T, baseURL string) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_dir"))

	prerequisiteWorkID := "task-prerequisite-a"
	dependentWorkID := "task-dependent-b"

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     prerequisiteWorkID,
		Payload:    []byte("prerequisite task"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     dependentWorkID,
		Payload:    []byte("dependent task"),
		Relations: []work.Relation{
			{
				Type:          work.RelationDependsOn,
				TargetWorkID:  prerequisiteWorkID,
				RequiredState: dependencyRequiredState,
			},
		},
	})

	session, listed, events := runSharedRelationshipFactoryToCompletion(t, baseURL, dir, 15*time.Second)

	assertDependencyWorkLocations(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "init"):       0,
		support.WorkCustomerLocation("task", "processing"): 0,
		support.WorkCustomerLocation("task", "complete"):   2,
	})
	if !support.HasWorkAtCustomerState(listed, prerequisiteWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("prerequisite work %q not at %q in public listing: %#v", prerequisiteWorkID, dependencyRequiredState, listed)
	}
	if !support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("dependent work %q not at %q in public listing: %#v", dependentWorkID, dependencyRequiredState, listed)
	}

	prerequisiteCompleteSequence, dependentStartSequence := dependencyDispatchOrdering(
		t,
		events,
		prerequisiteWorkID,
		dependentWorkID,
	)
	if dependentStartSequence <= prerequisiteCompleteSequence {
		t.Fatalf(
			"dependent %q dispatch at %q sequence = %d, want after prerequisite %q complete sequence %d",
			dependentWorkID,
			dependencyStartWorkstation,
			dependentStartSequence,
			prerequisiteWorkID,
			prerequisiteCompleteSequence,
		)
	}

	if session.Runtime.Progress.Categories.Terminal != 2 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress categories = %+v, want two terminal and zero failed", session.Runtime.Progress.Categories)
	}
}

const (
	sharedDependencyFailurePrerequisiteID = "shared-failure-prerequisite"
	sharedDependencyFailureDependentID    = "shared-failure-dependent"
)

// testDependentWorkDoesNotDispatchAfterPrerequisiteFailure proves through
// public Work listings and Factory Event dispatch observations that a
// DEPENDS_ON dependent never receives a worker dispatch when its prerequisite
// reaches a failed terminal outcome instead of the declared requiredState.
func testDependentWorkDoesNotDispatchAfterPrerequisiteFailure(t *testing.T, baseURL string) {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_dir"))

	prerequisiteWorkID := sharedDependencyFailurePrerequisiteID
	dependentWorkID := sharedDependencyFailureDependentID

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     prerequisiteWorkID,
		Payload:    []byte("prerequisite task"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     dependentWorkID,
		Payload:    []byte("dependent task"),
		Relations: []work.Relation{
			{
				Type:          work.RelationDependsOn,
				TargetWorkID:  prerequisiteWorkID,
				RequiredState: dependencyRequiredState,
			},
		},
	})

	session, listed, events := runSharedRelationshipFactoryToCompletionAndClose(
		t,
		baseURL,
		dir,
		15*time.Second,
	)

	if !support.HasWorkAtCustomerState(listed, prerequisiteWorkID, support.WorkCustomerLocation("task", "failed")) {
		t.Fatalf("prerequisite work %q not at failed in public listing: %#v", prerequisiteWorkID, listed)
	}
	if !support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("task", "failed")) {
		t.Fatalf("dependent work %q not at blocked failed state in public listing: %#v", dependentWorkID, listed)
	}
	if support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("dependent work %q reached %q after prerequisite failure: %#v", dependentWorkID, dependencyRequiredState, listed)
	}
	if support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("task", "processing")) {
		t.Fatalf("dependent work %q reached processing after prerequisite failure: %#v", dependentWorkID, listed)
	}

	assertOnlyPrerequisiteDispatches(t, events, prerequisiteWorkID, dependentWorkID)

	if session.Runtime.Progress.Categories.Terminal != 0 || session.Runtime.Progress.Categories.Failed != 2 {
		t.Fatalf("session progress categories = %+v, want zero terminal and two failed", session.Runtime.Progress.Categories)
	}
	runSharedHostReuseProbe(t, baseURL)
}

func assertOnlyPrerequisiteDispatches(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	prerequisiteWorkID, dependentWorkID string,
) {
	t.Helper()
	startDispatches := 0
	finishDispatches := 0
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode prerequisite dispatch event: %v", err)
		}
		if dispatchRequestIncludesWork(payload, dependentWorkID) {
			t.Fatalf("dependent Work %q received a dispatch at sequence %d", dependentWorkID, event.Context.Sequence)
		}
		if !dispatchRequestIncludesWork(payload, prerequisiteWorkID) {
			continue
		}
		switch payload.TransitionId {
		case dependencyStartWorkstation:
			startDispatches++
		case dependencyFinishWorkstation:
			finishDispatches++
		}
	}
	if startDispatches != 1 || finishDispatches != 1 {
		t.Fatalf(
			"prerequisite dispatches = start:%d finish:%d, want one start and one finish",
			startDispatches,
			finishDispatches,
		)
	}
}

// TestWorkWithoutDependsOnRelationsDispatchesNormally proves through public Work
// listings and Factory Event dispatch observations that work submitted without any
// DEPENDS_ON relations is not blocked by dependency tracking and reaches its
// terminal success state through the normal public work session path.
func testWorkWithoutDependsOnRelationsDispatchesNormally(t *testing.T, baseURL string) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_simple_dir"))
	workID := "task-no-deps"

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     workID,
		Payload:    []byte("no dependency relations"),
	})

	session, listed, events := runSharedRelationshipFactoryToCompletion(t, baseURL, dir, 5*time.Second)

	assertDependencyWorkLocations(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "init"):       0,
		support.WorkCustomerLocation("task", "processing"): 0,
		support.WorkCustomerLocation("task", "complete"):   1,
	})
	if !support.HasWorkAtCustomerState(listed, workID, support.WorkCustomerLocation("task", "complete")) {
		t.Fatalf("work %q without DEPENDS_ON not at complete in public listing: %#v", workID, listed)
	}

	dispatchSequence := -1
	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			continue
		}
		if payload.TransitionId != "process" {
			continue
		}
		if !dispatchRequestIncludesWork(payload, workID) {
			continue
		}
		dispatchSequence = event.Context.Sequence
		break
	}
	if dispatchSequence < 0 {
		t.Fatalf("work %q without DEPENDS_ON never received public process dispatch", workID)
	}

	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress categories = %+v, want one terminal and zero failed", session.Runtime.Progress.Categories)
	}
}

// TestFanInReleasesOnlyAfterEveryPrerequisite proves through public Work
// listings and Factory Event dispatch observations that a DEPENDS_ON join stays
// undispatched while only a proper subset of prerequisites has reached the
// declared requiredState, then proceeds only after every prerequisite target
// state is satisfied.
// backendsizecheck:ignore-function pre-existing baseline debt recorded 2026-08-08; split this oversized code into focused units and remove this exemption
func testFanInReleasesOnlyAfterEveryPrerequisite(t *testing.T, baseURL string, gate *support.MockWorkerGate) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_dir"))

	prerequisiteAWorkID := "task-prerequisite-a"
	prerequisiteBWorkID := "task-prerequisite-b"
	dependentWorkID := "task-dependent-join"

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     prerequisiteAWorkID,
		Payload:    []byte("prerequisite task A"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     prerequisiteBWorkID,
		Payload:    []byte("prerequisite task B"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     dependentWorkID,
		Payload:    []byte("fan-in dependent task"),
		Relations: []work.Relation{
			{
				Type:          work.RelationDependsOn,
				TargetWorkID:  prerequisiteAWorkID,
				RequiredState: dependencyRequiredState,
			},
			{
				Type:          work.RelationDependsOn,
				TargetWorkID:  prerequisiteBWorkID,
				RequiredState: dependencyRequiredState,
			},
		},
	})

	opened := support.OpenFactorySessionAt(t, baseURL, dir)
	sessionID := opened.Session.Id
	t.Cleanup(func() { support.CloseFactorySessionAt(t, baseURL, sessionID) })
	gate.WaitForArrival(t, 15*time.Second)

	partialListed, partialEvents := waitForPartialFanInObservation(
		t,
		baseURL,
		sessionID,
		15*time.Second,
		prerequisiteAWorkID,
		prerequisiteBWorkID,
		dependentWorkID,
	)
	assertFanInBlockedAfterPartialPrerequisites(
		t,
		partialListed,
		partialEvents,
		prerequisiteAWorkID,
		prerequisiteBWorkID,
		dependentWorkID,
	)

	gate.Release()
	support.WaitForSessionTerminalStatus(t, baseURL, sessionID, 15*time.Second)

	session := getRelationshipSession(t, baseURL, sessionID)
	listed := listRelationshipSessionWork(t, baseURL, sessionID)
	events := support.GetFactoryEventsForSessionAt(t, baseURL, sessionID)

	assertDependencyWorkLocations(t, listed, map[string]int{
		support.WorkCustomerLocation("task", "init"):       0,
		support.WorkCustomerLocation("task", "processing"): 0,
		support.WorkCustomerLocation("task", "complete"):   3,
	})
	if !support.HasWorkAtCustomerState(listed, prerequisiteAWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("prerequisite A %q not at %q in public listing: %#v", prerequisiteAWorkID, dependencyRequiredState, listed)
	}
	if !support.HasWorkAtCustomerState(listed, prerequisiteBWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("prerequisite B %q not at %q in public listing: %#v", prerequisiteBWorkID, dependencyRequiredState, listed)
	}
	if !support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("task", dependencyRequiredState)) {
		t.Fatalf("dependent work %q not at %q in public listing: %#v", dependentWorkID, dependencyRequiredState, listed)
	}

	prerequisiteASequence, prerequisiteBSequence, dependentStartSequence := fanInDispatchOrdering(
		t,
		events,
		prerequisiteAWorkID,
		prerequisiteBWorkID,
		dependentWorkID,
	)
	if dependentStartSequence <= prerequisiteASequence {
		t.Fatalf(
			"dependent %q dispatch at %q sequence = %d, want after prerequisite A %q complete sequence %d",
			dependentWorkID,
			dependencyStartWorkstation,
			dependentStartSequence,
			prerequisiteAWorkID,
			prerequisiteASequence,
		)
	}
	if dependentStartSequence <= prerequisiteBSequence {
		t.Fatalf(
			"dependent %q dispatch at %q sequence = %d, want after prerequisite B %q complete sequence %d",
			dependentWorkID,
			dependencyStartWorkstation,
			dependentStartSequence,
			prerequisiteBWorkID,
			prerequisiteBSequence,
		)
	}

	if session.Runtime.Progress.Categories.Terminal != 3 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress categories = %+v, want three terminal and zero failed", session.Runtime.Progress.Categories)
	}
}

// TestDependentWorkBlockedUntilPrerequisiteArchived proves through public Work
// listings and Factory Event dispatch
// ordering that a DEPENDS_ON dependent requiring archived stays undispatched
// until the prerequisite reaches archived, then both reach archived without
// failed terminals on the happy path.
func testDependentWorkBlockedUntilPrerequisiteArchived(t *testing.T, baseURL string) {
	runDependencyTerminalHappyPath(t, baseURL, "prd-A-work-id", "PRD A")
}

// TestDependentWorkBlockedDuringPrerequisiteProcessing proves the same archived
// terminal unlock behavior when the prerequisite work identifier reflects an
// in-flight processing phase before both items reach archived.
func testDependentWorkBlockedDuringPrerequisiteProcessing(t *testing.T, baseURL string) {
	runDependencyTerminalHappyPath(t, baseURL, "prd-A-processing", "PRD A")
}

// TestDependentWorkAndPrerequisiteBothReachArchived proves both prerequisite
// and dependent Work reach the archived terminal when the dependency requires
// that upstream terminal state.
func testDependentWorkAndPrerequisiteBothReachArchived(t *testing.T, baseURL string) {
	runDependencyTerminalHappyPath(t, baseURL, "prd-A-both", "PRD A")
}

func runDependencyTerminalHappyPath(t *testing.T, baseURL, prerequisiteWorkID, prerequisitePayload string) {
	t.Helper()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_terminal"))
	dependentWorkID := prerequisiteWorkID + "-dependent"

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "prd",
		WorkID:     prerequisiteWorkID,
		Payload:    []byte(prerequisitePayload),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "prd",
		WorkID:     dependentWorkID,
		Payload:    []byte("PRD B"),
		Relations: []work.Relation{
			{
				Type:          work.RelationDependsOn,
				TargetWorkID:  prerequisiteWorkID,
				RequiredState: dependencyArchivedState,
			},
		},
	})

	session, listed, events := runSharedRelationshipFactoryToCompletion(t, baseURL, dir, 10*time.Second)

	assertDependencyWorkLocations(t, listed, map[string]int{
		support.WorkCustomerLocation("prd", dependencyArchivedState): 2,
		support.WorkCustomerLocation("prd", "init"):                  0,
		support.WorkCustomerLocation("prd", "in-review"):             0,
		support.WorkCustomerLocation("prd", "failed"):                0,
	})
	if !support.HasWorkAtCustomerState(listed, prerequisiteWorkID, support.WorkCustomerLocation("prd", dependencyArchivedState)) {
		t.Fatalf("prerequisite work %q not at archived in public listing: %#v", prerequisiteWorkID, listed)
	}
	if !support.HasWorkAtCustomerState(listed, dependentWorkID, support.WorkCustomerLocation("prd", dependencyArchivedState)) {
		t.Fatalf("dependent work %q not at archived in public listing: %#v", dependentWorkID, listed)
	}

	prerequisiteArchivedSequence, dependentExecuteSequence := archivedTerminalDispatchOrdering(
		t,
		events,
		prerequisiteWorkID,
		dependentWorkID,
	)
	if dependentExecuteSequence <= prerequisiteArchivedSequence {
		t.Fatalf(
			"dependent %q execute dispatch at sequence = %d, want after prerequisite %q archived sequence %d",
			dependentWorkID,
			dependentExecuteSequence,
			prerequisiteWorkID,
			prerequisiteArchivedSequence,
		)
	}

	if session.Runtime.Progress.Categories.Terminal != 2 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf("session progress categories = %+v, want two terminal and zero failed", session.Runtime.Progress.Categories)
	}
}

func archivedTerminalDispatchOrdering(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	prerequisiteWorkID, dependentWorkID string,
) (prerequisiteArchivedSequence, dependentExecuteSequence int) {
	t.Helper()

	prerequisiteArchivedSequence = -1
	dependentExecuteSequence = -1

	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				continue
			}
			if payload.Outcome != factoryapi.WorkOutcomeAccepted || payload.TransitionId != dependencyReviewWorkstation {
				continue
			}
			if !dispatchEventIncludesWork(event.Context.WorkIds, prerequisiteWorkID) {
				continue
			}
			prerequisiteArchivedSequence = event.Context.Sequence
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				continue
			}
			if payload.TransitionId != dependencyExecuteWorkstation {
				continue
			}
			if !dispatchRequestIncludesWork(payload, dependentWorkID) {
				continue
			}
			if prerequisiteArchivedSequence < 0 {
				t.Fatalf(
					"dependent work %q received %q dispatch before prerequisite %q reached %q",
					dependentWorkID,
					dependencyExecuteWorkstation,
					prerequisiteWorkID,
					dependencyArchivedState,
				)
			}
			if dependentExecuteSequence < 0 {
				dependentExecuteSequence = event.Context.Sequence
			}
		}
	}

	if prerequisiteArchivedSequence < 0 {
		t.Fatalf("prerequisite work %q never reached %q through public dispatch", prerequisiteWorkID, dependencyArchivedState)
	}
	if dependentExecuteSequence < 0 {
		t.Fatalf("dependent work %q never received a public %q dispatch", dependentWorkID, dependencyExecuteWorkstation)
	}
	return prerequisiteArchivedSequence, dependentExecuteSequence
}

func assertDependencyWorkLocations(t *testing.T, listed factoryapi.ListWorkResponse, wants map[string]int) {
	t.Helper()
	for location, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, location); got != want {
			t.Fatalf("CountWorkAtCustomerState(%q) = %d, want %d; listed=%#v", location, got, want, listed)
		}
	}
}

func dependencyDispatchOrdering(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	prerequisiteWorkID, dependentWorkID string,
) (prerequisiteCompleteSequence, dependentStartSequence int) {
	t.Helper()

	prerequisiteCompleteSequence = -1
	dependentStartSequence = -1

	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				continue
			}
			if payload.Outcome != factoryapi.WorkOutcomeAccepted || payload.TransitionId != dependencyFinishWorkstation {
				continue
			}
			if !dispatchEventIncludesWork(event.Context.WorkIds, prerequisiteWorkID) {
				continue
			}
			prerequisiteCompleteSequence = event.Context.Sequence
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				continue
			}
			if payload.TransitionId != dependencyStartWorkstation {
				continue
			}
			if !dispatchRequestIncludesWork(payload, dependentWorkID) {
				continue
			}
			if prerequisiteCompleteSequence < 0 {
				t.Fatalf(
					"dependent work %q received %q dispatch before prerequisite %q reached %q",
					dependentWorkID,
					dependencyStartWorkstation,
					prerequisiteWorkID,
					dependencyRequiredState,
				)
			}
			if dependentStartSequence < 0 {
				dependentStartSequence = event.Context.Sequence
			}
		}
	}

	if prerequisiteCompleteSequence < 0 {
		t.Fatalf("prerequisite work %q never reached %q through public dispatch", prerequisiteWorkID, dependencyRequiredState)
	}
	if dependentStartSequence < 0 {
		t.Fatalf("dependent work %q never received a public %q dispatch", dependentWorkID, dependencyStartWorkstation)
	}
	return prerequisiteCompleteSequence, dependentStartSequence
}

func dispatchRequestIncludesWork(payload factoryapi.DispatchRequestEventPayload, workID string) bool {
	for _, input := range payload.Inputs {
		if input.WorkId == workID {
			return true
		}
	}
	return false
}

func dispatchEventIncludesWork(workIDs *[]string, workID string) bool {
	if workIDs == nil {
		return false
	}
	for _, candidate := range *workIDs {
		if candidate == workID {
			return true
		}
	}
	return false
}

func assertNoDependentStartDispatch(t *testing.T, events []factoryapi.FactoryEvent, dependentWorkID string) {
	t.Helper()

	for _, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			continue
		}
		if payload.TransitionId != dependencyStartWorkstation {
			continue
		}
		if dispatchRequestIncludesWork(payload, dependentWorkID) {
			t.Fatalf(
				"dependent work %q received public %q dispatch after prerequisite failure at sequence %d",
				dependentWorkID,
				dependencyStartWorkstation,
				event.Context.Sequence,
			)
		}
	}
}

func startDependencyFactory(
	t *testing.T,
	dir string,
	provider providers.Service,
) (baseURL string, daemon *support.ProcessCommand) {
	t.Helper()

	server := support.NewProcessAPIServer()
	process := support.BuildProcess(t, serviceedges.Edges{
		ProviderOverride: provider,
		APIServerStarter: server.Start,
	})
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "run",
		"--dir", dir,
		"--continuously",
		"--with-server",
		"--server", "http://127.0.0.1:1",
		"--quiet",
		"--no-record",
	})
	homeDir := t.TempDir()
	inputs.Input.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)
	inputs.Input.WorkingDirectory = dir
	daemon = support.StartProcessCommand(t, process, inputs.Input)
	return server.WaitForURL(t), daemon
}

func waitForPartialFanInObservation(
	t *testing.T,
	baseURL string,
	sessionID string,
	timeout time.Duration,
	prerequisiteAWorkID, prerequisiteBWorkID, dependentWorkID string,
) (factoryapi.ListWorkResponse, []factoryapi.FactoryEvent) {
	t.Helper()

	completeLocation := support.WorkCustomerLocation("task", dependencyRequiredState)
	processingLocation := support.WorkCustomerLocation("task", "processing")
	initLocation := support.WorkCustomerLocation("task", "init")
	type observation struct {
		listed factoryapi.ListWorkResponse
		events []factoryapi.FactoryEvent
	}
	last, err := support.WaitForObservation(
		timeout,
		func() (observation, error) {
			return observation{
				listed: listRelationshipSessionWork(t, baseURL, sessionID),
				events: support.GetFactoryEventsForSessionAt(t, baseURL, sessionID),
			}, nil
		},
		func(current observation) bool {
			listed := current.listed
			aComplete := support.HasWorkAtCustomerState(listed, prerequisiteAWorkID, completeLocation)
			bComplete := support.HasWorkAtCustomerState(listed, prerequisiteBWorkID, completeLocation)
			if aComplete != bComplete &&
				!support.HasWorkAtCustomerState(listed, dependentWorkID, completeLocation) &&
				!support.HasWorkAtCustomerState(listed, dependentWorkID, processingLocation) &&
				support.HasWorkAtCustomerState(listed, dependentWorkID, initLocation) {
				return true
			}
			return false
		},
	)
	if err != nil {
		t.Fatalf(
			"timed out waiting %s for partial fan-in observation: %v; listed=%#v",
			timeout,
			err,
			last.listed,
		)
	}
	return last.listed, last.events
}

func getRelationshipSession(t testing.TB, baseURL, sessionID string) factoryapi.FactorySession {
	t.Helper()
	response := support.GetJSON[factoryapi.FactorySessionGetResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID),
	)
	session, err := response.AsFactorySession()
	if err != nil {
		t.Fatalf("decode Factory Session %q: %v", sessionID, err)
	}
	return session
}

func listRelationshipSessionWork(t testing.TB, baseURL, sessionID string) factoryapi.ListWorkResponse {
	t.Helper()
	return support.GetJSON[factoryapi.ListWorkResponse](
		t,
		strings.TrimSuffix(baseURL, "/")+"/factory-sessions/"+url.PathEscape(sessionID)+"/work",
	)
}

func assertFanInBlockedAfterPartialPrerequisites(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	events []factoryapi.FactoryEvent,
	prerequisiteAWorkID, prerequisiteBWorkID, dependentWorkID string,
) {
	t.Helper()

	completeLocation := support.WorkCustomerLocation("task", dependencyRequiredState)
	processingLocation := support.WorkCustomerLocation("task", "processing")
	initLocation := support.WorkCustomerLocation("task", "init")

	aComplete := support.HasWorkAtCustomerState(listed, prerequisiteAWorkID, completeLocation)
	bComplete := support.HasWorkAtCustomerState(listed, prerequisiteBWorkID, completeLocation)
	if aComplete == bComplete {
		t.Fatalf(
			"expected exactly one prerequisite complete during partial fan-in; A complete=%t B complete=%t; listed=%#v",
			aComplete,
			bComplete,
			listed,
		)
	}

	if support.HasWorkAtCustomerState(listed, dependentWorkID, completeLocation) {
		t.Fatalf("dependent %q reached %q before every prerequisite completed: %#v", dependentWorkID, dependencyRequiredState, listed)
	}
	if support.HasWorkAtCustomerState(listed, dependentWorkID, processingLocation) {
		t.Fatalf("dependent %q reached processing before every prerequisite completed: %#v", dependentWorkID, listed)
	}
	if !support.HasWorkAtCustomerState(listed, dependentWorkID, initLocation) {
		t.Fatalf("dependent %q not at init while prerequisites are still releasing: %#v", dependentWorkID, listed)
	}

	assertNoDependentStartDispatch(t, events, dependentWorkID)
}

func fanInDispatchOrdering(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	prerequisiteAWorkID, prerequisiteBWorkID, dependentWorkID string,
) (prerequisiteASequence, prerequisiteBSequence, dependentStartSequence int) {
	t.Helper()

	prerequisiteASequence = -1
	prerequisiteBSequence = -1
	dependentStartSequence = -1

	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				continue
			}
			if payload.Outcome != factoryapi.WorkOutcomeAccepted || payload.TransitionId != dependencyFinishWorkstation {
				continue
			}
			if dispatchEventIncludesWork(event.Context.WorkIds, prerequisiteAWorkID) {
				prerequisiteASequence = event.Context.Sequence
			}
			if dispatchEventIncludesWork(event.Context.WorkIds, prerequisiteBWorkID) {
				prerequisiteBSequence = event.Context.Sequence
			}
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				continue
			}
			if payload.TransitionId != dependencyStartWorkstation {
				continue
			}
			if !dispatchRequestIncludesWork(payload, dependentWorkID) {
				continue
			}
			if prerequisiteASequence < 0 || prerequisiteBSequence < 0 {
				t.Fatalf(
					"dependent work %q received %q dispatch before both prerequisites reached %q",
					dependentWorkID,
					dependencyStartWorkstation,
					dependencyRequiredState,
				)
			}
			if dependentStartSequence < 0 {
				dependentStartSequence = event.Context.Sequence
			}
		}
	}

	if prerequisiteASequence < 0 {
		t.Fatalf("prerequisite A %q never reached %q through public dispatch", prerequisiteAWorkID, dependencyRequiredState)
	}
	if prerequisiteBSequence < 0 {
		t.Fatalf("prerequisite B %q never reached %q through public dispatch", prerequisiteBWorkID, dependencyRequiredState)
	}
	if dependentStartSequence < 0 {
		t.Fatalf("dependent work %q never received a public %q dispatch", dependentWorkID, dependencyStartWorkstation)
	}
	return prerequisiteASequence, prerequisiteBSequence, dependentStartSequence
}
