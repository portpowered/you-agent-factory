package runtime_api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/factory/validation"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
	"github.com/portpowered/infinite-you/pkg/service"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestSessionInvocationAPI_PackagedGoalReturnsExplicitSummaryPrimaryResult(t *testing.T) {
	dir := scaffoldPackagedGoalInvocationFactory(t)
	summary := "mock worker accepted"
	core, observedLogs := observer.New(zap.InfoLevel)
	server := startFunctionalServerWithConfig(t, dir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.Logger = zap.New(core)
	})

	submitted := "customer goal request text"
	response := postInvocation(t, server.URL(), textInvocationRequest(t, submitted, nil))
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED", response.Status)
	}
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("invocation primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	if part.Text != summary {
		t.Fatalf("primaryResult text = %q, want summary %q", part.Text, summary)
	}
	if part.Text == submitted {
		t.Fatal("primaryResult echoed submitted goal text")
	}

	submittedLogs := observedLogs.FilterMessage("factory session invocation submitted").All()
	if len(submittedLogs) != 1 {
		t.Fatalf("submitted invocation log count = %d, want 1", len(submittedLogs))
	}
	submittedFields := submittedLogs[0].ContextMap()
	if got := submittedFields["invocation_return_policy_mode"]; got != "authored" {
		t.Fatalf("submitted invocation_return_policy_mode = %#v, want authored", got)
	}
	if got := submittedFields["policy_resolution_path"]; got != "explicit_scoped_terminal_match" {
		t.Fatalf("submitted policy_resolution_path = %#v, want explicit_scoped_terminal_match", got)
	}
}

func TestSessionInvocationAPI_PackagedGoalUnresolvedPrimaryResultReturnsFailedStatus(t *testing.T) {
	dir := scaffoldPackagedGoalInvocationFactory(t)
	server := startFunctionalServerWithConfig(t, dir, false, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.ProviderCommandRunnerOverride = support.NewStaticSuccessCommandRunner("goal output without stop token")
	})

	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke packaged goal", nil))
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.INVOCATIONPRIMARYRESULTUNRESOLVED {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_PRIMARY_RESULT_UNRESOLVED", response.ErrorCode)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on unresolved output", response.PrimaryResult)
	}
}

func TestSessionInvocationAPI_PackagedGoalBlockedReturnsBlockedStatusDetails(t *testing.T) {
	server := startPackagedGoalBuiltInTopologyInvocationServer(t, "blocked")

	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke packaged goal", nil))
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.InvocationResponseErrorCode("INVOCATION_BLOCKED") {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_BLOCKED", response.ErrorCode)
	}
	if response.Message == nil || !strings.Contains(*response.Message, `state "goal:blocked"`) {
		t.Fatalf("invocation message = %#v, want goal:blocked state detail", response.Message)
	}
	if response.SessionId == nil || *response.SessionId != "~default" {
		t.Fatalf("invocation sessionId = %#v, want ~default", response.SessionId)
	}
	if response.WorkId == nil || *response.WorkId == "" {
		t.Fatalf("invocation workId = %#v, want populated work id", response.WorkId)
	}
	if response.WorkName == nil || *response.WorkName == "" {
		t.Fatalf("invocation workName = %#v, want populated work name", response.WorkName)
	}
	if response.WorkState == nil || *response.WorkState != "goal:blocked" {
		t.Fatalf("invocation workState = %#v, want goal:blocked", response.WorkState)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on blocked output", response.PrimaryResult)
	}
}

func TestSessionInvocationAPI_PackagedGoalNeedsHumanReturnsNeedsHumanStatusDetails(t *testing.T) {
	server := startPackagedGoalBuiltInTopologyInvocationServer(t, "needs_human")

	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke packaged goal", nil))
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.InvocationResponseErrorCode("INVOCATION_NEEDS_HUMAN") {
		t.Fatalf("invocation errorCode = %#v, want INVOCATION_NEEDS_HUMAN", response.ErrorCode)
	}
	if response.Message == nil || !strings.Contains(*response.Message, "needs human input") || !strings.Contains(*response.Message, `state "goal:needs-human"`) {
		t.Fatalf("invocation message = %#v, want needs-human explanation", response.Message)
	}
	if response.SessionId == nil || *response.SessionId != "~default" {
		t.Fatalf("invocation sessionId = %#v, want ~default", response.SessionId)
	}
	if response.WorkId == nil || *response.WorkId == "" {
		t.Fatalf("invocation workId = %#v, want populated work id", response.WorkId)
	}
	if response.WorkName == nil || *response.WorkName == "" {
		t.Fatalf("invocation workName = %#v, want populated work name", response.WorkName)
	}
	if response.WorkState == nil || *response.WorkState != "goal:needs-human" {
		t.Fatalf("invocation workState = %#v, want goal:needs-human", response.WorkState)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on needs-human output", response.PrimaryResult)
	}
}

func TestSessionInvocationAPI_PackagedGoalFailedReturnsFailedStatusDetails(t *testing.T) {
	dir, _ := scaffoldPackagedGoalBuiltInTopologyFactory(t)
	server := startFunctionalServerWithConfig(t, dir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.MockWorkersConfig = packagedGoalBuiltInTopologyMockWorkersConfigForRealChecker(
			goal.PackagedReviewWorkstationName,
			"failed",
		)
	})

	response := postInvocation(t, server.URL(), textInvocationRequest(t, "invoke packaged goal", nil))
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("invocation status = %q, want FAILED", response.Status)
	}
	if response.ErrorCode == nil || *response.ErrorCode != factoryapi.InvocationResponseErrorCode("INVOCATION_RUNTIME_FAILURE") {
		gotCode := "<nil>"
		if response.ErrorCode != nil {
			gotCode = string(*response.ErrorCode)
		}
		t.Fatalf("invocation errorCode = %q, want INVOCATION_RUNTIME_FAILURE", gotCode)
	}
	if response.Message == nil || !strings.Contains(*response.Message, "invocation failed") || !strings.Contains(*response.Message, `state "goal:failed"`) {
		t.Fatalf("invocation message = %#v, want failed goal explanation", response.Message)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("invocation primaryResult = %#v, want nil on failed output", response.PrimaryResult)
	}
}

func TestPackagedGoalBuiltInTopology_SubmitWhilePausedResumesThroughSessionControl(t *testing.T) {
	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), goal.PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	writePackagedGoalBuiltInTopologyFixtureFiles(t, dir, loaded.FactoryConfig())

	server := startFunctionalServerWithConfig(t, dir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.MockWorkersConfig = packagedGoalBuiltInTopologyMockWorkersConfigForRealChecker(
			goal.PackagedReviewWorkstationName,
			"accepted",
		)
	})

	pause := postJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		server.URL()+"/factory-sessions/~default/pause",
		factoryapi.FactorySessionLifecycleControlRequest{},
		"pause packaged goal session",
	)
	if pause.Operation != factoryapi.FactorySessionLifecycleControlKindPause || pause.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("pause response = %#v, want accepted pause", pause)
	}

	submitted := submitGeneratedGoalWork(t, server.URL(), "paused-goal-submit", "customer goal request text")
	snapshot := server.GetEngineStateSnapshot(t)
	if markingContainsWorkAtPlace(&snapshot.Marking, stringPointerValue(submitted.WorkId), "goal:init") {
		t.Fatalf("paused submit reached goal:init while session was paused: %#v", snapshot.Marking.Tokens)
	}
	if markingContainsWorkAtPlace(&snapshot.Marking, stringPointerValue(submitted.WorkId), "goal:complete") {
		t.Fatalf("paused submit reached goal:complete before resume: %#v", snapshot.Marking.Tokens)
	}

	resume := postJSON[factoryapi.FactorySessionLifecycleControlResponse](
		t,
		server.URL()+"/factory-sessions/~default/resume",
		factoryapi.FactorySessionLifecycleControlRequest{},
		"resume packaged goal session",
	)
	if resume.Operation != factoryapi.FactorySessionLifecycleControlKindResume || resume.Outcome != factoryapi.FactorySessionLifecycleControlOutcomeAccepted {
		t.Fatalf("resume response = %#v, want accepted resume", resume)
	}

	completed := waitForGeneratedWorkIDsComplete(t, server.URL(), []string{stringPointerValue(submitted.WorkId)}, 15*time.Second)
	if len(completed) != 1 || generatedWorkStateName(completed[0].State) != "complete" {
		t.Fatalf("completed work = %#v, want one completed goal after resume", completed)
	}
}

func TestPackagedGoalBuiltInTopology_BlockedGoalRecoversThroughExistingWorkMoveControl(t *testing.T) {
	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), goal.PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	writePackagedGoalBuiltInTopologyFixtureFiles(t, dir, loaded.FactoryConfig())

	server := startFunctionalServerWithConfig(t, dir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.MockWorkersConfig = packagedGoalBuiltInTopologySequencedReviewerMockWorkersConfig(t, dir, "blocked", "accepted")
	})

	submitted := submitGeneratedGoalWork(t, server.URL(), "blocked-goal-submit", "customer goal request text")
	waitForGeneratedWorkIDsAtState(t, server.URL(), []string{stringPointerValue(submitted.WorkId)}, "blocked", 15*time.Second)

	moved := postGeneratedMoveWork(t, server.URL(), stringPointerValue(submitted.WorkId), "review")
	if generatedWorkStateName(moved.State) != "review" {
		t.Fatalf("moved goal work = %#v, want review state", moved)
	}

	completed := waitForGeneratedWorkIDStateName(t, server.URL(), stringPointerValue(submitted.WorkId), "complete", 15*time.Second)
	if generatedWorkStateName(completed.State) != "complete" {
		t.Fatalf("completed work = %#v, want blocked goal to complete after move", completed)
	}
}

func TestPackagedGoalBuiltInTopology_InterruptedGoalRecoversThroughExistingWorkMoveControl(t *testing.T) {
	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), goal.PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	writePackagedGoalBuiltInTopologyFixtureFiles(t, dir, loaded.FactoryConfig())

	server := startFunctionalServerWithConfig(t, dir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.MockWorkersConfig = packagedGoalBuiltInTopologySequencedReviewerMockWorkersConfig(t, dir, "interrupted", "accepted")
	})

	submitted := submitGeneratedGoalWork(t, server.URL(), "interrupted-goal-submit", "customer goal request text")
	workID := stringPointerValue(submitted.WorkId)
	waitForGeneratedWorkIDsAtState(t, server.URL(), []string{workID}, "interrupted", 15*time.Second)
	interrupted := requireGeneratedWorkByID(t, server.URL(), workID)
	if generatedWorkStateName(interrupted.State) != "interrupted" {
		t.Fatalf("interrupted work = %#v, want interrupted state", interrupted)
	}

	moved := postGeneratedMoveWork(t, server.URL(), workID, "review")
	if generatedWorkStateName(moved.State) != "review" {
		t.Fatalf("moved goal work = %#v, want review state", moved)
	}

	completed := waitForGeneratedWorkIDStateName(t, server.URL(), workID, "complete", 15*time.Second)
	if generatedWorkStateName(completed.State) != "complete" {
		t.Fatalf("completed work = %#v, want interrupted goal to complete after move", completed)
	}
}

func TestPackagedGoalBuiltInTopology_PlainReviewLaneEndToEnd(t *testing.T) {
	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), goal.PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	writePackagedGoalBuiltInTopologyFixtureFiles(t, dir, loaded.FactoryConfig())

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithRunAsync(),
		testutil.WithMockWorkersConfig(packagedGoalBuiltInTopologyMockWorkersConfigForRealChecker(
			goal.PackagedReviewWorkstationName,
			"accepted",
		)),
	)
	if err := h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		WorkTypeID: goal.PackagedGoalWorkTypeName,
		TraceID:    "goal-plain-review-trace",
		Content: []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: "customer goal request text",
		}},
	}}); err != nil {
		t.Fatalf("SubmitFull: %v", err)
	}
	h.RunUntilComplete(t, 30*time.Second)
	h.Assert().HasTokenInPlace("goal:complete")

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	var reviewDispatches int
	for _, dispatch := range snapshot.DispatchHistory {
		switch dispatch.WorkstationName {
		case goal.PackagedReviewWorkstationName:
			reviewDispatches++
		case goal.PackagedStructuredReviewWorkstationName:
			t.Fatalf("structured review %q dispatched from plain review-mode run", dispatch.WorkstationName)
		}
	}
	if reviewDispatches != 1 {
		t.Fatalf("plain review dispatch count = %d, want 1", reviewDispatches)
	}
}

func TestPackagedGoalBuiltInTopology_StructuredReviewLaneEndToEnd(t *testing.T) {
	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), goal.PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	writePackagedGoalBuiltInTopologyFixtureFiles(t, dir, loaded.FactoryConfig())
	writePackagedGoalCheckWorkstationReviewMode(t, dir, goal.PackagedReviewModeStructuredLabel)

	envelope := `{"decision":"accepted","feedback":"review ok","output":"mock worker accepted"}`
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithRunAsync(),
		testutil.WithMockWorkersConfig(packagedGoalBuiltInTopologyMockWorkersConfigForRealChecker(
			goal.PackagedStructuredReviewWorkstationName,
			envelope,
		)),
	)
	if err := h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		WorkTypeID: goal.PackagedGoalWorkTypeName,
		TraceID:    "goal-structured-review-trace",
		Content: []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: "customer goal request text",
		}},
	}}); err != nil {
		t.Fatalf("SubmitFull: %v", err)
	}
	h.RunUntilComplete(t, 30*time.Second)
	h.Assert().HasTokenInPlace("goal:complete")

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	var structuredReviewDispatches int
	for _, dispatch := range snapshot.DispatchHistory {
		switch dispatch.WorkstationName {
		case goal.PackagedStructuredReviewWorkstationName:
			structuredReviewDispatches++
		case goal.PackagedReviewWorkstationName:
			t.Fatalf("plain classifier %q dispatched from structured review-mode run", dispatch.WorkstationName)
		}
	}
	if structuredReviewDispatches != 1 {
		t.Fatalf("structured review dispatch count = %d, want 1", structuredReviewDispatches)
	}
}

func TestPackagedGoalBuiltInTopology_StructuredReworkTripsLoopBreaker(t *testing.T) {
	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), goal.PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	writePackagedGoalBuiltInTopologyFixtureFiles(t, dir, loaded.FactoryConfig())
	writePackagedGoalCheckWorkstationReviewMode(t, dir, goal.PackagedReviewModeStructuredLabel)

	envelope := `{"decision":"needs_changes","feedback":"retry with more detail"}`
	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithRunAsync(),
		testutil.WithMockWorkersConfig(packagedGoalBuiltInTopologyMockWorkersConfigForRealChecker(
			goal.PackagedStructuredReviewWorkstationName,
			envelope,
		)),
	)
	if err := h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		WorkTypeID: goal.PackagedGoalWorkTypeName,
		TraceID:    "goal-structured-loop-breaker-trace",
		Content: []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: "customer goal request text",
		}},
	}}); err != nil {
		t.Fatalf("SubmitFull: %v", err)
	}
	h.RunUntilComplete(t, 30*time.Second)
	h.Assert().HasTokenInPlace("goal:failed")

	snapshot, err := h.GetEngineStateSnapshot()
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot: %v", err)
	}
	var structuredReviewDispatches int
	var structuredLoopBreakerDispatches int
	for _, dispatch := range snapshot.DispatchHistory {
		switch dispatch.WorkstationName {
		case goal.PackagedStructuredReviewWorkstationName:
			structuredReviewDispatches++
		case goal.PackagedStructuredLoopBreakerWorkstationName:
			structuredLoopBreakerDispatches++
		}
	}
	if structuredReviewDispatches < 5 {
		t.Fatalf("structured review dispatch count = %d, want at least 5 before exhaustion", structuredReviewDispatches)
	}
	if structuredLoopBreakerDispatches != 1 {
		t.Fatalf("structured loop breaker dispatch count = %d, want 1", structuredLoopBreakerDispatches)
	}
}

func TestPackagedGoalBuiltInTopologyScaffold_PrimaryResultIsExecutionSummaryNotReviewLabel(t *testing.T) {
	dir, _ := scaffoldPackagedGoalBuiltInTopologyFactory(t)
	wantSummary := "mock worker accepted"

	h := testutil.NewServiceTestHarness(t, dir,
		testutil.WithRunAsync(),
		testutil.WithMockWorkersConfig(packagedGoalReviewClassifierMockWorkersConfig("accepted")),
	)
	if err := h.SubmitFull(context.Background(), []interfaces.SubmitRequest{{
		WorkTypeID: goal.PackagedGoalWorkTypeName,
		TraceID:    "goal-topology-trace",
		Content: []interfaces.WorkContentPart{{
			Type: interfaces.WorkContentPartTypeText,
			Text: "customer goal request text",
		}},
	}}); err != nil {
		t.Fatalf("SubmitFull: %v", err)
	}
	h.RunUntilComplete(t, 30*time.Second)
	h.Assert().HasTokenInPlace("goal:complete")

	var summaryText string
	for _, token := range h.Marking().Tokens {
		if token == nil || token.PlaceID != "goal:complete" {
			continue
		}
		if len(token.Color.Content) != 1 || token.Color.Content[0].Type != interfaces.WorkContentPartTypeText {
			t.Fatalf("goal:complete content = %#v, want one text summary part", token.Color.Content)
		}
		summaryText = token.Color.Content[0].Text
		break
	}
	if summaryText == "" {
		t.Fatal("missing goal:complete summary content on terminal token")
	}
	if summaryText != wantSummary {
		t.Fatalf("goal:complete content = %q, want execution summary %q", summaryText, wantSummary)
	}
	if summaryText == "accepted" {
		t.Fatal("goal:complete content returned review classifier label instead of execution summary")
	}
}

func scaffoldPackagedGoalBuiltInTopologyFactory(t *testing.T) (string, *interfaces.InvocationReturnConfig) {
	t.Helper()

	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON([]byte(packagedGoalBuiltInTopologyOpenAPIJSON()))
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}

	dir := t.TempDir()
	factoryvalidation.NormalizeFixtureConfig(cfg)
	data, err := factoryconfig.MarshalCanonicalFactoryConfig(cfg)
	if err != nil {
		t.Fatalf("MarshalCanonicalFactoryConfig: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), data, 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
	writePackagedGoalBuiltInTopologyFixtureFiles(t, dir, cfg)
	return dir, cfg.InvocationReturn
}

func packagedGoalBuiltInTopologyOpenAPIJSON() string {
	return `{
		"name": "@you/goal",
		"invocationReturn": {
			"policy": "EXPLICIT",
			"workTypeName": "goal",
			"terminalState": "complete"
		},
		"workTypes": [{
			"name": "goal",
			"handlingBehavior": ["DEFAULT"],
			"states": [
				{"name": "init", "type": "INITIAL"},
				{"name": "plan", "type": "PROCESSING"},
				{"name": "execute", "type": "PROCESSING"},
				{"name": "check", "type": "PROCESSING"},
				{"name": "review", "type": "PROCESSING"},
				{"name": "complete", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"}
			]
		}],
		"workers": [
			{"name": "goal-planner", "type": "AGENT_WORKER"},
			{"name": "goal-executor", "type": "AGENT_WORKER"},
			{"name": "goal-checker", "type": "SCRIPT_WORKER"},
			{"name": "goal-reviewer", "type": "AGENT_WORKER"}
		],
		"workstations": [
			{
				"name": "plan-goal",
				"type": "AGENT_RUN",
				"worker": "goal-planner",
				"inputs": [{"workType": "goal", "state": "init"}],
				"outputs": [{"workType": "goal", "state": "plan"}],
				"onFailure": [{"workType": "goal", "state": "failed"}]
			},
			{
				"name": "execute-goal",
				"type": "AGENT_RUN",
				"worker": "goal-executor",
				"inputs": [{"workType": "goal", "state": "plan"}],
				"outputs": [{"workType": "goal", "state": "execute"}],
				"onFailure": [{"workType": "goal", "state": "failed"}]
			},
			{
				"name": "check-goal",
				"type": "SCRIPT_RUN",
				"worker": "goal-checker",
				"inputs": [{"workType": "goal", "state": "execute"}],
				"outputs": [{"workType": "goal", "state": "check"}],
				"onFailure": [{"workType": "goal", "state": "failed"}]
			},
			{
				"name": "advance-goal-review",
				"type": "LOGICAL_MOVE",
				"inputs": [{"workType": "goal", "state": "check"}],
				"outputs": [{"workType": "goal", "state": "review"}]
			},
			{
				"name": "review-goal",
				"type": "CLASSIFIER_WORKSTATION",
				"worker": "goal-reviewer",
				"inputs": [{"workType": "goal", "state": "review"}],
				"classificationRoutes": [
					{"label": "accepted", "outputs": [{"workType": "goal", "state": "complete"}]},
					{"label": "failed", "outputs": [{"workType": "goal", "state": "failed"}]}
				],
				"onFailure": [{"workType": "goal", "state": "failed"}]
			}
		]
	}`
}

func writePackagedGoalBuiltInTopologyFixtureFiles(t *testing.T, dir string, cfg *interfaces.FactoryConfig) {
	t.Helper()

	for _, workstation := range cfg.Workstations {
		body := "---\ntype: MODEL_WORKSTATION\n---\nProcess packaged goal work.\n"
		if workstation.Type == interfaces.WorkstationTypeClassify {
			body = "---\ntype: CLASSIFIER_WORKSTATION\n---\nReview packaged goal work.\n"
		}
		if workstation.Type == interfaces.WorkstationTypeLogical {
			continue
		}
		support.WriteWorkstationConfig(t, dir, workstation.Name, body)
	}
	for _, workerName := range []string{"goal-planner", "goal-executor", "goal-reviewer"} {
		support.WriteAgentConfig(
			t,
			dir,
			workerName,
			support.BuildModelWorkerConfig(interfaces.ModelProviderCodex, "gpt-5-codex"),
		)
	}
	checker := goalCheckerWorkerConfig(cfg)
	if checker != nil && checker.Command != "" {
		support.WriteAgentConfig(t, dir, "goal-checker", "You are the @you/goal checker worker.\n")
	} else {
		support.WriteAgentConfig(t, dir, "goal-checker", `---
type: SCRIPT_WORKER
command: echo
args:
  - "goal-check-ok"
---
`)
	}
	writePackagedGoalVerificationMakefile(t, dir)
}

func writePackagedGoalCheckWorkstationReviewMode(t *testing.T, dir, reviewMode string) {
	t.Helper()

	support.WriteWorkstationConfig(t, dir, goal.PackagedCheckWorkstationName, `---
type: CLASSIFIER_WORKSTATION
env:
  `+goal.PackagedCheckReviewModeEnvVar+`: "`+reviewMode+`"
---
Review packaged goal work.
`)
}

func writePackagedGoalVerificationMakefile(t *testing.T, dir string) {
	t.Helper()

	const makefile = ".PHONY: test\n\ntest:\n\t@:\n"
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte(makefile), 0o644); err != nil {
		t.Fatalf("write packaged goal verification Makefile: %v", err)
	}
}

func packagedGoalBuiltInTopologyMockWorkersConfigForRealChecker(reviewerWorkstation, reviewerOutput string) *factoryconfig.MockWorkersConfig {
	return &factoryconfig.MockWorkersConfig{
		UnmatchedDispatchPolicy: factoryconfig.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []factoryconfig.MockWorkerConfig{
			{
				WorkerName:      "goal-planner",
				WorkstationName: goal.PackagedPlanWorkstationName,
				RunType:         factoryconfig.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-executor",
				WorkstationName: goal.PackagedExecuteWorkstationName,
				RunType:         factoryconfig.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-reviewer",
				WorkstationName: reviewerWorkstation,
				RunType:         factoryconfig.MockWorkerRunTypeScript,
				ScriptConfig: &factoryconfig.MockWorkerScriptConfig{
					Command: "/bin/echo",
					Args:    []string{reviewerOutput},
				},
			},
		},
	}
}

func packagedGoalBuiltInTopologySequencedReviewerMockWorkersConfig(t *testing.T, dir string, reviewerOutputs ...string) *factoryconfig.MockWorkersConfig {
	t.Helper()

	scriptPath := filepath.Join(dir, "goal-reviewer-sequenced.sh")
	counterPath := filepath.Join(dir, "goal-reviewer-sequenced.count")
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

	return packagedGoalBuiltInTopologyMockWorkersConfigForScript(goal.PackagedReviewWorkstationName, scriptPath)
}

func packagedGoalBuiltInTopologyMockWorkersConfigForScript(reviewerWorkstation, scriptPath string) *factoryconfig.MockWorkersConfig {
	return &factoryconfig.MockWorkersConfig{
		UnmatchedDispatchPolicy: factoryconfig.MockWorkerUnmatchedDispatchPolicyPassthrough,
		MockWorkers: []factoryconfig.MockWorkerConfig{
			{
				WorkerName:      "goal-planner",
				WorkstationName: goal.PackagedPlanWorkstationName,
				RunType:         factoryconfig.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-executor",
				WorkstationName: goal.PackagedExecuteWorkstationName,
				RunType:         factoryconfig.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-reviewer",
				WorkstationName: reviewerWorkstation,
				RunType:         factoryconfig.MockWorkerRunTypeScript,
				ScriptConfig: &factoryconfig.MockWorkerScriptConfig{
					Command: scriptPath,
				},
			},
		},
	}
}

func goalCheckerWorkerConfig(cfg *interfaces.FactoryConfig) *interfaces.WorkerConfig {
	if cfg == nil {
		return nil
	}
	for i := range cfg.Workers {
		if cfg.Workers[i].Name == "goal-checker" {
			return &cfg.Workers[i]
		}
	}
	return nil
}

func packagedGoalReviewClassifierMockWorkersConfig(label string) *factoryconfig.MockWorkersConfig {
	return &factoryconfig.MockWorkersConfig{
		MockWorkers: []factoryconfig.MockWorkerConfig{{
			WorkerName:      "goal-reviewer",
			WorkstationName: goal.PackagedReviewWorkstationName,
			RunType:         factoryconfig.MockWorkerRunTypeScript,
			ScriptConfig: &factoryconfig.MockWorkerScriptConfig{
				Command: "/bin/echo",
				Args:    []string{label},
			},
		}},
	}
}

func startPackagedGoalBuiltInTopologyInvocationServer(t *testing.T, reviewerOutput string) *functionalAPIServer {
	t.Helper()

	dir, err := factoryconfig.PersistNamedFactory(t.TempDir(), goal.PackagedFactoryName, factoryconfig.BuiltInGoalFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	writePackagedGoalBuiltInTopologyFixtureFiles(t, dir, loaded.FactoryConfig())

	return startFunctionalServerWithConfig(t, dir, true, func(cfg *service.FactoryServiceConfig) {
		cfg.RuntimeMode = interfaces.RuntimeModeService
		cfg.MockWorkersConfig = packagedGoalBuiltInTopologyMockWorkersConfigForRealChecker(
			goal.PackagedReviewWorkstationName,
			reviewerOutput,
		)
	})
}

func scaffoldPackagedGoalInvocationFactory(t *testing.T) string {
	t.Helper()

	cfg := simplePipelineConfig()
	cfg["name"] = goal.PackagedFactoryName
	cfg["invocationReturn"] = map[string]any{
		"policy":        "EXPLICIT",
		"workTypeName":  goal.PackagedInvocationReturnWorkTypeName,
		"terminalState": goal.PackagedInvocationReturnTerminalState,
	}
	workTypes := cfg["workTypes"].([]map[string]any)
	workTypes[0]["name"] = goal.PackagedInvocationReturnWorkTypeName
	workTypes[0]["handlingBehavior"] = []string{"DEFAULT"}
	workstations := cfg["workstations"].([]map[string]any)
	workstations[0]["name"] = goal.PackagedInvokeWorkstationName
	workstations[0]["worker"] = "goal-executor"
	for _, ioKey := range []string{"inputs", "outputs", "onFailure"} {
		ios := workstations[0][ioKey].([]map[string]string)
		for i := range ios {
			ios[i]["workType"] = goal.PackagedInvocationReturnWorkTypeName
		}
	}
	cfg["workers"] = []map[string]string{{"name": "goal-executor"}}

	dir := support.ScaffoldFactory(t, cfg)
	support.WriteAgentConfig(
		t,
		dir,
		"goal-executor",
		support.BuildModelWorkerConfig(interfaces.ModelProviderCodex, "gpt-5-codex"),
	)
	return dir
}

func submitGeneratedGoalWork(t *testing.T, baseURL, name, text string) factoryapi.SubmitWorkResponse {
	t.Helper()

	body, err := json.Marshal(map[string]any{
		"name":         name,
		"workTypeName": goal.PackagedGoalWorkTypeName,
		"items": []map[string]any{{
			"type": "text",
			"text": text,
		}},
	})
	if err != nil {
		t.Fatalf("marshal generated goal submit request: %v", err)
	}
	resp, err := http.Post(support.DefaultSessionWorkURL(baseURL, "/work"), "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /work goal submit: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		payload, _ := io.ReadAll(resp.Body)
		t.Fatalf("POST /work goal submit status = %d, want 201: %s", resp.StatusCode, string(payload))
	}
	var submitted factoryapi.SubmitWorkResponse
	if err := json.NewDecoder(resp.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode goal submit response: %v", err)
	}
	if strings.TrimSpace(stringPointerValue(submitted.WorkId)) == "" {
		t.Fatalf("goal submit response = %#v, want work id", submitted)
	}
	return submitted
}

func waitForGeneratedWorkIDStateName(t *testing.T, baseURL, workID, wantState string, timeout time.Duration) factoryapi.Work {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		work := requireGeneratedWorkByID(t, baseURL, workID)
		if generatedWorkStateName(work.State) == wantState {
			return work
		}
		time.Sleep(100 * time.Millisecond)
	}

	work := requireGeneratedWorkByID(t, baseURL, workID)
	t.Fatalf(
		"timed out waiting for work %q at state %q; last state=%q place=%q work=%#v",
		workID,
		wantState,
		generatedWorkStateName(work.State),
		generatedWorkPlaceID(work),
		work,
	)
	return factoryapi.Work{}
}
