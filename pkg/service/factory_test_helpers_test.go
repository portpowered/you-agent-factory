// pkgmaintcheck:ignore-file-lines shared service test fixtures and helpers remain together until dedicated service test seams split.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/testutil/factoryfixtures"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/workers"
)

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
	svc.setStartupBundle(bundle)
}

type recordingDiagnosticsProvider struct{}

func (recordingDiagnosticsProvider) Infer(_ context.Context, _ interfaces.ProviderInferenceRequest) (interfaces.InferenceResponse, error) {
	return interfaces.InferenceResponse{
		Content: "Done. COMPLETE",
		Diagnostics: &interfaces.WorkDiagnostics{
			Provider: &interfaces.ProviderDiagnostic{
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
	if bundledFile.Content.Encoding != factoryapi.Utf8 {
		t.Fatalf("bundled file %q encoding = %q, want %q", wantPath, bundledFile.Content.Encoding, factoryapi.Utf8)
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
	if bundledFile.Content.Encoding != factoryapi.Utf8 {
		t.Fatalf("bundled file %q encoding = %q, want %q", wantPath, bundledFile.Content.Encoding, factoryapi.Utf8)
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
	agentsMD := "---\ntype: MODEL_WORKER\nmodel: claude-3-5-haiku-20241022\n---\nYou are a helpful assistant.\n"
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
	workers map[string]*interfaces.WorkerConfig,
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
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	_, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), "", cfg, logging.NoopLogger{}, false, nil, nil, nil, nil, nil, nil, nil, nil, nil)
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
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	if _, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), interfaces.RunnerIDGemini, cfg, logging.NoopLogger{}, false, nil, &workers.MockWorkerCommandRunner{}, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("loadWorkersFromConfig error = %v, want available gemini runner", err)
	}
}

func TestLoadWorkersFromConfig_AcceptsAvailableKiroFactoryRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	if _, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), interfaces.RunnerIDKiro, cfg, logging.NoopLogger{}, false, nil, &workers.MockWorkerCommandRunner{}, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("loadWorkersFromConfig error = %v, want available kiro runner", err)
	}
}

func TestLoadWorkersFromConfig_AcceptsAvailableCursorFactoryRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	if _, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), interfaces.RunnerIDCursorCLI, cfg, logging.NoopLogger{}, false, nil, &workers.MockWorkerCommandRunner{}, nil, nil, nil, nil, nil, nil, nil); err != nil {
		t.Fatalf("loadWorkersFromConfig error = %v, want available cursor runner", err)
	}
}

func TestLoadWorkersFromConfig_AcceptsAvailableOpenCodeFactoryRunner(t *testing.T) {
	dir := t.TempDir()

	writeWorkerAgentsMD(t, dir, "worker-a")
	writeWorkstationAgentsMD(t, dir, "review")

	cfg := newLoadedFactoryConfigForServiceTest(t, dir, &interfaces.FactoryConfig{
		Workstations: []interfaces.FactoryWorkstationConfig{{Name: "review"}},
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	if _, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), interfaces.RunnerIDOpenCode, cfg, logging.NoopLogger{}, false, nil, &workers.MockWorkerCommandRunner{}, nil, nil, nil, nil, nil, nil, nil); err != nil {
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
		Workers:      []interfaces.WorkerConfig{{Name: "worker-a"}},
	},
		map[string]*interfaces.WorkerConfig{
			"worker-a": mustLoadWorkerConfig(t, filepath.Join(dir, "workers", "worker-a")),
		},
		map[string]*interfaces.FactoryWorkstationConfig{
			"review": mustLoadWorkstationConfig(t, filepath.Join(dir, "workstations", "review")),
		},
	)

	_, err := loadWorkersFromConfig(cfg.FactoryDir(), cfg.FactoryConfig(), "mystery-runner", cfg, logging.NoopLogger{}, false, nil, nil, nil, nil, nil, nil, nil, nil, nil)
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
	runtimeCfg := configFixtureWithWorkerAndWorkstation("worker-a", "review", &interfaces.WorkerConfig{
		Name:          "worker-a",
		ModelProvider: interfaces.RunnerIDCodex,
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
	runtimeCfg := configFixtureWithWorkerAndWorkstation("worker-a", "review", &interfaces.WorkerConfig{
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
		workers: map[string]*interfaces.WorkerConfig{
			"worker-a": {
				Name:          "worker-a",
				ModelProvider: interfaces.RunnerIDCodex,
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
	runtimeCfg := configFixtureWithWorkerAndWorkstation("worker-a", "review", &interfaces.WorkerConfig{
		Name:          "worker-a",
		ModelProvider: "claude",
	})

	err := validateConfiguredWorkstationRunners(cfg, "mystery-runner", runtimeCfg, runnerSelectionPreflight{skipCommandAvailability: true})
	if err == nil || !strings.Contains(err.Error(), `unknown runner "mystery-runner"`) {
		t.Fatalf("validateConfiguredWorkstationRunners error = %v, want unknown factory runner", err)
	}
}

func TestEffectiveFactoryRunnerID_PrefersExplicitOverrideThenConfig(t *testing.T) {
	cfg := &interfaces.FactoryConfig{Runner: interfaces.RunnerIDGemini}

	if got := effectiveFactoryRunnerID("  cursor-cli  ", cfg); got != interfaces.RunnerIDCursorCLI {
		t.Fatalf("effectiveFactoryRunnerID override = %q, want %q", got, interfaces.RunnerIDCursorCLI)
	}
	if got := effectiveFactoryRunnerID("", cfg); got != interfaces.RunnerIDGemini {
		t.Fatalf("effectiveFactoryRunnerID config = %q, want %q", got, interfaces.RunnerIDGemini)
	}
	if got := effectiveFactoryRunnerID("", nil); got != "" {
		t.Fatalf("effectiveFactoryRunnerID nil config = %q, want empty", got)
	}
}

func configFixtureWithWorkerAndWorkstation(workerName, workstationName string, worker *interfaces.WorkerConfig) configRuntimeFixture {
	return configRuntimeFixture{
		workers: map[string]*interfaces.WorkerConfig{
			workerName: worker,
		},
		workstations: map[string]*interfaces.FactoryWorkstationConfig{
			workstationName: {Name: workstationName, WorkerTypeName: workerName},
		},
	}
}

type configRuntimeFixture struct {
	workers      map[string]*interfaces.WorkerConfig
	workstations map[string]*interfaces.FactoryWorkstationConfig
}

func (f configRuntimeFixture) Worker(name string) (*interfaces.WorkerConfig, bool) {
	worker, ok := f.workers[name]
	return worker, ok
}

func (f configRuntimeFixture) Workstation(name string) (*interfaces.FactoryWorkstationConfig, bool) {
	workstation, ok := f.workstations[name]
	return workstation, ok
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
func serviceReplayDispatchCreatedEvent(t *testing.T, dispatch interfaces.WorkDispatch, tick int) factoryapi.FactoryEvent {
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

func serviceReplayDispatchCompletedEvent(t *testing.T, completionID string, result interfaces.WorkResult, tick int) factoryapi.FactoryEvent {
	t.Helper()
	payload := factoryapi.DispatchResponseEventPayload{
		CompletionId:                serviceStringPtr(completionID),
		TransitionId:                result.TransitionID,
		Outcome:                     factoryapi.WorkOutcome(result.Outcome),
		Output:                      serviceStringPtr(result.Output),
		Error:                       serviceStringPtr(result.Error),
		Feedback:                    serviceStringPtr(result.Feedback),
		SelectedClassificationLabel: serviceStringPtr(result.SelectedClassificationLabel),
		ProviderFailure:             serviceProviderFailurePtr(result.ProviderFailure),
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
	for _, event := range artifact.Events {
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
	for _, event := range artifact.Events {
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
	for _, event := range artifact.Events {
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
	for _, event := range artifact.Events {
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

func serviceReplayWorksFromDispatch(dispatch interfaces.WorkDispatch) []factoryapi.Work {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	works := make([]factoryapi.Work, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == interfaces.DataTypeResource {
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

func serviceReplayDispatchInputRefsFromDispatch(dispatch interfaces.WorkDispatch) []factoryapi.DispatchConsumedWorkRef {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	refs := make([]factoryapi.DispatchConsumedWorkRef, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType == interfaces.DataTypeResource {
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

func serviceReplayResourcesFromDispatch(dispatch interfaces.WorkDispatch) *[]factoryapi.Resource {
	tokens := workers.WorkDispatchInputTokens(dispatch)
	resources := make([]factoryapi.Resource, 0, len(tokens))
	for _, token := range tokens {
		if token.Color.DataType != interfaces.DataTypeResource {
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

func serviceProviderFailurePtr(failure *interfaces.ProviderFailureMetadata) *factoryapi.ProviderFailureMetadata {
	return interfaces.GeneratedProviderFailureMetadata(failure)
}

func serviceWorkMetricsPtr(metrics interfaces.WorkMetrics) *factoryapi.WorkMetrics {
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
				"name": "executor",
				"type": "MODEL_WORKER",
				"body": "You are the executor.",
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
