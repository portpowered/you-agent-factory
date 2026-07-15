package mockworkers_test

import (
	"encoding/json"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/mockworkers"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

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
			if _, err := mockworkers.LoadMockWorkersConfig(path); err != nil {
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

		cfg, err := mockworkers.LoadMockWorkersConfig("")
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
	cfg, err := mockworkers.LoadMockWorkersConfig(path)
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
