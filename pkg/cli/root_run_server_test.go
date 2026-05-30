package cli

import (
	"context"
	"io"
	"strings"
	"testing"

	runcli "github.com/portpowered/infinite-you/pkg/cli/run"
)

func TestRunCommand_DefaultServerEnablesAutoPortAndLocalBind(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run: %v", err)
	}
	if got.Port != 7437 {
		t.Fatalf("port = %d, want 7437", got.Port)
	}
	if got.BindHost != "localhost" {
		t.Fatalf("bind host = %q, want localhost", got.BindHost)
	}
	if !got.AutoPort {
		t.Fatal("expected default --server to enable automatic port resolution")
	}
}

func TestRunCommand_ExplicitServerDerivesBindPortAndDisablesAutoPort(t *testing.T) {
	originalRunCLI := runCLI
	defer func() {
		runCLI = originalRunCLI
	}()

	var got runcli.RunConfig
	runCLI = func(_ context.Context, cfg runcli.RunConfig) error {
		got = cfg
		return nil
	}

	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--server", "http://127.0.0.1:9090"})

	if err := root.Execute(); err != nil {
		t.Fatalf("execute run --server: %v", err)
	}
	if got.Port != 9090 {
		t.Fatalf("port = %d, want 9090", got.Port)
	}
	if got.BindHost != "127.0.0.1" {
		t.Fatalf("bind host = %q, want 127.0.0.1", got.BindHost)
	}
	if got.AutoPort {
		t.Fatal("expected explicit --server to disable automatic port resolution")
	}
}

func TestRunCommand_NonLocalServerRejected(t *testing.T) {
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--server", "https://remote.example.com:7443"})

	if execErr := root.Execute(); execErr == nil {
		t.Fatal("expected non-local --server rejection")
	} else if !strings.Contains(execErr.Error(), "not a local bind target") {
		t.Fatalf("error = %v, want local bind guidance", execErr)
	}
}

func TestRunCommand_PortFlagRejected(t *testing.T) {
	root := NewRootCommand()
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"run", "--port", "7437"})

	if execErr := root.Execute(); execErr == nil {
		t.Fatal("expected --port rejection")
	} else if !strings.Contains(execErr.Error(), "--server") {
		t.Fatalf("error = %v, want --server guidance", execErr)
	}
}
