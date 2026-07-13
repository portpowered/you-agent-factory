package openapitests

import (
	"bytes"
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
