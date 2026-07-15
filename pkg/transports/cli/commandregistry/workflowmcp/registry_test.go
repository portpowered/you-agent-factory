package workflowmcp_test

import (
	"strings"
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifest"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry/workflowmcp"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

func TestNewRegistriesKeepCanonicalAndCompatibilityHandlersIsolated(t *testing.T) {
	registries, err := workflowmcp.NewRegistries(completeHandlers())
	if err != nil {
		t.Fatalf("NewRegistries() error = %v", err)
	}
	if _, err := registries.MCP.Lookup("you.mcp.serve"); err != nil {
		t.Fatalf("MCP.Lookup(you.mcp.serve) error = %v", err)
	}
	for _, commandID := range []string{"you.workflow.preview", "you.workflow.validate"} {
		if _, err := registries.WorkflowCompatibility.Lookup(commandID); err != nil {
			t.Fatalf("WorkflowCompatibility.Lookup(%q) error = %v", commandID, err)
		}
		if _, err := registries.MCP.Lookup(commandID); err == nil {
			t.Fatalf("MCP.Lookup(%q) error = nil, want classification isolation failure", commandID)
		}
	}
	if _, err := registries.WorkflowCompatibility.Lookup("you.mcp.serve"); err == nil {
		t.Fatal("WorkflowCompatibility.Lookup(you.mcp.serve) error = nil, want classification isolation failure")
	}
}

func TestNewRegistriesRejectsMissingStableHandler(t *testing.T) {
	handlers := completeHandlers()
	handlers.WorkflowPreview = nil
	_, err := workflowmcp.NewRegistries(handlers)
	if err == nil || !strings.Contains(err.Error(), "you.workflow.preview") {
		t.Fatalf("NewRegistries() error = %v, want stable command ID", err)
	}
}

func TestRunnableCommandIDsComeFromClassificationSpecificManifests(t *testing.T) {
	mcpManifest, err := generated.MCPFamilyManifest()
	if err != nil {
		t.Fatalf("MCPFamilyManifest() error = %v", err)
	}
	mcpIDs, err := workflowmcp.RunnableMCPCommandIDs(mcpManifest)
	if err != nil {
		t.Fatalf("RunnableMCPCommandIDs() error = %v", err)
	}
	assertIDs(t, mcpIDs, []string{"you.mcp.serve"})

	workflowManifest, err := generated.WorkflowCompatibilityFamilyManifest()
	if err != nil {
		t.Fatalf("WorkflowCompatibilityFamilyManifest() error = %v", err)
	}
	workflowIDs, err := workflowmcp.RunnableWorkflowCompatibilityCommandIDs(workflowManifest)
	if err != nil {
		t.Fatalf("RunnableWorkflowCompatibilityCommandIDs() error = %v", err)
	}
	assertIDs(t, workflowIDs, []string{"you.workflow.preview", "you.workflow.validate"})
}

func TestRunnableMCPCommandIDsRejectsClassificationMismatch(t *testing.T) {
	manifest, err := generated.MCPFamilyManifest()
	if err != nil {
		t.Fatalf("MCPFamilyManifest() error = %v", err)
	}
	delete(manifest.Commands, "you.mcp")
	manifest.Commands["you.workflow.preview"] = climanifest.Command{ID: "you.workflow.preview"}
	_, err = workflowmcp.RunnableMCPCommandIDs(manifest)
	if err == nil || !strings.Contains(err.Error(), "you.workflow.preview") {
		t.Fatalf("RunnableMCPCommandIDs() error = %v, want mismatched stable command ID", err)
	}
}

func TestVerifyCoverageRejectsMissingHandlerByStableID(t *testing.T) {
	manifest, err := generated.WorkflowCompatibilityFamilyManifest()
	if err != nil {
		t.Fatalf("WorkflowCompatibilityFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you.workflow.validate", noopRunE); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	err = workflowmcp.VerifyWorkflowCompatibilityRunnableCoverage(manifest, registry)
	if err == nil || !strings.Contains(err.Error(), "you.workflow.preview") {
		t.Fatalf("VerifyWorkflowCompatibilityRunnableCoverage() error = %v, want missing stable command ID", err)
	}
}

func completeHandlers() workflowmcp.Handlers {
	return workflowmcp.Handlers{
		MCPServe:         noopRunE,
		WorkflowPreview:  noopRunE,
		WorkflowValidate: noopRunE,
	}
}

func noopRunE(*cobra.Command, []string) error { return nil }

func assertIDs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("IDs = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("IDs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
