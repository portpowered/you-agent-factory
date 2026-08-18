package commandregistry_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	workcli "github.com/portpowered/infinite-you/pkg/services/work/transports/cli/work"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	"github.com/spf13/cobra"
)

func TestResolvedWorkListRejectsMutuallyExclusiveTerminalityBeforeHandler(t *testing.T) {
	called := false
	list := commandregistry.ResolvedListRunE(commandregistry.ResolvedListBinding{
		ListWork: func(workcli.ListConfig) error {
			called = true
			return nil
		},
	})
	err := executeResolvedListError(t, list, []string{
		"work", "list", "--terminal", "--non-terminal",
	}, io.Discard, io.Discard, context.Background())
	if err == nil || !strings.Contains(err.Error(), "terminal") || !strings.Contains(err.Error(), "non-terminal") {
		t.Fatalf("mutually-exclusive flags error = %v, want relationship validation error", err)
	}
	if called {
		t.Fatal("list handler was called after mutually-exclusive flag validation")
	}
}

func TestResolvedWorkAdaptersReportEveryMissingStableInput(t *testing.T) {
	stringInput := func(id string) resolvedTestValue {
		return resolvedTestValue{id: id, source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("value")}
	}
	boolInput := func(id string) resolvedTestValue {
		return resolvedTestValue{id: id, source: resolvedinput.SourceCLIFlag, value: resolvedinput.BoolValue(false)}
	}
	local := []resolvedTestValue{
		stringInput("you.work.list.flag.state-name"), stringInput("you.work.list.flag.state-type"), stringInput("you.work.list.flag.name"), stringInput("you.work.list.flag.work-type-name"), stringInput("you.work.list.flag.trace-id"), stringInput("you.work.list.flag.sort-by"), boolInput("you.work.list.flag.terminal"), boolInput("you.work.list.flag.non-terminal"), {id: "you.work.list.flag.max-results", source: resolvedinput.SourceCLIFlag, value: resolvedinput.IntValue(1)}, stringInput("you.work.list.flag.next-token"), boolInput("you.work.list.flag.counts"), stringInput("you.work.list.flag.session"), stringInput("you.work.show.arg.0"), stringInput("you.work.show.flag.session"), stringInput("you.work.move.arg.0"), stringInput("you.work.move.arg.1"), stringInput("you.work.move.flag.session"), stringInput("you.work.move.flag.request-id"), stringInput("you.work.visualize.arg.0"), stringInput("you.work.visualize.flag.format"),
	}
	globals := []resolvedTestValue{stringInput("you.flag.server"), {id: "you.flag.json", source: resolvedinput.SourceCLIFlag, value: resolvedinput.BoolValue(false)}, {id: "you.flag.verbose", source: resolvedinput.SourceCLIFlag, value: resolvedinput.BoolValue(false)}, {id: "you.flag.debug", source: resolvedinput.SourceCLIFlag, value: resolvedinput.BoolValue(false)}}
	noList := func(workcli.ListConfig) error { return nil }
	noShow := func(workcli.ShowConfig) error { return nil }
	noMove := func(workcli.MoveConfig) error { return nil }
	noVisualize := func(workcli.VisualizeConfig) error { return nil }
	tests := []struct {
		name    string
		handler commandregistry.ResolvedWorkRunE
		missing []string
	}{
		{"list", commandregistry.ResolvedListRunE(commandregistry.ResolvedListBinding{ListWork: noList}), []string{"you.work.list.flag.state-name", "you.work.list.flag.state-type", "you.work.list.flag.name", "you.work.list.flag.work-type-name", "you.work.list.flag.trace-id", "you.work.list.flag.sort-by", "you.work.list.flag.terminal", "you.work.list.flag.non-terminal", "you.work.list.flag.max-results", "you.work.list.flag.next-token", "you.work.list.flag.counts", "you.work.list.flag.session"}},
		{"show", commandregistry.ResolvedShowRunE(commandregistry.ResolvedShowBinding{ShowWork: noShow}), []string{"you.work.show.arg.0", "you.work.show.flag.session"}},
		{"move", commandregistry.ResolvedMoveRunE(commandregistry.ResolvedMoveBinding{MoveWork: noMove}), []string{"you.work.move.arg.0", "you.work.move.arg.1", "you.work.move.flag.session", "you.work.move.flag.request-id"}},
		{"visualize", commandregistry.ResolvedVisualizeRunE(commandregistry.ResolvedVisualizeBinding{VisualizeWork: noVisualize}), []string{"you.work.visualize.arg.0", "you.work.visualize.flag.format"}},
	}
	for _, test := range tests {
		missingInputs := test.missing
		if test.name != "visualize" {
			missingInputs = append(missingInputs, "you.flag.server", "you.flag.json", "you.flag.verbose", "you.flag.debug")
		}
		for _, missing := range missingInputs {
			t.Run(test.name+"/"+missing, func(t *testing.T) {
				inputs := resolvedTestInputsWithout(t, local, missing)
				inherited := resolvedTestInputsWithout(t, globals, missing)
				err := test.handler(&cobra.Command{}, inputs, inherited)
				var accessErr *resolvedinput.AccessError
				if !errors.As(err, &accessErr) || accessErr.InputID != missing {
					t.Fatalf("handler error = %v, want missing stable input %q", err, missing)
				}
			})
		}
	}
}

func TestResolvedWorkAdaptersRequireServices(t *testing.T) {
	tests := []struct {
		name    string
		handler commandregistry.ResolvedWorkRunE
	}{
		{"approval list", commandregistry.ResolvedApprovalListRunE(commandregistry.ResolvedApprovalListBinding{})},
		{"approval show", commandregistry.ResolvedApprovalShowRunE(commandregistry.ResolvedApprovalShowBinding{})},
		{"list", commandregistry.ResolvedListRunE(commandregistry.ResolvedListBinding{})},
		{"show", commandregistry.ResolvedShowRunE(commandregistry.ResolvedShowBinding{})},
		{"move", commandregistry.ResolvedMoveRunE(commandregistry.ResolvedMoveBinding{})},
		{"visualize", commandregistry.ResolvedVisualizeRunE(commandregistry.ResolvedVisualizeBinding{})},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.handler(&cobra.Command{}, resolvedinput.Inputs{}, resolvedinput.Inputs{})
			if err == nil || !strings.Contains(err.Error(), "service is required") {
				t.Fatalf("handler error = %v, want required service error", err)
			}
		})
	}
}

func TestResolvedApprovalAdaptersMapStableInputsIntoFreshRequests(t *testing.T) {
	var listRequests []workcli.ListHumanApprovalsConfig
	list := commandregistry.ResolvedApprovalListRunE(commandregistry.ResolvedApprovalListBinding{ListHumanApprovals: func(cfg workcli.ListHumanApprovalsConfig) error { listRequests = append(listRequests, cfg); return nil }, DiagnosticsWriter: func(cmd *cobra.Command) io.Writer { return cmd.ErrOrStderr() }})
	var showRequests []workcli.ShowHumanApprovalConfig
	show := commandregistry.ResolvedApprovalShowRunE(commandregistry.ResolvedApprovalShowBinding{ShowHumanApproval: func(cfg workcli.ShowHumanApprovalConfig) error { showRequests = append(showRequests, cfg); return nil }, DiagnosticsWriter: func(cmd *cobra.Command) io.Writer { return cmd.ErrOrStderr() }})
	executeResolvedApprovalList(t, list, []string{"--server", "https://factory.example", "--json", "--debug", "work", "approval", "list", "--session", "session-alpha"}, io.Discard, io.Discard, context.Background())
	executeResolvedApprovalShow(t, show, []string{"--server", "https://factory.example", "--json", "--debug", "work", "approval", "show", "approval-alpha", "--session", "session-alpha"}, io.Discard, io.Discard, context.Background())
	assertResolvedApprovalListRequest(t, listRequests)
	assertResolvedApprovalShowRequest(t, showRequests)
}

func assertResolvedApprovalListRequest(t *testing.T, requests []workcli.ListHumanApprovalsConfig) {
	t.Helper()
	if len(requests) != 1 {
		t.Fatalf("approval list request count = %d, want 1", len(requests))
	}
	request := requests[0]
	if request.Server != "https://factory.example" || request.SessionID != "session-alpha" || !request.JSON || !request.Verbose || !request.Debug {
		t.Fatalf("approval list config = %#v, want resolved request values", request)
	}
	if request.Output == nil || request.Diagnostics == nil || request.Context == nil {
		t.Fatalf("approval list config = %#v, want runtime output, diagnostics, and context", request)
	}
}

func assertResolvedApprovalShowRequest(t *testing.T, requests []workcli.ShowHumanApprovalConfig) {
	t.Helper()
	if len(requests) != 1 {
		t.Fatalf("approval show request count = %d, want 1", len(requests))
	}
	request := requests[0]
	if request.Server != "https://factory.example" || request.SessionID != "session-alpha" || request.ApprovalID != "approval-alpha" || !request.JSON || !request.Verbose || !request.Debug {
		t.Fatalf("approval show config = %#v, want resolved request values", request)
	}
	if request.Output == nil || request.Diagnostics == nil || request.Context == nil {
		t.Fatalf("approval show config = %#v, want runtime output, diagnostics, and context", request)
	}
}

func TestResolvedApprovalAdaptersRejectMissingResolvedInputs(t *testing.T) {
	list := commandregistry.ResolvedApprovalListRunE(commandregistry.ResolvedApprovalListBinding{ListHumanApprovals: func(workcli.ListHumanApprovalsConfig) error { return nil }})
	if err := list(&cobra.Command{}, resolvedinput.Inputs{}, resolvedinput.Inputs{}); err == nil || !strings.Contains(err.Error(), "resolve human approval list inputs") {
		t.Fatalf("approval list missing local inputs error = %v", err)
	}
	if err := list(&cobra.Command{}, resolvedTestInputs(t, resolvedTestValue{id: "you.work.approval.list.flag.session", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("session-alpha")}), resolvedinput.Inputs{}); err == nil || !strings.Contains(err.Error(), "resolve human approval list inputs") {
		t.Fatalf("approval list missing inherited inputs error = %v", err)
	}
	show := commandregistry.ResolvedApprovalShowRunE(commandregistry.ResolvedApprovalShowBinding{ShowHumanApproval: func(workcli.ShowHumanApprovalConfig) error { return nil }})
	if err := show(&cobra.Command{}, resolvedinput.Inputs{}, resolvedinput.Inputs{}); err == nil || !strings.Contains(err.Error(), "resolve human approval show inputs") {
		t.Fatalf("approval show missing local inputs error = %v", err)
	}
	if err := show(&cobra.Command{}, resolvedTestInputs(t, resolvedTestValue{id: "you.work.approval.show.arg.0", source: resolvedinput.SourcePositionalArgument, value: resolvedinput.StringValue("approval-alpha")}, resolvedTestValue{id: "you.work.approval.show.flag.session", source: resolvedinput.SourceCLIFlag, value: resolvedinput.StringValue("session-alpha")}), resolvedinput.Inputs{}); err == nil || !strings.Contains(err.Error(), "resolve human approval show inputs") {
		t.Fatalf("approval show missing inherited inputs error = %v", err)
	}
}

func executeResolvedApprovalList(t *testing.T, list commandregistry.ResolvedWorkRunE, args []string, output, diagnostics io.Writer, ctx context.Context) {
	t.Helper()
	if err := executeResolvedApprovalListError(t, list, args, output, diagnostics, ctx); err != nil {
		t.Fatalf("Execute(%v) error = %v", args, err)
	}
}

func executeResolvedApprovalListError(t *testing.T, list commandregistry.ResolvedWorkRunE, args []string, output, diagnostics io.Writer, ctx context.Context) error {
	t.Helper()
	noop := func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error { return nil }
	root, err := climanifestcobra.NewResolvedWorkCommandTree(commandregistry.ResolvedWorkHandlers{ApprovalList: list, ApprovalShow: noop, List: noop, Show: noop, Move: noop, Visualize: noop})
	if err != nil {
		t.Fatalf("NewResolvedWorkCommandTree() error = %v", err)
	}
	root.SetOut(output)
	root.SetErr(diagnostics)
	root.SetContext(ctx)
	root.SetArgs(args)
	return root.Execute()
}

func executeResolvedApprovalShow(t *testing.T, show commandregistry.ResolvedWorkRunE, args []string, output, diagnostics io.Writer, ctx context.Context) {
	t.Helper()
	if err := executeResolvedApprovalShowError(t, show, args, output, diagnostics, ctx); err != nil {
		t.Fatalf("Execute(%v) error = %v", args, err)
	}
}

func executeResolvedApprovalShowError(t *testing.T, show commandregistry.ResolvedWorkRunE, args []string, output, diagnostics io.Writer, ctx context.Context) error {
	t.Helper()
	noop := func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error { return nil }
	root, err := climanifestcobra.NewResolvedWorkCommandTree(commandregistry.ResolvedWorkHandlers{ApprovalList: noop, ApprovalShow: show, List: noop, Show: noop, Move: noop, Visualize: noop})
	if err != nil {
		t.Fatalf("NewResolvedWorkCommandTree() error = %v", err)
	}
	root.SetOut(output)
	root.SetErr(diagnostics)
	root.SetContext(ctx)
	root.SetArgs(args)
	return root.Execute()
}

func TestWorkWatchCommandMapsDefaultAndExplicitSessionModes(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantSessionID string
		wantExplicit  bool
		wantFollow    bool
	}{
		{name: "default session", args: []string{"watch"}},
		{
			name:          "explicit follow session",
			args:          []string{"watch", "--session", "session-beta", "--follow"},
			wantSessionID: "session-beta",
			wantExplicit:  true,
			wantFollow:    true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got workcli.WatchConfig
			work := newWatchCommandTree(t, func(cfg workcli.WatchConfig) error {
				got = cfg
				return nil
			})
			work.SetArgs(append([]string{"work"}, tc.args...))
			if err := work.Execute(); err != nil {
				t.Fatalf("Execute(%v) error = %v", tc.args, err)
			}
			if got.SessionID != tc.wantSessionID || got.SessionIDExplicit != tc.wantExplicit || got.Follow != tc.wantFollow {
				t.Fatalf("watch config = %#v, want session=%q explicit=%t follow=%t", got, tc.wantSessionID, tc.wantExplicit, tc.wantFollow)
			}
		})
	}
}

func TestWorkWatchCommandRejectsEmptyExplicitSessionAndUnknownFlags(t *testing.T) {
	called := false
	work := newWatchCommandTree(t, func(workcli.WatchConfig) error {
		called = true
		return nil
	})
	work.SetArgs([]string{"work", "watch", "--session", ""})
	err := work.Execute()
	if err == nil || called {
		t.Fatalf("empty explicit session error = %v, operation called = %t; want validation failure before operation", err, called)
	}

	work = newWatchCommandTree(t, func(workcli.WatchConfig) error { return nil })
	work.SetArgs([]string{"work", "watch", "--poll"})
	err = work.Execute()
	if err == nil {
		t.Fatal("unknown --poll flag error = nil")
	}
}

func TestWorkWatchCommandSeparatesTransitionOutputFromDiagnostics(t *testing.T) {
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	var got workcli.WatchConfig
	work := newWatchCommandTreeWithStreams(t, func(cfg workcli.WatchConfig) error {
		got = cfg
		return nil
	}, stdout, stderr)
	work.SetOut(stdout)
	work.SetErr(stderr)
	work.SetArgs([]string{"work", "watch", "--follow"})
	if err := work.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got.Output != stdout || got.Diagnostics != stderr {
		t.Fatalf("watch streams = output %T/%p diagnostics %T/%p, want stdout/stderr", got.Output, got.Output, got.Diagnostics, got.Diagnostics)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("command emitted output before a transition: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestResolvedWorkWatchRunEMapsStableInputs(t *testing.T) {
	output := &bytes.Buffer{}
	diagnostics := &bytes.Buffer{}
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(output)
	cmd.SetErr(diagnostics)
	cmd.Flags().String("session", "", "session")
	if err := cmd.Flags().Set("session", "session-alpha"); err != nil {
		t.Fatalf("set session flag: %v", err)
	}

	local, inherited := resolvedWatchInputs(t, "", "session-alpha", true)
	var got workcli.WatchConfig
	run := commandregistry.ResolvedWatchRunE(commandregistry.ResolvedWatchBinding{
		DiagnosticsWriter: func(cmd *cobra.Command) io.Writer { return cmd.ErrOrStderr() },
		WatchWork: func(cfg workcli.WatchConfig) error {
			got = cfg
			_, err := io.WriteString(cfg.Output, "watched\n")
			return err
		},
	})
	if err := run(cmd, local, inherited); err != nil {
		t.Fatalf("ResolvedWatchRunE() error = %v", err)
	}
	if got.Context == nil || got.Server != "https://factory.example" || got.SessionID != "session-alpha" ||
		!got.SessionIDExplicit || !got.Follow || !got.Verbose || !got.Debug || got.Output != output || got.Diagnostics != diagnostics {
		t.Fatalf("watch config = %#v, want resolved stable inputs and streams", got)
	}
	if output.String() != "watched\n" || diagnostics.Len() != 0 {
		t.Fatalf("watch output = %q diagnostics = %q", output.String(), diagnostics.String())
	}
}

func TestResolvedWorkWatchRunERejectsMissingInputsAndInvalidConfig(t *testing.T) {
	run := commandregistry.ResolvedWatchRunE(commandregistry.ResolvedWatchBinding{
		WatchWork: func(workcli.WatchConfig) error {
			t.Fatal("watch service called for invalid inputs")
			return nil
		},
	})
	missingService := commandregistry.ResolvedWatchRunE(commandregistry.ResolvedWatchBinding{})
	if err := missingService(&cobra.Command{}, resolvedinput.Inputs{}, resolvedinput.Inputs{}); err == nil || !strings.Contains(err.Error(), "work watch service is required") {
		t.Fatalf("missing watch service error = %v, want required error", err)
	}

	for _, missing := range []string{
		"you.work.watch.flag.session",
		"you.work.watch.flag.follow",
		"you.flag.server",
		"you.flag.json",
		"you.flag.verbose",
		"you.flag.debug",
	} {
		t.Run("missing "+missing, func(t *testing.T) {
			local, inherited := resolvedWatchInputs(t, missing, "session-alpha", true)
			err := run(&cobra.Command{}, local, inherited)
			if err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("error = %v, want missing input %q", err, missing)
			}
		})
	}

	local, inherited := resolvedWatchInputs(t, "", "", false)
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	cmd.Flags().String("session", "", "session")
	if err := cmd.Flags().Set("session", ""); err != nil {
		t.Fatalf("set empty session flag: %v", err)
	}
	if err := run(cmd, local, inherited); err == nil || !strings.Contains(err.Error(), "must not be empty") {
		t.Fatalf("explicit empty session error = %v, want validation error", err)
	}
}

func resolvedWatchInputs(
	t *testing.T,
	missing string,
	sessionID string,
	follow bool,
) (resolvedinput.Inputs, resolvedinput.Inputs) {
	t.Helper()
	localDefinitions := []resolvedinput.Definition{
		{ID: "you.work.watch.flag.session", Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
		{ID: "you.work.watch.flag.follow", Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceCLIFlag}},
	}
	localCandidates := []resolvedinput.Candidate{
		{InputID: "you.work.watch.flag.session", Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.StringValue(sessionID)},
		{InputID: "you.work.watch.flag.follow", Source: resolvedinput.SourceCLIFlag, Value: resolvedinput.BoolValue(follow)},
	}
	globalDefinitions := []resolvedinput.Definition{
		{ID: "you.flag.server", Kind: resolvedinput.ValueKindString, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
		{ID: "you.flag.json", Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
		{ID: "you.flag.verbose", Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
		{ID: "you.flag.debug", Kind: resolvedinput.ValueKindBool, Precedence: []resolvedinput.Source{resolvedinput.SourceManifestDefault}},
	}
	globalCandidates := []resolvedinput.Candidate{
		{InputID: "you.flag.server", Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.StringValue("https://factory.example")},
		{InputID: "you.flag.json", Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.BoolValue(false)},
		{InputID: "you.flag.verbose", Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.BoolValue(true)},
		{InputID: "you.flag.debug", Source: resolvedinput.SourceManifestDefault, Value: resolvedinput.BoolValue(true)},
	}
	localDefinitions, localCandidates = omitResolvedWatchInput(localDefinitions, localCandidates, missing)
	globalDefinitions, globalCandidates = omitResolvedWatchInput(globalDefinitions, globalCandidates, missing)
	local, err := resolvedinput.Resolve(localDefinitions, localCandidates)
	if err != nil {
		t.Fatalf("resolve local watch inputs: %v", err)
	}
	inherited, err := resolvedinput.Resolve(globalDefinitions, globalCandidates)
	if err != nil {
		t.Fatalf("resolve inherited watch inputs: %v", err)
	}
	return local, inherited
}

func omitResolvedWatchInput(
	definitions []resolvedinput.Definition,
	candidates []resolvedinput.Candidate,
	missing string,
) ([]resolvedinput.Definition, []resolvedinput.Candidate) {
	if missing == "" {
		return definitions, candidates
	}
	filteredDefinitions := definitions[:0]
	for _, definition := range definitions {
		if definition.ID != missing {
			filteredDefinitions = append(filteredDefinitions, definition)
		}
	}
	filteredCandidates := candidates[:0]
	for _, candidate := range candidates {
		if candidate.InputID != missing {
			filteredCandidates = append(filteredCandidates, candidate)
		}
	}
	return filteredDefinitions, filteredCandidates
}

func newWatchCommandTree(t *testing.T, watch func(workcli.WatchConfig) error) *cobra.Command {
	var output bytes.Buffer
	return newWatchCommandTreeWithStreams(t, watch, &output, io.Discard)
}

func newWatchCommandTreeWithStreams(
	t *testing.T,
	watch func(workcli.WatchConfig) error,
	output io.Writer,
	diagnostics io.Writer,
) *cobra.Command {
	t.Helper()
	noop := func(*cobra.Command, resolvedinput.Inputs, resolvedinput.Inputs) error { return nil }
	root, err := climanifestcobra.NewResolvedWorkCommandTree(commandregistry.ResolvedWorkHandlers{
		List: noop,
		Watch: commandregistry.ResolvedWatchRunE(commandregistry.ResolvedWatchBinding{
			DiagnosticsWriter: func(cmd *cobra.Command) io.Writer { return cmd.ErrOrStderr() },
			WatchWork:         watch,
		}),
		Show:      noop,
		Move:      noop,
		Visualize: noop,
	})
	if err != nil {
		t.Fatalf("NewResolvedWorkCommandTree() error = %v", err)
	}
	root.SetOut(output)
	root.SetErr(diagnostics)
	return root
}
