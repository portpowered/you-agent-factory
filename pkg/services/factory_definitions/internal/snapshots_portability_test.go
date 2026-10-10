package internal_test

import (
	"errors"
	"testing"

	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factoryinternal "github.com/portpowered/infinite-you/pkg/services/factory_definitions/internal"
)

type materializedSnapshotSource struct {
	factorydefinitions.MutableLoadedFactorySource
	baseDir string
}

func (source *materializedSnapshotSource) SetRuntimeBaseDir(dir string) { source.baseDir = dir }

func TestRuntimeSnapshotMaterializationDetachesSelectionsAndPreservesProvenance(t *testing.T) {
	t.Parallel()
	snapshot := factorydefinitions.RuntimeSnapshot{
		FactoryDir: "/factory", RuntimeBaseDir: "/runtime",
		EffectiveFactory:                factorydefinitions.FactoryConfig{Name: "factory", Workers: []factorydefinitions.FactoryWorkerConfig{{Name: "worker"}}},
		Workers:                         []factorydefinitions.FactoryWorkerConfig{{Name: "worker", ReasoningEffort: "high"}},
		PromptSources:                   []factorydefinitions.RuntimePromptSource{{Role: "worker", Name: "worker", Path: "worker.md"}},
		PromptProvenance:                []factorydefinitions.RuntimePromptProvenance{{Name: "worker"}},
		InvocationSensitiveJSONPointers: []string{"/workers/0/prompt"},
	}
	var configs []*factorydefinitions.FactoryConfig
	var lookups []factorydefinitions.RuntimeDefinitionLookup
	var sources []*materializedSnapshotSource
	owner := factoryinternal.NewRuntimeSnapshotMaterializer(func(dir string, config *factorydefinitions.FactoryConfig,
		lookup factorydefinitions.RuntimeDefinitionLookup, _ []factorydefinitions.PortableBundledFileReplacement,
	) (factorydefinitions.MutableLoadedFactorySource, error) {
		if dir != snapshot.FactoryDir {
			t.Fatalf("materialized directory = %s", dir)
		}
		source := &materializedSnapshotSource{}
		configs, lookups, sources = append(configs, config), append(lookups, lookup), append(sources, source)
		return source, nil
	})
	first, err := owner.Materialize(&snapshot, "")
	if err != nil {
		t.Fatal(err)
	}
	second, err := owner.Materialize(&snapshot, "/selected")
	if err != nil {
		t.Fatal(err)
	}
	snapshot.Workers[0].ReasoningEffort = "low"
	snapshot.EffectiveFactory.Workers[0].Name = "caller changed"
	snapshot.InvocationSensitiveJSONPointers[0] = "caller changed"
	for index, loaded := range []factorydefinitions.MutableLoadedFactorySource{first, second} {
		worker, ok := lookups[index].Worker("worker")
		if !ok || worker.ReasoningEffort != "high" || configs[index].Workers[0].PromptSourcePath != "worker.md" {
			t.Fatalf("opening %d lost detached prompt/source selections", index)
		}
		provenance := loaded.(factorydefinitions.RuntimePromptProvenanceLookup)
		if selected, ok := provenance.WorkerPromptProvenance("worker"); !ok || selected.Name != "worker" {
			t.Fatalf("opening %d lost prompt provenance", index)
		}
		pointers := loaded.(interface{ InvocationSensitiveJSONPointers() []string }).InvocationSensitiveJSONPointers()
		if len(pointers) != 1 || pointers[0] != "/workers/0/prompt" {
			t.Fatalf("opening %d lost redaction provenance: %v", index, pointers)
		}
	}
	if sources[0].baseDir != "/runtime" || sources[1].baseDir != "/selected" || configs[0] == configs[1] {
		t.Fatal("materialization retargeted an earlier opening's data")
	}
}

func TestRuntimeSnapshotMaterializationReturnsSelectedFailureWithoutPublication(t *testing.T) {
	t.Parallel()
	failure := errors.New("selected materialization failed")
	owner := factoryinternal.NewRuntimeSnapshotMaterializer(func(string, *factorydefinitions.FactoryConfig,
		factorydefinitions.RuntimeDefinitionLookup, []factorydefinitions.PortableBundledFileReplacement,
	) (factorydefinitions.MutableLoadedFactorySource, error) {
		return nil, failure
	})
	snapshot := factorydefinitions.RuntimeSnapshot{FactoryDir: "/factory", EffectiveFactory: factorydefinitions.FactoryConfig{Name: "factory"}}
	loaded, err := owner.Materialize(&snapshot, "")
	if loaded != nil || !errors.Is(err, failure) {
		t.Fatalf("failed materialization = %v, %v", loaded, err)
	}
	if loaded, err := owner.Materialize(nil, ""); loaded != nil || err == nil {
		t.Fatal("missing snapshot was accepted")
	}
}
