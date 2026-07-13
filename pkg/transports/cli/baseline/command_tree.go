package baseline

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// SerializeCommandTree records the production Cobra command tree in a
// deterministic textual form. Each line is "<commandPath>\t<use>\t<parentPath>".
// Lines are sorted by command path so repeated runs stay stable.
func SerializeCommandTree(root *cobra.Command) string {
	lines := collectCommandTreeLines(root)
	sort.Strings(lines)
	return strings.Join(lines, "\n") + "\n"
}

func collectCommandTreeLines(cmd *cobra.Command) []string {
	path := cmd.CommandPath()
	parent := ""
	if cmd.Parent() != nil {
		parent = cmd.Parent().CommandPath()
	}
	lines := []string{path + "\t" + cmd.Use + "\t" + parent}

	children := cmd.Commands()
	sort.Slice(children, func(i, j int) bool {
		left := children[i].CommandPath()
		right := children[j].CommandPath()
		if left == right {
			return children[i].Use < children[j].Use
		}
		return left < right
	})
	for _, child := range children {
		lines = append(lines, collectCommandTreeLines(child)...)
	}
	return lines
}
