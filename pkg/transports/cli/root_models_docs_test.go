package cli

import (
	"testing"
)

func TestNewModelsDocsHandlerRegistryWiresHandwrittenCommands(t *testing.T) {
	globals := &cliGlobalOptions{}
	diagnostics := &cliDiagnosticsOptions{}
	registry, _, err := newModelsDocsHandlerRegistry(globals, diagnostics, &cliOperatorDefaultsOptions{}, RootCommandOptions{})
	if err != nil {
		t.Fatalf("newModelsDocsHandlerRegistry() error = %v", err)
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

func TestProductionRootUsesGeneratedModelsDocsFamilyCutover(t *testing.T) {
	if !useGeneratedModelsDocsFamily {
		t.Fatal("useGeneratedModelsDocsFamily = false, want production cutover enabled")
	}

	root := NewRootCommand()
	docs, _, err := root.Find([]string{"docs"})
	if err != nil {
		t.Fatalf("Find(docs) error = %v", err)
	}
	if docs.RunE == nil {
		t.Fatal("you docs must attach handwritten RunE through generated cutover")
	}

	models, _, err := root.Find([]string{"models"})
	if err != nil {
		t.Fatalf("Find(models) error = %v", err)
	}
	if models.RunE != nil {
		t.Fatal("you models must remain non-runnable")
	}
	list, _, err := root.Find([]string{"models", "list"})
	if err != nil {
		t.Fatalf("Find(models list) error = %v", err)
	}
	if list.RunE == nil {
		t.Fatal("you models list must attach handwritten RunE through generated cutover")
	}
}
