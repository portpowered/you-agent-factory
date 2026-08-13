package pending_approval_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestHumanApprovalPendingReadAndReplayPreserveOneDurableClaim(t *testing.T) {
	t.Parallel()

	factoryDir := support.ScaffoldFactory(t, map[string]any{
		"name": "durable-human-approval",
		"workTypes": []any{map[string]any{
			"name": "task",
			"states": []any{
				map[string]any{"name": "init", "type": "INITIAL"},
				map[string]any{"name": "approved", "type": "TERMINAL"},
				map[string]any{"name": "rejected", "type": "TERMINAL"},
			},
		}},
		"workstations": []any{map[string]any{
			"id": "release-approval", "name": "Release Approval", "type": "HUMAN_APPROVAL",
			"description": map[string]any{"type": "LOCALIZABLE_ASSET", "value": "Approve the release"},
			"inputs":      []any{map[string]any{"workType": "task", "state": "init"}},
			"outputs":     []any{map[string]any{"workType": "task", "state": "approved"}},
			"onRejection": []any{map[string]any{"workType": "task", "state": "rejected"}},
		}},
	})
	artifactPath := filepath.Join(t.TempDir(), "human-approval.replay.json")
	recordServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: factoryDir,
		Args:       []string{"--record", artifactPath},
	})

	accepted := support.SubmitDefaultSessionWork(t, recordServer.URL(), factoryapi.SubmitWorkRequest{
		Name: stringPointer("Release candidate"), WorkTypeName: "task", TraceId: stringPointer("durable-approval-trace"),
		Payload: map[string]any{"title": "release candidate", "secret": "must-not-become-an-event-payload"},
	})
	if accepted.RequestId == "" {
		t.Fatalf("submitted work response = %#v, want request identity", accepted)
	}

	liveEvents := waitForHumanApprovalEvents(t, recordServer.URL())
	assertOneHumanApprovalRequest(t, liveEvents)
	liveApproval := assertPendingApprovalReadSurfaces(t, recordServer.URL())
	recordServer.Stop(t)

	artifact := testutil.LoadReplayArtifact(t, artifactPath)
	if countReplayEvents(artifact.Events, factorydefinitions.FactoryEventTypeHumanApprovalRequested) != 1 {
		t.Fatalf("recorded HUMAN_APPROVAL_REQUESTED events = %d, want one", countReplayEvents(artifact.Events, factorydefinitions.FactoryEventTypeHumanApprovalRequested))
	}

	replayServer := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir: t.TempDir(), Args: []string{"--replay", artifactPath, "--no-record"},
	})
	replayEvents := waitForHumanApprovalEvents(t, replayServer.URL())
	assertOneHumanApprovalRequest(t, replayEvents)
	replayApproval := assertPendingApprovalReadSurfaces(t, replayServer.URL())
	if replayApproval.ApprovalId != liveApproval.ApprovalId || replayApproval.DispatchId != liveApproval.DispatchId || replayApproval.WorkstationId != liveApproval.WorkstationId || !sameStrings(replayApproval.WorkIds, liveApproval.WorkIds) {
		t.Fatalf("replayed approval = %#v, want the same durable claim as %#v", replayApproval, liveApproval)
	}
}

func waitForHumanApprovalEvents(t *testing.T, baseURL string) []factoryapi.FactoryEvent {
	t.Helper()
	_, err := support.WaitForObservation(10*time.Second, func() (factoryapi.ListHumanApprovalsResponse, error) {
		return fetchPendingApprovals(baseURL)
	}, func(response factoryapi.ListHumanApprovalsResponse) bool {
		return len(response.Approvals) == 1
	})
	if err != nil {
		t.Fatalf("wait for pending human approval projection: %v", err)
	}
	return support.GetFactoryEventsAt(t, baseURL)
}

func fetchPendingApprovals(baseURL string) (factoryapi.ListHumanApprovalsResponse, error) {
	response, err := http.Get(strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + factorysessions.DefaultSessionID + "/approvals")
	if err != nil {
		return factoryapi.ListHumanApprovalsResponse{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(response.Body)
		return factoryapi.ListHumanApprovalsResponse{}, fmt.Errorf("approvals status %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.ListHumanApprovalsResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return factoryapi.ListHumanApprovalsResponse{}, err
	}
	return result, nil
}

func assertPendingApprovalReadSurfaces(t *testing.T, baseURL string) factoryapi.HumanApproval {
	t.Helper()
	collection := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/" + factorysessions.DefaultSessionID + "/approvals"
	listed := support.GetJSON[factoryapi.ListHumanApprovalsResponse](t, collection)
	if len(listed.Approvals) != 1 {
		t.Fatalf("pending approval list = %#v, want one canonical approval", listed.Approvals)
	}
	approval := listed.Approvals[0]
	shown := support.GetJSON[factoryapi.HumanApproval](t, collection+"/"+url.PathEscape(approval.ApprovalId))
	if shown.ApprovalId != approval.ApprovalId || shown.Status != factoryapi.HumanApprovalStatusPENDING {
		t.Fatalf("pending approval show = %#v, want listed pending approval", shown)
	}
	if len(approval.WorkIds) == 0 {
		t.Fatalf("pending approval = %#v, want ordered Work correlation", approval)
	}
	work := support.GetDefaultSessionWorkByID(t, baseURL, approval.WorkIds[0])
	if work.HumanApproval == nil || work.HumanApproval.ApprovalId != approval.ApprovalId {
		t.Fatalf("Work %q human approval = %#v, want approval %q; approval=%#v; work=%#v", approval.WorkIds[0], work.HumanApproval, approval.ApprovalId, approval, work)
	}
	workerSessions := support.ListDefaultSessionWorkerSessions(t, baseURL, approval.WorkIds[0])
	if len(workerSessions.Sessions) != 0 {
		t.Fatalf("Worker Sessions for human approval Work = %#v, want none", workerSessions.Sessions)
	}
	session := support.GetDefaultSession(t, baseURL)
	if session.Runtime.PendingHumanApprovals == nil || len(*session.Runtime.PendingHumanApprovals) != 1 || (*session.Runtime.PendingHumanApprovals)[0].ApprovalId != approval.ApprovalId {
		t.Fatalf("Factory Session pending approvals = %#v, want approval %q", session.Runtime.PendingHumanApprovals, approval.ApprovalId)
	}
	return approval
}

func assertOneHumanApprovalRequest(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	var approvalEvents []factoryapi.FactoryEvent
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeHumanApprovalRequested {
			approvalEvents = append(approvalEvents, event)
		}
	}
	if len(approvalEvents) != 1 {
		t.Fatalf("HUMAN_APPROVAL_REQUESTED events = %d, want one; events=%#v", len(approvalEvents), events)
	}
	payload, err := approvalEvents[0].Payload.AsHumanApprovalRequestedEventPayload()
	if err != nil {
		t.Fatalf("decode human approval event: %v", err)
	}
	if payload.Status != factoryapi.HumanApprovalRequestedEventPayloadStatusPENDING || len(payload.Decisions) != 2 || payload.Decisions[0] != factoryapi.HumanApprovalRequestedEventPayloadDecisionsAPPROVE || payload.Decisions[1] != factoryapi.HumanApprovalRequestedEventPayloadDecisionsREJECT {
		t.Fatalf("human approval payload = %#v, want pending APPROVE/REJECT", payload)
	}
	encoded, err := json.Marshal(approvalEvents[0])
	if err != nil {
		t.Fatalf("marshal human approval event: %v", err)
	}
	if strings.Contains(string(encoded), "must-not-become-an-event-payload") || strings.Contains(string(encoded), "secret") {
		t.Fatalf("human approval event leaked Work payload: %s", encoded)
	}
}

func countReplayEvents(events []factorydefinitions.FactoryEvent, kind factorydefinitions.FactoryEventType) int {
	count := 0
	for _, event := range events {
		if event.Type == kind {
			count++
		}
	}
	return count
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func stringPointer(value string) *string {
	return &value
}
