package climanifestparity

import (
	"fmt"
	"sort"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/baseline"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/spf13/cobra"
)

// Mismatch names one contracted vs live help/identity drift.
type Mismatch struct {
	CommandID string
	Field     string
	Want      string
	Got       string
}

func (m Mismatch) Error() string {
	return fmt.Sprintf("%s %s mismatch\n--- want ---\n%s--- got ---\n%s", m.CommandID, m.Field, m.Want, m.Got)
}

// CompareHelpIdentity compares contracted help/identity against one live Cobra command.
func CompareHelpIdentity(manifest climanifest.Manifest, record climanifest.Command, cmd *cobra.Command, helpOutput string) []Mismatch {
	var mismatches []Mismatch
	commandID := record.ID

	appendMismatch := func(field, want, got string) {
		if want == got {
			return
		}
		mismatches = append(mismatches, Mismatch{
			CommandID: commandID,
			Field:     field,
			Want:      want,
			Got:       got,
		})
	}

	appendMismatch("path", record.Path, cmd.CommandPath())
	appendMismatch("name", record.Name, cmd.Name())
	appendMismatch("aliases", formatStringList(record.Aliases), formatStringList(cmd.Aliases))
	appendMismatch("visibility", record.Visibility, commandVisibility(cmd))
	appendMismatch("runnable", fmt.Sprintf("%t", record.Runnable), fmt.Sprintf("%t", cmd.Runnable()))
	appendMismatch("shortDescription", record.Documentation.Documentation.Title.CanonicalEnglish, cmd.Short)
	appendMismatch("longDescription", record.Documentation.Documentation.Description.CanonicalEnglish, cmd.Long)
	appendMismatch("usage.line", record.Usage.Line, cmd.Use)
	appendMismatch("usage.example", baseline.NormalizeFixtureText(record.Usage.Example), baseline.NormalizeFixtureText(cmd.Example))
	appendMismatch("documentation.examples", formatStringList(record.Documentation.Examples), formatDocumentationExamples(cmd.Example))
	appendMismatch(
		"normalizedHelpUsageText",
		buildContractHelpIdentityPrefix(manifest, record),
		extractHelpIdentityPrefix(helpOutput),
	)

	return mismatches
}

// CompareModelsHelpIdentity compares contracted models-family help/identity against
// the production-wired live tree. Leaf commands keep title-only Short and empty Long
// during cutover, so normalized help uses the short headline instead of contracted long copy.
func CompareModelsHelpIdentity(
	manifest climanifest.Manifest,
	record climanifest.Command,
	root *cobra.Command,
	cmd *cobra.Command,
) []Mismatch {
	var mismatches []Mismatch
	commandID := record.ID

	appendMismatch := func(field, want, got string) {
		if want == got {
			return
		}
		mismatches = append(mismatches, Mismatch{
			CommandID: commandID,
			Field:     field,
			Want:      want,
			Got:       got,
		})
	}

	appendMismatch("path", record.Path, cmd.CommandPath())
	appendMismatch("name", record.Name, cmd.Name())
	appendMismatch("aliases", formatStringList(record.Aliases), formatStringList(cmd.Aliases))
	appendMismatch("visibility", record.Visibility, commandVisibility(cmd))
	appendMismatch("runnable", fmt.Sprintf("%t", record.Runnable), fmt.Sprintf("%t", cmd.Runnable()))
	appendMismatch("shortDescription", record.Documentation.Documentation.Title.CanonicalEnglish, cmd.Short)
	appendMismatch("usage.line", record.Usage.Line, cmd.Use)
	if strings.TrimSpace(cmd.Example) != "" {
		appendMismatch("usage.example", baseline.NormalizeFixtureText(record.Usage.Example), baseline.NormalizeFixtureText(cmd.Example))
		appendMismatch("documentation.examples", formatStringList(record.Documentation.Examples), formatDocumentationExamples(cmd.Example))
	}

	if record.ID == "you.models" {
		appendMismatch("longDescription", record.Documentation.Documentation.Description.CanonicalEnglish, cmd.Long)
	} else if strings.TrimSpace(cmd.Long) != "" {
		appendMismatch("longDescription", "", cmd.Long)
	}

	helpOutput, err := baseline.CaptureHelpOutput(root, HelpArgsForPath(record.Path))
	if err != nil {
		mismatches = append(mismatches, Mismatch{
			CommandID: commandID,
			Field:     "help.capture",
			Want:      "help output",
			Got:       err.Error(),
		})
		return mismatches
	}

	includeExamples := strings.TrimSpace(cmd.Example) != ""
	var wantHelp string
	switch record.ID {
	case "you.models":
		wantHelp = buildModelsParentHelpIdentityPrefix(manifest, record, includeExamples)
	default:
		wantHelp = buildModelsLeafHelpIdentityPrefix(record, includeExamples)
	}
	appendMismatch(
		"normalizedHelpUsageText",
		wantHelp,
		extractHelpIdentityPrefix(helpOutput),
	)

	return mismatches
}

// CompareDocsHelpIdentity compares contracted docs help/identity against the
// production-wired live tree. The live docs command keeps contracted long help
// but omits Example text during cutover, so example fields are skipped until
// legacy Example wiring returns.
func CompareDocsHelpIdentity(
	manifest climanifest.Manifest,
	record climanifest.Command,
	root *cobra.Command,
	cmd *cobra.Command,
) []Mismatch {
	var mismatches []Mismatch
	commandID := record.ID

	appendMismatch := func(field, want, got string) {
		if want == got {
			return
		}
		mismatches = append(mismatches, Mismatch{
			CommandID: commandID,
			Field:     field,
			Want:      want,
			Got:       got,
		})
	}

	appendMismatch("path", record.Path, cmd.CommandPath())
	appendMismatch("name", record.Name, cmd.Name())
	appendMismatch("aliases", formatStringList(record.Aliases), formatStringList(cmd.Aliases))
	appendMismatch("visibility", record.Visibility, commandVisibility(cmd))
	appendMismatch("runnable", fmt.Sprintf("%t", record.Runnable), fmt.Sprintf("%t", cmd.Runnable()))
	appendMismatch("shortDescription", record.Documentation.Documentation.Title.CanonicalEnglish, cmd.Short)
	appendMismatch("longDescription", record.Documentation.Documentation.Description.CanonicalEnglish, cmd.Long)
	appendMismatch("usage.line", record.Usage.Line, cmd.Use)

	helpOutput, err := baseline.CaptureHelpOutput(root, HelpArgsForPath(record.Path))
	if err != nil {
		mismatches = append(mismatches, Mismatch{
			CommandID: commandID,
			Field:     "help.capture",
			Want:      "help output",
			Got:       err.Error(),
		})
		return mismatches
	}

	appendMismatch(
		"normalizedHelpUsageText",
		buildDocsHelpIdentityPrefix(record),
		extractHelpIdentityPrefix(helpOutput),
	)

	return mismatches
}

func buildDocsHelpIdentityPrefix(record climanifest.Command) string {
	var b strings.Builder
	long := record.Documentation.Documentation.Description.CanonicalEnglish
	b.WriteString(long)
	if !strings.HasSuffix(long, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\nUsage:\n  ")
	b.WriteString(record.Path)
	usageTail := strings.TrimSpace(strings.TrimPrefix(record.Usage.Line, record.Name))
	if usageTail != "" {
		b.WriteString(" ")
		b.WriteString(usageTail)
	}
	b.WriteString(" [flags]")
	return baseline.NormalizeHelpOutput(b.String())
}

func buildModelsParentHelpIdentityPrefix(manifest climanifest.Manifest, record climanifest.Command, includeExamples bool) string {
	var b strings.Builder
	long := record.Documentation.Documentation.Description.CanonicalEnglish
	b.WriteString(long)
	if !strings.HasSuffix(long, "\n") {
		b.WriteString("\n")
	}
	b.WriteString("\nUsage:\n  ")
	b.WriteString(record.Path)
	b.WriteString(" [command]")
	if includeExamples {
		if example := record.Usage.Example; strings.TrimSpace(example) != "" {
			b.WriteString("\n\nExamples:\n")
			b.WriteString(strings.TrimRight(baseline.NormalizeFixtureText(example), "\n"))
		}
	}
	return baseline.NormalizeHelpOutput(b.String())
}

func buildModelsLeafHelpIdentityPrefix(record climanifest.Command, includeExamples bool) string {
	var b strings.Builder
	short := record.Documentation.Documentation.Title.CanonicalEnglish
	b.WriteString(short)
	b.WriteString("\n\nUsage:\n  ")
	b.WriteString(record.Path)
	usageTail := strings.TrimSpace(strings.TrimPrefix(record.Usage.Line, record.Name))
	if usageTail != "" {
		b.WriteString(" ")
		b.WriteString(usageTail)
	}
	b.WriteString(" [flags]")
	if includeExamples {
		if example := record.Usage.Example; strings.TrimSpace(example) != "" {
			b.WriteString("\n\nExamples:\n")
			b.WriteString(strings.TrimRight(baseline.NormalizeFixtureText(example), "\n"))
		}
	}
	return baseline.NormalizeHelpOutput(b.String())
}

func commandVisibility(cmd *cobra.Command) string {
	if cmd.Hidden {
		return "hidden"
	}
	return "visible"
}

func formatStringList(values []string) string {
	if len(values) == 0 {
		return "[]"
	}
	copied := append([]string(nil), values...)
	sort.Strings(copied)
	return fmt.Sprintf("%q", copied)
}

func formatDocumentationExamples(example string) string {
	lines := make([]string, 0)
	for _, line := range strings.Split(example, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		lines = append(lines, trimmed)
	}
	sort.Strings(lines)
	return fmt.Sprintf("%q", lines)
}

func extractHelpIdentityPrefix(help string) string {
	help = baseline.NormalizeHelpOutput(help)
	cutMarkers := []string{"\n\nAvailable Commands:", "\n\nFlags:"}
	end := len(help)
	for _, marker := range cutMarkers {
		if idx := strings.Index(help, marker); idx >= 0 && idx < end {
			end = idx
		}
	}
	prefix := strings.TrimRight(help[:end], "\n")
	if prefix == "" {
		return "\n"
	}
	return prefix + "\n"
}

func buildContractHelpIdentityPrefix(manifest climanifest.Manifest, record climanifest.Command) string {
	var b strings.Builder
	isRoot := record.Path == manifest.RootPath
	long := record.Documentation.Documentation.Description.CanonicalEnglish

	b.WriteString(long)
	if !strings.HasSuffix(long, "\n") {
		b.WriteString("\n")
	}

	b.WriteString("\nUsage:\n")
	if isRoot {
		b.WriteString("  ")
		b.WriteString(manifest.RootPath)
		b.WriteString(" [flags]\n  ")
		b.WriteString(manifest.RootPath)
		b.WriteString(" [command]")
	} else {
		b.WriteString("  ")
		b.WriteString(record.Path)
		usageTail := strings.TrimSpace(strings.TrimPrefix(record.Usage.Line, record.Name))
		if usageTail != "" {
			b.WriteString(" ")
			b.WriteString(usageTail)
		}
		b.WriteString(" [flags]")
	}

	if example := record.Usage.Example; strings.TrimSpace(example) != "" {
		b.WriteString("\n\nExamples:\n")
		b.WriteString(strings.TrimRight(baseline.NormalizeFixtureText(example), "\n"))
	}

	return baseline.NormalizeHelpOutput(b.String())
}

// FindCommandByPath resolves a live command from a manifest path such as "you session show".
func FindCommandByPath(root *cobra.Command, path string) (*cobra.Command, error) {
	parts := strings.Split(path, " ")
	if len(parts) == 0 || parts[0] != root.Name() {
		return nil, fmt.Errorf("path %q does not start at root %q", path, root.Name())
	}

	current := root
	for _, segment := range parts[1:] {
		found := false
		for _, child := range current.Commands() {
			if child.Name() == segment || containsString(child.Aliases, segment) {
				current = child
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("command segment %q not found under %q", segment, current.CommandPath())
		}
	}
	return current, nil
}

// HelpArgsForPath maps a manifest command path to argv for a help invocation.
func HelpArgsForPath(path string) []string {
	parts := strings.Split(path, " ")
	if len(parts) <= 1 {
		return []string{"--help"}
	}
	args := append([]string(nil), parts[1:]...)
	return append(args, "--help")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

// FormatMismatchReport renders reviewer-readable parity failures.
func FormatMismatchReport(mismatches []Mismatch) string {
	if len(mismatches) == 0 {
		return ""
	}
	var b strings.Builder
	for i, mismatch := range mismatches {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(mismatch.Error())
	}
	return b.String()
}
