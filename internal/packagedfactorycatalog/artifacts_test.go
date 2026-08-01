package packagedfactorycatalog_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"github.com/portpowered/infinite-you/internal/packagedfactorycatalog"
	"github.com/portpowered/infinite-you/internal/testpath"
	packagedfactories "github.com/portpowered/infinite-you/packages/packaged-factories"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

const testSchemaPath = "schemas/factory.schema.json"

func TestGenerateArtifactsProducesEquivalentSelfContainedPairsForCompleteInventory(t *testing.T) {
	t.Parallel()

	artifacts, err := packagedfactorycatalog.GenerateArtifacts(
		context.Background(),
		packagedfactories.Source(),
		"factories",
		testSchemaPath,
	)
	if err != nil {
		t.Fatalf("GenerateArtifacts: %v", err)
	}
	if len(artifacts) != 14 {
		t.Fatalf("artifacts = %d, want 14", len(artifacts))
	}

	bySlug := make(map[string]packagedfactorycatalog.ArtifactPair, len(artifacts))
	for _, artifact := range artifacts {
		bySlug[artifact.Slug] = artifact
		assertEquivalentArtifactPair(t, artifact)
		for _, worker := range artifact.Factory.Workers {
			if worker.Type == "AGENT_WORKER" && !worker.SkipPermissions {
				t.Fatalf("packaged Factory %q agent worker %q does not default skipPermissions to true", artifact.PublicName, worker.Name)
			}
		}
	}
	if bySlug["fusion"].PublicName != "@you/fusion" || bySlug["fusion"].Factory.Name != "fusion" {
		t.Fatalf("fusion public/artifact names = %q/%q", bySlug["fusion"].PublicName, bySlug["fusion"].Factory.Name)
	}
	if !strings.Contains(string(bySlug["fusion"].JSON), `"modelProvider": "${firstProvider}"`) {
		t.Fatal("fusion artifact did not preserve invocation-interpolated provider")
	}
	assertInlineAsset(t, bySlug["goal"].JSON, "workstations", "execute-goal", "body", "You are executing persistent goal work")
	if !strings.Contains(string(bySlug["deep-research"].JSON), `"inlineSource"`) ||
		!strings.Contains(string(bySlug["deep-research"].JSON), `@you-factory-meta`) {
		t.Fatal("deep-research artifact did not inline its standalone factory.js source")
	}
	assertExamplesPreserved(t, bySlug["subagent"])
	assertMetaPlannerContract(t, bySlug["plan-parallel"], "parallel-planner")
	assertPlanParallelDelegatesTerminalSynthesisToMerger(t, bySlug["plan-parallel"])
	assertPlanParallelExecutorsReturnSubstantiveResults(t, bySlug["plan-parallel"])
	assertPlanParallelMergerPrimaryResultContract(t, bySlug["plan-parallel"])
	assertMetaPlannerContract(t, bySlug["full-flow"], "full-flow-planner")
}

func TestPackagedFactoryPromptFilesRemainWorkstationOwned(t *testing.T) {
	t.Parallel()

	type worker struct {
		Name       string `yaml:"name"`
		PromptFile string `yaml:"promptFile"`
	}
	type workstation struct {
		Worker     string `yaml:"worker"`
		PromptFile string `yaml:"promptFile"`
	}
	type authoredFactory struct {
		Workers      []worker      `yaml:"workers"`
		Workstations []workstation `yaml:"workstations"`
	}

	want := map[string]map[string]string{
		"goal": {
			"goal-executor": "prompts/executor.md",
		},
		"plan-execute": {
			"prd-planner":  "prompts/planner.md",
			"prd-executor": "prompts/executor.md",
		},
		"review": {
			"review-work-executor": "prompts/executor.md",
			"review-work-reviewer": "prompts/reviewer.md",
		},
		"full-flow": {
			"full-flow-planner":  "prompts/planner.md",
			"full-flow-executor": "prompts/executor.md",
			"full-flow-reviewer": "prompts/reviewer.md",
			"full-flow-ci":       "prompts/ci.md",
			"full-flow-merger":   "prompts/merger.md",
		},
	}

	artifacts, err := packagedfactorycatalog.GenerateArtifacts(
		context.Background(),
		packagedfactories.Source(),
		"factories",
		testSchemaPath,
	)
	if err != nil {
		t.Fatalf("GenerateArtifacts: %v", err)
	}
	bySlug := make(map[string]packagedfactorycatalog.ArtifactPair, len(artifacts))
	for _, artifact := range artifacts {
		bySlug[artifact.Slug] = artifact
	}

	for slug, wantPromptFiles := range want {
		slug := slug
		wantPromptFiles := wantPromptFiles
		t.Run(slug, func(t *testing.T) {
			sourcePath := "factories/" + slug + "/factory.yaml"
			payload, err := fs.ReadFile(packagedfactories.Source(), sourcePath)
			if err != nil {
				t.Fatalf("read authored Factory %s: %v", sourcePath, err)
			}
			var authored authoredFactory
			if err := yaml.Unmarshal(payload, &authored); err != nil {
				t.Fatalf("decode authored Factory %s: %v", sourcePath, err)
			}

			for _, worker := range authored.Workers {
				if worker.PromptFile != "" {
					t.Fatalf("worker %q owns promptFile %q; prompt files belong to workstations", worker.Name, worker.PromptFile)
				}
			}
			authoredPromptFiles := make(map[string]string, len(authored.Workstations))
			for _, workstation := range authored.Workstations {
				if workstation.PromptFile != "" {
					authoredPromptFiles[workstation.Worker] = workstation.PromptFile
				}
			}
			if !reflect.DeepEqual(authoredPromptFiles, wantPromptFiles) {
				t.Fatalf("authored workstation prompt files = %#v, want %#v", authoredPromptFiles, wantPromptFiles)
			}

			artifact, ok := bySlug[slug]
			if !ok {
				t.Fatalf("missing generated artifact %q", slug)
			}
			materializedPromptFiles := make(map[string]string, len(artifact.Factory.Workstations))
			for _, workstation := range artifact.Factory.Workstations {
				if workstation.PromptFile != "" {
					materializedPromptFiles[workstation.WorkerTypeName] = workstation.PromptFile
				}
			}
			if !reflect.DeepEqual(materializedPromptFiles, wantPromptFiles) {
				t.Fatalf("materialized workstation prompt files = %#v, want %#v", materializedPromptFiles, wantPromptFiles)
			}

			root := decodeArtifactObject(t, artifact.JSON)
			for _, value := range root["workers"].([]any) {
				worker := value.(map[string]any)
				if _, ok := worker["promptFile"]; ok {
					t.Fatalf("materialized worker %q still contains promptFile", worker["name"])
				}
			}
		})
	}
}

func TestPackagedFactorySchemaRejectsWorkerPromptFileAndAcceptsWorkstationPromptFile(t *testing.T) {
	t.Parallel()

	schemaPayload, err := fs.ReadFile(packagedfactories.Source(), testSchemaPath)
	if err != nil {
		t.Fatalf("read packaged Factory schema: %v", err)
	}
	var schemaDocument any
	if err := json.Unmarshal(schemaPayload, &schemaDocument); err != nil {
		t.Fatalf("decode packaged Factory schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(testSchemaPath, schemaDocument); err != nil {
		t.Fatalf("register packaged Factory schema: %v", err)
	}
	schema, err := compiler.Compile(testSchemaPath)
	if err != nil {
		t.Fatalf("compile packaged Factory schema: %v", err)
	}

	worker := map[string]any{"name": "worker", "type": "AGENT_WORKER"}
	invalidWorker := map[string]any{
		"name":    "prompt-owner",
		"workers": []any{withPromptFile(worker, "prompts/worker.md")},
	}
	if err := schema.Validate(invalidWorker); err == nil {
		t.Fatal("schema accepted worker-level promptFile")
	}

	validWorkstation := map[string]any{
		"name":    "prompt-owner",
		"workers": []any{worker},
		"workstations": []any{map[string]any{
			"name":       "run-worker",
			"type":       "AGENT_RUN",
			"worker":     "worker",
			"inputs":     []any{},
			"promptFile": "prompts/worker.md",
		}},
	}
	if err := schema.Validate(validWorkstation); err != nil {
		t.Fatalf("schema rejected workstation-level promptFile: %v", err)
	}
}

func withPromptFile(fields map[string]any, promptFile string) map[string]any {
	withPrompt := make(map[string]any, len(fields)+1)
	for key, value := range fields {
		withPrompt[key] = value
	}
	withPrompt["promptFile"] = promptFile
	return withPrompt
}

func assertPlanParallelExecutorsReturnSubstantiveResults(t *testing.T, artifact packagedfactorycatalog.ArtifactPair) {
	t.Helper()
	for _, worker := range artifact.Factory.Workers {
		if worker.Name != "parallel-executor" {
			continue
		}
		for _, required := range []string{"Execute the one assigned Work item", "Never return a bare control token", "substantive evidence report"} {
			if !strings.Contains(worker.Body, required) {
				t.Fatalf("packaged Factory %q executor body does not contain %q", artifact.PublicName, required)
			}
		}
		return
	}
	t.Fatalf("packaged Factory %q does not contain parallel-executor", artifact.PublicName)
}

func assertPlanParallelDelegatesTerminalSynthesisToMerger(t *testing.T, artifact packagedfactorycatalog.ArtifactPair) {
	t.Helper()
	const required = "Do not create a catch-all synthesis, summary, merge, or final-answer planned task"
	for _, worker := range artifact.Factory.Workers {
		if worker.Name == "parallel-planner" && strings.Contains(worker.Body, required) {
			return
		}
	}
	t.Fatalf("packaged Factory %q planner does not reserve terminal synthesis for its merger", artifact.PublicName)
}

func assertPlanParallelMergerPrimaryResultContract(t *testing.T, artifact packagedfactorycatalog.ArtifactPair) {
	t.Helper()
	if artifact.Factory.InvocationReturn == nil ||
		artifact.Factory.InvocationReturn.Policy != work.InvocationReturnPolicyExplicit ||
		artifact.Factory.InvocationReturn.WorkTypeName != "parallel-plan" ||
		artifact.Factory.InvocationReturn.TerminalState != "complete" {
		t.Fatalf(
			"packaged Factory %q invocationReturn = %#v, want EXPLICIT parallel-plan/complete",
			artifact.PublicName,
			artifact.Factory.InvocationReturn,
		)
	}

	var merger *factorydefinitions.FactoryWorkstationConfig
	for index := range artifact.Factory.Workstations {
		if artifact.Factory.Workstations[index].Name == "merge-plan-results" {
			merger = &artifact.Factory.Workstations[index]
			break
		}
	}
	if merger == nil {
		t.Fatalf("packaged Factory %q does not contain merge-plan-results", artifact.PublicName)
	}
	body := merger.Body
	if body == "" {
		body = merger.PromptTemplate
	}
	for _, required := range []string{
		"Treat the original request and all completed generated Work inputs",
		"Original parent request and completed generated child results",
		"Never claim that completed generated task results are missing",
		"{{ if .Payload -}}",
		"{{- else if .PreviousOutput -}}",
		"{{ range .Content }}{{ .Text }}{{ end }}",
	} {
		if !strings.Contains(body, required) {
			t.Fatalf("packaged Factory %q merger body does not contain %q", artifact.PublicName, required)
		}
	}
	if len(merger.Inputs) != 2 ||
		merger.Inputs[0].WorkTypeName != "parallel-plan" || merger.Inputs[0].StateName != "waiting" ||
		merger.Inputs[1].WorkTypeName != "planned-task" || merger.Inputs[1].StateName != "complete" ||
		merger.Inputs[1].Guard == nil ||
		merger.Inputs[1].Guard.Type != factorydefinitions.GuardTypeAllChildrenComplete ||
		merger.Inputs[1].Guard.ParentInput != "parallel-plan" ||
		merger.Inputs[1].Guard.SpawnedBy != "plan-parallel-work" {
		t.Fatalf("packaged Factory %q merger inputs = %#v, want parent waiting plus ALL_CHILDREN_COMPLETE children", artifact.PublicName, merger.Inputs)
	}
	if len(merger.Outputs) != 1 ||
		merger.Outputs[0].WorkTypeName != "parallel-plan" ||
		merger.Outputs[0].StateName != "complete" {
		t.Fatalf("packaged Factory %q merger outputs = %#v, want parallel-plan complete", artifact.PublicName, merger.Outputs)
	}

	type contentPart struct{ Text string }
	type inputToken struct {
		Name           string
		WorkTypeID     string
		Payload        string
		PreviousOutput string
		Content        []contentPart
	}
	rendered, err := renderPlanParallelMergerPrompt(body, []inputToken{
		{Name: "work-1", WorkTypeID: "parallel-plan", Payload: "PARENT_REQUEST_MARKER"},
		{Name: "task-a", WorkTypeID: "planned-task", Payload: "CHILD_A_RESULT_MARKER"},
		{Name: "task-b", WorkTypeID: "planned-task", PreviousOutput: "CHILD_B_PREVIOUS_OUTPUT_MARKER"},
		{
			Name:       "task-c",
			WorkTypeID: "planned-task",
			Content:    []contentPart{{Text: "CHILD_C_CONTENT_MARKER"}},
		},
	})
	if err != nil {
		t.Fatalf("render packaged Factory %q merger prompt: %v", artifact.PublicName, err)
	}
	for _, required := range []string{
		"PARENT_REQUEST_MARKER",
		"CHILD_A_RESULT_MARKER",
		"CHILD_B_PREVIOUS_OUTPUT_MARKER",
		"CHILD_C_CONTENT_MARKER",
		"--- work-1 (parallel-plan) ---",
		"--- task-a (planned-task) ---",
		"--- task-b (planned-task) ---",
		"--- task-c (planned-task) ---",
	} {
		if !strings.Contains(rendered, required) {
			t.Fatalf("rendered merger prompt missing %q:\n%s", required, rendered)
		}
	}
}

func renderPlanParallelMergerPrompt(body string, inputs any) (string, error) {
	parsed, err := template.New("merge-plan-results").Parse(body)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := parsed.Execute(&buf, map[string]any{"Inputs": inputs}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func assertMetaPlannerContract(t *testing.T, artifact packagedfactorycatalog.ArtifactPair, workerName string) {
	t.Helper()
	var promptBody string
	foundWorker := false
	for _, worker := range artifact.Factory.Workers {
		if worker.Name != workerName {
			continue
		}
		foundWorker = true
		promptBody = worker.Body
		break
	}
	if strings.TrimSpace(promptBody) == "" {
		for _, workstation := range artifact.Factory.Workstations {
			if workstation.WorkerTypeName == workerName {
				promptBody = workstation.Body
				break
			}
		}
	}
	if !foundWorker {
		t.Fatalf("packaged Factory %q does not contain planner worker %q", artifact.PublicName, workerName)
	}
	for _, required := range []string{
		"you docs agents",
		"Never run bare `you`",
		`{"request":{"type":"FACTORY_REQUEST_BATCH"`,
		"sourceWorkName",
		"targetWorkName",
		"requiredState",
	} {
		if !strings.Contains(promptBody, required) {
			t.Fatalf("packaged Factory %q planner %q prompt body does not contain %q", artifact.PublicName, workerName, required)
		}
	}
}

func TestGenerateArtifactsEmbedsDocumentsInputsAndPreservesMetadataExactly(t *testing.T) {
	t.Parallel()

	source := artifactFixtureFS()
	artifacts, err := packagedfactorycatalog.GenerateArtifacts(
		context.Background(), source, "factories", testSchemaPath,
	)
	if err != nil {
		t.Fatalf("GenerateArtifacts: %v", err)
	}
	if len(artifacts) != 1 {
		t.Fatalf("artifacts = %d, want 1", len(artifacts))
	}
	artifact := artifacts[0]
	assertBundledFile(t, artifact.JSON, "factory/docs/guide.md", "DOC", "# Guide\n\nExact.\n")
	assertBundledFile(t, artifact.JSON, "factory/inputs/task/default/request.json", "INPUT", "{\n  \"input\": \"keep spaces  \"\n}\n")

	cfg, err := factorymapping.NewFactoryConfigMapper().Expand(artifact.JSON)
	if err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if cfg.Description == nil || cfg.Description.ID != "description.asset" ||
		!reflect.DeepEqual(cfg.Description.Locales, []string{"en-US", "fr-FR"}) ||
		cfg.Description.Values["fr-FR"] != "Description française" {
		t.Fatalf("description metadata changed: %#v", cfg.Description)
	}
	if got := cfg.Examples[0].Args["payload"]; got != "line one\n  line two\n" {
		t.Fatalf("opaque example payload = %#v", got)
	}
	if cfg.ResourceManifest == nil || len(cfg.ResourceManifest.RequiredTools) != 1 ||
		cfg.ResourceManifest.RequiredTools[0].Command != "external-tool" {
		t.Fatalf("validation-only dependency changed: %#v", cfg.ResourceManifest)
	}
}

func TestGeneratedFusionJSONAndYAMLValidateDirectlyAgainstPackagedSchema(t *testing.T) {
	t.Parallel()

	repositoryRoot := testpath.MustRepoPathFromCaller(t, 0)
	schemaPath := filepath.Join(repositoryRoot, "packages", "packaged-factories", testSchemaPath)
	schemaPayload, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read packaged Factory schema: %v", err)
	}
	var schemaDocument any
	if err := json.Unmarshal(schemaPayload, &schemaDocument); err != nil {
		t.Fatalf("decode packaged Factory schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(testSchemaPath, schemaDocument); err != nil {
		t.Fatalf("register packaged Factory schema: %v", err)
	}
	schema, err := compiler.Compile(testSchemaPath)
	if err != nil {
		t.Fatalf("compile packaged Factory schema: %v", err)
	}

	generatedRoot := filepath.Join(
		repositoryRoot,
		"packages",
		"packaged-factories",
		"generated",
		"factories",
		"fusion",
	)
	for _, format := range []string{"json", "yaml"} {
		format := format
		t.Run(format, func(t *testing.T) {
			t.Parallel()
			payload, err := os.ReadFile(filepath.Join(generatedRoot, "factory."+format))
			if err != nil {
				t.Fatalf("read generated Fusion %s: %v", format, err)
			}
			var document any
			if format == "json" {
				err = json.Unmarshal(payload, &document)
			} else {
				err = yaml.Unmarshal(payload, &document)
			}
			if err != nil {
				t.Fatalf("decode generated Fusion %s: %v", format, err)
			}
			if err := schema.Validate(document); err != nil {
				t.Fatalf("unmodified generated Fusion %s does not validate directly: %v", format, err)
			}
		})
	}
}

func TestGenerateArtifactsReportsMissingUnsafeAndUnsupportedAssets(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(fstest.MapFS)
		want   []string
	}{
		{
			name: "missing prompt",
			mutate: func(source fstest.MapFS) {
				delete(source, "factories/example/prompts/worker.md")
			},
			want: []string{"factories/example/factory.json", "prompt", "prompts/worker.md", "file does not exist"},
		},
		{
			name: "unsafe prompt",
			mutate: func(source fstest.MapFS) {
				root := source["factories/example/factory.json"]
				root.Data = []byte(strings.ReplaceAll(string(root.Data), "prompts/worker.md", "../outside.md"))
			},
			want: []string{"factories/example/factory.json", "../outside.md", "escapes the package asset root"},
		},
		{
			name: "unsupported asset kind",
			mutate: func(source fstest.MapFS) {
				source["factories/example/docs/link.md"] = &fstest.MapFile{Mode: fs.ModeSymlink | 0o777}
			},
			want: []string{"factories/example/factory.json", "docs/link.md", "unsupported non-regular"},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source := artifactFixtureFS()
			test.mutate(source)
			_, err := packagedfactorycatalog.GenerateArtifacts(context.Background(), source, "factories", testSchemaPath)
			assertErrorContains(t, err, test.want...)
		})
	}
}

func TestGenerateArtifactsRejectsMissingOrInvalidPackageSchema(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		schemaPath string
		schema     []byte
		want       string
	}{
		{name: "missing", schemaPath: "schemas/missing.json", want: "read package Factory schema"},
		{name: "invalid", schemaPath: testSchemaPath, schema: []byte(`{"type":`), want: "decode package Factory schema"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			source := artifactFixtureFS()
			if test.schema != nil {
				source[testSchemaPath] = &fstest.MapFile{Data: test.schema}
			}
			_, err := packagedfactorycatalog.GenerateArtifacts(context.Background(), source, "factories", test.schemaPath)
			assertErrorContains(t, err, "schema validation", test.want)
		})
	}
}

func assertEquivalentArtifactPair(t *testing.T, artifact packagedfactorycatalog.ArtifactPair) {
	t.Helper()
	var yamlDocument any
	if err := yaml.Unmarshal(artifact.YAML, &yamlDocument); err != nil {
		t.Fatalf("%s YAML decode: %v", artifact.Slug, err)
	}
	yamlJSON, err := json.Marshal(yamlDocument)
	if err != nil {
		t.Fatalf("%s YAML normalize: %v", artifact.Slug, err)
	}
	mapper := factorymapping.NewFactoryConfigMapper()
	jsonFactory, err := mapper.Expand(artifact.JSON)
	if err != nil {
		t.Fatalf("%s JSON canonical decode: %v", artifact.Slug, err)
	}
	yamlFactory, err := mapper.Expand(yamlJSON)
	if err != nil {
		t.Fatalf("%s YAML canonical decode: %v", artifact.Slug, err)
	}
	if !reflect.DeepEqual(jsonFactory, yamlFactory) {
		t.Fatalf("%s JSON/YAML canonical values differ", artifact.Slug)
	}
}

func assertInlineAsset(t *testing.T, payload []byte, collection, name, field, wantFragment string) {
	t.Helper()
	root := decodeArtifactObject(t, payload)
	for _, value := range root[collection].([]any) {
		entry := value.(map[string]any)
		if entry["name"] == name && strings.Contains(entry[field].(string), wantFragment) {
			return
		}
	}
	t.Fatalf("%s %q has no embedded %s containing %q", collection, name, field, wantFragment)
}

func assertBundledFile(t *testing.T, payload []byte, target, fileType, contentFragment string) {
	t.Helper()
	root := decodeArtifactObject(t, payload)
	manifest := root["supportingFiles"].(map[string]any)
	for _, value := range manifest["bundledFiles"].([]any) {
		file := value.(map[string]any)
		if file["targetPath"] != target {
			continue
		}
		if file["type"] != fileType {
			t.Fatalf("%s type = %#v, want %s", target, file["type"], fileType)
		}
		inline := file["content"].(map[string]any)["inline"].(string)
		if !strings.Contains(inline, contentFragment) {
			t.Fatalf("%s inline content = %q, want fragment %q", target, inline, contentFragment)
		}
		return
	}
	t.Fatalf("missing bundled file %s", target)
}

func assertExamplesPreserved(t *testing.T, artifact packagedfactorycatalog.ArtifactPair) {
	t.Helper()
	cfg, err := factorymapping.NewFactoryConfigMapper().Expand(artifact.JSON)
	if err != nil {
		t.Fatalf("decode examples: %v", err)
	}
	if len(cfg.Examples) != 2 || cfg.Examples[1].Args["input"] != "Summarize this release" {
		t.Fatalf("examples changed: %#v", cfg.Examples)
	}
}

func decodeArtifactObject(t *testing.T, payload []byte) map[string]any {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal(payload, &root); err != nil {
		t.Fatalf("decode artifact: %v", err)
	}
	return root
}

func artifactFixtureFS() fstest.MapFS {
	schema := packagedfactories.Source()
	schemaPayload, err := fs.ReadFile(schema, testSchemaPath)
	if err != nil {
		panic(err)
	}
	return fstest.MapFS{
		testSchemaPath: {Data: schemaPayload},
		"factories/example/factory.json": {Data: []byte(`{
		  "name":"@you/example",
		  "id":"factory.example",
		  "description":{"id":"description.asset","type":"LOCALIZABLE_ASSET","value":"Fallback","locales":["en-US","fr-FR"],"values":{"fr-FR":"Description française"}},
		  "examples":[{"name":"exact-payload","description":{"type":"LOCALIZABLE_ASSET","value":"Example"},"args":{"payload":"line one\n  line two\n"}}],
		  "supportingFiles":{"requiredTools":[{"command":"external-tool"}]},
		  "workTypes":[{"name":"task","handlingBehavior":["DEFAULT"],"states":[{"name":"default","type":"INITIAL"},{"name":"done","type":"TERMINAL"},{"name":"failed","type":"FAILED"}]}],
		  "resources":[],
		  "workers":[{"name":"worker","type":"AGENT_WORKER","promptFile":"prompts/worker.md"}],
		  "workstations":[]
		}`)},
		"factories/example/prompts/worker.md":                {Data: []byte("Exact prompt.\n")},
		"factories/example/docs/guide.md":                    {Data: []byte("# Guide\n\nExact.\n")},
		"factories/example/inputs/task/default/request.json": {Data: []byte("{\n  \"input\": \"keep spaces  \"\n}\n")},
	}
}
