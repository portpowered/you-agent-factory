package relationships

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/builtcliacceptance"
	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/work"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	parentChildLineageRequestID = "request-parent-child-lineage"
	parentChildLineageParentID  = "parent-work-lineage-id"
	parentChildLineageChildID   = "child-work-lineage-id"
	parentChildLineageParent    = "parent"
	parentChildLineageChild     = "child"
	parentChildLineageWorkType  = "task"

	parentChildFailureRequestID  = "request-parent-child-failure"
	parentChildFailureParentID   = "parent-story-set-id"
	parentChildFailureChildID    = "child-story-id"
	parentChildFailureParent     = "story-set"
	parentChildFailureChild      = "story-child"
	parentChildFailureParentType = "story-set"
	parentChildFailureChildType  = "story"
)

// TestParentChildLineageSurvivesDispatchAndReplay proves through public CLI
// batch submission, Work inspection, provider dispatch observations, and
// retained Factory Event history that PARENT_CHILD lineage on a child work item
// remains observable after the child dispatches and after event-history
// reconstruction from the same session.
// Isolation rationale (REL-006): shared complete-boundary execution would
// replace the built `you` executable -> server -> provider process boundary
// and invalidate the retained-history and replay proof.
func TestParentChildLineageSurvivesDispatchAndReplay(t *testing.T) {
	t.Parallel()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "dependency_tracking_dir"))

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "COMPLETE"},
		workerexecution.InferenceResponse{Content: "COMPLETE"},
		workerexecution.InferenceResponse{Content: "COMPLETE"},
		workerexecution.InferenceResponse{Content: "COMPLETE"},
	)

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	defer server.Stop(t)

	baseURL := server.URL()
	processHarness := newParentChildRootProcessHarness(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	batchJSON := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workId":%q,"workTypeName":%q,"payload":{"role":"parent"}},{"name":%q,"workId":%q,"workTypeName":%q,"payload":{"role":"child"}}],"relations":[{"type":"PARENT_CHILD","sourceWorkName":%q,"targetWorkName":%q}]}`,
		parentChildLineageRequestID,
		parentChildLineageParent,
		parentChildLineageParentID,
		parentChildLineageWorkType,
		parentChildLineageChild,
		parentChildLineageChildID,
		parentChildLineageWorkType,
		parentChildLineageChild,
		parentChildLineageParent,
	)
	terminalObservation := support.OpenDefaultSessionTerminalFactoryEventObservation(t, baseURL)
	submitOut, err := runParentChildCLI(ctx, processHarness, dir, baseURL,
		"--json",
		"submit", "batch",
		batchJSON,
	)
	if err != nil {
		t.Fatalf("you submit batch: %v\noutput:\n%s", err, submitOut)
	}
	assertParentChildBatchSubmitAcknowledgment(t, submitOut, parentChildLineageRequestID)

	support.WaitForSessionWorkIDsAtStateFromFactoryEvents(
		t,
		baseURL,
		"~default",
		[]string{parentChildLineageParentID, parentChildLineageChildID},
		"complete",
		15*time.Second,
	)

	listed := support.ListDefaultSessionWork(t, baseURL)
	assertParentChildLineageInWorkListing(t, listed, parentChildLineageChildID, parentChildLineageParentID)

	events := server.GetFactoryEvents(t)
	assertParentChildLineageInFactoryEvents(
		t,
		events,
		parentChildLineageRequestID,
		parentChildLineageChild,
		parentChildLineageParent,
		parentChildLineageParentID,
	)

	reconstructed := support.GetFactoryEventsAt(t, baseURL)
	if len(reconstructed) != len(events) {
		t.Fatalf(
			"reconstructed Factory Event count = %d, want %d from first retained-history read",
			len(reconstructed),
			len(events),
		)
	}
	assertParentChildLineageInFactoryEvents(
		t,
		reconstructed,
		parentChildLineageRequestID,
		parentChildLineageChild,
		parentChildLineageParent,
		parentChildLineageParentID,
	)

	assertParentChildLineageOnChildDispatch(t, provider, parentChildLineageChildID, parentChildLineageParentID)

	shown, err := runParentChildWorkShowCLIJSON(
		t,
		ctx,
		processHarness,
		dir,
		baseURL,
		parentChildLineageChildID,
	)
	if err != nil {
		t.Fatalf("you work show %s: %v", parentChildLineageChildID, err)
	}
	assertParentChildRelationOnWork(t, shown, parentChildLineageParentID)
	server.Stop(t)
	terminalObservation.Wait(15 * time.Second)
}

// TestChildFailureProjectsToDocumentedParentView proves through public CLI
// batch submission and Work inspection that when a PARENT_CHILD child reaches a
// failed terminal state, the documented parent-aware failure projection
// (ANY_CHILD_FAILED) surfaces on the parent Work as a failed customer-visible
// state while PARENT_CHILD lineage remains observable on the child.
// Isolation rationale (REL-007): shared complete-boundary execution would
// collapse the built-CLI child-failure path and isolate neither server nor
// provider state, invalidating the public CLI, event-order, and parent-
// projection proof.
func TestChildFailureProjectsToDocumentedParentView(t *testing.T) {
	t.Parallel()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "submitted_parent_child_filewatcher"))

	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"story-worker": {
			{Error: errors.New("child story processing failed")},
		},
		"story-set-failure-handler": {
			{Content: "Story set failed. COMPLETE"},
		},
	})

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges: serviceedges.Edges{
			ProviderOverride: provider,
		},
	})
	defer server.Stop(t)

	baseURL := server.URL()
	processHarness := newParentChildRootProcessHarness(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	batchJSON := fmt.Sprintf(
		`{"requestId":%q,"type":"FACTORY_REQUEST_BATCH","works":[{"name":%q,"workId":%q,"workTypeName":%q,"state":"waiting","payload":{"title":"release story set"}},{"name":%q,"workId":%q,"workTypeName":%q,"payload":{"title":"failing child story"}}],"relations":[{"type":"PARENT_CHILD","sourceWorkName":%q,"targetWorkName":%q}]}`,
		parentChildFailureRequestID,
		parentChildFailureParent,
		parentChildFailureParentID,
		parentChildFailureParentType,
		parentChildFailureChild,
		parentChildFailureChildID,
		parentChildFailureChildType,
		parentChildFailureChild,
		parentChildFailureParent,
	)
	terminalObservation := support.OpenDefaultSessionTerminalFactoryEventObservation(t, baseURL)
	submitOut, err := runParentChildCLI(ctx, processHarness, dir, baseURL,
		"--json",
		"submit", "batch",
		batchJSON,
	)
	if err != nil {
		t.Fatalf("you submit batch: %v\noutput:\n%s", err, submitOut)
	}
	assertParentChildFailureBatchSubmitAcknowledgment(t, submitOut, parentChildFailureRequestID)

	support.WaitForSessionWorkIDsAtStateFromFactoryEvents(
		t,
		baseURL,
		"~default",
		[]string{parentChildFailureParentID, parentChildFailureChildID},
		"failed",
		15*time.Second,
	)

	listed := support.ListDefaultSessionWork(t, baseURL)
	assertParentChildFailureProjectionInWorkListing(t, listed, parentChildFailureParentID, parentChildFailureChildID)

	parentShown, err := runParentChildWorkShowCLIJSON(
		t,
		ctx,
		processHarness,
		dir,
		baseURL,
		parentChildFailureParentID,
	)
	if err != nil {
		t.Fatalf("you work show %s: %v", parentChildFailureParentID, err)
	}
	assertParentChildFailureOnWork(t, parentShown, parentChildFailureParentType, "failed")

	childShown, err := runParentChildWorkShowCLIJSON(
		t,
		ctx,
		processHarness,
		dir,
		baseURL,
		parentChildFailureChildID,
	)
	if err != nil {
		t.Fatalf("you work show %s: %v", parentChildFailureChildID, err)
	}
	assertParentChildFailureOnWork(t, childShown, parentChildFailureChildType, "failed")
	assertParentChildRelationOnWork(t, childShown, parentChildFailureParentID)

	events := server.GetFactoryEvents(t)
	assertParentChildFailureProjectionInFactoryEvents(t, events, parentChildFailureRequestID, parentChildFailureChildID)
	server.Stop(t)
	terminalObservation.Wait(15 * time.Second)
}

// testDispatchPreservesSubmittedWorkPayloadTagsAndType proves through provider
// dispatch observations that the executor input preserves the submitted payload,
// tags, and work type identity for dispatched Work.
func testDispatchPreservesSubmittedWorkPayloadTagsAndType(t *testing.T, host *sharedRelationshipHost) {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

	payload := []byte(`{"feature": "dark mode", "priority": "high"}`)
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "code-change",
		Payload:    payload,
		Tags:       map[string]string{"team": "frontend", "sprint": "42"},
	})

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "COMPLETE"},
		workerexecution.InferenceResponse{Content: "COMPLETE"},
	)
	host.provider.register(t, dir, provider)
	runSharedRelationshipFactoryToCompletion(t, host, dir, 10*time.Second)

	sweCalls := support.ProviderCallsForWorker(provider, "swe")
	if len(sweCalls) != 1 {
		t.Fatalf("swe dispatch count = %d, want 1", len(sweCalls))
	}

	dispatch := sweCalls[0]
	if !bytes.Equal(support.FirstInputPayload(dispatch.InputTokens), payload) {
		t.Fatalf(
			"dispatch payload = %q, want %q",
			support.FirstInputPayload(dispatch.InputTokens),
			payload,
		)
	}
	tags := support.FirstInputTags(dispatch.InputTokens)
	if tags["team"] != "frontend" {
		t.Fatalf("dispatch tag team = %q, want frontend", tags["team"])
	}
	if tags["sprint"] != "42" {
		t.Fatalf("dispatch tag sprint = %q, want 42", tags["sprint"])
	}
	if got := support.FirstInputWorkID(dispatch.InputTokens); got == "" {
		t.Fatal("dispatch Work ID missing, want submitted work identity")
	}
}

// testRejectionFeedbackSurfacesOnExecutorRetry proves reviewer rejection feedback
// is attached to the next executor dispatch payload while the first dispatch
// remains free of rejection-feedback tags.
func testRejectionFeedbackSurfacesOnExecutorRetry(t *testing.T, host *sharedRelationshipHost) {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

	testutil.WriteSeedFile(t, dir, "code-change", []byte(`{"feature": "auth"}`))

	provider := testutil.NewMockWorkerMapProvider(map[string][]workerexecution.InferenceResponse{
		"swe":      {support.AcceptedProviderResponse(), support.AcceptedProviderResponse()},
		"reviewer": {{Content: "needs unit tests"}, support.AcceptedProviderResponse()},
	})
	host.provider.register(t, dir, provider)
	runSharedRelationshipFactoryToCompletion(t, host, dir, 10*time.Second)

	if got := provider.CallCount("swe"); got != 2 {
		t.Fatalf("swe dispatch count = %d, want 2", got)
	}

	calls := provider.Calls("swe")
	firstTags := support.FirstInputTags(calls[0].InputTokens)
	if _, ok := firstTags["_rejection_feedback"]; ok {
		t.Fatal("first swe dispatch should not include _rejection_feedback tag")
	}

	secondPayload := support.FirstInputPayload(calls[1].InputTokens)
	if !bytes.Contains(secondPayload, []byte("needs unit tests")) {
		t.Fatalf("retry dispatch payload = %q, want reviewer feedback", secondPayload)
	}
}

// testParentAndDependsOnLineageSurviveOnChildDispatch proves PARENT_CHILD and
// DEPENDS_ON relations remain observable on the child dispatch token after both
// prerequisite completion and child submission.
func testParentAndDependsOnLineageSurviveOnChildDispatch(t *testing.T, host *sharedRelationshipHost) {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "code_review"))

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID:  "code-change",
		WorkID:      "prereq-work-99",
		TargetState: "complete",
		Payload:     []byte(`{"feature": "prerequisite"}`),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "code-change",
		WorkID:     "child-work-1",
		Payload:    []byte(`{"feature": "login page"}`),
		Relations: []work.Relation{
			{
				Type:         work.RelationParentChild,
				TargetWorkID: "parent-prd-42",
			},
			{
				Type:          work.RelationDependsOn,
				TargetWorkID:  "prereq-work-99",
				RequiredState: "complete",
			},
		},
	})

	provider := testutil.NewMockProvider(
		workerexecution.InferenceResponse{Content: "COMPLETE"},
		workerexecution.InferenceResponse{Content: "COMPLETE"},
	)
	host.provider.register(t, dir, provider)
	runSharedRelationshipFactoryToCompletion(t, host, dir, 10*time.Second)

	sweCalls := support.ProviderCallsForWorker(provider, "swe")
	if len(sweCalls) != 1 {
		t.Fatalf("swe dispatch count = %d, want 1", len(sweCalls))
	}

	token := firstParentChildDispatchToken(sweCalls[0].InputTokens)
	if token.Color.WorkID != "child-work-1" {
		t.Fatalf("dispatch Work ID = %q, want child-work-1", token.Color.WorkID)
	}
	if len(token.Color.Relations) != 2 {
		t.Fatalf("dispatch relation count = %d, want 2", len(token.Color.Relations))
	}

	foundParent := false
	foundDependsOn := false
	for _, rel := range token.Color.Relations {
		switch rel.Type {
		case work.RelationParentChild:
			foundParent = true
			if rel.TargetWorkID != "parent-prd-42" {
				t.Fatalf("parent relation target = %q, want parent-prd-42", rel.TargetWorkID)
			}
		case work.RelationDependsOn:
			foundDependsOn = true
			if rel.TargetWorkID != "prereq-work-99" {
				t.Fatalf("depends-on target = %q, want prereq-work-99", rel.TargetWorkID)
			}
			if rel.RequiredState != "complete" {
				t.Fatalf("depends-on required state = %q, want complete", rel.RequiredState)
			}
		}
	}
	if !foundParent {
		t.Fatal("PARENT_CHILD relation missing from child dispatch token")
	}
	if !foundDependsOn {
		t.Fatal("DEPENDS_ON relation missing from child dispatch token")
	}
}

// testDependentWorkFailsWhenDirectPrerequisiteFails proves through public Work
// listings and captured provider command invocations that when a DEPENDS_ON
// prerequisite fails before reaching its required state, the dependent Work
// projects to failed without reaching a successful terminal state.
func testDependentWorkFailsWhenDirectPrerequisiteFails(t *testing.T, host *sharedRelationshipHost) {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "cascading_failure"))

	parentWorkID := "parent-work-id"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID:  "task",
		WorkID:      parentWorkID,
		TargetState: "processing",
		Payload:     []byte("parent"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("child"),
		Relations: []work.Relation{
			{Type: work.RelationDependsOn, TargetWorkID: parentWorkID, RequiredState: "complete"},
		},
	})

	runner := testutil.NewProviderCommandRunner(
		cascadingFailureProviderError("upstream service down"),
	)
	host.provider.registerCommandRunner(t, dir, runner)
	_, listed, _ := runSharedRelationshipFactoryToCompletionAndClose(t, host, dir, 10*time.Second)

	assertCascadingFailurePlaces(t, listed, map[string]int{
		"task:failed":     2,
		"task:init":       0,
		"task:processing": 0,
		"task:complete":   0,
	})
	assertFailedDependsOnWork(t, listed, parentWorkID)
	if runner.CallCount() != 1 {
		t.Fatalf(
			"provider command runner calls = %d, want 1 finisher failure for prerequisite already at processing",
			runner.CallCount(),
		)
	}
	assertCascadingFailureProviderRequests(t, runner)
}

// testTransitiveDependencyFailureCascadesToFailedTerminals proves through public
// Work listings and captured provider command invocations that a
// parent→child→grandchild DEPENDS_ON chain reaches failed terminals on every
// related Work item when the upstream finisher path fails.
func testTransitiveDependencyFailureCascadesToFailedTerminals(t *testing.T, host *sharedRelationshipHost) {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "cascading_failure"))

	pWorkID := "P-work-id"
	c1WorkID := "C1-work-id"

	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     pWorkID,
		Payload:    []byte("P"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     c1WorkID,
		Payload:    []byte("C1"),
		Relations: []work.Relation{
			{Type: work.RelationDependsOn, TargetWorkID: pWorkID, RequiredState: "complete"},
		},
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("C2"),
		Relations: []work.Relation{
			{Type: work.RelationDependsOn, TargetWorkID: c1WorkID, RequiredState: "complete"},
		},
	})

	runner := testutil.NewProviderCommandRunner(
		cascadingFailureProviderSuccess(),
		cascadingFailureProviderError("crash"),
		cascadingFailureProviderSuccess(),
		cascadingFailureProviderError("crash"),
		cascadingFailureProviderSuccess(),
		cascadingFailureProviderError("crash"),
	)
	host.provider.registerCommandRunner(t, dir, runner)
	_, listed, _ := runSharedRelationshipFactoryToCompletionAndClose(t, host, dir, 10*time.Second)

	assertCascadingFailurePlaces(t, listed, map[string]int{
		"task:failed":     3,
		"task:init":       0,
		"task:processing": 0,
		"task:complete":   0,
	})
	assertFailedDependsOnWork(t, listed, pWorkID)
	assertFailedDependsOnWork(t, listed, c1WorkID)
	if runner.CallCount() != 2 {
		t.Fatalf(
			"provider command runner calls = %d, want 2 starter and finisher invocations before dependents cascade to failed",
			runner.CallCount(),
		)
	}
	assertCascadingFailureProviderRequests(t, runner)
}

// testCompletedPrerequisiteIsNotCascadedWhenDependentFails proves through public
// Work listings and captured provider command invocations that a prerequisite
// Work already at complete is left unchanged when a later dependent Work fails.
func testCompletedPrerequisiteIsNotCascadedWhenDependentFails(t *testing.T, host *sharedRelationshipHost) {
	t.Helper()
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "cascading_failure"))

	aWorkID := "A-work-id"
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		WorkID:     aWorkID,
		Payload:    []byte("A"),
	})
	testutil.WriteSeedRequest(t, dir, work.SubmitRequest{
		WorkTypeID: "task",
		Payload:    []byte("B"),
		Relations: []work.Relation{
			{Type: work.RelationDependsOn, TargetWorkID: aWorkID, RequiredState: "complete"},
		},
	})

	runner := testutil.NewProviderCommandRunner(
		cascadingFailureProviderSuccess(),
		cascadingFailureProviderSuccess(),
		cascadingFailureProviderSuccess(),
		cascadingFailureProviderError("oops"),
	)
	host.provider.registerCommandRunner(t, dir, runner)
	_, listed, _ := runSharedRelationshipFactoryToCompletionAndClose(t, host, dir, 10*time.Second)

	assertCascadingFailurePlaces(t, listed, map[string]int{
		"task:complete": 1,
		"task:failed":   1,
	})
	if !support.HasWorkAtCustomerState(listed, aWorkID, support.WorkCustomerLocation("task", "complete")) {
		t.Fatalf("prerequisite work %q not at complete after dependent failure: %#v", aWorkID, listed.Results)
	}
	if runner.CallCount() != 4 {
		t.Fatalf(
			"provider command runner calls = %d, want 4 starter and finisher invocations for prerequisite then dependent",
			runner.CallCount(),
		)
	}
	assertCascadingFailureProviderRequests(t, runner)
}

func assertCascadingFailureProviderRequests(t *testing.T, runner *testutil.ProviderCommandRunner) {
	t.Helper()

	for index, request := range runner.Requests() {
		if strings.TrimSpace(request.Command) == "" {
			t.Fatalf("provider command request %d missing command: %#v", index, request)
		}
		if len(request.Args) == 0 {
			t.Fatalf("provider command request %d missing args: %#v", index, request)
		}
	}
}

func cascadingFailureProviderSuccess() platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("COMPLETE")}
}

func cascadingFailureProviderError(message string) platformprocess.CommandResult {
	return platformprocess.CommandResult{ExitCode: 1, Stderr: []byte(message)}
}

func assertCascadingFailurePlaces(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	wants map[string]int,
) {
	t.Helper()
	for placeID, want := range wants {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Fatalf("%s work count = %d, want %d; listed=%#v", placeID, got, want, listed.Results)
		}
	}
}

func assertFailedDependsOnWork(t *testing.T, listed factoryapi.ListWorkResponse, targetWorkID string) {
	t.Helper()
	for _, item := range listed.Results {
		if item.State == nil || item.State.Name != "failed" || item.Relations == nil {
			continue
		}
		for _, relation := range *item.Relations {
			if relation.Type == factoryapi.RelationTypeDependsOn &&
				relation.TargetWorkId != nil &&
				*relation.TargetWorkId == targetWorkID {
				return
			}
		}
	}
	t.Fatalf("listed failed Work missing DEPENDS_ON dependency on %q: %#v", targetWorkID, listed.Results)
}

func assertParentChildBatchSubmitAcknowledgment(t *testing.T, output []byte, requestID string) {
	t.Helper()

	text := string(output)
	for _, marker := range []string{
		`"requestId":` + jsonStringLiteral(requestID),
		`"traceId":`,
		`"workCount":2`,
		parentChildLineageParent,
		parentChildLineageChild,
		parentChildLineageWorkType,
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("submit batch output missing %q:\n%s", marker, text)
		}
	}
}

func assertParentChildFailureBatchSubmitAcknowledgment(t *testing.T, output []byte, requestID string) {
	t.Helper()

	text := string(output)
	for _, marker := range []string{
		`"requestId":` + jsonStringLiteral(requestID),
		`"traceId":`,
		`"workCount":2`,
		parentChildFailureParent,
		parentChildFailureChild,
		parentChildFailureParentType,
		parentChildFailureChildType,
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("submit batch output missing %q:\n%s", marker, text)
		}
	}
}

func assertParentChildFailureProjectionInWorkListing(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	parentWorkID, childWorkID string,
) {
	t.Helper()

	if !support.HasWorkAtCustomerState(
		listed,
		parentWorkID,
		support.WorkCustomerLocation(parentChildFailureParentType, "failed"),
	) {
		t.Fatalf(
			"public work list missing parent %q at %s:failed: %#v",
			parentWorkID,
			parentChildFailureParentType,
			listed.Results,
		)
	}
	if !support.HasWorkAtCustomerState(
		listed,
		childWorkID,
		support.WorkCustomerLocation(parentChildFailureChildType, "failed"),
	) {
		t.Fatalf(
			"public work list missing child %q at %s:failed: %#v",
			childWorkID,
			parentChildFailureChildType,
			listed.Results,
		)
	}
	if got := support.CountWorkAtCustomerState(
		listed,
		support.WorkCustomerLocation(parentChildFailureParentType, "waiting"),
	); got != 0 {
		t.Fatalf("parent still at waiting = %d, want 0 after ANY_CHILD_FAILED projection", got)
	}
}

func assertParentChildFailureOnWork(t *testing.T, item factoryapi.Work, workTypeName, stateName string) {
	t.Helper()

	if item.WorkTypeName == nil || *item.WorkTypeName != workTypeName {
		t.Fatalf("work type = %q, want %q: %#v", support.StringPointerValue(item.WorkTypeName), workTypeName, item)
	}
	if item.State == nil || item.State.Name != stateName {
		t.Fatalf("work state = %#v, want %q for %q", item.State, stateName, workTypeName)
	}
}

func assertParentChildFailureProjectionInFactoryEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	_ string,
	_ string,
) {
	t.Helper()

	childFailureIndex := -1
	parentFailureIndex := -1

	for i, event := range events {
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode DISPATCH_RESPONSE event: %v", err)
		}
		switch payload.TransitionId {
		case "process-story":
			if payload.Outcome == factoryapi.WorkOutcomeFailed {
				childFailureIndex = i
			}
		case "fail-story-set-from-child":
			if payload.Outcome == factoryapi.WorkOutcomeAccepted {
				parentFailureIndex = i
			}
		}
	}

	if childFailureIndex == -1 {
		t.Fatal("Factory Event history missing failed child dispatch completion for process-story")
	}
	if parentFailureIndex == -1 {
		t.Fatal("Factory Event history missing parent ANY_CHILD_FAILED dispatch completion for fail-story-set-from-child")
	}
	if parentFailureIndex <= childFailureIndex {
		t.Fatalf(
			"parent failure dispatch index %d should be after child failure index %d",
			parentFailureIndex,
			childFailureIndex,
		)
	}
}

func assertParentChildLineageInWorkListing(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	childWorkID, parentWorkID string,
) {
	t.Helper()

	child, ok := findListedWorkByID(listed, childWorkID)
	if !ok {
		t.Fatalf("public work list missing child %q: %#v", childWorkID, listed.Results)
	}
	assertParentChildRelationOnWork(t, child, parentWorkID)
}

func assertParentChildRelationOnWork(t *testing.T, item factoryapi.Work, parentWorkID string) {
	t.Helper()

	if item.Relations == nil || len(*item.Relations) == 0 {
		t.Fatalf("work %q missing relations in public listing/show: %#v", support.StringPointerValue(item.WorkId), item)
	}
	for _, relation := range *item.Relations {
		if relation.Type != factoryapi.RelationTypeParentChild {
			continue
		}
		if relation.TargetWorkId != nil && *relation.TargetWorkId == parentWorkID {
			return
		}
	}
	t.Fatalf(
		"work %q missing PARENT_CHILD relation to parent %q: relations=%#v",
		support.StringPointerValue(item.WorkId),
		parentWorkID,
		*item.Relations,
	)
}

func assertParentChildLineageInFactoryEvents(
	t *testing.T,
	events []factoryapi.FactoryEvent,
	requestID, childWorkName, parentWorkName, parentWorkID string,
) {
	t.Helper()

	foundWorkRequest := false
	foundRelationshipChange := false

	for _, event := range events {
		if support.StringPointerValue(event.Context.RequestId) != requestID {
			continue
		}
		switch event.Type {
		case factoryapi.FactoryEventTypeWorkRequest:
			foundWorkRequest = true
			payload, err := event.Payload.AsWorkRequestEventPayload()
			if err != nil {
				t.Fatalf("decode WORK_REQUEST event: %v", err)
			}
			if payload.Relations == nil {
				t.Fatalf("WORK_REQUEST payload missing relations: %#v", payload)
			}
			if !factoryEventRelationsIncludeParentChild(payload.Relations, childWorkName, parentWorkName) {
				t.Fatalf(
					"WORK_REQUEST relations = %#v, want PARENT_CHILD from %q to %q",
					payload.Relations,
					childWorkName,
					parentWorkName,
				)
			}
		case factoryapi.FactoryEventTypeRelationshipChangeRequest:
			foundRelationshipChange = true
			payload, err := event.Payload.AsRelationshipChangeRequestEventPayload()
			if err != nil {
				t.Fatalf("decode RELATIONSHIP_CHANGE_REQUEST event: %v", err)
			}
			if payload.Relation.Type != factoryapi.RelationTypeParentChild {
				t.Fatalf("relationship change type = %q, want PARENT_CHILD", payload.Relation.Type)
			}
			if payload.Relation.SourceWorkName != childWorkName || payload.Relation.TargetWorkName != parentWorkName {
				t.Fatalf(
					"relationship change = %#v, want %q PARENT_CHILD %q",
					payload.Relation,
					childWorkName,
					parentWorkName,
				)
			}
			if support.StringPointerValue(payload.Relation.TargetWorkId) != parentWorkID {
				t.Fatalf(
					"relationship target work id = %q, want %q",
					support.StringPointerValue(payload.Relation.TargetWorkId),
					parentWorkID,
				)
			}
		}
	}

	if !foundWorkRequest {
		t.Fatalf("Factory Event history missing WORK_REQUEST for request %q", requestID)
	}
	if !foundRelationshipChange {
		t.Fatalf("Factory Event history missing RELATIONSHIP_CHANGE_REQUEST for request %q", requestID)
	}
}

func factoryEventRelationsIncludeParentChild(
	relations *[]factoryapi.Relation,
	childWorkName, parentWorkName string,
) bool {
	if relations == nil {
		return false
	}
	for _, relation := range *relations {
		if relation.Type == factoryapi.RelationTypeParentChild &&
			relation.SourceWorkName == childWorkName &&
			relation.TargetWorkName == parentWorkName {
			return true
		}
	}
	return false
}

func assertParentChildLineageOnChildDispatch(
	t *testing.T,
	provider *testutil.MockProvider,
	childWorkID, parentWorkID string,
) {
	t.Helper()

	for _, call := range provider.Calls() {
		token := firstParentChildDispatchToken(call.InputTokens)
		if token.Color.WorkID != childWorkID {
			continue
		}
		for _, relation := range token.Color.Relations {
			if relation.Type == work.RelationParentChild && relation.TargetWorkID == parentWorkID {
				return
			}
		}
		t.Fatalf(
			"child dispatch token relations = %#v, want PARENT_CHILD target %q",
			token.Color.Relations,
			parentWorkID,
		)
	}
	t.Fatalf("provider dispatch history missing child work %q", childWorkID)
}

func firstParentChildDispatchToken(rawTokens any) workerexecution.Token {
	switch tokens := rawTokens.(type) {
	case []any:
		if len(tokens) == 0 {
			return workerexecution.Token{}
		}
		token, ok := tokens[0].(workerexecution.Token)
		if !ok {
			return workerexecution.Token{}
		}
		return token
	case []workerexecution.Token:
		if len(tokens) == 0 {
			return workerexecution.Token{}
		}
		return tokens[0]
	default:
		return workerexecution.Token{}
	}
}

func findListedWorkByID(listed factoryapi.ListWorkResponse, workID string) (factoryapi.Work, bool) {
	for _, item := range listed.Results {
		if support.StringPointerValue(item.WorkId) == workID {
			return item, true
		}
	}
	return factoryapi.Work{}, false
}

func newParentChildRootProcessHarness(t *testing.T) *builtcliacceptance.Harness {
	t.Helper()
	return builtcliacceptance.NewHarness(t, testutil.MustRepoRoot(t))
}

func runParentChildCLI(
	ctx context.Context,
	processHarness *builtcliacceptance.Harness,
	workingDir string,
	serverURL string,
	args ...string,
) ([]byte, error) {
	cmdArgs := append([]string{"--server", serverURL}, args...)
	cmd := processHarness.CommandContext(ctx, cmdArgs...)
	cmd.Dir = workingDir
	return cmd.CombinedOutput()
}

func runParentChildWorkShowCLIJSON(
	t *testing.T,
	ctx context.Context,
	processHarness *builtcliacceptance.Harness,
	workingDir string,
	serverURL string,
	workID string,
) (factoryapi.Work, error) {
	t.Helper()

	output, err := runParentChildCLI(ctx, processHarness, workingDir, serverURL,
		"--json",
		"work", "show", workID,
	)
	if err != nil {
		return factoryapi.Work{}, err
	}
	var shown factoryapi.Work
	if err := json.Unmarshal(bytesTrimSpace(output), &shown); err != nil {
		t.Fatalf("decode work show JSON: %v\noutput:\n%s", err, output)
	}
	return shown, nil
}

func jsonStringLiteral(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func bytesTrimSpace(value []byte) []byte {
	return []byte(strings.TrimSpace(string(value)))
}
