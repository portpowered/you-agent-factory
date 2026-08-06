package acpbaseline

import (
	"fmt"
	"sort"
	"strings"
)

// OurAgentName identifies our own matrix among the compared set. Verdicts are
// computed relative to it.
const OurAgentName = "you"

// Verdict classifies one capability row.
type Verdict string

const (
	// VerdictGap means at least one third party has it and we do not. Every
	// GAP row is a work item.
	VerdictGap Verdict = "GAP"
	// VerdictExtra means only we have it.
	VerdictExtra Verdict = "EXTRA"
	// VerdictParity means presence matches, including when nobody has it.
	VerdictParity Verdict = "PARITY"
)

// Row is one compared capability.
type Row struct {
	Capability string
	Present    map[string]bool
	Detail     map[string]string
	Verdict    Verdict
}

// Compare computes the capability rows across every captured matrix.
//
// Verdicts are computed rather than authored, so a row cannot quietly disagree
// with the captures it claims to summarize.
func Compare(matrices []*CapabilityMatrix) []Row {
	agents := make([]string, 0, len(matrices))
	for _, matrix := range matrices {
		agents = append(agents, matrix.Agent)
	}

	capabilities := map[string]bool{}
	presence := map[string]map[string]bool{}
	detail := map[string]map[string]string{}

	record := func(capability, agent string, present bool, note string) {
		capabilities[capability] = true
		if presence[capability] == nil {
			presence[capability] = map[string]bool{}
			detail[capability] = map[string]string{}
		}
		if present {
			presence[capability][agent] = true
		}
		if note != "" {
			detail[capability][agent] = note
		}
	}

	for _, matrix := range matrices {
		for _, variant := range sortedKeys(matrix.SessionUpdateVariants) {
			record("session/update -> "+variant, matrix.Agent, true,
				fmt.Sprintf("%d", matrix.SessionUpdateVariants[variant]))
		}
		for _, method := range matrix.AgentMethodsAccepted {
			record("agent method "+method, matrix.Agent, true, "")
		}
		for method, code := range matrix.AgentMethodsRejected {
			record("agent method "+method, matrix.Agent, false, fmt.Sprintf("%d", code))
		}
		for _, method := range matrix.ClientMethodsInvoked {
			if strings.HasPrefix(method, "session/update") {
				continue
			}
			record("client method "+method, matrix.Agent, true, "")
		}
		for _, capability := range matrix.AgentCapabilities {
			record("capability "+capability, matrix.Agent, true, "")
		}
		if matrix.ConfigOptionCount > 0 {
			record("config option category="+matrix.ConfigOptionCategory, matrix.Agent, true,
				fmt.Sprintf("%d options", matrix.ConfigOptionCount))
		}
	}

	names := make([]string, 0, len(capabilities))
	for capability := range capabilities {
		names = append(names, capability)
	}
	sort.Strings(names)

	rows := make([]Row, 0, len(names))
	for _, capability := range names {
		row := Row{
			Capability: capability,
			Present:    map[string]bool{},
			Detail:     detail[capability],
		}
		for _, agent := range agents {
			row.Present[agent] = presence[capability][agent]
		}
		row.Verdict = verdictFor(row, agents)
		rows = append(rows, row)
	}
	return rows
}

func verdictFor(row Row, agents []string) Verdict {
	ours := row.Present[OurAgentName]
	othersHave := false
	for _, agent := range agents {
		if agent != OurAgentName && row.Present[agent] {
			othersHave = true
		}
	}
	switch {
	case othersHave && !ours:
		return VerdictGap
	case ours && !othersHave && len(agents) > 1:
		return VerdictExtra
	default:
		return VerdictParity
	}
}

// RenderComparison writes the comparison as reviewable Markdown, with every
// GAP row phrased so it can be pasted straight into a work item.
func RenderComparison(matrices []*CapabilityMatrix, rows []Row) string {
	agents := make([]string, 0, len(matrices))
	for _, matrix := range matrices {
		agents = append(agents, matrix.Agent)
	}

	var out strings.Builder
	out.WriteString("# ACP capability comparison\n\n")
	out.WriteString("Computed from captured transcripts by `acpbaseline compare`. ")
	out.WriteString("Every **GAP** row is a work item: a capability at least one third-party ")
	out.WriteString("agent exhibits and `you serve acp` does not.\n\n")

	out.WriteString("Captures compared:\n\n")
	for _, matrix := range matrices {
		out.WriteString(fmt.Sprintf("- `%s` — %s, scenarios: %s\n",
			matrix.Agent, matrix.CapturedAtUTC, strings.Join(matrix.Scenarios, ", ")))
	}
	out.WriteString("\nModel and option identities are account-entitlement-scoped, so verdicts key ")
	out.WriteString("on a capability's existence and category, never on the exact option ids.\n")

	for _, matrix := range matrices {
		for _, caveat := range matrix.Caveats {
			out.WriteString(fmt.Sprintf("\n> **Caveat (%s):** %s\n", matrix.Agent, caveat))
		}
	}
	out.WriteString("\n")

	out.WriteString("| Capability |")
	for _, agent := range agents {
		out.WriteString(" " + agent + " |")
	}
	out.WriteString(" Verdict |\n|---|")
	for range agents {
		out.WriteString("---|")
	}
	out.WriteString("---|\n")

	gaps := 0
	for _, row := range rows {
		out.WriteString("| `" + row.Capability + "` |")
		for _, agent := range agents {
			out.WriteString(" " + cell(row, agent) + " |")
		}
		out.WriteString(" " + string(row.Verdict) + " |\n")
		if row.Verdict == VerdictGap {
			gaps++
		}
	}

	out.WriteString(fmt.Sprintf("\n**%d GAP row(s).**\n", gaps))
	if gaps > 0 {
		out.WriteString("\n## Work items\n\n")
		for _, row := range rows {
			if row.Verdict != VerdictGap {
				continue
			}
			out.WriteString(fmt.Sprintf(
				"- `you serve acp` does not exhibit `%s`, which %s does. Decide whether to implement it or record why it does not apply.\n",
				row.Capability, strings.Join(othersWith(row, agents), ", ")))
		}
	}
	return out.String()
}

func cell(row Row, agent string) string {
	if !row.Present[agent] {
		if note, ok := row.Detail[agent]; ok && note != "" && note != "0" {
			return "no (" + note + ")"
		}
		return "no"
	}
	if note, ok := row.Detail[agent]; ok && note != "" {
		return "yes (" + note + ")"
	}
	return "yes"
}

func othersWith(row Row, agents []string) []string {
	var names []string
	for _, agent := range agents {
		if agent != OurAgentName && row.Present[agent] {
			names = append(names, agent)
		}
	}
	return names
}
