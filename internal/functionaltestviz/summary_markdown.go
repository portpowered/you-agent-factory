package functionaltestviz

import (
	"fmt"
	"strings"
)

// RenderCatalogMarkdown renders the functional-test Markdown catalog.
// Golden-backed records must already carry attached provenance; missing
// provenance fails closed. Package coverage is rendered from summary JSON
// fields only.
func RenderCatalogMarkdown(inputs CatalogInputs) (string, error) {
	if err := RequireGoldenProvenance(inputs.Records); err != nil {
		return "", err
	}
	timing := inputs.Timing
	if timing.TestCount > 0 {
		// The source inventory is the authoritative count for declarations in
		// the tree; the timing artifact separately records what the run
		// observed. Keeping both prevents a partial run from looking complete.
		timing.InventoryTestCount = len(inputs.Records)
	}
	var b strings.Builder
	b.WriteString("# Functional tests\n\n")
	b.WriteString(RenderDomainSummariesMarkdown(BuildDomainSummaries(inputs.Records)))
	b.WriteString("\n")
	b.WriteString(RenderDetailCatalogMarkdown(BuildDetailCatalog(inputs.Records)))
	b.WriteString("\n")
	b.WriteString(RenderDebtMarkdown(BuildDebtReport(inputs.Records)))
	b.WriteString("\n")
	b.WriteString(RenderPackageCoverageMarkdown(inputs.Coverage))
	b.WriteString("\n")
	b.WriteString(RenderFunctionalTimingMarkdown(timing))
	return b.String(), nil
}

// RenderDomainSummariesMarkdown renders prioritized domain summary sections in
// DomainBrowseOrder. Empty domains still appear with an explicit zero count.
func RenderDomainSummariesMarkdown(summaries []DomainSummary) string {
	var b strings.Builder
	b.WriteString("## Domain summaries\n")
	for _, summary := range summaries {
		b.WriteString("\n")
		b.WriteString(renderDomainSummarySection(summary))
	}
	return b.String()
}

func renderDomainSummarySection(summary DomainSummary) string {
	var b strings.Builder
	b.WriteString("### ")
	b.WriteString(summary.Domain)
	b.WriteString("\n\n")
	b.WriteString(fmt.Sprintf("- Customer scenarios: %d\n", summary.CustomerScenarios))
	if summary.CustomerScenarios == 0 {
		return b.String()
	}
	if len(summary.Packages) > 0 {
		b.WriteString("- Packages: ")
		b.WriteString(formatNamedCounts(summary.Packages))
		b.WriteString("\n")
	}
	if len(summary.Subsections) > 0 {
		b.WriteString("- Subsections: ")
		b.WriteString(formatNamedCounts(summary.Subsections))
		b.WriteString("\n")
	}
	return b.String()
}

func formatNamedCounts(counts []NamedCount) string {
	parts := make([]string, 0, len(counts))
	for _, item := range counts {
		parts = append(parts, fmt.Sprintf("`%s` (%d)", item.Name, item.Count))
	}
	return strings.Join(parts, ", ")
}
