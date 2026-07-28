package definitions

import (
	"path/filepath"
	"strings"
	"testing"

	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
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
