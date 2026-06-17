package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func TestLoadedFactoryConfig_AccessorsAndMutations(t *testing.T) {
	t.Parallel()

	factoryCfg := &interfaces.FactoryConfig{
		Workers: []interfaces.WorkerConfig{{
			Name:    "executor",
			Command: "before",
		}},
		Workstations: []interfaces.FactoryWorkstationConfig{{
			Name:           "implement",
			WorkerTypeName: "executor",
		}},
	}

	loaded, err := NewLoadedFactoryConfig("/tmp/factory", factoryCfg, nil)
	if err != nil {
		t.Fatalf("NewLoadedFactoryConfig: %v", err)
	}

	if got := loaded.FactoryDir(); got != "/tmp/factory" {
		t.Fatalf("FactoryDir() = %q, want /tmp/factory", got)
	}
	if got := loaded.RuntimeBaseDir(); got != "/tmp/factory" {
		t.Fatalf("RuntimeBaseDir() = %q, want /tmp/factory", got)
	}

	loaded.SetRuntimeBaseDir(" ./runtime/../custom-runtime ")
	if got := loaded.RuntimeBaseDir(); got != "custom-runtime" {
		t.Fatalf("RuntimeBaseDir() after SetRuntimeBaseDir = %q, want custom-runtime", got)
	}
	loaded.SetRuntimeBaseDir("   ")
	if got := loaded.RuntimeBaseDir(); got != "/tmp/factory" {
		t.Fatalf("RuntimeBaseDir() after clearing override = %q, want /tmp/factory", got)
	}

	loaded.portableBundledReplacements = []PortableBundledFileReplacement{{
		TargetPath: "factory/docs/guide.md",
	}}
	replacements := loaded.PortableBundledFileReplacements()
	replacements[0].TargetPath = "changed"
	if loaded.portableBundledReplacements[0].TargetPath != "factory/docs/guide.md" {
		t.Fatal("PortableBundledFileReplacements() should return a clone")
	}

	if got := loaded.FactoryConfig(); got == nil || len(got.Workers) != 1 {
		t.Fatalf("FactoryConfig() = %#v, want one worker", got)
	}
	if got := loaded.WorkstationConfigs(); len(got) != 1 {
		t.Fatalf("WorkstationConfigs() len = %d, want 1", len(got))
	}

	worker, ok := loaded.Worker("executor")
	if !ok || worker == nil {
		t.Fatal("Worker(executor) should succeed")
	}
	workstation, ok := loaded.Workstation("implement")
	if !ok || workstation == nil {
		t.Fatal("Workstation(implement) should succeed")
	}

	if err := loaded.MutateWorkers(func(worker *interfaces.WorkerConfig) error {
		worker.Command = "after"
		return nil
	}); err != nil {
		t.Fatalf("MutateWorkers: %v", err)
	}

	if got := loaded.FactoryConfig().Workers[0].Command; got != "after" {
		t.Fatalf("factory worker command = %q, want after", got)
	}
	worker, ok = loaded.Worker("executor")
	if !ok || worker.Command != "after" {
		t.Fatalf("lookup worker = %#v, want updated command", worker)
	}
}

func TestLoadedFactoryConfig_MutateWorkersWrapsLookupErrors(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("boom")
	loaded := &LoadedFactoryConfig{
		factory: &interfaces.FactoryConfig{},
		lookup: &runtimeDefinitionLookupMaps{
			workers: map[string]*interfaces.WorkerConfig{
				"executor": {Name: "executor"},
			},
		},
	}

	err := loaded.MutateWorkers(func(worker *interfaces.WorkerConfig) error {
		return wantErr
	})
	if err == nil {
		t.Fatal("expected MutateWorkers to fail")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want wrapped %v", err, wantErr)
	}
	if !strings.Contains(err.Error(), `worker "executor"`) {
		t.Fatalf("error = %q, want worker context", err.Error())
	}
}

func TestRuntimeHelpers_NormalizeAndMerge(t *testing.T) {
	t.Parallel()

	if hasInlineRuntimeDefinitions(nil) {
		t.Fatal("hasInlineRuntimeDefinitions(nil) should be false")
	}
	if workerHasInlineRuntimeDefinitionFields(interfaces.WorkerConfig{Name: "executor"}) {
		t.Fatal("workerHasInlineRuntimeDefinitionFields should ignore empty workers")
	}
	if !workerHasInlineRuntimeDefinitionFields(interfaces.WorkerConfig{Name: "executor", Args: []string{"run"}}) {
		t.Fatal("workerHasInlineRuntimeDefinitionFields should detect runtime args")
	}
	if workstationHasInlineRuntimeDefinitionFields(interfaces.FactoryWorkstationConfig{
		Type: interfaces.WorkstationTypeLogical,
	}) {
		t.Fatal("topology-only logical workstation should not count as inline runtime")
	}
	if !workstationHasInlineRuntimeDefinitionFields(interfaces.FactoryWorkstationConfig{
		Type: interfaces.WorkstationTypeModel,
		Env:  map[string]string{"MODE": "test"},
	}) {
		t.Fatal("runtime workstation env should count as inline runtime")
	}

	inlineRuntime, err := workstationRuntimeDefinitionFromInline(interfaces.FactoryWorkstationConfig{
		Name:           "implement",
		WorkerTypeName: "executor",
		Body:           "Do the thing.",
		Timeout:        "45s",
	})
	if err != nil {
		t.Fatalf("workstationRuntimeDefinitionFromInline: %v", err)
	}
	if inlineRuntime == nil {
		t.Fatal("expected inline runtime definition")
	}
	if inlineRuntime.Type != interfaces.WorkstationTypeModel {
		t.Fatalf("inline runtime type = %q, want %q", inlineRuntime.Type, interfaces.WorkstationTypeModel)
	}
	if inlineRuntime.PromptTemplate != "Do the thing." {
		t.Fatalf("inline prompt template = %q, want body copy", inlineRuntime.PromptTemplate)
	}
	if inlineRuntime.Limits.MaxExecutionTime != "45s" || inlineRuntime.Timeout != "" {
		t.Fatalf("inline runtime timeout normalization = %#v", inlineRuntime.Limits)
	}

	canonical := &interfaces.FactoryWorkstationConfig{
		Type:    interfaces.WorkstationTypePoller,
		Timeout: "30s",
	}
	NormalizeCanonicalWorkstationRuntime(canonical)
	if canonical.Kind != interfaces.WorkstationKindPoller {
		t.Fatalf("Kind = %q, want %q", canonical.Kind, interfaces.WorkstationKindPoller)
	}
	if canonical.Limits.MaxExecutionTime != "30s" || canonical.Timeout != "" {
		t.Fatalf("normalized timeout fields = %#v", canonical)
	}

	if got := defaultWorkstationRuntimeType(""); got != interfaces.WorkstationTypeLogical {
		t.Fatalf("defaultWorkstationRuntimeType(\"\") = %q, want logical", got)
	}
	if got := defaultWorkstationRuntimeType("executor"); got != interfaces.WorkstationTypeModel {
		t.Fatalf("defaultWorkstationRuntimeType(non-empty) = %q, want model", got)
	}

	stopWords := mergeStopWords([]string{"END", "STOP"}, []string{"STOP", "DONE"})
	if want := []string{"END", "STOP", "DONE"}; strings.Join(stopWords, ",") != strings.Join(want, ",") {
		t.Fatalf("mergeStopWords() = %#v, want %#v", stopWords, want)
	}
	if got := firstNonEmpty("  ", "", "runner"); got != "runner" {
		t.Fatalf("firstNonEmpty() = %q, want runner", got)
	}

	cloned := cloneStringMap(map[string]string{"A": "1"})
	cloned["A"] = "2"
	if got := mergeStringMap(map[string]string{"A": "1"}, map[string]string{"B": "2"}); got["A"] != "1" || got["B"] != "2" {
		t.Fatalf("mergeStringMap() = %#v, want merged values", got)
	}
}

func TestWorkstationExecutionTimeout(t *testing.T) {
	t.Parallel()

	timeout, err := WorkstationExecutionTimeout(&interfaces.FactoryWorkstationConfig{
		Limits: interfaces.WorkstationLimits{MaxExecutionTime: "90s"},
	})
	if err != nil {
		t.Fatalf("WorkstationExecutionTimeout(valid): %v", err)
	}
	if timeout != 90*time.Second {
		t.Fatalf("timeout = %v, want 90s", timeout)
	}

	timeout, err = WorkstationExecutionTimeout(&interfaces.FactoryWorkstationConfig{
		Limits: interfaces.WorkstationLimits{MaxExecutionTime: "0s"},
	})
	if err != nil {
		t.Fatalf("WorkstationExecutionTimeout(zero): %v", err)
	}
	if timeout != 0 {
		t.Fatalf("timeout = %v, want 0", timeout)
	}

	_, err = WorkstationExecutionTimeout(&interfaces.FactoryWorkstationConfig{
		Limits: interfaces.WorkstationLimits{MaxExecutionTime: "not-a-duration"},
	})
	if err == nil {
		t.Fatal("expected invalid duration to fail")
	}
	if !strings.Contains(err.Error(), "limits.maxExecutionTime") {
		t.Fatalf("error = %q, want limits.maxExecutionTime context", err.Error())
	}
}

func TestFilesystemHelpers_ResolvePortableBundledCopyTargetAndAgentsFileExists(t *testing.T) {
	t.Parallel()

	targetDir := t.TempDir()
	validationRoot, err := preparePortableBundledValidationRoot(targetDir)
	if err != nil {
		t.Fatalf("preparePortableBundledValidationRoot: %v", err)
	}

	missingSource := filepath.Join(t.TempDir(), "missing.py")
	target, shouldCopy, err := resolvePortableBundledCopyTarget(validationRoot, "factory/scripts/setup.py", missingSource)
	if err != nil {
		t.Fatalf("resolvePortableBundledCopyTarget(missing): %v", err)
	}
	if shouldCopy || target.path != "" {
		t.Fatalf("missing source result = %#v, %v, want skip", target, shouldCopy)
	}

	sourceDir := t.TempDir()
	target, shouldCopy, err = resolvePortableBundledCopyTarget(validationRoot, "factory/scripts/setup.py", sourceDir)
	if err != nil {
		t.Fatalf("resolvePortableBundledCopyTarget(directory): %v", err)
	}
	if shouldCopy || target.path != "" {
		t.Fatalf("directory source result = %#v, %v, want skip", target, shouldCopy)
	}

	sourceFile := filepath.Join(t.TempDir(), "setup.py")
	if err := os.WriteFile(sourceFile, []byte("print('ok')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(sourceFile): %v", err)
	}
	target, shouldCopy, err = resolvePortableBundledCopyTarget(validationRoot, "factory/scripts/setup.py", sourceFile)
	if err != nil {
		t.Fatalf("resolvePortableBundledCopyTarget(valid): %v", err)
	}
	if !shouldCopy {
		t.Fatal("expected regular file source to be copied")
	}
	if want := filepath.Join(targetDir, "scripts", "setup.py"); target.path != want {
		t.Fatalf("target.path = %q, want %q", target.path, want)
	}

	_, _, err = resolvePortableBundledCopyTarget(validationRoot, "../outside.py", sourceFile)
	if err == nil {
		t.Fatal("expected unsafe target path to fail")
	}

	workerDir := filepath.Join(t.TempDir(), "worker")
	exists, err := agentsFileExists(workerDir)
	if err != nil {
		t.Fatalf("agentsFileExists(missing): %v", err)
	}
	if exists {
		t.Fatal("agentsFileExists should be false before AGENTS.md exists")
	}
	if err := os.MkdirAll(workerDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workerDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerDir, interfaces.FactoryAgentsFileName), []byte("---\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(AGENTS.md): %v", err)
	}
	exists, err = agentsFileExists(workerDir)
	if err != nil {
		t.Fatalf("agentsFileExists(existing): %v", err)
	}
	if !exists {
		t.Fatal("agentsFileExists should be true after AGENTS.md exists")
	}
}

func TestNamedFactoryHelpers_ValidateAndResolvePreparedPayload(t *testing.T) {
	t.Parallel()

	if err := ValidateNamedFactoryName("alpha"); err != nil {
		t.Fatalf("ValidateNamedFactoryName(alpha): %v", err)
	}
	if err := ValidateNamedFactoryName("@you/tts"); err != nil {
		t.Fatalf("ValidateNamedFactoryName(@you/tts): %v", err)
	}
	if err := ValidateNamedFactoryName("@broken"); err == nil {
		t.Fatal("expected invalid scoped name to fail")
	}

	prepared := &PreparedFactoryLayoutPayload{
		Config:    &interfaces.FactoryConfig{Name: "alpha"},
		Canonical: []byte(`{"name":"alpha"}`),
	}
	cfg, canonical, err := resolveNamedFactoryPersistPayload("alpha", nil, prepared)
	if err != nil {
		t.Fatalf("resolveNamedFactoryPersistPayload(prepared): %v", err)
	}
	if cfg != prepared.Config || string(canonical) != string(prepared.Canonical) {
		t.Fatalf("resolved prepared payload = %#v %q", cfg, string(canonical))
	}

	_, _, err = resolveNamedFactoryPersistPayload("alpha", nil, nil)
	if err == nil {
		t.Fatal("expected missing payload to fail")
	}
}
