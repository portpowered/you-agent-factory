package mockworkers_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/internal/testutil"
	"github.com/portpowered/infinite-you/pkg/services/workers/interface"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	documentationSchemaID = "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json"
	deprecationsSchemaID  = "https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json"
)

func TestMockWorkersSchema_DeclaresDraft202012AndStableID(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	if got, _ := schema["$schema"].(string); got != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("$schema = %q, want Draft 2020-12", got)
	}
	if got, _ := schema["$id"].(string); got != mockworkers.SchemaID {
		t.Fatalf("$id = %q, want %q", got, mockworkers.SchemaID)
	}
}

func TestMockWorkersSchema_ClosesAuthoredObjects(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	assertObjectClosed(t, schema, "/")

	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("$defs missing or not an object")
	}
	for name, def := range defs {
		defObject, ok := def.(map[string]any)
		if !ok {
			t.Fatalf("$defs[%q] is not an object", name)
		}
		assertObjectClosed(t, defObject, "/$defs/"+name)
	}
}

func TestMockWorkersSchema_ExpressesB03TopologyFields(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	rootProps := objectProperties(t, schema, "/")
	assertPropertyNames(t, rootProps, []string{"mockWorkers", "unmatchedDispatchPolicy"}, "/")

	mockWorker := defSchema(t, schema, "mockWorker")
	mockWorkerProps := objectProperties(t, mockWorker, "/$defs/mockWorker")
	assertPropertyNames(t, mockWorkerProps, []string{
		"id",
		"workerName",
		"workstationName",
		"workInputs",
		"runType",
		"scriptConfig",
		"rejectConfig",
	}, "/$defs/mockWorker")

	workInput := defSchema(t, schema, "workInput")
	assertPropertyNames(t, objectProperties(t, workInput, "/$defs/workInput"), []string{
		"workId",
		"workType",
		"state",
		"inputName",
		"traceId",
		"channel",
		"payloadHash",
	}, "/$defs/workInput")

	scriptConfig := defSchema(t, schema, "scriptConfig")
	assertPropertyNames(t, objectProperties(t, scriptConfig, "/$defs/scriptConfig"), []string{
		"command",
		"args",
		"env",
		"workingDirectory",
		"stdin",
		"timeout",
	}, "/$defs/scriptConfig")

	rejectConfig := defSchema(t, schema, "rejectConfig")
	assertPropertyNames(t, objectProperties(t, rejectConfig, "/$defs/rejectConfig"), []string{
		"stdout",
		"stderr",
		"exitCode",
	}, "/$defs/rejectConfig")

	inventory := mockworkers.ProjectTopologyInventory()
	for _, field := range inventory.Fields {
		if field.JSONName == "" {
			t.Fatalf("topology field %q missing jsonName", field.ID)
		}
	}
}

func TestMockWorkersSchema_ExpressesRunTypeAndPolicyEnums(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	mockWorker := defSchema(t, schema, "mockWorker")
	runType := objectProperties(t, mockWorker, "/$defs/mockWorker")["runType"].(map[string]any)
	assertEnumValues(t, runType, []string{"accept", "script", "reject"}, "/$defs/mockWorker.runType")

	rootProps := objectProperties(t, schema, "/")
	policy := rootProps["unmatchedDispatchPolicy"].(map[string]any)
	assertEnumValues(t, policy, []string{"accept", "passthrough"}, "/unmatchedDispatchPolicy")

	required, _ := schema["required"].([]any)
	if !containsString(required, "mockWorkers") {
		t.Fatalf("required = %#v, want mockWorkers", required)
	}
}

func TestMockWorkersSchema_EnforcesScriptConditionalUnion(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	mockWorker := defSchema(t, schema, "mockWorker")
	allOf, ok := mockWorker["allOf"].([]any)
	if !ok || len(allOf) == 0 {
		t.Fatal("mockWorker.allOf missing script conditional union")
	}

	scriptConfig := defSchema(t, schema, "scriptConfig")
	required, _ := scriptConfig["required"].([]any)
	if !containsString(required, "command") {
		t.Fatalf("scriptConfig.required = %#v, want command", required)
	}
	command := objectProperties(t, scriptConfig, "/$defs/scriptConfig")["command"].(map[string]any)
	if minLength, _ := command["minLength"].(float64); minLength < 1 {
		t.Fatalf("scriptConfig.command.minLength = %v, want >= 1", command["minLength"])
	}
}

func TestMockWorkersSchema_EnforcesRejectExitCodeBounds(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	rejectConfig := defSchema(t, schema, "rejectConfig")
	exitCode := objectProperties(t, rejectConfig, "/$defs/rejectConfig")["exitCode"].(map[string]any)
	if got, _ := exitCode["minimum"].(float64); got != 1 {
		t.Fatalf("rejectConfig.exitCode.minimum = %v, want 1", exitCode["minimum"])
	}
	if got, _ := exitCode["maximum"].(float64); got != 255 {
		t.Fatalf("rejectConfig.exitCode.maximum = %v, want 255", exitCode["maximum"])
	}
}

func TestMockWorkersSchema_DoesNotAdvertiseUnsupportedCapabilities(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	instanceSurface := map[string]any{
		"properties":  schema["properties"],
		"$defs":       schema["$defs"],
		"description": schema["description"],
	}
	encoded, err := json.Marshal(instanceSurface)
	if err != nil {
		t.Fatalf("marshal schema instance surface: %v", err)
	}
	lower := strings.ToLower(string(encoded))
	for _, forbidden := range []string{
		"\"media\"",
		"\"artifact\"",
		"\"responsesequence\"",
		"\"response_sequence\"",
		"\"dispatchdelay\"",
		"\"dispatch_delay\"",
		"\"sleep\"",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("schema advertises unsupported capability property %s", forbidden)
		}
	}

	description, _ := schema["description"].(string)
	for _, phrase := range []string{"response sequence", "artifact payload", "dispatch delay"} {
		if strings.Contains(strings.ToLower(description), phrase) {
			t.Fatalf("root description advertises unsupported capability %q", phrase)
		}
	}
}

func TestMockWorkersSchema_CompilesAsDraft202012(t *testing.T) {
	t.Parallel()

	compileAuthoredMockWorkersSchema(t)
}

func loadAuthoredMockWorkersSchema(t *testing.T) map[string]any {
	t.Helper()

	path := testutil.MustRepoPath(t, mockworkers.SchemaRelativePath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema %s: %v", path, err)
	}
	var schema map[string]any
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	return schema
}

func compileAuthoredMockWorkersSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	path := testutil.MustRepoPath(t, mockworkers.SchemaRelativePath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema %s: %v", path, err)
	}
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(mockworkers.SchemaID, document); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	compiled, err := compiler.Compile(mockworkers.SchemaID)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return compiled
}

func defSchema(t *testing.T, schema map[string]any, name string) map[string]any {
	t.Helper()

	defs, ok := schema["$defs"].(map[string]any)
	if !ok {
		t.Fatal("$defs missing or not an object")
	}
	def, ok := defs[name].(map[string]any)
	if !ok {
		t.Fatalf("$defs[%q] missing or not an object", name)
	}
	return def
}

func objectProperties(t *testing.T, object map[string]any, path string) map[string]any {
	t.Helper()

	props, ok := object["properties"].(map[string]any)
	if !ok {
		t.Fatalf("%s.properties missing or not an object", path)
	}
	return props
}

func assertObjectClosed(t *testing.T, object map[string]any, path string) {
	t.Helper()

	if object["type"] != "object" {
		return
	}
	if object["additionalProperties"] != false {
		t.Fatalf("%s.additionalProperties = %#v, want false", path, object["additionalProperties"])
	}
}

func assertPropertyNames(t *testing.T, props map[string]any, want []string, path string) {
	t.Helper()

	got := make([]string, 0, len(props))
	for name := range props {
		got = append(got, name)
	}
	if len(got) != len(want) {
		t.Fatalf("%s property count = %d (%#v), want %d (%#v)", path, len(got), got, len(want), want)
	}
	for _, name := range want {
		if _, ok := props[name]; !ok {
			t.Fatalf("%s missing property %q in %#v", path, name, got)
		}
	}
}

func assertEnumValues(t *testing.T, property map[string]any, want []string, path string) {
	t.Helper()

	raw, ok := property["enum"].([]any)
	if !ok {
		t.Fatalf("%s.enum missing", path)
	}
	got := make([]string, 0, len(raw))
	for _, value := range raw {
		text, ok := value.(string)
		if !ok {
			t.Fatalf("%s.enum contains non-string %#v", path, value)
		}
		got = append(got, text)
	}
	if len(got) != len(want) {
		t.Fatalf("%s.enum = %#v, want %#v", path, got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("%s.enum = %#v, want %#v", path, got, want)
		}
	}
}

func containsString(values []any, want string) bool {
	for _, value := range values {
		if text, ok := value.(string); ok && text == want {
			return true
		}
	}
	return false
}

func TestMockWorkersSchema_ContractCoversInventoriedTopology(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	contract, ok := schema["contract"].(map[string]any)
	if !ok {
		t.Fatal("schema missing contract annotation")
	}
	if contract["formatVersion"] != mockworkers.ContractFormatVersion {
		t.Fatalf("contract.formatVersion = %#v, want %q", contract["formatVersion"], mockworkers.ContractFormatVersion)
	}

	inventory := mockworkers.ProjectTopologyInventory()
	if contract["unknownFieldPolicy"] != inventory.UnknownFieldPolicy {
		t.Fatalf("contract.unknownFieldPolicy = %#v, want %q", contract["unknownFieldPolicy"], inventory.UnknownFieldPolicy)
	}
	if contract["entrySelectionPolicy"] != inventory.EntrySelectionPolicy {
		t.Fatalf("contract.entrySelectionPolicy = %#v, want %q", contract["entrySelectionPolicy"], inventory.EntrySelectionPolicy)
	}
	if contract["runTypeUnionSummary"] != inventory.RunTypeUnion.Summary {
		t.Fatalf("contract.runTypeUnionSummary = %#v, want %q", contract["runTypeUnionSummary"], inventory.RunTypeUnion.Summary)
	}

	fields, ok := contract["fields"].(map[string]any)
	if !ok {
		t.Fatal("contract missing fields map")
	}
	documentation := mockworkers.ProjectContractFieldDocumentation()
	for _, record := range inventory.Fields {
		field, ok := fields[record.ID].(map[string]any)
		if !ok {
			t.Fatalf("contract missing inventoried field %q", record.ID)
		}
		if field["inventoryId"] != record.ID {
			t.Fatalf("field %q inventoryId = %#v, want %q", record.ID, field["inventoryId"], record.ID)
		}
		if field["jsonPath"] != record.JSONPath {
			t.Fatalf("field %q jsonPath = %#v, want %q", record.ID, field["jsonPath"], record.JSONPath)
		}
		if field["defaultEmptyBehavior"] != record.DefaultEmptyBehavior {
			t.Fatalf("field %q defaultEmptyBehavior = %#v, want %q", record.ID, field["defaultEmptyBehavior"], record.DefaultEmptyBehavior)
		}
		if field["validationOwner"] != record.ValidationOwner {
			t.Fatalf("field %q validationOwner = %#v, want %q", record.ID, field["validationOwner"], record.ValidationOwner)
		}
		if _, ok := documentation[record.ID]; !ok {
			t.Fatalf("ProjectContractFieldDocumentation missing %q", record.ID)
		}
	}
}

func TestMockWorkersSchema_ContractMetadataValidatesThroughCommonVocabulary(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	contract := schema["contract"].(map[string]any)
	fields := contract["fields"].(map[string]any)
	documentation := mockworkers.ProjectContractFieldDocumentation()

	documentationSchema := compileContractSupportSchema(t, documentationSchemaID, [][2]string{
		{documentationSchemaID, "contracts/common/documentation.schema.json"},
	})
	lifecycleSchema := compileContractSupportSchema(t, deprecationsSchemaID, [][2]string{
		{documentationSchemaID, "contracts/common/documentation.schema.json"},
		{deprecationsSchemaID, "contracts/common/deprecations.schema.json"},
	})
	fieldContractSchema := compileMockWorkersFieldContractSchema(t, schema)

	for inventoryID, projected := range documentation {
		t.Run(inventoryID, func(t *testing.T) {
			field, ok := fields[inventoryID].(map[string]any)
			if !ok {
				t.Fatalf("contract missing field %q", inventoryID)
			}
			if err := fieldContractSchema.Validate(field); err != nil {
				t.Fatalf("validate field contract: %v", err)
			}
			if err := documentationSchema.Validate(field["documentation"]); err != nil {
				t.Fatalf("validate documentation metadata: %v", err)
			}
			if err := lifecycleSchema.Validate(field["lifecycle"]); err != nil {
				t.Fatalf("validate lifecycle metadata: %v", err)
			}

			docBlock := field["documentation"].(map[string]any)
			if docBlock["itemId"] != mockworkers.DocumentationItemID(inventoryID) {
				t.Fatalf("documentation.itemId = %#v, want %q", docBlock["itemId"], mockworkers.DocumentationItemID(inventoryID))
			}
			nested := docBlock["documentation"].(map[string]any)
			title := nested["title"].(map[string]any)
			if title["canonicalEnglish"] != projected.Title {
				t.Fatalf("title.canonicalEnglish = %#v, want %q", title["canonicalEnglish"], projected.Title)
			}
			description := nested["description"].(map[string]any)
			if description["canonicalEnglish"] != projected.Description {
				t.Fatalf("description.canonicalEnglish = %#v, want %q", description["canonicalEnglish"], projected.Description)
			}
		})
	}
}

func TestMockWorkersSchema_ContractExamplesReferenceAcceptedDocsShapes(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	contract := schema["contract"].(map[string]any)
	examples, ok := contract["examples"].([]any)
	if !ok || len(examples) == 0 {
		t.Fatal("contract missing examples")
	}
	want := []string{
		"docs/examples/mock-workers.json",
		"docs/examples/mock-workers-script.json",
		"docs/examples/mock-workers-mixed.json",
	}
	for _, path := range want {
		if !containsAnyString(examples, path) {
			t.Fatalf("contract examples = %#v, want %q", examples, path)
		}
	}

	encoded, err := json.Marshal(examples)
	if err != nil {
		t.Fatalf("marshal contract examples: %v", err)
	}
	lower := strings.ToLower(string(encoded))
	for _, forbidden := range []string{`"media"`, `"artifact"`, `"response_sequence"`, `"dispatch_delay"`, `"sleep"`} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("contract examples advertise unsupported capability property %s", forbidden)
		}
	}
}

func TestMockWorkersSchema_PropertyExamplesUseAcceptedShapes(t *testing.T) {
	t.Parallel()

	schema := loadAuthoredMockWorkersSchema(t)
	properties := schema["properties"].(map[string]any)
	mockWorkers := properties["mockWorkers"].(map[string]any)
	if _, ok := mockWorkers["examples"]; !ok {
		t.Fatal("mockWorkers missing examples")
	}
	policy := properties["unmatchedDispatchPolicy"].(map[string]any)
	if policy["default"] != "accept" {
		t.Fatalf("unmatchedDispatchPolicy.default = %#v, want accept", policy["default"])
	}
	if _, ok := policy["examples"]; !ok {
		t.Fatal("unmatchedDispatchPolicy missing examples")
	}
}

func TestMockWorkersSchema_StagedProjectionMatchesAuthoredCopy(t *testing.T) {
	t.Parallel()

	repositoryRoot := testutil.MustRepoPath(t, ".")
	authoredPath := filepath.Join(repositoryRoot, filepath.FromSlash(mockworkers.SchemaRelativePath))
	stagedPath := filepath.Join(repositoryRoot, filepath.FromSlash("packages/api/generated/schemas/mock-workers.schema.json"))

	authored, err := os.ReadFile(authoredPath)
	if err != nil {
		t.Fatalf("read authored schema: %v", err)
	}
	staged, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("read staged schema: %v", err)
	}
	if !bytes.Equal(authored, staged) {
		t.Fatalf("staged mock-workers schema diverges from authored copy; run make contracts-generate")
	}
}

func TestMockWorkersSchema_StaleStagingDetectedByContractCheck(t *testing.T) {
	defer contractstaging.LockRepositoryStagingForTest()()

	repositoryRoot := testutil.MustRepoPath(t, ".")
	target := "packages/api/generated/schemas/mock-workers.schema.json"
	wantStale := []string{target}
	stagedPath := filepath.Join(repositoryRoot, filepath.FromSlash(target))

	before, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("read staged schema: %v", err)
	}
	defer func() {
		_ = os.WriteFile(stagedPath, before, 0o644)
	}()

	corrupted := append(append([]byte(nil), before...), '\n')
	var (
		drift    contractstaging.Drift
		detected bool
	)
	for attempt := 0; attempt < 32; attempt++ {
		if err := os.WriteFile(stagedPath, corrupted, 0o644); err != nil {
			t.Fatalf("write stale staged schema: %v", err)
		}
		drift, err = contractstaging.Check(repositoryRoot)
		if err != nil {
			t.Fatalf("Check() error = %v", err)
		}
		if slices.Equal(drift.Stale, wantStale) {
			detected = true
			break
		}
	}
	if !detected {
		t.Fatalf("Check() stale = %#v, want %#v", drift.Stale, wantStale)
	}
}

func compileContractSupportSchema(t *testing.T, id string, resources [][2]string) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	for _, resource := range resources {
		resourceID := resource[0]
		resourcePath := testutil.MustRepoPath(t, resource[1])
		payload, err := os.ReadFile(resourcePath)
		if err != nil {
			t.Fatalf("read schema %s: %v", resourcePath, err)
		}
		var document any
		if err := json.Unmarshal(payload, &document); err != nil {
			t.Fatalf("decode schema %s: %v", resourcePath, err)
		}
		if err := compiler.AddResource(resourceID, document); err != nil {
			t.Fatalf("add schema resource %s: %v", resourceID, err)
		}
	}
	compiled, err := compiler.Compile(id)
	if err != nil {
		t.Fatalf("compile schema %s: %v", id, err)
	}
	return compiled
}

func compileMockWorkersFieldContractSchema(t *testing.T, schemaRoot map[string]any) *jsonschema.Schema {
	t.Helper()

	defs := schemaRoot["$defs"].(map[string]any)
	fieldContract := defs["fieldContract"]
	wrapped := map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"$id":     "https://schemas.portpowered.com/you/config/mock-workers.field-contract.schema.json",
		"$ref":    "#/$defs/fieldContract",
		"$defs": map[string]any{
			"fieldContract": fieldContract,
		},
	}
	payload, err := json.Marshal(wrapped)
	if err != nil {
		t.Fatalf("marshal field contract schema: %v", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	for _, pair := range [][2]string{
		{documentationSchemaID, "contracts/common/documentation.schema.json"},
		{deprecationsSchemaID, "contracts/common/deprecations.schema.json"},
	} {
		resourcePath := testutil.MustRepoPath(t, pair[1])
		resourcePayload, err := os.ReadFile(resourcePath)
		if err != nil {
			t.Fatalf("read schema %s: %v", resourcePath, err)
		}
		var document any
		if err := json.Unmarshal(resourcePayload, &document); err != nil {
			t.Fatalf("decode schema %s: %v", resourcePath, err)
		}
		if err := compiler.AddResource(pair[0], document); err != nil {
			t.Fatalf("add schema resource %s: %v", pair[0], err)
		}
	}
	wrappedDocument, err := jsonschema.UnmarshalJSON(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("decode field contract schema: %v", err)
	}
	if err := compiler.AddResource(wrapped["$id"].(string), wrappedDocument); err != nil {
		t.Fatalf("add field contract schema: %v", err)
	}
	compiled, err := compiler.Compile(wrapped["$id"].(string))
	if err != nil {
		t.Fatalf("compile field contract schema: %v", err)
	}
	return compiled
}

func containsAnyString(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestMockWorkersSchema_DocsExamplesPassSchemaAndLoader(t *testing.T) {
	t.Parallel()

	schema := compileAuthoredMockWorkersSchema(t)
	for _, fixture := range []string{
		"docs/examples/mock-workers.json",
		"docs/examples/mock-workers-script.json",
		"docs/examples/mock-workers-mixed.json",
	} {
		t.Run(fixture, func(t *testing.T) {
			t.Parallel()

			data := readRepoFixture(t, fixture)
			document := decodeJSONDocument(t, data)
			assertSchemaAccepts(t, schema, document)

			if _, err := mockworkers.ParseMockWorkersConfig(data); err != nil {
				t.Fatalf("ParseMockWorkersConfig() error = %v, want accept", err)
			}

			path := repoFixturePath(t, fixture)
			if _, err := localMockWorkersConfigLoader(t)(path); err != nil {
				t.Fatalf("LoadMockWorkersConfig(%q) error = %v, want accept", path, err)
			}
		})
	}
}

func TestMockWorkersSchema_ValidIndexedCasesAgreeWithLoader(t *testing.T) {
	schema := compileAuthoredMockWorkersSchema(t)
	inventory := mockworkers.ProjectInputInventory()

	for _, inputCase := range inventory.Cases {
		if inputCase.Outcome != "accept" {
			continue
		}

		t.Run(inputCase.ID, func(t *testing.T) {
			switch inputCase.Entrypoint {
			case "ParseMockWorkersConfig":
				assertParseCasePassesSchemaAndLoader(t, schema, inputCase)
			case "LoadMockWorkersConfig":
				assertLoadCasePassesSchemaAndLoader(t, schema, inputCase)
			default:
				t.Fatalf("unsupported entrypoint %q", inputCase.Entrypoint)
			}
		})
	}
}

func assertParseCasePassesSchemaAndLoader(
	t *testing.T,
	schema *jsonschema.Schema,
	inputCase mockworkers.InputCase,
) {
	t.Helper()

	if inputCase.Fixture == "" {
		t.Fatal("parse accept case missing fixture")
	}

	data := readRepoFixture(t, inputCase.Fixture)
	document := decodeJSONDocument(t, data)
	assertSchemaAccepts(t, schema, document)

	cfg, err := mockworkers.ParseMockWorkersConfig(data)
	if err != nil {
		t.Fatalf("ParseMockWorkersConfig() error = %v, want accept", err)
	}
	assertMockWorkersConfigExpectation(t, cfg, inputCase.ExpectedConfig)
}

func assertLoadCasePassesSchemaAndLoader(
	t *testing.T,
	schema *jsonschema.Schema,
	inputCase mockworkers.InputCase,
) {
	t.Helper()

	switch inputCase.ID {
	case "valid-load-empty-path":
		document := decodeJSONDocument(t, []byte(`{"mockWorkers":[]}`))
		assertSchemaAccepts(t, schema, document)

		cfg, err := localMockWorkersConfigLoader(t)("")
		if err != nil {
			t.Fatalf("LoadMockWorkersConfig() error = %v, want accept", err)
		}
		assertMockWorkersConfigExpectation(t, cfg, inputCase.ExpectedConfig)
		return
	}

	if inputCase.Fixture == "" {
		t.Fatal("load accept case missing fixture")
	}

	data := readRepoFixture(t, inputCase.Fixture)
	document := decodeJSONDocument(t, data)
	assertSchemaAccepts(t, schema, document)

	path := repoFixturePath(t, inputCase.Fixture)
	cfg, err := localMockWorkersConfigLoader(t)(path)
	if err != nil {
		t.Fatalf("LoadMockWorkersConfig(%q) error = %v, want accept", path, err)
	}
	assertMockWorkersConfigExpectation(t, cfg, inputCase.ExpectedConfig)
}

func decodeJSONDocument(t *testing.T, data []byte) any {
	t.Helper()

	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("decode json document: %v", err)
	}
	return document
}

func assertSchemaAccepts(t *testing.T, schema *jsonschema.Schema, document any) {
	t.Helper()

	if err := schema.Validate(document); err != nil {
		t.Fatalf("schema validation error = %v, want accept", err)
	}
}

func TestMockWorkersSchema_InvalidIndexedCasesAgreeWithLoaderOnRejectPaths(t *testing.T) {
	schema := compileAuthoredMockWorkersSchema(t)
	inventory := mockworkers.ProjectInputInventory()

	for _, inputCase := range inventory.Cases {
		if inputCase.Outcome != "reject" {
			continue
		}

		t.Run(inputCase.ID, func(t *testing.T) {
			if inputCase.Entrypoint != "ParseMockWorkersConfig" {
				t.Fatalf("unsupported entrypoint %q", inputCase.Entrypoint)
			}
			if inputCase.Fixture == "" {
				t.Fatal("reject case missing fixture")
			}

			data := readRepoFixture(t, inputCase.Fixture)
			schemaErr := validateAuthoredMockWorkersDocument(schema, data)
			if schemaErr == nil {
				t.Fatal("schema validation error = nil, want reject")
			}

			_, loaderErr := mockworkers.ParseMockWorkersConfig(data)
			if loaderErr == nil {
				t.Fatal("ParseMockWorkersConfig() error = nil, want reject")
			}
			assertErrorFragments(t, loaderErr, inputCase.ErrorFragments)

			want := expectedInvalidParity(inputCase.ID)
			assertRejectPathsAgree(t, inputCase.ID, schemaErr, loaderErr, want)
		})
	}
}

type invalidParityExpectation struct {
	semanticField string
	documentLevel bool
}

func expectedInvalidParity(caseID string) invalidParityExpectation {
	switch caseID {
	case "invalid-unknown-top-level":
		return invalidParityExpectation{semanticField: "unexpectedTopLevel"}
	case "invalid-unknown-nested-mock-worker":
		return invalidParityExpectation{semanticField: "unexpectedNested"}
	case "invalid-trailing-json":
		return invalidParityExpectation{documentLevel: true}
	case "invalid-unknown-run-type":
		return invalidParityExpectation{semanticField: "runType"}
	case "invalid-unknown-unmatched-policy":
		return invalidParityExpectation{semanticField: "unmatchedDispatchPolicy"}
	case "invalid-script-without-script-config":
		return invalidParityExpectation{semanticField: "scriptConfig"}
	case "invalid-script-without-command":
		return invalidParityExpectation{semanticField: "command"}
	case "invalid-reject-exit-code-out-of-range":
		return invalidParityExpectation{semanticField: "exitCode"}
	default:
		return invalidParityExpectation{}
	}
}

func validateAuthoredMockWorkersDocument(schema *jsonschema.Schema, data []byte) error {
	var document any
	if err := json.Unmarshal(data, &document); err != nil {
		return err
	}
	return schema.Validate(document)
}

func assertRejectPathsAgree(
	t *testing.T,
	caseID string,
	schemaErr error,
	loaderErr error,
	want invalidParityExpectation,
) {
	t.Helper()

	if want.documentLevel {
		if !isDocumentLevelSchemaReject(schemaErr) {
			t.Fatalf("schema error = %v, want document-level reject", schemaErr)
		}
		if !strings.Contains(loaderErr.Error(), "unexpected trailing JSON") {
			t.Fatalf("loader error = %q, want trailing JSON diagnostic", loaderErr.Error())
		}
		return
	}

	if want.semanticField == "" {
		t.Fatalf("missing invalid parity expectation for case %q", caseID)
	}

	schemaPaths := schemaValidationPaths(t, schemaErr)
	schemaMentionsField := schemaErrorMentionsField(schemaErr, want.semanticField)
	if !schemaMentionsSemanticField(schemaPaths, want.semanticField) && !schemaMentionsField {
		t.Fatalf("schema paths = %v and error = %v, want semantic field %q", schemaPaths, schemaErr, want.semanticField)
	}
	if !loaderErrorMentionsSemanticField(loaderErr.Error(), want.semanticField) {
		t.Fatalf("loader error = %q, want semantic field %q", loaderErr.Error(), want.semanticField)
	}
}

func isDocumentLevelSchemaReject(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	return strings.Contains(message, "after top-level value") ||
		strings.Contains(message, "unexpected trailing JSON")
}

func schemaValidationPaths(t *testing.T, err error) []string {
	t.Helper()

	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		return nil
	}
	paths := make([]string, 0)
	collectValidationPaths(validationErr, &paths)
	return paths
}

func collectValidationPaths(err *jsonschema.ValidationError, paths *[]string) {
	if len(err.Causes) == 0 {
		*paths = append(*paths, jsonPointer(err.InstanceLocation))
		return
	}
	for _, cause := range err.Causes {
		collectValidationPaths(cause, paths)
	}
}

func jsonPointer(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = strings.NewReplacer("~", "~0", "/", "~1").Replace(segment)
	}
	return "/" + strings.Join(escaped, "/")
}

func schemaMentionsSemanticField(paths []string, field string) bool {
	for _, path := range paths {
		if path == "" {
			continue
		}
		if strings.HasSuffix(path, "/"+field) || strings.Contains(path, "/"+field+"/") {
			return true
		}
	}
	return false
}

func schemaErrorMentionsField(err error, field string) bool {
	message := err.Error()
	return strings.Contains(message, "additional properties '"+field+"' not allowed") ||
		strings.Contains(message, "missing property '"+field+"'")
}

func loaderErrorMentionsSemanticField(message string, field string) bool {
	switch field {
	case "unexpectedTopLevel", "unexpectedNested":
		return strings.Contains(message, `unknown field "`+field+`"`)
	case "runType", "unmatchedDispatchPolicy", "scriptConfig", "command", "exitCode":
		return strings.Contains(message, field)
	default:
		return strings.Contains(message, field)
	}
}
