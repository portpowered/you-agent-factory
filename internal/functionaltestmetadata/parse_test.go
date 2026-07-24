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
		if first[i] != second[i] {
			t.Fatalf("repeated parse diverged at index %d: %#v vs %#v", i, first[i], second[i])
		}
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
