package baseline

import (
	cliobservation "github.com/portpowered/infinite-you/pkg/transports/cli/observation"
	"github.com/spf13/cobra"
)

// SerializeRunFlags records the production you run flag contract in a
// deterministic textual form. Each line is
// "<name>\t<shorthand>\t<default>\t<usage>" for local and inherited flags.
// Lines are sorted by flag name so repeated runs stay stable.
func SerializeRunFlags(runCmd *cobra.Command) string {
	return cliobservation.SerializeRunFlags(runCmd)
}
