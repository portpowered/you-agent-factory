package functionaltestviz_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
	"github.com/portpowered/infinite-you/internal/functionaltestviz"
)

func TestBuildDomainSummariesUsesBrowseOrderAndExcludesHarness(t *testing.T) {
	t.Parallel()

	records := functionaltestviz.ClassifyRecords([]functionaltestmetadata.Record{
		customerRecord("transport/cli/process/help_test.go", "process", "TestHelp"),
		customerRecord("transport/http/routing/route_test.go", "routing", "TestRoute"),
		customerRecord("workers/mock/replace_test.go", "mock", "TestReplace"),
		customerRecord("guards/global_test.go", "guards", "TestGlobal"),
		customerRecord("resources/concurrency_test.go", "resources", "TestConcurrency"),
		customerRecord("observability/logging/redaction_test.go", "logging", "TestRedaction"),
		customerRecord("product/docs/contract_test.go", "docs", "TestContract"),
		customerRecord("resilience/process/restart_test.go", "process", "TestRestart"),
		customerRecord("runtime_api/session_test.go", "runtime_api", "TestSession"),
		{
			File:           "internal/support/helpers_test.go",
			Package:        "support",
			Name:           "TestHelper",
			Line:           3,
			Classification: functionaltestmetadata.ClassificationHarness,
		},
	})

	summaries := functionaltestviz.BuildDomainSummaries(records)
	if len(summaries) != len(functionaltestviz.DomainBrowseOrder) {
		t.Fatalf("summaries len = %d, want %d", len(summaries), len(functionaltestviz.DomainBrowseOrder))
	}
	for i, wantDomain := range functionaltestviz.DomainBrowseOrder {
		if summaries[i].Domain != wantDomain {
			t.Fatalf("summaries[%d].Domain = %q, want %q", i, summaries[i].Domain, wantDomain)
		}
	}

	byDomain := map[string]functionaltestviz.DomainSummary{}
	for _, summary := range summaries {
		byDomain[summary.Domain] = summary
	}

	assertSummaryCount(t, byDomain["transport"], 2)
	assertNamedCounts(t, byDomain["transport"].Packages, []functionaltestviz.NamedCount{
		{Name: "process", Count: 1},
		{Name: "routing", Count: 1},
	})
	assertNamedCounts(t, byDomain["transport"].Subsections, []functionaltestviz.NamedCount{
		{Name: "cli", Count: 1},
		{Name: "http", Count: 1},
	})

	assertSummaryCount(t, byDomain["workers"], 1)
	assertSummaryCount(t, byDomain["orchestration"], 0)
	assertSummaryCount(t, byDomain["workstations"], 0)
	assertSummaryCount(t, byDomain["work"], 0)
	assertSummaryCount(t, byDomain["sessions"], 0)
	assertSummaryCount(t, byDomain["factory"], 0)
	assertSummaryCount(t, byDomain["provider_sessions"], 0)
	assertSummaryCount(t, byDomain["events"], 0)
	assertSummaryCount(t, byDomain["models"], 0)
	assertSummaryCount(t, byDomain[functionaltestviz.DomainBucketGuardsResources], 2)
	assertSummaryCount(t, byDomain[functionaltestviz.DomainBucketObservabilityProductResilience], 3)

	// Harness + runtime_api must not inflate prioritized customer totals.
	totalCustomer := 0
	for _, summary := range summaries {
		totalCustomer += summary.CustomerScenarios
	}
	if totalCustomer != 8 {
		t.Fatalf("browse-order customer total = %d, want 8 (harness and runtime_api excluded)", totalCustomer)
	}
}

func TestRenderDomainSummariesMarkdownIsStableAndIncludesZeros(t *testing.T) {
	t.Parallel()

	records := functionaltestviz.ClassifyRecords([]functionaltestmetadata.Record{
		customerRecord("transport/cli/process/help_test.go", "process", "TestHelp"),
		customerRecord("workers/inference/openai/invoke_test.go", "openai", "TestInvoke"),
		{
			File:           "internal/support/helpers_test.go",
			Package:        "support",
			Name:           "TestHelper",
			Line:           3,
			Classification: functionaltestmetadata.ClassificationHarness,
		},
	})
	summaries := functionaltestviz.BuildDomainSummaries(records)

	first := functionaltestviz.RenderDomainSummariesMarkdown(summaries)
	second := functionaltestviz.RenderDomainSummariesMarkdown(summaries)
	if first != second {
		t.Fatalf("repeated renders diverged:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	catalog := functionaltestviz.RenderCatalogMarkdown(functionaltestviz.CatalogInputs{Records: records})
	if !strings.HasPrefix(catalog, "# Functional tests\n\n## Domain summaries\n") {
		t.Fatalf("catalog does not begin with title + domain summaries:\n%s", catalog)
	}
	if !strings.Contains(catalog, "### transport\n\n- Customer scenarios: 1\n") {
		t.Fatalf("transport summary missing or wrong:\n%s", catalog)
	}
	if !strings.Contains(catalog, "- Packages: `process` (1)\n") {
		t.Fatalf("transport package tally missing:\n%s", catalog)
	}
	if !strings.Contains(catalog, "- Subsections: `cli` (1)\n") {
		t.Fatalf("transport subsection tally missing:\n%s", catalog)
	}
	if !strings.Contains(catalog, "### orchestration\n\n- Customer scenarios: 0\n") {
		t.Fatalf("zero orchestration domain missing:\n%s", catalog)
	}
	summarySection := catalog
	if detailIdx := strings.Index(catalog, "## Test catalog\n"); detailIdx >= 0 {
		summarySection = catalog[:detailIdx]
	}
	if strings.Contains(summarySection, "support") || strings.Contains(summarySection, "TestHelper") {
		t.Fatalf("harness leaked into domain summaries:\n%s", summarySection)
	}

	// Browse-order headings must appear exactly once each, in order.
	offset := 0
	for _, domain := range functionaltestviz.DomainBrowseOrder {
		heading := "### " + domain + "\n"
		idx := strings.Index(catalog[offset:], heading)
		if idx < 0 {
			t.Fatalf("missing ordered heading %q after offset %d in:\n%s", domain, offset, catalog)
		}
		offset += idx + len(heading)
	}
}

func TestSubsectionFromFileHandlesRepoRelativeAndRootRelativePaths(t *testing.T) {
	t.Parallel()

	cases := []struct {
		file string
		want string
	}{
		{file: "transport/cli/process/help_test.go", want: "cli"},
		{file: "tests/functional/sessions/lifecycle/open_test.go", want: "lifecycle"},
		{file: `guards\policy_test.go`, want: ""},
		{file: "models/catalog_test.go", want: ""},
		{file: "", want: ""},
	}
	for _, tc := range cases {
		if got := functionaltestviz.SubsectionFromFile(tc.file); got != tc.want {
			t.Fatalf("SubsectionFromFile(%q) = %q, want %q", tc.file, got, tc.want)
		}
	}
}

func customerRecord(file, pkg, name string) functionaltestmetadata.Record {
	return functionaltestmetadata.Record{
		File:           file,
		Package:        pkg,
		Name:           name,
		Line:           10,
		Description:    "scenario",
		Classification: functionaltestmetadata.ClassificationCustomer,
	}
}

func assertSummaryCount(t *testing.T, summary functionaltestviz.DomainSummary, want int) {
	t.Helper()
	if summary.CustomerScenarios != want {
		t.Fatalf("%s customer scenarios = %d, want %d", summary.Domain, summary.CustomerScenarios, want)
	}
}

func assertNamedCounts(t *testing.T, got, want []functionaltestviz.NamedCount) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("named counts len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("named counts[%d] = %#v, want %#v", i, got[i], want[i])
		}
	}
}
