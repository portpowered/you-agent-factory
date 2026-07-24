package functionaltestviz_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
	"github.com/portpowered/infinite-you/internal/functionaltestviz"
)

func TestBuildDebtReportListsStableIdentitiesAndExcludesHarness(t *testing.T) {
	t.Parallel()

	records := functionaltestviz.ClassifyRecords([]functionaltestmetadata.Record{
		{
			File:           "factory/definitions/load_test.go",
			Package:        "definitions",
			Name:           "TestLoad",
			Line:           7,
			Undocumented:   true,
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
		{
			File:           "runtime_api/session_test.go",
			Package:        "runtime_api",
			Name:           "TestSession",
			Line:           3,
			Description:    "legacy session path",
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
		{
			File:           "runtime_api/zzzz_other_test.go",
			Package:        "runtime_api",
			Name:           "TestOther",
			Line:           9,
			Undocumented:   true,
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
		{
			File:           "internal/support/helpers_test.go",
			Package:        "support",
			Name:           "TestHelper",
			Line:           3,
			Undocumented:   true,
			Classification: functionaltestmetadata.ClassificationHarness,
		},
		{
			File:           "aaa/first_undocumented_test.go",
			Package:        "first",
			Name:           "TestFirst",
			Line:           1,
			Undocumented:   true,
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
	})

	report := functionaltestviz.BuildDebtReport(records)
	wantUndocumented := []string{
		"aaa/first_undocumented_test.go::TestFirst",
		"factory/definitions/load_test.go::TestLoad",
		"runtime_api/zzzz_other_test.go::TestOther",
	}
	if len(report.Undocumented) != len(wantUndocumented) {
		t.Fatalf("undocumented = %#v, want %#v", report.Undocumented, wantUndocumented)
	}
	for i := range wantUndocumented {
		if report.Undocumented[i] != wantUndocumented[i] {
			t.Fatalf("undocumented[%d] = %q, want %q", i, report.Undocumented[i], wantUndocumented[i])
		}
	}
	wantDeprecated := []string{
		"runtime_api/session_test.go::TestSession",
		"runtime_api/zzzz_other_test.go::TestOther",
	}
	if len(report.Deprecated) != len(wantDeprecated) {
		t.Fatalf("deprecated = %#v, want %#v", report.Deprecated, wantDeprecated)
	}
	for i := range wantDeprecated {
		if report.Deprecated[i] != wantDeprecated[i] {
			t.Fatalf("deprecated[%d] = %q, want %q", i, report.Deprecated[i], wantDeprecated[i])
		}
	}
}

func TestRenderDebtMarkdownIsStableAndExplicit(t *testing.T) {
	t.Parallel()

	report := functionaltestviz.DebtReport{
		Undocumented: []string{"factory/definitions/load_test.go::TestLoad"},
		Deprecated:   []string{"runtime_api/session_test.go::TestSession"},
	}
	first := functionaltestviz.RenderDebtMarkdown(report)
	second := functionaltestviz.RenderDebtMarkdown(report)
	if first != second {
		t.Fatalf("repeated debt renders diverged:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	if !strings.Contains(first, "## Documentation debt\n\n### Undocumented customer tests\n\n") {
		t.Fatalf("undocumented debt heading missing:\n%s", first)
	}
	if !strings.Contains(first, "- `factory/definitions/load_test.go::TestLoad`\n") {
		t.Fatalf("undocumented identity missing:\n%s", first)
	}
	if !strings.Contains(first, "### Deprecated tests\n\n- `runtime_api/session_test.go::TestSession`\n") {
		t.Fatalf("deprecated identity missing:\n%s", first)
	}

	empty := functionaltestviz.RenderDebtMarkdown(functionaltestviz.DebtReport{})
	if !strings.Contains(empty, "- _No undocumented customer tests._\n") {
		t.Fatalf("empty undocumented presentation missing:\n%s", empty)
	}
	if !strings.Contains(empty, "- _No deprecated tests._\n") {
		t.Fatalf("empty deprecated presentation missing:\n%s", empty)
	}
}

func TestRenderCatalogMarkdownIncludesDebtSections(t *testing.T) {
	t.Parallel()

	records := functionaltestviz.ClassifyRecords([]functionaltestmetadata.Record{
		{
			File:           "factory/definitions/load_test.go",
			Package:        "definitions",
			Name:           "TestLoad",
			Line:           7,
			Undocumented:   true,
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
		{
			File:           "runtime_api/session_test.go",
			Package:        "runtime_api",
			Name:           "TestSession",
			Line:           3,
			Description:    "legacy session path",
			Classification: functionaltestmetadata.ClassificationCustomer,
		},
	})
	catalog, err := functionaltestviz.RenderCatalogMarkdown(functionaltestviz.CatalogInputs{Records: records})
	if err != nil {
		t.Fatalf("RenderCatalogMarkdown: %v", err)
	}
	if !strings.Contains(catalog, "  - Labels: short, undocumented\n") {
		t.Fatalf("undocumented row label missing:\n%s", catalog)
	}
	if !strings.Contains(catalog, "  - Labels: short, deprecated\n") {
		t.Fatalf("deprecated row label missing:\n%s", catalog)
	}
	if !strings.Contains(catalog, "## Documentation debt\n") {
		t.Fatalf("debt section missing:\n%s", catalog)
	}
	if !strings.Contains(catalog, "- `factory/definitions/load_test.go::TestLoad`\n") {
		t.Fatalf("undocumented debt identity missing:\n%s", catalog)
	}
	if !strings.Contains(catalog, "- `runtime_api/session_test.go::TestSession`\n") {
		t.Fatalf("deprecated debt identity missing:\n%s", catalog)
	}
}
