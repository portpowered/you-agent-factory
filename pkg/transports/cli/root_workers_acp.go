package cli

import (
	"fmt"
	"sort"
	"strings"
	"text/tabwriter"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	workersessions "github.com/portpowered/infinite-you/pkg/services/worker_sessions"
	acpcli "github.com/portpowered/infinite-you/pkg/transports/cli/acp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/cliserver"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	workersessionscli "github.com/portpowered/infinite-you/pkg/transports/cli/worker_sessions"
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
func productionWorkerSessionsCommand(
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	options CommandFactory,
) *cobra.Command {
	registry := commandregistry.NewRegistry()
	register := func(handlerID string, handler commandregistry.ResolvedRunE) {
		if err := registry.RegisterResolved(handlerID, handler); err != nil {
			panic(fmt.Sprintf("build worker sessions handler registry: %v", err))
		}
	}
	register("you.worker-sessions.list.handler", resolvedWorkerSessionsHandler(globals, diagnostics, func(cmd *cobra.Command, resolvedGlobals *cliGlobalOptions, resolvedDiagnostics *cliDiagnosticsOptions, values map[string]any) error {
		return executeGeneratedWorkerSessionsListWithValues(cmd, resolvedGlobals, resolvedDiagnostics, options.ListWorkerSessions, values)
	}))
	register("you.worker-sessions.show.handler", resolvedWorkerSessionsHandler(globals, diagnostics, func(cmd *cobra.Command, resolvedGlobals *cliGlobalOptions, resolvedDiagnostics *cliDiagnosticsOptions, values map[string]any) error {
		return executeGeneratedWorkerSessionsShowWithValues(cmd, resolvedGlobals, resolvedDiagnostics, options.ShowWorkerSession, values)
	}))
	register("you.worker-sessions.read.handler", resolvedWorkerSessionsHandler(globals, diagnostics, func(cmd *cobra.Command, resolvedGlobals *cliGlobalOptions, resolvedDiagnostics *cliDiagnosticsOptions, values map[string]any) error {
		return executeGeneratedWorkerSessionsReadWithValues(cmd, resolvedGlobals, resolvedDiagnostics, options.ReadWorkerSession, values)
	}))
	register("you.worker-sessions.stream.handler", resolvedWorkerSessionsHandler(globals, diagnostics, func(cmd *cobra.Command, resolvedGlobals *cliGlobalOptions, resolvedDiagnostics *cliDiagnosticsOptions, values map[string]any) error {
		return executeGeneratedWorkerSessionsStreamWithValues(cmd, resolvedGlobals, resolvedDiagnostics, options.StreamWorkerSession, values)
	}))
	register("you.worker-sessions.invoke.handler", resolvedWorkerSessionsHandler(globals, diagnostics, func(cmd *cobra.Command, resolvedGlobals *cliGlobalOptions, resolvedDiagnostics *cliDiagnosticsOptions, values map[string]any) error {
		return executeGeneratedWorkerSessionsInvokeWithValues(cmd, resolvedWorkerSessionPrompt(values, "you.worker-sessions.invoke.arg.0", nil), resolvedGlobals, resolvedDiagnostics, options.InvokeWorkerSession, options.LocalWorkerSessions, values)
	}))
	register("you.worker-sessions.continue.handler", resolvedWorkerSessionsHandler(globals, diagnostics, func(cmd *cobra.Command, resolvedGlobals *cliGlobalOptions, resolvedDiagnostics *cliDiagnosticsOptions, values map[string]any) error {
		return executeGeneratedWorkerSessionsContinueWithValues(cmd, nil, resolvedGlobals, resolvedDiagnostics, options.ContinueWorkerSession, options.LocalWorkerSessions, values)
	}))
	register("you.worker-sessions.interrupt.handler", resolvedWorkerSessionsHandler(globals, diagnostics, func(cmd *cobra.Command, resolvedGlobals *cliGlobalOptions, resolvedDiagnostics *cliDiagnosticsOptions, values map[string]any) error {
		return executeGeneratedWorkerSessionsInterruptWithValues(cmd, nil, resolvedGlobals, resolvedDiagnostics, options.InterruptWorkerSession, values)
	}))
	register("you.worker-sessions.pause.handler", resolvedWorkerSessionsHandler(globals, diagnostics, func(cmd *cobra.Command, resolvedGlobals *cliGlobalOptions, resolvedDiagnostics *cliDiagnosticsOptions, values map[string]any) error {
		return executeGeneratedWorkerSessionsControlWithValues(cmd, resolvedGlobals, resolvedDiagnostics, options.PauseWorkerSession, options.LocalWorkerSessionControls, workersessions.ControlActionPause, values)
	}))
	register("you.worker-sessions.resume.handler", resolvedWorkerSessionsHandler(globals, diagnostics, func(cmd *cobra.Command, resolvedGlobals *cliGlobalOptions, resolvedDiagnostics *cliDiagnosticsOptions, values map[string]any) error {
		return executeGeneratedWorkerSessionsControlWithValues(cmd, resolvedGlobals, resolvedDiagnostics, options.ResumeWorkerSession, options.LocalWorkerSessionControls, workersessions.ControlActionResume, values)
	}))
	register("you.worker-sessions.cancel.handler", resolvedWorkerSessionsHandler(globals, diagnostics, func(cmd *cobra.Command, resolvedGlobals *cliGlobalOptions, resolvedDiagnostics *cliDiagnosticsOptions, values map[string]any) error {
		return executeGeneratedWorkerSessionsControlWithValues(cmd, resolvedGlobals, resolvedDiagnostics, options.CancelWorkerSession, options.LocalWorkerSessionControls, workersessions.ControlActionCancel, values)
	}))
	register("you.worker-sessions.terminate.handler", resolvedWorkerSessionsHandler(globals, diagnostics, func(cmd *cobra.Command, resolvedGlobals *cliGlobalOptions, resolvedDiagnostics *cliDiagnosticsOptions, values map[string]any) error {
		return executeGeneratedWorkerSessionsControlWithValues(cmd, resolvedGlobals, resolvedDiagnostics, options.TerminateWorkerSession, options.LocalWorkerSessionControls, workersessions.ControlActionTerminate, values)
	}))
	command, err := climanifestcobra.NewWorkerSessionsFamilyCommand(registry)
	if err != nil {
		panic(fmt.Sprintf("build worker sessions family command: %v", err))
	}
	if err := installWorkerSessionsStreamModeConflictGuard(command); err != nil {
		panic(fmt.Sprintf("build worker sessions stream conflict guard: %v", err))
	}
	return command
}

type resolvedWorkerSessionsOperation func(
	*cobra.Command,
	*cliGlobalOptions,
	*cliDiagnosticsOptions,
	map[string]any,
) error

func resolvedWorkerSessionsHandler(
	fallbackGlobals *cliGlobalOptions,
	fallbackDiagnostics *cliDiagnosticsOptions,
	operation resolvedWorkerSessionsOperation,
) commandregistry.ResolvedRunE {
	return func(
		cmd *cobra.Command,
		inputs resolvedinput.Inputs,
		inherited resolvedinput.Inputs,
	) error {
		if operation == nil {
			return fmt.Errorf("worker session operation is required")
		}
		globals, diagnostics, err := resolveWorkerSessionGlobals(
			inherited, fallbackGlobals, fallbackDiagnostics,
		)
		if err != nil {
			return err
		}
		values, err := resolvedWorkerSessionValues(inputs)
		if err != nil {
			return err
		}
		return operation(cmd, &globals, &diagnostics, values)
	}
}

func resolveWorkerSessionGlobals(
	inputs resolvedinput.Inputs,
	fallbackGlobals *cliGlobalOptions,
	fallbackDiagnostics *cliDiagnosticsOptions,
) (cliGlobalOptions, cliDiagnosticsOptions, error) {
	globals := cliGlobalOptions{server: cliserver.DefaultBaseURI}
	if fallbackGlobals != nil {
		globals = *fallbackGlobals
	}
	if _, present := inputs.State("you.flag.server"); present {
		server, err := inputs.String("you.flag.server")
		if err != nil {
			return cliGlobalOptions{}, cliDiagnosticsOptions{}, err
		}
		globals.server = server
	}
	if _, present := inputs.State("you.flag.json"); present {
		jsonOutput, err := inputs.Bool("you.flag.json")
		if err != nil {
			return cliGlobalOptions{}, cliDiagnosticsOptions{}, err
		}
		globals.json = jsonOutput
	}
	if _, present := inputs.State("you.flag.remote"); present {
		remote, err := inputs.Bool("you.flag.remote")
		if err != nil {
			return cliGlobalOptions{}, cliDiagnosticsOptions{}, err
		}
		globals.remote = remote
	}
	diagnostics := cliDiagnosticsOptions{}
	if fallbackDiagnostics != nil {
		diagnostics = *fallbackDiagnostics
	}
	if _, present := inputs.State("you.flag.verbose"); present {
		verbose, err := inputs.Bool("you.flag.verbose")
		if err != nil {
			return cliGlobalOptions{}, cliDiagnosticsOptions{}, err
		}
		diagnostics.verbose = verbose
	}
	if _, present := inputs.State("you.flag.debug"); present {
		debug, err := inputs.Bool("you.flag.debug")
		if err != nil {
			return cliGlobalOptions{}, cliDiagnosticsOptions{}, err
		}
		diagnostics.debug = debug
	}
	return globals, diagnostics, nil
}

func resolvedWorkerSessionValues(inputs resolvedinput.Inputs) (map[string]any, error) {
	manifest, err := generated.WorkerSessionsFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("resolve worker session inputs: %w", err)
	}
	values := make(map[string]any)
	for _, record := range manifest.Commands {
		for inputID := range record.Arguments {
			appendResolvedWorkerSessionValue(values, inputs, inputID)
		}
		for inputID, flag := range record.Flags {
			if flag.Scope == "local" {
				appendResolvedWorkerSessionValue(values, inputs, inputID)
			}
		}
	}
	return values, nil
}

func appendResolvedWorkerSessionValue(
	values map[string]any,
	inputs resolvedinput.Inputs,
	inputID string,
) {
	value, present := inputs.Lookup(inputID)
	if !present {
		return
	}
	switch value.Kind() {
	case resolvedinput.ValueKindBool:
		if typed, err := inputs.Bool(inputID); err == nil {
			values[inputID] = typed
		}
	case resolvedinput.ValueKindString:
		if typed, err := inputs.String(inputID); err == nil {
			values[inputID] = typed
		}
	case resolvedinput.ValueKindInt:
		if typed, err := inputs.Int(inputID); err == nil {
			values[inputID] = typed
		}
	case resolvedinput.ValueKindInt64:
		if typed, err := inputs.Int64(inputID); err == nil {
			values[inputID] = typed
		}
	case resolvedinput.ValueKindStringArray:
		if typed, err := inputs.StringArray(inputID); err == nil {
			values[inputID] = typed
		}
	}
}

func resolvedWorkerSessionPrompt(values map[string]any, inputID string, fallback []string) []string {
	value, present := values[inputID]
	if !present {
		return fallback
	}
	if prompt, ok := value.([]string); ok {
		return prompt
	}
	if prompt, ok := value.(string); ok && prompt != "" {
		return []string{prompt}
	}
	return fallback
}

func requireWorkerSessionsCommand(cmd *cobra.Command) error {
	if cmd == nil {
		return fmt.Errorf("worker sessions command is required")
	}
	return nil
}

func executeGeneratedWorkerSessionsInvokeWithValues(
	cmd *cobra.Command,
	args []string,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	invoke workersessionscli.InvokeOperation,
	local workersessionscli.LocalInvokeBoundary,
	values map[string]any,
) error {
	if invoke == nil {
		return fmt.Errorf("worker sessions invoke service is required")
	}
	if err := requireWorkerSessionsCommand(cmd); err != nil {
		return err
	}
	inputs, err := readGeneratedWorkerSessionsInvokeInputs(values)
	if err != nil {
		return err
	}
	return invoke(workersessionscli.InvokeConfig{
		Context: cmd.Context(), Server: globals.server, Remote: remotePlacementSelected(globals),
		RequestID: inputs.requestID, WorkerSessionID: inputs.workerSessionID, DispatchID: inputs.dispatchID,
		WorkstationName: inputs.workstation, WorkerType: inputs.workerType, RunnerID: inputs.runner,
		Provider: inputs.provider, Model: inputs.model, ReasoningEffort: inputs.reasoningEffort,
		SystemPrompt: inputs.systemPrompt, UserMessage: inputs.userMessage, ExecutionJSON: inputs.execution,
		Prompt: append([]string(nil), args...), Stdin: cmd.InOrStdin(),
		StdinIsTTY: startupcli.StdinIsTTY(cmd.Context()), Async: inputs.async,
		RetryMaxAttempts: inputs.retryMaxAttempts, OutputFormat: inputs.outputFormat,
		JSON:   globals.json || strings.EqualFold(strings.TrimSpace(inputs.outputFormat), "json"),
		Local:  local,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics.writer(cmd),
		Verbose: diagnostics.verboseEnabled(), Debug: diagnostics.debug,
	})
}

type generatedWorkerSessionsInvokeInputs struct {
	execution, requestID, workerSessionID, dispatchID        string
	workstation, workerType, runner, provider, model         string
	reasoningEffort, systemPrompt, userMessage, outputFormat string
	async                                                    bool
	retryMaxAttempts                                         int
}

func readGeneratedWorkerSessionsInvokeInputs(values map[string]any) (generatedWorkerSessionsInvokeInputs, error) {
	var inputs generatedWorkerSessionsInvokeInputs
	readString := func(id string, target *string) error {
		value, err := commandInputValue[string](values, id)
		if err != nil {
			return err
		}
		*target = value
		return nil
	}
	for _, input := range []struct {
		id     string
		target *string
	}{
		{"you.worker-sessions.invoke.flag.execution", &inputs.execution},
		{"you.worker-sessions.invoke.flag.request-id", &inputs.requestID},
		{"you.worker-sessions.invoke.flag.worker-session-id", &inputs.workerSessionID},
		{"you.worker-sessions.invoke.flag.dispatch-id", &inputs.dispatchID},
		{"you.worker-sessions.invoke.flag.workstation", &inputs.workstation},
		{"you.worker-sessions.invoke.flag.worker-type", &inputs.workerType},
		{"you.worker-sessions.invoke.flag.runner", &inputs.runner},
		{"you.worker-sessions.invoke.flag.provider", &inputs.provider},
		{"you.worker-sessions.invoke.flag.model", &inputs.model},
		{"you.worker-sessions.invoke.flag.reasoning-effort", &inputs.reasoningEffort},
		{"you.worker-sessions.invoke.flag.system-prompt", &inputs.systemPrompt},
		{"you.worker-sessions.invoke.flag.user-message", &inputs.userMessage},
		{"you.worker-sessions.invoke.flag.output", &inputs.outputFormat},
	} {
		if err := readString(input.id, input.target); err != nil {
			return generatedWorkerSessionsInvokeInputs{}, err
		}
	}
	var err error
	inputs.async, err = commandInputValue[bool](values, "you.worker-sessions.invoke.flag.async")
	if err != nil {
		return generatedWorkerSessionsInvokeInputs{}, err
	}
	inputs.retryMaxAttempts, err = commandInputValue[int](values, "you.worker-sessions.invoke.flag.retry-max-attempts")
	if err != nil {
		return generatedWorkerSessionsInvokeInputs{}, err
	}
	return inputs, nil
}

func executeGeneratedWorkerSessionsContinueWithValues(
	cmd *cobra.Command,
	args []string,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	continueOperation workersessionscli.ContinueOperation,
	local workersessionscli.LocalInvokeBoundary,
	values map[string]any,
) error {
	if continueOperation == nil {
		return fmt.Errorf("worker sessions continue service is required")
	}
	if err := requireWorkerSessionsCommand(cmd); err != nil {
		return err
	}
	inputs, err := readGeneratedWorkerSessionsContinueInputs(values)
	if err != nil {
		return err
	}
	return continueOperation(workersessionscli.ContinueConfig{
		Context: cmd.Context(), Server: globals.server, Remote: remotePlacementSelected(globals),
		RequestID: inputs.requestID, SourceWorkerSessionID: inputs.sourceWorkerSessionID,
		SuccessorWorkerSessionID: inputs.successorWorkerSessionID, FollowUpInput: inputs.userMessage,
		Prompt: inputs.followUpInput, Stdin: cmd.InOrStdin(), StdinIsTTY: startupcli.StdinIsTTY(cmd.Context()),
		Async: inputs.async, OutputFormat: inputs.outputFormat,
		JSON:  globals.json || strings.EqualFold(strings.TrimSpace(inputs.outputFormat), "json"),
		Local: local, Output: cmd.OutOrStdout(), Diagnostics: diagnostics.writer(cmd),
		Verbose: diagnostics.verboseEnabled(), Debug: diagnostics.debug,
	})
}

type generatedWorkerSessionsContinueInputs struct {
	sourceWorkerSessionID, requestID, successorWorkerSessionID string
	userMessage, outputFormat                                  string
	followUpInput                                              []string
	async                                                      bool
}

func readGeneratedWorkerSessionsContinueInputs(values map[string]any) (generatedWorkerSessionsContinueInputs, error) {
	var inputs generatedWorkerSessionsContinueInputs
	readString := func(id string, target *string) error {
		value, err := commandInputValue[string](values, id)
		if err != nil {
			return err
		}
		*target = value
		return nil
	}
	for _, input := range []struct {
		id     string
		target *string
	}{
		{"you.worker-sessions.continue.arg.0", &inputs.sourceWorkerSessionID},
		{"you.worker-sessions.continue.flag.request-id", &inputs.requestID},
		{"you.worker-sessions.continue.flag.successor-worker-session-id", &inputs.successorWorkerSessionID},
		{"you.worker-sessions.continue.flag.user-message", &inputs.userMessage},
		{"you.worker-sessions.continue.flag.output", &inputs.outputFormat},
	} {
		if err := readString(input.id, input.target); err != nil {
			return generatedWorkerSessionsContinueInputs{}, err
		}
	}
	var err error
	inputs.followUpInput, err = commandInputValue[[]string](values, "you.worker-sessions.continue.arg.1")
	if err != nil {
		return generatedWorkerSessionsContinueInputs{}, err
	}
	inputs.async, err = commandInputValue[bool](values, "you.worker-sessions.continue.flag.async")
	if err != nil {
		return generatedWorkerSessionsContinueInputs{}, err
	}
	return inputs, nil
}

func executeGeneratedWorkerSessionsInterruptWithValues(
	cmd *cobra.Command,
	args []string,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	interrupt workersessionscli.InterruptOperation,
	values map[string]any,
) error {
	if interrupt == nil {
		return fmt.Errorf("worker sessions interrupt service is required")
	}
	if err := requireWorkerSessionsCommand(cmd); err != nil {
		return err
	}
	inputs, err := readGeneratedWorkerSessionsInterruptInputs(values)
	if err != nil {
		return err
	}
	return interrupt(workersessionscli.InterruptConfig{
		Context: cmd.Context(), Server: globals.server, Remote: remotePlacementSelected(globals),
		RequestID: inputs.requestID, SourceWorkerSessionID: inputs.sourceWorkerSessionID,
		SuccessorWorkerSessionID: inputs.successorWorkerSessionID, ReplacementMessage: inputs.userMessage,
		Prompt: inputs.replacementInput, Stdin: cmd.InOrStdin(), StdinIsTTY: startupcli.StdinIsTTY(cmd.Context()),
		Async: inputs.async, OutputFormat: inputs.outputFormat,
		JSON:   globals.json || strings.EqualFold(strings.TrimSpace(inputs.outputFormat), "json"),
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics.writer(cmd),
		Verbose: diagnostics.verboseEnabled(), Debug: diagnostics.debug,
	})
}

type generatedWorkerSessionsInterruptInputs struct {
	sourceWorkerSessionID, requestID, successorWorkerSessionID string
	userMessage, outputFormat                                  string
	replacementInput                                           []string
	async                                                      bool
}

func readGeneratedWorkerSessionsInterruptInputs(values map[string]any) (generatedWorkerSessionsInterruptInputs, error) {
	var inputs generatedWorkerSessionsInterruptInputs
	readString := func(id string, target *string) error {
		value, err := commandInputValue[string](values, id)
		if err != nil {
			return err
		}
		*target = value
		return nil
	}
	for _, input := range []struct {
		id     string
		target *string
	}{
		{"you.worker-sessions.interrupt.arg.0", &inputs.sourceWorkerSessionID},
		{"you.worker-sessions.interrupt.flag.request-id", &inputs.requestID},
		{"you.worker-sessions.interrupt.flag.successor-worker-session-id", &inputs.successorWorkerSessionID},
		{"you.worker-sessions.interrupt.flag.replacement-message", &inputs.userMessage},
		{"you.worker-sessions.interrupt.flag.output", &inputs.outputFormat},
	} {
		if err := readString(input.id, input.target); err != nil {
			return generatedWorkerSessionsInterruptInputs{}, err
		}
	}
	var err error
	inputs.replacementInput, err = commandInputValue[[]string](values, "you.worker-sessions.interrupt.arg.1")
	if err != nil {
		return generatedWorkerSessionsInterruptInputs{}, err
	}
	inputs.async, err = commandInputValue[bool](values, "you.worker-sessions.interrupt.flag.async")
	if err != nil {
		return generatedWorkerSessionsInterruptInputs{}, err
	}
	return inputs, nil
}

func executeGeneratedWorkerSessionsControlWithValues(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	operation func(workersessionscli.ControlConfig) error,
	local workersessionscli.LocalControlBoundary,
	action workersessions.ControlAction,
	values map[string]any,
) error {
	if operation == nil {
		return fmt.Errorf("worker sessions %s service is required", strings.ToLower(string(action)))
	}
	if err := requireWorkerSessionsCommand(cmd); err != nil {
		return err
	}
	commandName := strings.ToLower(string(action))
	workerSessionID, err := commandInputValue[string](values, "you.worker-sessions."+commandName+".arg.0")
	if err != nil {
		return err
	}
	outputFormat, err := commandInputValue[string](values, "you.worker-sessions."+commandName+".flag.output")
	if err != nil {
		return err
	}
	return operation(workersessionscli.ControlConfig{
		Context: cmd.Context(), Server: globals.server, Remote: remotePlacementSelected(globals),
		WorkerSessionID: workerSessionID, Action: action, OutputFormat: outputFormat,
		JSON:   globals.json || strings.EqualFold(strings.TrimSpace(outputFormat), "json"),
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics.writer(cmd),
		Verbose: diagnostics.verboseEnabled(), Debug: diagnostics.debug, Local: local,
	})
}

func installWorkerSessionsStreamModeConflictGuard(command *cobra.Command) error {
	stream, _, err := command.Find([]string{"stream"})
	if err != nil {
		return err
	}
	if stream == nil {
		return fmt.Errorf("worker sessions stream command is unavailable")
	}
	previous := stream.PreRunE
	stream.PreRunE = func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("replay-only") && cmd.Flags().Changed("follow") {
			return workersessionscli.NewStreamModeConflictError()
		}
		if previous != nil {
			return previous(cmd, args)
		}
		return nil
	}
	return nil
}

func executeGeneratedWorkerSessionsListWithValues(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	list workersessionscli.ListOperation,
	values map[string]any,
) error {
	if list == nil {
		return fmt.Errorf("worker sessions list service is required")
	}
	if err := requireWorkerSessionsCommand(cmd); err != nil {
		return err
	}
	workID, err := commandInputValue[string](values, "you.worker-sessions.list.flag.work-id")
	if err != nil {
		return err
	}
	scope, err := commandInputValue[string](values, "you.worker-sessions.list.flag.scope")
	if err != nil {
		return err
	}
	states, err := commandInputValue[[]string](values, "you.worker-sessions.list.flag.state")
	if err != nil {
		return err
	}
	maxResults, err := commandInputValue[int](values, "you.worker-sessions.list.flag.max-results")
	if err != nil {
		return err
	}
	nextToken, err := commandInputValue[string](values, "you.worker-sessions.list.flag.next-token")
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
		WorkID: workID, Scope: scope, States: states, MaxResults: maxResults, NextToken: nextToken,
		OutputFormat: outputFormat, JSON: jsonOutput,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics.writer(cmd),
		Verbose: diagnostics.verboseEnabled(), Debug: diagnostics.debug,
	})
}

func executeGeneratedWorkerSessionsShowWithValues(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	show workersessionscli.ShowOperation,
	values map[string]any,
) error {
	if show == nil {
		return fmt.Errorf("worker sessions show service is required")
	}
	if err := requireWorkerSessionsCommand(cmd); err != nil {
		return err
	}
	provider, err := commandInputValue[string](values, "you.worker-sessions.show.flag.provider")
	if err != nil {
		return err
	}
	workerSessionID, err := commandInputValue[string](values, "you.worker-sessions.show.flag.worker-session-id")
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
		WorkerSessionID: workerSessionID, Provider: provider, Kind: kind, ID: id, OutputFormat: outputFormat, JSON: jsonOutput,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics.writer(cmd),
		Verbose: diagnostics.verboseEnabled(), Debug: diagnostics.debug,
	})
}

func executeGeneratedWorkerSessionsStreamWithValues(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	stream workersessionscli.StreamOperation,
	values map[string]any,
) error {
	if stream == nil {
		return fmt.Errorf("worker sessions stream service is required")
	}
	if err := requireWorkerSessionsCommand(cmd); err != nil {
		return err
	}
	provider, err := commandInputValue[string](values, "you.worker-sessions.stream.flag.provider")
	if err != nil {
		return err
	}
	workerSessionID, err := commandInputValue[string](values, "you.worker-sessions.stream.flag.worker-session-id")
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
	replayOnly, err := commandInputValue[bool](values, "you.worker-sessions.stream.flag.replay-only")
	if err != nil {
		return err
	}
	follow, err := commandInputValue[bool](values, "you.worker-sessions.stream.flag.follow")
	if err != nil {
		return err
	}
	jsonOutput := globals.json || strings.EqualFold(strings.TrimSpace(outputFormat), "json")
	return stream(workersessionscli.StreamConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		WorkerSessionID: workerSessionID, Provider: provider, Kind: kind, ID: id, OutputFormat: outputFormat, JSON: jsonOutput, Follow: follow, ReplayOnly: replayOnly,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics.writer(cmd),
		Verbose: diagnostics.verboseEnabled(), Debug: diagnostics.debug,
	})
}

func executeGeneratedWorkerSessionsReadWithValues(
	cmd *cobra.Command,
	globals *cliGlobalOptions,
	diagnostics *cliDiagnosticsOptions,
	read workersessionscli.ReadOperation,
	values map[string]any,
) error {
	if read == nil {
		return fmt.Errorf("worker sessions read service is required")
	}
	if err := requireWorkerSessionsCommand(cmd); err != nil {
		return err
	}
	provider, err := commandInputValue[string](values, "you.worker-sessions.read.flag.provider")
	if err != nil {
		return err
	}
	workerSessionID, err := commandInputValue[string](values, "you.worker-sessions.read.flag.worker-session-id")
	if err != nil {
		return err
	}
	kind, err := commandInputValue[string](values, "you.worker-sessions.read.flag.kind")
	if err != nil {
		return err
	}
	id, err := commandInputValue[string](values, "you.worker-sessions.read.flag.id")
	if err != nil {
		return err
	}
	sessionID, err := commandInputValue[string](values, "you.worker-sessions.read.flag.session")
	if err != nil {
		return err
	}
	outputFormat, err := commandInputValue[string](values, "you.worker-sessions.read.flag.output")
	if err != nil {
		return err
	}
	jsonOutput := globals.json || strings.EqualFold(strings.TrimSpace(outputFormat), "json")
	return read(workersessionscli.ReadConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		WorkerSessionID: workerSessionID, Provider: provider, Kind: kind, ID: id, OutputFormat: outputFormat, JSON: jsonOutput,
		Output: cmd.OutOrStdout(), Diagnostics: diagnostics.writer(cmd),
		Verbose: diagnostics.verboseEnabled(), Debug: diagnostics.debug,
	})
}
