package subagent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
)

func TestEditedMaterializedPackagedSubagentFactoryChangesNextLoad(t *testing.T) {
	factoryDir := materializePackagedSubagentFactory(t, t.TempDir())
	initialWorker := loadPackagedSubagentWorker(t, factoryDir)
	if initialWorker.Body == "" {
		t.Fatal("expected initial materialized subagent worker body")
	}

	editedBody := "You are the customer-edited @you/subagent built-in.\n"
	editMaterializedWorkerBody(t, factoryDir, PackagedWorkerName, editedBody)

	editedWorker := loadPackagedSubagentWorker(t, factoryDir)
	if editedWorker.Body != editedBody {
		t.Fatalf("edited worker body = %q, want exact edited content %q", editedWorker.Body, editedBody)
	}
	if editedWorker.Body == initialWorker.Body {
		t.Fatalf("edited worker body = %q, want change from initial materialized content", editedWorker.Body)
	}
}

func TestEditedMaterializedPackagedSubagentFactoryWorkstationChangesNextLoad(t *testing.T) {
	factoryDir := materializePackagedSubagentFactory(t, t.TempDir())
	initialWorkstation := loadPackagedSubagentWorkstation(t, factoryDir)
	if initialWorkstation.Body == "" {
		t.Fatal("expected initial materialized subagent workstation body")
	}

	editedBody := "Respond to the customer-edited request for work {{ (index .Inputs 0).WorkID }}.\n"
	editMaterializedWorkstationBody(t, factoryDir, PackagedRunWorkstationName, editedBody)

	editedWorkstation := loadPackagedSubagentWorkstation(t, factoryDir)
	if editedWorkstation.Body != editedBody {
		t.Fatalf("edited workstation body = %q, want exact edited content %q", editedWorkstation.Body, editedBody)
	}
	if editedWorkstation.Body == initialWorkstation.Body {
		t.Fatalf("edited workstation body = %q, want change from initial materialized content", editedWorkstation.Body)
	}
}

func TestEditedMaterializedPackagedSubagentFactoryJSONChangesNextLoad(t *testing.T) {
	const editedProject = "customer-edited-subagent"
	factoryDir := materializePackagedSubagentFactory(t, t.TempDir())
	initialLoaded := loadPackagedSubagentRuntimeConfig(t, factoryDir)
	if initialLoaded.FactoryConfig().Project != PackagedFactoryProject {
		t.Fatalf("initial project = %q, want %s", initialLoaded.FactoryConfig().Project, PackagedFactoryProject)
	}

	editMaterializedFactoryProject(t, factoryDir, editedProject)

	editedLoaded := loadPackagedSubagentRuntimeConfig(t, factoryDir)
	if editedLoaded.FactoryConfig().Project != editedProject {
		t.Fatalf("edited project = %q, want %q", editedLoaded.FactoryConfig().Project, editedProject)
	}
	if editedLoaded.FactoryConfig().Project == initialLoaded.FactoryConfig().Project {
		t.Fatalf("edited project = %q, want change from initial materialized content", editedLoaded.FactoryConfig().Project)
	}
}

func loadPackagedSubagentRuntimeConfig(
	t *testing.T,
	factoryDir string,
) interfaces.MutableLoadedFactorySource {
	t.Helper()
	loaded, err := factorydefinitioncomposition.LoadDirectory(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir(%q): %v", factoryDir, err)
	}
	return loaded
}

func loadPackagedSubagentWorker(t *testing.T, factoryDir string) *workerconfig.Config {
	t.Helper()
	loaded := loadPackagedSubagentRuntimeConfig(t, factoryDir)
	worker, ok := loaded.Worker(PackagedWorkerName)
	if !ok {
		t.Fatal("expected materialized subagent-worker")
	}
	return worker
}

func loadPackagedSubagentWorkstation(t *testing.T, factoryDir string) *interfaces.FactoryWorkstationConfig {
	t.Helper()
	loaded := loadPackagedSubagentRuntimeConfig(t, factoryDir)
	workstation, ok := loaded.Workstation(PackagedRunWorkstationName)
	if !ok {
		t.Fatal("expected materialized run-subagent workstation")
	}
	return workstation
}

func editMaterializedWorkerBody(t *testing.T, factoryDir, workerName, body string) {
	t.Helper()
	workerPath := filepath.Join(factoryDir, interfaces.WorkersDir, workerName, interfaces.FactoryAgentsFileName)
	if err := os.WriteFile(workerPath, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(materialized worker body): %v", err)
	}
}

func editMaterializedWorkstationBody(t *testing.T, factoryDir, workstationName, body string) {
	t.Helper()
	workstationPath := filepath.Join(factoryDir, interfaces.WorkstationsDir, workstationName, interfaces.FactoryAgentsFileName)
	if err := os.WriteFile(workstationPath, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile(materialized workstation body): %v", err)
	}
}

func editMaterializedFactoryProject(t *testing.T, factoryDir, project string) {
	t.Helper()
	factoryJSONPath := filepath.Join(factoryDir, interfaces.FactoryConfigFile)
	factoryJSON, err := os.ReadFile(factoryJSONPath)
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var factoryDoc map[string]any
	if err := json.Unmarshal(factoryJSON, &factoryDoc); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	factoryDoc["id"] = project
	editedJSON, err := json.MarshalIndent(factoryDoc, "", "  ")
	if err != nil {
		t.Fatalf("Marshal(edited factory.json): %v", err)
	}
	if string(editedJSON) == string(factoryJSON) {
		t.Fatal("expected edited factory.json to differ from initial materialized content")
	}
	if err := os.WriteFile(factoryJSONPath, editedJSON, 0o644); err != nil {
		t.Fatalf("WriteFile(edited factory.json): %v", err)
	}
}
