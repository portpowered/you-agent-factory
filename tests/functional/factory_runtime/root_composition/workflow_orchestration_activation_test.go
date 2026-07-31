package root_composition_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	factoryRuntimeJavaScriptFactoryName   = "factory-runtime-js-workflow"
	factoryRuntimeJavaScriptSuccessResult = "factory-runtime-js:<SYNC_SUCCESS>"
)

// TestFactoryRuntimeJavaScriptWorkflowActivatesThroughRootBuildProcessAfterLifecycle
// proves an existing public JavaScript workflow surface produces a successful
// outcome after runtime lifecycle on a process composed only through
// root.BuildProcess with Factory Runtime effects replaced via edges.Edges.
func TestFactoryRuntimeJavaScriptWorkflowActivatesThroughRootBuildProcessAfterLifecycle(
	t *testing.T,
) {
	t.Parallel()

	homeDir := t.TempDir()
	sourceDir := scaffoldFactoryRuntimeJavaScriptWorkflowFactory(t)
	namedFactoryDir := support.CreateNamedFactory(
		t,
		homeDir,
		sourceDir,
		factoryRuntimeJavaScriptFactoryName,
		filepath.Join(sourceDir, interfaces.FactoryConfigFile),
	)

	recorder := newFactoryRuntimeDelegatingRecorder(t, homeDir)
	providerRunner := support.NewRecordingCommandRunner("unexpected live provider execution")
	edges := recorder.edges()
	edges.ProviderCommandRunner = providerRunner

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                namedFactoryDir,
		WorkingDirectory:          t.TempDir(),
		WaitForServiceModeRuntime: true,
		UseMockWorkers:            true,
		Env:                       []string{"HOME=" + homeDir, "USERPROFILE=" + homeDir},
		Edges:                     edges,
	})
	t.Cleanup(func() { server.Stop(t) })

	baseURL := server.URL()
	support.WaitForRuntimeIdle(t, baseURL, 10*time.Second)

	workflowBefore := recorder.totalJavaScriptOrchestration()
	response := postFactoryRuntimeJavaScriptSyncExecution(t, baseURL, map[string]any{
		"input": "structured-sync",
	})
	if response.Result == nil || response.Result.ResultStatus != factoryapi.FactorySessionResultStatusFinal {
		t.Fatalf("sync result = %#v, want FINAL primary outcome", response.Result)
	}
	if response.Result.PrimaryResult == nil || len(*response.Result.PrimaryResult) != 1 {
		t.Fatalf("primary result = %#v, want one content part", response.Result.PrimaryResult)
	}
	part, err := (*response.Result.PrimaryResult)[0].AsWorkTextContentPart()
	if err != nil {
		t.Fatalf("decode primary result: %v", err)
	}
	if part.Text != factoryRuntimeJavaScriptSuccessResult {
		t.Fatalf("primary result = %q, want %q", part.Text, factoryRuntimeJavaScriptSuccessResult)
	}
	if providerRunner.CallCount() != 0 {
		t.Fatalf("provider command runner call count = %d, want 0 for inline JavaScript workflow", providerRunner.CallCount())
	}
	if got := recorder.totalJavaScriptOrchestration() - workflowBefore; got <= 0 {
		t.Fatalf("JavaScript workflow effect calls after sync execution = %d, want > 0 via edges", got)
	}
}

// TestFactoryRuntimePetriOrchestrationActivatesThroughRootBuildProcessAfterLifecycle
// proves an existing public orchestration surface routes multi-transition work
// to documented terminals after runtime lifecycle on the same public-process path.
// Complements the CLI-owned proofs in tests/functional/factory_runtime/orchestrators/petri/routing.
func TestFactoryRuntimePetriOrchestrationActivatesThroughRootBuildProcessAfterLifecycle(
	t *testing.T,
) {
	t.Parallel()

	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "service_simple"))
	recorder := newFactoryRuntimeDelegatingRecorder(t)
	providerRunner := testutil.NewProviderCommandRunner(support.AcceptedCommandResults(2)...)
	edges := recorder.edges()
	edges.ProviderCommandRunner = providerRunner

	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                dir,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	t.Cleanup(func() { server.Stop(t) })

	baseURL := server.URL()
	support.WaitForRuntimeIdle(t, baseURL, 10*time.Second)

	dispatchBefore := recorder.totalDispatchPlan()
	traceID := "factory-runtime-orchestration-activation"
	submitted := support.SubmitDefaultSessionWork(t, baseURL, factoryapi.SubmitWorkRequest{
		Name:         stringPointer("factory-runtime-orchestration-activation"),
		WorkTypeName: "task",
		TraceId:      &traceID,
		Payload:      map[string]string{"title": "activate orchestration through public process"},
	})
	workID := stringPointerValue(submitted.WorkId)
	if workID == "" {
		t.Fatalf("submit work missing work id: %#v", submitted)
	}

	support.WaitForRuntimeIdle(t, baseURL, 15*time.Second)
	if got := recorder.totalDispatchPlan() - dispatchBefore; got <= 0 {
		t.Fatalf("dispatch-plan effect calls after orchestration = %d, want > 0 via edges", got)
	}
	if providerRunner.CallCount() != 2 {
		t.Fatalf("provider command call count = %d, want 2 for two-stage routing", providerRunner.CallCount())
	}

	terminal := support.WorkCustomerLocation("task", "complete")
	listed := support.ListDefaultSessionWork(t, baseURL)
	if !support.HasWorkAtCustomerState(listed, workID, terminal) {
		t.Fatalf("work %q did not reach %s: %#v", workID, terminal, listed.Results)
	}
	if support.CountWorkAtCustomerState(listed, support.WorkCustomerLocation("task", "processing")) != 0 {
		t.Fatalf("processing Work still present after orchestration completion: %#v", listed.Results)
	}

	events := support.GetFactoryEventsAt(t, baseURL)
	dispatches := support.ObserveDispatchEvents(t, events)
	foundCompleted := false
	for _, dispatch := range dispatches {
		if !support.DispatchObservationIncludesWork(dispatch, workID) || dispatch.Response == nil {
			continue
		}
		foundCompleted = true
		break
	}
	if !foundCompleted {
		t.Fatalf("public Factory Events missing completed dispatch for orchestrated work %q", workID)
	}
}

func scaffoldFactoryRuntimeJavaScriptWorkflowFactory(t *testing.T) string {
	t.Helper()

	return support.ScaffoldFactory(t, map[string]any{
		"name": factoryRuntimeJavaScriptFactoryName,
		"invocationSignature": map[string]any{
			"parameters": []any{map[string]any{
				"name":     "input",
				"required": true,
				"bindings": []any{map[string]any{"kind": "POSITIONAL", "position": 1}},
			}},
		},
		"orchestrator": map[string]any{
			"kind": "JAVASCRIPT",
			"javascript": map[string]any{
				"inlineSource": map[string]any{
					"encoding": "utf-8",
					"inline":   `workflow.final("` + factoryRuntimeJavaScriptSuccessResult + `");`,
				},
				"argsSchema": map[string]any{
					"type":                 "object",
					"properties":           map[string]any{"input": map[string]any{"type": "string"}},
					"additionalProperties": false,
				},
			},
		},
	})
}

func postFactoryRuntimeJavaScriptSyncExecution(
	t *testing.T,
	baseURL string,
	args map[string]any,
) factoryapi.FactorySessionSyncExecutionResponse {
	t.Helper()

	factoryID := factoryRuntimeJavaScriptFactoryName
	payload, err := json.Marshal(factoryapi.FactorySessionExecutionRequest{
		RequestId: "factory-runtime-js-workflow-sync",
		Args:      &args,
		Source: factoryapi.FactorySessionExecutionSource{
			Kind:      factoryapi.FactorySessionExecutionSourceKindFactoryId,
			FactoryId: &factoryID,
		},
	})
	if err != nil {
		t.Fatalf("marshal sync execution request: %v", err)
	}

	endpoint := strings.TrimSuffix(baseURL, "/") + "/factory-sessions/sync"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("POST %s status = %d, want 200: %s", endpoint, response.StatusCode, body)
	}

	var decoded factoryapi.FactorySessionSyncExecutionResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode sync execution response: %v", err)
	}
	return decoded
}
