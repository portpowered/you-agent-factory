// backendsizecheck:ignore-file shared service test fixtures and helpers remain together until dedicated service test seams split.
// pkgmaintcheck:ignore-file-lines shared service test fixtures and helpers remain together until dedicated service test seams split.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/internal/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/internal/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/factory"
	factory_context "github.com/portpowered/infinite-you/pkg/factory/context"
	interfaces "github.com/portpowered/infinite-you/pkg/factory/contracts"
	factorysessions "github.com/portpowered/infinite-you/pkg/factory/sessions"
	sessioninvocation "github.com/portpowered/infinite-you/pkg/factory/sessions/invocation"
	"github.com/portpowered/infinite-you/pkg/factory/state"
	factorytoken "github.com/portpowered/infinite-you/pkg/factory/token"
	modelhost "github.com/portpowered/infinite-you/pkg/models/host"
	localmodels "github.com/portpowered/infinite-you/pkg/models/local"
	modelsservice "github.com/portpowered/infinite-you/pkg/models/service"
	"github.com/portpowered/infinite-you/pkg/orchestrators/petri"
	"github.com/portpowered/infinite-you/pkg/platform/logging"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	apisurface "github.com/portpowered/infinite-you/pkg/transports/mapping"
	workerdiagnosticsmapping "github.com/portpowered/infinite-you/pkg/transports/mapping/workerdiagnostics"
	"github.com/portpowered/infinite-you/pkg/work"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
	workerapplication "github.com/portpowered/infinite-you/pkg/workers/application"
	workerconfig "github.com/portpowered/infinite-you/pkg/workers/config"
	workerexecution "github.com/portpowered/infinite-you/pkg/workers/execution"
	workeragentrun "github.com/portpowered/infinite-you/pkg/workers/executor/agentrun"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

func buildHostedWorkersConfigForServiceTest(
	cfg *FactoryServiceConfig,
	logger *zap.Logger,
	clock factory.Clock,
) hostedworkers.Config {
	if cfg != nil && cfg.WorkerApplication.Valid() {
		return cfg.WorkerApplication.Hosted
	}
	hostedClock, _ := clock.(clockwork.Clock)
	components, _ := workerapplication.New(logger, workerapplication.Edges{
		HostedClock: hostedClock,
	})
	return components.Hosted
}

func serviceTestConfigWithWorkerApplication(t *testing.T, cfg *FactoryServiceConfig) *FactoryServiceConfig {
	t.Helper()
	configured, err := ConfigWithWorkerApplication(cfg)
	if err != nil {
		t.Fatalf("construct worker application: %v", err)
	}
	return configured
}

func serviceTestConfigWithWorkerEdges(t *testing.T, cfg *FactoryServiceConfig, edges workerapplication.Edges) *FactoryServiceConfig {
	t.Helper()
	components, err := workerapplication.New(cfg.Logger, edges)
	if err != nil {
		t.Fatalf("construct worker application: %v", err)
	}
	cfg.WorkerApplication = components
	return cfg
}

// loadWorkersFromConfig preserves the package-test helper shape while
// production construction consumes the composed worker application directly.
func loadWorkersFromConfig(
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	factoryRunnerID string,
	runtimeCfg interfaces.RuntimeConfigLookup,
	workflowContext *factory_context.FactoryContext,
	logger logging.Logger,
	skipBuiltInRunnerPrerequisiteValidation bool,
	invocationSkipPermissionsOverride *bool,
	providerOverride workerprovider.Provider,
	inferenceProgressPublisher workerprovider.InferenceProgressPublisher,
	providerCommandRunner workers.CommandRunner,
	commandRunner workers.CommandRunner,
	scriptRecorder workers.ScriptEventRecorder,
	inferenceRecorder workerprovider.InferenceEventRecorder,
	modelRecorder modelEventRecorder,
	agentRunRecorder workeragentrun.AgentRunEventRecorder,
	now func() time.Time,
	modelDomain localModelDomain,
	agyPTYAllocators ...agypty.PTYAllocator,
) ([]factory.FactoryOption, error) {
	var allocator agypty.PTYAllocator
	if len(agyPTYAllocators) > 0 {
		allocator = agyPTYAllocators[0]
	}
	components, err := workerapplication.New(zap.NewNop(), workerapplication.Edges{
		ProviderCommandRunner: providerCommandRunner,
		ScriptCommandRunner:   commandRunner,
		AgyPTYAllocator:       allocator,
	})
	if err != nil {
		return nil, err
	}
	return loadWorkersFromApplication(
		factoryDir, factoryCfg, factoryRunnerID, runtimeCfg, workflowContext, logger,
		skipBuiltInRunnerPrerequisiteValidation, invocationSkipPermissionsOverride,
		providerOverride, inferenceProgressPublisher, components, scriptRecorder,
		inferenceRecorder, modelRecorder, agentRunRecorder, now, modelDomain,
	)
}

func minimalFactoryConfig() map[string]any {
	return factoryfixtures.MinimalFactoryConfig()
}

func writeFactoryJSON(t *testing.T, dir string, cfg map[string]any) {
	t.Helper()
	factoryfixtures.WriteFactoryJSON(t, dir, cfg)
}

func bindServiceStartupRuntime(svc *FactoryService, bundle *factoryRuntimeBundle) {
	if svc == nil {
		return
	}
	if svc.sessions == nil {
		svc.sessions = factorysessions.NewRegistry()
	}
	handle := &liveRuntimeHandle{Bundle: bundle, RunDone: make(chan struct{})}
	target := FactorySessionTarget{
		Ref: FactorySessionTargetRef{Kind: FactorySessionTargetKindDefault},
	}
	if svc.cfg != nil {
		if folderPath := strings.TrimSpace(svc.cfg.Dir); folderPath != "" {
			target.FolderPath = folderPath
		}
	}
	if strings.TrimSpace(target.FolderPath) == "" && strings.TrimSpace(svc.factoryRootDir) != "" {
		target.FolderPath = svc.factoryRootDir
	}
	svc.registerLiveSession(defaultFactorySessionID, handle, target, true)
	svc.setRunState(context.Background(), defaultFactorySessionID, handle)
}

func newLocalModelResourceLimiter() *localModelResourceLimiter {
	return localmodels.NewResourceLimiter(localModelHooks())
}

func newManagedLocalModelManager(assetPuller modelAssetPuller, runtime localModelRuntime) *managedLocalModelManager {
	manager, err := localmodels.NewManagedRuntime(localmodels.ManagedRuntimeDependencies{
		AssetPuller: assetPuller, Runtime: runtime, Hooks: localModelHooks(),
	})
	if err != nil {
		return nil
	}
	return manager
}

func attachModelServiceForTest(t *testing.T, svc *FactoryService) {
	t.Helper()
	if svc == nil {
		t.Fatal("attach model service: factory service is required")
	}
	puller := svc.modelAssets
	if puller == nil {
		puller = localmodels.NewAssetPuller(t.TempDir())
		svc.modelAssets = puller
	}
	host := svc.modelHost()
	if host == nil {
		gateway := modelhost.NewLocalAssetGateway(puller)
		var err error
		host, err = modelhost.NewHost(modelhost.Dependencies{
			AssetPuller: gateway, CacheInspector: gateway,
			ProcessLauncher: modelhost.DefaultProcessLauncher(),
		})
		if err != nil {
			t.Fatalf("NewHost: %v", err)
		}
	}
	modelAPI, err := modelsservice.NewService(modelsservice.Dependencies{
		RuntimeConfig: svc.currentRuntimeConfig, ModelHost: host,
		ModelAssetPuller: puller, Logger: svc.logger,
		ModelPullMetrics:        modelPullMetricsRecorderForService(svc.modelPullMetricsRecorder()),
		ModelInvocationExecutor: svc.modelInvocationExecutor,
		FactoryRunnerID:         svc.factoryRunnerID(),
	})
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	svc.modelService = AdaptModelService(modelAPI)
}

type recordingDiagnosticsProvider struct{}

func (recordingDiagnosticsProvider) Infer(_ context.Context, _ workerexecution.ProviderInferenceRequest) (workerexecution.InferenceResponse, error) {
	return workerexecution.InferenceResponse{
		Content: "Done. COMPLETE",
		Diagnostics: &workerexecution.WorkDiagnostics{
			Provider: &workerexecution.ProviderDiagnostic{
				ResponseMetadata: map[string]string{"request_id": "provider-request-1"},
			},
		},
	}, nil
}

func assertServiceBundledFactoryEntry(t *testing.T, bundledFile factoryapi.BundledFile, wantType factoryapi.BundledFileType, wantPath, wantContent string) {
	t.Helper()
	if bundledFile.Type != wantType {
		t.Fatalf("bundled file type = %q, want %q", bundledFile.Type, wantType)
	}
	if bundledFile.TargetPath != wantPath {
		t.Fatalf("bundled file targetPath = %q, want %q", bundledFile.TargetPath, wantPath)
	}
	if bundledFile.Content.Encoding != factoryapi.BundledFileContentEncodingUtf8 {
		t.Fatalf("bundled file %q encoding = %q, want %q", wantPath, bundledFile.Content.Encoding, factoryapi.BundledFileContentEncodingUtf8)
	}
	if bundledFile.Content.Inline != wantContent {
		t.Fatalf("bundled file %q content = %q, want %q", wantPath, bundledFile.Content.Inline, wantContent)
	}
}

func assertServiceBundledFactoryEntryWithoutInline(t *testing.T, bundledFile factoryapi.BundledFile, wantType factoryapi.BundledFileType, wantPath string) {
	t.Helper()
	if bundledFile.Type != wantType {
		t.Fatalf("bundled file type = %q, want %q", bundledFile.Type, wantType)
	}
	if bundledFile.TargetPath != wantPath {
		t.Fatalf("bundled file targetPath = %q, want %q", bundledFile.TargetPath, wantPath)
	}
	if bundledFile.Content.Encoding != factoryapi.BundledFileContentEncodingUtf8 {
		t.Fatalf("bundled file %q encoding = %q, want %q", wantPath, bundledFile.Content.Encoding, factoryapi.BundledFileContentEncodingUtf8)
	}
	if bundledFile.Content.Inline != "" {
		t.Fatalf("bundled file %q content = %q, want omitted inline content", wantPath, bundledFile.Content.Inline)
	}
}

func serviceBundledFilesByTarget(t *testing.T, factory factoryapi.Factory) map[string]factoryapi.BundledFile {
	t.Helper()
	if factory.SupportingFiles == nil || factory.SupportingFiles.BundledFiles == nil {
		t.Fatalf("factory %q missing supportingFiles.bundledFiles", factory.Name)
	}
	byTarget := make(map[string]factoryapi.BundledFile, len(*factory.SupportingFiles.BundledFiles))
	for _, bundledFile := range *factory.SupportingFiles.BundledFiles {
		byTarget[bundledFile.TargetPath] = bundledFile
	}
	return byTarget
}

func writePortableServiceBundledFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func assertPortableServiceBundledFile(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("file %s = %q, want %q", path, string(got), want)
	}
}

func assertPortableServiceBundledFileMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	if runtime.GOOS == "windows" {
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%s): %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("file %s mode = %v, want %v", path, got, want)
	}
}

func assertCurrentFactoryPointer(t *testing.T, rootDir, want, contextLabel string) {
	t.Helper()

	got, err := config.ReadCurrentFactoryPointer(rootDir)
	if err != nil {
		t.Fatalf("ReadCurrentFactoryPointer %s: %v", contextLabel, err)
	}
	if got != want {
		t.Fatalf("current factory pointer %s = %q, want %q", contextLabel, got, want)
	}
}

func assertCurrentFactoryPointerMissing(t *testing.T, rootDir, contextLabel string) {
	t.Helper()

	if _, err := config.ReadCurrentFactoryPointer(rootDir); !os.IsNotExist(err) {
		t.Fatalf("ReadCurrentFactoryPointer %s err = %v, want %v", contextLabel, err, os.ErrNotExist)
	}
}

func assertServiceCurrentFactory(t *testing.T, svc *FactoryService, want, contextLabel string) {
	t.Helper()

	current, err := svc.GetCurrentFactory(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentFactory %s: %v", contextLabel, err)
	}
	if current.Name != factoryapi.FactoryName(want) {
		t.Fatalf("current factory %s = %q, want %q", contextLabel, current.Name, want)
	}
}

func assertFactoryName(t *testing.T, got factoryapi.FactoryName, want, label string) {
	t.Helper()
	if got != factoryapi.FactoryName(want) {
		t.Fatalf("%s = %q, want %q", label, got, want)
	}
}

func assertMatchingFactoryVersion(t *testing.T, got, want *factoryapi.HybridLogicalTimestamp, label string) {
	t.Helper()
	if got == nil || want == nil || got.Logical != want.Logical || !got.Physical.Equal(want.Physical) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
}

func assertFactoryVersionAdvanced(t *testing.T, got *factoryapi.HybridLogicalTimestamp, previous factoryapi.HybridLogicalTimestamp) {
	t.Helper()
	if got == nil || got.Logical != previous.Logical+1 || !got.Physical.After(previous.Physical) {
		t.Fatalf("saved version = %#v, want logical=%d physical after %s", got, previous.Logical+1, previous.Physical)
	}
}

func assertPersistedFactoryWorkType(t *testing.T, workTypes []interfaces.WorkTypeConfig, want, label string) {
	t.Helper()
	if len(workTypes) != 1 || workTypes[0].Name != want {
		t.Fatalf("%s = %#v, want %s", label, workTypes, want)
	}
}

func assertPersistedFactoryVersionMatchesAPI(t *testing.T, got *interfaces.FactoryVersion, want *factoryapi.HybridLogicalTimestamp, label string) {
	t.Helper()
	if got == nil || want == nil || got.Logical != want.Logical.Int64() || !got.Physical.Equal(want.Physical) {
		t.Fatalf("%s = %#v, want %#v", label, got, want)
	}
}

func corruptNamedFactoryConfig(t *testing.T, rootDir, name string) {
	t.Helper()

	factoryPath := filepath.Join(rootDir, name, interfaces.FactoryConfigFile)
	if err := os.WriteFile(factoryPath, []byte(`{"id":"`+name+`","workTypes":[`), 0o644); err != nil {
		t.Fatalf("corrupt %s factory.json: %v", name, err)
	}
}

func writeWorkerAgentsMD(t *testing.T, factoryDir, workerName string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	agentsMD := "---\ntype: MODEL_WORKER\nmodelProvider: codex\nmodel: claude-3-5-haiku-20241022\n---\nYou are a helpful assistant.\n"
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func writeWorkerAgentsMDWithContent(t *testing.T, factoryDir, workerName, content string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func writeScriptWorkerAgentsMD(t *testing.T, factoryDir, workerName string) {
	t.Helper()
	writeScriptWorkerAgentsMDWithCommand(t, factoryDir, workerName, "echo", []string{"ok"})
}

func writeScriptWorkerAgentsMDWithCommand(t *testing.T, factoryDir, workerName, command string, args []string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	var argsYAML strings.Builder
	for _, arg := range args {
		argsYAML.WriteString("  - ")
		argsYAML.WriteString(strconv.Quote(arg))
		argsYAML.WriteString("\n")
	}
	agentsMD := fmt.Sprintf("---\ntype: SCRIPT_WORKER\ncommand: %s\nargs:\n%s---\n", command, argsYAML.String())
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(agentsMD), 0o644); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}
}

func writeWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	writeWorkstationAgentsMDWithPrompt(t, factoryDir, workstationName, "Do the work.")
}

type serviceTestRuntimeConfig = runtimefixtures.RuntimeDefinitionLookupFixture

func newLoadedFactoryConfigForServiceTest(
	t *testing.T,
	factoryDir string,
	factoryCfg *interfaces.FactoryConfig,
	workers map[string]*workerconfig.Config,
	workstations map[string]*interfaces.FactoryWorkstationConfig,
) *config.LoadedFactoryConfig {
	t.Helper()
	loaded, err := config.NewLoadedFactoryConfig(factoryDir, factoryCfg, serviceTestRuntimeConfig{
		Workers:      workers,
		Workstations: workstations,
	})
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}
	return loaded
}

func TestLoadWorkersFromConfig_RejectsUnknownConfiguredRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: codex
---
You are a helpful assistant.
`)
	workstationDir := filepath.Join(dir, "workstations", "review")
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(`---
type: MODEL_WORKSTATION
worker: worker-a
runner: mystery-runner
---
Review.
`), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "worker-a"}},
	},
		map[string]*workerconfig.Config{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	_, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, nil, logging.NoopLogger{}, false, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, localModelDomain{})
	if err == nil || !strings.Contains(err.Error(), `unknown runner "mystery-runner"`) {
		t.Fatalf("loadWorkersFromConfig error = %v, want unknown runner", err)
	}
}

func TestLoadWorkersFromConfig_AcceptsAvailableGeminiFactoryRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "worker-a"}},
	},
		map[string]*workerconfig.Config{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	if _, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), workerexecution.RunnerIDGemini, cfg, nil, logging.NoopLogger{}, false, nil, nil, nil, &workers.MockWorkerCommandRunner{}, nil, nil, nil, nil, nil, nil, localModelDomain{}); err != nil {
		t.Fatalf("loadWorkersFromConfig error = %v, want available gemini runner", err)
	}
}

func TestLoadWorkersFromConfig_AcceptsAvailableKiroFactoryRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "worker-a"}},
	},
		map[string]*workerconfig.Config{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	if _, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), workerexecution.RunnerIDKiro, cfg, nil, logging.NoopLogger{}, false, nil, nil, nil, &workers.MockWorkerCommandRunner{}, nil, nil, nil, nil, nil, nil, localModelDomain{}); err != nil {
		t.Fatalf("loadWorkersFromConfig error = %v, want available kiro runner", err)
	}
}

func TestLoadWorkersFromConfig_AcceptsAvailableCursorFactoryRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "worker-a"}},
	},
		map[string]*workerconfig.Config{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	if _, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), workerexecution.RunnerIDCursorCLI, cfg, nil, logging.NoopLogger{}, false, nil, nil, nil, &workers.MockWorkerCommandRunner{}, nil, nil, nil, nil, nil, nil, localModelDomain{}); err != nil {
		t.Fatalf("loadWorkersFromConfig error = %v, want available cursor runner", err)
	}
}

func TestLoadWorkersFromConfig_AcceptsAvailableOpenCodeFactoryRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "worker-a"}},
	},
		map[string]*workerconfig.Config{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	if _, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), workerexecution.RunnerIDOpenCode, cfg, nil, logging.NoopLogger{}, false, nil, nil, nil, &workers.MockWorkerCommandRunner{}, nil, nil, nil, nil, nil, nil, localModelDomain{}); err != nil {
		t.Fatalf("loadWorkersFromConfig error = %v, want available opencode runner", err)
	}
}

func TestLoadWorkersFromConfig_RejectsUnknownFactoryRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "worker-a", `---
type: MODEL_WORKER
model: gpt-5.4
modelProvider: claude
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "worker-a"}},
	},
		map[string]*workerconfig.Config{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	_, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), "mystery-runner", cfg, nil, logging.NoopLogger{}, false, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, localModelDomain{})
	if err == nil || !strings.Contains(err.Error(), `unknown runner "mystery-runner"`) {
		t.Fatalf("loadWorkersFromConfig error = %v, want unknown runner", err)
	}
}

func TestRunnerSelectionValidation_AcceptsLegacyBuiltInRunnerDefault(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "review",
			WorkerTypeName: "worker-a",
		}},
	}
	runtimeCfg := configFixtureWithWorkerAndWorkstation("worker-a", "review", &workerconfig.Config{
		Name:          "worker-a",
		ModelProvider: workerexecution.RunnerIDCodex,
	})

	if err := validateConfiguredWorkstationRunners(cfg, "", runtimeCfg, runnerSelectionPreflight{skipCommandAvailability: true}); err != nil {
		t.Fatalf("validateConfiguredWorkstationRunners: %v", err)
	}
}

func TestRunnerSelectionValidation_SkipsDefaultFallbackValidation(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "review",
			WorkerTypeName: "worker-a",
		}},
	}
	runtimeCfg := configFixtureWithWorkerAndWorkstation("worker-a", "review", &workerconfig.Config{
		Name:          "worker-a",
		ModelProvider: "claude",
	})

	if err := validateConfiguredWorkstationRunners(cfg, "", runtimeCfg, runnerSelectionPreflight{}); err != nil {
		t.Fatalf("validateConfiguredWorkstationRunners: %v", err)
	}
}

func TestRunnerSelectionValidation_RejectsUnknownExplicitWorkstationRunner(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "review",
			WorkerTypeName: "worker-a",
		}},
	}
	runtimeCfg := configRuntimeFixture{
		workers: map[string]*workerconfig.Config{
			"worker-a": {
				Name:          "worker-a",
				ModelProvider: workerexecution.RunnerIDCodex,
			},
		},
		workstations: map[string]*interfaces.FactoryWorkstationConfig{
			"review": {
				Name:           "review",
				WorkerTypeName: "worker-a",
				Runner:         "mystery-runner",
			},
		},
	}

	err := validateConfiguredWorkstationRunners(cfg, "", runtimeCfg, runnerSelectionPreflight{skipCommandAvailability: true})
	if err == nil || !strings.Contains(err.Error(), `unknown runner "mystery-runner"`) {
		t.Fatalf("validateConfiguredWorkstationRunners error = %v, want unknown workstation runner", err)
	}
}

func TestRunnerSelectionValidation_RejectsUnknownExplicitFactoryRunner(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "review",
			WorkerTypeName: "worker-a",
		}},
	}
	runtimeCfg := configFixtureWithWorkerAndWorkstation("worker-a", "review", &workerconfig.Config{
		Name:          "worker-a",
		ModelProvider: "claude",
	})

	err := validateConfiguredWorkstationRunners(cfg, "mystery-runner", runtimeCfg, runnerSelectionPreflight{skipCommandAvailability: true})
	if err == nil || !strings.Contains(err.Error(), `unknown runner "mystery-runner"`) {
		t.Fatalf("validateConfiguredWorkstationRunners error = %v, want unknown factory runner", err)
	}
}

func TestEffectiveFactoryRunnerID_PrefersExplicitOverrideThenConfig(t *testing.T) {
	cfg := &interfaces.FactoryConfig{Runner: workerexecution.RunnerIDGemini}

	if got := effectiveFactoryRunnerID("  cursor-cli  ", cfg); got != workerexecution.RunnerIDCursorCLI {
		t.Fatalf("effectiveFactoryRunnerID override = %q, want %q", got, workerexecution.RunnerIDCursorCLI)
	}
	if got := effectiveFactoryRunnerID("", cfg); got != workerexecution.RunnerIDGemini {
		t.Fatalf("effectiveFactoryRunnerID config = %q, want %q", got, workerexecution.RunnerIDGemini)
	}
	if got := effectiveFactoryRunnerID("", nil); got != "" {
		t.Fatalf("effectiveFactoryRunnerID nil config = %q, want empty", got)
	}
}

func configFixtureWithWorkerAndWorkstation(workerName, workstationName string, worker *workerconfig.Config) configRuntimeFixture {
	return configRuntimeFixture{
		workers: map[string]*workerconfig.Config{
			workerName: worker,
		},
		workstations: map[string]*interfaces.FactoryWorkstationConfig{
			workstationName: {Name: workstationName, WorkerTypeName: workerName},
		},
	}
}

type configRuntimeFixture struct {
	workers      map[string]*workerconfig.Config
	workstations map[string]*interfaces.FactoryWorkstationConfig
	factory      *interfaces.FactoryConfig
}

func (f configRuntimeFixture) Worker(name string) (*workerconfig.Config, bool) {
	worker, ok := f.workers[name]
	return worker, ok
}

func (f configRuntimeFixture) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := f.workstations[name]
	return workstation, ok
}

func (f configRuntimeFixture) FactoryConfig() *interfaces.FactoryConfig {
	return f.factory
}

func (configRuntimeFixture) FactoryDir() string     { return "" }
func (configRuntimeFixture) RuntimeBaseDir() string { return "" }

var _ interfaces.RuntimeConfigLookup = configRuntimeFixture{}

func parseRuntimeLogRecords(t *testing.T, data string) []map[string]any {
	t.Helper()
	var records []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(data), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("runtime log line is not structured JSON: %v\nline: %s", err, line)
		}
		records = append(records, record)
	}
	return records
}

type recordingDiagnosticsCommandRunner struct{}

func (recordingDiagnosticsCommandRunner) Run(_ context.Context, _ workers.CommandRequest) (workers.CommandResult, error) {
	return workers.CommandResult{Stdout: []byte("script done\n"), Stderr: []byte("script details\n")}, nil
}
func serviceReplayDispatchCreatedEvent(t *testing.T, dispatch work.WorkDispatch, tick int) factoryapi.FactoryEvent {
	t.Helper()
	metadata := map[string]string{}
	if dispatch.Execution.ReplayKey != "" {
		metadata["replayKey"] = dispatch.Execution.ReplayKey
	}
	payload := factoryapi.DispatchRequestEventPayload{
		TransitionId: dispatch.TransitionID,
		Inputs:       serviceReplayDispatchInputRefsFromDispatch(dispatch),
		Resources:    serviceReplayResourcesFromDispatch(dispatch),
		Metadata:     serviceDispatchRequestMetadata(metadata),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchRequestEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch created event: %v", err)
	}
	return factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-created/" + dispatch.DispatchID,
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:       tick,
			DispatchId: serviceStringPtr(dispatch.DispatchID),
			RequestId:  serviceStringPtr(dispatch.Execution.RequestID),
			TraceIds:   serviceStringSlicePtr([]string{dispatch.Execution.TraceID}),
			WorkIds:    serviceStringSlicePtr(dispatch.Execution.WorkIDs),
		},
		Payload: union,
	}
}

func serviceReplayWorkRequestEvent(t *testing.T, requestID string, tick int, source string, works []factoryapi.Work, relations []factoryapi.Relation) factoryapi.FactoryEvent {
	t.Helper()
	payload := factoryapi.WorkRequestEventPayload{
		Type:      factoryapi.WorkRequestTypeFactoryRequestBatch,
		Works:     serviceSlicePtr(works),
		Relations: serviceSlicePtr(relations),
		Source:    serviceStringPtr(source),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromWorkRequestEventPayload(payload); err != nil {
		t.Fatalf("encode work request event: %v", err)
	}
	var traceIDs []string
	var workIDs []string
	for _, work := range works {
		traceIDs = append(traceIDs, serviceStringValue(work.TraceId))
		workIDs = append(workIDs, serviceStringValue(work.WorkId))
	}
	return factoryapi.FactoryEvent{
		Id:            "factory-event/work-request/" + requestID,
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeWorkRequest,
		Context: factoryapi.FactoryEventContext{
			EventTime: time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:      tick,
			RequestId: serviceStringPtr(requestID),
			Source:    serviceStringPtr(source),
			TraceIds:  serviceStringSlicePtr(serviceUniqueNonEmpty(traceIDs)),
			WorkIds:   serviceStringSlicePtr(serviceUniqueNonEmpty(workIDs)),
		},
		Payload: union,
	}
}

func serviceReplayDispatchCompletedEvent(t *testing.T, completionID string, result workerexecution.WorkResult, tick int) factoryapi.FactoryEvent {
	t.Helper()
	payload := factoryapi.DispatchResponseEventPayload{
		CompletionId:                serviceStringPtr(completionID),
		TransitionId:                result.TransitionID,
		Outcome:                     factoryapi.WorkOutcome(result.Outcome),
		Output:                      serviceStringPtr(result.Output),
		Error:                       serviceStringPtr(result.Error),
		Feedback:                    serviceStringPtr(result.Feedback),
		SelectedClassificationLabel: serviceStringPtr(result.SelectedClassificationLabel),
		ProviderFailure:             workerdiagnosticsmapping.GeneratedWorkFailureMetadata(result.FailureMetadata),
		Metrics:                     serviceWorkMetricsPtr(result.Metrics),
	}
	var union factoryapi.FactoryEvent_Payload
	if err := union.FromDispatchResponseEventPayload(payload); err != nil {
		t.Fatalf("encode dispatch completed event: %v", err)
	}
	return factoryapi.FactoryEvent{
		Id:            "factory-event/dispatch-completed/" + result.DispatchID,
		SchemaVersion: factoryapi.AgentFactoryEventV1,
		Type:          factoryapi.FactoryEventTypeDispatchResponse,
		Context: factoryapi.FactoryEventContext{
			EventTime:  time.Date(2026, time.April, 10, 12, 0, tick, 0, time.UTC),
			Tick:       tick,
			DispatchId: serviceStringPtr(result.DispatchID),
		},
		Payload: union,
	}
}

func serviceReplayWorkRequestEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayWorkRequestRecord {
	t.Helper()
	var out []serviceReplayWorkRequestRecord
	for _, domainEvent := range artifact.Events {
		event := serviceGeneratedReplayEvent(t, domainEvent)
		if event.Type != factoryapi.FactoryEventTypeWorkRequest {
			continue
		}
		payload, err := event.Payload.AsWorkRequestEventPayload()
		if err != nil {
			t.Fatalf("decode work request event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayWorkRequestRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayWorkRequestRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.WorkRequestEventPayload
}

func serviceReplayDispatchCreatedEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayDispatchCreatedRecord {
	t.Helper()
	var out []serviceReplayDispatchCreatedRecord
	for _, domainEvent := range artifact.Events {
		event := serviceGeneratedReplayEvent(t, domainEvent)
		if event.Type != factoryapi.FactoryEventTypeDispatchRequest {
			continue
		}
		payload, err := event.Payload.AsDispatchRequestEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch created event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayDispatchCreatedRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayDispatchCreatedRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.DispatchRequestEventPayload
}

func serviceReplayDispatchCompletedEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayDispatchCompletedRecord {
	t.Helper()
	var out []serviceReplayDispatchCompletedRecord
	for _, domainEvent := range artifact.Events {
		event := serviceGeneratedReplayEvent(t, domainEvent)
		if event.Type != factoryapi.FactoryEventTypeDispatchResponse {
			continue
		}
		payload, err := event.Payload.AsDispatchResponseEventPayload()
		if err != nil {
			t.Fatalf("decode dispatch completed event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayDispatchCompletedRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayDispatchCompletedRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.DispatchResponseEventPayload
}

func serviceReplayInferenceResponseEvents(t *testing.T, artifact *interfaces.ReplayArtifact) []serviceReplayInferenceResponseRecord {
	t.Helper()
	var out []serviceReplayInferenceResponseRecord
	for _, domainEvent := range artifact.Events {
		event := serviceGeneratedReplayEvent(t, domainEvent)
		if event.Type != factoryapi.FactoryEventTypeInferenceResponse {
			continue
		}
		payload, err := event.Payload.AsInferenceResponseEventPayload()
		if err != nil {
			t.Fatalf("decode inference response event %q: %v", event.Id, err)
		}
		out = append(out, serviceReplayInferenceResponseRecord{Event: event, Payload: payload})
	}
	return out
}

type serviceReplayInferenceResponseRecord struct {
	Event   factoryapi.FactoryEvent
	Payload factoryapi.InferenceResponseEventPayload
}

func serviceGeneratedReplayEvent(t *testing.T, event interfaces.FactoryEvent) factoryapi.FactoryEvent {
	t.Helper()
	var generated factoryapi.FactoryEvent
	if err := event.Decode(&generated); err != nil {
		t.Fatalf("decode canonical replay event %q: %v", event.Id, err)
	}
	return generated
}

func serviceReplayWorksFromDispatch(dispatch work.WorkDispatch) []factoryapi.Work {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	works := make([]factoryapi.Work, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == factorytoken.DataTypeResource {
			continue
		}
		workID := firstNonEmpty(token.Color.WorkID, token.ID)
		works = append(works, factoryapi.Work{
			Name:         firstNonEmpty(token.Color.Name, workID),
			WorkId:       serviceStringPtr(workID),
			WorkTypeName: serviceStringPtr(token.Color.WorkTypeID),
			TraceId:      serviceStringPtr(token.Color.TraceID),
			Tags:         serviceStringMapPtr(token.Color.Tags),
		})
	}
	if len(works) == 0 {
		for _, workID := range dispatch.Execution.WorkIDs {
			works = append(works, factoryapi.Work{
				Name:         workID,
				WorkId:       serviceStringPtr(workID),
				WorkTypeName: serviceStringPtr("task"),
				TraceId:      serviceStringPtr(dispatch.Execution.TraceID),
			})
		}
	}
	return works
}

func serviceReplayDispatchInputRefsFromDispatch(dispatch work.WorkDispatch) []factoryapi.DispatchConsumedWorkRef {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	refs := make([]factoryapi.DispatchConsumedWorkRef, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == factorytoken.DataTypeResource {
			continue
		}
		workID := firstNonEmpty(token.Color.WorkID, token.ID)
		if workID == "" {
			continue
		}
		refs = append(refs, factoryapi.DispatchConsumedWorkRef{WorkId: workID})
	}
	if len(refs) == 0 {
		for _, workID := range dispatch.Execution.WorkIDs {
			if workID == "" {
				continue
			}
			refs = append(refs, factoryapi.DispatchConsumedWorkRef{WorkId: workID})
		}
	}
	return refs
}

func serviceReplayResourcesFromDispatch(dispatch work.WorkDispatch) *[]factoryapi.Resource {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	resources := make([]factoryapi.Resource, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType != factorytoken.DataTypeResource {
			continue
		}
		resources = append(resources, factoryapi.Resource{Name: firstNonEmpty(token.Color.WorkTypeID, token.Color.Name)})
	}
	return serviceSlicePtr(resources)
}

func serviceDispatchRequestMetadata(values map[string]string) *factoryapi.DispatchRequestEventMetadata {
	if len(values) == 0 {
		return nil
	}
	return &factoryapi.DispatchRequestEventMetadata{
		ReplayKey: serviceStringPtr(values["replayKey"]),
	}
}

func serviceWorkMetricsPtr(metrics workerexecution.WorkMetrics) *factoryapi.WorkMetrics {
	if metrics.Duration == 0 && metrics.Cost == 0 && metrics.RetryCount == 0 {
		return nil
	}
	return &factoryapi.WorkMetrics{
		DurationMillis: serviceInt64Ptr(metrics.Duration.Milliseconds()),
		Cost:           serviceFloat64Ptr(metrics.Cost),
		RetryCount:     serviceIntPtr(metrics.RetryCount),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func serviceStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func serviceFirstStringValue(values *[]string) string {
	if values == nil {
		return ""
	}
	for _, value := range *values {
		if value != "" {
			return value
		}
	}
	return ""
}

func serviceUniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func serviceStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func serviceEnumPtr[T ~string](value T) *T {
	if value == "" {
		return nil
	}
	return &value
}

func serviceIntPtr(value int) *int {
	if value == 0 {
		return nil
	}
	return &value
}

func serviceInt64Ptr(value int64) *int64 {
	if value == 0 {
		return nil
	}
	return &value
}

func serviceFloat64Ptr(value float64) *float64 {
	if value == 0 {
		return nil
	}
	return &value
}

func serviceStringSlicePtr(values []string) *[]string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			out = append(out, value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return &out
}

func serviceStringMapPtr(values map[string]string) *factoryapi.StringMap {
	if len(values) == 0 {
		return nil
	}
	converted := factoryapi.StringMap{}
	for key, value := range values {
		if value != "" {
			converted[key] = value
		}
	}
	if len(converted) == 0 {
		return nil
	}
	return &converted
}

func serviceSlicePtr[T any](values []T) *[]T {
	if len(values) == 0 {
		return nil
	}
	out := append([]T(nil), values...)
	return &out
}

func assertServiceFactoryEventsContainTypes(t *testing.T, events []factoryapi.FactoryEvent, wantTypes []factoryapi.FactoryEventType) {
	t.Helper()
	seen := make(map[factoryapi.FactoryEventType]bool, len(events))
	for _, event := range events {
		seen[event.Type] = true
	}
	for _, wantType := range wantTypes {
		if !seen[wantType] {
			t.Fatalf("factory event types = %v, want %s", serviceFactoryEventTypes(events), wantType)
		}
	}
}

func serviceFactoryEventTypes(events []factoryapi.FactoryEvent) []factoryapi.FactoryEventType {
	types := make([]factoryapi.FactoryEventType, 0, len(events))
	for _, event := range events {
		types = append(types, event.Type)
	}
	return types
}

func createReplacementWatchChannel(t *testing.T, factoryDir, workType, channel string) {
	t.Helper()

	inputDir := filepath.Join(factoryDir, interfaces.InputsDir, workType, channel)
	if err := os.MkdirAll(inputDir, 0o755); err != nil {
		t.Fatalf("create watched input dir %q: %v", inputDir, err)
	}
}

func writeNamedFactoryFixture(t *testing.T, rootDir, name string) string {
	t.Helper()

	payload, err := json.Marshal(map[string]any{
		"name": name,
		"id":   name,
		"workTypes": []map[string]any{
			{
				"name": "task",
				"states": []map[string]string{
					{"name": "init", "type": "INITIAL"},
					{"name": "complete", "type": "TERMINAL"},
					{"name": "failed", "type": "FAILED"},
				},
			},
		},
		"workers": []map[string]any{
			{
				"name":          "executor",
				"type":          "MODEL_WORKER",
				"modelProvider": "CODEX",
				"model":         "gpt-5-codex",
				"body":          "You are the executor.",
			},
		},
		"workstations": []map[string]any{
			{
				"name":      "execute-" + name,
				"worker":    "executor",
				"inputs":    []map[string]string{{"workType": "task", "state": "init"}},
				"outputs":   []map[string]string{{"workType": "task", "state": "complete"}},
				"onFailure": []map[string]string{{"workType": "task", "state": "failed"}},
				"type":      "MODEL_WORKSTATION",
				"body":      "Implement {{ .WorkID }}.",
			},
		},
	})
	if err != nil {
		t.Fatalf("Marshal(named factory fixture): %v", err)
	}

	factoryDir, err := config.PersistNamedFactory(rootDir, name, payload)
	if err != nil {
		t.Fatalf("PersistNamedFactory(%s): %v", name, err)
	}
	return factoryDir
}

func cleanResolvedPath(path string) string {
	cleaned := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return cleaned
	}
	return filepath.Clean(resolved)
}

func newServiceTestSupervisedModelHost(t *testing.T, puller modelAssetPuller, launcher modelhost.ProcessLauncher) modelhost.Host {
	t.Helper()
	gateway := modelhost.NewLocalAssetGateway(puller)
	host, err := modelhost.NewHost(modelhost.Dependencies{
		AssetPuller: gateway, CacheInspector: gateway, ProcessLauncher: launcher,
		Options: modelhost.Options{
			SourceResolver: modelhost.DefaultManagedRuntimeSourceResolverAdapter(),
			Supervisor: modelhost.SupervisorConfig{
				ReadinessTimeout:    500 * time.Millisecond,
				HealthCheckInterval: 10 * time.Millisecond,
				HealthChecker:       modelhost.HTTPHealthChecker{Path: "/health"},
			},
		}})
	if err != nil {
		t.Fatalf("NewHost: %v", err)
	}
	return host
}

type serviceTestFakeProcessLauncher struct {
	mu              sync.Mutex
	healthEndpoint  string
	supervisedStart int
}

func (f *serviceTestFakeProcessLauncher) Start(_ context.Context, _ modelhost.ProcessStartSpec) (modelhost.ManagedProcess, error) {
	f.mu.Lock()
	f.supervisedStart++
	f.mu.Unlock()
	return &serviceTestFakeManagedProcess{
		endpoint: f.healthEndpoint,
		stopCh:   make(chan struct{}),
	}, nil
}

func (f *serviceTestFakeProcessLauncher) startCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.supervisedStart
}

type serviceTestFakeManagedProcess struct {
	endpoint string
	stopCh   chan struct{}
}

func (p *serviceTestFakeManagedProcess) HealthEndpoint() string {
	return p.endpoint
}

func (p *serviceTestFakeManagedProcess) Wait() error {
	<-p.stopCh
	return nil
}

func (p *serviceTestFakeManagedProcess) Stop(context.Context) error {
	close(p.stopCh)
	return nil
}

type prefixBlockingExecutor struct {
	blockPrefix string
	started     chan struct{}
	release     chan struct{}
}

func (e *prefixBlockingExecutor) Execute(_ context.Context, dispatch work.WorkDispatch) (workerexecution.WorkResult, error) {
	workID := ""
	if len(dispatch.Execution.WorkIDs) > 0 {
		workID = dispatch.Execution.WorkIDs[0]
	}
	if strings.HasPrefix(workID, e.blockPrefix) {
		select {
		case e.started <- struct{}{}:
		default:
		}
		<-e.release
	}
	return workerexecution.WorkResult{
		DispatchID:   dispatch.DispatchID,
		TransitionID: dispatch.TransitionID,
		Outcome:      workerexecution.OutcomeAccepted,
		Output:       "done",
	}, nil
}

func pauseSessionFactory(t *testing.T, session *liveFactorySession) {
	t.Helper()
	if err := liveSessionHandle(session).Bundle.Factory.Pause(context.Background()); err != nil {
		t.Fatalf("Pause(%s): %v", session.ID, err)
	}
}

func resumeSessionFactory(t *testing.T, session *liveFactorySession) {
	t.Helper()
	if err := liveSessionHandle(session).Bundle.Factory.Resume(context.Background()); err != nil {
		t.Fatalf("Resume(%s): %v", session.ID, err)
	}
}

func requireLiveSession(t *testing.T, svc *FactoryService, sessionID string) *liveFactorySession {
	t.Helper()
	session := svc.sessionByID(sessionID)
	if session == nil {
		t.Fatalf("session %q is not registered", sessionID)
	}
	return session
}

func sessionEngineSnapshot(t *testing.T, session *liveFactorySession) *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net] {
	t.Helper()
	if session == nil || liveSessionHandle(session) == nil || liveSessionHandle(session).Bundle == nil {
		t.Fatal("live session runtime is required")
	}
	snap, err := liveSessionHandle(session).Bundle.Factory.GetEngineStateSnapshot(context.Background())
	if err != nil {
		t.Fatalf("GetEngineStateSnapshot(%s): %v", session.ID, err)
	}
	return snap
}

func snapshotHasTokenAtPlace(snap *interfaces.EngineStateSnapshot[petri.MarkingSnapshot, *state.Net], placeID string) bool {
	for _, token := range snap.Marking.Tokens {
		if token.PlaceID == placeID {
			return true
		}
	}
	return false
}

func waitForSessionInFlight(t *testing.T, session *liveFactorySession, wait time.Duration) {
	t.Helper()
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		snap := sessionEngineSnapshot(t, session)
		if snap.InFlightCount > 0 && len(snap.Dispatches) > 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	snap := sessionEngineSnapshot(t, session)
	t.Fatalf("timed out waiting for in-flight dispatch on session %s inFlight=%d dispatches=%d", session.ID, snap.InFlightCount, len(snap.Dispatches))
}

func TestNormalizeInvocationBootstrapConfig_ForcesNoServerShape(t *testing.T) {
	t.Parallel()

	ready := make(chan struct{})
	starter := func(context.Context, apisurface.APISurface, int, *zap.Logger) error {
		close(ready)
		return nil
	}
	renderer := func(SimpleDashboardRenderInput) {}

	cfg := &FactoryServiceConfig{
		Port:                    7437,
		RuntimeMode:             interfaces.RuntimeModeBatch,
		WorkFile:                "/tmp/work.json",
		APIServerStarter:        starter,
		APIServerReady:          ready,
		SimpleDashboardRenderer: renderer,
	}

	got := NormalizeInvocationBootstrapConfig(cfg)
	if got.Port != 0 {
		t.Fatalf("Port = %d, want 0", got.Port)
	}
	if got.APIServerStarter != nil {
		t.Fatal("APIServerStarter = non-nil, want nil")
	}
	if got.APIServerReady != nil {
		t.Fatal("APIServerReady = non-nil, want nil")
	}
	if got.SimpleDashboardRenderer != nil {
		t.Fatal("SimpleDashboardRenderer = non-nil, want nil")
	}
	if got.RuntimeMode != interfaces.RuntimeModeService {
		t.Fatalf("RuntimeMode = %q, want %q", got.RuntimeMode, interfaces.RuntimeModeService)
	}
	if got.WorkFile != "" {
		t.Fatalf("WorkFile = %q, want empty", got.WorkFile)
	}
}

func BuildInvocationBootstrap(ctx context.Context, cfg *FactoryServiceConfig) (*InvocationBootstrap, error) {
	svc, err := BuildFactoryService(ctx, NormalizeInvocationBootstrapConfig(cfg))
	if err != nil {
		return nil, err
	}
	return NewInvocationBootstrap(svc)
}

func TestBuildInvocationBootstrap_LeavesNoFactoryAPIServerListener(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeInvocationBootstrapWorkerAgentsMD(t, dir, "worker-a")
	writeInvocationBootstrapWorkstationAgentsMD(t, dir, "process")
	if err := os.MkdirAll(filepath.Join(dir, interfaces.InputsDir), 0o755); err != nil {
		t.Fatalf("create inputs dir: %v", err)
	}

	probePort := reserveInvocationBootstrapProbePort(t)
	apiReady := make(chan struct{})
	starterInvoked := make(chan struct{}, 1)
	cfg := &FactoryServiceConfig{
		Dir:               dir,
		Port:              probePort,
		RuntimeMode:       interfaces.RuntimeModeBatch,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
		APIServerReady:    apiReady,
		APIServerStarter: func(ctx context.Context, _ apisurface.APISurface, port int, _ *zap.Logger) error {
			starterInvoked <- struct{}{}
			listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			if err != nil {
				return err
			}
			close(apiReady)
			<-ctx.Done()
			return listener.Close()
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bootstrap, err := BuildInvocationBootstrap(ctx, cfg)
	if err != nil {
		t.Fatalf("BuildInvocationBootstrap: %v", err)
	}

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- bootstrap.Run(ctx)
	}()

	waitForInvocationBootstrapSessionReady(t, ctx, bootstrap, runErrCh)
	assertFactoryAPIServerPortFree(t, probePort)

	select {
	case <-starterInvoked:
		t.Fatal("APIServerStarter invoked, want no-server bootstrap to skip HTTP listener startup")
	default:
	}

	cancel()
	if err := <-runErrCh; err != nil && err != context.Canceled {
		t.Fatalf("Run: %v", err)
	}
}

func TestInvocationBootstrap_CloseFactorySessionReleasesLiveSession(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryfixtures.WriteFactoryJSON(t, dir, factoryfixtures.MinimalFactoryConfig())
	writeInvocationBootstrapWorkerAgentsMD(t, dir, "worker-a")
	writeInvocationBootstrapWorkstationAgentsMD(t, dir, "process")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bootstrap, err := BuildInvocationBootstrap(ctx, &FactoryServiceConfig{
		Dir:               dir,
		MockWorkersConfig: config.NewEmptyMockWorkersConfig(),
		Logger:            zap.NewNop(),
	})
	if err != nil {
		t.Fatalf("BuildInvocationBootstrap: %v", err)
	}

	runErrCh := make(chan error, 1)
	go func() {
		runErrCh <- bootstrap.Run(ctx)
	}()

	waitForInvocationBootstrapSessionReady(t, ctx, bootstrap, runErrCh)

	if _, err := bootstrap.Service.GetFactorySession(ctx, factorysessions.DefaultSessionID); err != nil {
		t.Fatalf("GetFactorySession before close: %v", err)
	}
	if err := bootstrap.CloseFactorySession(ctx, factorysessions.DefaultSessionID); err != nil {
		t.Fatalf("CloseFactorySession: %v", err)
	}
	if _, err := bootstrap.Service.GetFactorySession(ctx, factorysessions.DefaultSessionID); !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
		t.Fatalf("GetFactorySession after close = %v, want %v", err, apisurface.ErrFactorySessionNotFound)
	}

	cancel()
	if err := <-runErrCh; err != nil && !errors.Is(err, context.Canceled) {
		t.Fatalf("Run: %v", err)
	}
}

func TestInvocationBootstrap_InvokeFactorySessionForwardsToCanonicalOwner(t *testing.T) {
	t.Parallel()

	requestID := "request-1"
	request := factoryapi.InvocationRequest{RequestId: &requestID, Args: &map[string]any{"input": "hello"}}
	wantResult := sessioninvocation.FactoryInvocationResult{
		RequestID: "result-request",
		TraceID:   "trace-1",
		Status:    "COMPLETED",
	}
	wantErr := errors.New("owner failure")
	invoker := &forwardingSessionInvoker{result: wantResult, err: wantErr}
	svc := &FactoryService{sessionInvoker: invoker}
	bootstrap := &InvocationBootstrap{Service: svc}
	type contextKey string
	ctx := context.WithValue(context.Background(), contextKey("forwarding"), "preserved")

	got, err := bootstrap.InvokeFactorySession(ctx, "session-1", request)
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want owner error %v", err, wantErr)
	}
	if !reflect.DeepEqual(got, wantResult) {
		t.Fatalf("result = %#v, want %#v", got, wantResult)
	}
	if invoker.ctx != ctx || invoker.sessionID != "session-1" {
		t.Fatalf("forwarded ctx/session = %#v/%q", invoker.ctx, invoker.sessionID)
	}
	if invoker.request.RequestID == nil || *invoker.request.RequestID != requestID || invoker.request.Args == nil || (*invoker.request.Args)["input"] != "hello" {
		t.Fatalf("forwarded request = %#v", invoker.request)
	}
}

func reserveInvocationBootstrapProbePort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("Close listener: %v", err)
	}
	return port
}

func waitForInvocationBootstrapSessionReady(
	t *testing.T,
	ctx context.Context,
	bootstrap *InvocationBootstrap,
	runErrCh <-chan error,
) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := bootstrap.GetCurrentFactoryForSession(ctx, factorysessions.DefaultSessionID); err == nil {
			return
		} else if !errors.Is(err, apisurface.ErrFactorySessionNotFound) {
			t.Fatalf("GetCurrentFactoryForSession: %v", err)
		}

		select {
		case err := <-runErrCh:
			if err == nil || err == context.Canceled {
				t.Fatal("bootstrap runtime stopped before session became ready")
			}
			t.Fatalf("bootstrap runtime failed before session became ready: %v", err)
		case <-ctx.Done():
			t.Fatalf("context canceled before session became ready: %v", ctx.Err())
		default:
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("timed out waiting for bootstrap session readiness")
}

func assertFactoryAPIServerPortFree(t *testing.T, port int) {
	t.Helper()

	addr := fmt.Sprintf("127.0.0.1:%d", port)
	conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
	if err == nil {
		_ = conn.Close()
		t.Fatalf("tcp %s accepted a connection, want no factory API/dashboard listener", addr)
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("tcp %s still held by bootstrap listener: %v", addr, err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("Close probe listener: %v", err)
	}
}

func writeInvocationBootstrapWorkerAgentsMD(t *testing.T, factoryDir, workerName string) {
	t.Helper()
	workerDir := filepath.Join(factoryDir, "workers", workerName)
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("create worker dir: %v", err)
	}
	content := "---\ntype: MODEL_WORKER\nmodel: claude-3-5-haiku-20241022\n---\nYou are a helpful assistant.\n"
	if err := os.WriteFile(filepath.Join(workerDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write worker AGENTS.md: %v", err)
	}
}

func writeInvocationBootstrapWorkstationAgentsMD(t *testing.T, factoryDir, workstationName string) {
	t.Helper()
	workstationDir := filepath.Join(factoryDir, "workstations", workstationName)
	if err := os.MkdirAll(workstationDir, 0o755); err != nil {
		t.Fatalf("create workstation dir: %v", err)
	}
	content := "---\ntype: MODEL_WORKSTATION\n---\nDo the work.\n"
	if err := os.WriteFile(filepath.Join(workstationDir, "AGENTS.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write workstation AGENTS.md: %v", err)
	}
}

func TestLoadWorkersFromConfig_InvocationSkipPermissionsOverridePropagatesToAgentProviderCommand(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "agent-a", `---
type: AGENT_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful agent.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "agent-a"}},
	},
		map[string]*workerconfig.Config{
			"agent-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{Stdout: []byte("done COMPLETE")},
	}
	override := true
	opts, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		&override,
		nil,
		nil,
		runner,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	executeProviderBackedWorker(t, opts, "agent-a", runner)
	assertProviderArgsContain(t, runner.Requests(), "--dangerously-skip-permissions")
}

func TestLoadWorkersFromConfig_InvocationSkipPermissionsOverrideDoesNotApplyToModelWorker(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "model-a", `---
type: MODEL_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful assistant.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "model-a"}},
	},
		map[string]*workerconfig.Config{
			"model-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "model-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{Stdout: []byte("done COMPLETE")},
	}
	override := true
	opts, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		&override,
		nil,
		nil,
		runner,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	executeProviderBackedWorker(t, opts, "model-a", runner)
	assertProviderArgsDoNotContain(t, runner.Requests(), "--dangerously-skip-permissions")
}

func TestLoadWorkersFromConfig_PersistedSkipPermissionsTrueWithoutInvocationOverride(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "agent-a", `---
type: AGENT_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
stopToken: COMPLETE
skipPermissions: true
---
You are a helpful agent.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "agent-a"}},
	},
		map[string]*workerconfig.Config{
			"agent-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{Stdout: []byte("done COMPLETE")},
	}
	opts, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		nil,
		nil,
		nil,
		runner,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	executeProviderBackedWorker(t, opts, "agent-a", runner)
	assertProviderArgsContain(t, runner.Requests(), "--dangerously-skip-permissions")
}

func executeProviderBackedWorker(
	t *testing.T,
	opts []factory.FactoryOption,
	workerName string,
	runner *providerCommandRunnerRecorder,
) {
	t.Helper()

	fc := &factory.FactoryConfig{}
	for _, opt := range opts {
		opt(fc)
	}

	exec, ok := fc.WorkerExecutors[workerName]
	if !ok {
		t.Fatalf("expected %q executor to be registered", workerName)
	}
	wsExec, ok := exec.(*workers.WorkstationExecutor)
	if !ok {
		t.Fatalf("expected *workers.WorkstationExecutor, got %T", exec)
	}

	if _, err := wsExec.Execute(context.Background(), work.WorkDispatch{
		DispatchID:      "dispatch-skip-permissions",
		TransitionID:    "transition-skip-permissions",
		WorkerType:      workerName,
		WorkstationName: "review",
		InputTokens: workers.InputTokens(factorytoken.Token{
			ID: "token-skip-permissions",
			Color: factorytoken.Color{
				WorkID:  "work-skip-permissions",
				Payload: []byte("helpful input"),
			},
		}),
	}); err != nil {
		t.Fatalf("execute worker: %v", err)
	}
	if len(runner.Requests()) != 1 {
		t.Fatalf("provider command count = %d, want 1", len(runner.Requests()))
	}
}

func assertProviderArgsContain(t *testing.T, requests []workers.CommandRequest, want string) {
	t.Helper()
	if len(requests) == 0 {
		t.Fatal("expected provider command requests")
	}
	joined := strings.Join(requests[0].Args, " ")
	if !strings.Contains(joined, want) {
		t.Fatalf("provider args = %q, want substring %q", joined, want)
	}
}

func assertProviderArgsDoNotContain(t *testing.T, requests []workers.CommandRequest, unwanted string) {
	t.Helper()
	if len(requests) == 0 {
		t.Fatal("expected provider command requests")
	}
	joined := strings.Join(requests[0].Args, " ")
	if strings.Contains(joined, unwanted) {
		t.Fatalf("provider args = %q, want to omit %q", joined, unwanted)
	}
}

func TestLoadWorkersFromConfig_InvocationSkipPermissionsOverrideFailsForUnsupportedAgentProvider(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "agent-a", `---
type: AGENT_WORKER
model: custom-model
modelProvider: acme
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful agent.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "agent-a"}},
	},
		map[string]*workerconfig.Config{
			"agent-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{Stdout: []byte("done COMPLETE")},
	}
	override := true
	_, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		&override,
		nil,
		nil,
		runner,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
	if err == nil {
		t.Fatal("expected loadWorkersFromConfig to fail for unsupported provider with --skip-permissions")
	}
	if !strings.Contains(err.Error(), "skip-permissions") {
		t.Fatalf("error = %q, want skip-permissions failure", err.Error())
	}
	if !strings.Contains(err.Error(), "acme") {
		t.Fatalf("error = %q, want unsupported provider detail", err.Error())
	}
	if len(runner.Requests()) != 0 {
		t.Fatalf("provider command count = %d, want 0 before dispatch", len(runner.Requests()))
	}
}

func TestLoadWorkersFromConfig_InvocationSkipPermissionsOverrideFailsForLocalManagedAgentWorker(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "agent-a", `---
type: AGENT_WORKER
model: OMNIVOICE_Q4_K_M
modelProvider: claude
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful agent.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers: []workerconfig.Config{{
			Name:          "agent-a",
			ModelLocality: workerconfig.ModelLocalityLocal,
		}},
	},
		map[string]*workerconfig.Config{
			"agent-a": func() *workerconfig.Config {
				worker := mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-a"))
				worker.ModelLocality = workerconfig.ModelLocalityLocal
				return worker
			}(),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{Stdout: []byte("done COMPLETE")},
	}
	override := true
	_, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		&override,
		nil,
		nil,
		runner,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
	if err == nil {
		t.Fatal("expected loadWorkersFromConfig to fail for local managed agent worker with --skip-permissions")
	}
	if !strings.Contains(err.Error(), "local managed model workers cannot honor CLI skip-permissions") {
		t.Fatalf("error = %q, want local managed model failure detail", err.Error())
	}
	if len(runner.Requests()) != 0 {
		t.Fatalf("provider command count = %d, want 0 before dispatch", len(runner.Requests()))
	}
}

func TestLoadWorkersFromConfig_InvocationSkipPermissionsOverrideFailsWhenFactoryHasUnsupportedAgentPeer(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "agent-supported", `---
type: AGENT_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful agent.
`)
	writeWorkerAgentsMDWithContent(t, dir, "agent-unsupported", `---
type: AGENT_WORKER
model: custom-model
modelProvider: acme
stopToken: COMPLETE
skipPermissions: false
---
You are another agent.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers: []workerconfig.Config{
			{Name: "agent-supported"},
			{Name: "agent-unsupported"},
		},
	},
		map[string]*workerconfig.Config{
			"agent-supported":   mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-supported")),
			"agent-unsupported": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-unsupported")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{Stdout: []byte("done COMPLETE")},
	}
	override := true
	_, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		&override,
		nil,
		nil,
		runner,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
	if err == nil {
		t.Fatal("expected mixed factory to fail when --skip-permissions is set and any agent worker is unsupported")
	}
	if !strings.Contains(err.Error(), "agent-unsupported") {
		t.Fatalf("error = %q, want unsupported worker name", err.Error())
	}
	if len(runner.Requests()) != 0 {
		t.Fatalf("provider command count = %d, want 0 before dispatch", len(runner.Requests()))
	}
}

func TestLoadWorkersFromConfig_UnsupportedAgentProviderWithoutInvocationOverrideDoesNotFail(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "agent-a", `---
type: AGENT_WORKER
model: custom-model
modelProvider: acme
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful agent.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "agent-a"}},
	},
		map[string]*workerconfig.Config{
			"agent-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{Stdout: []byte("done COMPLETE")},
	}
	opts, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		nil,
		nil,
		nil,
		runner,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig without override: %v", err)
	}

	executeProviderBackedWorker(t, opts, "agent-a", runner)
	assertProviderArgsDoNotContain(t, runner.Requests(), "--dangerously-skip-permissions")
}

func TestS14AbsentOverrideUsesPersistedFalseOnly(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "agent-a", `---
type: AGENT_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful agent.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "agent-a"}},
	},
		map[string]*workerconfig.Config{
			"agent-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{Stdout: []byte("done COMPLETE")},
	}
	opts, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		nil,
		nil,
		nil,
		runner,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	executeProviderBackedWorker(t, opts, "agent-a", runner)
	assertProviderArgsDoNotContain(t, runner.Requests(), "--dangerously-skip-permissions")
}

func TestS14InvocationSkipPermissionsDoesNotMutateWorkerAgentsFrontmatter(t *testing.T) {
	dir := t.TempDir()

	agentsPath := filepath.Join(dir, "workers", "agent-a", "AGENTS.md")
	agentsContent := `---
type: AGENT_WORKER
model: claude-sonnet-4-20250514
modelProvider: claude
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful agent.
`
	writeWorkerAgentsMDWithContent(t, dir, "agent-a", agentsContent)
	writeWorkstationAgentsMD(t, dir, "review")

	before, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md before load: %v", err)
	}

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "agent-a"}},
	},
		map[string]*workerconfig.Config{
			"agent-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{Stdout: []byte("done COMPLETE")},
	}
	override := true
	opts, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		&override,
		nil,
		nil,
		runner,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
	if err != nil {
		t.Fatalf("loadWorkersFromConfig: %v", err)
	}

	executeProviderBackedWorker(t, opts, "agent-a", runner)
	assertProviderArgsContain(t, runner.Requests(), "--dangerously-skip-permissions")

	after, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatalf("read AGENTS.md after load: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("AGENTS.md changed after invocation override:\nbefore=%q\nafter=%q", before, after)
	}
	if !strings.Contains(string(after), "skipPermissions: false") {
		t.Fatalf("AGENTS.md = %q, want persisted skipPermissions:false unchanged", string(after))
	}
	if strings.Contains(string(after), "skipPermissions: true") {
		t.Fatalf("AGENTS.md = %q, want skipPermissions not persisted as true", string(after))
	}
}

func TestS14UnsupportedProviderFailsBeforeDispatchEvidence(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMDWithContent(t, dir, "agent-a", `---
type: AGENT_WORKER
model: custom-model
modelProvider: acme
stopToken: COMPLETE
skipPermissions: false
---
You are a helpful agent.
`)
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []workerconfig.Config{{Name: "agent-a"}},
	},
		map[string]*workerconfig.Config{
			"agent-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "agent-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	runner := &providerCommandRunnerRecorder{
		result: workers.CommandResult{Stdout: []byte("done COMPLETE")},
	}
	override := true
	_, err := loadWorkersFromConfig(
		cfg.FactoryDir(),
		cfg.FactoryConfig(),
		"",
		cfg,
		nil,
		logging.NoopLogger{},
		false,
		&override,
		nil,
		nil,
		runner,
		nil,
		nil,
		nil,
		nil,
		nil,
		time.Now,
		LocalModelDomain{},
	)
	if err == nil {
		t.Fatal("expected unsupported provider to fail before dispatch when --skip-permissions is set")
	}
	if !strings.Contains(err.Error(), "skip-permissions") {
		t.Fatalf("error = %q, want skip-permissions failure", err.Error())
	}
	if len(runner.Requests()) != 0 {
		t.Fatalf("provider command count = %d, want 0 before dispatch", len(runner.Requests()))
	}
}

func TestWorkerApplicationWithProgressPreservesCompositionSelection(t *testing.T) {
	t.Run("production graph installs progress runner", func(t *testing.T) {
		components, err := workerapplication.New(zap.NewNop(), workerapplication.Edges{})
		if err != nil {
			t.Fatalf("construct production worker application: %v", err)
		}
		got, err := workerApplicationWithProgress(runtimeBundleBuildInput{
			workerApplication:             components,
			inferenceProgressPublisherSet: true,
			inferenceProgressPublisher:    func(workerprovider.InferenceProgressFragment) {},
		})
		if err != nil {
			t.Fatalf("workerApplicationWithProgress() error = %v", err)
		}
		if !got.ProviderCommandInjected {
			t.Fatal("runtime progress runner was not installed on the composed provider factory")
		}
	})

	t.Run("functional provider edge is not replaced", func(t *testing.T) {
		runner := &providerCommandRunnerRecorder{}
		components, err := workerapplication.New(zap.NewNop(), workerapplication.Edges{
			ProviderCommandRunner: runner,
			AgyPTYAllocator:       &agypty.MockAllocator{},
		})
		if err != nil {
			t.Fatalf("construct functional worker application: %v", err)
		}
		got, err := workerApplicationWithProgress(runtimeBundleBuildInput{
			workerApplication:             components,
			inferenceProgressPublisherSet: true,
			inferenceProgressPublisher:    func(workerprovider.InferenceProgressFragment) {},
		})
		if err != nil {
			t.Fatalf("workerApplicationWithProgress() error = %v", err)
		}
		if got.Provider != components.Provider {
			t.Fatal("runtime progress setup replaced the composition-selected provider factory")
		}
	})
}
