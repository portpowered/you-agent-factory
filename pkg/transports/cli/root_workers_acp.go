package cli

import (
	"fmt"
	"sort"
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
	listName, listHelp := record("you.workers.list")
	acpName, acpHelp := record("you.workers.acp")
	workers := &cobra.Command{Use: workersName, Short: workersHelp}
	acp := &cobra.Command{
		Use: acpName, Short: acpHelp, Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	listWorkers := newWorkersListCommand(options)
	listWorkers.Use, listWorkers.Short = listName, listHelp
	add, deleteCommand := newACPAddCommand(options), newACPDeleteCommand(options)
	add.Use, add.Short = record("you.workers.acp.add")
	deleteCommand.Use, deleteCommand.Short = record("you.workers.acp.delete")
	acp.AddCommand(add, deleteCommand)
	workers.AddCommand(listWorkers, acp)
	return workers
}

func newWorkersListCommand(options CommandFactory) *cobra.Command {
	return &cobra.Command{
		Use: "list", Short: "List worker integrations", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			home, err := resolveProcessHomeDir(options)
			if err != nil {
				return err
			}
			catalog, err := options.acp.ListWorkers(cmd.Context(), home)
			if err != nil {
				return err
			}
			sort.Slice(catalog.Providers, func(i, j int) bool { return catalog.Providers[i].ID < catalog.Providers[j].ID })
			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
			if _, err := fmt.Fprintln(writer, "NAME\tTYPE\tAVAILABILITY\tSOURCE"); err != nil {
				return err
			}
			for _, provider := range catalog.Providers {
				kind, source := "AGENT", "built-in"
				if catalog.ACP[provider.ID] {
					kind = "AGENT-ACP"
				}
				if catalog.Custom[provider.ID] {
					source = "custom"
				}
				if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n", provider.ID, kind, provider.Readiness, source); err != nil {
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
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "install succeeded: %s\n", strings.TrimSpace(name))
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
