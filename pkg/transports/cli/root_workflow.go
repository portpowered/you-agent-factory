package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	fse "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	sessioncli "github.com/portpowered/infinite-you/pkg/services/factory_sessions/transports/cli/session"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func newSessionHandlerRegistry(
	options CommandFactory,
) (climanifestcobra.CobraHandlerRegistry, error) {
	adapter := sessionCommandAdapter{options: options}
	return climanifestcobra.CobraHandlerRegistry{
		"you.session.create.handler":     adapter.create,
		"you.session.list.handler":       adapter.list,
		"you.session.show.handler":       adapter.show,
		"you.session.delete.handler":     adapter.delete,
		"you.session.dispatches.handler": adapter.dispatches,
		"you.session.pause.handler":      adapter.pause,
		"you.session.resume.handler":     adapter.resume,
	}, nil
}

type sessionCommandAdapter struct {
	options CommandFactory
}

type sessionGlobalInputs struct {
	server      string
	json        bool
	verbose     bool
	debug       bool
	diagnostics io.Writer
}

func readSessionGlobalInputs(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
) (sessionGlobalInputs, error) {
	server, err := inputs.String("you.flag.server")
	if err != nil {
		return sessionGlobalInputs{}, err
	}
	jsonOutput, err := inputs.Bool("you.flag.json")
	if err != nil {
		return sessionGlobalInputs{}, err
	}
	verbose, err := inputs.Bool("you.flag.verbose")
	if err != nil {
		return sessionGlobalInputs{}, err
	}
	debug, err := inputs.Bool("you.flag.debug")
	if err != nil {
		return sessionGlobalInputs{}, err
	}
	diagnostics := io.Writer(nil)
	if verbose || debug {
		diagnostics = cmd.ErrOrStderr()
	}
	return sessionGlobalInputs{
		server: server, json: jsonOutput, verbose: verbose, debug: debug,
		diagnostics: diagnostics,
	}, nil
}

func sessionValue[T any](values map[string]any, inputID string) (T, error) {
	var zero T
	value, ok := values[inputID]
	if !ok || value == nil {
		return zero, nil
	}
	typed, ok := value.(T)
	if !ok {
		return zero, fmt.Errorf("resolved session input %q has incompatible type %T", inputID, value)
	}
	return typed, nil
}

func sessionGlobals(
	cmd *cobra.Command,
	inherited resolvedinput.Inputs,
) (sessionGlobalInputs, error) {
	globals, err := readSessionGlobalInputs(cmd, inherited)
	if err != nil {
		return sessionGlobalInputs{}, fmt.Errorf("resolve session inputs: %w", err)
	}
	return globals, nil
}

func (a sessionCommandAdapter) create(
	cmd *cobra.Command,
	_ []string,
	values map[string]any,
	inherited resolvedinput.Inputs,
) error {
	if a.options.CreateSession == nil {
		return fmt.Errorf("session create service is required")
	}
	globals, err := sessionGlobals(cmd, inherited)
	if err != nil {
		return err
	}
	dir, err := sessionValue[string](values, "you.session.create.flag.dir")
	if err != nil {
		return err
	}
	initNewFactory, err := sessionValue[bool](values, "you.session.create.flag.init-new-factory")
	if err != nil {
		return err
	}
	port, err := sessionValue[int](values, "you.session.create.flag.port")
	if err != nil {
		return err
	}
	targetKind, err := sessionValue[string](values, "you.session.create.flag.target-kind")
	if err != nil {
		return err
	}
	targetName, err := sessionValue[string](values, "you.session.create.flag.target-name")
	if err != nil {
		return err
	}
	validateOnly, err := sessionValue[bool](values, "you.session.create.flag.validate-only")
	if err != nil {
		return err
	}
	portExplicit, err := climanifestcobra.InputChanged(cmd, "you.session.create.flag.port")
	if err != nil {
		return err
	}
	return a.options.CreateSession(sessioncli.CreateConfig{
		Server: globals.server, Port: port, PortExplicit: portExplicit, Dir: dir,
		InitNewFactory: initNewFactory, ValidateOnly: validateOnly,
		TargetKind: targetKind, TargetName: targetName, JSON: globals.json,
		Verbose: globals.verbose, Debug: globals.debug, Output: cmd.OutOrStdout(),
		Diagnostics: globals.diagnostics,
	})
}

func (a sessionCommandAdapter) list(
	cmd *cobra.Command,
	_ []string,
	values map[string]any,
	inherited resolvedinput.Inputs,
) error {
	if a.options.ListSessions == nil {
		return fmt.Errorf("session list service is required")
	}
	globals, err := sessionGlobals(cmd, inherited)
	if err != nil {
		return err
	}
	port, err := sessionValue[int](values, "you.session.list.flag.port")
	if err != nil {
		return err
	}
	scope, err := sessionValue[string](values, "you.session.list.flag.scope")
	if err != nil {
		return err
	}
	cfg := sessioncli.ListConfig{
		Context: cmd.Context(), Port: port, Scope: scope, JSON: globals.json,
		Verbose: globals.verbose, Debug: globals.debug, Output: cmd.OutOrStdout(),
		Diagnostics: globals.diagnostics,
	}
	if state, ok := inherited.State("you.flag.server"); ok && state.Changed {
		cfg.Server = globals.server
	}
	if err := sessionListPrepare(a.options)(cmd.Context(), &cfg); err != nil {
		return err
	}
	return a.options.ListSessions(cfg)
}

func (a sessionCommandAdapter) show(
	cmd *cobra.Command,
	_ []string,
	values map[string]any,
	inherited resolvedinput.Inputs,
) error {
	if a.options.ShowSession == nil {
		return fmt.Errorf("session show service is required")
	}
	if err := rejectSessionDeprecatedPort(cmd, "you.session.show.flag.port"); err != nil {
		return err
	}
	globals, err := sessionGlobals(cmd, inherited)
	if err != nil {
		return err
	}
	sessionID, err := sessionValue[string](values, "you.session.show.arg.0")
	if err != nil {
		return err
	}
	return a.options.ShowSession(sessioncli.ShowConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		JSON: globals.json, Verbose: globals.verbose, Debug: globals.debug,
		Output: cmd.OutOrStdout(), Diagnostics: globals.diagnostics,
	})
}

func (a sessionCommandAdapter) delete(
	cmd *cobra.Command,
	_ []string,
	values map[string]any,
	inherited resolvedinput.Inputs,
) error {
	if a.options.DeleteSession == nil {
		return fmt.Errorf("session delete service is required")
	}
	globals, err := sessionGlobals(cmd, inherited)
	if err != nil {
		return err
	}
	sessionID, err := sessionValue[string](values, "you.session.delete.arg.0")
	if err != nil {
		return err
	}
	port, err := sessionValue[int](values, "you.session.delete.flag.port")
	if err != nil {
		return err
	}
	return a.options.DeleteSession(sessioncli.DeleteConfig{
		Port: port, SessionID: sessionID, JSON: globals.json,
		Verbose: globals.verbose, Debug: globals.debug,
		Output: cmd.OutOrStdout(), Diagnostics: globals.diagnostics,
	})
}

func (a sessionCommandAdapter) dispatches(
	cmd *cobra.Command,
	_ []string,
	values map[string]any,
	inherited resolvedinput.Inputs,
) error {
	if a.options.ListSessionDispatches == nil {
		return fmt.Errorf("session dispatches service is required")
	}
	if err := rejectSessionDeprecatedPort(cmd, "you.session.dispatches.flag.port"); err != nil {
		return err
	}
	globals, err := sessionGlobals(cmd, inherited)
	if err != nil {
		return err
	}
	sessionID, err := sessionValue[string](values, "you.session.dispatches.arg.0")
	if err != nil {
		return err
	}
	phase, err := sessionValue[string](values, "you.session.dispatches.flag.phase")
	if err != nil {
		return err
	}
	status, err := sessionValue[string](values, "you.session.dispatches.flag.status")
	if err != nil {
		return err
	}
	return a.options.ListSessionDispatches(sessioncli.DispatchesConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		Phase: phase, Status: status, JSON: globals.json,
		Verbose: globals.verbose, Debug: globals.debug,
		Output: cmd.OutOrStdout(), Diagnostics: globals.diagnostics,
	})
}

func (a sessionCommandAdapter) pause(
	cmd *cobra.Command,
	args []string,
	values map[string]any,
	inherited resolvedinput.Inputs,
) error {
	return a.lifecycle(cmd, args, values, inherited, "you.session.pause", a.options.PauseSession)
}

func (a sessionCommandAdapter) resume(
	cmd *cobra.Command,
	args []string,
	values map[string]any,
	inherited resolvedinput.Inputs,
) error {
	return a.lifecycle(cmd, args, values, inherited, "you.session.resume", a.options.ResumeSession)
}

func (a sessionCommandAdapter) lifecycle(
	cmd *cobra.Command,
	_ []string,
	values map[string]any,
	inherited resolvedinput.Inputs,
	commandID string,
	control func(sessioncli.LifecycleControlConfig) error,
) error {
	if control == nil {
		return fmt.Errorf("session lifecycle control handler is required")
	}
	if err := rejectSessionDeprecatedPort(cmd, commandID+".flag.port"); err != nil {
		return err
	}
	globals, err := sessionGlobals(cmd, inherited)
	if err != nil {
		return err
	}
	sessionID, err := sessionValue[string](values, commandID+".arg.0")
	if err != nil {
		return err
	}
	return control(sessioncli.LifecycleControlConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		JSON: globals.json, Verbose: globals.verbose, Debug: globals.debug,
		Output: cmd.OutOrStdout(), Diagnostics: globals.diagnostics,
	})
}

func rejectSessionDeprecatedPort(cmd *cobra.Command, inputID string) error {
	changed, err := climanifestcobra.InputChanged(cmd, inputID)
	if err != nil {
		return err
	}
	if changed {
		return fmt.Errorf("--port is no longer supported; use --server instead (for example, --server http://localhost:7437)")
	}
	return nil
}

func sessionListPrepare(options CommandFactory) func(context.Context, *sessioncli.ListConfig) error {
	return func(ctx context.Context, cfg *sessioncli.ListConfig) error {
		scope := fse.SessionListScope(strings.TrimSpace(cfg.Scope))
		if scope != fse.SessionListScopePersisted && scope != fse.SessionListScopeAll {
			return nil
		}
		service, err := buildWorkflowExecutionService(ctx, options, string(fse.ExecutionProviderFake), "", "", "")
		if err != nil {
			return err
		}
		cfg.DurableLister = service.ListSessions
		cfg.DurableCloser = service
		return nil
	}
}
