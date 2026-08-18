package commandregistry

import (
	"fmt"
	"io"

	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

const (
	workListStateNameInputID          = "you.work.list.flag.state-name"
	workListStateTypeInputID          = "you.work.list.flag.state-type"
	workListNameInputID               = "you.work.list.flag.name"
	workListWorkTypeNameInputID       = "you.work.list.flag.work-type-name"
	workListTraceIDInputID            = "you.work.list.flag.trace-id"
	workListTerminalInputID           = "you.work.list.flag.terminal"
	workListNonTerminalInputID        = "you.work.list.flag.non-terminal"
	workListSortByInputID             = "you.work.list.flag.sort-by"
	workListMaxResultsInputID         = "you.work.list.flag.max-results"
	workListNextTokenInputID          = "you.work.list.flag.next-token"
	workListCountsInputID             = "you.work.list.flag.counts"
	workListSessionInputID            = "you.work.list.flag.session"
	workWatchSessionInputID           = "you.work.watch.flag.session"
	workWatchFollowInputID            = "you.work.watch.flag.follow"
	workShowWorkIDInputID             = "you.work.show.arg.0"
	workShowSessionInputID            = "you.work.show.flag.session"
	workMoveWorkIDInputID             = "you.work.move.arg.0"
	workMoveStateNameInputID          = "you.work.move.arg.1"
	workMoveSessionInputID            = "you.work.move.flag.session"
	workMoveRequestIDInputID          = "you.work.move.flag.request-id"
	workVisualizeBatchInputID         = "you.work.visualize.arg.0"
	workVisualizeFormatInputID        = "you.work.visualize.flag.format"
	workApprovalListSessionInputID    = "you.work.approval.list.flag.session"
	workApprovalShowApprovalIDInputID = "you.work.approval.show.arg.0"
	workApprovalShowSessionInputID    = "you.work.approval.show.flag.session"
	workServerInputID                 = "you.flag.server"
	workJSONInputID                   = "you.flag.json"
	workVerboseInputID                = "you.flag.verbose"
	workDebugInputID                  = "you.flag.debug"
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
	ApprovalList ResolvedWorkRunE
	ApprovalShow ResolvedWorkRunE
	List         ResolvedWorkRunE
	Watch        ResolvedWorkRunE
	Show         ResolvedWorkRunE
	Move         ResolvedWorkRunE
	Visualize    ResolvedWorkRunE
}

// ResolvedApprovalListBinding supplies the effects used by the pending
// approval list stable-input adapter.
type ResolvedApprovalListBinding struct {
	ListHumanApprovals func(workcli.ListHumanApprovalsConfig) error
	DiagnosticsWriter  func(*cobra.Command) io.Writer
}

// ResolvedApprovalShowBinding supplies the effects used by the pending
// approval show stable-input adapter.
type ResolvedApprovalShowBinding struct {
	ShowHumanApproval func(workcli.ShowHumanApprovalConfig) error
	DiagnosticsWriter func(*cobra.Command) io.Writer
}

// ResolvedListBinding supplies the effects used by the Work list stable-input
// adapter. Each invocation maps resolved values into a fresh ListConfig.
type ResolvedListBinding struct {
	ListWork          func(workcli.ListConfig) error
	DiagnosticsWriter func(*cobra.Command) io.Writer
}

// ResolvedWatchBinding supplies the effects used by the Work watch stable-input
// adapter. Each invocation maps resolved values into a fresh WatchConfig.
type ResolvedWatchBinding struct {
	WatchWork         func(workcli.WatchConfig) error
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

// ResolvedVisualizeBinding supplies the local operation used by the Work
// visualize stable-input adapter.
type ResolvedVisualizeBinding struct {
	VisualizeWork func(workcli.VisualizeConfig) error
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

// ResolvedApprovalListRunE maps canonical approval-list input IDs into one
// transport request without retaining Cobra-backed pointers between calls.
func ResolvedApprovalListRunE(binding ResolvedApprovalListBinding) ResolvedWorkRunE {
	return func(cmd *cobra.Command, inputs resolvedinput.Inputs, inherited resolvedinput.Inputs) error {
		if binding.ListHumanApprovals == nil {
			return fmt.Errorf("human approval list service is required")
		}
		sessionID, err := inputs.String(workApprovalListSessionInputID)
		if err != nil {
			return fmt.Errorf("resolve human approval list inputs: %w", err)
		}
		globals, err := resolvedWorkGlobals(inherited)
		if err != nil {
			return fmt.Errorf("resolve human approval list inputs: %w", err)
		}
		cfg := workcli.ListHumanApprovalsConfig{
			Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
			JSON: globals.json, Verbose: globals.verbose || globals.debug,
			Debug: globals.debug, Output: cmd.OutOrStdout(),
		}
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		return binding.ListHumanApprovals(cfg)
	}
}

// ResolvedApprovalShowRunE maps canonical approval-show input IDs into one
// transport request without retaining Cobra-backed pointers between calls.
func ResolvedApprovalShowRunE(binding ResolvedApprovalShowBinding) ResolvedWorkRunE {
	return func(cmd *cobra.Command, inputs resolvedinput.Inputs, inherited resolvedinput.Inputs) error {
		if binding.ShowHumanApproval == nil {
			return fmt.Errorf("human approval show service is required")
		}
		approvalID, err := inputs.String(workApprovalShowApprovalIDInputID)
		if err != nil {
			return fmt.Errorf("resolve human approval show inputs: %w", err)
		}
		sessionID, err := inputs.String(workApprovalShowSessionInputID)
		if err != nil {
			return fmt.Errorf("resolve human approval show inputs: %w", err)
		}
		globals, err := resolvedWorkGlobals(inherited)
		if err != nil {
			return fmt.Errorf("resolve human approval show inputs: %w", err)
		}
		cfg := workcli.ShowHumanApprovalConfig{
			Context: cmd.Context(), Server: globals.server, SessionID: sessionID,
			ApprovalID: approvalID, JSON: globals.json,
			Verbose: globals.verbose || globals.debug, Debug: globals.debug,
			Output: cmd.OutOrStdout(),
		}
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		return binding.ShowHumanApproval(cfg)
	}
}

// ResolvedWatchRunE maps canonical Work watch input IDs into one typed watch
// request without retaining Cobra-backed pointers between invocations.
func ResolvedWatchRunE(binding ResolvedWatchBinding) ResolvedWorkRunE {
	return func(
		cmd *cobra.Command,
		inputs resolvedinput.Inputs,
		inherited resolvedinput.Inputs,
	) error {
		if binding.WatchWork == nil {
			return fmt.Errorf("work watch service is required")
		}
		cfg, err := resolvedWatchConfig(cmd, inputs, inherited)
		if err != nil {
			return fmt.Errorf("resolve work watch inputs: %w", err)
		}
		if binding.DiagnosticsWriter != nil {
			cfg.Diagnostics = binding.DiagnosticsWriter(cmd)
		}
		if err := workcli.ValidateWatchConfig(cfg); err != nil {
			return err
		}
		return binding.WatchWork(cfg)
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
	terminal, err := inputs.Bool(workListTerminalInputID)
	if err != nil {
		return workcli.ListConfig{}, err
	}
	nonTerminal, err := inputs.Bool(workListNonTerminalInputID)
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
	counts, err := inputs.Bool(workListCountsInputID)
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
		WorkTypeName: workTypeName, TraceID: traceID, Terminal: terminal,
		NonTerminal: nonTerminal, SortBy: sortBy, MaxResults: maxResults,
		NextToken: nextToken, Counts: counts, JSON: globals.json,
		Verbose: globals.verbose || globals.debug, Debug: globals.debug,
		Output: cmd.OutOrStdout(),
	}, nil
}

func resolvedWatchConfig(
	cmd *cobra.Command,
	inputs resolvedinput.Inputs,
	inherited resolvedinput.Inputs,
) (workcli.WatchConfig, error) {
	sessionID, err := inputs.String(workWatchSessionInputID)
	if err != nil {
		return workcli.WatchConfig{}, err
	}
	follow, err := inputs.Bool(workWatchFollowInputID)
	if err != nil {
		return workcli.WatchConfig{}, err
	}
	globals, err := resolvedWorkGlobals(inherited)
	if err != nil {
		return workcli.WatchConfig{}, err
	}
	return workcli.WatchConfig{
		Context:           cmd.Context(),
		Server:            globals.server,
		SessionID:         sessionID,
		SessionIDExplicit: workWatchSessionFlagChanged(cmd),
		Follow:            follow,
		Verbose:           globals.verbose || globals.debug,
		Debug:             globals.debug,
		Output:            cmd.OutOrStdout(),
	}, nil
}

func workWatchSessionFlagChanged(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flag("session")
	return flag != nil && flag.Changed
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

// ResolvedVisualizeRunE maps canonical Work visualize input IDs into one local
// request without retaining Cobra-backed pointers between invocations.
func ResolvedVisualizeRunE(binding ResolvedVisualizeBinding) ResolvedWorkRunE {
	return func(
		cmd *cobra.Command,
		inputs resolvedinput.Inputs,
		_ resolvedinput.Inputs,
	) error {
		if binding.VisualizeWork == nil {
			return fmt.Errorf("work visualize service is required")
		}
		batchFile, err := inputs.String(workVisualizeBatchInputID)
		if err != nil {
			return fmt.Errorf("resolve work visualize inputs: %w", err)
		}
		format, err := inputs.String(workVisualizeFormatInputID)
		if err != nil {
			return fmt.Errorf("resolve work visualize inputs: %w", err)
		}
		return binding.VisualizeWork(workcli.VisualizeConfig{
			Context: cmd.Context(), BatchFile: batchFile,
			Format: format, Output: cmd.OutOrStdout(),
		})
	}
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
