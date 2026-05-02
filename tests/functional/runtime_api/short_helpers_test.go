//go:build !functionallong

package runtime_api

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/projections"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/replay"
)

func projectReplayInitialStructureFromEmbeddedConfig(t *testing.T, dir string) interfaces.InitialStructurePayload {
	t.Helper()

	loaded, err := factoryconfig.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig: %v", err)
	}
	generatedFactory, err := replay.GeneratedFactoryFromLoadedConfig(loaded, replay.WithGeneratedFactorySourceDirectory(loaded.FactoryDir()))
	if err != nil {
		t.Fatalf("GeneratedFactoryFromLoadedConfig: %v", err)
	}
	replayRuntimeCfg, err := replay.RuntimeConfigFromGeneratedFactory(generatedFactory)
	if err != nil {
		t.Fatalf("RuntimeConfigFromGeneratedFactory: %v", err)
	}
	mapper := factoryconfig.ConfigMapper{}
	replayNet, err := mapper.Map(context.Background(), replayRuntimeCfg.Factory)
	if err != nil {
		t.Fatalf("Map replay factory: %v", err)
	}
	return projections.ProjectInitialStructure(replayNet, replayRuntimeCfg)
}

func writeWorkstationConfig(t *testing.T, dir, workstationName, content string) {
	t.Helper()

	path := filepath.Join(dir, "workstations", workstationName, "AGENTS.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create workstation config dir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
