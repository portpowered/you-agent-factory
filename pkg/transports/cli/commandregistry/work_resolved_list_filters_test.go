package commandregistry_test

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/resolvedinput"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
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
			work.SetArgs(tc.args)
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
	work.SetArgs([]string{"watch", "--session", ""})
	err := work.Execute()
	if err == nil || called {
		t.Fatalf("empty explicit session error = %v, operation called = %t; want validation failure before operation", err, called)
	}

	work = newWatchCommandTree(t, func(workcli.WatchConfig) error { return nil })
	work.SetArgs([]string{"watch", "--poll"})
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
	work.SetArgs([]string{"watch", "--follow"})
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

func TestNewWorkRegistryWatchFallbackHandlerRequiresService(t *testing.T) {
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE:      noopRunE,
		ShowRunE:      noopRunE,
		MoveRunE:      noopRunE,
		VisualizeRunE: noopRunE,
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}
	watch, err := registry.Lookup("you.work.watch")
	if err != nil {
		t.Fatalf("Lookup(you.work.watch) error = %v", err)
	}
	if err := watch(&cobra.Command{}, nil); err == nil || !strings.Contains(err.Error(), "work watch service is required") {
		t.Fatalf("fallback watch handler error = %v, want required service error", err)
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

func TestWatchRunERejectsMissingDependenciesAndUsesOptionalBindings(t *testing.T) {
	if err := commandregistry.WatchRunE(commandregistry.WatchBinding{})(nil, nil); err == nil || !strings.Contains(err.Error(), "work watch service is required") {
		t.Fatalf("missing service error = %v, want required error", err)
	}
	if err := commandregistry.WatchRunE(commandregistry.WatchBinding{
		WatchWork: func(workcli.WatchConfig) error { return nil },
	})(nil, nil); err == nil || !strings.Contains(err.Error(), "work watch config is required") {
		t.Fatalf("missing config error = %v, want required error", err)
	}

	var got workcli.WatchConfig
	run := commandregistry.WatchRunE(commandregistry.WatchBinding{
		Config: &workcli.WatchConfig{SessionID: "session-default"},
		WatchWork: func(cfg workcli.WatchConfig) error {
			got = cfg
			return nil
		},
	})
	cmd := &cobra.Command{}
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	if err := run(cmd, nil); err != nil {
		t.Fatalf("optional bindings execution error = %v", err)
	}
	if got.SessionID != "session-default" || got.SessionIDExplicit || got.Follow || got.Output == nil || got.Context == nil {
		t.Fatalf("watch config = %#v, want config defaults and command-owned context/output", got)
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
	sessionID := ""
	follow := false
	registry, err := commandregistry.NewWorkRegistry(commandregistry.WorkHandlers{
		ListRunE: func(*cobra.Command, []string) error { return nil },
		WatchRunE: commandregistry.WatchRunE(commandregistry.WatchBinding{
			Config:            &workcli.WatchConfig{},
			SessionID:         &sessionID,
			Follow:            &follow,
			DiagnosticsWriter: func(cmd *cobra.Command) io.Writer { return cmd.ErrOrStderr() },
			WatchWork:         watch,
		}),
		ShowRunE:      func(*cobra.Command, []string) error { return nil },
		MoveRunE:      func(*cobra.Command, []string) error { return nil },
		VisualizeRunE: func(*cobra.Command, []string) error { return nil },
	})
	if err != nil {
		t.Fatalf("NewWorkRegistry() error = %v", err)
	}
	work, err := climanifestcobra.NewWorkFamilyCommand(registry, climanifestcobra.WorkFamilyBindings{
		LocalTargets: map[string]any{
			"you.work.list.flag.state-name":       stringTarget(""),
			"you.work.list.flag.state-type":       stringTarget(""),
			"you.work.list.flag.terminal":         boolTarget(false),
			"you.work.list.flag.non-terminal":     boolTarget(false),
			"you.work.list.flag.name":             stringTarget(""),
			"you.work.list.flag.work-type-name":   stringTarget(""),
			"you.work.list.flag.trace-id":         stringTarget(""),
			"you.work.list.flag.sort-by":          stringTarget(""),
			"you.work.list.flag.max-results":      intTarget(0),
			"you.work.list.flag.next-token":       stringTarget(""),
			"you.work.list.flag.counts":           boolTarget(false),
			"you.work.list.flag.session":          stringTarget(""),
			"you.work.approval.list.flag.session": stringTarget(""),
			"you.work.approval.show.flag.session": stringTarget(""),
			"you.work.watch.flag.follow":          &follow,
			"you.work.watch.flag.session":         &sessionID,
			"you.work.show.flag.session":          stringTarget(""),
			"you.work.move.flag.session":          stringTarget(""),
			"you.work.move.flag.request-id":       stringTarget(""),
			"you.work.visualize.flag.format":      stringTarget("mermaid"),
		},
	})
	if err != nil {
		t.Fatalf("NewWorkFamilyCommand() error = %v", err)
	}
	work.SetOut(output)
	work.SetErr(diagnostics)
	return work
}

func stringTarget(value string) *string { return &value }
func boolTarget(value bool) *bool       { return &value }
func intTarget(value int) *int          { return &value }
