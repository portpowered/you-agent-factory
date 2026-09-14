package review_failure_routing

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
)

const (
	ciFailureFeedback = "CI failed: exact current-head feedback 7f1c"
	reviewFeedback    = "review feedback: exact current correction 4a6d"
	// The Codex mock emits a turn.failed record and the public provider
	// boundary exposes its stable customer-safe normalization.
	reviewFailureDiagnostic = "provider rejected the execution request"
)

type routingIDs struct {
	oldLineageTask, oldLineageReview string
	oldCIIdea, oldCITask             string
	oldRejectTask, oldRejectReview   string
	oldFailureTask, oldFailureReview string
	oldCompleteIdea                  string

	lineageTaskA, lineageReviewA               string
	lineageTaskB, lineageReviewB               string
	ciIdea, ciTask                             string
	rejectTask, rejectReview                   string
	failureTask, failureReview                 string
	completeIdea, completeTask, completeReview string
}

func childRoutingIDs() routingIDs {
	return routingIDs{
		oldLineageTask: "child-old-lineage-task", oldLineageReview: "child-old-lineage-review",
		oldCIIdea: "child-old-ci-idea", oldCITask: "child-old-ci-task",
		oldRejectTask: "child-old-reject-task", oldRejectReview: "child-old-reject-review",
		oldFailureTask: "child-old-failure-task", oldFailureReview: "child-old-failure-review",
		oldCompleteIdea: "child-old-complete-idea",
		lineageTaskA:    "child-lineage-task-a", lineageReviewA: "child-lineage-review-a",
		lineageTaskB: "child-lineage-task-b", lineageReviewB: "child-lineage-review-b",
		ciIdea: "child-ci-idea", ciTask: "child-ci-task",
		rejectTask: "child-reject-task", rejectReview: "child-reject-review",
		failureTask: "child-failure-task", failureReview: "child-failure-review",
		completeIdea: "child-complete-idea", completeTask: "child-complete-task", completeReview: "child-complete-review",
	}
}

// TestChildProcessExactLineageRouting is the I-01 through I-04 witness. One
// externally built CLI receives historical Work after readiness, then later
// public requests drive every current case without operator moves, retries,
// or a second runtime.
func TestChildProcessExactLineageRouting(t *testing.T) {
	artifact := requireRoutingArtifact(t)
	ids := childRoutingIDs()
	root := t.TempDir()
	factoryDir := filepath.Join(root, "factory")
	if err := os.CopyFS(factoryDir, os.DirFS(testutil.MustRepoPath(t, "factory"))); err != nil {
		t.Fatalf("copy authored Factory into isolated child fixture: %v", err)
	}
	homeDir := filepath.Join(root, "home")
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		t.Fatalf("create isolated child home: %v", err)
	}
	mockWorkersPath := filepath.Join(root, "mock-workers.json")
	reviewScriptPath := filepath.Join(root, "review-rejection-codex.jsonl")
	recordPath := filepath.Join(root, "child-recording.json")
	reviewScript := writeChildDecisionScript(t, reviewScriptPath, reviewFeedback)
	writeRoutingJSON(t, mockWorkersPath, childMockWorkers(ids, reviewScript, factoryDir))

	child := startRoutingChild(t, artifact, factoryDir, homeDir, mockWorkersPath, recordPath)
	waitForRoutingChildReady(t, child)
	if got := waitForRoutingSession(t, child); got == "" {
		t.Fatal("live default Factory Session has no public ID")
	}

	oldWorks := initialHistoryWorks(ids)
	submitRoutingBatch(t, child, "child-startup-history", oldWorks)
	before := waitForRoutingHistory(t, child, oldWorks)
	oldSnapshots := make(map[string]routingWorkSnapshot, len(oldWorks))
	for _, work := range before {
		if work.WorkId != nil {
			oldSnapshots[*work.WorkId] = snapshotRoutingWork(work)
		}
	}
	if len(oldSnapshots) != len(oldWorks) {
		t.Fatalf("startup historical Work count = %d, want %d", len(oldSnapshots), len(oldWorks))
	}

	for _, submission := range currentRoutingSubmissions(ids) {
		submitRoutingBatch(t, child, submission.requestID, submission.works)
	}
	works, events := waitForRoutingProof(t, child, ids, oldSnapshots)
	events = waitForStableRoutingEvents(t, child, childQuiescenceTicks)
	var err error
	works, err = readRoutingWorks(child.baseURL, child.sessionID)
	if err != nil {
		t.Fatalf("read final public Work history: %v\n%s", err, child.evidence())
	}
	assertRoutingProof(t, ids, works, events, oldSnapshots)

	child.stop(t)
	waitForRoutingEndpointClosed(t, child)
	assertRecordWritten(t, recordPath)
}

func waitForRoutingHistory(t *testing.T, child *routingChild, seeds []factoryapi.Work) []factoryapi.Work {
	t.Helper()
	want := make(map[string]struct{}, len(seeds))
	for _, seed := range seeds {
		if seed.WorkId != nil {
			want[*seed.WorkId] = struct{}{}
		}
	}
	deadline := time.NewTimer(childReadyTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(childPollInterval)
	defer ticker.Stop()
	for {
		works, err := readRoutingWorks(child.baseURL, child.sessionID)
		if err == nil {
			byID := routingWorkByID(works)
			complete := len(byID) >= len(want)
			for id := range want {
				if _, ok := byID[id]; !ok {
					complete = false
					break
				}
			}
			if complete {
				return works
			}
		}
		if exited, waitErr := child.exited(); exited {
			t.Fatalf("Factory child exited while admitting startup history: %v\n%s", waitErr, child.evidence())
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for startup historical Work\n%s", child.evidence())
		}
	}
}

func waitForRoutingProof(t *testing.T, child *routingChild, ids routingIDs, oldSnapshots map[string]routingWorkSnapshot) ([]factoryapi.Work, []factoryEvent) {
	t.Helper()
	deadline := time.NewTimer(childOperationTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(childPollInterval)
	defer ticker.Stop()
	lastSummary := ""
	var lastWorkErr, lastEventErr error
	for {
		works, workErr := readRoutingWorks(child.baseURL, child.sessionID)
		events, eventErr := readRoutingEvents(child.baseURL, child.sessionID)
		lastWorkErr, lastEventErr = workErr, eventErr
		if workErr == nil && eventErr == nil {
			lastSummary = routingProofSummary(ids, works, events, oldSnapshots)
			if routingProofReady(ids, works, events, oldSnapshots) {
				return works, events
			}
		}
		if exited, waitErr := child.exited(); exited {
			t.Fatalf("Factory child exited before routing proof completed: %v work_error=%v event_error=%v summary=%s\n%s", waitErr, lastWorkErr, lastEventErr, lastSummary, child.evidence())
		}
		select {
		case <-ticker.C:
		case <-deadline.C:
			t.Fatalf("timed out waiting for I-01 through I-04 routing proof; work_error=%v event_error=%v summary=%s\n%s", lastWorkErr, lastEventErr, lastSummary, child.evidence())
		}
	}
}

func routingProofReady(ids routingIDs, works []factoryapi.Work, events []factoryEvent, oldSnapshots map[string]routingWorkSnapshot) bool {
	byID := routingWorkByID(works)
	for id, want := range map[string]string{
		ids.lineageTaskA: "to-complete", ids.lineageReviewA: "complete",
		ids.lineageTaskB: "to-complete", ids.lineageReviewB: "complete",
		ids.ciIdea: "failed", ids.ciTask: "failed", ids.rejectTask: "to-complete",
		ids.failureTask: "failed", ids.failureReview: "fin",
		ids.completeIdea: "failed", ids.completeTask: "complete", ids.completeReview: "complete",
	} {
		work, ok := byID[id]
		if !ok || routingState(work) != want {
			return false
		}
	}
	replacementReviews := 0
	for _, work := range works {
		if isReplacementReview(work, ids.rejectReview) {
			replacementReviews++
		}
	}
	return replacementReviews == 1 && routingDispatchProofReady(ids, events) && routingHistoryUnchanged(works, oldSnapshots)
}

func routingDispatchProofReady(ids routingIDs, events []factoryEvent) bool {
	dispatches := routingDispatchesBestEffort(events)
	details := routingDispatchProofDetails(ids, dispatches)
	return details.lineageA && details.lineageB && details.ci && details.escalation &&
		details.rejection && details.rejectProcess && details.rejectCIWait &&
		details.failure && details.completion
}

type routingDispatchProofDetail struct {
	lineageA, lineageB, ci, escalation              bool
	rejection, rejectProcess, rejectCIWait, failure bool
	completion                                      bool
}

func routingDispatchProofDetails(ids routingIDs, dispatches []routingDispatch) routingDispatchProofDetail {
	rejectionReviews := dispatchesWithTransitionAndInput(dispatches, "review", ids.rejectTask)
	return routingDispatchProofDetail{
		lineageA:      countExactReview(dispatchesByTransition(dispatchesForID(dispatches, ids.lineageTaskA), "review"), ids.lineageTaskA, ids.lineageReviewA, factoryapi.WorkOutcomeAccepted) == 1,
		lineageB:      countExactReview(dispatchesByTransition(dispatchesForID(dispatches, ids.lineageTaskB), "review"), ids.lineageTaskB, ids.lineageReviewB, factoryapi.WorkOutcomeAccepted) == 1,
		ci:            countExactTransition(dispatches, "ci-wait", []string{ids.ciTask}, factoryapi.WorkOutcomeFailed) == 1,
		escalation:    countExactTransition(dispatches, "escalate-task-failure", []string{ids.ciIdea, ids.ciTask}, factoryapi.WorkOutcomeAccepted) == 1,
		rejection:     len(rejectionReviews) == 2 && hasReviewOutcome(rejectionReviews, ids.rejectReview, factoryapi.WorkOutcomeRejected, reviewFeedback) && hasAcceptedReviewWithoutInput(rejectionReviews, ids.rejectReview),
		rejectProcess: countExactTransition(dispatches, "process", []string{ids.rejectTask}, factoryapi.WorkOutcomeAccepted) == 1,
		rejectCIWait:  countExactTransition(dispatches, "ci-wait", []string{ids.rejectTask}, factoryapi.WorkOutcomeAccepted) == 1,
		failure:       countExactReview(dispatchesByTransition(dispatchesForID(dispatches, ids.failureTask), "review"), ids.failureTask, ids.failureReview, factoryapi.WorkOutcomeFailed) == 1,
		completion:    countExactTransition(dispatches, "complete-reviewed-task-after-failed-idea", []string{ids.completeIdea, ids.completeTask, ids.completeReview}, factoryapi.WorkOutcomeAccepted) == 1,
	}
}

func routingDispatchesBestEffort(events []factoryEvent) []routingDispatch {
	byID := make(map[string]int)
	dispatches := make([]routingDispatch, 0)
	for _, event := range events {
		if event.Context.DispatchId == nil || *event.Context.DispatchId == "" {
			continue
		}
		id := *event.Context.DispatchId
		switch event.Type {
		case factoryapi.FactoryEventTypeDispatchRequest:
			payload, err := event.Payload.AsDispatchRequestEventPayload()
			if err != nil {
				continue
			}
			inputIDs := make([]string, 0, len(payload.Inputs))
			for _, input := range payload.Inputs {
				inputIDs = append(inputIDs, input.WorkId)
			}
			byID[id] = len(dispatches)
			dispatches = append(dispatches, routingDispatch{ID: id, Transition: payload.TransitionId, InputIDs: inputIDs, RequestSeen: true})
		case factoryapi.FactoryEventTypeDispatchResponse:
			payload, err := event.Payload.AsDispatchResponseEventPayload()
			if err != nil {
				continue
			}
			if index, ok := byID[id]; ok {
				dispatches[index].Response = &payload
			} else {
				byID[id] = len(dispatches)
				dispatches = append(dispatches, routingDispatch{ID: id, Transition: payload.TransitionId, Response: &payload})
			}
		}
	}
	return dispatches
}

func dispatchesForID(dispatches []routingDispatch, workID string) []routingDispatch {
	filtered := make([]routingDispatch, 0)
	for _, dispatch := range dispatches {
		if routingDispatchHasInput(dispatch, workID) {
			filtered = append(filtered, dispatch)
		}
	}
	return filtered
}

func dispatchesByTransition(dispatches []routingDispatch, transition string) []routingDispatch {
	filtered := make([]routingDispatch, 0)
	for _, dispatch := range dispatches {
		if dispatch.Transition == transition {
			filtered = append(filtered, dispatch)
		}
	}
	return filtered
}

func countExactReview(dispatches []routingDispatch, taskID, reviewID string, outcome factoryapi.WorkOutcome) int {
	count := 0
	for _, dispatch := range dispatches {
		if routingDispatchHasInputs(dispatch, taskID, reviewID) && dispatch.Response != nil && dispatch.Response.Outcome == outcome {
			count++
		}
	}
	return count
}

func countExactTransition(dispatches []routingDispatch, transition string, inputIDs []string, outcome factoryapi.WorkOutcome) int {
	count := 0
	for _, dispatch := range dispatches {
		if dispatch.Transition == transition && routingDispatchHasInputs(dispatch, inputIDs...) && dispatch.Response != nil && dispatch.Response.Outcome == outcome {
			count++
		}
	}
	return count
}

func dispatchesWithTransitionAndInput(dispatches []routingDispatch, transition, workID string) []routingDispatch {
	filtered := make([]routingDispatch, 0)
	for _, dispatch := range dispatches {
		if dispatch.Transition == transition && routingDispatchHasInput(dispatch, workID) && dispatch.Response != nil {
			filtered = append(filtered, dispatch)
		}
	}
	return filtered
}

func hasReviewOutcome(dispatches []routingDispatch, rejectedReviewID string, outcome factoryapi.WorkOutcome, feedback string) bool {
	for _, dispatch := range dispatches {
		if routingDispatchHasInput(dispatch, rejectedReviewID) && dispatch.Response != nil && dispatch.Response.Outcome == outcome && routingDiagnostic(dispatch.Response) == feedback {
			return true
		}
	}
	return false
}

func hasAcceptedReviewWithoutInput(dispatches []routingDispatch, rejectedReviewID string) bool {
	for _, dispatch := range dispatches {
		if dispatch.Response == nil || dispatch.Response.Outcome != factoryapi.WorkOutcomeAccepted || routingDispatchHasInput(dispatch, rejectedReviewID) {
			continue
		}
		if dispatch.Response.OutputWork == nil {
			continue
		}
		for _, output := range *dispatch.Response.OutputWork {
			if isReplacementReview(output, rejectedReviewID) {
				return true
			}
		}
	}
	return false
}

func isReplacementReview(work factoryapi.Work, rejectedReviewID string) bool {
	return work.WorkId != nil && *work.WorkId != rejectedReviewID &&
		work.WorkTypeName != nil && *work.WorkTypeName == "review" &&
		routingState(work) == "complete" && work.CurrentChainingTraceId != nil &&
		*work.CurrentChainingTraceId == "reject-current"
}

func routingHistoryUnchanged(works []factoryapi.Work, before map[string]routingWorkSnapshot) bool {
	byID := routingWorkByID(works)
	for id, snapshot := range before {
		work, ok := byID[id]
		if !ok || snapshotRoutingWork(work) != snapshot {
			return false
		}
	}
	return true
}

func routingProofSummary(ids routingIDs, works []factoryapi.Work, events []factoryEvent, before map[string]routingWorkSnapshot) string {
	byID := routingWorkByID(works)
	dispatches := routingDispatchesBestEffort(events)
	proof := routingDispatchProofDetails(ids, dispatches)
	replacementReviews := 0
	for _, work := range works {
		if isReplacementReview(work, ids.rejectReview) {
			replacementReviews++
		}
	}
	rejection := make([]string, 0)
	for _, dispatch := range dispatchesWithTransitionAndInput(dispatches, "review", ids.rejectTask) {
		outcome := "<no-response>"
		diagnostic := ""
		if dispatch.Response != nil {
			outcome = string(dispatch.Response.Outcome)
			diagnostic = routingDiagnostic(dispatch.Response)
		}
		rejection = append(rejection, fmt.Sprintf("transition=%s inputs=%v outcome=%s diagnostic=%q", dispatch.Transition, dispatch.InputIDs, outcome, diagnostic))
	}
	return fmt.Sprintf("works=%d events=%d taskA=%s taskB=%s ci=%s/%s reject=%s failure=%s/%s complete=%s/%s/%s history_unchanged=%t replacement_reviews=%d proof=%+v rejection=[%s]", len(byID), len(events), routingState(byID[ids.lineageTaskA]), routingState(byID[ids.lineageTaskB]), routingState(byID[ids.ciIdea]), routingState(byID[ids.ciTask]), routingState(byID[ids.rejectTask]), routingState(byID[ids.failureTask]), routingState(byID[ids.failureReview]), routingState(byID[ids.completeIdea]), routingState(byID[ids.completeTask]), routingState(byID[ids.completeReview]), routingHistoryUnchanged(works, before), replacementReviews, proof, strings.Join(rejection, "; "))
}

func assertRoutingProof(t *testing.T, ids routingIDs, works []factoryapi.Work, events []factoryEvent, oldSnapshots map[string]routingWorkSnapshot) {
	t.Helper()
	if !routingProofReady(ids, works, events, oldSnapshots) {
		t.Fatalf("final public routing proof is incomplete: %s", routingProofSummary(ids, works, events, oldSnapshots))
	}
	dispatches := routingDispatches(t, events)
	oldIDs := []string{ids.oldLineageTask, ids.oldLineageReview, ids.oldCIIdea, ids.oldCITask, ids.oldRejectTask, ids.oldRejectReview, ids.oldFailureTask, ids.oldFailureReview, ids.oldCompleteIdea}
	for _, dispatch := range dispatches {
		for _, oldID := range oldIDs {
			if routingDispatchHasInput(dispatch, oldID) {
				t.Fatalf("historical Work %q was consumed by dispatch %#v", oldID, dispatch)
			}
		}
	}
	dispatches = routingDispatchesBestEffort(events)
	if response := firstMatchingResponse(dispatches, "ci-wait", ids.ciTask, factoryapi.WorkOutcomeFailed); response == nil || !strings.HasSuffix(routingDiagnostic(response), ciFailureFeedback) {
		t.Fatalf("CI failure diagnostic = %q, want exact feedback suffix %q", routingDiagnostic(response), ciFailureFeedback)
	}
	if response := firstMatchingResponse(dispatches, "review", ids.failureTask, factoryapi.WorkOutcomeFailed); response == nil || !strings.HasSuffix(routingDiagnostic(response), reviewFailureDiagnostic) {
		t.Fatalf("review failure diagnostic = %q, want exact feedback suffix %q", routingDiagnostic(response), reviewFailureDiagnostic)
	}
	byID := routingWorkByID(works)
	for id, want := range map[string]string{ids.lineageTaskA: "to-complete", ids.lineageReviewA: "complete", ids.lineageTaskB: "to-complete", ids.lineageReviewB: "complete", ids.ciIdea: "failed", ids.ciTask: "failed", ids.rejectTask: "to-complete", ids.failureTask: "failed", ids.failureReview: "fin", ids.completeIdea: "failed", ids.completeTask: "complete", ids.completeReview: "complete"} {
		if got := routingState(byID[id]); got != want {
			t.Fatalf("public Work %q state=%q, want %q", id, got, want)
		}
	}
	t.Logf("I-01-I-04 public proof: %s", routingProofSummary(ids, works, events, oldSnapshots))
}

func firstMatchingResponse(dispatches []routingDispatch, transition, workID string, outcome factoryapi.WorkOutcome) *factoryapi.DispatchResponseEventPayload {
	for _, dispatch := range dispatches {
		if dispatch.Transition == transition && routingDispatchHasInput(dispatch, workID) && dispatch.Response != nil && dispatch.Response.Outcome == outcome {
			return dispatch.Response
		}
	}
	return nil
}

func writeRoutingJSON(t *testing.T, path string, value interface{}) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal routing fixture %q: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write routing fixture %q: %v", path, err)
	}
}

func initialHistoryWorks(ids routingIDs) []factoryapi.Work {
	return []factoryapi.Work{
		childWork("child-lineage-task", ids.oldLineageTask, "task", "failed", "old-lineage", "old lineage task"),
		childWork("child-lineage-review", ids.oldLineageReview, "review", "fin", "old-lineage", "old lineage review"),
		childWork("child-ci-idea", ids.oldCIIdea, "idea", "to-complete", "old-ci-idea", "old CI idea"),
		childWork("child-ci-task", ids.oldCITask, "task", "failed", "old-ci-task", "old CI task"),
		childWork("child-reject-task", ids.oldRejectTask, "task", "failed", "old-reject", "old rejection task"),
		childWork("child-reject-review", ids.oldRejectReview, "review", "fin", "old-reject", "old rejection review"),
		childWork("child-failure-task", ids.oldFailureTask, "task", "failed", "old-failure", "old failure task"),
		childWork("child-failure-review", ids.oldFailureReview, "review", "fin", "old-failure", "old failure review"),
		childWork("child-complete-idea", ids.oldCompleteIdea, "idea", "failed", "old-complete", "old complete idea"),
	}
}

type routingBatchSubmission struct {
	requestID string
	works     []factoryapi.Work
}

func currentRoutingSubmissions(ids routingIDs) []routingBatchSubmission {
	return []routingBatchSubmission{
		{requestID: "child-post-lineage-a", works: []factoryapi.Work{
			childWork("child-lineage-task", ids.lineageTaskA, "task", "in-review", "lineage-a", "lineage A task"),
			childWork("child-lineage-review", ids.lineageReviewA, "review", "init", "lineage-a", "lineage A review"),
		}},
		{requestID: "child-post-lineage-b", works: []factoryapi.Work{
			childWork("child-lineage-task", ids.lineageTaskB, "task", "in-review", "lineage-b", "lineage B task"),
			childWork("child-lineage-review", ids.lineageReviewB, "review", "init", "lineage-b", "lineage B review"),
		}},
		{requestID: "child-post-ci", works: []factoryapi.Work{
			childWork("child-ci-idea", ids.ciIdea, "idea", "to-complete", "ci-current", "current CI idea"),
			childWork("child-ci-task", ids.ciTask, "task", "awaiting-ci", "ci-current", "current CI task"),
		}},
		{requestID: "child-post-rejection", works: []factoryapi.Work{
			childWork("child-reject-task", ids.rejectTask, "task", "in-review", "reject-current", "current rejection task"),
			childWork("child-reject-review", ids.rejectReview, "review", "init", "reject-current", "current rejection review"),
		}},
		{requestID: "child-post-failure", works: []factoryapi.Work{
			childWork("child-failure-task", ids.failureTask, "task", "in-review", "failure-current", "current failure task"),
			childWork("child-failure-review", ids.failureReview, "review", "init", "failure-current", "current failure review"),
		}},
		{requestID: "child-post-completion", works: []factoryapi.Work{
			childWork("child-complete-idea", ids.completeIdea, "idea", "failed", "complete-current", "current complete idea"),
			childWork("child-complete-task", ids.completeTask, "task", "to-complete", "complete-current", "current complete task"),
			childWork("child-complete-review", ids.completeReview, "review", "complete", "complete-current", "current complete review"),
		}},
	}
}

func childWork(name, workID, workType, state, traceID, payload string) factoryapi.Work {
	return factoryapi.Work{Name: name, WorkId: stringPointer(workID), WorkTypeName: stringPointer(workType), State: &factoryapi.WorkState{Name: state, Type: childStateType(state)}, CurrentChainingTraceId: stringPointer(traceID), TraceId: stringPointer(traceID), Payload: payload}
}

func childStateType(state string) factoryapi.WorkStateType {
	switch state {
	case "init":
		return factoryapi.WorkStateTypeINITIAL
	case "complete":
		return factoryapi.WorkStateTypeTERMINAL
	case "failed", "fin":
		return factoryapi.WorkStateTypeFAILED
	default:
		return factoryapi.WorkStateTypePROCESSING
	}
}

func stringPointer(value string) *string { return &value }

func childMockWorkers(ids routingIDs, reviewScript, workingDirectory string) map[string]interface{} {
	return map[string]interface{}{
		"unmatchedDispatchPolicy": "accept",
		"mockWorkers": []interface{}{
			map[string]interface{}{"id": "reject-current-review-once", "workerName": "reviewer", "workstationName": "review", "workInputs": []interface{}{map[string]string{"workId": ids.rejectReview}}, "runType": "script", "scriptConfig": childDecisionScript(reviewScript, workingDirectory)},
			map[string]interface{}{"id": "fail-current-ci", "workerName": "ci-waiter", "workstationName": "ci-wait", "workInputs": []interface{}{map[string]string{"workId": ids.ciTask}}, "runType": "reject", "rejectConfig": map[string]interface{}{"stderr": ciFailureFeedback, "exitCode": 23}},
			map[string]interface{}{"id": "fail-current-review", "workerName": "reviewer", "workstationName": "review", "workInputs": []interface{}{map[string]string{"workId": ids.failureReview}}, "runType": "reject", "rejectConfig": map[string]interface{}{"stderr": reviewFailureDiagnostic, "exitCode": 24}},
		},
	}
}

func writeChildDecisionScript(t *testing.T, path, feedback string) string {
	t.Helper()
	decision, _ := json.Marshal(map[string]string{"decision": "REJECTED", "feedback": feedback})
	line, _ := json.Marshal(map[string]interface{}{"type": "item.completed", "item": map[string]interface{}{"id": "message-final", "type": "agent_message", "text": string(decision)}})
	turnStarted, _ := json.Marshal(map[string]string{"type": "turn.started"})
	turnCompleted, _ := json.Marshal(map[string]interface{}{"type": "turn.completed", "usage": map[string]int{"input_tokens": 1, "output_tokens": 1}})
	contents := append(append(append([]byte(nil), turnStarted...), '\n'), line...)
	contents = append(contents, '\n')
	contents = append(contents, turnCompleted...)
	contents = append(contents, '\n')
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write deterministic Codex rejection stream: %v", err)
	}
	return string(contents)
}

func childDecisionScript(contents, workingDirectory string) map[string]interface{} {
	// The worker command executes in a request-scoped worktree. Feed the
	// deterministic provider stream through stdin so the fixture does not
	// depend on that worktree containing a temporary host path.
	script := map[string]interface{}{"workingDirectory": workingDirectory, "stdin": contents}
	if runtime.GOOS == "windows" {
		script["command"] = "cmd.exe"
		script["args"] = []string{"/d", "/c", "findstr ."}
		return script
	}
	script["command"] = "cat"
	script["args"] = []string{}
	return script
}
