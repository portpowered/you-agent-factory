package commandregistry_test

import (
	"testing"

	"github.com/portpowered/infinite-you/pkg/transports/cli/commandregistry"
	"github.com/portpowered/infinite-you/pkg/transports/cli/generated"
)

func TestRunnableModelsDocsCommandIDs(t *testing.T) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		t.Fatalf("ModelsDocsFamilyManifest() error = %v", err)
	}
	ids, err := commandregistry.RunnableModelsDocsCommandIDs(manifest)
	if err != nil {
		t.Fatalf("RunnableModelsDocsCommandIDs() error = %v", err)
	}
	if len(ids) != 5 {
		t.Fatalf("runnable IDs = %#v, want 5 runnable models/docs commands", ids)
	}
}

func TestVerifyModelsDocsRunnableCoverageRejectsMissingHandler(t *testing.T) {
	manifest, err := generated.ModelsDocsFamilyManifest()
	if err != nil {
		t.Fatalf("ModelsDocsFamilyManifest() error = %v", err)
	}
	registry := commandregistry.NewRegistry()
	if err := registry.Register("you.docs", noopRunE); err != nil {
		t.Fatalf("Register(you.docs) error = %v", err)
	}
	if err := registry.VerifyModelsDocsRunnableCoverage(manifest); err == nil {
		t.Fatal("VerifyModelsDocsRunnableCoverage() missing models handlers = nil, want error")
	}
}

func TestNewModelsDocsRegistryWiresAllRunnableCommands(t *testing.T) {
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
}
