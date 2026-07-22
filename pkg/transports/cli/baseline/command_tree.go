package baseline

import (
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
	"github.com/spf13/cobra"
)

// SerializeCommandTree records the production Cobra command tree in a
// deterministic textual form. Each line is "<commandPath>\t<use>\t<parentPath>".
// Lines are sorted by command path so repeated runs stay stable.
func SerializeCommandTree(root *cobra.Command) string {
	return cliobservation.SerializeCommandTree(root)
}
