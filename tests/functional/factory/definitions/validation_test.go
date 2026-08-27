package definitions

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	"github.com/portpowered/infinite-you/pkg/root"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	modelprovider "github.com/portpowered/infinite-you/pkg/services/models"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	validationCodeDuplicateIdentifier        = "factory.duplicateIdentifier"
	validationCodeDanglingWorkerReference    = "factory.worker.danglingReference"
	validationCodeDanglingPlaceReference     = "factory.route.danglingPlaceReference"
	validationCodeLayoutUnknownNodeReference = "factory.layout.unknownNodeReference"
)

// TestFactoryValidationAcceptsMultiWorkTypeExecutableTopology proves a
// multi-work-type Factory definition is accepted as valid executable topology
// and both Work types reach terminal success through the public process
// boundary with asserted provider-process edge captures.
func TestFactoryValidationAcceptsMultiWorkTypeExecutableTopology(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, "multi_work_type"))
	support.WriteAgentConfig(
		t,
		dir,
		"request-handler",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"),
	)
	support.WriteAgentConfig(
		t,
		dir,
		"review-handler",
		support.BuildModelWorkerConfig(modelprovider.ProviderCodex, "test-model"),
	)

	testutil.WriteSeedFile(t, dir, "request", []byte(`{"title": "New request"}`))
	testutil.WriteSeedFile(t, dir, "review", []byte(`{"title": "New review"}`))

	providerCallsBefore := sharedDefinitionsProviderCallCount(t)
	assertFactoryValidationAccepts(t, dir)
	if providerCalls := sharedDefinitionsProviderCallCount(t); providerCalls != providerCallsBefore {
		t.Fatalf(
			"provider command runner calls during validate = %d, want unchanged %d",
			providerCalls,
			providerCallsBefore,
		)
	}

	runner := support.NewShapedProviderCommandRunner(
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("Request handled. COMPLETE")},
		platformprocess.CommandResult{Stdout: support.CodexSuccessStdout("Review handled. COMPLETE")},
	)

	session, listed := support.RunFactoryToCompletionWithEdgesAndWork(
		t,
		dir,
		serviceedges.Edges{ProviderCommandRunner: runner},
		15*time.Second,
	)

	if session.Runtime.Progress.Categories.Terminal != 2 || session.Runtime.Progress.Categories.Failed != 0 {
		t.Fatalf(
			"session progress categories = %+v, want two terminal and zero failed",
			session.Runtime.Progress.Categories,
		)
	}
	if got := support.CountWorkAtCustomerState(listed, "request:complete"); got != 1 {
		t.Fatalf("request:complete count = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "review:complete"); got != 1 {
		t.Fatalf("review:complete count = %d, want 1; listed=%#v", got, listed)
	}
	if got := support.CountWorkAtCustomerState(listed, "request:init"); got != 0 {
		t.Fatalf("request:init count = %d, want 0 after completion", got)
	}
	if got := support.CountWorkAtCustomerState(listed, "review:init"); got != 0 {
		t.Fatalf("review:init count = %d, want 0 after completion", got)
	}
	if runner.CallCount() != 2 {
		t.Fatalf("provider command runner calls = %d, want 2", runner.CallCount())
	}
}

// TestFactoryValidationRejectsMissingWorkerWorkstationAndRoute proves public
// Factory validation rejects authored definitions that reference missing workers,
// workstations, or routes before runtime execution and reports customer-visible
// diagnostics through the CLI validate path.
func TestFactoryValidationRejectsMissingWorkerWorkstationAndRoute(t *testing.T) {
	process := buildDefinitionsProcess(t)
	providerCallsBefore := sharedDefinitionsProviderCallCount(t)

	t.Run("missing_worker", func(t *testing.T) {
		dir := support.ScaffoldFactory(t, missingWorkerFactoryConfig())
		assertFactoryValidationRejectsWithProcess(
			t,
			process,
			dir,
			validationCodeDanglingWorkerReference,
			`ghost-worker`,
			`workstation "processor" references non-existent worker "ghost-worker"`,
		)
	})

	t.Run("missing_workstation", func(t *testing.T) {
		dir := support.ScaffoldFactory(t, missingWorkstationFactoryConfig())
		assertFactoryValidationRejectsWithProcess(
			t,
			process,
			dir,
			validationCodeLayoutUnknownNodeReference,
			`workstation:ghost-workstation`,
			`layout node "workstation:ghost-workstation" does not match any pending graph node`,
		)
	})

	t.Run("missing_route", func(t *testing.T) {
		dir := support.ScaffoldFactory(t, missingRouteFactoryConfig())
		assertFactoryValidationRejectsWithProcess(
			t,
			process,
			dir,
			validationCodeDanglingPlaceReference,
			`missing-state`,
			`references non-existent state "missing-state" of work type "task"`,
			`ROUTE(process->task:missing-state)`,
		)
	})

	if providerCalls := sharedDefinitionsProviderCallCount(t); providerCalls != providerCallsBefore {
		t.Fatalf(
			"provider command runner calls = %d, want unchanged %d before validation succeeds",
			providerCalls,
			providerCallsBefore,
		)
	}
}

// TestFactoryValidationReportsAllActionableDefinitionErrors proves public
// Factory validation reports every independent actionable defect in one CLI
// outcome instead of stopping after the first error.
func TestFactoryValidationReportsAllActionableDefinitionErrors(t *testing.T) {
	providerCallsBefore := sharedDefinitionsProviderCallCount(t)

	dir := support.ScaffoldFactory(t, multipleActionableDefectsFactoryConfig())
	assertFactoryValidationRejects(
		t,
		dir,
		"Blocking targets:",
		validationCodeDuplicateIdentifier,
		validationCodeDanglingWorkerReference,
		validationCodeDanglingPlaceReference,
		`worker-a`,
		`missing-worker`,
		`missing-state`,
		`duplicate worker name "worker-a"`,
		`references non-existent worker "missing-worker"`,
		`references non-existent state "missing-state"`,
	)

	if providerCalls := sharedDefinitionsProviderCallCount(t); providerCalls != providerCallsBefore {
		t.Fatalf(
			"provider command runner calls = %d, want unchanged %d before validation fails",
			providerCalls,
			providerCallsBefore,
		)
	}
}

// TestAPIValidateFactoryAcceptsValidAndRejectsInvalidDefinitions proves the public
// Validate API accepts structurally valid Factory definitions with an empty target
// list and returns actionable validation targets for invalid definitions without
// persisting or activating runtime work.
func TestAPIValidateFactoryAcceptsValidAndRejectsInvalidDefinitions(t *testing.T) {
	runner := support.NewRecordingCommandRunner("runtime must not execute")
	edges := serviceedges.Edges{ProviderCommandRunner: runner}

	hostDir := support.ScaffoldFactory(t, validAPIValidationFactoryConfig())
	var validFactory factoryapi.Factory
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                hostDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
		BeforeStart: func(tb testing.TB, process support.Process, inputs root.Input) {
			var err error
			validFactory, err = support.LoadedFactoryWithProcessAndEnv(
				tb,
				process,
				inputs.Env,
				filepath.Join(hostDir, "factory.json"),
			)
			if err != nil {
				tb.Fatalf("load valid factory definition: %v", err)
			}
		},
	})
	defer server.Stop(t)

	currentBefore := getDefaultSessionFactory(t, server.URL())
	sessionsBefore := support.GetJSON[factoryapi.ListFactorySessionsResponse](
		t,
		server.URL()+"/factory-sessions",
	)

	validResult, validStatus := postValidateFactory(t, server.URL(), validFactory)
	if validStatus != http.StatusOK {
		t.Fatalf("POST /factory-validations valid status = %d, want 200", validStatus)
	}
	if len(validResult.Targets) != 0 {
		t.Fatalf("valid factory validation targets = %#v, want empty acceptance outcome", validResult.Targets)
	}

	invalidFactory, err := factoryDefinitionFromConfig(multipleActionableDefectsFactoryConfig())
	if err != nil {
		t.Fatalf("marshal invalid factory definition: %v", err)
	}
	invalidResult, invalidStatus := postValidateFactory(t, server.URL(), invalidFactory)
	if invalidStatus != http.StatusOK {
		t.Fatalf("POST /factory-validations invalid status = %d, want 200 with validation targets", invalidStatus)
	}
	if len(invalidResult.Targets) < 3 {
		t.Fatalf("invalid factory validation targets = %d, want multiple actionable defects", len(invalidResult.Targets))
	}
	for _, code := range []string{
		validationCodeDuplicateIdentifier,
		validationCodeDanglingWorkerReference,
		validationCodeDanglingPlaceReference,
	} {
		if !hasValidationTargetCode(invalidResult.Targets, code) {
			t.Fatalf("invalid factory validation targets = %#v, want code %q", invalidResult.Targets, code)
		}
	}

	currentAfter := getDefaultSessionFactory(t, server.URL())
	if currentAfter.Name != currentBefore.Name {
		t.Fatalf(
			"current factory name after validate = %q, want unchanged %q",
			currentAfter.Name,
			currentBefore.Name,
		)
	}
	sessionsAfter := support.GetJSON[factoryapi.ListFactorySessionsResponse](
		t,
		server.URL()+"/factory-sessions",
	)
	if len(sessionsAfter.Sessions) != len(sessionsBefore.Sessions) {
		t.Fatalf(
			"factory sessions after validate = %d, want unchanged count %d",
			len(sessionsAfter.Sessions),
			len(sessionsBefore.Sessions),
		)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 during validate-only API calls", runner.CallCount())
	}
}

// TestAPIPreviewFactoryReturnsPublicTopology proves the public Preview API returns the
// customer-facing preview projection for a valid orchestrator source: resolved workflow
// reference, effective policy bounds, and result constraints without Petri-net vocabulary.
func TestAPIPreviewFactoryReturnsPublicTopology(t *testing.T) {
	runner := support.NewRecordingCommandRunner("runtime must not execute")
	edges := serviceedges.Edges{ProviderCommandRunner: runner}

	hostDir := support.ScaffoldFactory(t, previewTopologyFactoryConfig())
	writeDefinitionsPreviewWorkflow(t, hostDir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                hostDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	defer server.Stop(t)

	previewResult, previewStatus := postPreviewFactory(t, server.URL(), hostDir, definitionsPreviewWorkflowName)
	if previewStatus != http.StatusOK {
		t.Fatalf("POST /factories/preview status = %d, want 200", previewStatus)
	}
	assertPublicPreviewProjection(t, previewResult, definitionsPreviewWorkflowName)

	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 during preview inspection", runner.CallCount())
	}
}

// TestAPIPreviewDoesNotStartWorkersOrSessions proves the public Preview API inspects
// Factory definitions without starting workers, dispatch activation, or new Factory
// Sessions beyond the already-running service session.
func TestAPIPreviewDoesNotStartWorkersOrSessions(t *testing.T) {
	runner := support.NewRecordingCommandRunner("runtime must not execute")
	edges := serviceedges.Edges{ProviderCommandRunner: runner}

	hostDir := support.ScaffoldFactory(t, previewTopologyFactoryConfig())
	writeDefinitionsPreviewWorkflow(t, hostDir)
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                hostDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	defer server.Stop(t)

	sessionsBefore := support.GetJSON[factoryapi.ListFactorySessionsResponse](
		t,
		server.URL()+"/factory-sessions",
	)
	statusBefore := support.GetJSON[factoryapi.StatusResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/~default/status",
	)

	previewResult, previewStatus := postPreviewFactory(t, server.URL(), hostDir, definitionsPreviewWorkflowName)
	if previewStatus != http.StatusOK {
		t.Fatalf("POST /factories/preview status = %d, want 200", previewStatus)
	}
	if !previewResult.Valid {
		t.Fatalf("preview result = %#v, want valid preview acceptance", previewResult)
	}

	sessionsAfter := support.GetJSON[factoryapi.ListFactorySessionsResponse](
		t,
		server.URL()+"/factory-sessions",
	)
	if len(sessionsAfter.Sessions) != len(sessionsBefore.Sessions) {
		t.Fatalf(
			"factory sessions after preview = %d, want unchanged count %d",
			len(sessionsAfter.Sessions),
			len(sessionsBefore.Sessions),
		)
	}
	statusAfter := support.GetJSON[factoryapi.StatusResponse](
		t,
		strings.TrimSuffix(server.URL(), "/")+"/factory-sessions/~default/status",
	)
	if statusAfter.TotalTokens != statusBefore.TotalTokens {
		t.Fatalf(
			"default session total tokens after preview = %d, want unchanged %d",
			statusAfter.TotalTokens,
			statusBefore.TotalTokens,
		)
	}
	if statusAfter.RuntimeStatus != statusBefore.RuntimeStatus {
		t.Fatalf(
			"default session runtime status after preview = %q, want unchanged %q",
			statusAfter.RuntimeStatus,
			statusBefore.RuntimeStatus,
		)
	}
	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 during preview-only inspection", runner.CallCount())
	}
}

func missingWorkerFactoryConfig() map[string]any {
	return map[string]any{
		"name": "missing-worker-reference",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "real-worker"},
		},
		"workstations": []map[string]any{
			{
				"name":   "processor",
				"worker": "ghost-worker",
				"inputs": []map[string]string{
					{"workType": "task", "state": "init"},
				},
				"outputs": []map[string]string{
					{"workType": "task", "state": "complete"},
				},
			},
		},
	}
}

func missingWorkstationFactoryConfig() map[string]any {
	return map[string]any{
		"name": "missing-workstation-reference",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":   "process",
				"worker": "worker-a",
				"inputs": []map[string]string{
					{"workType": "task", "state": "init"},
				},
				"outputs": []map[string]string{
					{"workType": "task", "state": "complete"},
				},
				"onFailure": []map[string]string{
					{"workType": "task", "state": "failed"},
				},
			},
		},
		"layout": map[string]any{
			"schemaVersion": 1,
			"nodes": []map[string]any{
				{
					"id":       "workstation:ghost-workstation",
					"position": map[string]int{"x": 1, "y": 2},
				},
			},
		},
	}
}

func multipleActionableDefectsFactoryConfig() map[string]any {
	return map[string]any{
		"name": "multiple-actionable-defects",
		"workTypes": []map[string]any{
			{
				"name": "story",
				"states": []map[string]string{
					{"name": "queued", "type": "INITIAL"},
					{"name": "queued-dup", "type": "PROCESSING"},
					{"name": "done", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":   "process",
				"worker": "missing-worker",
				"inputs": []map[string]string{
					{"workType": "story", "state": "queued"},
				},
				"outputs": []map[string]string{
					{"workType": "story", "state": "missing-state"},
				},
				"onFailure": []map[string]string{
					{"workType": "story", "state": "failed"},
				},
			},
		},
	}
}

func missingRouteFactoryConfig() map[string]any {
	return map[string]any{
		"name": "missing-route-reference",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":   "process",
				"worker": "worker-a",
				"inputs": []map[string]string{
					{"workType": "task", "state": "init"},
				},
				"outputs": []map[string]string{
					{"workType": "task", "state": "missing-state"},
				},
				"onFailure": []map[string]string{
					{"workType": "task", "state": "failed"},
				},
			},
		},
	}
}

const (
	definitionsPreviewWorkflowName = "definitions-preview"
	definitionsPreviewWorkflowDir  = ".claude/workflows"
)

func previewTopologyFactoryConfig() map[string]any {
	cfg := validAPIValidationFactoryConfig()
	cfg["name"] = "api-preview-topology-host"
	return cfg
}

func validAPIValidationFactoryConfig() map[string]any {
	return map[string]any{
		"name": "api-validation-host",
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]string{
			{"name": "worker-a"},
		},
		"workstations": []map[string]any{
			{
				"name":   "process",
				"worker": "worker-a",
				"inputs": []map[string]string{
					{"workType": "task", "state": "init"},
				},
				"outputs": []map[string]string{
					{"workType": "task", "state": "complete"},
				},
				"onFailure": []map[string]string{
					{"workType": "task", "state": "failed"},
				},
			},
		},
	}
}

func factoryDefinitionFromConfig(cfg map[string]any) (factoryapi.Factory, error) {
	payload, err := json.Marshal(cfg)
	if err != nil {
		return factoryapi.Factory{}, err
	}
	return support.DecodeFactoryDefinition(payload)
}

const definitionsPreviewWorkflowSource = `
meta({ name: "definitions-preview", version: 1 });
phase("setup");
log("definitions preview topology fixture");
workflow.log("step");
workflow.artifact({ kind: "log", label: "step" });
const result = await agent.run({ prompt: "preview topology" });
workflow.final({ ok: true, result });
pipeline([], function () {}, function () {});
`

func writeDefinitionsPreviewWorkflow(t *testing.T, projectRoot string) {
	t.Helper()

	workflowDir := filepath.Join(projectRoot, definitionsPreviewWorkflowDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir preview workflow dir: %v", err)
	}
	workflowPath := filepath.Join(workflowDir, definitionsPreviewWorkflowName+".js")
	if err := os.WriteFile(workflowPath, []byte(definitionsPreviewWorkflowSource), 0o600); err != nil {
		t.Fatalf("write preview workflow %s: %v", workflowPath, err)
	}
}

func postPreviewFactory(
	t *testing.T,
	serverURL string,
	projectRoot string,
	workflowName string,
) (factoryapi.FactoryPreviewResult, int) {
	t.Helper()

	projectRoot = strings.TrimSpace(projectRoot)
	workflowName = strings.TrimSpace(workflowName)
	payload, err := json.Marshal(factoryapi.FactoryPreviewRequest{
		SourceKind:  factoryapi.WORKFLOWNAME,
		ProjectRoot: &projectRoot,
		SourceValue: &workflowName,
	})
	if err != nil {
		t.Fatalf("marshal preview factory request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factories/preview"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read POST %s response: %v", endpoint, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST %s status = %d body = %s, want 200", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.FactoryPreviewResult
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode POST %s response: %v: %s", endpoint, err, strings.TrimSpace(string(body)))
	}
	return result, response.StatusCode
}

func assertPublicPreviewProjection(
	t *testing.T,
	preview factoryapi.FactoryPreviewResult,
	workflowName string,
) {
	t.Helper()

	if !preview.Valid {
		t.Fatalf("preview result = %#v, want valid preview acceptance", preview)
	}
	if len(preview.SourceValidationIssues) != 0 {
		t.Fatalf("preview source validation issues = %#v, want empty for valid source", preview.SourceValidationIssues)
	}

	resolution := preview.SourceResolution
	if !resolution.Found {
		t.Fatalf("preview source resolution = %#v, want resolved source", resolution)
	}
	if resolution.RequestKind != string(factoryapi.WORKFLOWNAME) {
		t.Fatalf("preview request kind = %q, want WORKFLOW_NAME", resolution.RequestKind)
	}
	if resolution.RequestValue == nil || strings.TrimSpace(*resolution.RequestValue) != workflowName {
		t.Fatalf("preview request value = %v, want %q", resolution.RequestValue, workflowName)
	}
	if resolution.ResolvedKind == nil || strings.TrimSpace(*resolution.ResolvedKind) != string(factoryapi.WORKFLOWFILE) {
		t.Fatalf("preview resolved kind = %v, want WORKFLOW_FILE for resolved workflow file source", resolution.ResolvedKind)
	}
	wantSourceRef := definitionsPreviewWorkflowDir + "/" + workflowName + ".js"
	if resolution.SourceRef == nil || strings.TrimSpace(*resolution.SourceRef) != wantSourceRef {
		t.Fatalf("preview source ref = %v, want %q", resolution.SourceRef, wantSourceRef)
	}
	if resolution.SourceHash == nil || strings.TrimSpace(*resolution.SourceHash) == "" {
		t.Fatalf("preview source resolution = %#v, want non-empty source hash", resolution)
	}
	if resolution.OrchestratorKind == nil || strings.TrimSpace(*resolution.OrchestratorKind) == "" {
		t.Fatalf("preview orchestrator kind = %v, want non-empty public orchestrator label", resolution.OrchestratorKind)
	}

	policy := preview.PolicyPreview
	if strings.TrimSpace(policy.PolicyHash) == "" {
		t.Fatalf("preview policy hash = %q, want non-empty hash", policy.PolicyHash)
	}
	if policy.MaxChildCount <= 0 {
		t.Fatalf("preview max child count = %d, want positive bound", policy.MaxChildCount)
	}
	if policy.MaxConcurrency <= 0 {
		t.Fatalf("preview max concurrency = %d, want positive bound", policy.MaxConcurrency)
	}
	if len(policy.EffectivePolicy) == 0 {
		t.Fatalf("preview effective policy = %#v, want customer-visible policy projection", policy.EffectivePolicy)
	}
	if len(policy.ValidationIssues) != 0 {
		t.Fatalf("preview policy validation issues = %#v, want empty for valid source", policy.ValidationIssues)
	}

	constraints := preview.ResultConstraints
	if constraints.ArtifactUriScheme != "you-artifact" {
		t.Fatalf("preview artifact uri scheme = %q, want you-artifact", constraints.ArtifactUriScheme)
	}
	if !constraints.RequiresStructuredCloneableJson {
		t.Fatalf("preview requires structured cloneable json = false, want true")
	}
	if constraints.MaxEmbeddedBytes <= 0 {
		t.Fatalf("preview max embedded bytes = %d, want positive bound", constraints.MaxEmbeddedBytes)
	}
	if len(constraints.RejectedValueKinds) == 0 {
		t.Fatalf("preview rejected value kinds = %#v, want public rejection vocabulary", constraints.RejectedValueKinds)
	}
}

func postValidateFactory(
	t *testing.T,
	serverURL string,
	factory factoryapi.Factory,
) (factoryapi.FactoryValidationResult, int) {
	t.Helper()

	payload, err := json.Marshal(factory)
	if err != nil {
		t.Fatalf("marshal validate factory request: %v", err)
	}
	endpoint := strings.TrimSuffix(serverURL, "/") + "/factory-validations"
	response, err := http.Post(endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("POST %s: %v", endpoint, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read POST %s response: %v", endpoint, err)
	}
	if response.StatusCode != http.StatusOK {
		t.Fatalf("POST %s status = %d body = %s, want 200", endpoint, response.StatusCode, strings.TrimSpace(string(body)))
	}
	var result factoryapi.FactoryValidationResult
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode POST %s response: %v: %s", endpoint, err, strings.TrimSpace(string(body)))
	}
	return result, response.StatusCode
}

func getDefaultSessionFactory(t *testing.T, serverURL string) factoryapi.Factory {
	t.Helper()
	return support.GetJSON[factoryapi.Factory](
		t,
		strings.TrimSuffix(serverURL, "/")+"/factory-sessions/~default/factory",
	)
}

func hasValidationTargetCode(targets []factoryapi.FactoryValidationTarget, code string) bool {
	for _, target := range targets {
		if target.Code == code {
			return true
		}
	}
	return false
}

func assertFactoryValidationAccepts(
	t *testing.T,
	factoryDir string,
) {
	t.Helper()
	process := buildDefinitionsProcess(t)
	assertFactoryValidationAcceptsWithProcess(t, process, factoryDir)
}

func assertFactoryValidationAcceptsWithProcess(
	t *testing.T,
	process support.Process,
	factoryDir string,
) {
	t.Helper()

	factoryPath := filepath.Join(factoryDir, "factory.json")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "factory", "config", "validate", factoryPath,
	})
	inputs.Input.Env = isolatedHomeEnvironment(t)
	inputs.Input.WorkingDirectory = factoryDir

	err := process.Execute(inputs.Input)
	if err != nil {
		t.Fatalf(
			"Process.Execute(factory config validate) error = %v, want validation success; stdout=%q stderr=%q",
			err,
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	diagnostic := inputs.Stdout() + "\n" + inputs.Stderr()
	if !strings.Contains(diagnostic, "Factory validation passed.") {
		t.Fatalf("diagnostic missing validation success marker:\n%s", diagnostic)
	}
}

func assertFactoryValidationRejects(
	t *testing.T,
	factoryDir string,
	wants ...string,
) {
	t.Helper()
	process := buildDefinitionsProcess(t)
	assertFactoryValidationRejectsWithProcess(t, process, factoryDir, wants...)
}

func assertFactoryValidationRejectsWithProcess(
	t *testing.T,
	process support.Process,
	factoryDir string,
	wants ...string,
) {
	t.Helper()

	factoryPath := filepath.Join(factoryDir, "factory.json")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "factory", "config", "validate", factoryPath,
	})
	inputs.Input.Env = isolatedHomeEnvironment(t)
	inputs.Input.WorkingDirectory = factoryDir

	err := process.Execute(inputs.Input)
	if err == nil {
		t.Fatalf(
			"Process.Execute(factory config validate) error = nil, want validation failure; stdout=%q stderr=%q",
			inputs.Stdout(),
			inputs.Stderr(),
		)
	}

	diagnostic := err.Error() + "\n" + inputs.Stdout() + "\n" + inputs.Stderr()
	if !strings.Contains(diagnostic, "Factory validation failed.") {
		t.Fatalf("diagnostic missing validation failure marker:\n%s", diagnostic)
	}
	for _, want := range wants {
		if !strings.Contains(diagnostic, want) {
			t.Fatalf("diagnostic %q does not contain %q", diagnostic, want)
		}
	}
}
