package goal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	"github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	packagedGoalFactoryName                 = "@you/goal"
	packagedGoalPlanWorkstationName         = "plan-goal"
	packagedGoalExecuteWorkstationName        = "execute-goal"
	packagedGoalCheckWorkstationName          = "check-goal"
	packagedGoalReviewWorkstationName         = "review-goal"
	packagedGoalMockWorkerAcceptedSummary     = "mock worker accepted"
	packagedGoalRejectThenCompleteSummary     = "finished after rejection"
)

func scaffoldPackagedGoalBuiltInFactory(t *testing.T) string {
	t.Helper()
	dir := support.InstallPackagedFactory(t, t.TempDir(), packagedGoalFactoryName)
	if _, err := support.LoadedFactory(t, dir); err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}
	return dir
}

func writePackagedGoalBuiltinMockWorkersConfig(t *testing.T) string {
	t.Helper()

	cfg := workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      "goal-planner",
				WorkstationName: packagedGoalPlanWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-executor",
				WorkstationName: packagedGoalExecuteWorkstationName,
				RunType:         workers.MockWorkerRunTypeAccept,
			},
			{
				WorkerName:      "goal-checker",
				WorkstationName: packagedGoalCheckWorkstationName,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: "/bin/echo",
					Args:    []string{"plain"},
				},
			},
			{
				WorkerName:      "goal-reviewer",
				WorkstationName: packagedGoalReviewWorkstationName,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: "/bin/echo",
					Args:    []string{"accepted"},
				},
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal packaged goal mock-workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers-packaged-goal.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write packaged goal mock-workers config: %v", err)
	}
	return path
}

func writePackagedGoalRejectThenAcceptMockWorkersConfig(t *testing.T) (string, string) {
	t.Helper()
	return writePackagedGoalSequencedExecutorMockWorkersConfig(
		t,
		"goal is not complete yet",
		"__COMPLETE_WITH_STOP_TOKEN__",
	)
}

func writePackagedGoalSequencedExecutorMockWorkersConfig(t *testing.T, executorOutputs ...string) (string, string) {
	t.Helper()

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "goal-executor-sequenced.sh")
	counterPath := filepath.Join(scriptDir, "goal-executor-sequenced.count")
	lines := []string{
		"#!/bin/sh",
		"count=0",
		"if [ -f \"" + counterPath + "\" ]; then",
		"  count=$(cat \"" + counterPath + "\")",
		"fi",
		"case \"$count\" in",
	}
	for idx, output := range executorOutputs {
		switch output {
		case "__COMPLETE_WITH_STOP_TOKEN__":
			lines = append(lines, "  "+strconv.Itoa(idx)+") printf '%s\\n%s' '"+packagedGoalRejectThenCompleteSummary+"' '<COMPLETE>' ;;")
		default:
			lines = append(lines, "  "+strconv.Itoa(idx)+") printf '%s' '"+output+"' ;;")
		}
	}
	fallback := packagedGoalMockWorkerAcceptedSummary
	if len(executorOutputs) > 0 {
		last := executorOutputs[len(executorOutputs)-1]
		if last == "__COMPLETE_WITH_STOP_TOKEN__" {
			fallback = packagedGoalRejectThenCompleteSummary + "\n<COMPLETE>"
		} else {
			fallback = last
		}
	}
	lines = append(lines,
		"  *) printf '%s' '"+fallback+"' ;;",
		"esac",
		"printf '%s' $((count + 1)) > \""+counterPath+"\"",
	)
	if err := os.WriteFile(scriptPath, []byte(strings.Join(lines, "\n")+"\n"), 0o755); err != nil {
		t.Fatalf("write sequenced goal executor script: %v", err)
	}

	cfg := workers.MockWorkersConfig{
		MockWorkers: []workers.MockWorkerConfig{
			{
				WorkerName:      "goal-executor",
				WorkstationName: packagedGoalExecuteWorkstationName,
				RunType:         workers.MockWorkerRunTypeScript,
				ScriptConfig: &workers.MockWorkerScriptConfig{
					Command: scriptPath,
				},
			},
		},
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal packaged goal reject-then-accept mock-workers config: %v", err)
	}
	path := filepath.Join(t.TempDir(), "mock-workers-packaged-goal-reject-then-accept.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write packaged goal reject-then-accept mock-workers config: %v", err)
	}
	return path, counterPath
}

func readPackagedGoalExecutorInvocationCount(t *testing.T, counterPath string) int {
	t.Helper()

	payload, err := os.ReadFile(counterPath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatalf("read executor invocation counter: %v", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(payload)))
	if err != nil {
		t.Fatalf("parse executor invocation counter %q: %v", string(payload), err)
	}
	return count
}

func startPackagedGoalInvocationServer(t *testing.T, factoryDir, mockWorkersPath string) *support.FunctionalAPIServer {
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

func postPackagedGoalInvocation(
	t *testing.T,
	factoryDir string,
	mockWorkersPath string,
	goalText string,
) factoryapi.InvocationResponse {
	t.Helper()

	_, response := invokePackagedGoal(t, factoryDir, mockWorkersPath, goalText)
	return response
}

func invokePackagedGoal(
	t *testing.T,
	factoryDir string,
	mockWorkersPath string,
	goalText string,
) (*support.FunctionalAPIServer, factoryapi.InvocationResponse) {
	t.Helper()

	body := textInvocationRequestBody(goalText)
	server := startPackagedGoalInvocationServer(t, factoryDir, mockWorkersPath)
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
	return server, decoded
}

func textInvocationRequestBody(goalText string) []byte {
	body, err := json.Marshal(factoryapi.InvocationRequest{
		SourceKind: invocationTextSourceKindPtr(),
		Content:    invocationTextContentPtr(goalText),
	})
	if err != nil {
		panic(fmt.Sprintf("marshal invocation request: %v", err))
	}
	return body
}

func invocationTextSourceKindPtr() *factoryapi.InvocationInputSourceKind {
	sourceKind := factoryapi.InvocationInputSourceKindText
	return &sourceKind
}

func invocationTextContentPtr(goalText string) *factoryapi.WorkContent {
	var part factoryapi.WorkContentPart
	if err := part.FromWorkTextContentPart(factoryapi.WorkTextContentPart{
		Type: factoryapi.WorkContentPartTypeText,
		Text: goalText,
	}); err != nil {
		panic(fmt.Sprintf("build invocation text content: %v", err))
	}
	content := factoryapi.WorkContent{part}
	return &content
}

func assertPackagedGoalCompletedWithSummary(
	t *testing.T,
	response factoryapi.InvocationResponse,
	wantSummary string,
) {
	t.Helper()

	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("invocation status = %q, want COMPLETED; response = %#v", response.Status, response)
	}
	if got := primaryResultText(t, response); got != wantSummary {
		t.Fatalf("primaryResult text = %q, want %q", got, wantSummary)
	}
}

func primaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()

	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("invocation primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}
