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
	if err := registry.Register("you.worker-sessions.show.handler", func(cmd *cobra.Command, _ []string) error {
		return executeGeneratedWorkerSessionsShow(cmd, globals, diagnostics, options.ShowWorkerSession)
	}); err != nil {
		panic(fmt.Sprintf("build worker sessions handler registry: %v", err))
	}
	if err := registry.Register("you.worker-sessions.stream.handler", func(cmd *cobra.Command, _ []string) error {
		return executeGeneratedWorkerSessionsStream(cmd, globals, diagnostics, options.StreamWorkerSession)
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

func executeGeneratedWorkerSessionsShow(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	show workersessionscli.ShowOperation,
) error {
	if show == nil {
		return fmt.Errorf("worker sessions show service is required")
	}
	values, err := generatedCommandInputs(cmd)
	if err != nil {
		return err
	}
	provider, err := commandInputValue[string](values, "you.worker-sessions.show.flag.provider")
	if err != nil {
		return err
	}
	kind, err := commandInputValue[string](values, "you.worker-sessions.show.flag.kind")
	if err != nil {
		return err
	}
	id, err := commandInputValue[string](values, "you.worker-sessions.show.flag.id")
	if err != nil {
		return err
	}
	sessionID, err := commandInputValue[string](values, "you.worker-sessions.show.flag.session")
	if err != nil {
		return err
	}
	outputFormat, err := commandInputValue[string](values, "you.worker-sessions.show.flag.output")
	if err != nil {
		return err
	}
	jsonOutput := globals.json || strings.EqualFold(strings.TrimSpace(outputFormat), "json")
	return show(workersessionscli.ShowConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		Provider: provider, Kind: kind, ID: id, OutputFormat: outputFormat, JSON: jsonOutput,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics.writer(cmd),
		Verbose: diagnostics.verboseEnabled(), Debug: diagnostics.debug,
	})
}

func executeGeneratedWorkerSessionsStream(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	stream workersessionscli.StreamOperation,
) error {
	if stream == nil {
		return fmt.Errorf("worker sessions stream service is required")
	}
	values, err := generatedCommandInputs(cmd)
	if err != nil {
		return err
	}
	provider, err := commandInputValue[string](values, "you.worker-sessions.stream.flag.provider")
	if err != nil {
		return err
	}
	kind, err := commandInputValue[string](values, "you.worker-sessions.stream.flag.kind")
	if err != nil {
		return err
	}
	id, err := commandInputValue[string](values, "you.worker-sessions.stream.flag.id")
	if err != nil {
		return err
	}
	sessionID, err := commandInputValue[string](values, "you.worker-sessions.stream.flag.session")
	if err != nil {
		return err
	}
	outputFormat, err := commandInputValue[string](values, "you.worker-sessions.stream.flag.output")
	if err != nil {
		return err
	}
	jsonOutput := globals.json || strings.EqualFold(strings.TrimSpace(outputFormat), "json")
	return stream(workersessionscli.StreamConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		Provider: provider, Kind: kind, ID: id, OutputFormat: outputFormat, JSON: jsonOutput,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics.writer(cmd),
		Verbose: diagnostics.verboseEnabled(), Debug: diagnostics.debug,
	})
}
