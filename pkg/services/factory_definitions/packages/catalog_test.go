package packages_test

import (
	"reflect"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions/contracts"
	factorypackages "github.com/portpowered/infinite-you/pkg/services/factory_definitions/packages"
	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"
)

var expectedCatalog = []struct {
	name    string
	project string
}{
	{name: "@you/deep-research", project: "builtin-deep-research"},
	{name: "@you/fusion", project: "builtin-fusion"},
	{name: "@you/goal", project: "builtin-goal"},
	{name: "@you/quorum", project: "builtin-quorum"},
	{name: "@you/review", project: "builtin-review"},
	{name: "@you/subagent", project: "builtin-subagent"},
	{name: "@you/tts", project: "builtin-tts"},
}

func TestCatalogPublicIdentitiesRemainStable(t *testing.T) {
	for _, expected := range expectedCatalog {
		definition, ok := factorypackages.Lookup(expected.name)
		if !ok || definition.Name != expected.name || definition.Project != expected.project {
			t.Errorf("Lookup(%q) identity = (%q, %q, found=%t), want (%q, %q, found=true)",
				expected.name, definition.Name, definition.Project, ok, expected.name, expected.project)
		}
	}
}

func TestCatalogDefinitionsAreRunnableAndMatchMetadata(t *testing.T) {
	wantNames := make([]string, 0, len(expectedCatalog))
	for _, expected := range expectedCatalog {
		wantNames = append(wantNames, expected.name)
	}
	if got := factorypackages.Names(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("Names() = %v, want %v", got, wantNames)
	}

	for _, expected := range expectedCatalog {
		t.Run(expected.name, func(t *testing.T) {
			definition, ok := factorypackages.Lookup(expected.name)
			if !ok {
				t.Fatalf("Lookup(%q) did not find packaged definition", expected.name)
			}
			cfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(definition.JSON)
			if err != nil {
				t.Fatalf("load packaged definition %q: %v", expected.name, err)
			}
			if cfg.Name != definition.Name {
				t.Fatalf("definition name = %q, loaded name = %q", definition.Name, cfg.Name)
			}
			if cfg.Project != definition.Project {
				t.Fatalf("definition project = %q, loaded project = %q", definition.Project, cfg.Project)
			}
			if interfaces.IsJavaScriptOrchestratorFactory(cfg) {
				if cfg.Orchestrator.JavaScript == nil || cfg.Orchestrator.JavaScript.SourceRef == "" {
					t.Fatalf("JavaScript packaged definition %q has no workflow source: %#v", expected.name, cfg.Orchestrator)
				}
				return
			}
			if len(cfg.Workers) == 0 || len(cfg.Workstations) == 0 || len(cfg.WorkTypes) == 0 {
				t.Fatalf("packaged definition %q is not runnable: %#v", expected.name, cfg)
			}
		})
	}
}

func TestLookupReturnsIsolatedPayloadAndMissingOutcome(t *testing.T) {
	first, ok := factorypackages.Lookup("@you/goal")
	if !ok {
		t.Fatal("Lookup(@you/goal) did not find packaged definition")
	}
	first.JSON[0] = 'x'

	second, ok := factorypackages.Lookup("@you/goal")
	if !ok {
		t.Fatal("second Lookup(@you/goal) did not find packaged definition")
	}
	if second.JSON[0] == 'x' {
		t.Fatal("Lookup returned shared mutable packaged payload")
	}
	if _, ok := factorypackages.Lookup("@you/missing"); ok {
		t.Fatal("Lookup(@you/missing) unexpectedly found a definition")
	}
}
