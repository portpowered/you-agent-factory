package openapitests

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/globalconfiginventory"
	"github.com/portpowered/infinite-you/pkg/testutil"
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

// productionBoundarySources records Factory schema, OpenAPI fragment, mapping,
// and generated-client files this inventory lane must not alter. Update hashes
// only when intentionally changing boundary behavior outside this lane.
var productionBoundarySources = []struct {
	relativePath string
	sha256Hex    string
}{
	{
		relativePath: "pkg/config/openapi_factory.go",
		sha256Hex:    "fd2750ac9c16b7f818162c2af6b05cbcf8da2847deb824748dba00feb2419aba",
	},
	{
		relativePath: "pkg/config/factory_config_mapping.go",
		sha256Hex:    "c629d624b6da772c02b96d394ea77f206495b8e28d90f6ce42f3f79e68b9200c",
	},
	{
		relativePath: "pkg/config/factory_config_mapping_internal.go",
		sha256Hex:    "71aac265e4ddb0997735e1691ec407227584d7e81cbd2423da4fab45ca6508b0",
	},
	{
		relativePath: "pkg/interfaces/factory_config.go",
		sha256Hex:    "3287ccc0708b7e92fb69772da05c86a9c87d4c27301cb318a6c1f85acca2cef9",
	},
	{
		relativePath: "api/components/schemas/data-models/Factory.yaml",
		sha256Hex:    "af7cbb1c6d33c3bd2f8af6d99105683ffa072038aa3c7e5671d7aa3e8a2e31b3",
	},
	{
		relativePath: "api/components/schemas/data-models/WorkerType.yaml",
		sha256Hex:    "9d50ae595e3274ca543265b7765a1135c3ae6f6be918a72875b2f5ab7dadb477",
	},
	{
		relativePath: "api/components/schemas/data-models/WorkstationType.yaml",
		sha256Hex:    "415584f86f33599b6d1676f97f3d442ae91cd8010f48c5496f079da9954b14ea",
	},
	{
		relativePath: "api/components/schemas/data-models/FactoryGuard.yaml",
		sha256Hex:    "db2adae1dee03fa54073ad6bdb9958707cd1885130389f52dedf7c7f905e590b",
	},
	{
		relativePath: "api/components/schemas/data-models/FactoryLayout.yaml",
		sha256Hex:    "f5b867440c883b88838197fc42d67f3efaf17ba76ac36eb802c38a428377dc73",
	},
	{
		relativePath: "api/components/schemas/data-models/FactoryOrchestrator.yaml",
		sha256Hex:    "46717f730e2b864a1c1979bc41ec2720b14cd7fe6cd73ad8025a18c64c8e39a6",
	},
	{
		relativePath: "api/components/schemas/data-models/Resource.yaml",
		sha256Hex:    "56099e658940395035851cfcce7424782ef9e42193b8eb608b93b45d5866ab28",
	},
	{
		relativePath: "pkg/api/generated/server.gen.go",
		sha256Hex:    "e19e1c9af8f878ef11aede6f6604fb40e2ff58c95c87d956f09cf226c8889e12",
	},
	{
		relativePath: "pkg/generatedclient/client.gen.go",
		sha256Hex:    "2ad86aba1ef76f69b1bfdeee0d02cfc4398ebab95b556dfc01cec778eb9f8233",
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
