package planexecute

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

var planExecuteWorkName = regexp.MustCompile(`tasks/todo/([^` + "`" + `]+)\.md`)

// TestPackagedPlanExecute groups the package's plan-and-execute behavior under
// one reusable root-built process.
func TestPackagedPlanExecute(t *testing.T) {
	fixture := newPlanExecuteSharedFixture(t)
	t.Run("TestPackagedPlanExecutePlansThenExecutesWithOperatorDefaults", func(t *testing.T) {
		testPackagedPlanExecutePlansThenExecutesWithOperatorDefaults(t, fixture)
	})
}

// testPackagedPlanExecutePlansThenExecutesWithOperatorDefaults proves the
// customer invocation path dispatches exactly the two documented stages and
// uses the run-level default provider/model when no role-specific overrides
// are passed.
func testPackagedPlanExecutePlansThenExecutesWithOperatorDefaults(
	t *testing.T,
	fixture *planExecuteSharedFixture,
) {
	runner := &planExecuteRunner{workspace: t.TempDir()}
	scenario := fixture.newScenario(t, runner)
	scenario.open(t)
	assertPlanExecutePromptContracts(t)

	requestID := fmt.Sprintf("plan-execute-%d", time.Now().UnixNano())
	args := map[string]any{"request": "Deliver the packaged two-stage flow"}
	response := postPlanExecuteInvocation(t, scenario, requestID, args)
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response = %#v", response)
	}
	if got := strings.Join(runner.Calls(), ","); got != "planner,executor" {
		t.Fatalf("role calls = %q, want planner,executor", got)
	}
	for index, request := range runner.Requests() {
		if request.Command != "codex" || !containsPlanExecuteArgPair(request.Args, "--model", "operator-default-model") {
			t.Fatalf("request[%d] provider selection = command %q args %#v", index, request.Command, request.Args)
		}
	}
	if content, err := os.ReadFile(filepath.Join(runner.workspace, "implemented.txt")); err != nil || string(content) != "implemented from prd\n" {
		t.Fatalf("implemented artifact = %q, error = %v", content, err)
	}
	assertPlanExecutePRDPassed(t, runner.PRDPath())
}

func postPlanExecuteInvocation(
	t *testing.T,
	scenario *planExecuteScenario,
	requestID string,
	args map[string]any,
) factoryapi.InvocationResponse {
	t.Helper()
	payload, err := json.Marshal(factoryapi.InvocationRequest{
		RequestId: &requestID,
		Args:      &args,
	})
	if err != nil {
		t.Fatalf("marshal plan-execute invocation: %v", err)
	}
	endpoint := strings.TrimSuffix(scenario.fixture.baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(scenario.sessionID) + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST plan-execute invocation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST plan-execute invocation status = %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode plan-execute invocation: %v", err)
	}
	return decoded
}

type planExecuteRunner struct {
	mu        sync.Mutex
	workspace string
	calls     []string
	requests  []platformprocess.CommandRequest
	prdPath   string
}

func (runner *planExecuteRunner) Run(_ context.Context, request platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	prompt := planExecuteCommandPrompt(request)
	runner.mu.Lock()
	runner.requests = append(runner.requests, request)
	runner.mu.Unlock()
	switch {
	case strings.Contains(prompt, "planning stage of a two-stage"):
		matches := planExecuteWorkName.FindStringSubmatch(prompt)
		if len(matches) != 2 {
			return platformprocess.CommandResult{}, fmt.Errorf("planner prompt omitted concrete Work name: %s", prompt)
		}
		runner.record("planner")
		directory := filepath.Join(runner.workspace, "tasks", "todo")
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return platformprocess.CommandResult{}, err
		}
		runner.prdPath = filepath.Join(directory, matches[1]+".json")
		prd := `{"project":"delivery","description":"deliver","context":"fixture","acceptanceCriteria":["implemented"],"stories":[{"id":"US-001","priority":1,"description":"implement","acceptanceCriteria":["file exists"],"tests":["read file"],"passes":false,"notes":""}]}`
		if err := os.WriteFile(runner.prdPath, []byte(prd), 0o600); err != nil {
			return platformprocess.CommandResult{}, err
		}
		if err := os.WriteFile(filepath.Join(directory, matches[1]+".md"), []byte("# Delivery PRD\n"), 0o600); err != nil {
			return platformprocess.CommandResult{}, err
		}
		return planExecuteResult("<COMPLETE>"), nil
	case strings.Contains(prompt, "execution stage of a two-stage"):
		runner.record("executor")
		if _, err := os.Stat(runner.prdPath); err != nil {
			return platformprocess.CommandResult{}, fmt.Errorf("executor could not read planner PRD: %w", err)
		}
		if err := os.WriteFile(filepath.Join(runner.workspace, "implemented.txt"), []byte("implemented from prd\n"), 0o600); err != nil {
			return platformprocess.CommandResult{}, err
		}
		payload, err := os.ReadFile(runner.prdPath)
		if err != nil {
			return platformprocess.CommandResult{}, err
		}
		var prd map[string]any
		if err := json.Unmarshal(payload, &prd); err != nil {
			return platformprocess.CommandResult{}, err
		}
		story := prd["stories"].([]any)[0].(map[string]any)
		story["passes"] = true
		story["notes"] = "verified implemented.txt"
		updated, _ := json.Marshal(prd)
		if err := os.WriteFile(runner.prdPath, updated, 0o600); err != nil {
			return platformprocess.CommandResult{}, err
		}
		return planExecuteResult("implemented from PRD\n<COMPLETE>"), nil
	default:
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected plan-execute prompt: %s", prompt)
	}
}

func assertPlanExecutePromptContracts(t *testing.T) {
	t.Helper()
	promptsDir := filepath.Join(
		testutil.MustRepoRoot(t),
		"packages", "packaged-factories", "factories", "plan-execute", "prompts",
	)
	for _, fixture := range []struct {
		name     string
		required []string
	}{
		{
			name: "planner.md",
			required: []string{
				"ends after verified implementation in the current",
				"do not make them a story acceptance criterion",
				"end the response with the exact raw token",
				"below as its final non-empty line",
				"Do not wrap that token in backticks",
				"successful planner completion",
			},
		},
		{
			name: "executor.md",
			required: []string{
				"end the response with the exact raw",
				"token below as its final non-empty line",
				"Do not wrap that token in backticks",
				"successful executor completion",
			},
		},
	} {
		payload, err := os.ReadFile(filepath.Join(promptsDir, fixture.name))
		if err != nil {
			t.Fatalf("read authored %s prompt: %v", fixture.name, err)
		}
		prompt := string(payload)
		for _, required := range fixture.required {
			if !strings.Contains(prompt, required) {
				t.Fatalf("%s prompt missing completion contract %q", fixture.name, required)
			}
		}
	}
}

func planExecuteCommandPrompt(request platformprocess.CommandRequest) string {
	if len(request.Stdin) > 0 {
		return string(request.Stdin)
	}
	if len(request.Args) > 0 {
		return request.Args[len(request.Args)-1]
	}
	return ""
}

func (runner *planExecuteRunner) record(role string) {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	runner.calls = append(runner.calls, role)
}

func (runner *planExecuteRunner) Calls() []string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]string(nil), runner.calls...)
}

func (runner *planExecuteRunner) Requests() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]platformprocess.CommandRequest(nil), runner.requests...)
}

func (runner *planExecuteRunner) PRDPath() string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.prdPath
}

func planExecuteResult(value string) platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(value)}
}

func containsPlanExecuteArgPair(args []string, name, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name && args[index+1] == value {
			return true
		}
	}
	return false
}

func assertPlanExecutePRDPassed(t *testing.T, path string) {
	t.Helper()
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read PRD: %v", err)
	}
	var prd struct {
		Stories []struct {
			Passes bool   `json:"passes"`
			Notes  string `json:"notes"`
		} `json:"stories"`
	}
	if err := json.Unmarshal(payload, &prd); err != nil {
		t.Fatalf("decode PRD: %v", err)
	}
	if len(prd.Stories) != 1 || !prd.Stories[0].Passes || prd.Stories[0].Notes == "" {
		t.Fatalf("PRD stories = %#v", prd.Stories)
	}
}
