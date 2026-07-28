package promptassets_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages/promptassets"
	factoryvalidation "github.com/portpowered/infinite-you/pkg/services/factory_definitions/validation"
	workerconfig "github.com/portpowered/infinite-you/pkg/services/factory_definitions/workers"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

const (
	materializedWorkerPrompt      = "\n  worker prompt with preserved whitespace  \n"
	materializedWorkstationPrompt = "  workstation prompt without final newline"
)

func TestAssembledPrompts_PreserveExactContentAcrossMaterializationAndReload(t *testing.T) {
	assembled := assembleMaterializationFixture(t)
	source := loadAssembledFixture(t, assembled)

	firstDir := persistAssembledFixture(t, assembled)
	secondDir := persistAssembledFixture(t, assembled)

	assertMaterializedPromptFiles(t, firstDir)
	assertMaterializedPromptFiles(t, secondDir)
	assertThinManifestOmitsPromptBodies(t, firstDir)

	first := loadMaterializedFixture(t, firstDir)
	second := loadMaterializedFixture(t, secondDir)
	assertLoadedPrompts(t, first)
	assertLoadedPrompts(t, second)
	if !reflect.DeepEqual(first.FactoryConfig(), source.FactoryConfig()) {
		t.Fatal("first materialized runtime config differs from assembled source")
	}
	if !reflect.DeepEqual(second.FactoryConfig(), source.FactoryConfig()) {
		t.Fatal("second materialized runtime config differs from assembled source")
	}
	if !reflect.DeepEqual(first.FactoryConfig(), second.FactoryConfig()) {
		t.Fatal("two fresh materializations produced different runtime configs")
	}
}

func assembleMaterializationFixture(t *testing.T) []byte {
	t.Helper()
	payload := []byte(`{
  "name": "prompt-fixture",
  "id": "prompt-fixture",
  "workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
  "workers": [{"name":"author","type":"MODEL_WORKER","promptFile":"prompts/worker.md"}],
  "workstations": [{"name":"draft","worker":"author","inputs":[{"workType":"task","state":"init"}],"outputs":[{"workType":"task","state":"complete"}],"onFailure":[{"workType":"task","state":"failed"}],"type":"MODEL_WORKSTATION","promptFile":"prompts/workstation.md"}]
}`)
	assembled, err := promptassets.Assemble(promptassets.Definition{
		Package:     "@you/prompt-fixture",
		FactoryJSON: payload,
		Assets: fstest.MapFS{
			"assets/prompts/worker.md":      {Data: []byte(materializedWorkerPrompt)},
			"assets/prompts/workstation.md": {Data: []byte(materializedWorkstationPrompt)},
		},
		AssetRoot: "assets",
	})
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	return assembled
}

type loadedPromptFixture interface {
	FactoryConfig() *factorydefinitions.FactoryConfig
	Worker(string) (*workerconfig.Config, bool)
	Workstation(string) (*factorydefinitions.FactoryWorkstationConfig, bool)
}

func loadAssembledFixture(t *testing.T, assembled []byte) loadedPromptFixture {
	t.Helper()
	loaded, err := factorydefinitioncomposition.LoadCanonicalJSON(assembled, nil)
	if err != nil {
		t.Fatalf("LoadFromCanonicalJSON: %v", err)
	}
	return loaded
}

func persistAssembledFixture(t *testing.T, assembled []byte) string {
	t.Helper()
	factoryDir, err := factorydefinitioncomposition.PersistNamedFactory(
		t.TempDir(),
		"prompt-fixture",
		assembled,
		factoryvalidation.New(nil, factorydefinitioncomposition.LoadCanonicalJSON),
	)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	return factoryDir
}

func loadMaterializedFixture(t *testing.T, factoryDir string) loadedPromptFixture {
	t.Helper()
	loaded, err := factorydefinitioncomposition.LoadDirectory(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadFromFactoryDir: %v", err)
	}
	return loaded
}

func assertMaterializedPromptFiles(t *testing.T, factoryDir string) {
	t.Helper()
	assertFileContent(t, filepath.Join(factoryDir, factorydefinitions.WorkersDir, "author", factorydefinitions.FactoryAgentsFileName), materializedWorkerPrompt)
	if _, err := os.Stat(filepath.Join(factoryDir, factorydefinitions.WorkstationsDir, "draft", factorydefinitions.FactoryAgentsFileName)); err != nil {
		t.Fatalf("expected editable workstation AGENTS.md: %v", err)
	}
	assertFileContent(t, filepath.Join(factoryDir, factorydefinitions.WorkstationsDir, "draft", "prompts", "workstation.md"), materializedWorkstationPrompt)
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(content) != want {
		t.Fatalf("file %s = %q, want exact authored content %q", path, content, want)
	}
}

func assertThinManifestOmitsPromptBodies(t *testing.T, factoryDir string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(factoryDir, factorydefinitions.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	for _, collection := range []string{"workers", "workstations"} {
		entry := root[collection].([]any)[0].(map[string]any)
		for _, field := range []string{"body", "promptTemplate"} {
			if _, exists := entry[field]; exists {
				t.Fatalf("persisted %s entry duplicated prompt in %s: %#v", collection, field, entry)
			}
		}
	}
}

func assertLoadedPrompts(t *testing.T, loaded loadedPromptFixture) {
	t.Helper()
	worker, ok := loaded.Worker("author")
	if !ok || worker.Body != materializedWorkerPrompt {
		t.Fatalf("loaded worker body = %#v, want exact authored content %q", worker, materializedWorkerPrompt)
	}
	workstation, ok := loaded.Workstation("draft")
	if !ok || workstation.PromptTemplate != materializedWorkstationPrompt {
		t.Fatalf("loaded workstation = %#v, want exact prompt %q", workstation, materializedWorkstationPrompt)
	}
}
