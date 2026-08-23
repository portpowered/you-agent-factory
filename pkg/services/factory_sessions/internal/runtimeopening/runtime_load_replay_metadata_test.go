package runtimeopening

import (
	"testing"

	"github.com/portpowered/infinite-you/internal/testutil/factorydefinitionfixtures"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factorydefinitionswire "github.com/portpowered/infinite-you/pkg/services/factory_definitions/wire"
	operatorconfig "github.com/portpowered/infinite-you/pkg/services/operator_settings"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestWarnReplayMetadataMismatchesResolvesCurrentOperatorDefaults(t *testing.T) {
	t.Parallel()

	defaults := operatorconfig.ResolvedDefaults{
		WorkerModelProvider: "CODEX",
		WorkerModel:         "replay-model",
	}
	factoryConfig := &factorydefinitions.FactoryConfig{
		Name: "factory",
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name: "worker",
			Type: factorydefinitions.WorkerTypeModel,
		}},
	}
	factoryDir := t.TempDir()
	recorded, err := factorydefinitionfixtures.NewLoadedSource(
		factoryDir, factoryConfig, nil, nil,
	)
	if err != nil {
		t.Fatalf("construct recorded source: %v", err)
	}
	if err := applyOperatorDefaults(recorded, defaults); err != nil {
		t.Fatalf("apply recorded operator defaults: %v", err)
	}
	capture := factorydefinitionswire.LoadedFactorySnapshotCapturer()
	artifactFactory, err := capture(recorded, factoryDir, nil)
	if err != nil {
		t.Fatalf("capture recorded source: %v", err)
	}

	current, err := factorydefinitionfixtures.NewLoadedSource(
		factoryDir, factoryConfig, nil, nil,
	)
	if err != nil {
		t.Fatalf("construct current source: %v", err)
	}
	core, logs := observer.New(zapcore.WarnLevel)
	warnReplayMetadataMismatches(
		factoryDir,
		"recording.replay.json",
		nil,
		&factorydefinitions.ReplayArtifact{Factory: artifactFactory},
		zap.New(core),
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return current, nil
		},
		capture,
		defaults,
	)
	if logs.Len() != 0 {
		t.Fatalf("equivalent effective config emitted replay metadata warnings: %v", logs.All())
	}
}

func TestWarnReplayMetadataMismatchesReturnsStructuredComponentWarnings(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	recordedConfig := &factorydefinitions.FactoryConfig{
		Name: "factory",
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name: "worker", Type: factorydefinitions.WorkerTypeModel, Body: "recorded prompt",
		}},
	}
	currentConfig := &factorydefinitions.FactoryConfig{
		Name: "factory",
		Workers: []factorydefinitions.FactoryWorkerConfig{{
			Name: "worker", Type: factorydefinitions.WorkerTypeModel, Body: "changed prompt",
		}},
	}
	recorded, err := factorydefinitionfixtures.NewLoadedSource(factoryDir, recordedConfig, nil, nil)
	if err != nil {
		t.Fatalf("construct recorded source: %v", err)
	}
	current, err := factorydefinitionfixtures.NewLoadedSource(factoryDir, currentConfig, nil, nil)
	if err != nil {
		t.Fatalf("construct current source: %v", err)
	}
	capture := factorydefinitionswire.LoadedFactorySnapshotCapturer()
	artifactFactory, err := capture(recorded, factoryDir, nil)
	if err != nil {
		t.Fatalf("capture recorded source: %v", err)
	}
	core, logs := observer.New(zapcore.WarnLevel)
	warnings := warnReplayMetadataMismatches(
		factoryDir,
		"recording.replay.json",
		nil,
		&factorydefinitions.ReplayArtifact{Factory: artifactFactory},
		zap.New(core),
		func(string, factorydefinitions.WorkstationLoader) (factorydefinitions.MutableLoadedFactorySource, error) {
			return current, nil
		},
		capture,
		operatorconfig.ResolvedDefaults{},
	)
	gotKeys := make(map[string]bool, len(warnings))
	for _, warning := range warnings {
		gotKeys[warning.Key] = true
	}
	for _, key := range []string{"factory_hash", "workers_hash", "runtime_config_hash"} {
		if !gotKeys[key] {
			t.Fatalf("metadata warnings = %#v, missing %q", warnings, key)
		}
	}
	if logs.Len() != len(warnings) {
		t.Fatalf("structured warning log count = %d, want %d", logs.Len(), len(warnings))
	}
	for _, entry := range logs.All() {
		if entry.ContextMap()["metadata_key"] == nil {
			t.Fatalf("structured warning omitted metadata_key: %#v", entry)
		}
	}
}
