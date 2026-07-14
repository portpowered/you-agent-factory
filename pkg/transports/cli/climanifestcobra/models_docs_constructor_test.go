package climanifestcobra_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestgen"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestcobra"
	"github.com/portpowered/infinite-you/pkg/transports/cli/climanifestparity"
	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
	"github.com/spf13/cobra"
)

func TestNewModelsDocsFamilyComponentsBuildsContractedPaths(t *testing.T) {
	components, registry := mustModelsDocsFamilyComponents(t)

	if components.Docs.Name() != "docs" {
		t.Fatalf("docs name = %q, want docs", components.Docs.Name())
	}
	if !components.Docs.Runnable() || components.Docs.RunE == nil {
		t.Fatal("you docs must attach handwritten RunE")
	}
	if components.Models.Name() != "models" {
		t.Fatalf("models name = %q, want models", components.Models.Name())
	}
	if components.Models.Runnable() {
		t.Fatal("you models must remain non-runnable")
	}
	if len(components.Models.Commands()) != 4 {
		t.Fatalf("models child count = %d, want 4 leaves", len(components.Models.Commands()))
	}

	for _, commandID := range []string{
		"you.docs",
		"you.models.list",
		"you.models.inspect",
		"you.models.invoke",
		"you.models.pull",
	} {
		if _, err := registry.Lookup(commandID); err != nil {
			t.Fatalf("Lookup(%s) error = %v", commandID, err)
		}
	}

	for _, tc := range []struct {
		root *cobra.Command
		path string
	}{
		{components.Docs, "docs"},
		{components.Models, "models list"},
		{components.Models, "models inspect"},
		{components.Models, "models invoke"},
		{components.Models, "models pull"},
	} {
		if _, err := climanifestparity.FindCommandByPath(tc.root, tc.path); err != nil {
			t.Fatalf("FindCommandByPath(%q) error = %v", tc.path, err)
		}
	}
}

func TestNewModelsDocsFamilyComponentsRejectsMissingHandler(t *testing.T) {
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you.docs", noopRunE); err != nil {
		t.Fatalf("Register(you.docs) error = %v", err)
	}
	_, err := climanifestcobra.NewModelsDocsFamilyComponents(registry, testInvokeFlagBindings())
	if err == nil {
		t.Fatal("NewModelsDocsFamilyComponents() missing models handlers = nil, want error")
	}
}

func TestNewModelsDocsFamilyComponentsRejectsOutOfFamilyManifestCommand(t *testing.T) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		t.Fatalf("ModelsDocsFamilyManifest() error = %v", err)
	}
	manifest.Commands["you.run"] = manifest.Commands["you.docs"]
	delete(manifest.Commands, "you.docs")

	registry, err := commandregistry.NewModelsDocsRegistry(commandregistry.ModelsDocsHandlers{
		DocsRunE:          noopRunE,
		ModelsListRunE:    noopRunE,
		ModelsInspectRunE: noopRunE,
		ModelsInvokeRunE:  noopRunE,
		ModelsPullRunE:    noopRunE,
	})
	if err != nil {
		t.Fatalf("NewModelsDocsRegistry() error = %v", err)
	}

	_, err = climanifestcobra.NewModelsDocsFamilyComponentsFromManifest(manifest, registry, testInvokeFlagBindings())
	if err == nil {
		t.Fatal("NewModelsDocsFamilyComponentsFromManifest() error = nil, want out-of-family rejection")
	}
}

func TestNewModelsDocsFamilyComponentsRegistersInvokeLocalFlags(t *testing.T) {
	components, _ := mustModelsDocsFamilyComponents(t)
	invoke, err := climanifestparity.FindCommandByPath(components.Models, "models invoke")
	if err != nil {
		t.Fatalf("FindCommandByPath(you models invoke) error = %v", err)
	}
	for _, flag := range []string{"operation", "text", "output", "port"} {
		if invoke.Flags().Lookup(flag) == nil {
			t.Fatalf("invoke missing local flag %q", flag)
		}
	}
	portFlag := invoke.Flags().Lookup("port")
	if portFlag == nil || !portFlag.Hidden {
		t.Fatalf("invoke port flag = %#v, want hidden deprecated flag", portFlag)
	}
}

func mustModelsDocsFamilyComponents(t *testing.T) (climanifestcobra.ModelsDocsFamilyComponents, *commandregistry.Registry) {
	t.Helper()
	registry, err := commandregistry.NewModelsDocsRegistry(commandregistry.ModelsDocsHandlers{
		DocsRunE:          noopRunE,
		ModelsListRunE:    noopRunE,
		ModelsInspectRunE: noopRunE,
		ModelsInvokeRunE:  noopRunE,
		ModelsPullRunE:    noopRunE,
	})
	if err != nil {
		t.Fatalf("NewModelsDocsRegistry() error = %v", err)
	}
	components, err := climanifestcobra.NewModelsDocsFamilyComponents(registry, testInvokeFlagBindings())
	if err != nil {
		t.Fatalf("NewModelsDocsFamilyComponents() error = %v", err)
	}
	return components, registry
}

func testInvokeFlagBindings() climanifestcobra.ModelsInvokeFlagBindings {
	operation := "TTS"
	text := ""
	output := ""
	return climanifestcobra.ModelsInvokeFlagBindings{
		Operation:  &operation,
		Text:       &text,
		OutputPath: &output,
		FlagUsages: map[string]string{
			"operation": "uppercase provider-agnostic operation name",
			"text":      "text input for direct invocation",
			"output":    "output file path for streamed audio responses",
		},
	}
}

func TestModelsDocsFamilyCommandIDsStayWithinGeneratorScope(t *testing.T) {
	for _, id := range climanifestgen.ModelsDocsFamilyCommandIDs {
		if err := climanifestgen.AssertModelsDocsFamilyCommandID(id); err != nil {
			t.Fatalf("AssertModelsDocsFamilyCommandID(%s) error = %v", id, err)
		}
	}
	if err := climanifestgen.AssertModelsDocsFamilyCommandID("you.run"); err == nil {
		t.Fatal("expected out-of-family rejection for you.run")
	}
}
