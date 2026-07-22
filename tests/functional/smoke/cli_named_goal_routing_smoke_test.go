package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestNamedGoalRouting_AcceptedCompletesWithPrimaryResult(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal routing smoke")
	}

	response, err := runNamedGoalRoutingInvocationCLIJSON(
		t,
		writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
			reviewerOutput: "accepted",
		}),
		fmt.Sprintf("functional-smoke-goal-routing-accepted-%d", time.Now().UnixNano()),
	)
	if err != nil {
		t.Fatalf("accepted routing invocation: %v", err)
	}
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want COMPLETED", response.Status)
	}
	if got := invocationPrimaryResultText(t, response); got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("primaryResult = %q, want %q", got, packagedGoalMockWorkerAcceptedSummary)
	}
}

func TestNamedGoalRouting_ClassifierNonSuccessOutcomesSurfaceDistinctStates(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal routing smoke")
	}

	cases := []struct {
		name          string
		reviewerLabel string
		wantErrorCode factoryapi.InvocationResponseErrorCode
		wantWorkState string
	}{
		{
			name:          "blocked",
			reviewerLabel: "blocked",
			wantErrorCode: factoryapi.InvocationResponseErrorCode("INVOCATION_BLOCKED"),
			wantWorkState: "goal:blocked",
		},
		{
			name:          "needs_human",
			reviewerLabel: "needs_human",
			wantErrorCode: factoryapi.InvocationResponseErrorCode("INVOCATION_NEEDS_HUMAN"),
			wantWorkState: "goal:needs-human",
		},
		{
			name:          "failed",
			reviewerLabel: "failed",
			wantErrorCode: factoryapi.InvocationResponseErrorCode("INVOCATION_RUNTIME_FAILURE"),
			wantWorkState: "goal:failed",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
				reviewerOutput: tc.reviewerLabel,
			})
			goalText := fmt.Sprintf("functional-smoke-goal-routing-%s-%d", tc.name, time.Now().UnixNano())

			response, err := runNamedGoalRoutingInvocationCLIJSON(t, mockWorkersPath, goalText)
			if err == nil {
				t.Fatal("expected routing failure without primary result")
			}
			if response.Status != factoryapi.InvocationTerminalStatusFailed {
				t.Fatalf("status = %q, want FAILED", response.Status)
			}
			if response.ErrorCode == nil || *response.ErrorCode != tc.wantErrorCode {
				t.Fatalf("errorCode = %#v, want %q", response.ErrorCode, tc.wantErrorCode)
			}
			if response.PrimaryResult != nil {
				t.Fatalf("primaryResult = %#v, want nil on non-success routing", response.PrimaryResult)
			}
			if response.WorkState == nil || *response.WorkState != tc.wantWorkState {
				t.Fatalf("workState = %#v, want %q", response.WorkState, tc.wantWorkState)
			}
			if response.WorkId == nil || strings.TrimSpace(*response.WorkId) == "" {
				t.Fatalf("workId = %#v, want populated work id", response.WorkId)
			}
		})
	}
}

func TestNamedGoalRouting_InterruptedSuppressesSuccessPrimaryResult(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal routing smoke")
	}

	dir := materializeNamedGoalFactoryForRoutingSmoke(t)
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "interrupted",
	})
	goalText := fmt.Sprintf("functional-smoke-goal-routing-interrupted-%d", time.Now().UnixNano())

	server := startNamedGoalRoutingAPIServer(t, dir, mockWorkersPath)
	submitted := submitNamedGoalRoutingWork(t, server, "interrupted-routing-submit", goalText)
	workID := stringPointerValue(submitted.WorkId)
	waitForNamedGoalRoutingWorkAtState(t, server, []string{workID}, "interrupted", 15*time.Second)

	interrupted := getNamedGoalRoutingWorkByIDOnServer(t, server, workID)
	if generatedWorkStateName(interrupted.State) != "interrupted" {
		t.Fatalf("work state = %#v, want interrupted", interrupted.State)
	}

	session := support.GetDefaultSession(t, server.URL())
	if markingContainsNamedGoalRoutingWorkAtPlace(session, workID, "goal:complete") {
		t.Fatalf("interrupted routing reached goal:complete for work %q", workID)
	}
	if !markingContainsNamedGoalRoutingWorkAtPlace(session, workID, "goal:interrupted") {
		t.Fatalf("marking missing goal:interrupted token for work %q", workID)
	}
}

func TestNamedGoalRouting_UnknownClassifierLabelRoutesToFailed(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal routing smoke")
	}

	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		reviewerOutput: "MAYBE",
	})
	goalText := fmt.Sprintf("functional-smoke-goal-routing-unknown-%d", time.Now().UnixNano())

	response, err := runNamedGoalRoutingInvocationCLIJSON(t, mockWorkersPath, goalText)
	if err == nil {
		t.Fatal("expected unknown classifier label to fail without primary result")
	}
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("status = %q, want FAILED", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.InvocationResponseErrorCode("INVOCATION_RUNTIME_FAILURE") {
		t.Fatalf("errorCode = %#v, want INVOCATION_RUNTIME_FAILURE", response.ErrorCode)
	}
	if response.WorkState == nil || *response.WorkState != "goal:failed" {
		t.Fatalf("workState = %#v, want goal:failed", response.WorkState)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("primaryResult = %#v, want nil on malformed routing", response.PrimaryResult)
	}
}

func TestNamedGoalRouting_ReworkLoopsBackThenCompletesWithGoalContext(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal routing smoke")
	}

	dir := materializeNamedGoalFactoryForRoutingSmoke(t)
	mockWorkersPath := writePackagedGoalBuiltinTopologySequencedReviewerMockWorkers(t, "needs_changes", "accepted")
	goalText := fmt.Sprintf("functional-smoke-goal-routing-rework-%d", time.Now().UnixNano())

	server := startNamedGoalRoutingAPIServer(t, dir, mockWorkersPath)
	response := postNamedGoalRoutingInvocationOnServer(t, server, goalText)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("status = %q, want COMPLETED after rework loop", response.Status)
	}
	if got := invocationPrimaryResultText(t, response); got != packagedGoalMockWorkerAcceptedSummary {
		t.Fatalf("primaryResult = %q, want %q", got, packagedGoalMockWorkerAcceptedSummary)
	}

	reviewDispatches := countNamedGoalRoutingDispatchesOnServer(t, server, publicGoal.PackagedReviewWorkstationName)
	if reviewDispatches < 2 {
		t.Fatalf("review dispatch count = %d, want at least 2 for needs_changes rework loop", reviewDispatches)
	}
	if !namedGoalRoutingDispatchHistoryIncludesLabel(
		t,
		server,
		publicGoal.PackagedReviewWorkstationName,
		"needs_changes",
	) {
		t.Fatal("dispatch history missing needs_changes review classification before accepted completion")
	}

	work := findNamedGoalRoutingWorkAtCompleteState(t, server)
	if !markingContainsNamedGoalRoutingWorkAtPlace(support.GetDefaultSession(t, server.URL()), stringPointerValue(work.WorkId), "goal:complete") {
		t.Fatalf("completed work %q missing goal:complete token after rework", stringPointerValue(work.WorkId))
	}
}

func TestNamedGoalRouting_StructuredUnknownDecisionRoutesToFailed(t *testing.T) {
	if testing.Short() {
		t.Skip("slow CLI named @you/goal routing smoke")
	}

	dir := materializeNamedGoalFactoryForRoutingSmoke(t)
	writePackagedGoalCheckWorkstationReviewModeForSmoke(t, dir, publicGoal.PackagedReviewModeStructuredLabel)
	envelope := `{"decision":"MAYBE","feedback":"unknown structured decision"}`
	mockWorkersPath := writePackagedGoalBuiltinTopologyMockWorkers(t, packagedGoalTopologyMockOptions{
		checkerOutput:       "structured",
		reviewerWorkstation: publicGoal.PackagedStructuredReviewWorkstationName,
		reviewerOutput:      envelope,
	})
	goalText := fmt.Sprintf("functional-smoke-goal-routing-structured-unknown-%d", time.Now().UnixNano())

	server := startNamedGoalRoutingAPIServer(t, dir, mockWorkersPath)
	response := postNamedGoalRoutingInvocationOnServer(t, server, goalText)
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("status = %q, want FAILED", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.InvocationResponseErrorCode("INVOCATION_RUNTIME_FAILURE") {
		t.Fatalf("errorCode = %#v, want INVOCATION_RUNTIME_FAILURE", response.ErrorCode)
	}
	if response.WorkState == nil || *response.WorkState != "goal:failed" {
		t.Fatalf("workState = %#v, want goal:failed", response.WorkState)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("primaryResult = %#v, want nil on unknown structured decision", response.PrimaryResult)
	}
}

type packagedGoalTopologyMockOptions struct {
	checkerOutput       string
	reviewerWorkstation string
	reviewerOutput      string
}

func materializeNamedGoalFactoryForRoutingSmoke(t *testing.T) string {
	t.Helper()

	return support.InstallPackagedFactory(t, t.TempDir(), publicGoal.PackagedFactoryName)
}

func writePackagedGoalCheckWorkstationReviewModeForSmoke(t *testing.T, dir, reviewMode string) {
	t.Helper()

	support.WriteWorkstationConfig(t, dir, publicGoal.PackagedCheckWorkstationName, `---
type: CLASSIFIER_WORKSTATION
env:
  `+publicGoal.PackagedCheckReviewModeEnvVar+`: "`+reviewMode+`"
---
Review packaged goal work.
`)
}

func writePackagedGoalBuiltinTopologyMockWorkers(t *testing.T, opts packagedGoalTopologyMockOptions) string {
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
	checkerCommand, checkerArgs := mockWorkerEchoCommand(checkerOutput)
	reviewerCommand, reviewerArgs := mockWorkerEchoCommand(reviewerOutput)

	cfg := workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      "goal-planner",
				WorkstationName: publicGoal.PackagedPlanWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
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
					Command: checkerCommand,
					Args:    checkerArgs,
				},
			},
			{
				WorkerName:      "goal-reviewer",
				WorkstationName: reviewerWorkstation,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: reviewerCommand,
					Args:    reviewerArgs,
				},
			},
		},
	}
	return writeMockWorkersConfigFile(t, cfg, "mock-workers-packaged-goal-routing.json")
}

func mockWorkerEchoCommand(output string) (string, []string) {
	if runtime.GOOS == "windows" {
		literal := strings.ReplaceAll(output, "'", "''")
		return "powershell.exe", []string{
			"-NoProfile",
			"-NonInteractive",
			"-Command",
			"[Console]::Out.Write('" + literal + "')",
		}
	}
	return "/bin/echo", []string{output}
}

func writePackagedGoalBuiltinTopologySequencedReviewerMockWorkers(t *testing.T, reviewerOutputs ...string) string {
	t.Helper()

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "goal-reviewer-sequenced.sh")
	counterPath := filepath.Join(scriptDir, "goal-reviewer-sequenced.count")
	lines := []string{
		"#!/bin/sh",
		"count=0",
		"if [ -f \"" + counterPath + "\" ]; then",
		"  count=$(cat \"" + counterPath + "\")",
		"fi",
		"case \"$count\" in",
	}
	for idx, output := range reviewerOutputs {
		lines = append(lines, "  "+strconv.Itoa(idx)+") printf '%s' '"+output+"' ;;")
	}
	fallback := "accepted"
	if len(reviewerOutputs) > 0 {
		fallback = reviewerOutputs[len(reviewerOutputs)-1]
	}
	lines = append(lines,
		"  *) printf '%s' '"+fallback+"' ;;",
		"esac",
		"printf '%s' $((count + 1)) > \""+counterPath+"\"",
	)
	if err := os.WriteFile(scriptPath, []byte(strings.Join(lines, "\n")+"\n"), 0o755); err != nil {
		t.Fatalf("write sequenced goal reviewer script: %v", err)
	}

	cfg := workers.MockWorkersConfig{
		UnmatchedDispatchPolicy: workers.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      "goal-planner",
				WorkstationName: publicGoal.PackagedPlanWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
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
					Args:    []string{"plain"},
				},
			},
			{
				WorkerName:      "goal-reviewer",
				WorkstationName: publicGoal.PackagedReviewWorkstationName,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: scriptPath,
				},
			},
		},
	}
	return writeMockWorkersConfigFile(t, cfg, "mock-workers-packaged-goal-routing-sequenced.json")
}

func writeMockWorkersConfigFile(t *testing.T, cfg workers.MockWorkersConfig, fileName string) string {
	t.Helper()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal mock-workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), fileName)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write mock-workers config: %v", err)
	}
	return path
}

func runNamedGoalRoutingInvocationCLIJSON(
	t *testing.T,
	mockWorkersPath string,
	goalText string,
) (factoryapi.InvocationResponse, error) {
	t.Helper()

	homeDir := t.TempDir()
	support.InstallPackagedFactory(t, homeDir, publicGoal.PackagedFactoryName)

	port, err := reserveLocalTCPPort()
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	binaryPath := buildYouCLIBinary(t)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		binaryPath,
		"--json",
		"run",
		"--named", publicGoal.PackagedFactoryName,
		"--with-mock-workers",
		"--no-record",
		"--server", baseURL,
		"--quiet",
		mockWorkersPath,
		goalText,
	)
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "HOME="+homeDir, "USERPROFILE="+homeDir)

	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()

	var response factoryapi.InvocationResponse
	if stdout.Len() > 0 {
		if err := json.Unmarshal(bytes.TrimSpace([]byte(stdout.String())), &response); err != nil {
			t.Fatalf("decode CLI invocation response: %v\nstdout:\n%s", err, stdout.String())
		}
	}
	if runErr != nil && stderr.Len() > 0 {
		runErr = fmt.Errorf("%w\nstderr:\n%s", runErr, stderr.String())
	}
	return response, runErr
}

func postNamedGoalRoutingInvocationOnServer(
	t *testing.T,
	server *support.FunctionalAPIServer,
	goalText string,
) factoryapi.InvocationResponse {
	t.Helper()

	body, err := json.Marshal(factoryapi.InvocationRequest{
		SourceKind: invocationTextSourceKindPtr(),
		Content:    invocationTextContentPtr(goalText),
	})
	if err != nil {
		t.Fatalf("marshal invocation request: %v", err)
	}
	response, err := http.Post(
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/"+factorysessions.DefaultSessionID+"/invocations",
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /factory-sessions/~default/invocations: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("POST /factory-sessions/~default/invocations status = %d, want 200: %s", response.StatusCode, string(payload))
	}

	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode invocation response: %v", err)
	}
	return decoded
}

func startNamedGoalRoutingAPIServer(t *testing.T, factoryDir, mockWorkersPath string) *support.FunctionalAPIServer {
	t.Helper()

	payload, err := os.ReadFile(mockWorkersPath)
	if err != nil {
		t.Fatalf("read customer mock-workers config: %v", err)
	}
	var mockWorkersConfig workers.MockWorkersConfig
	if err := json.Unmarshal(payload, &mockWorkersConfig); err != nil {
		t.Fatalf("decode customer mock-workers config: %v", err)
	}
	return support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                factoryDir,
		WaitForServiceModeRuntime: true,
		MockWorkersConfig:         &mockWorkersConfig,
	})
}

func getNamedGoalRoutingWorkByIDOnServer(
	t *testing.T,
	server *support.FunctionalAPIServer,
	workID string,
) factoryapi.Work {
	t.Helper()

	response, err := http.Get(support.DefaultSessionWorkURL(server.URL(), "/work/"+workID))
	if err != nil {
		t.Fatalf("GET /work/%s: %v", workID, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("GET /work/%s status = %d, want 200: %s", workID, response.StatusCode, string(payload))
	}
	var work factoryapi.Work
	if err := json.NewDecoder(response.Body).Decode(&work); err != nil {
		t.Fatalf("decode work response: %v", err)
	}
	return work
}

func countNamedGoalRoutingDispatchesOnServer(
	t *testing.T,
	server *support.FunctionalAPIServer,
	workstationName string,
) int {
	t.Helper()

	count := 0
	for _, dispatch := range support.ObserveDispatchEvents(t, server.GetFactoryEvents(t)) {
		if dispatch.Request.TransitionId == workstationName {
			count++
		}
	}
	return count
}

func namedGoalRoutingDispatchHistoryIncludesLabel(
	t *testing.T,
	server *support.FunctionalAPIServer,
	workstationName string,
	label string,
) bool {
	for _, dispatch := range support.ObserveDispatchEvents(t, server.GetFactoryEvents(t)) {
		if dispatch.Request.TransitionId != workstationName || dispatch.Response == nil {
			continue
		}
		if dispatch.Response.SelectedClassificationLabel != nil &&
			*dispatch.Response.SelectedClassificationLabel == label {
			return true
		}
	}
	return false
}

func submitNamedGoalRoutingWork(
	t *testing.T,
	server *support.FunctionalAPIServer,
	name string,
	text string,
) factoryapi.SubmitWorkResponse {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"name":         name,
		"workTypeName": publicGoal.PackagedGoalWorkTypeName,
		"items": []map[string]any{{
			"type": "text",
			"text": text,
		}},
	})
	if err != nil {
		t.Fatalf("marshal goal submit request: %v", err)
	}
	resp, err := http.Post(
		support.DefaultSessionWorkURL(server.URL(), "/work"),
		"application/json",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatalf("POST /work: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /work status = %d, want 201: %s", resp.StatusCode, string(payload))
	}
	var submitted factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	if strings.TrimSpace(stringPointerValue(submitted.WorkId)) == "" {
		t.Fatalf("submit response = %#v, want work id", submitted)
	}
	return submitted
}

func waitForNamedGoalRoutingWorkAtState(
	t *testing.T,
	server *support.FunctionalAPIServer,
	workIDs []string,
	stateName string,
	timeout time.Duration,
) {
	t.Helper()

	want := make(map[string]bool, len(workIDs))
	for _, workID := range workIDs {
		want[workID] = true
	}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := listNamedGoalRoutingWork(t, server)
		found := 0
		for _, item := range work.Results {
			workID := stringPointerValue(item.WorkId)
			if want[workID] && generatedWorkStateName(item.State) == stateName {
				found++
			}
		}
		if found == len(want) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	work := listNamedGoalRoutingWork(t, server)
	t.Fatalf("timed out waiting for work IDs %v at state %q; last work response: %#v", workIDs, stateName, work)
}

func listNamedGoalRoutingWork(t *testing.T, server *support.FunctionalAPIServer) factoryapi.ListWorkResponse {
	t.Helper()

	response, err := http.Get(support.DefaultSessionWorkURL(server.URL(), "/work"))
	if err != nil {
		t.Fatalf("GET /work: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		payload, _ := io.ReadAll(response.Body)
		t.Fatalf("GET /work status = %d, want 200: %s", response.StatusCode, string(payload))
	}
	var work factoryapi.ListWorkResponse
	if err := json.NewDecoder(response.Body).Decode(&work); err != nil {
		t.Fatalf("decode work list: %v", err)
	}
	return work
}

func findNamedGoalRoutingWorkAtCompleteState(
	t *testing.T,
	server *support.FunctionalAPIServer,
) factoryapi.Work {
	t.Helper()

	work := listNamedGoalRoutingWork(t, server)
	for _, item := range work.Results {
		if generatedWorkStateName(item.State) == "complete" {
			return item
		}
	}
	t.Fatalf("work list = %#v, want one completed goal work item", work)
	return factoryapi.Work{}
}

func generatedWorkStateName(state *factoryapi.WorkState) string {
	if state == nil {
		return ""
	}
	return state.Name
}

func markingContainsNamedGoalRoutingWorkAtPlace(
	session factoryapi.FactorySession,
	workID string,
	placeID string,
) bool {
	return support.SessionHasWorkAtPlace(session, workID, placeID)
}
