package packages_test

import (
	"reflect"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorypackages "github.com/portpowered/infinite-you/pkg/factory/packages"
)

func TestCatalogDefinitionsAreRunnableAndMatchMetadata(t *testing.T) {
	wantNames := []string{"@you/deep-research", "@you/fusion", "@you/goal", "@you/quorum", "@you/review", "@you/subagent", "@you/tts"}
	if got := factorypackages.Names(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("Names() = %v, want %v", got, wantNames)
	}

	for _, name := range wantNames {
		t.Run(name, func(t *testing.T) {
			definition, ok := factorypackages.Lookup(name)
			if !ok {
				t.Fatalf("Lookup(%q) did not find packaged definition", name)
			}
			cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(definition.JSON)
			if err != nil {
				t.Fatalf("load packaged definition %q: %v", name, err)
			}
			if cfg.Name != definition.Name {
				t.Fatalf("definition name = %q, loaded name = %q", definition.Name, cfg.Name)
			}
			if cfg.Project != definition.Project {
				t.Fatalf("definition project = %q, loaded project = %q", definition.Project, cfg.Project)
			}
			if interfaces.IsJavaScriptOrchestratorFactory(cfg) {
				if cfg.Orchestrator.JavaScript == nil || cfg.Orchestrator.JavaScript.SourceRef == "" {
					t.Fatalf("JavaScript packaged definition %q has no workflow source: %#v", name, cfg.Orchestrator)
				}
				return
			}
			if len(cfg.Workers) == 0 || len(cfg.Workstations) == 0 || len(cfg.WorkTypes) == 0 {
				t.Fatalf("packaged definition %q is not runnable: %#v", name, cfg)
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
