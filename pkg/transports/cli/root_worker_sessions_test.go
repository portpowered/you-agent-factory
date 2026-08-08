package cli

import (
	"io"
	"testing"

	workersessionscli "github.com/portpowered/infinite-you/pkg/transports/cli/worker_sessions"
)

func TestWorkerSessionsListCommandMapsManifestInputsToOperation(t *testing.T) {
	var got workersessionscli.ListConfig
	factory := withTestInjectedPlatformRoles(CommandFactory{
		ListWorkerSessions: func(config workersessionscli.ListConfig) error {
			got = config
			return nil
		},
	})
	root := factory.NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json", "--server", "http://factory.test:7437",
		"worker-sessions", "list", "--work-id", "work-1",
		"--session", "session-1", "--output", "json",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute worker-sessions list: %v", err)
	}
	if got.WorkID == "" {
		t.Fatal("worker sessions list operation was not invoked")
	}
	if got.WorkID != "work-1" || got.SessionID != "session-1" || got.Server != "http://factory.test:7437" {
		t.Fatalf("operation config = %#v, want manifest values", got)
	}
	if got.OutputFormat != "json" || !got.JSON {
		t.Fatalf("output config = %#v, want json output", got)
	}
}

func TestWorkerSessionsShowCommandMapsManifestInputsToOperation(t *testing.T) {
	var got workersessionscli.ShowConfig
	factory := withTestInjectedPlatformRoles(CommandFactory{
		ShowWorkerSession: func(config workersessionscli.ShowConfig) error {
			got = config
			return nil
		},
	})
	root := factory.NewCommand(nil, nil, nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{
		"--json", "--server", "http://factory.test:7437",
		"worker-sessions", "show", "--provider", "codex", "--kind", "session_id", "--id", "provider-session-1",
		"--session", "session-1", "--output", "json",
	})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute worker-sessions show: %v", err)
	}
	if got.Provider != "codex" || got.Kind != "session_id" || got.ID != "provider-session-1" || got.SessionID != "session-1" {
		t.Fatalf("operation config = %#v, want manifest identity values", got)
	}
	if got.Server != "http://factory.test:7437" || got.OutputFormat != "json" || !got.JSON {
		t.Fatalf("output/config = %#v, want server and json values", got)
	}
}
