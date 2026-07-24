package functionaltestviz_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
	"github.com/portpowered/infinite-you/internal/functionaltestviz"
)

func TestBuildDetailCatalogGroupsBrowseOrderAndSeparatesHarness(t *testing.T) {
	t.Parallel()

	records := functionaltestviz.ClassifyRecords([]functionaltestmetadata.Record{
		customerRecord("workers/mock/replace_test.go", "mock", "TestReplace"),
		customerRecord("transport/http/routing/route_test.go", "routing", "TestRoute"),
		customerRecord("transport/cli/process/help_test.go", "process", "TestHelp"),
		customerRecord("runtime_api/session_test.go", "runtime_api", "TestSession"),
		{
			File:           "internal/support/helpers_test.go",
			Package:        "support",
			Name:           "TestHelper",
			Line:           3,
			Classification: functionaltestmetadata.ClassificationHarness,
		},
	})

	catalog := functionaltestviz.BuildDetailCatalog(records)
	if len(catalog.CustomerBuckets) != len(functionaltestviz.DomainBrowseOrder) {
		t.Fatalf("customer buckets len = %d, want %d", len(catalog.CustomerBuckets), len(functionaltestviz.DomainBrowseOrder))
	}
	if catalog.CustomerBuckets[0].Domain != functionaltestviz.DomainBucketTransport {
		t.Fatalf("first bucket = %q, want transport", catalog.CustomerBuckets[0].Domain)
	}
	if len(catalog.CustomerBuckets[0].Records) != 2 {
		t.Fatalf("transport rows = %d, want 2", len(catalog.CustomerBuckets[0].Records))
	}
	if catalog.CustomerBuckets[0].Records[0].Record.Name != "TestHelp" {
		t.Fatalf("transport first row = %q, want TestHelp (file sort)", catalog.CustomerBuckets[0].Records[0].Record.Name)
	}
	if catalog.CustomerBuckets[0].Records[1].Record.Name != "TestRoute" {
		t.Fatalf("transport second row = %q, want TestRoute", catalog.CustomerBuckets[0].Records[1].Record.Name)
	}
	if len(catalog.OtherCustomer) != 1 || catalog.OtherCustomer[0].Record.Name != "TestSession" {
		t.Fatalf("other customer = %#v, want single TestSession", catalog.OtherCustomer)
	}
	if len(catalog.Harness) != 1 || catalog.Harness[0].Record.Name != "TestHelper" {
		t.Fatalf("harness = %#v, want single TestHelper", catalog.Harness)
	}
}

func TestRenderDetailCatalogMarkdownIncludesLabelsAndStableOrdering(t *testing.T) {
	t.Parallel()

	records := functionaltestviz.ClassifyRecords([]functionaltestmetadata.Record{
		{
			File:           "workers/inference/openai/invoke_test.go",
			Package:        "openai",
			Name:           "TestInvoke",
			Line:           42,
			Description:    "verifies provider replay",
			BuildTags:      []string{"functionallong"},
			Golden:         "docs/temp/functional/provider-sessions/openai/invoke/manifest.json",
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
			File:           "internal/support/helpers_test.go",
			Package:        "support",
			Name:           "TestHelper",
			Line:           3,
			Classification: functionaltestmetadata.ClassificationHarness,
		},
	})
	records[0].Provenance = functionaltestviz.GoldenProvenance{
		Provider:      "openai",
		Case:          "invoke",
		FidelityClass: "partial-stream",
		ID:            "openai-invoke",
		ManifestPath:  "docs/temp/functional/provider-sessions/openai/invoke/manifest.json",
	}
	catalog := functionaltestviz.BuildDetailCatalog(records)

	first := functionaltestviz.RenderDetailCatalogMarkdown(catalog)
	second := functionaltestviz.RenderDetailCatalogMarkdown(catalog)
	if first != second {
		t.Fatalf("repeated detail renders diverged:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	if !strings.Contains(first, "## Test catalog\n") {
		t.Fatalf("missing test catalog heading:\n%s", first)
	}
	if !strings.Contains(first, "### workers\n\n- **TestInvoke** — verifies provider replay\n") {
		t.Fatalf("workers detail row missing or wrong:\n%s", first)
	}
	if !strings.Contains(first, "  - Source: `workers/inference/openai/invoke_test.go:42`\n") {
		t.Fatalf("source location missing:\n%s", first)
	}
	if !strings.Contains(first, "  - Labels: long-only, golden-backed\n") {
		t.Fatalf("labels missing:\n%s", first)
	}
	if !strings.Contains(first, "  - Golden provenance:\n    - Provider: `openai`\n") {
		t.Fatalf("golden provenance missing:\n%s", first)
	}
	if !strings.Contains(first, "    - Case: `invoke`\n") {
		t.Fatalf("golden case missing:\n%s", first)
	}
	if !strings.Contains(first, "    - Fidelity class: `partial-stream`\n") {
		t.Fatalf("golden fidelity class missing:\n%s", first)
	}
	if !strings.Contains(first, "    - Golden id: `openai-invoke`\n") {
		t.Fatalf("golden id missing:\n%s", first)
	}
	if !strings.Contains(first, "    - Manifest: `docs/temp/functional/provider-sessions/openai/invoke/manifest.json`\n") {
		t.Fatalf("golden manifest path missing:\n%s", first)
	}
	if !strings.Contains(first, "- **TestLoad** — (undocumented)\n") {
		t.Fatalf("undocumented marker missing:\n%s", first)
	}
	if !strings.Contains(first, "  - Labels: short, undocumented\n") {
		t.Fatalf("undocumented label missing:\n%s", first)
	}
	if !strings.Contains(first, "## Harness verification\n\n- **TestHelper**") {
		t.Fatalf("harness section missing:\n%s", first)
	}
	if strings.Count(first, "TestHelper") != 1 {
		t.Fatalf("harness must not appear in customer catalog:\n%s", first)
	}
}

func TestRenderCatalogMarkdownAppendsDetailAfterDomainSummaries(t *testing.T) {
	t.Parallel()

	records := functionaltestviz.ClassifyRecords([]functionaltestmetadata.Record{
		customerRecord("transport/cli/process/help_test.go", "process", "TestHelp"),
	})
	catalog, err := functionaltestviz.RenderCatalogMarkdown(functionaltestviz.CatalogInputs{Records: records})
	if err != nil {
		t.Fatalf("RenderCatalogMarkdown: %v", err)
	}

	summaryIdx := strings.Index(catalog, "## Domain summaries\n")
	detailIdx := strings.Index(catalog, "## Test catalog\n")
	debtIdx := strings.Index(catalog, "## Documentation debt\n")
	if summaryIdx < 0 || detailIdx < 0 || detailIdx <= summaryIdx {
		t.Fatalf("catalog must contain domain summaries before detail catalog:\n%s", catalog)
	}
	if debtIdx < 0 || debtIdx <= detailIdx {
		t.Fatalf("catalog must append documentation debt after detail catalog:\n%s", catalog)
	}
	if !strings.Contains(catalog, "- **TestHelp** — scenario\n") {
		t.Fatalf("customer detail row missing:\n%s", catalog)
	}
}
