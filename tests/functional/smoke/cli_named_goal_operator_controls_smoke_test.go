package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestNamedGoalOperatorControls_PauseBuffersSubmitUntilResume(t *testing.T) {
	if testing.Short() {
		t.Skip("slow named @you/goal operator pause/resume smoke")
	}

	factoryDir := materializeNamedGoalFactoryForRoutingSmoke(t)
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "accepted",
	})
	server := startNamedGoalRoutingAPIServer(t, factoryDir, mockWorkersPath)

	pause := postNamedGoalOperatorLifecycleControl(
		t,
		server.URL(),
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}

	submitted := submitNamedGoalRoutingWork(t, server, "paused-operator-submit", "customer goal request text")
	workID := stringPointerValue(submitted.WorkId)
	session := support.GetDefaultSession(t, server.URL())
	if markingContainsNamedGoalRoutingWorkAtPlace(session, workID, "goal:init") {
		t.Fatalf("paused submit reached goal:init while session was paused: %#v", session.Runtime.Petri)
	}
	if markingContainsNamedGoalRoutingWorkAtPlace(session, workID, "goal:complete") {
		t.Fatalf("paused submit reached goal:complete before resume: %#v", session.Runtime.Petri)
	}

	resume := postNamedGoalOperatorLifecycleControl(
		t,
		server.URL(),
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	if resume.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume response = %#v, want accepted resume", resume)
	}

	waitForNamedGoalRoutingWorkAtState(t, server, []string{workID}, "complete", 15*time.Second)
}

func TestNamedGoalOperatorControls_ClIPauseResumeDrainsBufferedGoalsInOrder(t *testing.T) {
	if testing.Short() {
		t.Skip("slow named @you/goal CLI operator pause/resume smoke")
	}

	factoryDir := materializeNamedGoalFactoryForRoutingSmoke(t)
	mockWorkersPath := writePackagedGoalSlowPlannerTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "accepted",
	})
	server := startNamedGoalRoutingAPIServer(t, factoryDir, mockWorkersPath)
	baseURL := server.URL()
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	pauseResponse := runNamedGoalOperatorSessionLifecycleCLIJSON(
		t, ctx, binaryPath, baseURL, "pause",
	)
	if pauseResponse.Operation != factoryapi.FactorySessionLifecycleControlKindPause ||
		pauseResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("CLI pause response = %#v, want accepted pause", pauseResponse)
	}

	first := submitNamedGoalRoutingWork(t, server, "paused-cli-submit-1", "first paused goal text")
	second := submitNamedGoalRoutingWork(t, server, "paused-cli-submit-2", "second paused goal text")
	firstID := stringPointerValue(first.WorkId)
	secondID := stringPointerValue(second.WorkId)

	session := support.GetDefaultSession(t, server.URL())
	for _, workID := range []string{firstID, secondID} {
		if markingContainsNamedGoalRoutingWorkAtPlace(session, workID, "goal:complete") {
			t.Fatalf("work %q reached goal:complete before resume", workID)
		}
	}

	resumeResponse := runNamedGoalOperatorSessionLifecycleCLIJSON(
		t, ctx, binaryPath, baseURL, "resume",
	)
	if resumeResponse.Operation != factoryapi.FactorySessionLifecycleControlKindResume ||
		resumeResponse.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("CLI resume response = %#v, want accepted resume", resumeResponse)
	}

	assertNamedGoalOperatorBufferedGoalsDrainedInSubmissionOrder(t, server, firstID, secondID)
}

func assertNamedGoalOperatorBufferedGoalsDrainedInSubmissionOrder(
	t *testing.T,
	server *support.FunctionalAPIServer,
	firstID string,
	secondID string,
) {
	t.Helper()

	waitForNamedGoalRoutingWorkAtState(t, server, []string{firstID, secondID}, "complete", 30*time.Second)

	dispatches := support.ObserveDispatchEvents(t, server.GetFactoryEvents(t))
	firstPlan, okFirst := namedGoalOperatorPlanGoalDispatchForWork(
		dispatches,
		firstID,
	)
	secondPlan, okSecond := namedGoalOperatorPlanGoalDispatchForWork(
		dispatches,
		secondID,
	)
	if !okFirst || !okSecond {
		t.Fatalf(
			"dispatch history missing plan-goal dispatches for buffered goals %q and %q: %#v",
			firstID,
			secondID,
			dispatches,
		)
	}
	if !firstPlan.StartedAt.Before(secondPlan.StartedAt) {
		t.Fatalf(
			"plan-goal start order = first@%s second@%s for works %q then %q; want first buffered goal to start before second",
			firstPlan.StartedAt.UTC(),
			secondPlan.StartedAt.UTC(),
			firstID,
			secondID,
		)
	}
}

func namedGoalOperatorPlanGoalDispatchForWork(
	dispatches []support.DispatchEventObservation,
	workID string,
) (support.DispatchEventObservation, bool) {
	for _, dispatch := range dispatches {
		if dispatch.Request.TransitionId != publicGoal.PackagedPlanWorkstationName {
			continue
		}
		if support.DispatchObservationIncludesWork(dispatch, workID) {
			return dispatch, true
		}
	}
	return support.DispatchEventObservation{}, false
}

func TestNamedGoalOperatorControls_InterruptedGoalInspectSurfacesDispatchAndStopSummary(t *testing.T) {
	if testing.Short() {
		t.Skip("slow named @you/goal interrupted inspect smoke")
	}

	factoryDir := materializeNamedGoalFactoryForRoutingSmoke(t)
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "interrupted",
	})
	server := startNamedGoalRoutingAPIServer(t, factoryDir, mockWorkersPath)
	baseURL := server.URL()
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	submitted := submitNamedGoalRoutingWork(
		t,
		server,
		"interrupted-operator-inspect",
		fmt.Sprintf("functional-smoke-goal-operator-interrupted-%d", time.Now().UnixNano()),
	)
	workID := stringPointerValue(submitted.WorkId)
	waitForNamedGoalRoutingWorkAtState(t, server, []string{workID}, "interrupted", 15*time.Second)

	session := runNamedGoalOperatorSessionShowCLIJSON(t, ctx, binaryPath, baseURL)
	if session.Runtime.StopSummary == nil {
		t.Fatalf("session show = %#v, want runtime.stopSummary on interrupted goal", session)
	}
	if session.Runtime.StopSummary.StopKind != factoryapi.FactoryStopKind("INTERRUPTED") {
		t.Fatalf("session stopKind = %q, want INTERRUPTED", session.Runtime.StopSummary.StopKind)
	}
	if session.Runtime.StopSummary.LatestDispatch == nil ||
		session.Runtime.StopSummary.LatestDispatch.Status != factoryapi.FactoryDispatchStatusINTERRUPTED {
		t.Fatalf("session latestDispatch = %#v, want INTERRUPTED dispatch context", session.Runtime.StopSummary.LatestDispatch)
	}
	if session.Runtime.StopSummary.LatestResultSummary == nil ||
		strings.TrimSpace(*session.Runtime.StopSummary.LatestResultSummary) == "" {
		t.Fatalf("session latestResultSummary = %#v, want interrupted stop explanation", session.Runtime.StopSummary.LatestResultSummary)
	}

	work := runNamedGoalOperatorWorkShowCLIJSON(t, ctx, binaryPath, baseURL, workID)
	if work.StopSummary == nil {
		t.Fatalf("work show = %#v, want stopSummary on interrupted goal", work)
	}
	if work.StopSummary.StopKind != factoryapi.FactoryStopKind("INTERRUPTED") {
		t.Fatalf("work stopKind = %q, want INTERRUPTED", work.StopSummary.StopKind)
	}
	if work.StopSummary.LatestDispatch == nil ||
		work.StopSummary.LatestDispatch.Status != factoryapi.FactoryDispatchStatusINTERRUPTED {
		t.Fatalf("work latestDispatch = %#v, want INTERRUPTED dispatch context", work.StopSummary.LatestDispatch)
	}

	session = support.GetDefaultSession(t, server.URL())
	if markingContainsNamedGoalRoutingWorkAtPlace(session, workID, "goal:complete") {
		t.Fatalf("interrupted work %q reached goal:complete", workID)
	}
	if !markingContainsNamedGoalRoutingWorkAtPlace(session, workID, "goal:interrupted") {
		t.Fatalf("marking missing goal:interrupted token for work %q", workID)
	}
}

func TestNamedGoalOperatorControls_PauseResumeRecordsLifecycleControlEventsForReplay(t *testing.T) {
	if testing.Short() {
		t.Skip("slow named @you/goal operator replay smoke")
	}

	factoryDir := materializeNamedGoalFactoryForRoutingSmoke(t)
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "accepted",
	})
	server := startNamedGoalRoutingAPIServer(t, factoryDir, mockWorkersPath)

	postNamedGoalOperatorLifecycleControl(
		t,
		server.URL(),
		factoryapi.FactorySessionLifecycleControlKindPause,
	)
	submitted := submitNamedGoalRoutingWork(t, server, "replay-paused-submit", "customer goal request text")
	postNamedGoalOperatorLifecycleControl(
		t,
		server.URL(),
		factoryapi.FactorySessionLifecycleControlKindResume,
	)
	waitForNamedGoalRoutingWorkAtState(
		t,
		server,
		[]string{stringPointerValue(submitted.WorkId)},
		"complete",
		15*time.Second,
	)

	events := server.GetFactoryEvents(t)
	lifecycleControls := filterNamedGoalOperatorLifecycleControlEvents(events)
	if len(lifecycleControls) < 2 {
		t.Fatalf("SESSION_LIFECYCLE_CONTROL events = %d, want pause and resume", len(lifecycleControls))
	}
	pauseEvent := lifecycleControls[len(lifecycleControls)-2]
	resumeEvent := lifecycleControls[len(lifecycleControls)-1]
	if pauseEvent.Type != factoryapi.FactoryEventTypeSessionLifecycleControl {
		t.Fatalf("pause event type = %q, want SESSION_LIFECYCLE_CONTROL", pauseEvent.Type)
	}
	if resumeEvent.Type != factoryapi.FactoryEventTypeSessionLifecycleControl {
		t.Fatalf("resume event type = %q, want SESSION_LIFECYCLE_CONTROL", resumeEvent.Type)
	}
}

func postNamedGoalOperatorLifecycleControl(
	t *testing.T,
	baseURL string,
	operation factoryapi.FactorySessionLifecycleControlKind,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	pathSegment := "pause"
	if operation == factoryapi.FactorySessionLifecycleControlKindResume {
		pathSegment = "resume"
	}
	endpoint := strings.TrimSuffix(baseURL, "/") +
		"/factory-sessions/" + factorysessions.DefaultSessionID + "/" + pathSegment
	resp, err := http.Post(endpoint, "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("POST %s: %v", pathSegment, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", pathSegment, resp.StatusCode, string(payload))
	}
	var decoded factoryapi.FactorySessionLifecycleControlResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode %s response: %v", pathSegment, err)
	}
	return decoded
}

func runNamedGoalOperatorSessionLifecycleCLIJSON(
	t *testing.T,
	ctx context.Context,
	binaryPath string,
	baseURL string,
	operation string,
) factoryapi.FactorySessionLifecycleControlResponse {
	t.Helper()

	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--json",
		"--server", baseURL,
		"session",
		operation,
		factorysessions.DefaultSessionID,
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("you session %s: %v\nstdout:\n%s\nstderr:\n%s", operation, err, stdout.String(), stderr.String())
	}
	var response factoryapi.FactorySessionLifecycleControlResponse
	if err := json.Unmarshal(bytes.TrimSpace([]byte(stdout.String())), &response); err != nil {
		t.Fatalf("decode session %s JSON: %v\nstdout:\n%s", operation, err, stdout.String())
	}
	return response
}

func runNamedGoalOperatorSessionShowCLIJSON(
	t *testing.T,
	ctx context.Context,
	binaryPath string,
	baseURL string,
) factoryapi.FactorySession {
	t.Helper()

	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--json",
		"--server", baseURL,
		"session",
		"show",
		factorysessions.DefaultSessionID,
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("you session show: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	var session factoryapi.FactorySession
	if err := json.Unmarshal(bytes.TrimSpace([]byte(stdout.String())), &session); err != nil {
		t.Fatalf("decode session show JSON: %v\nstdout:\n%s", err, stdout.String())
	}
	return session
}

func runNamedGoalOperatorWorkShowCLIJSON(
	t *testing.T,
	ctx context.Context,
	binaryPath string,
	baseURL string,
	workID string,
) factoryapi.Work {
	t.Helper()

	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--json",
		"--server", baseURL,
		"work",
		"show",
		workID,
	)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("you work show %s: %v\nstdout:\n%s\nstderr:\n%s", workID, err, stdout.String(), stderr.String())
	}
	var work factoryapi.Work
	if err := json.Unmarshal(bytes.TrimSpace([]byte(stdout.String())), &work); err != nil {
		t.Fatalf("decode work show JSON: %v\nstdout:\n%s", err, stdout.String())
	}
	return work
}

func writePackagedGoalSlowPlannerTopologyMockWorkers(t *testing.T, opts packagedGoalTopologyMockOptions) string {
	t.Helper()

	checkerOutput := strings.TrimSpace(opts.checkerOutput)
	if checkerOutput == "" {
		checkerOutput = "plain"
	}
	reviewerWorkstation := strings.TrimSpace(opts.reviewerWorkstation)
	if reviewerWorkstation == "" {
		reviewerWorkstation = publicGoal.PackagedReviewWorkstationName
	}
	reviewerOutput := strings.TrimSpace(opts.reviewerOutput)
	if reviewerOutput == "" {
		reviewerOutput = "accepted"
	}

	cfg := workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      "goal-planner",
				WorkstationName: publicGoal.PackagedPlanWorkstationName,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: "/bin/sh",
					Args:    []string{"-c", "sleep 2"},
				},
			},
			{
				WorkerName:      "goal-executor",
				WorkstationName: publicGoal.PackagedExecuteWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-checker",
				WorkstationName: publicGoal.PackagedCheckWorkstationName,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: "/bin/echo",
					Args:    []string{checkerOutput},
				},
			},
			{
				WorkerName:      "goal-reviewer",
				WorkstationName: reviewerWorkstation,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: "/bin/echo",
					Args:    []string{reviewerOutput},
				},
			},
		},
	}
	return writeMockWorkersConfigFile(t, cfg, "mock-workers-packaged-goal-operator-slow-planner.json")
}

func filterNamedGoalOperatorLifecycleControlEvents(events []factoryapi.FactoryEvent) []factoryapi.FactoryEvent {
	var lifecycleControls []factoryapi.FactoryEvent
	for _, event := range events {
		if event.Type == factoryapi.FactoryEventTypeSessionLifecycleControl {
			lifecycleControls = append(lifecycleControls, event)
		}
	}
	return lifecycleControls
}
