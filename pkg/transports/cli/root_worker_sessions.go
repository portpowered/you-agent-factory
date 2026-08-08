package cli

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	workersessionscli "github.com/portpowered/infinite-you/pkg/transports/cli/worker_sessions"
	"github.com/spf13/cobra"
)

func productionWorkerSessionsCommand(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	options CommandFactory,
) *cobra.Command {
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you.worker-sessions.list.handler", func(cmd *cobra.Command, _ []string) error {
		return executeGeneratedWorkerSessionsList(cmd, globals, diagnostics, options.ListWorkerSessions)
	}); err != nil {
		panic(fmt.Sprintf("build worker sessions handler registry: %v", err))
	}
	command, err := climanifestcobra.NewWorkerSessionsFamilyCommand(registry)
	if err != nil {
		panic(fmt.Sprintf("build worker sessions family command: %v", err))
	}
	return command
}

func executeGeneratedWorkerSessionsList(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	list workersessionscli.ListOperation,
) error {
	if list == nil {
		return fmt.Errorf("worker sessions list service is required")
	}
	values, err := generatedCommandInputs(cmd)
	if err != nil {
		return err
	}
	workID, err := commandInputValue[string](values, "you.worker-sessions.list.flag.work-id")
	if err != nil {
		return err
	}
	sessionID, err := commandInputValue[string](values, "you.worker-sessions.list.flag.session")
	if err != nil {
		return err
	}
	outputFormat, err := commandInputValue[string](values, "you.worker-sessions.list.flag.output")
	if err != nil {
		return err
	}
	jsonOutput := globals.json || strings.EqualFold(strings.TrimSpace(outputFormat), "json")
	return list(workersessionscli.ListConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		WorkID: workID, OutputFormat: outputFormat, JSON: jsonOutput,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics.writer(cmd),
		Verbose: diagnostics.verboseEnabled(), Debug: diagnostics.debug,
	})
}
