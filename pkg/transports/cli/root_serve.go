package cli

import (
	"fmt"

	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

// productionServeCommand builds the you serve family, whose acp leaf hosts
// the process-composed ACP server over caller-owned stdio.
func productionServeCommand(options CommandFactory) *cobra.Command {
	manifest, err := generated.ServeFamilyManifest()
	if err != nil {
		panic(fmt.Sprintf("load generated serve command manifest: %v", err))
	}
	record := func(id string) (name, short, long string) {
		command, lookupErr := manifest.CommandByID(id)
		if lookupErr != nil {
			panic(fmt.Sprintf("load generated serve command %s: %v", id, lookupErr))
		}
		return command.Name, command.Documentation.Documentation.Title.CanonicalEnglish,
			command.Documentation.Documentation.Description.CanonicalEnglish
	}
	serveName, serveHelp, serveLong := record("you.serve")
	acpName, acpHelp, acpLong := record("you.serve.acp")
	serve := &cobra.Command{Use: serveName, Short: serveHelp, Long: serveLong}
	acpCmd := newServeACPCommand(options)
	acpCmd.Use, acpCmd.Short, acpCmd.Long = acpName, acpHelp, acpLong
	serve.AddCommand(acpCmd)
	return serve
}

// newServeACPCommand hosts You as an ACP agent over the invocation's own
// stdio. It invokes Serve on the exact acp.Server instance Wire composed
// from the process's canonical Chat Sessions and Factory Sessions authority
// -- no command-local ACP server, service graph, or secondary injector is
// constructed, and no HTTP/dashboard listener is started.
func newServeACPCommand(options CommandFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "acp",
		Short: "Host You as an ACP agent over stdio",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if options.acpServer == nil {
				return fmt.Errorf("serve acp: ACP server is required")
			}
			return options.acpServer.Serve(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
}
