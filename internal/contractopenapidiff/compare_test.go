package contractopenapidiff_test

import (
	"errors"
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

func TestCompareYAML_AddRouteFixture_ClassifiesMinor(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "add-route", "before.yaml")
	after := readFixture(t, "add-route", "after.yaml")

	result, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("CompareYAML() error = %v", err)
	}
	if result.Classification != contractopenapidiff.ClassificationMinor {
		t.Fatalf("Classification = %q, want %q", result.Classification, contractopenapidiff.ClassificationMinor)
	}

	wantChanges := []contractopenapidiff.Change{
		{Code: contractopenapidiff.CodeOperationAdded, Path: "POST /pets"},
	}
	if !slices.Equal(result.Changes, wantChanges) {
		t.Fatalf("Changes = %#v, want %#v", result.Changes, wantChanges)
	}
}

func TestCompareYAML_AddParameterFixture_ClassifiesMinor(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "add-parameter", "before.yaml")
	after := readFixture(t, "add-parameter", "after.yaml")

	result, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("CompareYAML() error = %v", err)
	}
	if result.Classification != contractopenapidiff.ClassificationMinor {
		t.Fatalf("Classification = %q, want %q", result.Classification, contractopenapidiff.ClassificationMinor)
	}

	wantChanges := []contractopenapidiff.Change{
		{Code: contractopenapidiff.CodeParameterAdded, Path: "GET /pets.parameters[query:offset]"},
	}
	if !slices.Equal(result.Changes, wantChanges) {
		t.Fatalf("Changes = %#v, want %#v", result.Changes, wantChanges)
	}
}

func TestCompareYAML_AddSchemaPropertyFixture_ClassifiesMinor(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "add-schema-property", "before.yaml")
	after := readFixture(t, "add-schema-property", "after.yaml")

	result, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("CompareYAML() error = %v", err)
	}
	if result.Classification != contractopenapidiff.ClassificationMinor {
		t.Fatalf("Classification = %q, want %q", result.Classification, contractopenapidiff.ClassificationMinor)
	}

	wantChanges := []contractopenapidiff.Change{
		{Code: contractopenapidiff.CodeSchemaPropertyAdded, Path: "components.schemas.Pet.properties.nickname"},
	}
	if !slices.Equal(result.Changes, wantChanges) {
		t.Fatalf("Changes = %#v, want %#v", result.Changes, wantChanges)
	}
}

func TestCompareYAML_WidenEnumFixture_ClassifiesMinor(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "widen-enum", "before.yaml")
	after := readFixture(t, "widen-enum", "after.yaml")

	result, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("CompareYAML() error = %v", err)
	}
	if result.Classification != contractopenapidiff.ClassificationMinor {
		t.Fatalf("Classification = %q, want %q", result.Classification, contractopenapidiff.ClassificationMinor)
	}

	wantChanges := []contractopenapidiff.Change{
		{Code: contractopenapidiff.CodeEnumValueAdded, Path: "components.schemas.Pet.properties.status.enum.pending"},
	}
	if !slices.Equal(result.Changes, wantChanges) {
		t.Fatalf("Changes = %#v, want %#v", result.Changes, wantChanges)
	}
}

func TestCompareYAML_RelaxParameterRequiredFixture_ClassifiesMinor(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "relax-parameter-required", "before.yaml")
	after := readFixture(t, "relax-parameter-required", "after.yaml")

	result, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("CompareYAML() error = %v", err)
	}
	if result.Classification != contractopenapidiff.ClassificationMinor {
		t.Fatalf("Classification = %q, want %q", result.Classification, contractopenapidiff.ClassificationMinor)
	}

	wantChanges := []contractopenapidiff.Change{
		{Code: contractopenapidiff.CodeParameterRequiredRelaxed, Path: "GET /pets.parameters[query:limit]"},
	}
	if !slices.Equal(result.Changes, wantChanges) {
		t.Fatalf("Changes = %#v, want %#v", result.Changes, wantChanges)
	}
}

func TestCompareYAML_RelaxSchemaRequiredFixture_ClassifiesMinor(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "relax-schema-required", "before.yaml")
	after := readFixture(t, "relax-schema-required", "after.yaml")

	result, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("CompareYAML() error = %v", err)
	}
	if result.Classification != contractopenapidiff.ClassificationMinor {
		t.Fatalf("Classification = %q, want %q", result.Classification, contractopenapidiff.ClassificationMinor)
	}

	wantChanges := []contractopenapidiff.Change{
		{Code: contractopenapidiff.CodeSchemaRequiredRelaxed, Path: "components.schemas.Pet.required.name"},
	}
	if !slices.Equal(result.Changes, wantChanges) {
		t.Fatalf("Changes = %#v, want %#v", result.Changes, wantChanges)
	}
}

func TestCompareYAML_RemoveRouteFixture_ClassifiesMajor(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "remove-route", "before.yaml")
	after := readFixture(t, "remove-route", "after.yaml")

	result, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("CompareYAML() error = %v", err)
	}
	if result.Classification != contractopenapidiff.ClassificationMajor {
		t.Fatalf("Classification = %q, want %q", result.Classification, contractopenapidiff.ClassificationMajor)
	}

	wantChanges := []contractopenapidiff.Change{
		{Code: contractopenapidiff.CodeOperationRemoved, Path: "POST /pets"},
	}
	if !slices.Equal(result.Changes, wantChanges) {
		t.Fatalf("Changes = %#v, want %#v", result.Changes, wantChanges)
	}
}

func TestCompareYAML_RemoveParameterFixture_ClassifiesMajor(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "remove-parameter", "before.yaml")
	after := readFixture(t, "remove-parameter", "after.yaml")

	result, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("CompareYAML() error = %v", err)
	}
	if result.Classification != contractopenapidiff.ClassificationMajor {
		t.Fatalf("Classification = %q, want %q", result.Classification, contractopenapidiff.ClassificationMajor)
	}

	wantChanges := []contractopenapidiff.Change{
		{Code: contractopenapidiff.CodeParameterRemoved, Path: "GET /pets.parameters[query:offset]"},
	}
	if !slices.Equal(result.Changes, wantChanges) {
		t.Fatalf("Changes = %#v, want %#v", result.Changes, wantChanges)
	}
}

func TestCompareYAML_RemoveSchemaPropertyFixture_ClassifiesMajor(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "remove-schema-property", "before.yaml")
	after := readFixture(t, "remove-schema-property", "after.yaml")

	result, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("CompareYAML() error = %v", err)
	}
	if result.Classification != contractopenapidiff.ClassificationMajor {
		t.Fatalf("Classification = %q, want %q", result.Classification, contractopenapidiff.ClassificationMajor)
	}

	wantChanges := []contractopenapidiff.Change{
		{Code: contractopenapidiff.CodeSchemaPropertyRemoved, Path: "components.schemas.Pet.properties.nickname"},
	}
	if !slices.Equal(result.Changes, wantChanges) {
		t.Fatalf("Changes = %#v, want %#v", result.Changes, wantChanges)
	}
}

func TestCompareYAML_NarrowEnumFixture_ClassifiesMajor(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "narrow-enum", "before.yaml")
	after := readFixture(t, "narrow-enum", "after.yaml")

	result, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("CompareYAML() error = %v", err)
	}
	if result.Classification != contractopenapidiff.ClassificationMajor {
		t.Fatalf("Classification = %q, want %q", result.Classification, contractopenapidiff.ClassificationMajor)
	}

	wantChanges := []contractopenapidiff.Change{
		{Code: contractopenapidiff.CodeEnumValueRemoved, Path: "components.schemas.Pet.properties.status.enum.pending"},
	}
	if !slices.Equal(result.Changes, wantChanges) {
		t.Fatalf("Changes = %#v, want %#v", result.Changes, wantChanges)
	}
}

func TestCompareYAML_NarrowTypeFixture_ClassifiesMajor(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "narrow-type", "before.yaml")
	after := readFixture(t, "narrow-type", "after.yaml")

	result, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("CompareYAML() error = %v", err)
	}
	if result.Classification != contractopenapidiff.ClassificationMajor {
		t.Fatalf("Classification = %q, want %q", result.Classification, contractopenapidiff.ClassificationMajor)
	}

	wantChanges := []contractopenapidiff.Change{
		{Code: contractopenapidiff.CodeSchemaTypeNarrowed, Path: "components.schemas.Pet.properties.age.type"},
	}
	if !slices.Equal(result.Changes, wantChanges) {
		t.Fatalf("Changes = %#v, want %#v", result.Changes, wantChanges)
	}
}

func TestCompareYAML_MajorWinsMixedFixture_ClassifiesMajor(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "major-wins-mixed", "before.yaml")
	after := readFixture(t, "major-wins-mixed", "after.yaml")

	result, err := contractopenapidiff.CompareYAML(before, after)
	if err != nil {
		t.Fatalf("CompareYAML() error = %v", err)
	}
	if result.Classification != contractopenapidiff.ClassificationMajor {
		t.Fatalf("Classification = %q, want %q", result.Classification, contractopenapidiff.ClassificationMajor)
	}

	wantChanges := []contractopenapidiff.Change{
		{Code: contractopenapidiff.CodeParameterAdded, Path: "GET /pets.parameters[query:offset]"},
		{Code: contractopenapidiff.CodeOperationRemoved, Path: "POST /pets"},
	}
	if !slices.Equal(result.Changes, wantChanges) {
		t.Fatalf("Changes = %#v, want %#v", result.Changes, wantChanges)
	}
}

func TestCompareYAML_UnsupportedOperationIDFixture_FailsClosed(t *testing.T) {
	t.Parallel()

	before := readFixture(t, "unsupported-operation-id", "before.yaml")
	after := readFixture(t, "unsupported-operation-id", "after.yaml")

	result, err := contractopenapidiff.CompareYAML(before, after)
	if err == nil {
		t.Fatalf("CompareYAML() = %#v, want unsupported diff error", result)
	}
	if !contractopenapidiff.IsUnsupportedDiff(err) {
		t.Fatalf("CompareYAML() error = %v, want unsupported diff refusal", err)
	}

	var unsupported *contractopenapidiff.UnsupportedDiffError
	if !errors.As(err, &unsupported) {
		t.Fatalf("errors.As() = false, want *UnsupportedDiffError")
	}
	if unsupported.Path != "GET /pets.operationId" {
		t.Fatalf("unsupported.Path = %q, want %q", unsupported.Path, "GET /pets.operationId")
	}
}

func TestCompareYAML_SupportedFixtures_StillClassifySuccessfully(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		fixture        string
		classification contractopenapidiff.Classification
	}{
		{name: "docs-only", fixture: "docs-only", classification: contractopenapidiff.ClassificationPatch},
		{name: "add-route", fixture: "add-route", classification: contractopenapidiff.ClassificationMinor},
		{name: "remove-route", fixture: "remove-route", classification: contractopenapidiff.ClassificationMajor},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			before := readFixture(t, tc.fixture, "before.yaml")
			after := readFixture(t, tc.fixture, "after.yaml")

			result, err := contractopenapidiff.CompareYAML(before, after)
			if err != nil {
				t.Fatalf("CompareYAML() error = %v", err)
			}
			if result.Classification != tc.classification {
				t.Fatalf("Classification = %q, want %q", result.Classification, tc.classification)
			}
		})
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
