package contractopenapidiff_test

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"github.com/portpowered/infinite-you/internal/contractopenapidiff"
)

func TestCompareYAML_DocsOnlyFixture_ClassifiesPatchWithStableChanges(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "docs-only", "before.yaml")
	after := readFixture(t, "docs-only", "after.yaml")

	result, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("CompareYAML() error = %v", err)
	}
	if result.Classification != contractopenapidiff.ClassificationPatch {
		t.Fatalf("Classification = %q, want %q", result.Classification, contractopenapidiff.ClassificationPatch)
	}

	wantChanges := []contractopenapidiff.Change{
		{Code: contractopenapidiff.CodeOperationDescriptionChanged, Path: "GET /pets"},
		{Code: contractopenapidiff.CodeOperationSummaryChanged, Path: "GET /pets"},
		{Code: contractopenapidiff.CodeParameterDescriptionChanged, Path: "GET /pets.parameters[query:limit]"},
		{Code: contractopenapidiff.CodeRequestBodyDescriptionChanged, Path: "GET /pets.requestBody"},
		{Code: contractopenapidiff.CodeResponseDescriptionChanged, Path: "GET /pets.responses.200"},
		{Code: contractopenapidiff.CodeSchemaDescriptionChanged, Path: "components.schemas.Pet"},
		{Code: contractopenapidiff.CodeSchemaTitleChanged, Path: "components.schemas.Pet"},
		{Code: contractopenapidiff.CodeSchemaDescriptionChanged, Path: "components.schemas.Pet.properties.id"},
		{Code: contractopenapidiff.CodeSchemaDescriptionChanged, Path: "components.schemas.Pet.properties.name"},
		{Code: contractopenapidiff.CodeSchemaDescriptionChanged, Path: "components.schemas.PetFilter.properties.name"},
		{Code: contractopenapidiff.CodeInfoDescriptionChanged, Path: "info"},
		{Code: contractopenapidiff.CodeTagDescriptionChanged, Path: "tags.pets"},
	}
	if !slices.Equal(result.Changes, wantChanges) {
		t.Fatalf("Changes = %#v, want %#v", result.Changes, wantChanges)
	}
}

func TestCompareYAML_DocsOnlyFixture_IsDeterministic(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "docs-only", "before.yaml")
	after := readFixture(t, "docs-only", "after.yaml")

	first, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("first CompareYAML() error = %v", err)
	}
	second, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("second CompareYAML() error = %v", err)
	}
	if !slices.Equal(first.Changes, second.Changes) {
		t.Fatalf("changes differ across runs: first=%#v second=%#v", first.Changes, second.Changes)
	}
	if first.Classification != second.Classification {
		t.Fatalf("classification differ across runs: first=%q second=%q", first.Classification, second.Classification)
	}
}

func TestCompareYAML_StructuralDifferenceFailsClosed(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "docs-only", "before.yaml")
	after := readFixture(t, "structural-add-route", "after.yaml")

	_, err := contractopenapidiff.CompareYAML(before, after)
	if err == nil {
		t.Fatal("expected structural difference to fail closed")
	}
}

func readFixture(t *testing.T, parts ...string) []byte {
	t.Helper()

	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(append([]string{filepath.Dir(file), "testdata"}, parts...)...)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}
