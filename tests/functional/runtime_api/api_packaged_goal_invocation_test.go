package runtime_api

import (
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/packagedfactories/goal"
	"github.com/portpowered/infinite-you/pkg/service"
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
