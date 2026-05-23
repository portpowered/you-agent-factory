package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/apisurface"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/petri"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestBuildFactoryService_LoadsFromFactoryJSON(t *testing.T) {
	dir := t.TempDir()
	writeFactoryJSON(t, dir, minimalFactoryConfig())
	writeWorkstationAgentsMD(t, dir, "process")

	// Create the inputs/ directory that the file watcher expects.
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	// Verify the service was constructed with the correct net topology.
	if svc.net == nil {
		t.Fatal("expected non-nil net")
	}
	if _, ok := svc.net.WorkTypes["task"]; !ok {
		t.Error("expected 'task' work type in net topology")
	}

	// Verify factory is accessible internally.
	if svc.factory == nil {
		t.Fatal("expected non-nil factory")
	}

}

func TestBuildFactoryService_ResolvesCurrentFactoryFromNamedLayoutPointer(t *testing.T) {
	rootDir := t.TempDir()

	alphaPayload := serviceNamedFactoryPayload(t, "alpha")
	if _, err := config.PersistNamedFactory(rootDir, "alpha", alphaPayload); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer: %v", err)
	}

	ctx := context.Background()
	svc, err := BuildFactoryService(ctx, &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	wantDir := filepath.Join(rootDir, "alpha")
	if svc.cfg.Dir != wantDir {
		t.Fatalf("service dir = %q, want %q", svc.cfg.Dir, wantDir)
	}
	if svc.runtimeCfg == nil {
		t.Fatal("expected runtime config")
	}
	if svc.runtimeCfg.FactoryDir() != wantDir {
		t.Fatalf("runtime config dir = %q, want %q", svc.runtimeCfg.FactoryDir(), wantDir)
	}
	if svc.runtimeCfg.FactoryConfig().Project != "alpha" {
		t.Fatalf("project = %q, want alpha", svc.runtimeCfg.FactoryConfig().Project)
	}
}

func TestFactoryService_ActivateNamedFactory_SwapsPersistedFactoryAndUpdatesCurrentPointer(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "beta", serviceNamedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err != nil {
		t.Fatalf("ActivateNamedFactory(beta): %v", err)
	}

	wantDir := filepath.Join(rootDir, "beta")
	if svc.cfg.Dir != wantDir {
		t.Fatalf("service dir = %q, want %q", svc.cfg.Dir, wantDir)
	}
	if svc.runtimeCfg == nil {
		t.Fatal("expected runtime config after activation")
	}
	if got := svc.runtimeCfg.FactoryConfig().Project; got != "beta" {
		t.Fatalf("active project = %q, want beta", got)
	}
	if got, err := config.ReadCurrentFactoryPointer(rootDir); err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	} else if got != "beta" {
		t.Fatalf("current factory pointer = %q, want beta", got)
	}
	if got, err := config.ResolveCurrentFactoryDir(rootDir); err != nil {
		t.Fatalf("ResolveCurrentFactoryDir: %v", err)
	} else if got != wantDir {
		t.Fatalf("resolved current dir = %q, want %q", got, wantDir)
	}
}

func TestFactoryService_ActivateNamedFactory_CanActivateSecondPersistedFactory(t *testing.T) {
	rootDir := t.TempDir()

	for _, name := range []string{"alpha", "beta", "gamma"} {
		if _, err := config.PersistNamedFactory(rootDir, name, serviceNamedFactoryPayload(t, name)); err != nil {
			t.Fatalf("PersistNamedFactory(%s): %v", name, err)
		}
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err != nil {
		t.Fatalf("ActivateNamedFactory(beta): %v", err)
	}
	if err := svc.ActivateNamedFactory(context.Background(), "gamma"); err != nil {
		t.Fatalf("ActivateNamedFactory(gamma): %v", err)
	}

	if got := svc.runtimeCfg.FactoryConfig().Project; got != "gamma" {
		t.Fatalf("active project after second activation = %q, want gamma", got)
	}
	if got, err := config.ReadCurrentFactoryPointer(rootDir); err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	} else if got != "gamma" {
		t.Fatalf("current factory pointer = %q, want gamma", got)
	}
}

func TestFactoryService_ActivateNamedFactory_RejectsNonIdleRuntime(t *testing.T) {
	svc := &FactoryService{
		factory: &aggregateSnapshotFactory{
			engineState: &interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net]{
				RuntimeStatus: interfaces.RuntimeStatusActive,
			},
		},
		logger: zap.NewNop(),
	}

	err := svc.ActivateNamedFactory(context.Background(), "beta")
	if err == nil {
		t.Fatal("expected non-idle activation to fail")
	}
	if !errors.Is(err, ErrFactoryActivationRequiresIdle) {
		t.Fatalf("expected ErrFactoryActivationRequiresIdle, got %v", err)
	}
}

func TestFactoryService_ActivateNamedFactory_RollsBackCurrentPointerWhenReplacementBuildFails(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "beta", serviceNamedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	betaFactoryPath := filepath.Join(rootDir, "beta", interfaces.FactoryConfigFile)
	if err := os.WriteFile(betaFactoryPath, []byte(`{"id":"beta","workTypes":[`), 0o644); err != nil {
		t.Fatalf("corrupt beta factory.json: %v", err)
	}

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err == nil {
		t.Fatal("expected replacement build failure")
	}

	wantCurrentDir := filepath.Join(rootDir, "alpha")
	if svc.cfg.Dir != wantCurrentDir {
		t.Fatalf("service dir after failed activation = %q, want %q", svc.cfg.Dir, wantCurrentDir)
	}
	if got := svc.runtimeCfg.FactoryConfig().Project; got != "alpha" {
		t.Fatalf("active project after failed activation = %q, want alpha", got)
	}
	if got, err := config.ReadCurrentFactoryPointer(rootDir); err != nil {
		t.Fatalf("ReadCurrentFactoryPointer: %v", err)
	} else if got != "alpha" {
		t.Fatalf("current factory pointer after failed activation = %q, want alpha", got)
	}
	if got, err := config.ResolveCurrentFactoryDir(rootDir); err != nil {
		t.Fatalf("ResolveCurrentFactoryDir: %v", err)
	} else if got != wantCurrentDir {
		t.Fatalf("resolved current dir after failed activation = %q, want %q", got, wantCurrentDir)
	}
}

func TestFactoryService_CreateNamedFactory_ActivatesPersistedFactoryFromDefaultRuntime(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, serviceNamedFactoryPayload(t, "root-runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", factoryPath, err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	created, err := svc.CreateNamedFactory(context.Background(), serviceNamedFactoryContract(t, "beta"))
	if err != nil {
		t.Fatalf("CreateNamedFactory(beta): %v", err)
	}
	if created.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("created factory name = %q, want beta", created.Name)
	}
	assertCurrentFactoryPointer(t, rootDir, "beta", "after create from default runtime")
	assertServiceCurrentFactory(t, svc, "beta", "after create from default runtime")
	if svc.runtimeCfg == nil || svc.runtimeCfg.FactoryDir() != filepath.Join(rootDir, "beta") {
		t.Fatalf("service runtime dir after create = %q, want %q", svc.runtimeCfg.FactoryDir(), filepath.Join(rootDir, "beta"))
	}
}

// pkgmaintcheck:ignore-cyclomatic-complexity this named-factory portability test keeps bundled-file materialization assertions together on the service seam.
func TestFactoryService_CreateNamedFactory_MaterializesSupportedPortableBundledFiles(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, serviceNamedFactoryPayload(t, "root-runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", factoryPath, err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	created, err := svc.CreateNamedFactory(context.Background(), serviceNamedFactoryContractWithBundledFiles(t, "beta"))
	if err != nil {
		t.Fatalf("CreateNamedFactory(beta): %v", err)
	}
	if created.Name != factoryapi.FactoryName("beta") {
		t.Fatalf("created factory name = %q, want beta", created.Name)
	}
	if created.SupportingFiles == nil || created.SupportingFiles.BundledFiles == nil {
		t.Fatalf("created factory supportingFiles = %#v, want bundled files", created.SupportingFiles)
	}
	if len(*created.SupportingFiles.BundledFiles) != 3 {
		t.Fatalf("created factory bundled files = %#v, want 3 entries", created.SupportingFiles.BundledFiles)
	}
	bundledFiles := *created.SupportingFiles.BundledFiles
	assertServiceBundledFactoryEntry(t, bundledFiles[0], factoryapi.BundledFileTypeROOTHELPER, "Makefile", "test:\n\tgo test ./...\n")
	assertServiceBundledFactoryEntryWithoutInline(t, bundledFiles[1], factoryapi.BundledFileTypeDOC, "factory/docs/README.md")
	assertServiceBundledFactoryEntryWithoutInline(t, bundledFiles[2], factoryapi.BundledFileTypeSCRIPT, "factory/scripts/execute-story.ps1")

	importedDir := filepath.Join(rootDir, "beta")
	assertPortableServiceBundledFile(t, filepath.Join(importedDir, "Makefile"), "test:\n\tgo test ./...\n")
	assertPortableServiceBundledFile(t, filepath.Join(importedDir, "docs", "README.md"), "# Portable factory\n")
	assertPortableServiceBundledFile(t, filepath.Join(importedDir, "scripts", "execute-story.ps1"), servicePortableBundledScriptBody)

	factoryJSON, err := os.ReadFile(filepath.Join(importedDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("ReadFile(factory.json): %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(factoryJSON, &payload); err != nil {
		t.Fatalf("Unmarshal(factory.json): %v", err)
	}
	supportingFiles, ok := payload["supportingFiles"].(map[string]any)
	if !ok {
		t.Fatalf("expected supportingFiles object, got %#v", payload["supportingFiles"])
	}
	persistedBundledFiles, ok := supportingFiles["bundledFiles"].([]any)
	if !ok || len(persistedBundledFiles) != 3 {
		t.Fatalf("expected three bundled files, got %#v", supportingFiles["bundledFiles"])
	}
	for _, entry := range persistedBundledFiles {
		bundledFile, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("expected bundled file object, got %#v", entry)
		}
		content, ok := bundledFile["content"].(map[string]any)
		if !ok {
			t.Fatalf("expected bundled file content object, got %#v", bundledFile["content"])
		}
		targetPath, _ := bundledFile["targetPath"].(string)
		switch targetPath {
		case "Makefile":
			if got := content["inline"]; got != "test:\n\tgo test ./...\n" {
				t.Fatalf("expected persisted root helper inline content to stay inlined, got %#v", content)
			}
			if got := content["encoding"]; got != "utf-8" {
				t.Fatalf("expected persisted root helper encoding to stay canonical, got %#v", content)
			}
		case "factory/docs/README.md", "factory/scripts/execute-story.ps1":
			if _, ok := content["inline"]; ok {
				t.Fatalf("expected persisted bundled file inline content to be omitted, got %#v", content)
			}
			if got := content["encoding"]; got != "utf-8" {
				t.Fatalf("expected persisted bundled file encoding to stay canonical, got %#v", content)
			}
		default:
			t.Fatalf("unexpected persisted bundled file targetPath = %#v", targetPath)
		}
	}
}

func TestFactoryService_BuildFactoryService_LogsPortableBundledFileReplacements(t *testing.T) {
	projectDir := t.TempDir()
	sourceDir := filepath.Join(projectDir, "factory")
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(factory): %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "Makefile"), []byte("test:\n\tgo test ./...\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(Makefile): %v", err)
	}
	writeFactoryJSON(t, sourceDir, map[string]any{
		"name": "portable-runtime",
		"supportingFiles": map[string]any{
			"bundledFiles": []map[string]any{
				{
					"type":       "SCRIPT",
					"targetPath": "factory/scripts/execute-story.ps1",
					"content": map[string]any{
						"encoding": "utf-8",
						"inline":   servicePortableBundledScriptBody,
					},
				},
			},
		},
		"workTypes": []map[string]any{{
			"name": "task",
			"states": []map[string]string{
				{"name": "init", "type": "INITIAL"},
				{"name": "complete", "type": "TERMINAL"},
			},
		}},
		"workers": []map[string]any{{
			"name":    "worker-a",
			"type":    "SCRIPT_WORKER",
			"command": "powershell",
			"args":    []string{"-File", "scripts/execute-story.ps1"},
		}},
		"workstations": []map[string]any{{
			"name":    "process",
			"worker":  "worker-a",
			"inputs":  []map[string]string{{"workType": "task", "state": "init"}},
			"outputs": []map[string]string{{"workType": "task", "state": "complete"}},
		}},
	})
	writeWorkstationAgentsMD(t, sourceDir, "process")
	if err := os.MkdirAll(filepath.Join(sourceDir, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll(factory/scripts): %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "scripts", "execute-story.ps1"), []byte("Write-Output 'stale script'\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(portable script): %v", err)
	}

	logCore, observedLogs := observer.New(zap.WarnLevel)
	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               sourceDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.New(logCore),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if svc.logSink != nil {
		defer func() {
			if err := svc.logSink.Close(); err != nil {
				t.Fatalf("Close(runtime log sink): %v", err)
			}
		}()
	}

	if svc.runtimeCfg == nil {
		t.Fatal("expected runtime config after portable load")
	}
	warnings := observedLogs.FilterMessage("runtime config load replaced portable bundled files").All()
	if len(warnings) != 1 {
		t.Fatalf("replacement warning count = %d, want 1", len(warnings))
	}
	fields := warnings[0].ContextMap()
	targetPaths, ok := fields["target_paths"].([]any)
	if !ok {
		t.Fatalf("replacement warning target_paths = %#v, want []any", fields["target_paths"])
	}
	if len(targetPaths) != 1 || targetPaths[0] != "factory/scripts/execute-story.ps1" {
		t.Fatalf("replacement warning target_paths = %#v, want [factory/scripts/execute-story.ps1]", targetPaths)
	}
	data, err := os.ReadFile(filepath.Join(sourceDir, "scripts", "execute-story.ps1"))
	if err != nil {
		t.Fatalf("ReadFile(portable script): %v", err)
	}
	if got := string(data); got != servicePortableBundledScriptBody {
		t.Fatalf("materialized script after replacement = %q, want %q", got, servicePortableBundledScriptBody)
	}
}

func TestFactoryService_CreateNamedFactory_RejectsReservedCurrentFactoryName(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, serviceNamedFactoryPayload(t, "root-runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", factoryPath, err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	_, err = svc.CreateNamedFactory(context.Background(), serviceNamedFactoryContract(t, string(apisurface.DefaultCurrentFactoryName)))
	if !errors.Is(err, apisurface.ErrInvalidNamedFactoryName) {
		t.Fatalf("CreateNamedFactory(%q) error = %v, want %v", apisurface.DefaultCurrentFactoryName, err, apisurface.ErrInvalidNamedFactoryName)
	}
	assertCurrentFactoryPointerMissing(t, rootDir, "after reserved-name rejection")
}

func TestFactoryService_CreateNamedFactory_RejectsDuplicatePersistedName(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, serviceNamedFactoryPayload(t, "root-runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", factoryPath, err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if _, err := svc.CreateNamedFactory(context.Background(), serviceNamedFactoryContract(t, "beta")); err != nil {
		t.Fatalf("CreateNamedFactory(beta): %v", err)
	}
	_, err = svc.CreateNamedFactory(context.Background(), serviceNamedFactoryContract(t, "beta"))
	if !errors.Is(err, config.ErrNamedFactoryAlreadyExists) {
		t.Fatalf("duplicate CreateNamedFactory(beta) error = %v, want %v", err, config.ErrNamedFactoryAlreadyExists)
	}
	assertCurrentFactoryPointer(t, rootDir, "beta", "after duplicate create rejection")
}

func TestFactoryService_ActivateNamedFactory_FromDefaultRuntimeLeavesRootReadableWhenReplacementBuildFails(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, serviceNamedFactoryPayload(t, "root-runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", factoryPath, err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "beta", serviceNamedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	corruptNamedFactoryConfig(t, rootDir, "beta")

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	if err := svc.ActivateNamedFactory(context.Background(), "beta"); err == nil {
		t.Fatal("expected replacement build failure")
	}

	assertCurrentFactoryPointerMissing(t, rootDir, "after failed activation from default runtime")
	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory after failed activation from default runtime: %v", err)
	}
	if current.Name != apisurface.DefaultCurrentFactoryName {
		t.Fatalf("current factory name after failed activation = %q, want %q", current.Name, apisurface.DefaultCurrentFactoryName)
	}
	if current.Id == nil || *current.Id != "root-runtime" {
		t.Fatalf("current factory id after failed activation = %#v, want root-runtime", current.Id)
	}
	if svc.runtimeCfg == nil || svc.runtimeCfg.FactoryDir() != rootDir {
		t.Fatalf("service runtime dir after failed activation = %q, want %q", svc.runtimeCfg.FactoryDir(), rootDir)
	}
}

func TestFactoryService_GetCurrentFactory_ReadsDurablePointerAndCanonicalPayload(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if _, err := config.PersistNamedFactory(rootDir, "beta", serviceNamedFactoryPayload(t, "beta")); err != nil {
		t.Fatalf("PersistNamedFactory(beta): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "beta"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(beta): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}
	if current.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("current factory name = %q, want alpha", current.Name)
	}
	if current.Id == nil || *current.Id != "alpha" {
		t.Fatalf("current factory id = %#v, want alpha", current.Id)
	}
	if svc.runtimeCfg == nil || svc.runtimeCfg.FactoryConfig().Project != "beta" {
		t.Fatalf("service runtime project = %q, want unchanged beta runtime", svc.runtimeCfg.FactoryConfig().Project)
	}
}

func TestFactoryService_GetCurrentFactory_IncludesVersionMetadata(t *testing.T) {
	rootDir := t.TempDir()
	versionTime := time.Date(2026, 5, 23, 12, 30, 0, 0, time.UTC)

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithVersion(t, "alpha", factoryapi.HybridLogicalTimestamp{
		Logical:  23,
		Physical: versionTime,
	})); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}
	if current.Name != factoryapi.FactoryName("alpha") {
		t.Fatalf("current factory name = %q, want alpha", current.Name)
	}
	if current.Version == nil || current.Version.Logical != 23 || !current.Version.Physical.Equal(versionTime) {
		t.Fatalf("current factory version = %#v, want logical=23 physical=%s", current.Version, versionTime)
	}
}

func TestFactoryService_SaveCurrentFactory_ReplacesCurrentDefinition(t *testing.T) {
	rootDir := t.TempDir()
	initialVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  41,
		Physical: time.Date(2026, 5, 23, 13, 0, 0, 0, time.UTC),
	}

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithVersion(t, "alpha", initialVersion)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "alpha", "story")
	saved, err := svc.SaveCurrentFactory(context.Background(), replacement)
	if err != nil {
		t.Fatalf("SaveCurrentFactory: %v", err)
	}
	if saved.WorkTypes == nil || len(*saved.WorkTypes) != 1 || (*saved.WorkTypes)[0].Name != "story" {
		t.Fatalf("saved work types = %#v, want story", saved.WorkTypes)
	}
	if saved.Version == nil || saved.Version.Logical != initialVersion.Logical+1 || !saved.Version.Physical.After(initialVersion.Physical) {
		t.Fatalf("saved version = %#v, want logical=%d physical after %s", saved.Version, initialVersion.Logical+1, initialVersion.Physical)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory after save: %v", err)
	}
	if current.WorkTypes == nil || (*current.WorkTypes)[0].Name != "story" {
		t.Fatalf("current work types after save = %#v, want story", current.WorkTypes)
	}
	if current.Version == nil || current.Version.Logical != saved.Version.Logical || !current.Version.Physical.Equal(saved.Version.Physical) {
		t.Fatalf("current version after save = %#v, want %#v", current.Version, saved.Version)
	}
	loaded, err := config.LoadRuntimeConfig(filepath.Join(rootDir, "alpha"), nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(alpha) after save: %v", err)
	}
	if loaded.FactoryConfig().Version == nil || loaded.FactoryConfig().Version.Logical != saved.Version.Logical || !loaded.FactoryConfig().Version.Physical.Equal(saved.Version.Physical) {
		t.Fatalf("persisted version after save = %#v, want %#v", loaded.FactoryConfig().Version, saved.Version)
	}
	restarted, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService(restarted): %v", err)
	}
	restartedCurrent, err := restarted.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory(restarted): %v", err)
	}
	if restartedCurrent.Version == nil || restartedCurrent.Version.Logical != saved.Version.Logical || !restartedCurrent.Version.Physical.Equal(saved.Version.Physical) {
		t.Fatalf("restarted version = %#v, want %#v", restartedCurrent.Version, saved.Version)
	}
	assertCurrentFactoryPointer(t, rootDir, "alpha", "after current factory save")
}

func TestFactoryService_SaveCurrentFactory_RejectsStaleBaseVersion(t *testing.T) {
	rootDir := t.TempDir()
	initialVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  7,
		Physical: time.Date(2026, 5, 23, 14, 0, 0, 0, time.UTC),
	}
	newerVersion := factoryapi.HybridLogicalTimestamp{
		Logical:  8,
		Physical: initialVersion.Physical.Add(time.Second),
	}

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithVersion(t, "alpha", initialVersion)); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}

	if current.Version == nil {
		t.Fatal("expected current factory version metadata")
	}
	if _, err := config.ReplaceNamedFactory(rootDir, "alpha", serviceNamedFactoryPayloadWithVersion(t, "alpha", newerVersion)); err != nil {
		t.Fatalf("ReplaceNamedFactory(alpha newer version): %v", err)
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "alpha", "story")
	replacement.Version = current.Version
	_, err = svc.SaveCurrentFactory(context.Background(), replacement)
	if !errors.Is(err, apisurface.ErrFactoryVersionStale) {
		t.Fatalf("SaveCurrentFactory error = %v, want stale version", err)
	}

	currentAfterStaleSave, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory after stale save: %v", err)
	}
	if currentAfterStaleSave.WorkTypes == nil || (*currentAfterStaleSave.WorkTypes)[0].Name != "task" {
		t.Fatalf("current work types after stale save = %#v, want unchanged task", currentAfterStaleSave.WorkTypes)
	}
	if currentAfterStaleSave.Version == nil || currentAfterStaleSave.Version.Logical != newerVersion.Logical || !currentAfterStaleSave.Version.Physical.Equal(newerVersion.Physical) {
		t.Fatalf("current version after stale save = %#v, want %#v", currentAfterStaleSave.Version, newerVersion)
	}
}

func TestFactoryService_SaveCurrentFactory_RejectsDuplicateAndDanglingTopology(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	replacement := serviceNamedFactoryContractWithWorkType(t, "alpha", "story")
	if replacement.Workers == nil || replacement.Workstations == nil {
		t.Fatal("expected fixture workers and workstations")
	}
	*replacement.Workers = append(*replacement.Workers, (*replacement.Workers)[0])
	(*replacement.Workstations)[0].Worker = "missing-worker"
	(*replacement.Workstations)[0].Outputs = &[]factoryapi.WorkstationIO{{WorkType: "story", State: "missing-state"}}

	_, err = svc.SaveCurrentFactory(context.Background(), replacement)
	var topologyErr *apisurface.TopologyValidationError
	if !errors.As(err, &topologyErr) {
		t.Fatalf("SaveCurrentFactory error = %v, want topology validation error", err)
	}
	if len(topologyErr.Targets) < 3 {
		t.Fatalf("topology targets = %#v, want duplicate worker, missing worker, and dangling output targets", topologyErr.Targets)
	}
	if !hasServiceErrorTarget(topologyErr.Targets, "node", "worker-a") {
		t.Fatalf("topology targets = %#v, want duplicate worker node target", topologyErr.Targets)
	}
	if !hasServiceErrorTarget(topologyErr.Targets, "field", "process") {
		t.Fatalf("topology targets = %#v, want missing workstation worker field target", topologyErr.Targets)
	}
	if !hasServiceErrorTarget(topologyErr.Targets, "edge", "process->story:missing-state") {
		t.Fatalf("topology targets = %#v, want dangling output edge target", topologyErr.Targets)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory after rejected save: %v", err)
	}
	if current.WorkTypes == nil || (*current.WorkTypes)[0].Name != "task" {
		t.Fatalf("current work types after rejected topology = %#v, want unchanged task", current.WorkTypes)
	}
}

func hasServiceErrorTarget(targets []factoryapi.ErrorTarget, kind, id string) bool {
	for _, target := range targets {
		if target.Kind == kind && target.Id != nil && *target.Id == id {
			return true
		}
	}
	return false
}

func TestFactoryService_GetCurrentFactory_CollectsSupportedPortableBundledFilesFromDisk(t *testing.T) {
	rootDir := t.TempDir()

	if _, err := config.PersistNamedFactory(rootDir, "alpha", serviceNamedFactoryPayload(t, "alpha")); err != nil {
		t.Fatalf("PersistNamedFactory(alpha): %v", err)
	}
	if err := config.WriteCurrentFactoryPointer(rootDir, "alpha"); err != nil {
		t.Fatalf("WriteCurrentFactoryPointer(alpha): %v", err)
	}

	alphaDir := filepath.Join(rootDir, "alpha")
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "scripts", "execute-story.ps1"), servicePortableBundledScriptBody)
	writePortableServiceBundledFile(t, filepath.Join(alphaDir, "docs", "README.md"), "# Portable factory\n")
	writePortableServiceBundledFile(t, filepath.Join(rootDir, "Makefile"), "test:\n\tgo test ./...\n")
	writePortableServiceBundledFile(t, filepath.Join(rootDir, "README.md"), "outside allowlist\n")

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}
	if current.SupportingFiles == nil {
		t.Fatal("expected current factory to include supportingFiles")
	}
	if current.SupportingFiles.BundledFiles == nil || len(*current.SupportingFiles.BundledFiles) != 3 {
		t.Fatalf("expected 3 bundled files, got %#v", current.SupportingFiles.BundledFiles)
	}
	bundledFiles := *current.SupportingFiles.BundledFiles
	assertServiceBundledFactoryEntry(t, bundledFiles[0], factoryapi.BundledFileTypeROOTHELPER, "Makefile", "test:\n\tgo test ./...\n")
	assertServiceBundledFactoryEntry(t, bundledFiles[1], factoryapi.BundledFileTypeDOC, "factory/docs/README.md", "# Portable factory\n")
	assertServiceBundledFactoryEntry(t, bundledFiles[2], factoryapi.BundledFileTypeSCRIPT, "factory/scripts/execute-story.ps1", servicePortableBundledScriptBody)
}

func TestFactoryService_GetCurrentFactory_FallsBackToRootRuntimeWhenPointerMissing(t *testing.T) {
	rootDir := t.TempDir()
	factoryPath := filepath.Join(rootDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, serviceNamedFactoryPayload(t, "root-runtime"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", factoryPath, err)
	}

	svc, err := BuildFactoryService(context.Background(), &FactoryServiceConfig{
		Dir:               rootDir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildFactoryService: %v", err)
	}

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory: %v", err)
	}
	if current.Name != apisurface.DefaultCurrentFactoryName {
		t.Fatalf("current factory name = %q, want %q", current.Name, apisurface.DefaultCurrentFactoryName)
	}
	if current.Id == nil || *current.Id != "root-runtime" {
		t.Fatalf("current factory id = %#v, want root-runtime", current.Id)
	}
	if svc.runtimeCfg == nil || svc.runtimeCfg.FactoryDir() != rootDir {
		t.Fatalf("service runtime dir = %q, want %q", svc.runtimeCfg.FactoryDir(), rootDir)
	}
}

func TestFactoryService_GetCurrentFactory_ReturnsNotFoundWhenPointerMissingWithoutRuntimeFallback(t *testing.T) {
	rootDir := t.TempDir()
	svc := &FactoryService{
		cfg: &FactoryServiceConfig{
			Dir: rootDir,
		},
	}

	_, err := svc.GetCurrentFactory(context.Background())
	if !errors.Is(err, ErrCurrentFactoryNotFound) {
		t.Fatalf("GetCurrentFactory missing pointer error = %v, want %v", err, ErrCurrentFactoryNotFound)
	}
}

func TestFactoryService_GetCurrentFactory_WrapsMissingPersistedFactoryDir(t *testing.T) {
	rootDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(rootDir, interfaces.CurrentFactoryPointerFile),
		[]byte("missing\n"),
		0o644,
	); err != nil {
		t.Fatalf("WriteFile(current-factory.txt): %v", err)
	}

	svc := &FactoryService{
		cfg: &FactoryServiceConfig{
			Dir: rootDir,
		},
		factoryRootDir: rootDir,
	}

	_, err := svc.GetCurrentFactory(context.Background())
	if err == nil {
		t.Fatal("expected missing persisted factory dir error")
	}
	if !strings.Contains(err.Error(), `resolve current factory "missing"`) {
		t.Fatalf("GetCurrentFactory resolve error = %v, want wrapped missing-factory context", err)
	}
}

func TestFactoryService_WaitToComplete_ReturnsClosedChannelWithoutRuntime(t *testing.T) {
	svc := &FactoryService{}

	select {
	case <-svc.WaitToComplete():
	default:
		t.Fatal("expected WaitToComplete without runtime to return a closed channel")
	}
}

func TestFactoryService_WaitToComplete_DelegatesToActiveRuntime(t *testing.T) {
	waitCh := make(chan struct{})
	svc := &FactoryService{
		factory: &aggregateSnapshotFactory{
			waitToComplete: waitCh,
		},
	}

	if got := svc.WaitToComplete(); got != waitCh {
		t.Fatalf("WaitToComplete channel = %p, want %p", got, waitCh)
	}
	close(waitCh)
}

func TestFactoryService_Pause_RequiresActiveRuntimeAndWrapsPauseErrors(t *testing.T) {
	svc := &FactoryService{}
	if err := svc.Pause(context.Background()); err == nil || !strings.Contains(err.Error(), "runtime is not available") {
		t.Fatalf("Pause without runtime error = %v, want runtime unavailable", err)
	}

	svc.factory = &aggregateSnapshotFactory{pauseErr: fmt.Errorf("pause failed")}
	if err := svc.Pause(context.Background()); err == nil || !strings.Contains(err.Error(), "pause factory: pause failed") {
		t.Fatalf("Pause wrapped error = %v, want wrapped pause failure", err)
	}

	svc.factory = &aggregateSnapshotFactory{}
	if err := svc.Pause(context.Background()); err != nil {
		t.Fatalf("Pause success error = %v", err)
	}
}

func TestFactoryService_CurrentRuntimeBundleAndDirComparisonHelpers(t *testing.T) {
	if bundle := (*FactoryService)(nil).currentRuntimeBundle(); bundle != nil {
		t.Fatalf("nil service currentRuntimeBundle = %#v, want nil", bundle)
	}

	svc := &FactoryService{}
	if bundle := svc.currentRuntimeBundle(); bundle != nil {
		t.Fatalf("empty service currentRuntimeBundle = %#v, want nil", bundle)
	}

	svc.cfg = &FactoryServiceConfig{Dir: "C:/factory"}
	svc.factory = &aggregateSnapshotFactory{}
	svc.runtimeCfg = &config.LoadedFactoryConfig{}
	bundle := svc.currentRuntimeBundle()
	if bundle == nil {
		t.Fatal("expected populated currentRuntimeBundle")
	}
	if bundle.dir != svc.cfg.Dir || bundle.factory != svc.factory || bundle.runtimeCfg != svc.runtimeCfg {
		t.Fatalf("currentRuntimeBundle = %#v, want service fields copied through", bundle)
	}

	if sameFactoryDir("", svc.cfg.Dir) {
		t.Fatal("sameFactoryDir should reject blank paths")
	}
	if !sameFactoryDir("C:/factory/./named", "C:/factory/named") {
		t.Fatal("sameFactoryDir should normalize equivalent paths")
	}
}

func TestLiveRuntimeHandle_CompletionHelpers(t *testing.T) {
	if !(*liveRuntimeHandle)(nil).completed() {
		t.Fatal("nil liveRuntimeHandle should report completed")
	}
	if err := (*liveRuntimeHandle)(nil).wait(); err != nil {
		t.Fatalf("nil liveRuntimeHandle wait error = %v, want nil", err)
	}

	handle := &liveRuntimeHandle{
		runDone: make(chan struct{}),
	}
	if handle.completed() {
		t.Fatal("open runDone should report incomplete")
	}
	handle.setRunResult(fmt.Errorf("run failed"))
	if !handle.completed() {
		t.Fatal("closed runDone should report completed")
	}
	if err := handle.wait(); err == nil || err.Error() != "run failed" {
		t.Fatalf("wait error = %v, want run failed", err)
	}
}
