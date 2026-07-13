package exemptionbudget

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseRequiresAccountableSortedEntries(t *testing.T) {
	t.Parallel()

	valid := `{
  "version": 1,
  "entries": [
    {
      "rule": "backendsizecheck:ignore-file",
      "target": "pkg/service/factory.go",
      "owner": "backend-maintainers",
      "removalReason": "Split transport wiring from runtime construction."
    }
  ]
}`
	baseline, err := Parse([]byte(valid))
	if err != nil {
		t.Fatalf("Parse() error = %v, want nil", err)
	}
	if len(baseline.Entries) != 1 || baseline.Entries[0].Target != "pkg/service/factory.go" {
		t.Fatalf("Parse() baseline = %+v, want the checked entry", baseline)
	}

	tests := []struct {
		name    string
		replace string
		with    string
		want    string
	}{
		{name: "blank owner", replace: `"backend-maintainers"`, with: `" "`, want: "backendsizecheck:ignore-file/pkg/service/factory.go has empty owner"},
		{name: "blank removal reason", replace: `"Split transport wiring from runtime construction."`, with: `""`, want: "backendsizecheck:ignore-file/pkg/service/factory.go has empty removalReason"},
		{name: "unknown rule", replace: `backendsizecheck:ignore-file`, with: `pkgboundarycheck:ignore-root`, want: "pkgboundarycheck:ignore-root/pkg/service/factory.go has unsupported rule"},
		{name: "unknown field", replace: `"owner":`, with: `"ticket": "ABC-1", "owner":`, want: "unknown field"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := Parse([]byte(strings.Replace(valid, test.replace, test.with, 1)))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Parse() error = %v, want containing %q", err, test.want)
			}
		})
	}
}

func TestValidateRejectsDuplicateAndUnsortedEntries(t *testing.T) {
	t.Parallel()

	entry := Entry{Rule: RuleBackendFile, Target: "pkg/z.go", Owner: "backend-maintainers", RemovalReason: "Split the file."}
	if err := Validate(Baseline{Version: Version, Entries: []Entry{entry, entry}}); err == nil || !strings.Contains(err.Error(), "is duplicated") {
		t.Fatalf("Validate() duplicate error = %v", err)
	}
	first := entry
	second := entry
	second.Target = "pkg/a.go"
	if err := Validate(Baseline{Version: Version, Entries: []Entry{first, second}}); err == nil || !strings.Contains(err.Error(), "appears out of order") {
		t.Fatalf("Validate() ordering error = %v", err)
	}
}

func TestCompareReportsAllDifferencesInDeterministicOrder(t *testing.T) {
	t.Parallel()

	directives := []Directive{
		{Rule: RulePackageComplexity, Target: "pkg/z.go#branchy"},
		{Rule: RuleBackendFile, Target: "pkg/a.go"},
		{Rule: RuleBackendFile, Target: "pkg/a.go"},
		{Rule: RuleBackendFunction, Target: "pkg/new.go#large"},
	}
	baseline := Baseline{Version: Version, Entries: []Entry{
		{Rule: RuleBackendFile, Target: "pkg/a.go"},
		{Rule: RulePackageComplexity, Target: "pkg/stale.go#old"},
		{Rule: RulePackageComplexity, Target: "pkg/z.go#branchy"},
	}}
	want := []Difference{
		{Kind: DifferenceDuplicate, Rule: RuleBackendFile, Target: "pkg/a.go"},
		{Kind: DifferenceUnregistered, Rule: RuleBackendFunction, Target: "pkg/new.go#large"},
		{Kind: DifferenceStale, Rule: RulePackageComplexity, Target: "pkg/stale.go#old"},
	}
	if got := Compare(directives, baseline); !reflect.DeepEqual(got, want) {
		t.Fatalf("Compare() = %#v, want %#v", got, want)
	}
}
