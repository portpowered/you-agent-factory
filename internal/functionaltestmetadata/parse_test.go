package functionaltestmetadata_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
)

func TestParseInventoriesTopLevelTestMetadata(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"smoke/documented_test.go": `package smoke

import "testing"

// TestAlpha verifies the happy path for cold start.
func TestAlpha(t *testing.T) {}

// TestBeta checks retries after a transient failure.
func TestBeta(t *testing.T) {}
`,
		"smoke/undocumented_test.go": `package smoke

import "testing"

func TestGamma(t *testing.T) {}
`,
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("Parse() returned %d records, want 3: %#v", len(records), records)
	}

	assertRecord(t, records[0], "smoke/documented_test.go", "smoke", "TestAlpha", "TestAlpha verifies the happy path for cold start.", false)
	assertRecord(t, records[1], "smoke/documented_test.go", "smoke", "TestBeta", "TestBeta checks retries after a transient failure.", false)
	assertRecord(t, records[2], "smoke/undocumented_test.go", "smoke", "TestGamma", "", true)
	if records[0].Line <= 0 || records[1].Line <= records[0].Line {
		t.Fatalf("source lines not increasing within file: %#v", records[:2])
	}
	for _, record := range records {
		if record.Classification != functionaltestmetadata.ClassificationCustomer {
			t.Fatalf("%s Classification = %q, want %q", record.Name, record.Classification, functionaltestmetadata.ClassificationCustomer)
		}
	}
	if got := functionaltestmetadata.CustomerScenarioCount(records); got != 3 {
		t.Fatalf("CustomerScenarioCount = %d, want 3", got)
	}
}

func TestParseMarksMissingDocAsUndocumentedWithoutInventingText(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"pkg/missing_test.go": `package pkg

import "testing"

func TestWithoutDoc(t *testing.T) {}
`,
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Parse() returned %d records, want 1", len(records))
	}
	if !records[0].Undocumented {
		t.Fatalf("Undocumented = false, want true")
	}
	if records[0].Description != "" {
		t.Fatalf("Description = %q, want empty", records[0].Description)
	}
}

func TestParseDoesNotExpandTableDrivenCases(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"pkg/table_test.go": `package pkg

import "testing"

// TestTable covers multiple cases inside one top-level Test*.
func TestTable(t *testing.T) {
	cases := []struct{ name string }{
		{name: "one"},
		{name: "two"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {})
	}
}
`,
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Parse() returned %d records, want 1 enclosing Test*: %#v", len(records), records)
	}
	if records[0].Name != "TestTable" {
		t.Fatalf("Name = %q, want TestTable", records[0].Name)
	}
}

func TestParseReturnsStableOrderAcrossRepeatedRuns(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"b/second_test.go": `package b

import "testing"

// TestZ comes last alphabetically by name within file.
func TestZ(t *testing.T) {}

// TestA comes first alphabetically by name within file.
func TestA(t *testing.T) {}
`,
		"a/first_test.go": `package a

import "testing"

// TestMid is the only test in the earlier file path.
func TestMid(t *testing.T) {}
`,
	})

	first, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() first error = %v", err)
	}
	second, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() second error = %v", err)
	}
	if len(first) != 3 {
		t.Fatalf("Parse() returned %d records, want 3", len(first))
	}
	wantNames := []string{"TestMid", "TestA", "TestZ"}
	wantFiles := []string{"a/first_test.go", "b/second_test.go", "b/second_test.go"}
	for i := range first {
		if first[i].Name != wantNames[i] || first[i].File != wantFiles[i] {
			t.Fatalf("record[%d] = %s::%s, want %s::%s", i, first[i].File, first[i].Name, wantFiles[i], wantNames[i])
		}
		assertRecordsEqual(t, first[i], second[i])
	}
}

func TestParseFailsClosedOnMalformedSource(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"pkg/good_test.go": `package pkg

import "testing"

// TestGood is valid.
func TestGood(t *testing.T) {}
`,
		"pkg/bad_test.go": `package pkg

func TestBroken(t *testing.T {
`,
	})

	records, err := functionaltestmetadata.Parse(root)
	if err == nil {
		t.Fatalf("Parse() error = nil, want malformed-source failure; records=%#v", records)
	}
	if !strings.Contains(err.Error(), "pkg/bad_test.go") {
		t.Fatalf("Parse() error = %v, want actionable file-scoped path pkg/bad_test.go", err)
	}
	if records != nil {
		t.Fatalf("Parse() records = %#v, want nil on failure", records)
	}
}

func TestParseNormalizesWindowsStylePaths(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"smoke/path_test.go": `package smoke

import "testing"

// TestPathIdentity stays stable across separators.
func TestPathIdentity(t *testing.T) {}
`,
	})

	windowsRoot := strings.ReplaceAll(root, "/", `\`)
	records, err := functionaltestmetadata.Parse(windowsRoot)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Parse() returned %d records, want 1", len(records))
	}
	if records[0].File != "smoke/path_test.go" {
		t.Fatalf("File = %q, want slash-normalized smoke/path_test.go", records[0].File)
	}
	if strings.Contains(records[0].File, `\`) {
		t.Fatalf("File still contains backslashes: %q", records[0].File)
	}
	if got := records[0].Identity(); got != "smoke/path_test.go::TestPathIdentity" {
		t.Fatalf("Identity() = %q, want smoke/path_test.go::TestPathIdentity", got)
	}
}

func TestParsePreservesMarkdownSensitiveDescriptions(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"pkg/markdown_test.go": "package pkg\n\nimport \"testing\"\n\n" +
			"// TestMarkdown checks `code`, *emphasis*, _italics_, and [link](https://example.com).\n" +
			"func TestMarkdown(t *testing.T) {}\n",
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Parse() returned %d records, want 1", len(records))
	}
	want := "TestMarkdown checks `code`, *emphasis*, _italics_, and [link](https://example.com)."
	if records[0].Description != want {
		t.Fatalf("Description = %q, want %q", records[0].Description, want)
	}
	if records[0].Name != "TestMarkdown" || records[0].File != "pkg/markdown_test.go" {
		t.Fatalf("surrounding fields corrupted: %#v", records[0])
	}
}

func TestParseSkipsNonTestHelpersAndLowercaseNames(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"pkg/helpers_test.go": `package pkg

import "testing"

func helper(t *testing.T) {}

func testLower(t *testing.T) {}

type harness struct{}

func (h harness) TestMethod(t *testing.T) {}

// TestOnly counts as the sole inventory record.
func TestOnly(t *testing.T) {}
`,
		"pkg/doc.go": `package pkg
`,
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 1 || records[0].Name != "TestOnly" {
		t.Fatalf("Parse() = %#v, want only TestOnly", records)
	}
}

func TestParseCapturesFileLevelBuildTags(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"long/tagged_test.go": `//go:build functionallong

package long

import "testing"

// TestLongOnly is gated by the functionallong build tag.
func TestLongOnly(t *testing.T) {}
`,
		"short/untagged_test.go": `package short

import "testing"

// TestShort has no file-level build constraints.
func TestShort(t *testing.T) {}
`,
		"platform/negated_test.go": `//go:build !windows

package platform

import "testing"

// TestNotWindows excludes Windows hosts.
func TestNotWindows(t *testing.T) {}
`,
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("Parse() returned %d records, want 3: %#v", len(records), records)
	}

	byName := map[string]functionaltestmetadata.Record{}
	for _, record := range records {
		byName[record.Name] = record
	}

	if got := byName["TestLongOnly"].BuildTags; len(got) != 1 || got[0] != "functionallong" {
		t.Fatalf("TestLongOnly.BuildTags = %#v, want [functionallong]", got)
	}
	if got := byName["TestNotWindows"].BuildTags; len(got) != 1 || got[0] != "!windows" {
		t.Fatalf("TestNotWindows.BuildTags = %#v, want [!windows]", got)
	}
	if got := byName["TestShort"].BuildTags; len(got) != 0 {
		t.Fatalf("TestShort.BuildTags = %#v, want empty (no fabricated default)", got)
	}
}

func TestParsePrefersGoBuildOverPlusBuild(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"pkg/both_test.go": `//go:build functionallong
// +build functionallong

package pkg

import "testing"

// TestBothConstraints prefers the go:build expression.
func TestBothConstraints(t *testing.T) {}
`,
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Parse() returned %d records, want 1", len(records))
	}
	if got := records[0].BuildTags; len(got) != 1 || got[0] != "functionallong" {
		t.Fatalf("BuildTags = %#v, want exactly [functionallong]", got)
	}
}

func TestParseCapturesGoldenFromCommentDirective(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"workers/inference/codex/golden_success_test.go": `package codex

import "testing"

// TestCodexGoldenSuccess replays the sanitized success transcript.
//golden: tests/functional/internal/support/testdata/provider-sessions/codex/success/manifest.json
func TestCodexGoldenSuccess(t *testing.T) {}
`,
		"workers/inference/codex/plain_test.go": `package codex

import "testing"

// TestCodexPlain has no golden declaration.
func TestCodexPlain(t *testing.T) {}
`,
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("Parse() returned %d records, want 2: %#v", len(records), records)
	}

	byName := map[string]functionaltestmetadata.Record{}
	for _, record := range records {
		byName[record.Name] = record
	}

	wantGolden := "tests/functional/internal/support/testdata/provider-sessions/codex/success/manifest.json"
	if got := byName["TestCodexGoldenSuccess"].Golden; got != wantGolden {
		t.Fatalf("Golden = %q, want %q", got, wantGolden)
	}
	if byName["TestCodexGoldenSuccess"].Description != "TestCodexGoldenSuccess replays the sanitized success transcript." {
		t.Fatalf("Description polluted by golden directive: %#v", byName["TestCodexGoldenSuccess"])
	}
	if got := byName["TestCodexPlain"].Golden; got != "" {
		t.Fatalf("TestCodexPlain.Golden = %q, want empty (no fabricated reference)", got)
	}
}

func TestParseCapturesGoldenFromTestOwnedDeclaration(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"workers/inference/claude/golden_failure_test.go": `package claude

import "testing"

// TestClaudeGoldenFailure uses a test-owned golden manifest declaration.
func TestClaudeGoldenFailure(t *testing.T) {
	const goldenManifest = "tests/functional/internal/support/testdata/provider-sessions/claude/failure/manifest.json"
	_ = goldenManifest
}
`,
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Parse() returned %d records, want 1", len(records))
	}
	want := "tests/functional/internal/support/testdata/provider-sessions/claude/failure/manifest.json"
	if records[0].Golden != want {
		t.Fatalf("Golden = %q, want %q", records[0].Golden, want)
	}
	if records[0].Description != "TestClaudeGoldenFailure uses a test-owned golden manifest declaration." {
		t.Fatalf("Description = %q, want ordinary doc synopsis", records[0].Description)
	}
}

func TestParseNormalizesWindowsStyleGoldenPaths(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"pkg/golden_test.go": "package pkg\n\nimport \"testing\"\n\n" +
			"// TestGoldenPath normalizes separators in golden references.\n" +
			"//golden: docs\\temp\\functional\\provider-sessions\\cursor\\success\\manifest.json\n" +
			"func TestGoldenPath(t *testing.T) {}\n",
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("Parse() returned %d records, want 1", len(records))
	}
	want := "tests/functional/internal/support/testdata/provider-sessions/cursor/success/manifest.json"
	if records[0].Golden != want {
		t.Fatalf("Golden = %q, want slash-normalized %q", records[0].Golden, want)
	}
}

func TestParseClassifiesInternalSupportAsHarness(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"internal/support/paths_test.go": `package support_test

import "testing"

// TestDefaultSessionEventsURL verifies harness URL helpers.
func TestDefaultSessionEventsURL(t *testing.T) {}
`,
		"smoke/customer_test.go": `package smoke

import "testing"

// TestCustomerColdStart is a customer-facing scenario.
func TestCustomerColdStart(t *testing.T) {}
`,
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("Parse() returned %d records, want 2: %#v", len(records), records)
	}

	byName := map[string]functionaltestmetadata.Record{}
	for _, record := range records {
		byName[record.Name] = record
	}

	internal := byName["TestDefaultSessionEventsURL"]
	if internal.Classification != functionaltestmetadata.ClassificationHarness {
		t.Fatalf("internal Classification = %q, want %q", internal.Classification, functionaltestmetadata.ClassificationHarness)
	}
	if internal.IsCustomerScenario() {
		t.Fatalf("internal record must not count as a customer scenario")
	}

	customer := byName["TestCustomerColdStart"]
	if customer.Classification != functionaltestmetadata.ClassificationCustomer {
		t.Fatalf("customer Classification = %q, want %q", customer.Classification, functionaltestmetadata.ClassificationCustomer)
	}
	if got := functionaltestmetadata.CustomerScenarioCount(records); got != 1 {
		t.Fatalf("CustomerScenarioCount = %d, want 1 (internal excluded)", got)
	}
}

func TestParseExcludesHelperOnlyFilesFromCustomerCounts(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"smoke/helpers_test.go": `package smoke

func sharedHelper() {}
`,
		"smoke/short_helpers_contract_test.go": `package smoke

import "testing"

// TestHelperContract verifies shared helper behavior only.
func TestHelperContract(t *testing.T) {}
`,
		"smoke/customer_test.go": `package smoke

import "testing"

// TestCustomerScenario is a customer-facing proof.
func TestCustomerScenario(t *testing.T) {}
`,
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("Parse() returned %d records, want 2 (helper-only file with no Test* emits nothing): %#v", len(records), records)
	}

	byName := map[string]functionaltestmetadata.Record{}
	for _, record := range records {
		byName[record.Name] = record
	}

	helper := byName["TestHelperContract"]
	if helper.Classification != functionaltestmetadata.ClassificationHarness {
		t.Fatalf("helpers Classification = %q, want %q", helper.Classification, functionaltestmetadata.ClassificationHarness)
	}
	if helper.IsCustomerScenario() {
		t.Fatalf("helper-only Test* must not count as a customer scenario")
	}

	customer := byName["TestCustomerScenario"]
	if customer.Classification != functionaltestmetadata.ClassificationCustomer {
		t.Fatalf("customer Classification = %q, want %q", customer.Classification, functionaltestmetadata.ClassificationCustomer)
	}
	if got := functionaltestmetadata.CustomerScenarioCount(records); got != 1 {
		t.Fatalf("CustomerScenarioCount = %d, want 1 after helper-only exclusion", got)
	}
}

func TestParseClassifiesMixedCustomerScenarioFile(t *testing.T) {
	t.Parallel()

	root := writeTree(t, map[string]string{
		"smoke/mixed_test.go": `package smoke

import "testing"

func localHelper(t *testing.T) {
	t.Helper()
}

// TestAlpha is the first customer scenario in a mixed file.
func TestAlpha(t *testing.T) {}

// TestBeta is the second customer scenario in a mixed file.
func TestBeta(t *testing.T) {}
`,
		"internal/support/harness_test.go": `package support

import "testing"

// TestHarnessOnly stays out of customer totals.
func TestHarnessOnly(t *testing.T) {}
`,
		"smoke/helpers_test.go": `package smoke

func unusedSharedHelper() string { return "ok" }
`,
	})

	records, err := functionaltestmetadata.Parse(root)
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("Parse() returned %d records, want 3: %#v", len(records), records)
	}

	customerNames := make([]string, 0, 2)
	for _, record := range records {
		switch record.Name {
		case "TestAlpha", "TestBeta":
			if record.Classification != functionaltestmetadata.ClassificationCustomer {
				t.Fatalf("%s Classification = %q, want %q", record.Name, record.Classification, functionaltestmetadata.ClassificationCustomer)
			}
			customerNames = append(customerNames, record.Name)
		case "TestHarnessOnly":
			if record.Classification != functionaltestmetadata.ClassificationHarness {
				t.Fatalf("TestHarnessOnly Classification = %q, want %q", record.Classification, functionaltestmetadata.ClassificationHarness)
			}
		default:
			t.Fatalf("unexpected record %#v", record)
		}
	}
	if len(customerNames) != 2 {
		t.Fatalf("customer names = %#v, want TestAlpha and TestBeta", customerNames)
	}
	if got := functionaltestmetadata.CustomerScenarioCount(records); got != 2 {
		t.Fatalf("CustomerScenarioCount = %d, want 2 (equals inventoried customer Test* after exclusions)", got)
	}
}

func assertRecord(t *testing.T, got functionaltestmetadata.Record, file, pkg, name, description string, undocumented bool) {
	t.Helper()
	if got.File != file {
		t.Fatalf("File = %q, want %q", got.File, file)
	}
	if got.Package != pkg {
		t.Fatalf("Package = %q, want %q", got.Package, pkg)
	}
	if got.Name != name {
		t.Fatalf("Name = %q, want %q", got.Name, name)
	}
	if got.Description != description {
		t.Fatalf("Description = %q, want %q", got.Description, description)
	}
	if got.Undocumented != undocumented {
		t.Fatalf("Undocumented = %v, want %v", got.Undocumented, undocumented)
	}
	if got.Classification == "" {
		t.Fatalf("Classification is empty on %#v", got)
	}
}

func assertRecordsEqual(t *testing.T, got, want functionaltestmetadata.Record) {
	t.Helper()
	if got.File != want.File || got.Package != want.Package || got.Name != want.Name ||
		got.Line != want.Line || got.Description != want.Description || got.Undocumented != want.Undocumented ||
		got.Golden != want.Golden || got.Classification != want.Classification {
		t.Fatalf("records differ:\ngot  %#v\nwant %#v", got, want)
	}
	if len(got.BuildTags) != len(want.BuildTags) {
		t.Fatalf("BuildTags length %d != %d:\ngot  %#v\nwant %#v", len(got.BuildTags), len(want.BuildTags), got, want)
	}
	for i := range got.BuildTags {
		if got.BuildTags[i] != want.BuildTags[i] {
			t.Fatalf("BuildTags[%d] = %q, want %q", i, got.BuildTags[i], want.BuildTags[i])
		}
	}
}

func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for relative, contents := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return root
}
