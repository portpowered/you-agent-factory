package classifier

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/config/operatorconfig"
	"github.com/portpowered/infinite-you/pkg/config/operatordefaultsruntime"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
)

func TestBuiltInFactoryJSON_MapsComplexityLabelsToOnlyTheirTargetWorkstations(t *testing.T) {
	cfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON: %v", err)
	}
	if cfg.Name != PackagedFactoryName || cfg.Project != PackagedFactoryProject {
		t.Fatalf("packaged identity = %q/%q", cfg.Name, cfg.Project)
	}

	classifier := workstation(t, cfg, ClassifierWorkstation)
	if classifier.Type != interfaces.WorkstationTypeClassify || len(classifier.Outputs) != 0 {
		t.Fatalf("classifier routing = %#v, want classificationRoutes without normal outputs", classifier)
	}
	assertPresetReferences(t, cfg)

	net, err := (&factoryconfig.ConfigMapper{}).Map(context.Background(), cfg)
	if err != nil {
		t.Fatalf("ConfigMapper.Map: %v", err)
	}
	transition := net.Transitions[ClassifierWorkstation]
	if transition == nil {
		t.Fatal("classifier transition is missing")
	}
	assertClassificationArc(t, transition.OutputArcs, "small", "task:small")
	assertClassificationArc(t, transition.OutputArcs, "medium", "task:medium")
	assertClassificationArc(t, transition.OutputArcs, "large", "task:large")
	if len(transition.OutputArcs) != 3 {
		t.Fatalf("classifier output arcs = %#v, want exactly one per label", transition.OutputArcs)
	}
	if len(transition.FailureArcs) != 1 || transition.FailureArcs[0].PlaceID != "task:failed" {
		t.Fatalf("classifier failure arcs = %#v, want task:failed for invalid labels", transition.FailureArcs)
	}
}

func TestBuiltInFactory_OperatorModelOverridesPreserveAuthoredTierRoutes(t *testing.T) {
	factoryDir, err := factoryconfig.PersistNamedFactory(t.TempDir(), PackagedFactoryName, BuiltInFactoryJSON)
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	loaded, err := factoryconfig.LoadRuntimeConfigFromFactoryDir(factoryDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfigFromFactoryDir: %v", err)
	}

	if err := operatordefaultsruntime.ApplyToLoadedConfig(loaded, operatorconfig.ResolvedDefaults{
		ConfigPath:                writeBaselinePresets(t),
		WorkerModelProvider:       "CLAUDE",
		WorkerModel:               "claude-sonnet-4-20250514",
		WorkerModelProviderSource: operatorconfig.SourceFlag,
		WorkerModelSource:         operatorconfig.SourceFlag,
	}); err != nil {
		t.Fatalf("ApplyToLoadedConfig: %v", err)
	}

	cfg := loaded.FactoryConfig()
	assertPresetReferences(t, cfg)
	if worker := workerByName(t, cfg, "run-large"); worker.ModelProvider != "CODEX" || worker.Model != "gpt-5.4" {
		t.Fatalf("resolved large worker = %s/%s, want CODEX/gpt-5.4", worker.ModelProvider, worker.Model)
	}
	classifier := workstation(t, cfg, ClassifierWorkstation)
	for _, route := range classifier.ClassificationRoutes {
		if len(route.Outputs) != 1 || route.Outputs[0].StateName != route.Label {
			t.Fatalf("route %q = %#v, want its matching target state", route.Label, route)
		}
	}
}

func assertPresetReferences(t *testing.T, cfg *interfaces.FactoryConfig) {
	t.Helper()
	for workerName, presetID := range map[string]string{
		"classify-complexity": operatorconfig.ClassifierSmallPresetID,
		"run-small":           operatorconfig.ClassifierSmallPresetID,
		"run-medium":          operatorconfig.ClassifierMediumPresetID,
		"run-large":           operatorconfig.ClassifierLargePresetID,
	} {
		worker := workerByName(t, cfg, workerName)
		if worker.Preset != presetID {
			t.Fatalf("worker %q preset = %q, want %q", workerName, worker.Preset, presetID)
		}
	}
}

func writeBaselinePresets(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(`{"workerPresets":[{"id":"small","modelProvider":"CODEX","model":"gpt-5-mini"},{"id":"medium","modelProvider":"CODEX","model":"gpt-5"},{"id":"large","modelProvider":"CODEX","model":"gpt-5.4"}]}`), 0o600); err != nil {
		t.Fatalf("write operator config: %v", err)
	}
	return path
}

func workerByName(t *testing.T, cfg *interfaces.FactoryConfig, name string) interfaces.WorkerConfig {
	t.Helper()
	for _, candidate := range cfg.Workers {
		if candidate.Name == name {
			return candidate
		}
	}
	t.Fatalf("worker %q is missing", name)
	return interfaces.WorkerConfig{}
}

func workstation(t *testing.T, cfg *interfaces.FactoryConfig, name string) interfaces.FactoryWorkstationConfig {
	t.Helper()
	for _, candidate := range cfg.Workstations {
		if candidate.Name == name {
			return candidate
		}
	}
	t.Fatalf("workstation %q is missing", name)
	return interfaces.FactoryWorkstationConfig{}
}

func assertClassificationArc(t *testing.T, arcs []petri.Arc, label, placeID string) {
	t.Helper()
	for _, arc := range arcs {
		if arc.ClassificationLabel == label && arc.PlaceID == placeID {
			return
		}
	}
	t.Fatalf("classification route %q to %q missing from %#v", label, placeID, arcs)
}
