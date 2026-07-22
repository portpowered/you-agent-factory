package factory_transformation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
)

// Factory validation codes are part of the generated HTTP response contract.
// Functional tests assert the serialized customer-facing values instead of
// importing the validation implementation that produces them.
const (
	factoryValidationCodeDanglingPlaceReference         = "factory.route.danglingPlaceReference"
	factoryValidationCodeDanglingWorkerReference        = "factory.worker.danglingReference"
	factoryValidationCodeDuplicateIdentifier            = "factory.duplicateIdentifier"
	factoryValidationCodeLayoutUnknownNodeReference     = "factory.layout.unknownNodeReference"
	factoryValidationCodeWorkstationMissingFailureRoute = "factory.workstation.missingFailureRoute"
)

func assertFunctionalSplitLayoutAtRoot(t *testing.T, rootDir, project string) {
	t.Helper()

	factoryJSON, err := os.ReadFile(filepath.Join(rootDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	if strings.Contains(string(factoryJSON), "You are the planner.") {
		t.Fatalf("factory.json should omit inlined planner body after split save, got %s", factoryJSON)
	}

	agentsPath := filepath.Join(rootDir, interfaces.WorkersDir, "planner", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(agentsPath); err != nil {
		t.Fatalf("expected planner AGENTS.md at %s: %v", agentsPath, err)
	}

	workstationPath := filepath.Join(rootDir, interfaces.WorkstationsDir, "plan-task", interfaces.FactoryAgentsFileName)
	if _, err := os.Stat(workstationPath); err != nil {
		t.Fatalf("expected plan-task AGENTS.md at %s: %v", workstationPath, err)
	}

	loaded, err := support.LoadedCurrentFactory(t, rootDir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	if loaded.Id == nil || *loaded.Id != project {
		t.Fatalf("factory id = %v, want %q", loaded.Id, project)
	}
}
