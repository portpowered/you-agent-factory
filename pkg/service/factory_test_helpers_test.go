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

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/logging"
	"github.com/portpowered/infinite-you/pkg/testutil/runtimefixtures"
	"github.com/portpowered/infinite-you/pkg/workers"
)

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
	if got == nil || want == nil || got.Logical != want.Logical || !got.Physical.Equal(want.Physical) {
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
