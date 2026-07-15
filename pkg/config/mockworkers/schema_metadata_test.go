package mockworkers_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractstaging"
	"github.com/portpowered/infinite-you/pkg/config/mockworkers"
	"github.com/portpowered/infinite-you/pkg/testutil"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	documentationSchemaID = "https://schemas.portpowered.com/you/contracts/common/documentation.schema.json"
	deprecationsSchemaID  = "https://schemas.portpowered.com/you/contracts/common/deprecations.schema.json"
)

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
	repositoryRoot := testutil.MustRepoPath(t, ".")
	target := "packages/api/generated/schemas/mock-workers.schema.json"
	stagedPath := filepath.Join(repositoryRoot, filepath.FromSlash(target))

	before, err := os.ReadFile(stagedPath)
	if err != nil {
		t.Fatalf("read staged schema: %v", err)
	}
	if err := os.WriteFile(stagedPath, append(before, '\n'), 0o644); err != nil {
		t.Fatalf("write stale staged schema: %v", err)
	}
	t.Cleanup(func() {
		_ = os.WriteFile(stagedPath, before, 0o644)
	})

	drift, err := contractstaging.Check(repositoryRoot)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if len(drift.Stale) != 1 || drift.Stale[0] != target {
		t.Fatalf("Check() stale = %#v, want [%q]", drift.Stale, target)
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
