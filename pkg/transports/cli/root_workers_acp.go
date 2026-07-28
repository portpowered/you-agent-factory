package cli

import (
	"fmt"
	"strings"
	"text/tabwriter"

	acpcli "github.com/portpowered/infinite-you/pkg/transports/cli/acp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

func productionWorkersCommand(options CommandFactory) *cobra.Command {
	manifest, err := generated.WorkersFamilyManifest()
	if err != nil {
		panic(fmt.Sprintf("load generated workers command manifest: %v", err))
	}
	record := func(id string) (string, string) {
		command, lookupErr := manifest.CommandByID(id)
		if lookupErr != nil {
			panic(fmt.Sprintf("load generated workers command %s: %v", id, lookupErr))
		}
		return command.Name, command.Documentation.Documentation.Title.CanonicalEnglish
	}
	workersName, workersHelp := record("you.workers")
	acpName, acpHelp := record("you.workers.acp")
	workers := &cobra.Command{Use: workersName, Short: workersHelp}
	acp := &cobra.Command{Use: acpName, Short: acpHelp}
	list, add, deleteCommand := newACPListCommand(options), newACPAddCommand(options), newACPDeleteCommand(options)
	list.Use, list.Short = record("you.workers.acp.list")
	add.Use, add.Short = record("you.workers.acp.add")
	deleteCommand.Use, deleteCommand.Short = record("you.workers.acp.delete")
	acp.AddCommand(list, add, deleteCommand)
	workers.AddCommand(acp)
	return workers
}

func newACPListCommand(options CommandFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List ACP provider integrations and availability",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, err := resolveProcessHomeDir(options)
			if err != nil {
				return err
			}
			response, err := options.acp.List(cmd.Context(), home)
			if err != nil {
				return err
			}
			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			for _, provider := range response.Providers {
				if _, err := fmt.Fprintf(writer, "%s\tACP\t%s\n", provider.ID, provider.Availability); err != nil {
					return err
				}
			}
			return writer.Flush()
		},
	}
}

func newACPAddCommand(options CommandFactory) *cobra.Command {
	var name, transport, command string
	result := &cobra.Command{
		Use:   "add",
		Short: "Add or override one ACP provider integration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := acpcli.ValidateAdd(name, transport, command); err != nil {
				return err
			}
			home, err := resolveProcessHomeDir(options)
			if err != nil {
				return err
			}
			if err := options.acp.Add(cmd.Context(), home, strings.TrimSpace(name), strings.ToLower(strings.TrimSpace(transport)), strings.TrimSpace(command)); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "installed ACP provider %s\n", strings.TrimSpace(name))
			return err
		},
	}
	result.Flags().StringVar(&name, "name", "", "canonical provider identity, such as cursor-acp")
	result.Flags().StringVar(&transport, "transport", "stdio", "ACP transport (stdio)")
	result.Flags().StringVar(&command, "argument", "", "ACP agent launch command")
	_ = result.MarkFlagRequired("name")
	_ = result.MarkFlagRequired("argument")
	return result
}

func newACPDeleteCommand(options CommandFactory) *cobra.Command {
	var name string
	result := &cobra.Command{
		Use:   "delete",
		Short: "Delete one configured ACP provider integration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, err := resolveProcessHomeDir(options)
			if err != nil {
				return err
			}
			if err := options.acp.Delete(cmd.Context(), home, strings.TrimSpace(name)); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "deleted ACP provider %s\n", strings.TrimSpace(name))
			return err
		},
	}
	result.Flags().StringVar(&name, "name", "", "configured ACP provider identity")
	_ = result.MarkFlagRequired("name")
	return result
}
