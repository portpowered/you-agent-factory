package mcpcli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
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

func TestBuildServeApplicationRequiresInjectedService(t *testing.T) {
	application, err := BuildServeApplication(ServeConfig{})
	if err == nil || application != nil {
		t.Fatalf("BuildServeApplication() = (%+v, %v), want missing-service error", application, err)
	}
	if !strings.Contains(err.Error(), "durable execution service is required") {
		t.Fatalf("BuildServeApplication() error = %q, want actionable missing-service error", err)
	}
}

func TestBuildServeApplicationAcceptsInjectedService(t *testing.T) {
	injected := factorysessionexecution.NewFakeService()
	application, err := BuildServeApplication(ServeConfig{Service: injected})
	if err != nil {
		t.Fatalf("BuildServeApplication() error = %v", err)
	}
	if application == nil {
		t.Fatal("BuildServeApplication() = nil, want constructed application")
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
