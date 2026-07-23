package openapitests

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	. "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const parityFixturesRelativeDir = "pkg/transports/mapping/factoryconfig/openapitests/testdata/fixtures"

func TestIndexedParityCases_MatchDocumentedBoundaryOutcomes(t *testing.T) {
	inventory := ProjectParityInventory()
	for _, parityCase := range inventory.Cases {
		t.Run(parityCase.ID, func(t *testing.T) {
			runIndexedParityCase(t, parityCase)
		})
	}
}

func runIndexedParityCase(t *testing.T, parityCase ParityCase) {
	t.Helper()

	payload := readParityFixture(t, parityCase.Fixture)
	assertParityOutcome(t, parityCase.ID, entrypointGeneratedFactory, parityCase.APIOutcome, func() error {
		_, err := GeneratedFactoryFromOpenAPIJSON(payload)
		return err
	}, parityCase)
	assertParityOutcome(t, parityCase.ID, entrypointFactoryConfig, parityCase.LoaderOutcome, func() error {
		_, err := FactoryConfigFromOpenAPIJSON(payload)
		return err
	}, parityCase)
}

func assertParityOutcome(
	t *testing.T,
	caseID string,
	entrypoint string,
	wantOutcome string,
	run func() error,
	parityCase ParityCase,
) {
	t.Helper()

	err := run()
	switch wantOutcome {
	case outcomeAccept:
		if err != nil {
			t.Fatalf("%s %s() error = %v, want accept", caseID, entrypoint, err)
		}
	case outcomeReject:
		if err == nil {
			t.Fatalf("%s %s() error = nil, want reject", caseID, entrypoint)
		}
		for _, fragment := range parityCase.ErrorFragments {
			if !strings.Contains(err.Error(), fragment) {
				t.Fatalf("%s %s() error = %v, want fragment %q", caseID, entrypoint, err, fragment)
			}
		}
		if parityCase.ExpectedErrorPath != "" && !strings.Contains(err.Error(), parityCase.ExpectedErrorPath) {
			t.Fatalf("%s %s() error = %v, want path %q", caseID, entrypoint, err, parityCase.ExpectedErrorPath)
		}
	default:
		t.Fatalf("%s %s() unsupported outcome %q", caseID, entrypoint, wantOutcome)
	}
}

func readParityFixture(t *testing.T, rel string) []byte {
	t.Helper()

	path := testutil.MustRepoPath(t, filepath.Join(parityFixturesRelativeDir, rel))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read parity fixture %s: %v", rel, err)
	}
	return data
}

const (
	factorySchemaRelativePath = "contracts/config/factory.schema.json"
	factorySchemaID           = "https://schemas.portpowered.com/you/config/factory.schema.json"
	entrypointProjectedSchema = "ProjectedFactorySchema"
)

type factorySchemaParityRule struct {
	kind         factorySchemaParityKind
	schemaAccept *bool
	reason       string
}

type factorySchemaParityKind string

const (
	factorySchemaParityMatchesLoader factorySchemaParityKind = "matches-loader"
	factorySchemaParityNotApplicable factorySchemaParityKind = "not-applicable"
	factorySchemaParityDiverges      factorySchemaParityKind = "diverges"
)

var factorySchemaParityOverrides = map[string]factorySchemaParityRule{}

func TestIndexedParityCases_MatchProjectedFactorySchema(t *testing.T) {
	schema := projectedFactorySchema(t)

	for _, parityCase := range ProjectParityInventory().Cases {
		t.Run(parityCase.ID, func(t *testing.T) {
			runProjectedFactorySchemaParityCase(t, schema, parityCase)
		})
	}
}

func runProjectedFactorySchemaParityCase(t *testing.T, schema *jsonschema.Schema, parityCase ParityCase) {
	t.Helper()

	payload := readParityFixture(t, parityCase.Fixture)
	rule := factorySchemaParityRuleForCase(parityCase)

	switch rule.kind {
	case factorySchemaParityNotApplicable:
		return
	case factorySchemaParityDiverges:
		if rule.schemaAccept == nil {
			t.Fatal("divergent factory schema parity rule missing schemaAccept")
		}
		assertProjectedSchemaParityForFixture(t, schema, rule, payload, parityCase, *rule.schemaAccept)
		return
	case factorySchemaParityMatchesLoader:
		wantAccept := projectedSchemaWantAccept(t, parityCase)
		assertProjectedSchemaParityForFixture(t, schema, rule, payload, parityCase, wantAccept)
		assertParityOutcome(t, parityCase.ID, entrypointGeneratedFactory, parityCase.APIOutcome, func() error {
			_, err := GeneratedFactoryFromOpenAPIJSON(payload)
			return err
		}, parityCase)
		assertParityOutcome(t, parityCase.ID, entrypointFactoryConfig, parityCase.LoaderOutcome, func() error {
			_, err := FactoryConfigFromOpenAPIJSON(payload)
			return err
		}, parityCase)
	default:
		t.Fatalf("unsupported factory schema parity kind %q", rule.kind)
	}
}

func assertProjectedSchemaParityForFixture(
	t *testing.T,
	schema *jsonschema.Schema,
	rule factorySchemaParityRule,
	payload []byte,
	parityCase ParityCase,
	wantAccept bool,
) {
	t.Helper()

	instance, ok := parseFactoryJSONDocument(payload)
	if !ok {
		t.Fatalf("%s projected schema parity requires parseable JSON fixture", parityCase.ID)
	}

	err := schema.Validate(instance)
	gotAccept := err == nil

	switch rule.kind {
	case factorySchemaParityNotApplicable:
		return
	case factorySchemaParityDiverges:
		if rule.schemaAccept == nil {
			t.Fatal("divergent factory schema parity rule missing schemaAccept")
		}
		if gotAccept != *rule.schemaAccept {
			t.Fatalf(
				"%s %s() accept = %t, want %t (%s)",
				parityCase.ID,
				entrypointProjectedSchema,
				gotAccept,
				*rule.schemaAccept,
				rule.reason,
			)
		}
		return
	case factorySchemaParityMatchesLoader:
		if gotAccept != wantAccept {
			if wantAccept {
				t.Fatalf("%s %s() error = %v, want accept", parityCase.ID, entrypointProjectedSchema, err)
			}
			t.Fatalf("%s %s() error = nil, want reject", parityCase.ID, entrypointProjectedSchema)
		}
		if !wantAccept && parityCase.ExpectedErrorPath != "" {
			assertProjectedSchemaValidationPath(t, parityCase, err)
		}
	default:
		t.Fatalf("unsupported factory schema parity kind %q", rule.kind)
	}
}

func projectedSchemaWantAccept(t *testing.T, parityCase ParityCase) bool {
	t.Helper()

	if parityCase.APIOutcome == outcomeAccept && parityCase.LoaderOutcome == outcomeAccept {
		return true
	}
	if parityCase.APIOutcome == outcomeReject && parityCase.LoaderOutcome == outcomeReject {
		return false
	}
	t.Fatalf(
		"parity case %q has mixed API/loader outcomes (%q vs %q); projected schema parity requires agreement",
		parityCase.ID,
		parityCase.APIOutcome,
		parityCase.LoaderOutcome,
	)
	return false
}

func assertProjectedSchemaValidationPath(t *testing.T, parityCase ParityCase, err error) {
	t.Helper()

	paths := projectedSchemaValidationPaths(t, err)
	wantPath := dottedPathToJSONPointer(parityCase.ExpectedErrorPath)
	for _, got := range paths {
		if pathsEquivalent(got, wantPath) {
			return
		}
	}
	t.Fatalf(
		"%s %s() validation paths = %v, want actionable path equivalent to %q (%s)",
		parityCase.ID,
		entrypointProjectedSchema,
		paths,
		parityCase.ExpectedErrorPath,
		parityCase.ExpectedErrorCategory,
	)
}

func projectedSchemaValidationPaths(t *testing.T, err error) []string {
	t.Helper()

	var validationErr *jsonschema.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("projected schema validation returned %T, want *jsonschema.ValidationError", err)
	}
	paths := make([]string, 0)
	collectProjectedSchemaValidationPaths(validationErr, &paths)
	return paths
}

func collectProjectedSchemaValidationPaths(err *jsonschema.ValidationError, paths *[]string) {
	if len(err.Causes) == 0 {
		*paths = append(*paths, jsonPointerFromSegments(err.InstanceLocation))
		return
	}
	for _, cause := range err.Causes {
		collectProjectedSchemaValidationPaths(cause, paths)
	}
}

func jsonPointerFromSegments(segments []string) string {
	if len(segments) == 0 {
		return ""
	}
	escaped := make([]string, len(segments))
	for i, segment := range segments {
		escaped[i] = strings.NewReplacer("~", "~0", "/", "~1").Replace(segment)
	}
	return "/" + strings.Join(escaped, "/")
}

func dottedPathToJSONPointer(path string) string {
	if path == "" {
		return ""
	}
	var builder strings.Builder
	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '.':
			builder.WriteByte('/')
		case '[':
			builder.WriteByte('/')
		case ']':
			continue
		default:
			builder.WriteByte(path[i])
		}
	}
	return "/" + builder.String()
}

func pathsEquivalent(got, want string) bool {
	if got == want {
		return true
	}
	if want == "" {
		return false
	}
	return strings.HasPrefix(got, want) || strings.HasPrefix(want, got)
}

func factorySchemaParityRuleForCase(parityCase ParityCase) factorySchemaParityRule {
	if override, ok := factorySchemaParityOverrides[parityCase.ID]; ok {
		return override
	}
	if parityCase.Fixture == "" {
		return factorySchemaParityRule{
			kind:   factorySchemaParityNotApplicable,
			reason: "indexed case has no JSON fixture document",
		}
	}
	return factorySchemaParityRule{
		kind: factorySchemaParityMatchesLoader,
	}
}

func projectedFactorySchema(t *testing.T) *jsonschema.Schema {
	t.Helper()

	path := testutil.MustRepoPath(t, factorySchemaRelativePath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema %s: %v", path, err)
	}
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode schema %s: %v", path, err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(factorySchemaID, document); err != nil {
		t.Fatalf("add schema resource: %v", err)
	}
	compiled, err := compiler.Compile(factorySchemaID)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return compiled
}

func parseFactoryJSONDocument(data []byte) (any, bool) {
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return nil, false
	}
	return document, true
}
