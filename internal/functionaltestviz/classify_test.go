package functionaltestviz_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
	"github.com/portpowered/infinite-you/internal/functionaltestviz"
)

func TestDecodeCoverageSummaryAcceptsGocoveragecheckShape(t *testing.T) {
	t.Parallel()

	const raw = `{
  "coveredStatements": 8,
  "measurableStatements": 10,
  "coveragePercent": 80.0,
  "packages": [
    {
      "package": "github.com/portpowered/infinite-you/pkg/config",
      "coveredStatements": 3,
      "measurableStatements": 3,
      "coveragePercent": 100.0,
      "packageFloor": 66.66,
      "measurementException": null
    },
    {
      "package": "github.com/portpowered/infinite-you/pkg/service",
      "coveredStatements": 0,
      "measurableStatements": 0,
      "coveragePercent": 0.0,
      "packageFloor": null,
      "measurementException": {
        "kind": "measurement",
        "justification": "no measurable statements",
        "owner": "backend-quality",
        "deadline": "2027-07-15",
        "removalGate": "profile reports measurable statements"
      }
    }
  ]
}
`
	summary, err := functionaltestviz.DecodeCoverageSummary([]byte(raw))
	if err != nil {
		t.Fatalf("DecodeCoverageSummary() error = %v", err)
	}
	if summary.CoveredStatements != 8 || summary.MeasurableStatements != 10 {
		t.Fatalf("totals = %d/%d, want 8/10", summary.CoveredStatements, summary.MeasurableStatements)
	}
	if summary.CoveragePercent != 80.0 {
		t.Fatalf("coveragePercent = %v, want 80.0", summary.CoveragePercent)
	}
	if len(summary.Packages) != 2 {
		t.Fatalf("packages len = %d, want 2", len(summary.Packages))
	}
	if summary.Packages[0].PackageFloor == nil || *summary.Packages[0].PackageFloor != 66.66 {
		t.Fatalf("packages[0].packageFloor = %v, want 66.66", summary.Packages[0].PackageFloor)
	}
	if summary.Packages[1].MeasurementException == nil {
		t.Fatal("packages[1].measurementException is nil, want structured exception")
	}
	if got := summary.Packages[1].MeasurementException.Kind; got != "measurement" {
		t.Fatalf("measurementException.kind = %q, want measurement", got)
	}
}

func TestLoadCoverageSummaryFailsClosedForMissingAndMalformed(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing-coverage-summary.json")
	_, err := functionaltestviz.LoadCoverageSummary(missing)
	if err == nil {
		t.Fatal("LoadCoverageSummary(missing) error = nil, want actionable error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("LoadCoverageSummary(missing) error = %v, want not-found guidance", err)
	}

	_, err = functionaltestviz.LoadCoverageSummary("")
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("LoadCoverageSummary(\"\") error = %v, want required-path guidance", err)
	}

	badPath := filepath.Join(t.TempDir(), "bad.json")
	if writeErr := os.WriteFile(badPath, []byte(`{"coveredStatements":1`), 0o644); writeErr != nil {
		t.Fatalf("write malformed summary: %v", writeErr)
	}
	_, err = functionaltestviz.LoadCoverageSummary(badPath)
	if err == nil || !strings.Contains(err.Error(), "invalid coverage-summary JSON") {
		t.Fatalf("LoadCoverageSummary(malformed) error = %v, want invalid JSON guidance", err)
	}

	_, err = functionaltestviz.DecodeCoverageSummary([]byte(`{"coveredStatements":1,"measurableStatements":1,"coveragePercent":100}`))
	if err == nil || !strings.Contains(err.Error(), "packages") {
		t.Fatalf("DecodeCoverageSummary(missing packages) error = %v, want packages guidance", err)
	}
}

func TestAssembleCatalogInputsClassifiesRecordsAndLoadsCoverage(t *testing.T) {
	t.Parallel()

	summaryPath := filepath.Join(t.TempDir(), "coverage-summary.json")
	const raw = `{
  "coveredStatements": 1,
  "measurableStatements": 2,
  "coveragePercent": 50.0,
  "packages": [
    {
      "package": "github.com/portpowered/infinite-you/pkg/example",
      "coveredStatements": 1,
      "measurableStatements": 2,
      "coveragePercent": 50.0,
      "packageFloor": 40.0,
      "measurementException": null
    }
  ]
}
`
	if err := os.WriteFile(summaryPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write coverage summary: %v", err)
	}

	records := []functionaltestmetadata.Record{
		{
			File:           "transport/cli/process/help_test.go",
			Package:        "process",
			Name:           "TestHelp",
			Line:           10,
			Description:    "verifies help output",
			BuildTags:      nil,
			Golden:         "",
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
		{
			File:           "workers/inference/openai/invoke_long_test.go",
			Package:        "openai",
			Name:           "TestInvoke",
			Line:           20,
			Description:    "verifies provider invoke",
			BuildTags:      []string{"functionallong"},
			Golden:         "testdata/goldens/openai/manifest.json",
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
			File:           "tests/functional/factory/definitions/validate_test.go",
			Package:        "definitions",
			Name:           "TestValidate",
			Line:           8,
			Description:    "validates factory definitions",
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
		{
			File:           "internal/support/helpers_test.go",
			Package:        "support",
			Name:           "TestHelper",
			Line:           3,
			Classification: functionaltestmetadata.ClassificationHarness,
		},
	}

	inputs, err := functionaltestviz.AssembleCatalogInputs(records, summaryPath)
	if err != nil {
		t.Fatalf("AssembleCatalogInputs() error = %v", err)
	}
	if inputs.Coverage.CoveredStatements != 1 || inputs.Coverage.MeasurableStatements != 2 {
		t.Fatalf("coverage totals = %d/%d, want 1/2", inputs.Coverage.CoveredStatements, inputs.Coverage.MeasurableStatements)
	}
	if len(inputs.Records) != len(records) {
		t.Fatalf("classified len = %d, want %d", len(inputs.Records), len(records))
	}

	assertClassified(t, inputs.Records[0], "transport", "process", functionaltestviz.LaneShort, false, false, false)
	assertClassified(t, inputs.Records[1], "workers", "openai", functionaltestviz.LaneLongOnly, true, false, false)
	assertClassified(t, inputs.Records[2], "runtime_api", "runtime_api", functionaltestviz.LaneShort, false, true, true)
	assertClassified(t, inputs.Records[3], "factory", "definitions", functionaltestviz.LaneShort, false, false, false)
	assertClassified(t, inputs.Records[4], "internal", "support", functionaltestviz.LaneShort, false, false, false)
}

func TestLaneFromBuildTagsDetectsLongOnlyWithoutFabricatingDefaults(t *testing.T) {
	t.Parallel()

	if got := functionaltestviz.LaneFromBuildTags(nil); got != functionaltestviz.LaneShort {
		t.Fatalf("LaneFromBuildTags(nil) = %q, want %q", got, functionaltestviz.LaneShort)
	}
	if got := functionaltestviz.LaneFromBuildTags([]string{"!functionallong"}); got != functionaltestviz.LaneShort {
		t.Fatalf("LaneFromBuildTags(!functionallong) = %q, want %q", got, functionaltestviz.LaneShort)
	}
	if got := functionaltestviz.LaneFromBuildTags([]string{"functionallong"}); got != functionaltestviz.LaneLongOnly {
		t.Fatalf("LaneFromBuildTags(functionallong) = %q, want %q", got, functionaltestviz.LaneLongOnly)
	}
	if got := functionaltestviz.LaneFromBuildTags([]string{"linux && functionallong"}); got != functionaltestviz.LaneLongOnly {
		t.Fatalf("LaneFromBuildTags(compound) = %q, want %q", got, functionaltestviz.LaneLongOnly)
	}
}

func TestDomainFromFileHandlesWindowsSeparatorsAndRepoRelativePaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		file string
		want string
	}{
		{file: `transport\cli\help_test.go`, want: "transport"},
		{file: "tests/functional/sessions/lifecycle/open_test.go", want: "sessions"},
		{file: "guards/policy_test.go", want: "guards"},
		{file: "", want: ""},
	}
	for _, tc := range cases {
		if got := functionaltestviz.DomainFromFile(tc.file); got != tc.want {
			t.Fatalf("DomainFromFile(%q) = %q, want %q", tc.file, got, tc.want)
		}
	}
}

func assertClassified(
	t *testing.T,
	got functionaltestviz.ClassifiedRecord,
	domain, pkg, lane string,
	goldenBacked, undocumented, deprecated bool,
) {
	t.Helper()
	if got.Domain != domain {
		t.Fatalf("Domain = %q, want %q", got.Domain, domain)
	}
	if got.Package != pkg {
		t.Fatalf("Package = %q, want %q", got.Package, pkg)
	}
	if got.Lane != lane {
		t.Fatalf("Lane = %q, want %q", got.Lane, lane)
	}
	if got.GoldenBacked != goldenBacked {
		t.Fatalf("GoldenBacked = %v, want %v", got.GoldenBacked, goldenBacked)
	}
	if got.Undocumented != undocumented {
		t.Fatalf("Undocumented = %v, want %v", got.Undocumented, undocumented)
	}
	if got.Deprecated != deprecated {
		t.Fatalf("Deprecated = %v, want %v", got.Deprecated, deprecated)
	}
}
