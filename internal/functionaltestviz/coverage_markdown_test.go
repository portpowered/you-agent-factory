package functionaltestviz_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
	"github.com/portpowered/infinite-you/internal/functionaltestviz"
)

func TestStableOrderedPackagesSortsByImportPath(t *testing.T) {
	t.Parallel()

	packages := []functionaltestviz.PackageCoverage{
		{Package: "github.com/portpowered/infinite-you/pkg/zeta"},
		{Package: "github.com/portpowered/infinite-you/pkg/alpha"},
		{Package: "github.com/portpowered/infinite-you/pkg/beta"},
	}
	ordered := functionaltestviz.StableOrderedPackages(packages)
	want := []string{
		"github.com/portpowered/infinite-you/pkg/alpha",
		"github.com/portpowered/infinite-you/pkg/beta",
		"github.com/portpowered/infinite-you/pkg/zeta",
	}
	if len(ordered) != len(want) {
		t.Fatalf("ordered len = %d, want %d", len(ordered), len(want))
	}
	for i := range want {
		if ordered[i].Package != want[i] {
			t.Fatalf("ordered[%d] = %q, want %q", i, ordered[i].Package, want[i])
		}
	}
	if packages[0].Package != "github.com/portpowered/infinite-you/pkg/zeta" {
		t.Fatal("StableOrderedPackages mutated input slice")
	}
}

func TestRenderPackageCoverageMarkdownUsesSummaryFieldsOnly(t *testing.T) {
	t.Parallel()

	floor := 66.66
	summary := functionaltestviz.CoverageSummary{
		CoveredStatements:    8,
		MeasurableStatements: 10,
		CoveragePercent:      80.0,
		Packages: []functionaltestviz.PackageCoverage{
			{
				Package:              "github.com/portpowered/infinite-you/pkg/service",
				CoveredStatements:    0,
				MeasurableStatements: 0,
				CoveragePercent:      0.0,
				PackageFloor:         nil,
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
				MeasurementException: nil,
			},
		},
	}

	first := functionaltestviz.RenderPackageCoverageMarkdown(summary)
	second := functionaltestviz.RenderPackageCoverageMarkdown(summary)
	if first != second {
		t.Fatalf("repeated coverage renders diverged:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	if !strings.HasPrefix(first, "## Package coverage\n\n") {
		t.Fatalf("coverage section heading missing:\n%s", first)
	}
	if !strings.Contains(first, "- Covered statements: 8\n") {
		t.Fatalf("overall covered statements missing:\n%s", first)
	}
	if !strings.Contains(first, "- Measurable statements: 10\n") {
		t.Fatalf("overall measurable statements missing:\n%s", first)
	}
	if !strings.Contains(first, "- Coverage percent: 80.0%\n") {
		t.Fatalf("overall coverage percent missing:\n%s", first)
	}

	configIdx := strings.Index(first, "| `github.com/portpowered/infinite-you/pkg/config` |")
	serviceIdx := strings.Index(first, "| `github.com/portpowered/infinite-you/pkg/service` |")
	if configIdx < 0 || serviceIdx < 0 || configIdx >= serviceIdx {
		t.Fatalf("packages must render in stable path order:\n%s", first)
	}
	if !strings.Contains(first, "| `github.com/portpowered/infinite-you/pkg/config` | 3 | 3 | 100.0 | 66.66 | — |") {
		t.Fatalf("config package row missing floor rendering:\n%s", first)
	}
	if !strings.Contains(first, "| `github.com/portpowered/infinite-you/pkg/service` | 0 | 0 | 0.0 | — | measurement: no measurable statements (owner=backend-quality; deadline=2027-07-15; removalGate=profile reports measurable statements) |") {
		t.Fatalf("service package row missing measurement exception:\n%s", first)
	}
}

func TestRenderPackageCoverageMarkdownEmptyPackages(t *testing.T) {
	t.Parallel()

	got := functionaltestviz.RenderPackageCoverageMarkdown(functionaltestviz.CoverageSummary{
		CoveredStatements:    0,
		MeasurableStatements: 0,
		CoveragePercent:      0.0,
		Packages:             []functionaltestviz.PackageCoverage{},
	})
	if !strings.Contains(got, "- Covered statements: 0\n") {
		t.Fatalf("zero overall totals missing:\n%s", got)
	}
	if !strings.Contains(got, "- _No production packages in coverage summary._\n") {
		t.Fatalf("empty packages presentation missing:\n%s", got)
	}
	if strings.Contains(got, "| Package |") {
		t.Fatalf("empty packages must not render a table:\n%s", got)
	}
}

func TestRenderCatalogMarkdownAppendsPackageCoverageAfterDebt(t *testing.T) {
	t.Parallel()

	floor := 40.0
	records := functionaltestviz.ClassifyRecords([]functionaltestmetadata.Record{
		customerRecord("transport/cli/process/help_test.go", "process", "TestHelp"),
	})
	catalog, err := functionaltestviz.RenderCatalogMarkdown(functionaltestviz.CatalogInputs{
		Records: records,
		Coverage: functionaltestviz.CoverageSummary{
			CoveredStatements:    1,
			MeasurableStatements: 2,
			CoveragePercent:      50.0,
			Packages: []functionaltestviz.PackageCoverage{
				{
					Package:              "github.com/portpowered/infinite-you/pkg/example",
					CoveredStatements:    1,
					MeasurableStatements: 2,
					CoveragePercent:      50.0,
					PackageFloor:         &floor,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("RenderCatalogMarkdown: %v", err)
	}

	debtIdx := strings.Index(catalog, "## Documentation debt\n")
	coverageIdx := strings.Index(catalog, "## Package coverage\n")
	if debtIdx < 0 || coverageIdx < 0 || coverageIdx <= debtIdx {
		t.Fatalf("catalog must append package coverage after documentation debt:\n%s", catalog)
	}
	if !strings.Contains(catalog, "- Coverage percent: 50.0%\n") {
		t.Fatalf("overall coverage percent missing from catalog:\n%s", catalog)
	}
	if !strings.Contains(catalog, "| `github.com/portpowered/infinite-you/pkg/example` | 1 | 2 | 50.0 | 40 | — |") {
		t.Fatalf("package coverage row missing from catalog:\n%s", catalog)
	}
}
