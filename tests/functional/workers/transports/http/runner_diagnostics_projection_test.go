package http_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	"github.com/portpowered/infinite-you/pkg/services/recordings"
	recordingshttp "github.com/portpowered/infinite-you/pkg/services/recordings/transports/http"
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	runnerDiagnosticsRawPrompt    = "RAW_PROMPT_DIAGNOSTICS_SENTINEL_7e9a"
	runnerDiagnosticsCommandInput = "COMMAND_INPUT_DIAGNOSTICS_SENTINEL_4d2c"
)

// TestRunnerSelectionAndSafeDiagnosticsThroughHTTPProjection exercises the
// customer-facing runner-selection precedence and the generated workstation
// projection from a root-built Factory run. The canonical events are read
// through the public HTTP event endpoint; Recordings then reconstructs the
// selected world state and owns the HTTP representation mapping.
func TestRunnerSelectionAndSafeDiagnosticsThroughHTTPProjection(t *testing.T) {
	dir := support.ScaffoldFactory(t, runnerDiagnosticsFactoryConfig())
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"request":"inspect selected runner"}`))
	runner := support.NewRecordingCommandRunner("safe agent result COMPLETE")

	session, _, events, projection := support.RunFactoryToCompletionWithEdgesAndObservationsAndProjection(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		60*time.Second,
	)
	if session.Runtime.Progress.Categories.Terminal != 1 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf(
			"session progress = %+v, want one terminal and zero failed Work items",
			session.Runtime.Progress.Categories,
		)
	}

	requests := runner.Requests()
	if len(requests) != 1 {
		t.Fatalf("provider command requests = %d, want exactly one selected workstation invocation", len(requests))
	}
	if strings.ToLower(strings.TrimSpace(requests[0].Command)) != "claude" {
		t.Fatalf(
			"provider command = %q, want workstation runner claude; request=%#v",
			requests[0].Command,
			requests[0],
		)
	}
	externalPrompt := string(requests[0].Stdin) + "\n" + strings.Join(requests[0].Args, "\n")
	if !strings.Contains(externalPrompt, runnerDiagnosticsRawPrompt) {
		t.Fatalf("provider command input = %q, want the authored prompt sentinel at the external edge", externalPrompt)
	}

	workstationProjection := generatedWorkstationProjectionFromHTTPEvents(t, projection, events)
	if workstationProjection.WorkstationRequestsByDispatchId == nil || len(*workstationProjection.WorkstationRequestsByDispatchId) != 1 {
		t.Fatalf("workstation projection = %#v, want one dispatch keyed view", workstationProjection)
	}
	for dispatchID, view := range *workstationProjection.WorkstationRequestsByDispatchId {
		if view.Request.Runner == nil || view.Response == nil || view.Response.Runner == nil {
			t.Fatalf("workstation projection[%q] = %#v, want request and response runner views", dispatchID, view)
		}
		assertSelectedClaudeRunner(t, "request", view.Request.Runner)
		assertSelectedClaudeRunner(t, "response", view.Response.Runner)
		assertAgentRunInspection(t, view.Response.AgentRunInspection)

		encoded, err := json.Marshal(view.Response)
		if err != nil {
			t.Fatalf("marshal generated workstation response: %v", err)
		}
		assertSafeDiagnosticPayload(t, string(encoded))
	}

	assertSafeEventDiagnostics(t, events)
}

// TestRunnerDiagnosticsHTTPProjectionIncludesFailureClassification exercises
// the same generated workstation projection for a failed provider turn. The
// public response preserves the bounded agent-run classification while still
// omitting provider command output and other sensitive execution inputs.
func TestRunnerDiagnosticsHTTPProjectionIncludesFailureClassification(t *testing.T) {
	dir := support.ScaffoldFactory(t, runnerDiagnosticsCodexFactoryConfig())
	testutil.WriteSeedFile(t, dir, "task", []byte(`{"request":"inspect failed runner diagnostics"}`))
	runner := support.NewShapedProviderCommandRunner(platformprocess.CommandResult{
		ExitCode: 1,
		Stderr:   []byte("temporary provider failure credential=DIAGNOSTIC_CREDENTIAL_SENTINEL_91f3"),
	})

	session, _, events, projection := support.RunFactoryToCompletionWithEdgesAndObservationsAndProjection(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		60*time.Second,
	)
	if session.Runtime.Progress.Categories.Terminal != 0 || session.Runtime.Progress.Categories.Failed != 1 {
		t.Fatalf(
			"session progress = %+v, want zero terminal and one failed Work item",
			session.Runtime.Progress.Categories,
		)
	}

	workstationProjection := generatedWorkstationProjectionFromHTTPEvents(t, projection, events)
	if workstationProjection.WorkstationRequestsByDispatchId == nil || len(*workstationProjection.WorkstationRequestsByDispatchId) != 1 {
		t.Fatalf("workstation projection = %#v, want one dispatch keyed view", workstationProjection)
	}
	for dispatchID, view := range *workstationProjection.WorkstationRequestsByDispatchId {
		if view.Response == nil || view.Response.AgentRunInspection == nil {
			t.Fatalf("workstation projection[%q] = %#v, want agent-run inspection", dispatchID, view)
		}
		inspection := view.Response.AgentRunInspection
		if inspection.FailureClass == nil || *inspection.FailureClass == "" {
			t.Fatalf("agent-run failure class = %#v, want bounded provider classification", inspection.FailureClass)
		}
		if inspection.ToolPolicy == nil || *inspection.ToolPolicy != "ENABLED" {
			t.Fatalf("agent-run tool policy = %#v, want ENABLED", inspection.ToolPolicy)
		}
		encoded, err := json.Marshal(view.Response)
		if err != nil {
			t.Fatalf("marshal generated workstation response: %v", err)
		}
		assertSafeDiagnosticPayload(t, string(encoded))
	}
}

func runnerDiagnosticsCodexFactoryConfig() map[string]any {
	config := runnerDiagnosticsFactoryConfig()
	config["runner"] = "codex"
	config["workers"].([]map[string]any)[0]["executorProvider"] = "codex"
	config["workstations"].([]any)[0].(map[string]any)["runner"] = "codex"
	return config
}

// TestGeneratedRunnerProjectionPreservesOptionalAgentRunCollections exercises
// the public Recordings HTTP adapter with the optional agent-run fields that an
// operator uses when a provider reports tool lifecycle facts. The adapter is
// the same generated projection boundary used by the HTTP read scenario above;
// this keeps collection and empty-value behavior observable without importing
// the representation mapper directly.
func TestGeneratedRunnerProjectionPreservesOptionalAgentRunCollections(t *testing.T) {
	toolName, phase, detail := "read_file", "success", "bytes=12"
	projection := recordings.WorkstationFactoryWorldWorkstationRequestProjectionSlice{
		WorkstationRequestsByDispatchId: &map[string]recordings.WorkstationFactoryWorldWorkstationRequestView{
			"dispatch-with-tools": {
				Response: &recordings.WorkstationFactoryWorldWorkstationRequestResponseView{
					AgentRunInspection: &workerexecution.SafeAgentRunDiagnostic{
						ExecutionBehavior: workerexecution.AgentRunExecutionBehavior,
						FailureClass:      workerexecution.AgentRunFailureClassProvider,
						RecoveryAction:    "retry the agent run after the provider recovers",
						ToolPolicy:        "ENABLED",
						ToolCallCount:     1,
						ToolDiagnostics: []workerexecution.AgentRunToolDiagnostic{{
							ToolName: toolName,
							Phase:    phase,
							Detail:   detail,
						}},
					},
				},
			},
			"dispatch-without-agent-run": {
				Response: &recordings.WorkstationFactoryWorldWorkstationRequestResponseView{},
			},
		},
	}

	generated := recordingshttp.Generated(projection)
	if generated.WorkstationRequestsByDispatchId == nil {
		t.Fatal("generated workstation projection is nil")
	}
	withTools := (*generated.WorkstationRequestsByDispatchId)["dispatch-with-tools"]
	if withTools.Response == nil || withTools.Response.AgentRunInspection == nil {
		t.Fatalf("generated tool projection = %#v, want agent-run inspection", withTools.Response)
	}
	inspection := withTools.Response.AgentRunInspection
	if inspection.ToolCallCount == nil || *inspection.ToolCallCount != 1 ||
		inspection.ToolDiagnostics == nil || len(*inspection.ToolDiagnostics) != 1 {
		t.Fatalf("generated agent-run collections = %#v, want one tool fact", inspection)
	}
	entry := (*inspection.ToolDiagnostics)[0]
	if entry.ToolName == nil || *entry.ToolName != toolName || entry.Phase == nil || *entry.Phase != phase ||
		entry.Detail == nil || *entry.Detail != detail {
		t.Fatalf("generated tool diagnostic = %#v, want bounded lifecycle fields", entry)
	}
	withoutAgentRun := (*generated.WorkstationRequestsByDispatchId)["dispatch-without-agent-run"]
	if withoutAgentRun.Response == nil || withoutAgentRun.Response.AgentRunInspection != nil {
		t.Fatalf("generated empty agent-run projection = %#v, want omitted inspection", withoutAgentRun.Response)
	}
}

func generatedWorkstationProjectionFromHTTPEvents(
	t *testing.T,
	projection root.RecordingsProjection,
	events []factoryapi.FactoryEvent,
) factoryapi.FactoryWorldWorkstationRequestProjectionSlice {
	t.Helper()
	if len(events) == 0 {
		t.Fatal("HTTP Factory Event read returned no events")
	}
	selectedTick := 0
	for _, event := range events {
		if event.Context.Tick > selectedTick {
			selectedTick = event.Context.Tick
		}
	}

	world, err := recordingshttp.ReconstructFactoryWorldState(
		projection.ReconstructFactoryWorldState,
		events,
		selectedTick,
	)
	if err != nil {
		t.Fatalf("reconstruct Factory World from composed Recordings projection: %v", err)
	}
	return recordingshttp.Generated(projection.ProjectWorkstationRequests(world))
}

func assertSelectedClaudeRunner(
	t *testing.T,
	label string,
	runner *factoryapi.FactoryWorldSelectedRunnerView,
) {
	t.Helper()
	if runner.RunnerId == nil || *runner.RunnerId != factoryapi.RunnerIDClaude {
		t.Fatalf("%s runner ID = %#v, want claude", label, runner.RunnerId)
	}
	if runner.SelectionSource == nil || *runner.SelectionSource != factoryapi.RunnerSelectionSourceWorkstation {
		t.Fatalf("%s runner selection source = %#v, want workstation", label, runner.SelectionSource)
	}
	if runner.DisplayName == nil || *runner.DisplayName != "Claude Code" {
		t.Fatalf("%s runner display name = %#v, want Claude Code", label, runner.DisplayName)
	}
	if runner.Capabilities == nil || len(runner.Capabilities.BaselineCapabilities) != 2 {
		t.Fatalf("%s runner capabilities = %#v, want two baseline capabilities", label, runner.Capabilities)
	}
	if len(runner.Capabilities.OptionalCapabilities) != 5 {
		t.Fatalf("%s optional runner capabilities = %#v, want the complete collection", label, runner.Capabilities.OptionalCapabilities)
	}
	statusByCapability := make(map[string]factoryapi.FactoryWorldRunnerOptionalCapabilityStatus)
	for _, capability := range runner.Capabilities.OptionalCapabilities {
		statusByCapability[string(capability.Capability)] = capability.Status
	}
	if statusByCapability["session_resume"] != factoryapi.FactoryWorldRunnerOptionalCapabilityStatusSupported {
		t.Fatalf("%s session_resume capability = %#v, want supported", label, statusByCapability)
	}
	if statusByCapability["structured_output"] != factoryapi.FactoryWorldRunnerOptionalCapabilityStatusUnsupported {
		t.Fatalf("%s structured_output capability = %#v, want unsupported", label, statusByCapability)
	}
}

func assertAgentRunInspection(
	t *testing.T,
	inspection *factoryapi.FactoryWorldAgentRunInspectionView,
) {
	t.Helper()
	if inspection == nil {
		t.Fatal("generated workstation response agentRunInspection = nil")
	}
	if inspection.ExecutionBehavior == nil || string(*inspection.ExecutionBehavior) != string(factoryapi.AgentRun) {
		t.Fatalf("agent-run execution behavior = %#v, want agent_run", inspection.ExecutionBehavior)
	}
	if inspection.ToolPolicy == nil || *inspection.ToolPolicy != "ENABLED" {
		t.Fatalf("agent-run tool policy = %#v, want ENABLED", inspection.ToolPolicy)
	}
	if inspection.Transcript == nil || len(*inspection.Transcript) == 0 {
		t.Fatalf("agent-run transcript = %#v, want bounded transcript collection", inspection.Transcript)
	}
}

func assertSafeEventDiagnostics(t *testing.T, events []factoryapi.FactoryEvent) {
	t.Helper()
	foundAgentRun := false
	foundProviderSession := false
	foundProviderDiagnostic := false
	for _, event := range events {
		switch event.Type {
		case factoryapi.FactoryEventTypeAgentRunResponse:
			payload, err := event.Payload.AsAgentRunResponseEventPayload()
			if err != nil {
				t.Fatalf("decode AGENT_RUN_RESPONSE: %v", err)
			}
			if payload.Diagnostics == nil || payload.Diagnostics.AgentRun == nil {
				t.Fatalf("AGENT_RUN_RESPONSE diagnostics = %#v, want safe agent-run facts", payload.Diagnostics)
			}
			if payload.Diagnostics.AgentRun.Transcript == nil || len(*payload.Diagnostics.AgentRun.Transcript) == 0 {
				t.Fatalf("AGENT_RUN_RESPONSE transcript = %#v, want collection", payload.Diagnostics.AgentRun.Transcript)
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal AGENT_RUN_RESPONSE payload: %v", err)
			}
			assertSafeDiagnosticPayload(t, string(encoded))
			foundAgentRun = true
		case factoryapi.FactoryEventTypeInferenceResponse:
			payload, err := event.Payload.AsInferenceResponseEventPayload()
			if err != nil {
				t.Fatalf("decode INFERENCE_RESPONSE: %v", err)
			}
			assertProviderDiagnostics(
				t,
				"INFERENCE_RESPONSE",
				payload.ProviderSession,
				payload.Diagnostics,
				"",
				&foundProviderSession,
				&foundProviderDiagnostic,
			)
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal INFERENCE_RESPONSE payload: %v", err)
			}
			assertSafeDiagnosticPayload(t, string(encoded))
		case factoryapi.FactoryEventTypeModelResponse:
			payload, err := event.Payload.AsModelResponseEventPayload()
			if err != nil {
				t.Fatalf("decode MODEL_RESPONSE: %v", err)
			}
			assertProviderDiagnostics(
				t,
				"MODEL_RESPONSE",
				payload.ProviderSession,
				payload.Diagnostics,
				payload.Model,
				&foundProviderSession,
				&foundProviderDiagnostic,
			)
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal MODEL_RESPONSE payload: %v", err)
			}
			assertSafeDiagnosticPayload(t, string(encoded))
		}
	}
	if !foundAgentRun {
		t.Fatal("HTTP Factory Event read has no AGENT_RUN_RESPONSE")
	}
	if !foundProviderSession {
		t.Fatal("HTTP Factory Event read has no Provider Session metadata")
	}
	if !foundProviderDiagnostic {
		t.Fatal("HTTP Factory Event read has no safe provider diagnostics")
	}
}

func assertProviderDiagnostics(
	t *testing.T,
	label string,
	session *factoryapi.ProviderSessionMetadata,
	diagnostics *factoryapi.SafeWorkDiagnostics,
	model string,
	foundSession *bool,
	foundDiagnostic *bool,
) {
	t.Helper()
	if session == nil || session.Id == nil || *session.Id == "" ||
		session.Provider == nil || *session.Provider != "claude" {
		t.Fatalf("%s provider session metadata = %#v, want Claude session identity", label, session)
	}
	*foundSession = true
	if diagnostics == nil || diagnostics.Provider == nil {
		t.Fatalf("%s diagnostics = %#v, want safe provider facts", label, diagnostics)
	}
	provider := diagnostics.Provider
	if provider.Provider == nil || *provider.Provider != "claude" {
		t.Fatalf("%s provider diagnostics = %#v, want Claude provider", label, provider)
	}
	if strings.TrimSpace(model) == "" && (provider.Model == nil || strings.TrimSpace(*provider.Model) == "") {
		t.Fatalf("%s provider diagnostics = %#v, want a model fact", label, provider)
	}
	if provider.ResponseMetadata == nil || len(*provider.ResponseMetadata) == 0 {
		t.Fatalf("%s provider response metadata = %#v, want safe collection", label, provider.ResponseMetadata)
	}
	*foundDiagnostic = true
}

func assertSafeDiagnosticPayload(t *testing.T, payload string) {
	t.Helper()
	for _, forbidden := range []string{
		runnerDiagnosticsRawPrompt,
		runnerDiagnosticsCommandInput,
		"DIAGNOSTIC_CREDENTIAL_SENTINEL_91f3",
		"DIAGNOSTIC_ENV_SENTINEL_2b70",
	} {
		if strings.Contains(payload, forbidden) {
			t.Fatalf("generated safe diagnostics contain forbidden raw value %q: %s", forbidden, payload)
		}
	}
}

func runnerDiagnosticsFactoryConfig() map[string]any {
	return map[string]any{
		"name":   "runner-diagnostics-projection",
		"runner": "antigravity",
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "done", "type": "TERMINAL"},
				{"name": "failed", "type": "FAILED"},
			},
		}},
		"workers": []map[string]any{{
			"name":             "diagnostics-worker",
			"type":             "AGENT_WORKER",
			"model":            "runner-diagnostics-model",
			"modelProvider":    "codex",
			"executorProvider": "claude",
			"skipPermissions":  true,
			"agentTools": map[string]any{
				"policy": "ENABLED",
			},
			"body": "Agent worker system instructions.",
		}},
		// []any keeps ScaffoldFactory from creating a generated MODEL_WORKSTATION
		// markdown file and preserves this authored AGENT_RUN definition.
		"workstations": []any{
			map[string]any{
				"name":   "diagnostics-review",
				"type":   "AGENT_RUN",
				"worker": "diagnostics-worker",
				"runner": "claude",
				"body": "Review the submitted task and return a concise completion. " +
					runnerDiagnosticsCommandInput + " " +
					"DIAGNOSTIC_CREDENTIAL_SENTINEL_91f3 " +
					"DIAGNOSTIC_ENV_SENTINEL_2b70 " +
					runnerDiagnosticsRawPrompt,
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "done"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
			},
		},
	}
}
