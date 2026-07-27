package commandregistry

import (
	"fmt"
	"io"
	"sort"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	"github.com/spf13/cobra"
)

const (
	workListStateNameInputID    = "you.work.list.flag.state-name"
	workListStateTypeInputID    = "you.work.list.flag.state-type"
	workListNameInputID         = "you.work.list.flag.name"
	workListWorkTypeNameInputID = "you.work.list.flag.work-type-name"
	workListTraceIDInputID      = "you.work.list.flag.trace-id"
	workListSortByInputID       = "you.work.list.flag.sort-by"
	workListMaxResultsInputID   = "you.work.list.flag.max-results"
	workListNextTokenInputID    = "you.work.list.flag.next-token"
	workListSessionInputID      = "you.work.list.flag.session"
	workShowWorkIDInputID       = "you.work.show.arg.0"
	workShowSessionInputID      = "you.work.show.flag.session"
	workMoveWorkIDInputID       = "you.work.move.arg.0"
	workMoveStateNameInputID    = "you.work.move.arg.1"
	workMoveSessionInputID      = "you.work.move.flag.session"
	workMoveRequestIDInputID    = "you.work.move.flag.request-id"
	workServerInputID           = "you.flag.server"
	workJSONInputID             = "you.flag.json"
	workVerboseInputID          = "you.flag.verbose"
	workDebugInputID            = "you.flag.debug"
)

// ResolvedWorkRunE executes one Work command from invocation-local resolved
// inputs. The second snapshot contains inherited root inputs.
type ResolvedWorkRunE func(
	*cobra.Command,
	resolvedinput.Inputs,
	resolvedinput.Inputs,
) error

// ResolvedWorkHandlers supplies typed handlers for the runnable Work commands.
// Construction maps these handlers through the stable IDs in the manifest.
type ResolvedWorkHandlers struct {
	List      ResolvedWorkRunE
	Show      ResolvedWorkRunE
	Move      ResolvedWorkRunE
	Visualize ResolvedWorkRunE
}

// ResolvedListBinding supplies the effects used by the Work list stable-input
// adapter. Each invocation maps resolved values into a fresh ListConfig.
type ResolvedListBinding struct {
	ListWork          func(workcli.ListConfig) error
	DiagnosticsWriter func(*cobra.Command) io.Writer
}

// ResolvedShowBinding supplies the effects used by the Work show stable-input
// adapter. Each invocation maps resolved values into a fresh ShowConfig.
type ResolvedShowBinding struct {
	ShowWork          func(workcli.ShowConfig) error
	DiagnosticsWriter func(*cobra.Command) io.Writer
}

// ResolvedMoveBinding supplies the effects used by the Work move stable-input
// adapter. Each invocation maps resolved values into a fresh MoveConfig.
type ResolvedMoveBinding struct {
	MoveWork          func(workcli.MoveConfig) error
	DiagnosticsWriter func(*cobra.Command) io.Writer
}

// ResolvedListRunE maps canonical Work list input IDs into one transport
// request without retaining Cobra-backed pointers between invocations.
func ResolvedListRunE(binding ResolvedListBinding) ResolvedWorkRunE {
	return func(
		cmd *cobra.Command,
		inputs resolvedinput.Inputs,
		inherited resolvedinput.Inputs,
	) error {
		if binding.ListWork == nil {
			return fmt.Errorf("work list service is required")
		}
		cfg, err := resolvedListConfig(cmd, inputs, inherited)
		if err != nil {
			return fmt.Errorf("resolve work list inputs: %w", err)
		}
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		return binding.ListWork(cfg)
	}
}

func resolvedListConfig(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) (workcli.ListConfig, error) {
	stateName, err := inputs.String(workListStateNameInputID)
	if err != nil {
		return workcli.ListConfig{}, err
	}
	stateType, err := inputs.String(workListStateTypeInputID)
	if err != nil {
		return workcli.ListConfig{}, err
	}
	name, err := inputs.String(workListNameInputID)
	if err != nil {
		return workcli.ListConfig{}, err
	}
	workTypeName, err := inputs.String(workListWorkTypeNameInputID)
	if err != nil {
		return workcli.ListConfig{}, err
	}
	traceID, err := inputs.String(workListTraceIDInputID)
	if err != nil {
		return workcli.ListConfig{}, err
	}
	sortBy, err := inputs.String(workListSortByInputID)
	if err != nil {
		return workcli.ListConfig{}, err
	}
	maxResults, err := inputs.Int(workListMaxResultsInputID)
	if err != nil {
		return workcli.ListConfig{}, err
	}
	nextToken, err := inputs.String(workListNextTokenInputID)
	if err != nil {
		return workcli.ListConfig{}, err
	}
	sessionID, err := inputs.String(workListSessionInputID)
	if err != nil {
		return workcli.ListConfig{}, err
	}
	globals, err := resolvedWorkGlobals(inherited)
	if err != nil {
		return workcli.ListConfig{}, err
	}
	return workcli.ListConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		StateName: stateName, StateType: stateType, Name: name,
		WorkTypeName: workTypeName, TraceID: traceID, SortBy: sortBy,
		MaxResults: maxResults, NextToken: nextToken, JSON: globals.json,
		Verbose: globals.verbose || globals.debug, Debug: globals.debug,
		Output: cmd.OutOrStdout(),
	}, nil
}

// ResolvedShowRunE maps canonical Work show input IDs into one transport
// request without retaining Cobra-backed pointers between invocations.
func ResolvedShowRunE(binding ResolvedShowBinding) ResolvedWorkRunE {
	return func(
		cmd *cobra.Command,
		inputs resolvedinput.Inputs,
		inherited resolvedinput.Inputs,
	) error {
		if binding.ShowWork == nil {
			return fmt.Errorf("work show service is required")
		}
		cfg, err := resolvedShowConfig(cmd, inputs, inherited)
		if err != nil {
			return fmt.Errorf("resolve work show inputs: %w", err)
		}
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		return binding.ShowWork(cfg)
	}
}

func resolvedShowConfig(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) (workcli.ShowConfig, error) {
	workID, err := inputs.String(workShowWorkIDInputID)
	if err != nil {
		return workcli.ShowConfig{}, err
	}
	sessionID, err := inputs.String(workShowSessionInputID)
	if err != nil {
		return workcli.ShowConfig{}, err
	}
	globals, err := resolvedWorkGlobals(inherited)
	if err != nil {
		return workcli.ShowConfig{}, err
	}
	return workcli.ShowConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		WorkID: workID, JSON: globals.json,
		Verbose: globals.verbose || globals.debug, Debug: globals.debug,
		Output: cmd.OutOrStdout(),
	}, nil
}

// ResolvedMoveRunE maps canonical Work move input IDs into one transport
// request without retaining Cobra-backed pointers between invocations.
func ResolvedMoveRunE(binding ResolvedMoveBinding) ResolvedWorkRunE {
	return func(
		cmd *cobra.Command,
		inputs resolvedinput.Inputs,
		inherited resolvedinput.Inputs,
	) error {
		if binding.MoveWork == nil {
			return fmt.Errorf("work move service is required")
		}
		cfg, err := resolvedMoveConfig(cmd, inputs, inherited)
		if err != nil {
			return fmt.Errorf("resolve work move inputs: %w", err)
		}
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		return binding.MoveWork(cfg)
	}
}

func resolvedMoveConfig(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) (workcli.MoveConfig, error) {
	workID, err := inputs.String(workMoveWorkIDInputID)
	if err != nil {
		return workcli.MoveConfig{}, err
	}
	stateName, err := inputs.String(workMoveStateNameInputID)
	if err != nil {
		return workcli.MoveConfig{}, err
	}
	sessionID, err := inputs.String(workMoveSessionInputID)
	if err != nil {
		return workcli.MoveConfig{}, err
	}
	requestID, err := inputs.String(workMoveRequestIDInputID)
	if err != nil {
		return workcli.MoveConfig{}, err
	}
	globals, err := resolvedWorkGlobals(inherited)
	if err != nil {
		return workcli.MoveConfig{}, err
	}
	return workcli.MoveConfig{
		Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
		WorkID: workID, StateName: stateName, RequestID: requestID,
		JSON: globals.json, Verbose: globals.verbose || globals.debug,
		Debug: globals.debug, Output: cmd.OutOrStdout(),
	}, nil
}

type resolvedWorkGlobalValues struct {
	server  string
	json    bool
	verbose bool
	debug   bool
}

func resolvedWorkGlobals(inputs resolvedinput.Inputs) (resolvedWorkGlobalValues, error) {
	server, err := inputs.String(workServerInputID)
	if err != nil {
		return resolvedWorkGlobalValues{}, err
	}
	jsonOutput, err := inputs.Bool(workJSONInputID)
	if err != nil {
		return resolvedWorkGlobalValues{}, err
	}
	verbose, err := inputs.Bool(workVerboseInputID)
	if err != nil {
		return resolvedWorkGlobalValues{}, err
	}
	debug, err := inputs.Bool(workDebugInputID)
	if err != nil {
		return resolvedWorkGlobalValues{}, err
	}
	return resolvedWorkGlobalValues{
		server: server, json: jsonOutput, verbose: verbose, debug: debug,
	}, nil
}

// RunnableWorkCommandIDs returns contracted runnable command IDs for the work
// family in stable sorted order.
func RunnableWorkCommandIDs(manifest climanifest.Manifest) ([]string, error) {
	ids := make([]string, 0, len(climanifestgen.WorkFamilyCommandIDs))
	for _, commandID := range climanifestgen.WorkFamilyCommandIDs {
		if err := climanifestgen.AssertWorkFamilyCommandID(commandID); err != nil {
			return nil, err
		}
		record, err := manifest.CommandByID(commandID)
		if err != nil {
			return nil, err
		}
		if record.Runnable {
			ids = append(ids, commandID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

// VerifyWorkRunnableCoverage fails when any contracted runnable work-family
// command ID lacks a registered handwritten handler.
func (r *Registry) VerifyWorkRunnableCoverage(manifest climanifest.Manifest) error {
	runnableIDs, err := RunnableWorkCommandIDs(manifest)
	if err != nil {
		return err
	}
	var missing []string
	for _, commandID := range runnableIDs {
		if _, lookupErr := r.Lookup(commandID); lookupErr != nil {
			missing = append(missing, commandID)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf(
			"work runnable command handlers missing for: %v",
			missing,
		)
	}
	return nil
}

// WorkHandlers carries handwritten RunE handlers for contracted runnable
// work-family command IDs.
type WorkHandlers struct {
	ListRunE      RunE
	ShowRunE      RunE
	MoveRunE      RunE
	VisualizeRunE RunE
}

// NewWorkRegistry registers handwritten handlers for the work family and
// verifies contracted runnable command coverage.
func NewWorkRegistry(handlers WorkHandlers) (*Registry, error) {
	if handlers.ListRunE == nil {
		return nil, fmt.Errorf("build work handler registry: list handler is required")
	}
	if handlers.ShowRunE == nil {
		return nil, fmt.Errorf("build work handler registry: show handler is required")
	}
	if handlers.MoveRunE == nil {
		return nil, fmt.Errorf("build work handler registry: move handler is required")
	}
	if handlers.VisualizeRunE == nil {
		return nil, fmt.Errorf("build work handler registry: visualize handler is required")
	}

	registry := NewRegistry()
	registrations := []struct {
		commandID string
		handler   RunE
	}{
		{commandID: "you.work.list", handler: handlers.ListRunE},
		{commandID: "you.work.show", handler: handlers.ShowRunE},
		{commandID: "you.work.move", handler: handlers.MoveRunE},
		{commandID: "you.work.visualize", handler: handlers.VisualizeRunE},
	}
	for _, registration := range registrations {
		if err := registry.Register(registration.commandID, registration.handler); err != nil {
			return nil, fmt.Errorf("build work handler registry: %w", err)
		}
	}

	manifest, err := generated.WorkFamilyManifest()
	if err != nil {
		return nil, fmt.Errorf("build work handler registry: %w", err)
	}
	if err := registry.VerifyWorkRunnableCoverage(manifest); err != nil {
		return nil, fmt.Errorf("build work handler registry: %w", err)
	}
	return registry, nil
}

// ListBinding supplies handwritten work list execution dependencies.
type ListBinding struct {
	Config            *workcli.ListConfig
	Server            *string
	JSON              *bool
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	ListWork          func(workcli.ListConfig) error
}

// ListRunE returns the handwritten work list RunE used by production wiring.
func ListRunE(binding ListBinding) RunE {
	return func(cmd *cobra.Command, args []string) error {
		if binding.ListWork == nil {
			return fmt.Errorf("work list service is required")
		}
		cfg := *binding.Config
		cfg.Context = cmd.Context()
		if binding.Server != nil {
			cfg.Server = *binding.Server
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		if binding.Verbose != nil {
			cfg.Verbose = binding.Verbose()
		}
		if binding.Debug != nil {
			cfg.Debug = *binding.Debug
		}
		return binding.ListWork(cfg)
	}
}

// ShowBinding supplies handwritten work show execution dependencies.
type ShowBinding struct {
	Config            *workcli.ShowConfig
	Server            *string
	JSON              *bool
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	ShowWork          func(workcli.ShowConfig) error
}

// ShowRunE returns the handwritten work show RunE used by production wiring.
func ShowRunE(binding ShowBinding) RunE {
	return func(cmd *cobra.Command, args []string) error {
		if binding.ShowWork == nil {
			return fmt.Errorf("work show service is required")
		}
		cfg := *binding.Config
		cfg.Context = cmd.Context()
		if binding.Server != nil {
			cfg.Server = *binding.Server
		}
		if len(args) == 1 {
			cfg.WorkID = args[0]
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		if binding.Verbose != nil {
			cfg.Verbose = binding.Verbose()
		}
		if binding.Debug != nil {
			cfg.Debug = *binding.Debug
		}
		return binding.ShowWork(cfg)
	}
}

// MoveBinding supplies handwritten work move execution dependencies.
type MoveBinding struct {
	Config            *workcli.MoveConfig
	Server            *string
	JSON              *bool
	Verbose           func() bool
	Debug             *bool
	DiagnosticsWriter func(cmd *cobra.Command) io.Writer
	MoveWork          func(workcli.MoveConfig) error
}

// MoveRunE returns the handwritten work move RunE used by production wiring.
func MoveRunE(binding MoveBinding) RunE {
	return func(cmd *cobra.Command, args []string) error {
		if binding.MoveWork == nil {
			return fmt.Errorf("work move service is required")
		}
		cfg := *binding.Config
		cfg.Context = cmd.Context()
		if binding.Server != nil {
			cfg.Server = *binding.Server
		}
		if len(args) >= 1 {
			cfg.WorkID = args[0]
		}
		if len(args) >= 2 {
			cfg.StateName = args[1]
		}
		if binding.JSON != nil {
			cfg.JSON = *binding.JSON
		}
		cfg.Output = cmd.OutOrStdout()
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		if binding.Verbose != nil {
			cfg.Verbose = binding.Verbose()
		}
		if binding.Debug != nil {
			cfg.Debug = *binding.Debug
		}
		return binding.MoveWork(cfg)
	}
}

// VisualizeBinding supplies handwritten work visualize execution dependencies.
type VisualizeBinding struct {
	Format    *string
	Visualize func(workcli.VisualizeConfig) error
}

// VisualizeRunE returns the handwritten work visualize RunE used by production wiring.
func VisualizeRunE(binding VisualizeBinding) RunE {
	return func(cmd *cobra.Command, args []string) error {
		if binding.Visualize == nil {
			return fmt.Errorf("work visualize service is required")
		}
		format := ""
		if binding.Format != nil {
			format = *binding.Format
		}
		return binding.Visualize(workcli.VisualizeConfig{
			BatchFile: args[0],
			Format:    format,
			Output:    cmd.OutOrStdout(),
		})
	}
}
