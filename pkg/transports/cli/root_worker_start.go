package cli

import (
	"fmt"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	workersessionscli "github.com/portpowered/infinite-you/pkg/services/worker_sessions/transports/cli"
	"github.com/spf13/cobra"
)

func executeGeneratedWorkerSessionsStart(cmd *cobra.Command, globals *cliGlobalOptions, diagnostics *cliDiagnosticsOptions, options CommandFactory, values map[string]any) error {
	if options.InvokeWorkerSession == nil {
		return fmt.Errorf("worker sessions start service is required")
	}
	config := workersessionscli.InvokeConfig{
		Context: cmd.Context(), Server: globals.server, PromptStart: true,
		LookupEnv: options.lookupEnv, Prompt: resolvedWorkerSessionPrompt(values, "you.worker-sessions.start.arg.0", nil),
		Stdin: cmd.InOrStdin(), StdinIsTTY: startupcli.StdinIsTTY(cmd.Context()),
		JSON: globals.json, Output: cmd.OutOrStdout(), Diagnostics: diagnostics.writer(cmd),
		Verbose: diagnostics.verboseEnabled(), Debug: diagnostics.debug,
	}
	for _, input := range []struct {
		name   string
		target *string
	}{
		{"request-id", &config.RequestID}, {"worker-session-id", &config.WorkerSessionID},
		{"provider", &config.Provider}, {"model", &config.Model}, {"reasoning-effort", &config.ReasoningEffort},
		{"working-root", &config.WorkingRoot}, {"output", &config.OutputFormat},
	} {
		value, err := commandInputValue[string](values, "you.worker-sessions.start.flag."+input.name)
		if err != nil {
			return err
		}
		*input.target = value
	}
	if cmd.Flags().Changed("timeout-millis") {
		value, err := commandInputValue[int64](values, "you.worker-sessions.start.flag.timeout-millis")
		if err != nil {
			return err
		}
		config.TimeoutMillis = &value
	}
	return options.InvokeWorkerSession(config)
}
