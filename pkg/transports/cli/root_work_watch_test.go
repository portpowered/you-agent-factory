package cli

import (
	"bytes"
	"io"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	workcli "github.com/portpowered/infinite-you/pkg/transports/cli/work"
)

func TestProductionWorkHandlerRegistryExecutesWatch(t *testing.T) {
	var got workcli.WatchConfig
	registry, bindings, err := newWorkHandlerRegistry(
		&cliGlobalOptions{server: "https://factory.example"},
		&cliDiagnosticsOptions{verbose: true, debug: true},
		CommandFactory{
			WatchWork: func(cfg workcli.WatchConfig) error {
				got = cfg
				_, err := io.WriteString(cfg.Output, "watched\n")
				return err
			},
		},
	)
	if err != nil {
		t.Fatalf("newWorkHandlerRegistry() error = %v", err)
	}
	work, err := climanifestcobra.NewWorkFamilyCommand(registry, bindings)
	if err != nil {
		t.Fatalf("NewWorkFamilyCommand() error = %v", err)
	}

	var stdout, stderr bytes.Buffer
	work.SetOut(&stdout)
	work.SetErr(&stderr)
	work.SetArgs([]string{"watch", "--session", "session-alpha", "--follow"})
	if err := work.Execute(); err != nil {
		t.Fatalf("work watch Execute() error = %v", err)
	}

	if got.Context == nil || got.Server != "https://factory.example" || got.SessionID != "session-alpha" ||
		!got.SessionIDExplicit || !got.Follow || !got.Verbose || !got.Debug || got.Output != &stdout || got.Diagnostics != &stderr {
		t.Fatalf("watch config = %#v, want production stable-input mapping", got)
	}
	if stdout.String() != "watched\n" || stderr.Len() != 0 {
		t.Fatalf("watch output = %q, diagnostics = %q", stdout.String(), stderr.String())
	}
}
