package openapitests

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil"
	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/globalconfiginventory"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const parityFixturesRelativeDir = "pkg/config/openapitests/testdata/fixtures"

func TestProjectParityInventory_RecordsFactoryOpenAPIScope(t *testing.T) {
	t.Parallel()

	inventory := ProjectParityInventory()
	if inventory.FormatVersion != ParityInventoryFormatVersion {
		t.Fatalf("FormatVersion = %q, want %q", inventory.FormatVersion, ParityInventoryFormatVersion)
	}
	if !strings.Contains(inventory.Scope, "GeneratedFactoryFromOpenAPIJSON") {
		t.Fatalf("scope = %q, want generated factory entrypoint reference", inventory.Scope)
	}
	if !strings.Contains(inventory.Scope, "FactoryConfigFromOpenAPIJSON") {
		t.Fatalf("scope = %q, want config-loader entrypoint reference", inventory.Scope)
	}
}

func TestProjectParityInventory_CoversRepresentativeFactoryShapes(t *testing.T) {
	t.Parallel()

	requiredShapes := []string{
		shapeOrchestrator,
		shapeWorkstation,
		shapeWorker,
		shapeResource,
		shapeGuard,
		shapeLayout,
	}
	covered := make(map[string]struct{}, len(requiredShapes))
	for _, parityCase := range ProjectParityInventory().Cases {
		covered[parityCase.Shape] = struct{}{}
	}
	for _, shape := range requiredShapes {
		if _, ok := covered[shape]; !ok {
			t.Fatalf("missing representative parity case for shape %q", shape)
		}
	}
}

func TestProjectParityInventory_CoversRepresentativeShapeAcceptRejectPairs(t *testing.T) {
	t.Parallel()

	requiredShapes := []string{
		shapeOrchestrator,
		shapeWorkstation,
		shapeWorker,
		shapeResource,
		shapeGuard,
		shapeLayout,
	}
	acceptByShape := make(map[string]struct{}, len(requiredShapes))
	rejectByShape := make(map[string]struct{}, len(requiredShapes))
	for _, parityCase := range ProjectParityInventory().Cases {
		switch parityCase.APIOutcome {
		case outcomeAccept:
			acceptByShape[parityCase.Shape] = struct{}{}
		case outcomeReject:
			rejectByShape[parityCase.Shape] = struct{}{}
		}
	}
	for _, shape := range requiredShapes {
		if _, ok := acceptByShape[shape]; !ok {
			t.Fatalf("missing accept parity case for shape %q", shape)
		}
		if _, ok := rejectByShape[shape]; !ok {
			t.Fatalf("missing reject parity case for shape %q", shape)
		}
	}
}

func TestProjectParityInventory_IndexesRepresentativeUnionAndEnumCases(t *testing.T) {
	t.Parallel()

	requiredCategories := []string{
		categoryTaxonomyEnum,
		categoryBoundaryEnum,
		categoryGuardUnion,
		categoryLayoutContract,
	}
	covered := make(map[string]struct{}, len(requiredCategories))
	for _, parityCase := range ProjectParityInventory().Cases {
		covered[parityCase.Category] = struct{}{}
	}
	for _, category := range requiredCategories {
		if _, ok := covered[category]; !ok {
			t.Fatalf("missing representative parity case for category %q", category)
		}
	}
}

func TestProjectParityInventory_HasStableCaseIDsAndFixtureLocators(t *testing.T) {
	t.Parallel()

	inventory := ProjectParityInventory()
	seen := make(map[string]struct{}, len(inventory.Cases))
	for _, parityCase := range inventory.Cases {
		if parityCase.ID == "" {
			t.Fatal("parity case missing id")
		}
		if _, exists := seen[parityCase.ID]; exists {
			t.Fatalf("duplicate parity case id %q", parityCase.ID)
		}
		seen[parityCase.ID] = struct{}{}

		if parityCase.Fixture == "" {
			t.Fatalf("parity case %q missing fixture locator", parityCase.ID)
		}
		if parityCase.SourceTest == "" {
			t.Fatalf("parity case %q missing source test locator", parityCase.ID)
		}
		if parityCase.APIOutcome != outcomeAccept && parityCase.APIOutcome != outcomeReject {
			t.Fatalf("parity case %q apiOutcome = %q, want accept or reject", parityCase.ID, parityCase.APIOutcome)
		}
		if parityCase.LoaderOutcome != outcomeAccept && parityCase.LoaderOutcome != outcomeReject {
			t.Fatalf("parity case %q loaderOutcome = %q, want accept or reject", parityCase.ID, parityCase.LoaderOutcome)
		}
		if parityCase.APIOutcome == outcomeReject || parityCase.LoaderOutcome == outcomeReject {
			if parityCase.ExpectedErrorCategory == "" {
				t.Fatalf("parity case %q missing expectedErrorCategory", parityCase.ID)
			}
			if len(parityCase.ErrorFragments) == 0 {
				t.Fatalf("parity case %q missing errorFragments", parityCase.ID)
			}
		}
	}
}

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

func TestMarshalParityInventoryJSON_IsByteIdenticalAcrossRepeatedProjections(t *testing.T) {
	t.Parallel()

	first := ProjectParityInventory()
	second := ProjectParityInventory()

	firstJSON, err := MarshalParityInventoryJSON(first)
	if err != nil {
		t.Fatalf("first MarshalParityInventoryJSON() error = %v", err)
	}
	secondJSON, err := MarshalParityInventoryJSON(second)
	if err != nil {
		t.Fatalf("second MarshalParityInventoryJSON() error = %v", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("repeated factory openapi parity inventory json differs")
	}
	if firstJSON[len(firstJSON)-1] != '\n' {
		t.Fatalf("factory openapi parity inventory json missing trailing newline")
	}
}

func TestProjectParityInventory_MatchesCommittedBaseline(t *testing.T) {
	inventory := ProjectParityInventory()
	got, err := MarshalParityInventoryJSON(inventory)
	if err != nil {
		t.Fatalf("MarshalParityInventoryJSON() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, ParityIndexBaselineRelativePath)
	want, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read baseline fixture %s: %v", fixturePath, err)
	}
	want = globalconfiginventory.NormalizeFixtureBytes(want)
	if bytes.Equal(got, want) {
		return
	}

	t.Fatalf(
		"factory openapi parity index baseline drift detected; update %s when intentional\nwant %d bytes, got %d bytes",
		ParityIndexBaselineRelativePath,
		len(want),
		len(got),
	)
}

func TestWriteFactoryOpenAPIParityIndexBaseline(t *testing.T) {
	if os.Getenv("WRITE_OPENAPI_PARITY_BASELINE") != "1" {
		t.Skip("set WRITE_OPENAPI_PARITY_BASELINE=1 to regenerate baseline fixture")
	}

	inventory := ProjectParityInventory()
	got, err := MarshalParityInventoryJSON(inventory)
	if err != nil {
		t.Fatalf("MarshalParityInventoryJSON() error = %v", err)
	}

	fixturePath := testutil.MustRepoPath(t, ParityIndexBaselineRelativePath)
	if err := os.WriteFile(fixturePath, got, 0o644); err != nil {
		t.Fatalf("write baseline fixture %s: %v", fixturePath, err)
	}
}

// productionBoundarySources records the handwritten Factory schema, OpenAPI
// fragments, and mappings this inventory lane must not alter. Generated API
// artifacts are protected by the generation drift gate and consumer compile
// tests instead of source hashes.
var productionBoundarySources = []struct {
	relativePath string
	sha256Hex    string
}{
	{
		relativePath: "pkg/config/openapi_factory.go",
		sha256Hex:    "2e2709e8a15fcd509889e1b788d14b4ca7ab59904485c87069695417eaea6e8b",
	},
	{
		relativePath: "pkg/config/factory_config_mapping.go",
		sha256Hex:    "b1f4960ff751d29734dd7bd323b57c89130154296cc4bb5da14768ed1ce0e368",
	},
	{
		relativePath: "pkg/config/factory_config_mapping_internal.go",
		sha256Hex:    "d85fa445f98a445915668adf685b086e7ea9d579b587f96a2337d0001fb9623d",
	},
	{
		relativePath: "pkg/factory/contracts/factory_config.go",
		sha256Hex:    "986b5388f1d3025026c54d920d5d19987e46d9388c5d428a892522227446bdbc",
	},
	{
		relativePath: "api/components/schemas/data-models/Factory.yaml",
		sha256Hex:    "bf3b2eb5709d1b4828bb6c0542b22ff50fb1cfc56ab24cf02ae8f546388a9304",
	},
	{
		relativePath: "api/components/schemas/data-models/WorkerType.yaml",
		sha256Hex:    "8f559a3646c66ac4e08eabd72edb7dea7eeec81f1a165169abc1879c3b46fe57",
	},
	{
		relativePath: "api/components/schemas/data-models/WorkstationType.yaml",
		sha256Hex:    "9eb93d0f1f16f1baf00a1441d111bfa7d57e76f4d80795cae35376fe8632ddde",
	},
	{
		relativePath: "api/components/schemas/data-models/FactoryGuard.yaml",
		sha256Hex:    "da72ecfe42451c48100348d8161f4511fcf208ce46adbae4b147e2b766d8f793",
	},
	{
		relativePath: "api/components/schemas/data-models/FactoryLayout.yaml",
		sha256Hex:    "32709e48f177114300e4d128e5c869d0efc840b020eafd06741d200886c57566",
	},
	{
		relativePath: "api/components/schemas/data-models/FactoryOrchestrator.yaml",
		sha256Hex:    "08669ab5b1aa7af5c55185a9ec5827959dac35d51f0c0098732852a21d18d84c",
	},
	{
		relativePath: "api/components/schemas/data-models/Resource.yaml",
		sha256Hex:    "e19cd17f7daddfcedc7c46dffb75dced03b2021f8ce33543a88f4a105df9632b",
	},
}

var parityInventoryLaneRoots = []string{
	"pkg/config/openapitests/testdata",
}

func TestProductionBoundarySources_UnchangedForParityLane(t *testing.T) {
	t.Parallel()

	for _, src := range productionBoundarySources {
		src := src
		t.Run(src.relativePath, func(t *testing.T) {
			t.Parallel()

			path := testutil.MustRepoPath(t, src.relativePath)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read production boundary source %s: %v", path, err)
			}
			data = globalconfiginventory.NormalizeSourceBytes(data)

			sum := sha256.Sum256(data)
			got := hex.EncodeToString(sum[:])
			if got != src.sha256Hex {
				t.Fatalf(
					"production boundary source drift detected for %s; update lane gate hashes only when intentionally changing schema, mapping, or generated clients\ngot %s, want %s",
					src.relativePath,
					got,
					src.sha256Hex,
				)
			}
		})
	}
}

func TestFactoryOpenAPIParityLane_DoesNotAuthorDraft202012Schemas(t *testing.T) {
	t.Parallel()

	repoRoot := testutil.MustRepoPath(t, ".")
	for _, root := range parityInventoryLaneRoots {
		root := root
		t.Run(root, func(t *testing.T) {
			t.Parallel()

			scanRoot := filepath.Join(repoRoot, filepath.FromSlash(root))
			err := filepath.WalkDir(scanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
					return nil
				}

				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				lower := strings.ToLower(string(data))
				if strings.Contains(lower, `"$schema"`) && strings.Contains(lower, "draft-2020-12") {
					t.Fatalf("draft-2020-12 schema document found in factory openapi parity lane: %s", path)
				}
				return nil
			})
			if err != nil {
				t.Fatalf("walk factory openapi parity lane root %s: %v", scanRoot, err)
			}
		})
	}
}

func TestFactoryOpenAPIParityLane_DoesNotIndexMockWorkerOrJSCallSurfaces(t *testing.T) {
	t.Parallel()

	scope := strings.ToLower(ProjectParityInventory().Scope)
	for _, forbidden := range []string{
		"mock-worker inventory",
		"mock worker inventory",
		"js-call inventory",
		"js call inventory",
		"javascript call inventory",
	} {
		if strings.Contains(scope, forbidden) {
			t.Fatalf("parity inventory scope must not claim %q inventory: %q", forbidden, scope)
		}
	}

	for _, parityCase := range ProjectParityInventory().Cases {
		lowerID := strings.ToLower(parityCase.ID)
		lowerCategory := strings.ToLower(parityCase.Category)
		for _, forbidden := range []string{"mock-worker", "mockworker", "js-call", "jscall"} {
			if strings.Contains(lowerID, forbidden) || strings.Contains(lowerCategory, forbidden) {
				t.Fatalf("parity case %q must not index %q surfaces", parityCase.ID, forbidden)
			}
		}
	}

	repoRoot := testutil.MustRepoPath(t, ".")
	for _, root := range parityInventoryLaneRoots {
		scanRoot := filepath.Join(repoRoot, filepath.FromSlash(root))
		err := filepath.WalkDir(scanRoot, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				return nil
			}
			lowerName := strings.ToLower(entry.Name())
			for _, forbidden := range []string{
				"mock-worker-inventory",
				"mockworker-inventory",
				"js-call-inventory",
				"jscall-inventory",
			} {
				if strings.Contains(lowerName, forbidden) {
					t.Fatalf("factory openapi parity lane must not start %q inventory artifacts: %s", forbidden, path)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk factory openapi parity lane root %s: %v", scanRoot, err)
		}
	}
}

func TestFactoryOpenAPIParityLane_RepeatedSerializationIsByteIdentical(t *testing.T) {
	t.Parallel()

	first, err := MarshalParityInventoryJSON(ProjectParityInventory())
	if err != nil {
		t.Fatalf("first MarshalParityInventoryJSON() error = %v", err)
	}
	second, err := MarshalParityInventoryJSON(ProjectParityInventory())
	if err != nil {
		t.Fatalf("second MarshalParityInventoryJSON() error = %v", err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("repeated factory openapi parity inventory json differs")
	}
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

func TestProjectedFactorySchemaParityMatrixCoversAllIndexedCases(t *testing.T) {
	t.Parallel()

	for _, parityCase := range ProjectParityInventory().Cases {
		assertFactorySchemaParityRuleRegistered(t, parityCase.ID)
	}
}

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

func assertFactorySchemaParityRuleRegistered(t *testing.T, caseID string) {
	t.Helper()

	if _, ok := factorySchemaParityOverrides[caseID]; ok {
		return
	}
	for _, parityCase := range ProjectParityInventory().Cases {
		if parityCase.ID == caseID && parityCase.Fixture != "" {
			return
		}
	}
	t.Fatalf("missing factory schema parity rule for indexed case %q", caseID)
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
