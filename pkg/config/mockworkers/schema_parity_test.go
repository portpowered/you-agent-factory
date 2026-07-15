package mockworkers_test

import (
	"encoding/json"
	"errors"
	"strings"
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
