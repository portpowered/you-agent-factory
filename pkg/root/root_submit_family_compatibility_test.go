package root

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/providers"
	providerswire "github.com/portpowered/infinite-you/pkg/services/providers/wire"
	"github.com/portpowered/infinite-you/pkg/services/workers"
)

const rootSubmitBatch = `{
	"requestId": "agent-batch",
	"type": "FACTORY_REQUEST_BATCH",
	"works": [
		{"name": "review", "workTypeName": "task", "payload": {"title": "Review"}},
		{"name": "publish", "workTypeName": "task", "payload": {"title": "Publish"}}
	],
	"relations": [
		{"type": "DEPENDS_ON", "sourceWorkName": "publish", "targetWorkName": "review", "requiredState": "complete"}
	]
}`

type observedSubmitRequest struct {
	method string
	path   string
	body   []byte
}

type batchCompatibilityCase struct {
	name        string
	args        func(string) []string
	stdin       string
	stdinIsTTY  bool
	wantSession string
	wantSource  string
}

func TestSubmitBatchDocumentedIngressThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	batchFile := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(batchFile, []byte(rootSubmitBatch), 0o600); err != nil {
		t.Fatalf("write batch fixture: %v", err)
	}
	tests := []batchCompatibilityCase{
		{
			name: "positional file",
			args: func(server string) []string {
				return []string{"you", "--server", server, "submit", "batch", batchFile}
			},
			stdin:      `{"ignored":"stdin"}`,
			stdinIsTTY: true,
			wantSource: "file",
		},
		{
			name: "file flag precedence",
			args: func(server string) []string {
				return []string{"you", "--server", server, "submit", "batch", "--file", batchFile, `{"ignored":"positional"}`}
			},
			stdin:      `{"ignored":"stdin"}`,
			stdinIsTTY: false,
			wantSource: "file",
		},
		{
			name: "explicit stdin",
			args: func(server string) []string {
				return []string{"you", "--server", server, "submit", "batch", "-"}
			},
			stdin:      rootSubmitBatch,
			stdinIsTTY: false,
			wantSource: "stdin",
		},
		{
			name: "implicit piped stdin",
			args: func(server string) []string {
				return []string{"you", "--server", server, "submit", "batch"}
			},
			stdin:      rootSubmitBatch,
			stdinIsTTY: false,
			wantSource: "stdin",
		},
		{
			name: "inline JSON and named session",
			args: func(server string) []string {
				return []string{
					"you", "--server", server, "--json", "submit", "batch",
					"--session", "session-agent", rootSubmitBatch,
				}
			},
			stdinIsTTY:  true,
			wantSession: "session-agent",
			wantSource:  "inline",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runBatchCompatibilityCase(t, test)
		})
	}
}

func TestSubmitBatchLocalFailuresAndDryRunAvoidHTTPThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	t.Cleanup(server.Close)
	process := buildSubmitCompatibilityProcess(t)

	invalid := submitCompatibilityInput(
		t,
		[]string{"you", "--server", server.URL, "submit", "batch", `{"type":"FACTORY_REQUEST_BATCH"}`},
		"",
		true,
	)
	if err := process.Execute(invalid); err == nil {
		t.Fatal("Process.Execute(invalid batch) error = nil")
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid batch HTTP calls = %d, want 0", calls.Load())
	}

	dryRun := submitCompatibilityInput(
		t,
		[]string{"you", "--server", "http://127.0.0.1:1", "submit", "batch", "--dry-run", rootSubmitBatch},
		"",
		true,
	)
	if err := process.Execute(dryRun); err != nil {
		t.Fatalf("Process.Execute(dry-run against unreachable server) error = %v", err)
	}
	for _, marker := range []string{
		"requestId: agent-batch",
		"relationCount: 1",
		"batchSource: inline",
		"dry-run: no request sent",
	} {
		if !strings.Contains(inputOutput(dryRun), marker) {
			t.Fatalf("dry-run output omitted %q: %q", marker, inputOutput(dryRun))
		}
	}
}

func TestUnarySubmitCompatibilityThroughRootBuildProcess(t *testing.T) {
	t.Parallel()

	payloadPath := filepath.Join(t.TempDir(), "request.md")
	if err := os.WriteFile(payloadPath, []byte("# Review\n\nCheck the release."), 0o600); err != nil {
		t.Fatalf("write unary payload fixture: %v", err)
	}
	tests := []struct {
		name        string
		jsonOutput  bool
		sessionID   string
		wantPath    string
		wantMarkers []string
	}{
		{
			name:        "default session human output",
			wantPath:    "/factory-sessions/~default/work",
			wantMarkers: []string{"Submitted: release-review (task)", "traceId: unary-trace", "workId: unary-work"},
		},
		{
			name:        "named session JSON output",
			jsonOutput:  true,
			sessionID:   "session-unary",
			wantPath:    "/factory-sessions/session-unary/work",
			wantMarkers: []string{`"sessionId":"session-unary"`, `"name":"release-review"`, `"workTypeName":"task"`},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runUnaryCompatibilityCase(
				t,
				payloadPath,
				test.jsonOutput,
				test.sessionID,
				test.wantPath,
				test.wantMarkers,
			)
		})
	}

	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	t.Cleanup(server.Close)
	missing := submitCompatibilityInput(
		t,
		[]string{"you", "--server", server.URL, "submit", "--name", "release-review", "--payload", payloadPath},
		"",
		true,
	)
	err := buildSubmitCompatibilityProcess(t).Execute(missing)
	if err == nil || !strings.Contains(err.Error(), "--work-type-name") {
		t.Fatalf("Process.Execute(missing required input) error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("missing required input HTTP calls = %d, want 0", calls.Load())
	}
}

func runBatchCompatibilityCase(t *testing.T, test batchCompatibilityCase) {
	t.Helper()
	var observation observedSubmitRequest
	server := newSubmitCompatibilityServer(t, &observation)
	input := submitCompatibilityInput(t, test.args(server.URL), test.stdin, test.stdinIsTTY)
	if err := buildSubmitCompatibilityProcess(t).Execute(input); err != nil {
		t.Fatalf("Process.Execute(%s) error = %v", test.name, err)
	}

	wantSession := test.wantSession
	if wantSession == "" {
		wantSession = "~default"
	}
	wantPath := "/factory-sessions/" + wantSession + "/work-requests/agent-batch"
	if observation.method != http.MethodPut || observation.path != wantPath {
		t.Fatalf("request = %s %s, want PUT %s", observation.method, observation.path, wantPath)
	}
	assertCanonicalAgentBatch(t, observation.body)
	output := inputOutput(input)
	if !strings.Contains(output, `"requestId":"agent-batch"`) &&
		!strings.Contains(output, "requestId: agent-batch") {
		t.Fatalf("submit output omitted stable request ID: %q", output)
	}
	if test.wantSession != "" && !strings.Contains(output, `"sessionId":"session-agent"`) {
		t.Fatalf("named-session JSON output = %q", output)
	}
	if test.wantSession != "" && !strings.Contains(output, test.wantSource) {
		t.Fatalf("submit output = %q, want source %q", output, test.wantSource)
	}
}

func runUnaryCompatibilityCase(
	t *testing.T,
	payloadPath string,
	jsonOutput bool,
	sessionID string,
	wantPath string,
	wantMarkers []string,
) {
	t.Helper()
	var observation observedSubmitRequest
	server := newSubmitCompatibilityServer(t, &observation)
	args := []string{"you", "--server", server.URL}
	if jsonOutput {
		args = append(args, "--json")
	}
	args = append(args,
		"submit",
		"--name", "release-review",
		"--work-type-name", "task",
		"--payload", payloadPath,
	)
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	input := submitCompatibilityInput(t, args, "", true)
	if err := buildSubmitCompatibilityProcess(t).Execute(input); err != nil {
		t.Fatalf("Process.Execute(unary submit) error = %v", err)
	}
	if observation.method != http.MethodPost || observation.path != wantPath {
		t.Fatalf("request = %s %s, want POST %s", observation.method, observation.path, wantPath)
	}
	var body map[string]any
	if err := json.Unmarshal(observation.body, &body); err != nil {
		t.Fatalf("decode unary request: %v", err)
	}
	if body["name"] != "release-review" || body["workTypeName"] != "task" {
		t.Fatalf("unary request body = %#v", body)
	}
	for _, marker := range wantMarkers {
		if !strings.Contains(inputOutput(input), marker) {
			t.Fatalf("unary output omitted %q: %q", marker, inputOutput(input))
		}
	}
}

func buildSubmitCompatibilityProcess(t *testing.T) interface{ Execute(Input) error } {
	t.Helper()
	process, err := BuildProcess(t.Context(), serviceedges.Edges{})
	if err != nil {
		t.Fatalf("BuildProcess() error = %v", err)
	}
	return process
}

func submitCompatibilityInput(
	t *testing.T,
	args []string,
	stdin string,
	stdinIsTTY bool,
) Input {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	return Input{
		Args:             args,
		Env:              homeEnvironment(t.TempDir()),
		Stdin:            strings.NewReader(stdin),
		Stdout:           &stdout,
		Stderr:           &stderr,
		Context:          context.Background(),
		WorkingDirectory: t.TempDir(),
		StdinIsTTY:       &stdinIsTTY,
	}
}

func inputOutput(input Input) string {
	return input.Stdout.(*bytes.Buffer).String()
}

func newSubmitCompatibilityServer(
	t *testing.T,
	observation *observedSubmitRequest,
) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		*observation = observedSubmitRequest{method: r.Method, path: r.URL.Path, body: body}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{
				"accepted": true,
				"requestId": "unary-request",
				"traceId": "unary-trace",
				"workId": "unary-work",
				"name": "release-review",
				"workTypeName": "task"
			}`)
			return
		}
		_, _ = io.WriteString(w, `{
			"requestId": "agent-batch",
			"traceId": "batch-trace",
			"works": [
				{"name": "review", "workTypeName": "task", "workId": "work-review"},
				{"name": "publish", "workTypeName": "task", "workId": "work-publish"}
			]
		}`)
	}))
	t.Cleanup(server.Close)
	return server
}

func assertCanonicalAgentBatch(t *testing.T, body []byte) {
	t.Helper()
	var batch struct {
		RequestID string `json:"requestId"`
		Type      string `json:"type"`
		Works     []struct {
			Name         string `json:"name"`
			WorkTypeName string `json:"workTypeName"`
		} `json:"works"`
		Relations []struct {
			Type           string `json:"type"`
			SourceWorkName string `json:"sourceWorkName"`
			TargetWorkName string `json:"targetWorkName"`
			RequiredState  string `json:"requiredState"`
		} `json:"relations"`
	}
	if err := json.Unmarshal(body, &batch); err != nil {
		t.Fatalf("decode canonical batch request: %v", err)
	}
	if batch.RequestID != "agent-batch" || batch.Type != "FACTORY_REQUEST_BATCH" {
		t.Fatalf("canonical batch identity = %#v", batch)
	}
	if len(batch.Works) != 2 ||
		batch.Works[0].Name != "review" ||
		batch.Works[1].Name != "publish" ||
		batch.Works[0].WorkTypeName != "task" ||
		batch.Works[1].WorkTypeName != "task" {
		t.Fatalf("canonical batch works = %#v", batch.Works)
	}
	if len(batch.Relations) != 1 ||
		batch.Relations[0].Type != "DEPENDS_ON" ||
		batch.Relations[0].SourceWorkName != "publish" ||
		batch.Relations[0].TargetWorkName != "review" ||
		batch.Relations[0].RequiredState != "complete" {
		t.Fatalf("canonical batch relationships = %#v", batch.Relations)
	}
}

// TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot proves the
// standalone Workers root composes and executes without opening Factory Runtime
// or a Factory Session. This is a root contract, not an application workflow.
func TestBuildStatelessWorkersExecutesDetachedAttemptThroughRoot(t *testing.T) {
	t.Parallel()

	service := mustBuildStatelessWorkers(t, serviceedges.Edges{
		ScriptCommandRunner: statelessCommandRunner{},
	})
	assertStatelessScriptExecution(t, service)
	assertStatelessPromptContracts(t, service)
	assertStatelessModelRecording(t, service)
	if service, err := BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ProviderRegistrations: []providerswire.Registration{{
			Manifest:    providerswire.Manifest{ID: "codex"},
			Integration: providerswire.ProgressingExternalIntegration("codex", "unused"),
		}},
	}); service != nil || err == nil || !strings.Contains(err.Error(), "provider registry validation failed") {
		t.Fatalf("invalid stateless provider registration = (%#v, %v), want provider registry validation failure", service, err)
	}
}

func mustBuildStatelessWorkers(t *testing.T, edges serviceedges.Edges) workers.Service {
	t.Helper()
	service, err := BuildStatelessWorkers(t.Context(), edges)
	if err != nil {
		t.Fatalf("BuildStatelessWorkers() error = %v", err)
	}
	return service
}

func assertStatelessScriptExecution(t *testing.T, service workers.Service) {
	t.Helper()
	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-contract",
			RuntimeID:        "runtime-contract",
			GenerationID:     "generation-contract",
			DispatchID:       "contract-stateless-dispatch",
			AttemptID:        "contract-stateless-attempt",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "script-worker",
			RunnerID:   "script",
			Command:    "contract-stateless-script",
		},
	})
	if err != nil {
		t.Fatalf("stateless Execute() error = %v", err)
	}
	if result.Outcome != workers.ExecutionOutcomeAccepted || len(result.Output.Primary) != 1 ||
		result.Output.Primary[0].Text != "stateless-contract-output" {
		t.Fatalf("stateless result = %#v, want accepted contract output", result)
	}
}

func assertStatelessPromptContracts(t *testing.T, service workers.Service) {
	t.Helper()
	promptTemplates, ok := service.(workers.PromptTemplates)
	if !ok {
		t.Fatal("stateless Workers root does not expose prompt template contracts")
	}
	contract := promptTemplates.BuildPromptTemplateContract(1, []string{"factory/docs/guide.md"})
	if contract.InputCount != 1 || len(contract.AvailableVariables) == 0 {
		t.Fatalf("prompt contract = %#v, want selected input variables", contract)
	}
	validation := promptTemplates.ValidatePromptTemplate("{{ .Context.Project }}", 1, nil)
	if !validation.Valid || len(validation.Diagnostics) != 0 {
		t.Fatalf("prompt validation = %#v, want valid detached template", validation)
	}
	fieldResolver, ok := service.(interface {
		ResolveTemplateFields(string, map[string]string, []workers.Token, *workers.Context, string) (*workers.ResolvedTemplateFields, error)
	})
	if !ok {
		t.Fatal("stateless Workers root does not expose template field resolution")
	}
	fields, err := fieldResolver.ResolveTemplateFields(
		"{{.Context.WorkDir}}", map[string]string{"TOKEN": "{{.Context.Project}}"}, nil,
		&workers.Context{WorkDirectory: "/workspace", ProjectID: "project-1"}, "",
	)
	if err != nil || fields.WorkingDirectory != "/workspace" || fields.Env["TOKEN"] != "project-1" {
		t.Fatalf("resolved fields = %#v, error = %v, want detached context values", fields, err)
	}
}

func assertStatelessModelRecording(t *testing.T, service workers.Service) {
	t.Helper()
	recorder, ok := service.(interface{ RuntimeOwnsModelEventRecording() bool })
	if !ok || !recorder.RuntimeOwnsModelEventRecording() {
		t.Fatal("stateless Workers root does not own model event recording")
	}
}

// TestBuildStatelessWorkersExecutesProviderAttemptThroughRoot proves the
// standalone Workers root reaches the Providers-owned command boundary for a
// detached agent attempt without opening Factory Runtime or a Factory Session.
func TestBuildStatelessWorkersExecutesProviderAttemptThroughRoot(t *testing.T) {
	t.Parallel()

	runner := testutil.NewProviderCommandRunner(platformprocess.CommandResult{
		Stdout: statelessCodexSuccessStdout("stateless-provider-output"),
	})
	service := mustBuildStatelessWorkers(t, serviceedges.Edges{
		ProviderCommandRunner: runner,
	})
	result := executeStatelessProviderAttempt(t, service)
	if result.Outcome != workers.ExecutionOutcomeAccepted ||
		len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "stateless-provider-output" {
		t.Fatalf("stateless provider result = %#v, want accepted contract output", result)
	}
	if runner.CallCount() != 1 {
		t.Fatalf("provider command calls = %d, want one", runner.CallCount())
	}
}

func executeStatelessProviderAttempt(t *testing.T, service workers.Service) workers.ExecuteResult {
	t.Helper()
	result, err := service.Execute(context.Background(), workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-contract-provider",
			RuntimeID:        "runtime-contract-provider",
			GenerationID:     "generation-contract-provider",
			DispatchID:       "contract-provider-dispatch",
			AttemptID:        "contract-provider-attempt",
		},
		Target: workers.ExecutionTarget{
			WorkerName: "agent-worker",
			RunnerID:   "agent",
			Provider:   workers.ProviderReference{ID: "codex"},
			Prompt:     workers.PromptPolicy{UserMessage: "answer the contract provider test"},
		},
	})
	if err != nil {
		t.Fatalf("stateless provider Execute() error = %v", err)
	}
	return result
}

func TestBuildStatelessWorkersPreservesDetachedAgentContractThroughRoot(t *testing.T) {
	t.Parallel()

	provider := testutil.NewNativeMockProvider(providers.ExecuteResult{
		Content: `{"decision":"ACCEPTED","feedback":"ready","output":"ship"}`,
		SessionRef: &providers.SessionRef{
			Provider: providers.IDCodex,
			Kind:     providers.SessionIDKind,
			ID:       "contract-agent-session",
		},
	})
	service, err := BuildStatelessWorkers(t.Context(), serviceedges.Edges{
		ProviderOverride: provider,
	})
	if err != nil {
		t.Fatalf("BuildStatelessWorkers() error = %v", err)
	}

	request := detachedAgentContractRequest()
	result, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("detached agent Execute() error = %v", err)
	}
	assertDetachedAgentResult(t, result)
	assertDetachedAgentProviderCall(t, provider, request)
}

func detachedAgentContractRequest() workers.ExecuteRequest {
	return workers.ExecuteRequest{
		Correlation: workers.ExecutionCorrelation{
			FactorySessionID: "session-contract-agent",
			RuntimeID:        "runtime-contract-agent",
			GenerationID:     "generation-contract-agent",
			DispatchID:       "contract-agent-dispatch",
			AttemptID:        "contract-agent-attempt",
		},
		Target: workers.ExecutionTarget{
			WorkerName:      "agent-worker",
			WorkstationName: "execute-goal",
			RunnerID:        "agent",
			Provider:        workers.ProviderReference{ID: string(providers.IDCodex)},
			Prompt:          workers.PromptPolicy{UserMessage: "complete this goal"},
			Tools:           workers.ToolPolicy{AgentLoop: true, AgentToolPolicy: "DISABLED"},
			Output:          workers.OutputPolicy{DecisionEnvelope: true, GoalRoutingDecisionEnvelope: true},
		},
	}
}

func assertDetachedAgentResult(t *testing.T, result workers.ExecuteResult) {
	t.Helper()
	if result.Outcome != workers.ExecutionOutcomeAccepted {
		t.Fatalf("outcome = %q, failure = %#v, want ACCEPTED", result.Outcome, result.Failure)
	}
	if len(result.Output.Primary) != 1 || result.Output.Primary[0].Text != "ship" ||
		result.Output.Feedback != "ready" || result.Output.Classification != "accepted" {
		t.Fatalf("output = %#v, want normalized goal decision-envelope output", result.Output)
	}
	if result.Continuation == nil || result.Continuation.Provider != string(providers.IDCodex) ||
		result.Continuation.ProviderSessionID != "contract-agent-session" {
		t.Fatalf("continuation = %#v, want exact detached provider identity", result.Continuation)
	}
	if result.Diagnostics == nil ||
		result.Diagnostics.Metadata[workers.AgentRunMetadataExecutionBehavior] != workers.AgentRunExecutionBehavior ||
		result.Diagnostics.Metadata[workers.AgentRunMetadataToolPolicy] != "DISABLED" {
		t.Fatalf("diagnostics = %#v, want safe detached agent-run metadata", result.Diagnostics)
	}
}

func assertDetachedAgentProviderCall(
	t *testing.T,
	provider *testutil.NativeMockProvider,
	request workers.ExecuteRequest,
) {
	t.Helper()
	if provider.CallCount() != 1 {
		t.Fatalf("provider calls = %d, want one detached agent attempt", provider.CallCount())
	}
	call := provider.Calls()[0]
	if call.Correlation.DispatchID != request.Correlation.DispatchID ||
		call.Correlation.AttemptID != request.Correlation.AttemptID {
		t.Fatalf("provider correlation = %#v, want detached request correlation", call.Correlation)
	}
}

type statelessCommandRunner struct{}

func (statelessCommandRunner) Run(context.Context, platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	return platformprocess.CommandResult{Stdout: []byte("stateless-contract-output")}, nil
}

func statelessCodexSuccessStdout(result string) []byte {
	item, err := json.Marshal(map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   "root-contract-message",
			"type": "agent_message",
			"text": result,
		},
	})
	if err != nil {
		panic(err)
	}
	return append([]byte(`{"type":"turn.started"}`+"\n"), append(item, []byte("\n{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1,\"output_tokens\":1}}\n")...)...)
}
