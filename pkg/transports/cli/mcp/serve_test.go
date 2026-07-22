package mcpcli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	startupcli "github.com/portpowered/infinite-you/pkg/initializer/process"
	"github.com/spf13/cobra"
)

func TestNewServeCommand_HelpRoutesToCanonicalMCPTopic(t *testing.T) {
	cmd := NewServeCommand()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if got := output.String(); !strings.Contains(got, "you docs mcp") {
		t.Fatalf("help missing canonical MCP topic:\n%s", got)
	} else if strings.Contains(got, "mcp-hosts") {
		t.Fatalf("help still routes to retired MCP topic:\n%s", got)
	} else if !strings.Contains(got, "pkg/transports/http/testdata/durable-session-contract-fixtures.json") {
		t.Fatalf("help missing the real fixture catalog path:\n%s", got)
	}
}

func TestNewServeCommand_RejectsRuntimeAndFixtureCatalogTogether(t *testing.T) {
	cmd := NewServeCommand()
	if err := cmd.Flags().Set("runtime", "true"); err != nil {
		t.Fatalf("set runtime flag: %v", err)
	}
	if err := cmd.Flags().Set("fixture-catalog", "/tmp/fixtures.json"); err != nil {
		t.Fatalf("set fixture-catalog flag: %v", err)
	}
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("RunE: expected error combining --runtime with --fixture-catalog")
	}
	if !strings.Contains(err.Error(), "cannot combine --runtime with --fixture-catalog") {
		t.Fatalf("RunE error = %q, want combined-flag rejection", err.Error())
	}
}

func TestNewServeCommand_RequiresInjectedStdioInitializer(t *testing.T) {
	cmd := NewServeCommand()
	err := cmd.RunE(cmd, nil)
	if err == nil || !strings.Contains(err.Error(), "MCP stdio initializer is required") {
		t.Fatalf("RunE() error = %v, want missing injected initializer", err)
	}
}

func TestServeRunEDelegatesUnchangedIntentToStdioInitializer(t *testing.T) {
	fixtureCatalogPath := "fixtures.json"
	runtimeBacked := false
	projectRoot := "/workspace/project"
	stdin := strings.NewReader("request\n")
	var stdout bytes.Buffer
	var got startupcli.MCPIntent
	runE := ServeRunE(ServeBinding{
		FixtureCatalogPath: &fixtureCatalogPath,
		RuntimeBacked:      &runtimeBacked,
		ProjectRoot:        &projectRoot,
		HomeDir:            func() (string, error) { return "/home/test", nil },
		InitializeStdio: func(_ context.Context, intent startupcli.MCPIntent) error {
			got = intent
			return nil
		},
	})
	cmd := &cobra.Command{Use: "serve"}
	cmd.SetIn(stdin)
	cmd.SetOut(&stdout)
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("ServeRunE() error = %v", err)
	}
	if got.FixtureCatalogPath != fixtureCatalogPath || got.RuntimeBacked || got.ProjectRoot != projectRoot {
		t.Fatalf("stdio intent = %#v, want bound fixture/runtime/project-root", got)
	}
	if got.Stdin != io.Reader(stdin) || got.Stdout != io.Writer(&stdout) {
		t.Fatalf("stdio = (%T, %T), want original command streams", got.Stdin, got.Stdout)
	}
}

func TestServeRunERuntimeCarriesInjectedHomeToInitializer(t *testing.T) {
	runtimeBacked := true
	projectRoot := "/workspace/project"
	var got startupcli.MCPIntent
	runE := ServeRunE(ServeBinding{
		RuntimeBacked: &runtimeBacked,
		ProjectRoot:   &projectRoot,
		HomeDir:       func() (string, error) { return "/home/test", nil },
		InitializeStdio: func(_ context.Context, intent startupcli.MCPIntent) error {
			got = intent
			return nil
		},
	})
	cmd := &cobra.Command{Use: "serve"}
	cmd.SetIn(strings.NewReader(""))
	cmd.SetOut(io.Discard)
	if err := runE(cmd, nil); err != nil {
		t.Fatalf("ServeRunE() error = %v", err)
	}
	if got.HomeDir != "/home/test" || got.ProjectRoot != projectRoot || !got.RuntimeBacked {
		t.Fatalf("stdio intent = %#v, want injected home and runtime roots", got)
	}
}
