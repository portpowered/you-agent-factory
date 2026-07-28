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

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

const (
	validationCodeDuplicateIdentifier        = "factory.duplicateIdentifier"
	validationCodeDanglingWorkerReference    = "factory.worker.danglingReference"
	validationCodeDanglingPlaceReference     = "factory.route.danglingPlaceReference"
	validationCodeLayoutUnknownNodeReference = "factory.layout.unknownNodeReference"
)

// TestFactoryValidationRejectsMissingWorkerWorkstationAndRoute proves public
// Factory validation rejects authored definitions that reference missing workers,
// workstations, or routes before runtime execution and reports customer-visible
// diagnostics through the CLI validate path.
func TestFactoryValidationRejectsMissingWorkerWorkstationAndRoute(t *testing.T) {
	runner := support.NewRecordingCommandRunner("runtime must not execute")
	edges := serviceedges.Edges{ProviderCommandRunner: runner}

	t.Run("missing_worker", func(t *testing.T) {
		dir := support.ScaffoldFactory(t, missingWorkerFactoryConfig())
		assertFactoryValidationRejects(
			t,
			dir,
			edges,
			validationCodeDanglingWorkerReference,
			`ghost-worker`,
			`workstation "processor" references non-existent worker "ghost-worker"`,
		)
	})

	t.Run("missing_workstation", func(t *testing.T) {
		dir := support.ScaffoldFactory(t, missingWorkstationFactoryConfig())
		assertFactoryValidationRejects(
			t,
			dir,
			edges,
			validationCodeLayoutUnknownNodeReference,
			`workstation:ghost-workstation`,
			`layout node "workstation:ghost-workstation" does not match any pending graph node`,
		)
	})

	t.Run("missing_route", func(t *testing.T) {
		dir := support.ScaffoldFactory(t, missingRouteFactoryConfig())
		assertFactoryValidationRejects(
			t,
			dir,
			edges,
			validationCodeDanglingPlaceReference,
			`missing-state`,
			`references non-existent state "missing-state" of work type "task"`,
			`ROUTE(process->task:missing-state)`,
		)
	})

	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 before validation succeeds", runner.CallCount())
	}
}

// TestFactoryValidationReportsAllActionableDefinitionErrors proves public
// Factory validation reports every independent actionable defect in one CLI
// outcome instead of stopping after the first error.
func TestFactoryValidationReportsAllActionableDefinitionErrors(t *testing.T) {
	runner := support.NewRecordingCommandRunner("runtime must not execute")
	edges := serviceedges.Edges{ProviderCommandRunner: runner}

	dir := support.ScaffoldFactory(t, multipleActionableDefectsFactoryConfig())
	assertFactoryValidationRejects(
		t,
		dir,
		edges,
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

	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 before validation fails", runner.CallCount())
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
	server := support.StartFunctionalAPIServer(t, support.FunctionalAPIServerConfig{
		FactoryDir:                hostDir,
		UseMockWorkers:            true,
		WaitForServiceModeRuntime: true,
		Edges:                     edges,
	})
	defer server.Stop(t)

	currentBefore := getDefaultSessionFactory(t, server.URL())
	sessionsBefore := support.GetJSON[factoryapi.ListFactorySessionsResponse](
		t,
		server.URL()+"/factory-sessions",
	)

	validFactory, err := support.LoadedFactory(t, filepath.Join(hostDir, "factory.json"))
	if err != nil {
		t.Fatalf("load valid factory definition: %v", err)
	}
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

// TestAPIPreviewFactoryReturnsPublicTopology proves the public Preview API accepts a
// valid Factory source and that the running session exposes customer-visible work
// types, workers, workstations, and route wiring without Petri-net vocabulary.
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
	if !previewResult.Valid {
		t.Fatalf("preview result = %#v, want valid preview acceptance", previewResult)
	}
	if previewResult.SourceResolution.Found != true {
		t.Fatalf("preview source resolution = %#v, want resolved source", previewResult.SourceResolution)
	}
	if previewResult.SourceResolution.SourceHash == nil || strings.TrimSpace(*previewResult.SourceResolution.SourceHash) == "" {
		t.Fatalf("preview source resolution = %#v, want non-empty source hash", previewResult.SourceResolution)
	}

	currentFactory := getDefaultSessionFactory(t, server.URL())
	assertPublicFactoryTopology(t, currentFactory)

	if runner.CallCount() != 0 {
		t.Fatalf("provider command runner calls = %d, want 0 during preview inspection", runner.CallCount())
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

func assertPublicFactoryTopology(t *testing.T, factory factoryapi.Factory) {
	t.Helper()

	if factory.WorkTypes == nil || len(*factory.WorkTypes) != 1 {
		t.Fatalf("factory work types = %#v, want one public work type", factory.WorkTypes)
	}
	workType := (*factory.WorkTypes)[0]
	if workType.Name != "task" {
		t.Fatalf("factory work type name = %q, want task", workType.Name)
	}
	if len(workType.States) < 2 {
		t.Fatalf("factory work type states = %#v, want init and completion states", workType.States)
	}

	if factory.Workers == nil || len(*factory.Workers) != 1 {
		t.Fatalf("factory workers = %#v, want one worker", factory.Workers)
	}
	if (*factory.Workers)[0].Name != "worker-a" {
		t.Fatalf("factory worker name = %q, want worker-a", (*factory.Workers)[0].Name)
	}

	if factory.Workstations == nil || len(*factory.Workstations) != 1 {
		t.Fatalf("factory workstations = %#v, want one workstation", factory.Workstations)
	}
	workstation := (*factory.Workstations)[0]
	if workstation.Name != "process" {
		t.Fatalf("factory workstation name = %q, want process", workstation.Name)
	}
	if workstation.Worker != "worker-a" {
		t.Fatalf("factory workstation worker = %q, want worker-a", workstation.Worker)
	}
	if len(workstation.Inputs) != 1 {
		t.Fatalf("factory workstation inputs = %#v, want one input route", workstation.Inputs)
	}
	input := workstation.Inputs[0]
	if input.WorkType != "task" || input.State != "init" {
		t.Fatalf("factory workstation input route = %#v, want task/init", input)
	}
	if workstation.Outputs == nil || len(*workstation.Outputs) != 1 {
		t.Fatalf("factory workstation outputs = %#v, want one output route", workstation.Outputs)
	}
	output := (*workstation.Outputs)[0]
	if output.WorkType != "task" || output.State != "complete" {
		t.Fatalf("factory workstation output route = %#v, want task/complete", output)
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

func assertFactoryValidationRejects(
	t *testing.T,
	factoryDir string,
	edges serviceedges.Edges,
	wants ...string,
) {
	t.Helper()

	factoryPath := filepath.Join(factoryDir, "factory.json")
	inputs := support.FakeInputs(t.Context(), []string{
		"you", "factory", "config", "validate", factoryPath,
	})
	inputs.Input.WorkingDirectory = factoryDir

	err := support.BuildProcess(t, edges).Execute(inputs.Input)
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
