package recovery

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// TestFailedCascadeCanBeRecoveredByPublicWorkMove proves a failed DEPENDS_ON
// cascade can be repaired through the public Work move API so parent and child
// work resume and reach successful terminal customer-visible states.
func TestFailedCascadeCanBeRecoveredByPublicWorkMove(t *testing.T) {
	if testing.Short() {
		t.Skip("slow work recovery functional test")
	}

	const (
		parentWorkID = "recovery-parent-work-id"
		childWorkID  = "recovery-child-work-id"
		traceID      = "trace-cascade-recovery-move"
		requestID    = "request-cascade-recovery-move"
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "cascading_failure"))
	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"starter": {
			{Content: "COMPLETE"},
			{Content: "COMPLETE"},
		},
		"finisher": {
			{Error: errors.New("upstream service down")},
			{Content: "COMPLETE"},
			{Content: "COMPLETE"},
		},
	})
	server := startRecoveryAPIServer(t, dir, provider)
	defer server.Stop(t)

	requiredState := "complete"
	workTypeName := "task"
	support.UpsertDefaultSessionWorkRequest(t, server.URL(), factoryapi.WorkRequest{
		RequestId: requestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{
			{
				Name:         "parent",
				WorkId:       stringPtr(parentWorkID),
				WorkTypeName: &workTypeName,
				TraceId:      stringPtr(traceID),
				Payload:      map[string]string{"role": "parent"},
			},
			{
				Name:         "child",
				WorkId:       stringPtr(childWorkID),
				WorkTypeName: &workTypeName,
				TraceId:      stringPtr(traceID),
				Payload:      map[string]string{"role": "child"},
			},
		},
		Relations: &[]factoryapi.Relation{{
			Type:           factoryapi.RelationTypeDependsOn,
			SourceWorkName: "child",
			TargetWorkName: "parent",
			RequiredState:  &requiredState,
		}},
	})

	waitForWorkIDsAtState(t, server.URL(), []string{parentWorkID, childWorkID}, "failed", 15*time.Second)

	parentMoved := postMoveWork(t, server.URL(), parentWorkID, "processing")
	if workStateName(parentMoved.State) != "processing" {
		t.Fatalf("parent move response = %#v, want processing", parentMoved)
	}

	childMoved := postMoveWork(t, server.URL(), childWorkID, "init")
	if workStateName(childMoved.State) != "init" {
		t.Fatalf("child move response = %#v, want init", childMoved)
	}

	completed := waitForWorkIDsComplete(t, server.URL(), []string{childWorkID}, 15*time.Second)
	if len(completed) != 1 || workStateName(completed[0].State) != "complete" {
		t.Fatalf("child completion = %#v, want complete", completed)
	}

	listed := support.ListDefaultSessionWork(t, server.URL())
	if !support.HasWorkAtCustomerState(listed, childWorkID, support.WorkCustomerLocation("task", "complete")) {
		t.Fatalf("work listing = %#v, want child at task:complete", listed.Results)
	}
	if !support.HasWorkAtCustomerState(listed, parentWorkID, support.WorkCustomerLocation("task", "complete")) {
		t.Fatalf("work listing = %#v, want parent at task:complete", listed.Results)
	}
}

// TestTerminalFailedWorkCannotBeRedispatchedIllegally proves terminal failed work
// rejects a forbidden public move while an in-flight redispatch is consuming the
// work item, leaving the customer-visible state unchanged after the refusal.
func TestTerminalFailedWorkCannotBeRedispatchedIllegally(t *testing.T) {
	if testing.Short() {
		t.Skip("slow work recovery functional test")
	}

	const (
		workID    = "redispatch-guard-work-id"
		traceID   = "trace-redispatch-guard"
		requestID = "request-redispatch-guard"
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "happy_path"))
	provider := newRecoveryRedispatchBlockingProvider("worker-a", "worker-a")
	server := startRecoveryAPIServer(t, dir, provider)
	defer server.Stop(t)

	workTypeName := "task"
	support.UpsertDefaultSessionWorkRequest(t, server.URL(), factoryapi.WorkRequest{
		RequestId: requestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         "task",
			WorkId:       stringPtr(workID),
			WorkTypeName: &workTypeName,
			TraceId:      stringPtr(traceID),
			Payload:      map[string]string{"title": "terminal failed redispatch guard"},
		}},
	})

	waitForWorkIDsAtState(t, server.URL(), []string{workID}, "failed", 15*time.Second)
	failedLocation := support.WorkCustomerLocation(workTypeName, "failed")
	listed := support.ListDefaultSessionWork(t, server.URL())
	if !support.HasWorkAtCustomerState(listed, workID, failedLocation) {
		t.Fatalf("work listing = %#v, want %s before illegal redispatch attempt", listed.Results, failedLocation)
	}

	recoveryMoved := postMoveWork(t, server.URL(), workID, "init")
	if workStateName(recoveryMoved.State) != "init" {
		t.Fatalf("recovery move response = %#v, want init", recoveryMoved)
	}

	provider.waitForBlockedRedispatch(t, 15*time.Second)
	waitForSessionInFlightDispatches(t, server.URL(), 1, 15*time.Second)

	beforeIllegalMove := support.ListDefaultSessionWork(t, server.URL())
	beforeState := workStateName(requireWorkByID(t, beforeIllegalMove, workID).State)

	status, body := postMoveWorkStatus(t, server.URL(), workID, "processing")
	if status != http.StatusBadRequest && status != http.StatusNotFound {
		t.Fatalf("illegal redispatch move status = %d, want 400 or 404 refusal: %s", status, body)
	}

	afterIllegalMove := support.ListDefaultSessionWork(t, server.URL())
	afterState := workStateName(requireWorkByID(t, afterIllegalMove, workID).State)
	if afterState != beforeState {
		t.Fatalf("work state after illegal move = %q, want unchanged %q; listing=%#v", afterState, beforeState, afterIllegalMove.Results)
	}
	if support.HasWorkAtCustomerState(afterIllegalMove, workID, support.WorkCustomerLocation(workTypeName, "complete")) {
		t.Fatalf("work listing = %#v, want no unauthorized completion after illegal redispatch", afterIllegalMove.Results)
	}

	provider.releaseBlockedRedispatch()
	waitForWorkIDsAtState(t, server.URL(), []string{workID}, "failed", 15*time.Second)

	finalListed := support.ListDefaultSessionWork(t, server.URL())
	if !support.HasWorkAtCustomerState(finalListed, workID, failedLocation) {
		t.Fatalf("final work listing = %#v, want terminal failed %s", finalListed.Results, failedLocation)
	}
}

// TestAPIMoveWorkResumesRecoverableFlow proves a valid POST /work/{id}/move against
// recoverable failed work returns HTTP success with the requested customer-visible
// state and allows the public session flow to resume to completion.
func TestAPIMoveWorkResumesRecoverableFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("slow work recovery functional test")
	}

	const (
		workID    = "api-move-resume-work-id"
		traceID   = "trace-api-move-resume"
		requestID = "request-api-move-resume"
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "happy_path"))
	provider := testutil.NewMockWorkerMapProviderWithDefault(map[string][]testutil.WorkResponse{
		"worker-a": {
			{Error: errors.New("initial recoverable failure")},
			{Content: "COMPLETE"},
		},
		"worker-b": {
			{Content: "COMPLETE"},
		},
	})
	server := startRecoveryAPIServer(t, dir, provider)
	defer server.Stop(t)

	workTypeName := "task"
	support.UpsertDefaultSessionWorkRequest(t, server.URL(), factoryapi.WorkRequest{
		RequestId: requestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         "task",
			WorkId:       stringPtr(workID),
			WorkTypeName: &workTypeName,
			TraceId:      stringPtr(traceID),
			Payload:      map[string]string{"title": "api move resume"},
		}},
	})

	waitForWorkIDsAtState(t, server.URL(), []string{workID}, "failed", 15*time.Second)

	const recoverState = "init"
	status, body := postMoveWorkStatus(t, server.URL(), workID, recoverState)
	if status != http.StatusOK {
		t.Fatalf("API move status = %d, want 200 success: %s", status, body)
	}

	var moved factoryapi.Work
	if err := json.Unmarshal([]byte(body), &moved); err != nil {
		t.Fatalf("decode API move response: %v", err)
	}
	if support.StringPointerValue(moved.WorkId) != workID {
		t.Fatalf("API move WorkId = %q, want %q", support.StringPointerValue(moved.WorkId), workID)
	}
	if workStateName(moved.State) != recoverState {
		t.Fatalf("API move response state = %q, want requested %q; body=%s", workStateName(moved.State), recoverState, body)
	}

	completed := waitForWorkIDsComplete(t, server.URL(), []string{workID}, 15*time.Second)
	if len(completed) != 1 || workStateName(completed[0].State) != "complete" {
		t.Fatalf("resumed flow completion = %#v, want complete", completed)
	}

	listed := support.ListDefaultSessionWork(t, server.URL())
	completeLocation := support.WorkCustomerLocation(workTypeName, "complete")
	if !support.HasWorkAtCustomerState(listed, workID, completeLocation) {
		t.Fatalf("work listing after API move resume = %#v, want %s", listed.Results, completeLocation)
	}
}

// TestAPIInvalidMoveReturnsConflictWithoutMutation proves a duplicate public
// Work move request-id returns HTTP 409 Conflict and leaves the customer-visible
// work state unchanged after a successful move has already been applied.
func TestAPIInvalidMoveReturnsConflictWithoutMutation(t *testing.T) {
	if testing.Short() {
		t.Skip("slow work recovery functional test")
	}

	const (
		workID        = "api-invalid-move-work-id"
		traceID       = "trace-api-invalid-move"
		requestID     = "request-api-invalid-move"
		moveRequestID = "move-request-api-invalid-move"
	)

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "happy_path"))
	provider := newRecoveryRedispatchBlockingProvider("worker-a", "worker-a")
	server := startRecoveryAPIServer(t, dir, provider)
	defer server.Stop(t)

	workTypeName := "task"
	support.UpsertDefaultSessionWorkRequest(t, server.URL(), factoryapi.WorkRequest{
		RequestId: requestID,
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works: &[]factoryapi.Work{{
			Name:         "task",
			WorkId:       stringPtr(workID),
			WorkTypeName: &workTypeName,
			TraceId:      stringPtr(traceID),
			Payload:      map[string]string{"title": "api invalid move conflict"},
		}},
	})

	waitForWorkIDsAtState(t, server.URL(), []string{workID}, "failed", 15*time.Second)

	const recoverState = "init"
	moved := postMoveWorkWithRequestID(t, server.URL(), workID, recoverState, moveRequestID)
	if workStateName(moved.State) != recoverState {
		t.Fatalf("initial API move response state = %q, want %q", workStateName(moved.State), recoverState)
	}

	provider.waitForBlockedRedispatch(t, 15*time.Second)
	waitForSessionInFlightDispatches(t, server.URL(), 1, 15*time.Second)

	beforeInvalidMove := support.ListDefaultSessionWork(t, server.URL())
	beforeState := workStateName(requireWorkByID(t, beforeInvalidMove, workID).State)
	if beforeState != recoverState {
		t.Fatalf("work state before invalid move = %q, want %q", beforeState, recoverState)
	}

	status, body := postMoveWorkStatusWithRequestID(
		t,
		server.URL(),
		workID,
		recoverState,
		moveRequestID,
	)
	if status != http.StatusConflict {
		t.Fatalf("duplicate API move status = %d, want 409 conflict: %s", status, body)
	}

	afterInvalidMove := support.ListDefaultSessionWork(t, server.URL())
	afterState := workStateName(requireWorkByID(t, afterInvalidMove, workID).State)
	if afterState != beforeState {
		t.Fatalf(
			"work state after invalid move = %q, want unchanged %q; listing=%#v",
			afterState,
			beforeState,
			afterInvalidMove.Results,
		)
	}

	provider.releaseBlockedRedispatch()
}
