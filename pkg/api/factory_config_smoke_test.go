package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"gopkg.in/yaml.v3"
)

func TestFactoryConfigSmoke_CanonicalBoundaryPreservesPublicContract(t *testing.T) {
	factorySchema := loadFactorySchemaForSmoke(t)

	canonicalDir := writeFactoryConfigSmokeDir(t, factoryConfigSmokeCanonicalJSON())
	canonicalLoaded := loadRuntimeConfigForSmoke(t, canonicalDir)
	assertLoadedFactoryRuntimeForSmoke(t, canonicalLoaded)
	canonicalFlattened := flattenFactoryConfigForSmoke(t, canonicalDir)
	assertFactorySchemaAcceptsJSON(t, factorySchema, canonicalFlattened)
	assertFactoryConfigJSONUsesCanonicalPublicKeys(t, canonicalFlattened)
	canonicalFlattenedFactory := decodeGeneratedFactoryForSmoke(t, canonicalFlattened)
	canonicalGenerated := generatedFactoryForSmoke(t, canonicalLoaded)
	canonicalGeneratedJSON := marshalJSONForSmoke(t, canonicalGenerated)
	assertFactorySchemaAcceptsJSON(t, factorySchema, canonicalGeneratedJSON)
	assertFactoryConfigJSONUsesCanonicalPublicKeys(t, canonicalGeneratedJSON)
	assertComparableFactoryContractsMatch(t, canonicalFlattenedFactory, canonicalGenerated)

}

func TestFactoryConfigSmoke_LegacyBoundaryAliasesAreRejected(t *testing.T) {
	legacyDir := writeFactoryConfigSmokeDir(t, factoryConfigSmokeLegacyJSON())

	_, err := config.LoadRuntimeConfig(legacyDir, nil)
	if err == nil {
		t.Fatal("expected legacy boundary aliases to be rejected")
	}
	if !strings.Contains(err.Error(), "decode factory generated-schema boundary") {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), "workers[0].provider is not supported; use executorProvider") {
		t.Fatalf("expected provider retirement guidance, got %v", err)
	}
}

func TestFactoryConfigSmoke_OpenAPIDescriptionsAndEnumContractsReachRuntimeBoundary(t *testing.T) {
	doc := loadOpenAPI3DocumentForSmoke(t)
	factorySchema := requireOpenAPI3ComponentSchema(t, doc, "Factory")

	assertFactoryConfigSmokeDescriptions(t, factorySchema)
	assertFactoryConfigSmokeEnumRefs(t)

	canonicalJSON := []byte(factoryConfigSmokeCanonicalJSON())
	assertFactorySchemaAcceptsJSON(t, factorySchema, canonicalJSON)

	generatedBoundary, err := config.GeneratedFactoryFromOpenAPIJSON(canonicalJSON)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromOpenAPIJSON(canonical payload): %v", err)
	}
	assertFactoryConfigSmokeGeneratedBoundary(t, generatedBoundary)

	canonicalDir := writeFactoryConfigSmokeDir(t, factoryConfigSmokeCanonicalJSON())
	canonicalLoaded := loadRuntimeConfigForSmoke(t, canonicalDir)
	assertLoadedFactoryRuntimeForSmoke(t, canonicalLoaded)

	testCases := []struct {
		name      string
		payload   string
		fieldPath string
		value     string
	}{
		{
			name: "rejects mis-cased worker model provider",
			payload: strings.Replace(
				factoryConfigSmokeCanonicalJSON(),
				`"type":"MODEL_WORKER",
    "executorProvider":"SCRIPT_WRAP",
    "modelProvider":"CLAUDE",`,
				`"type":"MODEL_WORKER",
    "executorProvider":"SCRIPT_WRAP",
    "modelProvider":"Claude",`,
				1,
			),
			fieldPath: "workers[0].modelProvider",
			value:     `unsupported value "Claude"`,
		},
		{
			name: "rejects undocumented workstation behavior",
			payload: strings.Replace(
				factoryConfigSmokeCanonicalJSON(),
				`"behavior":"CRON"`,
				`"behavior":"SCHEDULED"`,
				1,
			),
			fieldPath: "workstations[0].behavior",
			value:     `unsupported value "SCHEDULED"`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			invalidDir := writeFactoryConfigSmokeDir(t, tc.payload)
			_, err := config.LoadRuntimeConfig(invalidDir, nil)
			if err == nil {
				t.Fatal("expected invalid enum payload to fail before runtime scheduling")
			}
			if !strings.Contains(err.Error(), "decode factory generated-schema boundary") {
				t.Fatalf("expected generated boundary context, got %v", err)
			}
			if !strings.Contains(err.Error(), tc.fieldPath) {
				t.Fatalf("expected field path %q in error, got %v", tc.fieldPath, err)
			}
			if !strings.Contains(err.Error(), tc.value) {
				t.Fatalf("expected error containing %q, got %v", tc.value, err)
			}
		})
	}
}

func TestFactoryConfigSmoke_RepresentativeFactoryDirectoryPreservesPublicContract(t *testing.T) {
	factorySchema := loadFactorySchemaForSmoke(t)

	canonicalDir := testutil.CopyFixtureDir(t, factoryConfigSmokeFixtureDir(t, "service_simple"))

	canonicalLoaded := loadRuntimeConfigForSmoke(t, canonicalDir)
	assertRepresentativeLoadedFactoryRuntimeForSmoke(t, canonicalLoaded)
	canonicalFlattened := flattenFactoryConfigForSmoke(t, canonicalDir)
	assertFactorySchemaAcceptsJSON(t, factorySchema, canonicalFlattened)
	assertRepresentativeFactoryConfigJSONUsesCanonicalPublicKeys(t, canonicalFlattened)
	canonicalFlattenedFactory := decodeGeneratedFactoryForSmoke(t, canonicalFlattened)
	canonicalGenerated := generatedFactoryForSmoke(t, canonicalLoaded)
	canonicalGeneratedJSON := marshalJSONForSmoke(t, canonicalGenerated)
	assertFactorySchemaAcceptsJSON(t, factorySchema, canonicalGeneratedJSON)
	assertRepresentativeFactoryConfigJSONUsesCanonicalPublicKeys(t, canonicalGeneratedJSON)
	assertComparableFactoryContractsMatch(t, canonicalFlattenedFactory, canonicalGenerated)
}

func TestFactoryConfigSmoke_RepresentativeFactoryDirectoryRejectsLegacyCopy(t *testing.T) {
	legacyDir := testutil.CopyFixtureDir(t, factoryConfigSmokeFixtureDir(t, "service_simple"))
	rewriteRepresentativeSmokeFixtureToLegacyAliases(t, legacyDir)

	_, err := config.LoadRuntimeConfig(legacyDir, nil)
	if err == nil {
		t.Fatal("expected representative legacy copy to be rejected")
	}
	if !strings.Contains(err.Error(), "decode factory generated-schema boundary") {
		t.Fatalf("expected generated boundary context, got %v", err)
	}
	if !strings.Contains(err.Error(), `json: unknown field "work_types"`) {
		t.Fatalf("expected work_types rejection, got %v", err)
	}
}

func TestFactoryConfigSmoke_OpenAPIFactorySchemaRejectsLegacySnakeCasePublicFields(t *testing.T) {
	factorySchema := loadFactorySchemaForSmoke(t)

	var payload any
	if err := json.Unmarshal([]byte(factoryConfigSmokeLegacyJSON()), &payload); err != nil {
		t.Fatalf("unmarshal legacy factory payload: %v", err)
	}

	err := factorySchema.VisitJSON(payload)
	if err == nil {
		t.Fatal("expected raw legacy snake_case factory config to be rejected by the public OpenAPI Factory schema")
	}

	for _, fragment := range []string{"inputTypes", "workTypes", "executorProvider", "promptFile", "resources"} {
		if strings.Contains(err.Error(), fragment) {
			return
		}
	}

	t.Fatalf("expected schema rejection to reference missing canonical camelCase fields, got %v", err)
}

func loadFactorySchemaForSmoke(t *testing.T) *openapi3.Schema {
	t.Helper()

	doc := loadOpenAPI3DocumentForSmoke(t)
	schemaRef, ok := doc.Components.Schemas["Factory"]
	if !ok || schemaRef == nil || schemaRef.Value == nil {
		t.Fatal("components.schemas.Factory is missing")
	}
	return schemaRef.Value
}

func loadOpenAPI3DocumentForSmoke(t *testing.T) *openapi3.T {
	t.Helper()

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("load openapi contract: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("validate openapi contract: %v", err)
	}
	return doc
}

func factoryConfigSmokeFixtureDir(t *testing.T, name string) string {
	t.Helper()

	return filepath.Clean(filepath.Join("..", "..", "tests", "functional_test", "testdata", name))
}

func writeFactoryConfigSmokeDir(t *testing.T, content string) string {
	t.Helper()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), []byte(content), 0o644); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}
	return dir
}

func loadRuntimeConfigForSmoke(t *testing.T, dir string) *config.LoadedFactoryConfig {
	t.Helper()

	loaded, err := config.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(%s): %v", dir, err)
	}
	return loaded
}

func assertLoadedFactoryRuntimeForSmoke(t *testing.T, loaded *config.LoadedFactoryConfig) {
	t.Helper()

	worker, ok := loaded.Worker("executor")
	if !ok {
		t.Fatal("loaded runtime config is missing worker definition for executor")
	}
	if worker.ExecutorProvider != "script_wrap" || worker.ModelProvider != "claude" || worker.StopToken != "COMPLETE" {
		t.Fatalf("loaded worker runtime config = %#v", worker)
	}
	if len(worker.Resources) != 1 || worker.Resources[0].Name != "agent-slot" || worker.Resources[0].Capacity != 1 {
		t.Fatalf("loaded worker resources = %#v, want agent-slot capacity 1", worker.Resources)
	}

	workstation, ok := loaded.Workstation("execute-story")
	if !ok {
		t.Fatal("loaded runtime config is missing workstation definition for execute-story")
	}
	if workstation.Type != interfaces.WorkstationTypeModel || workstation.PromptTemplate != "Implement {{ .WorkID }}." {
		t.Fatalf("loaded workstation runtime config = %#v", workstation)
	}
	if workstation.Limits.MaxExecutionTime != "30m" {
		t.Fatalf("loaded workstation limits = %#v, want maxExecutionTime 30m", workstation.Limits)
	}
	if len(workstation.StopWords) != 1 || workstation.StopWords[0] != "DONE" {
		t.Fatalf("loaded workstation stopWords = %#v, want [DONE]", workstation.StopWords)
	}
	if len(workstation.Resources) != 1 || workstation.Resources[0].Name != "agent-slot" || workstation.Resources[0].Capacity != 2 {
		t.Fatalf("loaded workstation resources = %#v, want agent-slot capacity 2", workstation.Resources)
	}
	if workstation.Cron == nil || !workstation.Cron.TriggerAtStart || workstation.Cron.ExpiryWindow != "20s" {
		t.Fatalf("loaded workstation cron config = %#v", workstation.Cron)
	}
}

func assertRepresentativeLoadedFactoryRuntimeForSmoke(t *testing.T, loaded *config.LoadedFactoryConfig) {
	t.Helper()

	worker, ok := loaded.Worker("worker-a")
	if !ok {
		t.Fatal("loaded runtime config is missing worker definition for worker-a")
	}
	if worker.Type != interfaces.WorkerTypeModel || worker.Model != "test-model" || worker.StopToken != "COMPLETE" {
		t.Fatalf("loaded worker-a runtime config = %#v", worker)
	}

	workstation, ok := loaded.Workstation("step-one")
	if !ok {
		t.Fatal("loaded runtime config is missing workstation definition for step-one")
	}
	if workstation.WorkerTypeName != "worker-a" || workstation.Type != interfaces.WorkstationTypeModel {
		t.Fatalf("loaded step-one runtime config = %#v", workstation)
	}
	if workstation.PromptTemplate != "Do the work." {
		t.Fatalf("loaded step-one prompt template = %q, want split AGENTS.md body", workstation.PromptTemplate)
	}
}

func flattenFactoryConfigForSmoke(t *testing.T, dir string) []byte {
	t.Helper()

	flattened, err := config.FlattenFactoryConfig(dir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig(%s): %v", dir, err)
	}
	return flattened
}

func assertFactorySchemaAcceptsJSON(t *testing.T, schema *openapi3.Schema, data []byte) {
	t.Helper()

	var payload any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal factory payload: %v", err)
	}
	if err := schema.VisitJSON(payload); err != nil {
		t.Fatalf("factory payload should validate against the OpenAPI Factory schema: %v", err)
	}
}

func assertFactoryConfigJSONUsesCanonicalPublicKeys(t *testing.T, data []byte) {
	t.Helper()

	assertFactoryConfigJSONUsesRequiredAndForbiddenKeys(t, data, []string{
		`"inputTypes"`,
		`"workTypes"`,
		`"executorProvider"`,
		`"modelProvider"`,
		`"stopToken"`,
		`"skipPermissions"`,
		`"promptFile"`,
		`"promptTemplate"`,
		`"outputSchema"`,
		`"onRejection"`,
		`"onFailure"`,
		`"resources"`,
		`"stopWords"`,
		`"maxExecutionTime"`,
		`"workingDirectory"`,
		`"triggerAtStart"`,
		`"expiryWindow"`,
		`"workType"`,
		`"parentInput"`,
		`"spawnedBy"`,
		`"maxVisits"`,
	}, []string{
		`"input_types"`,
		`"work_types"`,
		`"provider"`,
		`"model_provider"`,
		`"session_id"`,
		`"stop_token"`,
		`"skip_permissions"`,
		`"runtimeType"`,
		`"runtime_type"`,
		`"prompt_file"`,
		`"prompt_template"`,
		`"output_schema"`,
		`"on_rejection"`,
		`"on_failure"`,
		`"resource_usage"`,
		`"resource-usage"`,
		`"runtimeStopWords"`,
		`"stop_words"`,
		`"kind"`,
		`"max_execution_time"`,
		`"working_directory"`,
		`"trigger_at_start"`,
		`"expiry_window"`,
		`"timeout"`,
		`"work_type"`,
		`"parent_input"`,
		`"spawned_by"`,
		`"exhaustionRules"`,
		`"watch_workstation"`,
		`"max_visits"`,
	})
}

func assertRepresentativeFactoryConfigJSONUsesCanonicalPublicKeys(t *testing.T, data []byte) {
	t.Helper()

	assertFactoryConfigJSONUsesRequiredAndForbiddenKeys(t, data, []string{
		`"workTypes"`,
		`"workType"`,
		`"onFailure"`,
		`"stopToken"`,
	}, []string{
		`"work_types"`,
		`"work_type"`,
		`"on_failure"`,
		`"kind"`,
		`"stop_token"`,
		`"prompt_template"`,
		`"runtime_type"`,
	})
}

func assertFactoryConfigJSONUsesRequiredAndForbiddenKeys(t *testing.T, data []byte, requiredKeys, forbiddenKeys []string) {
	t.Helper()

	text := string(data)
	for _, key := range requiredKeys {
		if !strings.Contains(text, key) {
			t.Fatalf("factory payload is missing canonical key %s: %s", key, text)
		}
	}
	for _, key := range forbiddenKeys {
		if strings.Contains(text, key) {
			t.Fatalf("factory payload must not advertise legacy key %s: %s", key, text)
		}
	}
}

func decodeGeneratedFactoryForSmoke(t *testing.T, data []byte) factoryapi.Factory {
	t.Helper()

	var factory factoryapi.Factory
	if err := json.Unmarshal(data, &factory); err != nil {
		t.Fatalf("unmarshal generated Factory payload: %v", err)
	}
	return factory
}

func generatedFactoryForSmoke(t *testing.T, loaded *config.LoadedFactoryConfig) factoryapi.Factory {
	t.Helper()

	generated, err := replay.GeneratedFactoryFromRuntimeConfig(
		"smoke-factory",
		loaded.FactoryConfig(),
		loaded,
		replay.WithGeneratedFactorySourceDirectory("smoke-factory"),
	)
	if err != nil {
		t.Fatalf("GeneratedFactoryFromRuntimeConfig: %v", err)
	}
	return generated
}

func marshalJSONForSmoke(t *testing.T, v any) []byte {
	t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal JSON: %v", err)
	}
	return data
}

func assertComparableFactoryContractsMatch(t *testing.T, flattened factoryapi.Factory, generated factoryapi.Factory) {
	t.Helper()

	comparable := generated
	comparable.FactoryDirectory = nil
	comparable.SourceDirectory = nil
	comparable.Metadata = nil
	if !reflect.DeepEqual(flattened, comparable) {
		t.Fatalf("flattened canonical config and generated Factory model diverged\nflattened: %#v\ngenerated: %#v", flattened, comparable)
	}
}

func assertFactoryConfigSmokeDescriptions(t *testing.T, factory *openapi3.Schema) {
	t.Helper()

	assertOpenAPI3Description(t, "Factory", factory.Description)
	assertOpenAPI3PropertyDescription(t, factory, "Factory", "id")
	workType := assertOpenAPI3ArrayPropertyDescription(t, factory, "Factory", "workTypes")
	resource := assertOpenAPI3ArrayPropertyDescription(t, factory, "Factory", "resources")
	worker := assertOpenAPI3ArrayPropertyDescription(t, factory, "Factory", "workers")
	workstation := assertOpenAPI3ArrayPropertyDescription(t, factory, "Factory", "workstations")

	assertOpenAPI3Description(t, "WorkType", workType.Description)
	assertOpenAPI3PropertyDescription(t, workType, "WorkType", "states")
	workState := assertOpenAPI3ArrayPropertyDescription(t, workType, "WorkType", "states")

	assertOpenAPI3Description(t, "WorkState", workState.Description)
	assertOpenAPI3PropertyDescription(t, workState, "WorkState", "type")

	assertOpenAPI3Description(t, "Resource", resource.Description)
	assertOpenAPI3PropertyDescription(t, resource, "Resource", "capacity")

	assertOpenAPI3Description(t, "Worker", worker.Description)
	for _, propertyName := range []string{"type", "model", "executorProvider", "modelProvider", "timeout"} {
		assertOpenAPI3PropertyDescription(t, worker, "Worker", propertyName)
	}

	assertOpenAPI3Description(t, "Workstation", workstation.Description)
	for _, propertyName := range []string{"behavior", "type", "worker", "limits", "inputs", "guards", "resources", "stopWords", "cron"} {
		assertOpenAPI3PropertyDescription(t, workstation, "Workstation", propertyName)
	}
	workstationIO := assertOpenAPI3ArrayPropertyDescription(t, workstation, "Workstation", "inputs")
	workstationGuard := assertOpenAPI3ArrayPropertyDescription(t, workstation, "Workstation", "guards")
	workstationCron := assertOpenAPI3RefPropertyDescription(t, workstation, "Workstation", "cron")

	assertOpenAPI3Description(t, "WorkstationIO", workstationIO.Description)
	assertOpenAPI3PropertyDescription(t, workstationIO, "WorkstationIO", "guards")
	guard := assertOpenAPI3ArrayPropertyDescription(t, workstationIO, "WorkstationIO", "guards")

	assertOpenAPI3Description(t, "Guard", guard.Description)
	assertOpenAPI3PropertyDescription(t, guard, "Guard", "type")

	assertOpenAPI3Description(t, "Guard", workstationGuard.Description)
	assertOpenAPI3PropertyDescription(t, workstationGuard, "Guard", "type")

	assertOpenAPI3Description(t, "WorkstationCron", workstationCron.Description)
	assertOpenAPI3PropertyDescription(t, workstationCron, "WorkstationCron", "triggerAtStart")
}

func assertFactoryConfigSmokeEnumRefs(t *testing.T) {
	t.Helper()

	data, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatalf("read openapi contract: %v", err)
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("parse openapi contract: %v", err)
	}

	schemas := componentSchemas(t, raw)
	assertSchemaPropertyRef(t, schemas, "InputType", "type", "#/components/schemas/InputKind")
	assertSchemaPropertyRef(t, schemas, "WorkState", "type", "#/components/schemas/WorkStateType")
	assertSchemaPropertyRef(t, schemas, "Worker", "type", "#/components/schemas/WorkerType")
	assertSchemaPropertyRef(t, schemas, "Worker", "executorProvider", "#/components/schemas/WorkerProvider")
	assertSchemaPropertyRef(t, schemas, "Worker", "modelProvider", "#/components/schemas/WorkerModelProvider")
	assertSchemaPropertyRef(t, schemas, "Workstation", "behavior", "#/components/schemas/WorkstationKind")
	assertSchemaPropertyRef(t, schemas, "Workstation", "type", "#/components/schemas/WorkstationType")
	assertSchemaPropertyRef(t, schemas, "Guard", "type", "#/components/schemas/GuardType")
}

func assertFactoryConfigSmokeGeneratedBoundary(t *testing.T, factory factoryapi.Factory) {
	t.Helper()

	if factory.Workers == nil || len(*factory.Workers) != 1 {
		t.Fatalf("generated boundary workers = %#v, want one worker", factory.Workers)
	}
	if (*factory.Workers)[0].ModelProvider == nil || *(*factory.Workers)[0].ModelProvider != factoryapi.WorkerModelProviderClaude {
		t.Fatalf("generated boundary worker modelProvider = %#v, want CLAUDE", (*factory.Workers)[0].ModelProvider)
	}
	if (*factory.Workers)[0].ExecutorProvider == nil || *(*factory.Workers)[0].ExecutorProvider != factoryapi.WorkerProviderScriptWrap {
		t.Fatalf("generated boundary worker executorProvider = %#v, want SCRIPT_WRAP", (*factory.Workers)[0].ExecutorProvider)
	}

	if factory.Workstations == nil || len(*factory.Workstations) < 1 {
		t.Fatalf("generated boundary workstations = %#v, want at least one workstation", factory.Workstations)
	}
	firstWorkstation := (*factory.Workstations)[0]
	if firstWorkstation.Behavior == nil || *firstWorkstation.Behavior != factoryapi.WorkstationKindCron {
		t.Fatalf("generated boundary workstation behavior = %#v, want CRON", firstWorkstation.Behavior)
	}
	if firstWorkstation.Type == nil || *firstWorkstation.Type != factoryapi.WorkstationTypeModelWorkstation {
		t.Fatalf("generated boundary workstation type = %#v, want MODEL_WORKSTATION", firstWorkstation.Type)
	}
	if firstWorkstation.Guards == nil || len(*firstWorkstation.Guards) != 1 || (*firstWorkstation.Guards)[0].Type != factoryapi.GuardTypeVisitCount {
		t.Fatalf("generated boundary workstation guards = %#v, want VISIT_COUNT", firstWorkstation.Guards)
	}
	if len(firstWorkstation.Inputs) < 2 || firstWorkstation.Inputs[1].Guards == nil || len(*firstWorkstation.Inputs[1].Guards) != 1 || (*firstWorkstation.Inputs[1].Guards)[0].Type != factoryapi.GuardTypeAllChildrenComplete {
		t.Fatalf("generated boundary input guards = %#v, want ALL_CHILDREN_COMPLETE", firstWorkstation.Inputs)
	}
}

func rewriteRepresentativeSmokeFixtureToLegacyAliases(t *testing.T, dir string) {
	t.Helper()

	rewriteSmokeFixtureFile(t, filepath.Join(dir, interfaces.FactoryConfigFile), strings.NewReplacer(
		`"workTypes"`, `"work_types"`,
		`"workType"`, `"work_type"`,
		`"onFailure"`, `"on_failure"`,
	))
	for _, worker := range []string{"worker-a", "worker-b"} {
		rewriteSmokeFixtureFile(t, filepath.Join(dir, "workers", worker, "AGENTS.md"), strings.NewReplacer(
			"stopToken:", "stop_token:",
		))
	}
}

func rewriteSmokeFixtureFile(t *testing.T, path string, replacer *strings.Replacer) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read smoke fixture %s: %v", path, err)
	}
	rewritten := replacer.Replace(string(content))
	if err := os.WriteFile(path, []byte(rewritten), 0o644); err != nil {
		t.Fatalf("write smoke fixture %s: %v", path, err)
	}
}

func factoryConfigSmokeCanonicalJSON() string {
  return `{
  "name": "analytics-platform",
  "id": "analytics-platform",
  "inputTypes": [{"name":"batch","type":"DEFAULT"}],
  "guards": [{"type":"INFERENCE_THROTTLE_GUARD","modelProvider":"CLAUDE","model":"claude-sonnet-4-20250514","refreshWindow":"15m"}],
  "workTypes": [
    {"name":"parent","states":[{"name":"init","type":"INITIAL"}]},
    {"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"failed","type":"FAILED"},{"name":"complete","type":"TERMINAL"}]}
  ],
  "resources": [{"name":"agent-slot","capacity":2}],
  "workers": [{
    "name":"executor",
    "type":"MODEL_WORKER",
    "executorProvider":"SCRIPT_WRAP",
    "modelProvider":"CLAUDE",
    "resources":[{"name":"agent-slot","capacity":1}],
    "stopToken":"COMPLETE",
    "skipPermissions":true,
    "body":"You are the executor."
  }],
  "workstations": [{
    "id":"execute-story-id",
    "name":"execute-story",
    "behavior":"CRON",
    "type":"MODEL_WORKSTATION",
    "worker":"executor",
    "promptFile":"prompt.md",
    "promptTemplate":"Implement {{ .WorkID }}.",
    "outputSchema":"schema.json",
    "limits":{"maxExecutionTime":"30m"},
    "onRejection":{"workType":"story","state":"init"},
    "onFailure":{"workType":"story","state":"failed"},
    "resources":[{"name":"agent-slot","capacity":2}],
    "stopWords":["DONE"],
    "workingDirectory":"/repo/{{ .WorkID }}",
    "cron":{"schedule":"*/10 * * * *","triggerAtStart":true,"expiryWindow":"20s"},
    "inputs":[
      {"workType":"parent","state":"init"},
      {"workType":"story","state":"complete","guards":[{"type":"ALL_CHILDREN_COMPLETE","parentInput":"parent","spawnedBy":"fanout"}]}
    ],
    "outputs":[{"workType":"story","state":"complete"}],
    "guards":[{"type":"VISIT_COUNT","workstation":"execute-story","maxVisits":3}],
    "env":{"TEAM":"factory"}
  }, {
    "name":"fanout",
    "worker":"executor",
    "type":"MODEL_WORKSTATION",
    "inputs":[{"workType":"parent","state":"init"}],
    "outputs":[{"workType":"story","state":"init"}]
  }, {
    "name":"guard-cycle",
    "worker":"executor",
    "type":"LOGICAL_MOVE",
    "inputs":[{"workType":"story","state":"complete"}],
    "outputs":[{"workType":"story","state":"failed"}],
    "guards":[{"type":"VISIT_COUNT","workstation":"execute-story","maxVisits":3}]
  }]
}`
}

func factoryConfigSmokeLegacyJSON() string {
  return `{
  "name": "analytics-platform",
  "id": "analytics-platform",
  "input_types": [{"name":"batch","type":"default"}],
  "work_types": [
    {"name":"parent","states":[{"name":"init","type":"INITIAL"}]},
    {"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"failed","type":"FAILED"},{"name":"complete","type":"TERMINAL"}]}
  ],
  "resources": [{"name":"agent-slot","capacity":2}],
  "workers": [{
    "name":"executor",
    "type":"MODEL_WORKER",
    "provider":"script_wrap",
    "model_provider":"anthropic",
    "resources":[{"name":"agent-slot","capacity":1}],
    "stop_token":"COMPLETE",
    "skip_permissions":true,
    "body":"You are the executor."
  }],
  "workstations": [{
    "id":"execute-story-id",
    "name":"execute-story",
    "kind":"cron",
    "runtime_type":"MODEL_WORKSTATION",
    "worker":"executor",
    "prompt_file":"prompt.md",
    "prompt_template":"Implement {{ .WorkID }}.",
    "output_schema":"schema.json",
    "timeout":"30m",
    "on_rejection":{"work_type":"story","state":"init"},
    "on_failure":{"work_type":"story","state":"failed"},
    "resource-usage":[{"name":"agent-slot","capacity":2}],
    "stopToken":"DONE",
    "working_directory":"/repo/{{ .WorkID }}",
    "cron":{"schedule":"*/10 * * * *","trigger_at_start":true,"expiry_window":"20s"},
    "inputs":[
      {"work_type":"parent","state":"init"},
      {"work_type":"story","state":"complete","guards":[{"type":"all_children_complete","parent_input":"parent","spawned_by":"fanout"}]}
    ],
    "outputs":[{"work_type":"story","state":"complete"}],
    "guards":[{"type":"visit_count","workstation":"execute-story","max_visits":3}],
    "env":{"TEAM":"factory"}
  }, {
    "name":"fanout",
    "worker":"executor",
    "type":"MODEL_WORKSTATION",
    "inputs":[{"work_type":"parent","state":"init"}],
    "outputs":[{"work_type":"story","state":"init"}]
  }, {
    "name":"guard-cycle",
    "worker":"executor",
    "type":"LOGICAL_MOVE",
    "inputs":[{"work_type":"story","state":"complete"}],
    "outputs":[{"work_type":"story","state":"failed"}],
    "guards":[{"type":"visit_count","workstation":"execute-story","max_visits":3}]
  }]
}`
}
