package functionaltestviz

import (
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/internal/functionaltestmetadata"
)

// DebtReport lists stable identities for undocumented and deprecated customer
// debt. Harness records are excluded.
type DebtReport struct {
	Undocumented []string
	Deprecated   []string
}

// BuildDebtReport collects undocumented and deprecated customer identities in
// stable sorted order (file::name).
func BuildDebtReport(records []ClassifiedRecord) DebtReport {
	undocumented := make([]string, 0)
	deprecated := make([]string, 0)
	for _, record := range records {
		if record.Record.Classification != functionaltestmetadata.ClassificationCustomer {
			continue
		}
		identity := record.Record.Identity()
		if record.Undocumented {
			undocumented = append(undocumented, identity)
		}
		if record.Deprecated {
			deprecated = append(deprecated, identity)
		}
	}
	sort.Strings(undocumented)
	sort.Strings(deprecated)
	return DebtReport{
		Undocumented: undocumented,
		Deprecated:   deprecated,
	}
}

// RenderDebtMarkdown renders explicit undocumented and deprecated debt sections.
func RenderDebtMarkdown(report DebtReport) string {
	var b strings.Builder
	b.WriteString("## Documentation debt\n\n")
	b.WriteString("### Undocumented customer tests\n\n")
	b.WriteString(renderDebtIdentityList(report.Undocumented, "No undocumented customer tests."))
	b.WriteString("\n### Deprecated tests\n\n")
	b.WriteString(renderDebtIdentityList(report.Deprecated, "No deprecated tests."))
	return b.String()
}

func renderDebtIdentityList(identities []string, emptyMessage string) string {
	if len(identities) == 0 {
		return "- _" + emptyMessage + "_\n"
	}
	var b strings.Builder
	for _, identity := range identities {
		b.WriteString("- `")
		b.WriteString(identity)
		b.WriteString("`\n")
	}
	return b.String()
}
