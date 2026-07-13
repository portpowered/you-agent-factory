package mcpcli

import (
	"bytes"
	"path/filepath"
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
	} else if !strings.Contains(got, "pkg/api/testdata/durable-session-contract-fixtures.json") {
		t.Fatalf("help missing the real fixture catalog path:\n%s", got)
	}
}

func TestResolveServeService_RuntimeBackedSelectsJavaScriptRuntimeService(t *testing.T) {
	projectRoot := t.TempDir()
	service, err := resolveServeService(ServeConfig{
		RuntimeBacked: true,
		ProjectRoot:   projectRoot,
	})
	if err != nil {
		t.Fatalf("resolveServeService: %v", err)
	}
	if _, ok := service.(*factorysessionexecution.JavaScriptRuntimeService); !ok {
		t.Fatalf("service type = %T, want *factorysessionexecution.JavaScriptRuntimeService", service)
	}
}

func TestResolveServeService_DefaultSelectsFixtureService(t *testing.T) {
	path := filepath.Join("..", "..", "api", "testdata", "durable-session-contract-fixtures.json")
	service, err := resolveServeService(ServeConfig{FixtureCatalogPath: path})
	if err != nil {
		t.Fatalf("resolveServeService: %v", err)
	}
	if _, ok := service.(*factorysessionexecution.FakeService); !ok {
		t.Fatalf("service type = %T, want *factorysessionexecution.FakeService", service)
	}
}

func TestResolveServeService_InjectedServiceTakesPrecedence(t *testing.T) {
	injected := factorysessionexecution.NewFakeService()
	service, err := resolveServeService(ServeConfig{
		RuntimeBacked: true,
		Service:       injected,
	})
	if err != nil {
		t.Fatalf("resolveServeService: %v", err)
	}
	if service != injected {
		t.Fatalf("service = %p, want injected %p", service, injected)
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
