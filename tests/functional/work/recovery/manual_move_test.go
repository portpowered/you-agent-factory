package recovery

import (
	"errors"
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
