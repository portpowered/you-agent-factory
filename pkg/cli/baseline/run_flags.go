package baseline

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// SerializeRunFlags records the production you run flag contract in a
// deterministic textual form. Each line is
// "<name>\t<shorthand>\t<default>\t<usage>" for local and inherited flags.
// Lines are sorted by flag name so repeated runs stay stable.
func SerializeRunFlags(runCmd *cobra.Command) string {
	lines := collectRunFlagLines(runCmd)
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

func collectRunFlagLines(runCmd *cobra.Command) []string {
	flags := collectRunFlags(runCmd)
	lines := make([]string, 0, len(flags))
	for _, flag := range flags {
		lines = append(lines, formatRunFlagLine(flag))
	}
	return lines
}

func collectRunFlags(runCmd *cobra.Command) []*pflag.Flag {
	byName := map[string]*pflag.Flag{}
	visit := func(flagSet *pflag.FlagSet) {
		if flagSet == nil {
			return
		}
		flagSet.VisitAll(func(flag *pflag.Flag) {
			byName[flag.Name] = flag
		})
	}
	visit(runCmd.InheritedFlags())
	visit(runCmd.Flags())

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	flags := make([]*pflag.Flag, 0, len(names))
	for _, name := range names {
		flags = append(flags, byName[name])
	}
	return flags
}

func formatRunFlagLine(flag *pflag.Flag) string {
	usage := strings.ReplaceAll(flag.Usage, "\t", " ")
	usage = strings.ReplaceAll(usage, "\n", " ")
	return flag.Name + "\t" + flag.Shorthand + "\t" + flag.DefValue + "\t" + usage
}
