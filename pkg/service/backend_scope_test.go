package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/portpowered/infinite-you/pkg/config/systemconfig"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"go.uber.org/zap"
)

func TestEnsureServiceBackendScope_GeneratesAndPersistsBeforeServiceBuild(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())

	homeDir := t.TempDir()
	configPath := systemconfig.DefaultConfigPath(homeDir)

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                                     dir,
		Logger:                                  zap.NewNop(),
		SystemConfigHomeDir:                     homeDir,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if !systemconfig.IsLocalBackendScopeID(svc.cfg.BackendScopeID) {
		t.Fatalf("BackendScopeID = %q, want local-<uuid>", svc.cfg.BackendScopeID)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var persisted struct {
		BackendScopeID string `json:"backendScopeID"`
	}
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if persisted.BackendScopeID != svc.cfg.BackendScopeID {
		t.Fatalf("persisted backendScopeID = %q, want %q", persisted.BackendScopeID, svc.cfg.BackendScopeID)
	}
}

func TestEnsureServiceBackendScope_ExplicitScopeSkipsPersistence(t *testing.T) {
	t.Parallel()

	homeDir := t.TempDir()
	configPath := systemconfig.DefaultConfigPath(homeDir)
	existing := "local-aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"

	cfg := &FactoryServiceConfig{
		BackendScopeID:      existing,
		SystemConfigHomeDir: homeDir,
		Logger:              zap.NewNop(),
	}
	if err := ensureServiceBackendScope(cfg, cfg.Logger); err != nil {
		t.Fatalf("ensureServiceBackendScope: %v", err)
	}
	if cfg.BackendScopeID != existing {
		t.Fatalf("BackendScopeID = %q, want %q", cfg.BackendScopeID, existing)
	}
	if _, err := os.Stat(configPath); err == nil {
		t.Fatalf("expected no system config write when backend scope is explicit")
	} else if !os.IsNotExist(err) {
		t.Fatalf("Stat: %v", err)
	}
}

func TestEnsureServiceBackendScope_ReplayModeSkipsPersistence(t *testing.T) {
	t.Parallel()

	cfg := &FactoryServiceConfig{
		ReplayPath: filepath.Join(t.TempDir(), "replay.json"),
		Logger:     zap.NewNop(),
	}
	if err := ensureServiceBackendScope(cfg, cfg.Logger); err != nil {
		t.Fatalf("ensureServiceBackendScope: %v", err)
	}
	if cfg.BackendScopeID != "" {
		t.Fatalf("BackendScopeID = %q, want empty in replay mode", cfg.BackendScopeID)
	}
}

func TestBuildFactoryService_ResolvesBackendScopeBeforeSessionIdentity(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	homeDir := t.TempDir()

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:                                     dir,
		RuntimeMode:                             interfaces.RuntimeModeService,
		Port:                                    1,
		Logger:                                  zap.NewNop(),
		SystemConfigHomeDir:                     homeDir,
		SkipBuiltInRunnerPrerequisiteValidation: true,
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.cfg.BackendScopeID == "" {
		t.Fatal("expected backend scope to be resolved before service build completes")
	}
	if got := factorySessionBackendScopeID(svc, nil); got != svc.cfg.BackendScopeID {
		t.Fatalf("factorySessionBackendScopeID = %q, want %q", got, svc.cfg.BackendScopeID)
	}
}
