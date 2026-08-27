package javascript_families

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

func TestPackagedJavaScriptFamilies(t *testing.T) {
	fixture := newJavascriptFamiliesSharedFixture(t)
	t.Run("TestPackagedTournamentRunsOneOnOneBracketThroughCodexCommandRunner", func(t *testing.T) {
		testPackagedTournamentRunsOneOnOneBracketThroughCodexCommandRunner(t, fixture)
	})
	t.Run("TestPackagedSpawnPlansExactCountRunsChildrenAndMergesThroughCodexCommandRunner", func(t *testing.T) {
		testPackagedSpawnPlansExactCountRunsChildrenAndMergesThroughCodexCommandRunner(t, fixture)
	})
	t.Run("TestPackagedSpawnRunsAntigravityChildrenWithExactModel", func(t *testing.T) {
		testPackagedSpawnRunsAntigravityChildrenWithExactModel(t, fixture)
	})
}

func testPackagedTournamentRunsOneOnOneBracketThroughCodexCommandRunner(
	t *testing.T,
	fixture *javascriptFamiliesSharedFixture,
) {
	runner := testutil.NewProviderCommandRunner(
		providerResult(support.CodexSuccessStdout("candidate A")),
		providerResult(support.CodexSuccessStdout("candidate B")),
		providerResult(support.CodexSuccessStdout(`{"winner":"B","rationale":"more complete"}`)),
	)
	response := invokeJavaScriptFactory(t, fixture, javascriptInvocation{
		factoryName: factorydefinitions.PackagedTournamentFactoryName,
		requestID:   "packaged-tournament-codex-1v1",
		args: map[string]any{
			"request": "Propose a launch strategy",
			"rounds":  1,
		},
		runner: runner,
	})
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		t.Logf("provider command calls before tournament failure: %d", runner.CallCount())
	}

	assertSucceededPrimaryContains(t, response, "Tournament decision trail", "more complete")
	part, err := (*response.Result.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primary result is not CLI-renderable text: %v", err)
	}
	if !strings.HasPrefix(part.Text, "candidate A\n\nTournament decision trail:") &&
		!strings.HasPrefix(part.Text, "candidate B\n\nTournament decision trail:") {
		t.Fatalf("primary result text = %q, want a candidate followed by decision trail", part.Text)
	}
	requests := runner.Requests()
	if len(requests) != 3 {
		t.Fatalf("provider calls = %d, want two competitors plus one judge", len(requests))
	}
	for index, request := range requests {
		if request.Command != "codex" || !containsArgPair(request.Args, "--model", "operator-js-model") {
			t.Fatalf("provider request[%d] = %#v, want inherited codex/operator-js-model defaults", index, request)
		}
	}
	if !strings.Contains(string(requests[2].Stdin), "Candidate A") &&
		!strings.Contains(strings.Join(requests[2].Args, " "), "Candidate A") {
		t.Fatalf("judge request does not contain the 1v1 candidates: %#v", requests[2])
	}
}

func testPackagedSpawnPlansExactCountRunsChildrenAndMergesThroughCodexCommandRunner(
	t *testing.T,
	fixture *javascriptFamiliesSharedFixture,
) {
	runner := testutil.NewProviderCommandRunner(
		providerResult(support.CodexSuccessStdout(`{"tasks":["research climate","research cost"]}`)),
		providerResult(support.CodexSuccessStdout(`{"result":"climate findings"}`)),
		providerResult(support.CodexSuccessStdout(`{"result":"cost findings"}`)),
		providerResult(support.CodexSuccessStdout(`{"answer":"merged travel answer"}`)),
	)
	response := invokeJavaScriptFactory(t, fixture, javascriptInvocation{
		factoryName: factorydefinitions.PackagedSpawnFactoryName,
		requestID:   "packaged-spawn-codex-exact-count",
		args: map[string]any{
			"request": "research the best places to travel", "count": 2,
		},
		runner: runner,
	})
	assertSucceededPrimaryContains(t, response, "merged travel answer")
	part, err := (*response.Result.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("primary result is not CLI-renderable text: %v", err)
	}
	if part.Text != "merged travel answer" {
		t.Fatalf("primary result text = %q, want only the merged answer", part.Text)
	}
	requests := runner.Requests()
	if len(requests) != 4 {
		t.Fatalf("provider calls = %d, want planner, exactly two tasks, and merger", len(requests))
	}
	for index, request := range requests {
		if request.Command != "codex" || !containsArgPair(request.Args, "--model", "operator-js-model") {
			t.Fatalf("provider request[%d] = %#v, want inherited codex/operator-js-model defaults", index, request)
		}
	}
	mergeInput := string(requests[3].Stdin) + " " + strings.Join(requests[3].Args, " ")
	firstIndex, secondIndex := strings.Index(mergeInput, `"index":1`), strings.Index(mergeInput, `"index":2`)
	if firstIndex < 0 || secondIndex <= firstIndex || !strings.Contains(mergeInput, "climate findings") || !strings.Contains(mergeInput, "cost findings") {
		t.Fatalf("merger input does not preserve planned result order: %s", mergeInput)
	}
}

// TestPackagedSpawnRunsAntigravityChildrenWithExactModel proves packaged spawning preserves the exact Antigravity child model.
func testPackagedSpawnRunsAntigravityChildrenWithExactModel(
	t *testing.T,
	fixture *javascriptFamiliesSharedFixture,
) {
	for _, executorProvider := range []string{"", "SCRIPT_WRAP"} {
		t.Run("executorProvider="+executorProvider, func(t *testing.T) {
			const model = "gemini-3.6-flash-medium"
			runner := testutil.NewProviderCommandRunner(
				providerResult(antigravityStructuredResult("agy-spawn-plan", map[string]any{"tasks": []string{"one task"}}, antigravityTaskPlanSchema)),
				providerResult(antigravityStructuredResult("agy-spawn-task", map[string]any{"result": "one task result"}, antigravityTaskResultSchema)),
				providerResult(antigravityStructuredResult("agy-spawn-merge", map[string]any{"answer": "merged Antigravity spawn answer"}, antigravityMergerResultSchema)),
			)
			response := invokeJavaScriptFactory(t, fixture, javascriptInvocation{
				factoryName: factorydefinitions.PackagedSpawnFactoryName,
				requestID:   "packaged-spawn-antigravity-" + strings.ToLower(strings.ReplaceAll(executorProvider, "_", "-")),
				args: map[string]any{
					"request":          "research the best places to travel",
					"count":            1,
					"executorProvider": executorProvider,
					"modelProvider":    "ANTIGRAVITY",
					"model":            model,
				},
				runner: runner,
			})
			assertSucceededPrimaryContains(t, response, "merged Antigravity spawn answer")
			requests := runner.Requests()
			if len(requests) != 3 {
				t.Fatalf("provider calls = %d, want planner, task, and merger", len(requests))
			}
			for index, request := range requests {
				if request.Command != "agy" || !containsArgPair(request.Args, "--model", model) || containsArg(request.Args, "--effort") {
					t.Fatalf("provider request[%d] = %#v, want agy with exact model and no separate effort", index, request)
				}
			}
		})
	}
}

type javascriptInvocation struct {
	factoryName string
	requestID   string
	args        map[string]any
	runner      *testutil.ProviderCommandRunner
}

func invokeJavaScriptFactory(
	t *testing.T,
	fixture *javascriptFamiliesSharedFixture,
	invocation javascriptInvocation,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()
	scenario := fixture.newScenario(t, invocation.factoryName, invocation.runner)
	scenario.open(t)
	factory := support.GetJSON[factoryapi.Factory](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+
			"/factory-sessions/"+url.PathEscape(scenario.sessionID)+"/factory",
	)
	if factory.Orchestrator == nil || factory.Orchestrator.Javascript == nil || factory.Orchestrator.Javascript.InlineSource == nil {
		t.Fatalf("opened JavaScript factory %q does not carry inline source", invocation.factoryName)
	}
	javascript := factory.Orchestrator.Javascript
	response := postJSON[factoryapi.FactorySessionSyncExecutionResponse](
		t,
		strings.TrimSuffix(fixture.baseURL, "/")+"/factory-sessions/sync",
		factoryapi.FactorySessionExecutionRequest{
			RequestId: invocation.requestID,
			Source: factoryapi.FactorySessionExecutionSource{
				Kind: factoryapi.FactorySessionExecutionSourceKindInlineWorkflow,
				InlineWorkflow: &factoryapi.FactorySessionExecutionInlineWorkflow{
					Dialect:      javascript.Dialect,
					Entrypoint:   javascript.Entrypoint,
					InlineSource: *javascript.InlineSource,
					Metadata:     javascript.Metadata,
				},
			},
			Args:         &invocation.args,
			Orchestrator: factory.Orchestrator,
		},
	)
	return response
}

func containsArgPair(args []string, name, value string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == name && args[index+1] == value {
			return true
		}
	}
	return false
}

func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func assertSucceededPrimaryContains(t *testing.T, response factoryapi.FactorySessionSyncExecutionResponse, fragments ...string) {
	t.Helper()
	if response.Status != factoryapi.FactorySessionDurableLifecycleStatusSucceeded {
		payload, _ := json.Marshal(response)
		t.Fatalf("session status = %q, want SUCCEEDED; response = %s", response.Status, payload)
	}
	if response.Result == nil || response.Result.PrimaryResult == nil || len(*response.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one JavaScript result", response.Result)
	}
	payload, err := json.Marshal((*response.Result.PrimaryResult)[0])
	if err != nil {
		t.Fatalf("marshal primary result: %v", err)
	}
	for _, fragment := range fragments {
		if !strings.Contains(string(payload), fragment) {
			t.Fatalf("primary result = %s, want %q", payload, fragment)
		}
	}
}

func postJSON[T any](t *testing.T, endpoint string, request any) T {
	t.Helper()
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		var body bytes.Buffer
		_, _ = body.ReadFrom(response.Body)
		t.Fatalf("POST %s status = %d: %s", endpoint, response.StatusCode, body.String())
	}
	var decoded T
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return decoded
}

func providerResult(stdout []byte) platformprocess.CommandResult {
	return platformprocess.CommandResult{Stdout: stdout}
}

const (
	antigravityTaskPlanSchema     = `{"type":"object","properties":{"tasks":{"type":"array","minItems":1,"maxItems":1,"items":{"type":"string","minLength":1}}},"required":["tasks"],"additionalProperties":false}`
	antigravityTaskResultSchema   = `{"type":"object","properties":{"result":{"type":"string","minLength":1}},"required":["result"],"additionalProperties":false}`
	antigravityMergerResultSchema = `{"type":"object","properties":{"answer":{"type":"string","minLength":1}},"required":["answer"],"additionalProperties":false}`
)

func antigravityStructuredResult(conversationID string, output map[string]any, schema string) []byte {
	encodedOutput, err := json.Marshal(output)
	if err != nil {
		panic(err)
	}
	encoded, err := json.Marshal(map[string]any{
		"event": "result",
		"result": map[string]any{
			"conversation_id":   conversationID,
			"status":            "SUCCESS",
			"response":          string(encodedOutput),
			"structured_output": output,
			"json_schema":       json.RawMessage(schema),
			"duration_seconds":  1,
			"num_turns":         1,
			"usage":             map[string]any{"input_tokens": 1, "output_tokens": 1, "thinking_tokens": 0, "cache_read_tokens": 0, "total_tokens": 2},
		},
	})
	if err != nil {
		panic(err)
	}
	return append(encoded, '\n')
}
