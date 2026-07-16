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

func TestLoadRuntimeConfigRejectsRetiredGeneratedFactoryFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	factoryJSON := []byte(`{
		"workTypes": [{"name":"task","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"workers": [{"name":"worker-a"}],
		"workstations": [{
			"name":"step-one",
			"worker":"worker-a",
			"inputs":[{"workType":"task","state":"init"}],
			"outputs":[{"workType":"task","state":"complete"}],
			"join":{"waitFor":"task","waitState":"complete","require":"all"}
		}]
	}`)
	if err := os.WriteFile(filepath.Join(dir, interfaces.FactoryConfigFile), factoryJSON, 0o600); err != nil {
		t.Fatalf("write factory.json: %v", err)
	}

	_, err := LoadRuntimeConfig(dir, nil)
	assertRetiredGeneratedFactoryFieldFailure(t, "LoadRuntimeConfig", err)
}

func assertRetiredGeneratedFactoryFieldFailure(t *testing.T, operation string, err error) {
	t.Helper()

	if err == nil {
		t.Fatalf("%s() error = nil, want retired generated field failure", operation)
	}
	for _, snippet := range []string{"is not supported", "use "} {
		if !strings.Contains(err.Error(), snippet) {
			t.Fatalf("%s() error = %q, want substring %q", operation, err, snippet)
		}
	}
}

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

	t.Run("directory accessors and replacements", func(t *testing.T) {
		testLoadedFactoryConfigAccessorsAndReplacements(t, loaded)
	})
	t.Run("config lookups and worker mutation", func(t *testing.T) {
		testLoadedFactoryConfigLookupsAndMutation(t, loaded)
	})
}

func testLoadedFactoryConfigAccessorsAndReplacements(t *testing.T, loaded *LoadedFactoryConfig) {
	t.Helper()

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
}

func testLoadedFactoryConfigLookupsAndMutation(t *testing.T, loaded *LoadedFactoryConfig) {
	t.Helper()

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

	t.Run("inline runtime detection", testRuntimeHelperInlineDetection)
	t.Run("runtime normalization", testRuntimeHelperNormalization)
	t.Run("merge helpers", testRuntimeHelperMerges)
}

func testRuntimeHelperInlineDetection(t *testing.T) {
	t.Helper()

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
}

func testRuntimeHelperNormalization(t *testing.T) {
	t.Helper()

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
}

func testRuntimeHelperMerges(t *testing.T) {
	t.Helper()

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

	t.Run("portable bundled copy target", func(t *testing.T) {
		testResolvePortableBundledCopyTargetBranches(t, targetDir, validationRoot)
	})
	t.Run("agents file exists", testAgentsFileExistsBranches)
}

func testResolvePortableBundledCopyTargetBranches(t *testing.T, targetDir string, validationRoot portableBundledValidationRoot) {
	t.Helper()

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
}

func testAgentsFileExistsBranches(t *testing.T) {
	t.Helper()

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
	if err := ValidateNamedFactoryName("@you/goal"); err != nil {
		t.Fatalf("ValidateNamedFactoryName(@you/goal): %v", err)
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

func TestRuleBundledFiles_AcceptsSupportedDiskBackedScriptAndDocWithoutInline(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{
			{
				Type:       interfaces.BundledFileTypeScript,
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
			{
				Type:       interfaces.BundledFileTypeDoc,
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
		},
	}

	findings := ruleBundledFiles(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no bundled-file findings, got %#v", findings)
	}
}

func TestValidatePortableResourceManifestOnPath_AcceptsSupportedDiskBackedBundledFilesWithoutInline(t *testing.T) {
	factoryDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(factoryDir, "scripts"), 0o755); err != nil {
		t.Fatalf("MkdirAll(scripts): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(factoryDir, "docs"), 0o755); err != nil {
		t.Fatalf("MkdirAll(docs): %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "scripts", "setup-workspace.py"), []byte("print('portable')\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(script): %v", err)
	}
	if err := os.WriteFile(filepath.Join(factoryDir, "docs", "usage.md"), []byte("# Usage\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(doc): %v", err)
	}

	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{
			{
				Type:       interfaces.BundledFileTypeScript,
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
			{
				Type:       interfaces.BundledFileTypeDoc,
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
		},
	}

	if err := validatePortableResourceManifestOnPath(factoryDir, cfg); err != nil {
		t.Fatalf("validatePortableResourceManifestOnPath: %v", err)
	}
}

func TestConfigValidator_ValidateAcceptsSupportedDiskBackedBundledFilesWithoutInline(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{
			{
				Type:       interfaces.BundledFileTypeScript,
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
			{
				Type:       interfaces.BundledFileTypeDoc,
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			},
		},
	}

	result := NewConfigValidator().Validate(cfg)
	if result.HasErrors() {
		t.Fatalf("expected config validator to accept supported disk-backed bundled files without inline content, got %#v", result.Errors())
	}
}

func TestConfigValidator_BundledFilesAcceptCanonicalScriptAndDocTargets(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{
			{
				Type:       "SCRIPT",
				TargetPath: "factory/scripts/setup-workspace.py",
				Content: interfaces.BundledFileContentConfig{
					Encoding: "utf-8",
					Inline:   "print('portable')\n",
				},
			},
			{
				Type:       "DOC",
				TargetPath: "factory/docs/usage.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: "utf-8",
					Inline:   "# Usage\n",
				},
			},
			{
				Type:       interfaces.BundledFileTypeRootHelper,
				TargetPath: "Makefile",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
					Inline:   "test:\n\tgo test ./...\n",
				},
			},
		},
	}

	findings := ruleBundledFiles(cfg)
	if len(findings) != 0 {
		t.Fatalf("expected no bundled-file findings, got %#v", findings)
	}
}

func TestRuleBundledFiles_RejectsTargetOutsideCanonicalRootForType(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       "DOC",
			TargetPath: "factory/scripts/usage.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: "utf-8",
				Inline:   "# Usage\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-root")
}

func TestRuleBundledFiles_RejectsUnsupportedInputTargetShape(t *testing.T) {
	cfg := testBaseConfig()
	cfg.ResourceManifest = &interfaces.PortableResourceManifestConfig{
		BundledFiles: []interfaces.BundledFileConfig{{
			Type:       interfaces.BundledFileTypeInput,
			TargetPath: "factory/inputs/task/default/nested/starter.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "starter work\n",
			},
		}},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-root")
	if !strings.Contains(findings[0].Message, "factory/inputs/<work-type>/<channel>/<file>") {
		t.Fatalf("expected INPUT shape guidance, got %#v", findings[0])
	}
}

func TestRuleBundledFiles_RejectsDuplicateTargetPath(t *testing.T) {
	cfg := &interfaces.FactoryConfig{
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			BundledFiles: []interfaces.BundledFileConfig{
				{
					Type:       interfaces.BundledFileTypeDoc,
					TargetPath: "factory/docs/overview.md",
					Content: interfaces.BundledFileContentConfig{
						Encoding: interfaces.BundledFileEncodingUTF8,
						Inline:   "first",
					},
				},
				{
					Type:       interfaces.BundledFileTypeDoc,
					TargetPath: "factory/docs/overview.md",
					Content: interfaces.BundledFileContentConfig{
						Encoding: interfaces.BundledFileEncodingUTF8,
						Inline:   "second",
					},
				},
			},
		},
	}

	findings := ruleBundledFiles(cfg)
	assertFindingExists(t, findings, "bundled-file-target-duplicate")
}

func TestMergePortableBundledFiles_ManifestAuthoritativeDocsSkipsUnlistedDiskDocs(t *testing.T) {
	existing := []interfaces.BundledFileConfig{{
		Type:       interfaces.BundledFileTypeDoc,
		TargetPath: "factory/docs/listed.md",
		Content: interfaces.BundledFileContentConfig{
			Encoding: interfaces.BundledFileEncodingUTF8,
		},
	}}
	collected := []interfaces.BundledFileConfig{
		{
			Type:       interfaces.BundledFileTypeDoc,
			TargetPath: "factory/docs/listed.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "listed content",
			},
		},
		{
			Type:       interfaces.BundledFileTypeDoc,
			TargetPath: "factory/docs/orphan.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "orphan content",
			},
		},
	}

	merged := mergePortableBundledFiles(existing, collected, false)
	if len(merged) != 1 {
		t.Fatalf("merged bundled files = %#v, want one listed doc", merged)
	}
	if merged[0].TargetPath != "factory/docs/listed.md" {
		t.Fatalf("merged doc target = %q, want factory/docs/listed.md", merged[0].TargetPath)
	}
	if merged[0].Content.Inline != "listed content" {
		t.Fatalf("merged doc inline = %q, want listed content", merged[0].Content.Inline)
	}
}

func TestMergePortableBundledFiles_ManifestAuthoritativeDocsIncludesNestedUnlistedDiskDocs(t *testing.T) {
	existing := []interfaces.BundledFileConfig{{
		Type:       interfaces.BundledFileTypeDoc,
		TargetPath: "factory/docs/README.md",
		Content: interfaces.BundledFileContentConfig{
			Encoding: interfaces.BundledFileEncodingUTF8,
		},
	}}
	collected := []interfaces.BundledFileConfig{
		{
			Type:       interfaces.BundledFileTypeDoc,
			TargetPath: "factory/docs/README.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "# Factory docs\n",
			},
		},
		{
			Type:       interfaces.BundledFileTypeDoc,
			TargetPath: "factory/docs/standards/review.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "# Review standards\n",
			},
		},
		{
			Type:       interfaces.BundledFileTypeDoc,
			TargetPath: "factory/docs/orphan.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "orphan content",
			},
		},
	}

	merged := mergePortableBundledFiles(existing, collected, false)
	if len(merged) != 2 {
		t.Fatalf("merged bundled files = %#v, want listed and nested docs only", merged)
	}
	assertPortableBundledDocsMergeTarget(t, merged, "factory/docs/README.md", "# Factory docs\n")
	assertPortableBundledDocsMergeTarget(t, merged, "factory/docs/standards/review.md", "# Review standards\n")
}

func TestMergePortableBundledFiles_DiscoverUnlistedDocsAddsDiskOnlyDocs(t *testing.T) {
	existing := []interfaces.BundledFileConfig{{
		Type:       interfaces.BundledFileTypeDoc,
		TargetPath: "factory/docs/listed.md",
		Content: interfaces.BundledFileContentConfig{
			Encoding: interfaces.BundledFileEncodingUTF8,
		},
	}}
	collected := []interfaces.BundledFileConfig{
		{
			Type:       interfaces.BundledFileTypeDoc,
			TargetPath: "factory/docs/listed.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "listed content",
			},
		},
		{
			Type:       interfaces.BundledFileTypeDoc,
			TargetPath: "factory/docs/orphan.md",
			Content: interfaces.BundledFileContentConfig{
				Encoding: interfaces.BundledFileEncodingUTF8,
				Inline:   "orphan content",
			},
		},
	}

	merged := mergePortableBundledFiles(existing, collected, true)
	if len(merged) != 2 {
		t.Fatalf("merged bundled files = %#v, want listed and orphan docs", merged)
	}
}

func TestPruneRemovedPortableBundledDocs_RemovesDocsMissingFromManifest(t *testing.T) {
	factoryDir := filepath.Join(t.TempDir(), "factory")
	docsDir := filepath.Join(factoryDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", docsDir, err)
	}
	writePortableBundledDocsTestFile(t, filepath.Join(docsDir, "keep.md"), "keep")
	writePortableBundledDocsTestFile(t, filepath.Join(docsDir, "remove.md"), "remove")

	cfg := &interfaces.FactoryConfig{
		ResourceManifest: &interfaces.PortableResourceManifestConfig{
			BundledFiles: []interfaces.BundledFileConfig{{
				Type:       interfaces.BundledFileTypeDoc,
				TargetPath: "factory/docs/keep.md",
				Content: interfaces.BundledFileContentConfig{
					Encoding: interfaces.BundledFileEncodingUTF8,
				},
			}},
		},
	}
	if err := pruneRemovedPortableBundledDocs(factoryDir, cfg); err != nil {
		t.Fatalf("pruneRemovedPortableBundledDocs: %v", err)
	}
	assertPortableBundledDocsTestFile(t, filepath.Join(docsDir, "keep.md"), "keep")
	if _, err := os.Stat(filepath.Join(docsDir, "remove.md")); !os.IsNotExist(err) {
		t.Fatalf("removed doc stat error = %v, want not exist", err)
	}
}

func writePortableBundledDocsTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
}

func assertPortableBundledDocsTestFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("file %s = %q, want %q", path, string(data), want)
	}
}

func assertPortableBundledDocsMergeTarget(t *testing.T, merged []interfaces.BundledFileConfig, targetPath, wantInline string) {
	t.Helper()

	for _, bundledFile := range merged {
		if bundledFile.TargetPath != targetPath {
			continue
		}
		if bundledFile.Content.Inline != wantInline {
			t.Fatalf("merged doc %q inline = %q, want %q", targetPath, bundledFile.Content.Inline, wantInline)
		}
		return
	}
	t.Fatalf("merged bundled files missing target %q: %#v", targetPath, merged)
}
