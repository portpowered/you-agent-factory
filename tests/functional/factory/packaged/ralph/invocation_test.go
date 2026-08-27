package ralph

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	packagedRalphFactoryName           = "@you/ralph"
	configuredPackagedRalphFactoryName = "@test/ralph"
)

var packagedRalphPlanFile = regexp.MustCompile(`tasks/todo/([A-Za-z0-9._-]+)\.json`)

// TestPackagedRalphPlansThenIteratesToCompletionThroughNamedCLI proves the
// customer-facing named route preserves the request, materializes a durable
// plan, repeats an incomplete iteration, and returns the final iterator output
// only after the plan's story is marked complete.
func TestPackagedRalph(t *testing.T) {
	fixture := newRalphSharedFixture(t)
	t.Run("TestPackagedRalphPlansThenIteratesToCompletionThroughNamedCLI", func(t *testing.T) {
		testPackagedRalphPlansThenIteratesToCompletionThroughNamedCLI(t, fixture)
	})
	t.Run("TestPackagedRalphUsesOperatorDefaultsWhenOptionalRoleParametersAreOmitted", func(t *testing.T) {
		testPackagedRalphUsesOperatorDefaultsWhenOptionalRoleParametersAreOmitted(t, fixture)
	})
	t.Run("TestPackagedRalphUsesConfiguredAndRoleOverrideModels", func(t *testing.T) {
		testPackagedRalphUsesConfiguredAndRoleOverrideModels(t, fixture)
	})
	t.Run("TestPackagedRalphFailsOnIteratorWorkerFailure", func(t *testing.T) {
		testPackagedRalphFailsOnIteratorWorkerFailure(t, fixture)
	})
	t.Run("TestPackagedRalphFailsAfterBoundedIncompleteIterations", func(t *testing.T) {
		testPackagedRalphFailsAfterBoundedIncompleteIterations(t, fixture)
	})
}

func testPackagedRalphPlansThenIteratesToCompletionThroughNamedCLI(t *testing.T, fixture *ralphSharedFixture) {
	runner := &packagedRalphCommandRunner{workspace: t.TempDir()}
	scenario := fixture.newScenario(t, runner, packagedRalphFactoryName)
	scenario.open(t)

	response := postPackagedRalphInvocation(t, scenario, map[string]any{
		"request":         "deliver the named Ralph request",
		"plannerProvider": "CODEX", "plannerModel": "operator-default-model",
		"iteratorProvider": "CODEX", "iteratorModel": "operator-default-model",
	})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response status = %q, want COMPLETED: %#v", response.Status, response)
	}
	if got := packagedRalphPrimaryResultText(t, response); !strings.Contains(got, "all plan stories verified") {
		t.Fatalf("primary result = %q, want final iterator output", got)
	}

	requests := runner.Requests()
	if len(requests) != 3 {
		t.Fatalf("provider request count = %d, want planner plus two iterator visits", len(requests))
	}
	if got := strings.Join(runner.Roles(), ","); got != "planner,iterator,iterator" {
		t.Fatalf("worker stage order = %q, want planner,iterator,iterator", got)
	}
	plannerPrompt := packagedRalphProviderPrompt(requests[0])
	firstIteratorPrompt := packagedRalphProviderPrompt(requests[1])
	secondIteratorPrompt := packagedRalphProviderPrompt(requests[2])
	for index, prompt := range []string{plannerPrompt, firstIteratorPrompt, secondIteratorPrompt} {
		if !strings.Contains(prompt, "deliver the named Ralph request") {
			t.Fatalf("request[%d] prompt = %q, want original request preserved", index, prompt)
		}
		if !containsPackagedRalphModel(requests[index], "operator-default-model") {
			t.Fatalf("request[%d] args = %#v, want operator model", index, requests[index].Args)
		}
	}
	if !strings.Contains(firstIteratorPrompt, "durable plan prepared") {
		t.Fatalf("first iterator prompt = %q, want planner output payload", firstIteratorPrompt)
	}
	if !strings.Contains(secondIteratorPrompt, "one story remains incomplete") {
		t.Fatalf("second iterator prompt = %q, want prior incomplete iteration output", secondIteratorPrompt)
	}

	state := readPackagedRalphPlan(t, runner.PlanPath())
	if len(state.Stories) != 1 || !state.Stories[0].Passes || state.Stories[0].Notes == "" {
		t.Fatalf("durable Ralph plan = %#v, want one verified story with notes", state)
	}
}

// TestPackagedRalphUsesOperatorDefaultsWhenOptionalRoleParametersAreOmitted
// proves the named route remains invocable with only its required request and
// resolves the operator provider/model defaults for every worker role.
func testPackagedRalphUsesOperatorDefaultsWhenOptionalRoleParametersAreOmitted(t *testing.T, fixture *ralphSharedFixture) {
	runner := &packagedRalphCommandRunner{workspace: t.TempDir()}
	scenario := fixture.newScenario(t, runner, packagedRalphFactoryName)
	scenario.open(t)
	response := postPackagedRalphInvocation(t, scenario, map[string]any{
		"request": "complete Ralph with operator defaults",
	})
	if response.Status != factoryapi.InvocationTerminalStatusCompleted {
		t.Fatalf("response status = %q, want COMPLETED: %#v", response.Status, response)
	}
	for index, request := range runner.Requests() {
		if request.Command != "codex" || !containsPackagedRalphModel(request, "operator-configured-model") {
			t.Fatalf("request[%d] = command %q args %#v, want operator CODEX/operator-configured-model", index, request.Command, request.Args)
		}
	}
}

// TestPackagedRalphUsesConfiguredAndRoleOverrideModels proves authored worker
// configuration is honored when role flags are omitted and that explicit
// planner/iterator flags take precedence over those configured values.
func testPackagedRalphUsesConfiguredAndRoleOverrideModels(t *testing.T, fixture *ralphSharedFixture) {
	tests := []struct {
		name             string
		configure        func(*testing.T, string)
		invocationArgs   map[string]any
		plannerModel     string
		iteratorModel    string
		plannerProvider  string
		iteratorProvider string
	}{
		{
			name: "installed worker configuration",
			configure: func(t *testing.T, factoryDir string) {
				configurePackagedRalphWorkerModels(t, factoryDir, map[string]string{
					"ralph-planner":  "configured-planner-model",
					"ralph-iterator": "configured-iterator-model",
				})
			},
			plannerModel:     "configured-planner-model",
			iteratorModel:    "configured-iterator-model",
			plannerProvider:  "codex",
			iteratorProvider: "codex",
		},
		{
			name: "explicit role flags",
			invocationArgs: map[string]any{
				"plannerProvider":  "CODEX",
				"plannerModel":     "flag-planner-model",
				"iteratorProvider": "CODEX",
				"iteratorModel":    "flag-iterator-model",
			},
			plannerModel:     "flag-planner-model",
			iteratorModel:    "flag-iterator-model",
			plannerProvider:  "codex",
			iteratorProvider: "codex",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			factoryName := packagedRalphFactoryName
			if test.configure != nil {
				factoryName = configuredPackagedRalphFactoryName
			}
			runner := &packagedRalphCommandRunner{workspace: t.TempDir()}
			scenario := fixture.newScenario(t, runner, factoryName)
			if test.configure != nil {
				test.configure(t, scenario.factoryDir)
			}
			scenario.open(t)
			args := map[string]any{"request": "complete a configured Ralph request"}
			for key, value := range test.invocationArgs {
				args[key] = value
			}
			response := postPackagedRalphInvocation(t, scenario, args)
			if response.Status != factoryapi.InvocationTerminalStatusCompleted {
				t.Fatalf("response status = %q, want COMPLETED: %#v", response.Status, response)
			}
			requests := runner.Requests()
			if len(requests) != 3 {
				t.Fatalf("provider request count = %d, want three stages", len(requests))
			}
			for index, request := range requests {
				wantModel := test.plannerModel
				wantProvider := test.plannerProvider
				if index > 0 {
					wantModel = test.iteratorModel
					wantProvider = test.iteratorProvider
				}
				if request.Command != wantProvider || !containsPackagedRalphModel(request, wantModel) {
					t.Fatalf("request[%d] = command %q args %#v, want %s/%s", index, request.Command, request.Args, wantProvider, wantModel)
				}
			}
		})
	}
}

// TestPackagedRalphFailsOnIteratorWorkerFailure proves provider failure is a
// failed public invocation and never a successful completion with partial
// iterator output.
func testPackagedRalphFailsOnIteratorWorkerFailure(t *testing.T, fixture *ralphSharedFixture) {
	runner := &packagedRalphCommandRunner{workspace: t.TempDir(), failIterator: true}
	scenario := fixture.newScenario(t, runner, packagedRalphFactoryName)
	scenario.open(t)
	response := postPackagedRalphInvocation(t, scenario, map[string]any{
		"request":         "fail the Ralph iterator",
		"plannerProvider": "CODEX", "plannerModel": "failure-model",
		"iteratorProvider": "CODEX", "iteratorModel": "failure-model",
	})
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("response status = %q, want FAILED: %#v", response.Status, response)
	}
	if response.WorkState == nil || *response.WorkState != "ralph:failed" {
		t.Fatalf("response workState = %#v, want ralph:failed", response.WorkState)
	}
	if response.PrimaryResult != nil {
		t.Fatalf("response primaryResult = %#v, want nil after failure", response.PrimaryResult)
	}
	if got := len(runner.Requests()); got != 2 {
		t.Fatalf("provider request count = %d, want planner plus failed iterator", got)
	}
}

// TestPackagedRalphFailsAfterBoundedIncompleteIterations proves explicit
// continuation cannot run forever and the logical breaker returns ralph:failed
// without launching an extra iterator visit.
func testPackagedRalphFailsAfterBoundedIncompleteIterations(t *testing.T, fixture *ralphSharedFixture) {
	runner := &packagedRalphCommandRunner{workspace: t.TempDir(), alwaysContinue: true}
	scenario := fixture.newScenario(t, runner, packagedRalphFactoryName)
	scenario.open(t)
	response := postPackagedRalphInvocation(t, scenario, map[string]any{
		"request":         "keep iterating this Ralph request",
		"plannerProvider": "CODEX", "plannerModel": "bounded-model",
		"iteratorProvider": "CODEX", "iteratorModel": "bounded-model",
	})
	if response.Status != factoryapi.InvocationTerminalStatusFailed {
		t.Fatalf("response status = %q, want FAILED: %#v", response.Status, response)
	}
	if response.WorkState == nil || *response.WorkState != "ralph:failed" {
		t.Fatalf("response workState = %#v, want ralph:failed", response.WorkState)
	}
	if got := len(runner.Requests()); got != 13 {
		t.Fatalf("provider request count = %d, want planner plus twelve bounded iterator visits", got)
	}
}

type packagedRalphCommandRunner struct {
	mu             sync.Mutex
	workspace      string
	requests       []platformprocess.CommandRequest
	roles          []string
	planPath       string
	iteratorCalls  int
	failIterator   bool
	alwaysContinue bool
}

func (runner *packagedRalphCommandRunner) Run(
	_ context.Context,
	request platformprocess.CommandRequest,
) (platformprocess.CommandResult, error) {
	prompt := packagedRalphProviderPrompt(request)
	runner.mu.Lock()
	runner.requests = append(runner.requests, clonePackagedRalphCommandRequest(request))
	runner.mu.Unlock()

	switch {
	case strings.Contains(prompt, "planning stage of @you/ralph"):
		workName := packagedRalphPlanWorkName(prompt)
		if workName == "" {
			return platformprocess.CommandResult{}, errors.New("planner prompt omitted durable plan path")
		}
		planPath := filepath.Join(request.WorkDir, "tasks", "todo", workName+".json")
		if err := os.MkdirAll(filepath.Dir(planPath), 0o700); err != nil {
			return platformprocess.CommandResult{}, err
		}
		plan := packagedRalphPlan{
			Project: "ralph-functional",
			Stories: []packagedRalphStory{{
				ID:                 "RALPH-001",
				Description:        "complete the requested work",
				AcceptanceCriteria: []string{"the request is verified"},
				Tests:              []string{"the named CLI test"},
				Passes:             false,
				Notes:              "",
			}},
		}
		payload, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return platformprocess.CommandResult{}, err
		}
		if err := os.WriteFile(planPath, payload, 0o600); err != nil {
			return platformprocess.CommandResult{}, err
		}
		if err := os.WriteFile(filepath.Join(filepath.Dir(planPath), workName+".md"), []byte("# Ralph plan\n"), 0o600); err != nil {
			return platformprocess.CommandResult{}, err
		}
		runner.mu.Lock()
		runner.planPath = planPath
		runner.roles = append(runner.roles, "planner")
		runner.mu.Unlock()
		return packagedRalphProviderResult("durable plan prepared\n<COMPLETE>"), nil
	case strings.Contains(prompt, "iterative execution stage of @you/ralph"):
		runner.mu.Lock()
		runner.iteratorCalls++
		call := runner.iteratorCalls
		planPath := runner.planPath
		runner.roles = append(runner.roles, "iterator")
		fail := runner.failIterator
		alwaysContinue := runner.alwaysContinue
		runner.mu.Unlock()
		if fail {
			return platformprocess.CommandResult{}, errors.New("mock Ralph iterator failure")
		}
		if planPath == "" {
			return platformprocess.CommandResult{}, errors.New("iterator started before planner created durable plan")
		}
		plan := readPackagedRalphPlanFromPath(planPath)
		if alwaysContinue || call == 1 {
			return packagedRalphProviderResult("one story remains incomplete\n<CONTINUE>"), nil
		}
		plan.Stories[0].Passes = true
		plan.Stories[0].Notes = "verified by the named CLI functional test"
		payload, err := json.MarshalIndent(plan, "", "  ")
		if err != nil {
			return platformprocess.CommandResult{}, err
		}
		if err := os.WriteFile(planPath, payload, 0o600); err != nil {
			return platformprocess.CommandResult{}, err
		}
		return packagedRalphProviderResult("all plan stories verified\n<COMPLETE>"), nil
	default:
		return platformprocess.CommandResult{}, fmt.Errorf("unexpected Ralph prompt: %s", prompt)
	}
}

func postPackagedRalphInvocation(
	t *testing.T,
	scenario *ralphScenario,
	args map[string]any,
) factoryapi.InvocationResponse {
	t.Helper()
	requestID := fmt.Sprintf("packaged-ralph-%d", time.Now().UnixNano())
	payload, err := json.Marshal(factoryapi.InvocationRequest{
		RequestId: &requestID,
		Args:      &args,
	})
	if err != nil {
		t.Fatalf("marshal Ralph invocation: %v", err)
	}
	endpoint := strings.TrimSuffix(scenario.fixture.baseURL, "/") +
		"/factory-sessions/" + url.PathEscape(scenario.sessionID) + "/invocations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST Ralph invocation: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST Ralph invocation status = %d, want 200: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var decoded factoryapi.InvocationResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode Ralph invocation: %v", err)
	}
	return decoded
}

func configurePackagedRalphWorkerModels(t *testing.T, factoryDir string, models map[string]string) {
	t.Helper()
	path := filepath.Join(factoryDir, "factory.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read materialized Ralph factory: %v", err)
	}
	var factory map[string]any
	if err := json.Unmarshal(payload, &factory); err != nil {
		t.Fatalf("decode materialized Ralph factory: %v", err)
	}
	workers, ok := factory["workers"].([]any)
	if !ok {
		t.Fatal("materialized Ralph workers are not an array")
	}
	for _, raw := range workers {
		worker := raw.(map[string]any)
		model, ok := models[worker["name"].(string)]
		if !ok {
			continue
		}
		worker["modelProvider"] = "CODEX"
		worker["model"] = model
	}
	updated, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("encode materialized Ralph factory: %v", err)
	}
	if err := os.WriteFile(path, updated, 0o644); err != nil {
		t.Fatalf("write materialized Ralph factory: %v", err)
	}
}

type packagedRalphPlan struct {
	Project string               `json:"project"`
	Stories []packagedRalphStory `json:"stories"`
}

type packagedRalphStory struct {
	ID                 string   `json:"id"`
	Description        string   `json:"description"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
	Tests              []string `json:"tests"`
	Passes             bool     `json:"passes"`
	Notes              string   `json:"notes"`
}

func readPackagedRalphPlan(t *testing.T, path string) packagedRalphPlan {
	t.Helper()
	return readPackagedRalphPlanFromPath(path)
}

func readPackagedRalphPlanFromPath(path string) packagedRalphPlan {
	payload, err := os.ReadFile(path)
	if err != nil {
		panic(fmt.Sprintf("read Ralph plan: %v", err))
	}
	var plan packagedRalphPlan
	if err := json.Unmarshal(payload, &plan); err != nil {
		panic(fmt.Sprintf("decode Ralph plan: %v", err))
	}
	return plan
}

func packagedRalphPlanWorkName(prompt string) string {
	matches := packagedRalphPlanFile.FindStringSubmatch(prompt)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func packagedRalphProviderPrompt(request platformprocess.CommandRequest) string {
	if len(request.Stdin) > 0 {
		return string(request.Stdin)
	}
	return strings.Join(request.Args, " ")
}

func packagedRalphProviderResult(output string) platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: support.CodexSuccessStdout(output)}
}

func containsPackagedRalphModel(request platformprocess.CommandRequest, model string) bool {
	for index := 0; index+1 < len(request.Args); index++ {
		if request.Args[index] == "--model" && request.Args[index+1] == model {
			return true
		}
	}
	return false
}

func packagedRalphPrimaryResultText(t *testing.T, response factoryapi.InvocationResponse) string {
	t.Helper()
	if response.PrimaryResult == nil || len(*response.PrimaryResult) != 1 {
		t.Fatalf("primaryResult = %#v, want one text part", response.PrimaryResult)
	}
	part, err := (*response.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primaryResult[0] as text part: %v", err)
	}
	return part.Text
}

func (runner *packagedRalphCommandRunner) Requests() []platformprocess.CommandRequest {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	requests := make([]platformprocess.CommandRequest, len(runner.requests))
	for index, request := range runner.requests {
		requests[index] = clonePackagedRalphCommandRequest(request)
	}
	return requests
}

func (runner *packagedRalphCommandRunner) Roles() []string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return append([]string(nil), runner.roles...)
}

func (runner *packagedRalphCommandRunner) PlanPath() string {
	runner.mu.Lock()
	defer runner.mu.Unlock()
	return runner.planPath
}

func clonePackagedRalphCommandRequest(request platformprocess.CommandRequest) platformprocess.CommandRequest {
	request.Args = append([]string(nil), request.Args...)
	request.Stdin = append([]byte(nil), request.Stdin...)
	request.Env = append([]string(nil), request.Env...)
	return request
}
