package commandregistry_test

import (
	"bytes"
	"io"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
	"github.com/spf13/cobra"
)

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
			"you.work.list.flag.state-name":     stringTarget(""),
			"you.work.list.flag.state-type":     stringTarget(""),
			"you.work.list.flag.name":           stringTarget(""),
			"you.work.list.flag.work-type-name": stringTarget(""),
			"you.work.list.flag.trace-id":       stringTarget(""),
			"you.work.list.flag.sort-by":        stringTarget(""),
			"you.work.list.flag.max-results":    intTarget(0),
			"you.work.list.flag.next-token":     stringTarget(""),
			"you.work.list.flag.session":        stringTarget(""),
			"you.work.watch.flag.follow":        &follow,
			"you.work.watch.flag.session":       &sessionID,
			"you.work.show.flag.session":        stringTarget(""),
			"you.work.move.flag.session":        stringTarget(""),
			"you.work.move.flag.request-id":     stringTarget(""),
			"you.work.visualize.flag.format":    stringTarget("mermaid"),
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
