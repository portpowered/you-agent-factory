package functionaltestviz_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
	"github.com/portpowered/infinite-you/internal/functionaltestviz"
)

func TestRenderCatalogMarkdownFullReportGolden(t *testing.T) {
	t.Parallel()

	inputs := functionaltestviz.CatalogInputs{
		Records:  fullReportFixtureRecords(),
		Coverage: fullReportCoverageSummary(),
	}

	first, err := functionaltestviz.RenderCatalogMarkdown(inputs)
	if err != nil {
		t.Fatalf("RenderCatalogMarkdown first: %v", err)
	}
	second, err := functionaltestviz.RenderCatalogMarkdown(inputs)
	if err != nil {
		t.Fatalf("RenderCatalogMarkdown second: %v", err)
	}
	if first != second {
		t.Fatalf("repeated full-report renders diverged:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	assertFullReportCoversRepresentativeSections(t, first)

	goldenPath := filepath.Join("testdata", "full-report", "functional-tests.md")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	if !bytes.Equal([]byte(first), golden) {
		t.Fatalf("full-report markdown differs from golden %s:\ngot:\n%s\nwant:\n%s", goldenPath, first, golden)
	}
}

func fullReportFixtureRecords() []functionaltestviz.ClassifiedRecord {
	records := functionaltestviz.ClassifyRecords([]functionaltestmetadata.Record{
		{
			File:           "transport/cli/process/help_test.go",
			Package:        "process",
			Name:           "TestHelp",
			Line:           12,
			Description:    "verifies help output",
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
		{
			File:           "workers/inference/openai/invoke_long_test.go",
			Package:        "openai",
			Name:           "TestInvoke",
			Line:           40,
			Description:    "verifies provider invoke replay",
			BuildTags:      []string{"functionallong"},
			Golden:         "testdata/goldens/openai/invoke/manifest.json",
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
		{
			File:           "factory/definitions/load_test.go",
			Package:        "definitions",
			Name:           "TestLoad",
			Line:           7,
			Undocumented:   true,
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
		{
			File:           "guards/policy/enforce_test.go",
			Package:        "policy",
			Name:           "TestEnforce",
			Line:           15,
			Description:    "verifies guard enforcement",
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
		{
			File:           "runtime_api/session_test.go",
			Package:        "runtime_api",
			Name:           "TestSession",
			Line:           5,
			Undocumented:   true,
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
		{
			File:           "internal/support/helpers_test.go",
			Package:        "support",
			Name:           "TestHelper",
			Line:           3,
			Classification: functionaltestmetadata.ClassificationHarness,
		},
	})
	records[1].Provenance = functionaltestviz.GoldenProvenance{
		Provider:      "openai",
		Case:          "invoke",
		FidelityClass: "partial-stream",
		ID:            "openai-invoke",
		ManifestPath:  "testdata/goldens/openai/invoke/manifest.json",
	}
	return records
}

func fullReportCoverageSummary() functionaltestviz.CoverageSummary {
	floor := 66.66
	return functionaltestviz.CoverageSummary{
		CoveredStatements:    8,
		MeasurableStatements: 10,
		CoveragePercent:      80.0,
		Packages: []functionaltestviz.PackageCoverage{
			{
				Package:              "github.com/portpowered/infinite-you/pkg/service",
				CoveredStatements:    0,
				MeasurableStatements: 0,
				CoveragePercent:      0.0,
				MeasurementException: &functionaltestviz.MeasurementException{
					Kind:          "measurement",
					Justification: "no measurable statements",
					Owner:         "backend-quality",
					Deadline:      "2027-07-15",
					RemovalGate:   "profile reports measurable statements",
				},
			},
			{
				Package:              "github.com/portpowered/infinite-you/pkg/config",
				CoveredStatements:    3,
				MeasurableStatements: 3,
				CoveragePercent:      100.0,
				PackageFloor:         &floor,
			},
		},
	}
}

func assertFullReportCoversRepresentativeSections(t *testing.T, catalog string) {
	t.Helper()
	required := []string{
		"# Functional tests\n",
		"## Domain summaries\n",
		"### transport\n",
		"### workers\n",
		"### guards / resources\n",
		"### observability / product / resilience\n",
		"## Test catalog\n",
		"- Labels: short\n",
		"- Labels: long-only, golden-backed\n",
		"- Labels: short, undocumented\n",
		"- Labels: short, deprecated, undocumented\n",
		"  - Golden provenance:\n",
		"## Documentation debt\n",
		"### Undocumented customer tests\n",
		"### Deprecated tests\n",
		"## Package coverage\n",
		"| `github.com/portpowered/infinite-you/pkg/config` |",
		"## Harness verification\n",
	}
	for _, fragment := range required {
		if !strings.Contains(catalog, fragment) {
			t.Fatalf("full report missing %q:\n%s", fragment, catalog)
		}
	}
}
