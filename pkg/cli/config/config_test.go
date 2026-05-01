package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	factoryconfig "github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/interfaces"
)

// portos:func-length-exception owner=agent-factory reason=table-heavy-cli-config-round-trip-fixture review=2026-07-18 removal=split-fixture-setup-and-idempotency-assertions-before-next-cli-config-expand-change
func TestExpandFactoryConfig_CreatesDeterministicSplitLayout(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	writeCLITestFile(t, factoryPath, `{
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"resources": [{"name":"agent-slot","capacity":2}],
		"workers": [{
			"name":"executor",
				"type":"MODEL_WORKER",
				"model":"claude-sonnet-4-20250514",
				"modelProvider":"claude",
				"executorProvider":"script_wrap",
				"resources":[{"name":"agent-slot","capacity":1}],
				"timeout":"20m",
				"stopToken":"COMPLETE",
				"skipPermissions":true,
				"body":"You are the expanded executor."
		}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"resources":[{"name":"agent-slot","capacity":2}],
			"stopWords":["DONE"],
			"definition":{
				"type":"MODEL_WORKSTATION",
				"worker":"executor",
				"promptFile":"prompt.md",
				"outputSchema":"schema.json",
				"limits":{"maxRetries":2,"maxExecutionTime":"30m"},
				"stopWords":["DONE"],
				"body":"This body stays in AGENTS.md.",
				"promptTemplate":"Complete {{ .WorkID }} deterministically."
			}
		}]
	}`)

	var out bytes.Buffer
	if err := ExpandFactoryConfig(FactoryConfigExpandConfig{Path: factoryPath, Output: &out}); err != nil {
		t.Fatalf("ExpandFactoryConfig: %v", err)
	}
	if !strings.Contains(out.String(), "Expanded factory config into") {
		t.Fatalf("expected expand result output, got %q", out.String())
	}

	canonical := readCLITestFile(t, factoryPath)
	var payload map[string]any
	if err := json.Unmarshal(canonical, &payload); err != nil {
		t.Fatalf("expanded factory.json is not JSON: %v\n%s", err, string(canonical))
	}
	if _, ok := payload["workTypes"]; !ok {
		t.Fatalf("expected canonical workTypes key in expanded factory.json")
	}
	if _, ok := payload["work_types"]; ok {
		t.Fatalf("expected expanded factory.json not to include legacy work_types key")
	}

	workerAgentsPath := filepath.Join(dir, "workers", "executor", "AGENTS.md")
	workerAgents := readCLITestFile(t, workerAgentsPath)
	if !strings.Contains(string(workerAgents), "type: MODEL_WORKER") {
		t.Fatalf("expected worker AGENTS.md to be loadable MODEL_WORKER, got:\n%s", string(workerAgents))
	}
	assertExpandedAgentsFrontmatterUsesCamelCase(t, string(workerAgents), []string{
		"modelProvider: claude",
		"executorProvider: script_wrap",
		"stopToken: COMPLETE",
		"skipPermissions: true",
	}, []string{
		"model_provider:",
		"provider:",
		"concurrency:",
		"stop_token:",
		"skip_permissions:",
	})
	workerDef, err := factoryconfig.LoadWorkerConfig(filepath.Join(dir, "workers", "executor"))
	if err != nil {
		t.Fatalf("LoadWorkerConfig: %v", err)
	}
	if workerDef.Model != "claude-sonnet-4-20250514" || workerDef.ModelProvider != "claude" || workerDef.ExecutorProvider != "script_wrap" {
		t.Fatalf("expanded worker definition did not preserve model/provider fields: %#v", workerDef)
	}
	if workerDef.StopToken != "COMPLETE" || !workerDef.SkipPermissions || workerDef.Body != "You are the expanded executor." {
		t.Fatalf("expanded worker definition did not preserve behavior fields: %#v", workerDef)
	}

	workstationAgentsPath := filepath.Join(dir, "workstations", "execute-story", "AGENTS.md")
	workstationAgents := readCLITestFile(t, workstationAgentsPath)
	if !strings.Contains(string(workstationAgents), "type: MODEL_WORKSTATION") {
		t.Fatalf("expected workstation AGENTS.md to be loadable MODEL_WORKSTATION, got:\n%s", string(workstationAgents))
	}
	if !strings.Contains(string(workstationAgents), "worker: executor") {
		t.Fatalf("expected workstation AGENTS.md to reference executor, got:\n%s", string(workstationAgents))
	}
	assertExpandedAgentsFrontmatterUsesCamelCase(t, string(workstationAgents), []string{
		"promptFile: prompt.md",
		"outputSchema: schema.json",
		"maxRetries: 2",
		"maxExecutionTime: 30m",
		"stopWords:",
	}, []string{
		"prompt_file:",
		"output_schema:",
		"max_retries:",
		"max_execution_time:",
		"stop_words:",
	})
	workstationDef, err := factoryconfig.LoadWorkstationConfig(filepath.Join(dir, "workstations", "execute-story"))
	if err != nil {
		t.Fatalf("LoadWorkstationConfig: %v", err)
	}
	if workstationDef.OutputSchema != "schema.json" || workstationDef.Limits.MaxRetries != 2 || workstationDef.Limits.MaxExecutionTime != "30m" || workstationDef.Timeout != "" {
		t.Fatalf("expanded workstation definition did not preserve frontmatter: %#v", workstationDef)
	}
	if workstationDef.Body != "This body stays in AGENTS.md." {
		t.Fatalf("expanded workstation body = %q", workstationDef.Body)
	}
	if workstationDef.PromptTemplate != "Complete {{ .WorkID }} deterministically." {
		t.Fatalf("expanded workstation prompt template = %q", workstationDef.PromptTemplate)
	}
	promptContent := readCLITestFile(t, filepath.Join(dir, "workstations", "execute-story", "prompt.md"))
	if string(promptContent) != "Complete {{ .WorkID }} deterministically." {
		t.Fatalf("expanded prompt file content = %q", string(promptContent))
	}
	flattened, err := factoryconfig.FlattenFactoryConfig(dir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig(expanded split layout): %v", err)
	}
	flattenedCfg, err := factoryconfig.NewFactoryConfigMapper().Expand(flattened)
	if err != nil {
		t.Fatalf("flattened expanded layout should parse: %v", err)
	}
	flattenedWorkstation := flattenedCfg.Workstations[0]
	if flattenedWorkstation.PromptFile != "prompt.md" || flattenedWorkstation.PromptTemplate != "Complete {{ .WorkID }} deterministically." {
		t.Fatalf("flattened workstation prompt file/template = %q/%q", flattenedWorkstation.PromptFile, flattenedWorkstation.PromptTemplate)
	}
	if flattenedWorkstation.OutputSchema != "schema.json" || flattenedWorkstation.Limits.MaxRetries != 2 || flattenedWorkstation.Limits.MaxExecutionTime != "30m" || flattenedWorkstation.Timeout != "" {
		t.Fatalf("flattened workstation runtime fields = %#v", flattenedWorkstation)
	}

	loaded, err := factoryconfig.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("expanded layout should load through runtime config: %v", err)
	}
	if _, ok := loaded.Worker("executor"); !ok {
		t.Fatal("expected expanded worker definition to load")
	}
	if _, ok := loaded.Workstation("execute-story"); !ok {
		t.Fatal("expected expanded workstation definition to load")
	}

	before := map[string][]byte{
		"factory":     append([]byte(nil), canonical...),
		"worker":      append([]byte(nil), workerAgents...),
		"workstation": append([]byte(nil), workstationAgents...),
	}
	if err := ExpandFactoryConfig(FactoryConfigExpandConfig{Path: factoryPath, Output: io.Discard}); err != nil {
		t.Fatalf("ExpandFactoryConfig second run: %v", err)
	}
	after := map[string][]byte{
		"factory":     readCLITestFile(t, factoryPath),
		"worker":      readCLITestFile(t, workerAgentsPath),
		"workstation": readCLITestFile(t, workstationAgentsPath),
	}
	for name, beforeBytes := range before {
		if !bytes.Equal(beforeBytes, after[name]) {
			t.Fatalf("%s changed after idempotent expand\nbefore:\n%s\nafter:\n%s", name, string(beforeBytes), string(after[name]))
		}
	}
}

func TestExpandFactoryConfig_WritesPromptFileFromBodyWhenPromptTemplateMissing(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	writeCLITestFile(t, factoryPath, `{
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"resources": [],
		"workers": [{
			"name":"executor",
			"definition":{
				"type":"MODEL_WORKER",
				"model":"claude-sonnet-4-20250514",
				"body":"Execute the task."
			}
		}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}],
			"resources":[],
			"definition":{
				"type":"MODEL_WORKSTATION",
				"worker":"executor",
				"promptFile":"prompts/task.md",
				"body":"Use {{ .WorkID }} as the prompt."
			}
		}]
	}`)

	if err := ExpandFactoryConfig(FactoryConfigExpandConfig{Path: factoryPath, Output: io.Discard}); err != nil {
		t.Fatalf("ExpandFactoryConfig: %v", err)
	}

	promptPath := filepath.Join(dir, "workstations", "execute-story", "prompts", "task.md")
	promptContent := readCLITestFile(t, promptPath)
	if string(promptContent) != "Use {{ .WorkID }} as the prompt." {
		t.Fatalf("prompt file content = %q", string(promptContent))
	}

	workstationDef, err := factoryconfig.LoadWorkstationConfig(filepath.Join(dir, "workstations", "execute-story"))
	if err != nil {
		t.Fatalf("LoadWorkstationConfig: %v", err)
	}
	if workstationDef.PromptTemplate != "Use {{ .WorkID }} as the prompt." {
		t.Fatalf("expanded workstation prompt template = %q", workstationDef.PromptTemplate)
	}
}

func TestExpandFactoryConfig_PreservesPortableResourceManifestAndMaterializesBundledFiles(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	writePortableResourceManifestFactoryConfig(t, factoryPath, portableRequiredToolWithPurposeJSON("python"))

	targetDir, _, canonical := expandPortableResourceManifestFactory(t, factoryPath)
	assertPortableResourceManifestPayload(t, canonical, "python")
	assertFlattenedPortableResourceManifestPreserved(t, targetDir)
	assertPortableBundledFilesMaterialized(t, targetDir)
	assertLoadedPortableResourceManifest(t, targetDir)
}

func TestExpandFactoryConfig_PortableResourceManifestSmoke_ValidatesRequiredToolsAndPreservesManifestAcrossExpandAndFlatten(t *testing.T) {
	dir := t.TempDir()
	toolsDir := t.TempDir()
	presentCommand := writeCLIRequiredToolExecutable(t, toolsDir, "portable-helper")
	t.Setenv("PATH", toolsDir)

	factoryPath := filepath.Join(dir, "factory.json")
	writePortableResourceManifestFactoryConfig(t, factoryPath, portableRequiredToolsJSON(presentCommand)+`, {"name":"Missing helper","command":"missing-helper"}`)
	assertPortableResourceManifestMissingToolLoadFailure(t, dir)

	writePortableResourceManifestFactoryConfig(t, factoryPath, portableRequiredToolsJSON(presentCommand))
	targetDir, _, canonical := expandPortableResourceManifestFactory(t, factoryPath)
	assertPortableResourceManifestPayload(t, canonical, presentCommand)
	assertFlattenedPortableResourceManifestPreservedWithCommand(t, targetDir, presentCommand)
	assertPortableBundledFilesMaterialized(t, targetDir)
	assertLoadedPortableResourceManifest(t, targetDir)
}

func TestExpandFactoryConfig_PortableResourceManifestSmoke_PreservesContractAndRejectsMissingTools(t *testing.T) {
	dir := t.TempDir()
	toolsDir := t.TempDir()
	presentCommand := writeCLIRequiredToolExecutable(t, toolsDir, "portable-helper")
	t.Setenv("PATH", toolsDir)

	factoryPath := filepath.Join(dir, "factory.json")
	writePortableResourceManifestFactoryConfig(t, factoryPath, portableRequiredToolsJSON(presentCommand))

	targetDir, canonicalPath, canonical := expandPortableResourceManifestFactory(t, factoryPath)
	assertPortableResourceManifestPayload(t, canonical, presentCommand)
	assertFlattenedPortableResourceManifestPreserved(t, targetDir)
	assertPortableBundledFilesMaterialized(t, targetDir)
	assertLoadedPortableResourceManifest(t, targetDir)

	writePortableResourceManifestWithMissingTool(t, canonicalPath, canonical)
	assertPortableResourceManifestMissingToolLoadFailure(t, targetDir)
}

func TestExpandFactoryConfig_RejectsMissingRequiredToolFromPortableResourceManifest(t *testing.T) {
	dir := t.TempDir()
	factoryPath := filepath.Join(dir, "factory.json")
	writePortableResourceManifestFactoryConfig(t, factoryPath, `{"name":"Missing helper","command":"missing-helper"}`)

	if err := ExpandFactoryConfig(FactoryConfigExpandConfig{Path: factoryPath, Output: io.Discard}); err != nil {
		t.Fatalf("expected expand to preserve external required-tool declarations, got %v", err)
	}
	assertPortableBundledFilesMaterialized(t, dir)
	assertPortableResourceManifestMissingToolLoadFailureAtIndex(t, dir, 0)
}

func TestExpandFactoryConfig_RejectsInvalidBundledFileRootFromPortableResourceManifest(t *testing.T) {
	dir := t.TempDir()
	toolsDir := t.TempDir()
	presentCommand := writeCLIRequiredToolExecutable(t, toolsDir, "portable-helper")
	t.Setenv("PATH", toolsDir)

	factoryPath := filepath.Join(dir, "factory.json")
	writePortableResourceManifestFactoryConfigWithScriptTarget(t, factoryPath, portableRequiredToolWithPurposeJSON(presentCommand), "factory/docs/not-a-script.md")

	err := ExpandFactoryConfig(FactoryConfigExpandConfig{Path: factoryPath, Output: io.Discard})
	if err == nil {
		t.Fatal("expected expand to reject invalid bundled file root")
	}
	if !containsAll(err.Error(),
		"validation failed: 1 errors",
		"[bundled-file-target-root] resourceManifest.bundledFiles[0].targetPath",
		`must stay under "factory/scripts/" for SCRIPT bundled files`,
	) {
		t.Fatalf("expected bundled-file root validation failure, got %v", err)
	}
	assertExpandDidNotWriteSplitRuntimeFiles(t, dir)
}

func portableRequiredToolsJSON(command string) string {
	return `{
				"name":"Portable helper",
				"command":"` + command + `",
				"purpose":"Runs portable helper scripts",
				"versionArgs":["--version"]
			}`
}

func portableRequiredToolWithPurposeJSON(command string) string {
	return `{
				"name":"python",
				"command":"` + command + `",
				"purpose":"Runs portable helper scripts",
				"versionArgs":["--version"]
			}`
}

func writePortableResourceManifestFactoryConfig(t *testing.T, factoryPath string, requiredToolsJSON string) {
	t.Helper()

	writePortableResourceManifestFactoryConfigWithScriptTarget(t, factoryPath, requiredToolsJSON, "factory/scripts/setup-workspace.py")
}

func writePortableResourceManifestFactoryConfigWithScriptTarget(t *testing.T, factoryPath string, requiredToolsJSON, scriptTargetPath string) {
	t.Helper()

	writeCLITestFile(t, factoryPath, `{
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"supportingFiles": {
			"requiredTools": [`+requiredToolsJSON+`],
			"bundledFiles": [{
				"type":"SCRIPT",
				"targetPath":"`+scriptTargetPath+`",
				"content":{"encoding":"utf-8","inline":"print('portable')\n"}
			}, {
				"type":"ROOT_HELPER",
				"targetPath":"Makefile",
				"content":{"encoding":"utf-8","inline":"test:\n\tgo test ./...\n"}
			}, {
				"type":"DOC",
				"targetPath":"factory/docs/usage.md",
				"content":{"encoding":"utf-8","inline":"# Usage\n"}
			}]
		},
		"workers": [{
			"name":"executor",
			"type":"SCRIPT_WORKER",
			"command":"echo"
		}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)
}

func expandPortableResourceManifestFactory(t *testing.T, factoryPath string) (string, string, map[string]any) {
	t.Helper()

	var out bytes.Buffer
	if err := ExpandFactoryConfig(FactoryConfigExpandConfig{Path: factoryPath, Output: &out}); err != nil {
		t.Fatalf("ExpandFactoryConfig: %v", err)
	}
	targetDir := strings.TrimSpace(strings.TrimPrefix(out.String(), "Expanded factory config into "))
	if targetDir == "" {
		t.Fatalf("expected expand output to include target directory, got %q", out.String())
	}

	canonicalPath := filepath.Join(targetDir, "factory.json")
	return targetDir, canonicalPath, mustReadCanonicalFactoryPayload(t, canonicalPath)
}

func assertPortableResourceManifestPayload(t *testing.T, canonical map[string]any, wantCommand string) {
	t.Helper()

	resourceManifest, ok := canonical["supportingFiles"].(map[string]any)
	if !ok {
		t.Fatalf("expected expanded canonical factory.json to preserve supportingFiles, got %#v", canonical["supportingFiles"])
	}
	requiredTools, ok := resourceManifest["requiredTools"].([]any)
	if !ok || len(requiredTools) != 1 {
		t.Fatalf("requiredTools = %#v, want one entry", resourceManifest["requiredTools"])
	}
	if got := requiredTools[0].(map[string]any)["command"]; got != wantCommand {
		t.Fatalf("required tool command = %#v, want %q", got, wantCommand)
	}
	assertPortableBundledFileTargets(t, resourceManifest)
}

func assertPortableBundledFileTargets(t *testing.T, resourceManifest map[string]any) {
	t.Helper()

	bundledFiles, ok := resourceManifest["bundledFiles"].([]any)
	if !ok || len(bundledFiles) != 3 {
		t.Fatalf("bundledFiles = %#v, want three entries", resourceManifest["bundledFiles"])
	}
	if got := bundledFiles[0].(map[string]any)["targetPath"]; got != "Makefile" {
		t.Fatalf("bundled root helper targetPath = %#v", got)
	}
	if got := bundledFiles[1].(map[string]any)["targetPath"]; got != "factory/docs/usage.md" {
		t.Fatalf("bundled doc targetPath = %#v", got)
	}
	if got := bundledFiles[2].(map[string]any)["targetPath"]; got != "factory/scripts/setup-workspace.py" {
		t.Fatalf("bundled script targetPath = %#v", got)
	}
}

func assertFlattenedPortableResourceManifestPreserved(t *testing.T, targetDir string) {
	t.Helper()

	flattened, err := factoryconfig.FlattenFactoryConfig(targetDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig(expanded split layout): %v", err)
	}
	flattenedPayload := mustDecodeCanonicalFactoryPayload(t, flattened)
	if _, ok := flattenedPayload["supportingFiles"].(map[string]any); !ok {
		t.Fatalf("expected flattened expanded layout to preserve supportingFiles, got %#v", flattenedPayload["supportingFiles"])
	}
}

func assertFlattenedPortableResourceManifestPreservedWithCommand(t *testing.T, targetDir string, wantCommand string) {
	t.Helper()

	flattened, err := factoryconfig.FlattenFactoryConfig(targetDir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig(expanded split layout): %v", err)
	}
	flattenedPayload := mustDecodeCanonicalFactoryPayload(t, flattened)
	flattenedManifest, ok := flattenedPayload["supportingFiles"].(map[string]any)
	if !ok {
		t.Fatalf("expected flattened expanded layout to preserve supportingFiles, got %#v", flattenedPayload["supportingFiles"])
	}
	requiredTools, ok := flattenedManifest["requiredTools"].([]any)
	if !ok || len(requiredTools) != 1 {
		t.Fatalf("flattened requiredTools = %#v, want one entry", flattenedManifest["requiredTools"])
	}
	if got := requiredTools[0].(map[string]any)["command"]; got != wantCommand {
		t.Fatalf("flattened required tool command = %#v, want %q", got, wantCommand)
	}
}

func assertPortableBundledFilesMaterialized(t *testing.T, targetDir string) {
	t.Helper()

	for _, file := range []struct {
		path    string
		content string
	}{
		{
			path:    filepath.Join(targetDir, "Makefile"),
			content: "test:\n\tgo test ./...\n",
		},
		{
			path:    filepath.Join(targetDir, "docs", "usage.md"),
			content: "# Usage\n",
		},
		{
			path:    filepath.Join(targetDir, "scripts", "setup-workspace.py"),
			content: "print('portable')\n",
		},
	} {
		got, err := os.ReadFile(file.path)
		if err != nil {
			t.Fatalf("expected expand to materialize bundled file %s: %v", file.path, err)
		}
		if string(got) != file.content {
			t.Fatalf("bundled file %s content = %q, want %q", file.path, string(got), file.content)
		}
	}
}

func assertLoadedPortableResourceManifest(t *testing.T, targetDir string) {
	t.Helper()

	loaded, err := factoryconfig.LoadRuntimeConfig(targetDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(expanded split layout): %v", err)
	}
	if loaded.FactoryConfig().ResourceManifest == nil {
		t.Fatal("expected expanded runtime config to retain resourceManifest")
	}
	if len(loaded.FactoryConfig().ResourceManifest.RequiredTools) != 1 {
		t.Fatalf("expanded runtime manifest requiredTools = %#v", loaded.FactoryConfig().ResourceManifest.RequiredTools)
	}
	if len(loaded.FactoryConfig().ResourceManifest.BundledFiles) != 3 {
		t.Fatalf("expanded runtime manifest bundledFiles = %#v", loaded.FactoryConfig().ResourceManifest.BundledFiles)
	}
}

func writePortableResourceManifestWithMissingTool(t *testing.T, canonicalPath string, canonical map[string]any) {
	t.Helper()

	resourceManifest := canonical["supportingFiles"].(map[string]any)
	requiredTools := resourceManifest["requiredTools"].([]any)
	resourceManifest["requiredTools"] = append(requiredTools, map[string]any{
		"name":    "Missing helper",
		"command": "missing-helper",
	})

	mutatedCanonical, err := json.Marshal(canonical)
	if err != nil {
		t.Fatalf("Marshal(mutated canonical factory): %v", err)
	}
	writeCLITestFile(t, canonicalPath, string(mutatedCanonical))
}

func assertPortableResourceManifestMissingToolLoadFailure(t *testing.T, targetDir string) {
	t.Helper()
	assertPortableResourceManifestMissingToolLoadFailureAtIndex(t, targetDir, 1)
}

func assertPortableResourceManifestMissingToolLoadFailureAtIndex(t *testing.T, targetDir string, requiredToolIndex int) {
	t.Helper()

	_, err := factoryconfig.LoadRuntimeConfig(targetDir, nil)
	if err == nil {
		t.Fatal("expected missing required tool to fail runtime config load")
	}
	if !containsAll(err.Error(),
		"validation failed: 1 errors",
		fmt.Sprintf("[required-tool-missing] resourceManifest.requiredTools[%d].command", requiredToolIndex),
		`"missing-helper" was not found on PATH`,
	) {
		t.Fatalf("expected required-tool load validation failure, got %v", err)
	}
}

func assertExpandDidNotWriteSplitRuntimeFiles(t *testing.T, dir string) {
	t.Helper()

	for _, path := range []string{
		filepath.Join(dir, "workers", "executor", interfaces.FactoryAgentsFileName),
		filepath.Join(dir, "workstations", "execute-story", interfaces.FactoryAgentsFileName),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected expand validation failure to avoid writing %s, stat err = %v", path, err)
		}
	}
}

// portos:func-length-exception owner=agent-factory reason=split-layout-canonicalization-fixture review=2026-07-18 removal=extract-shared-split-layout-fixture-before-next-cli-config-expand-change
func TestExpandFactoryConfig_KeepsExistingCanonicalSplitDefinitionsWhenInlineDefinitionsMissing(t *testing.T) {
	tests := []struct {
		name      string
		inputPath func(string) string
	}{
		{
			name: "directory input",
			inputPath: func(dir string) string {
				return dir
			},
		},
		{
			name: "factory file beside split files",
			inputPath: func(dir string) string {
				return filepath.Join(dir, "factory.json")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeCLITestFile(t, filepath.Join(dir, "factory.json"), `{
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"resources": [],
				"workers": [{"name":"executor"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}]
				}]
			}`)
			workerAgentsPath := filepath.Join(dir, "workers", "executor", "AGENTS.md")
			writeCLITestFile(t, workerAgentsPath, `---
type: SCRIPT_WORKER
command: powershell
args:
  - -NoProfile
  - -Command
  - Write-Output preserved
executorProvider: local
timeout: 45m
stopToken: DONE
---
Existing worker body.
`)
			workstationAgentsPath := filepath.Join(dir, "workstations", "execute-story", "AGENTS.md")
			writeCLITestFile(t, workstationAgentsPath, `---
type: MODEL_WORKSTATION
worker: executor
promptFile: prompts/task.md
limits:
  maxRetries: 3
  maxExecutionTime: 15m
stopWords:
  - DONE
---
Existing workstation body.
`)
			promptPath := filepath.Join(dir, "workstations", "execute-story", "prompts", "task.md")
			writeCLITestFile(t, promptPath, "Preserve {{ .WorkID }}.\n")

			if err := ExpandFactoryConfig(FactoryConfigExpandConfig{Path: tt.inputPath(dir), Output: io.Discard}); err != nil {
				t.Fatalf("ExpandFactoryConfig: %v", err)
			}

			workerAgents := string(readCLITestFile(t, workerAgentsPath))
			assertExpandedAgentsFrontmatterUsesCamelCase(t, workerAgents, []string{
				"type: SCRIPT_WORKER",
				"stopToken: DONE",
			}, []string{
				"stop_token:",
			})
			if !strings.Contains(workerAgents, "Existing worker body.") {
				t.Fatalf("expected expanded worker AGENTS.md body to be preserved:\n%s", workerAgents)
			}

			workstationAgents := string(readCLITestFile(t, workstationAgentsPath))
			assertExpandedAgentsFrontmatterUsesCamelCase(t, workstationAgents, []string{
				"type: MODEL_WORKSTATION",
				"worker: executor",
				"promptFile: prompts/task.md",
				"maxRetries: 3",
				"maxExecutionTime: 15m",
				"stopWords:",
			}, []string{
				"prompt_file:",
				"max_retries:",
				"max_execution_time:",
				"stop_words:",
			})
			if !strings.Contains(workstationAgents, "Existing workstation body.") {
				t.Fatalf("expected expanded workstation AGENTS.md body to be preserved:\n%s", workstationAgents)
			}
			if got := string(readCLITestFile(t, promptPath)); got != "Preserve {{ .WorkID }}.\n" {
				t.Fatalf("prompt file content = %q, want preserved prompt file content", got)
			}

			workerDef, err := factoryconfig.LoadWorkerConfig(filepath.Join(dir, "workers", "executor"))
			if err != nil {
				t.Fatalf("LoadWorkerConfig: %v", err)
			}
			if workerDef.Type != interfaces.WorkerTypeScript || workerDef.Command != "powershell" || workerDef.Timeout != "45m" {
				t.Fatalf("expected existing worker definition to be preserved, got %#v", workerDef)
			}
			workstationDef, err := factoryconfig.LoadWorkstationConfig(filepath.Join(dir, "workstations", "execute-story"))
			if err != nil {
				t.Fatalf("LoadWorkstationConfig: %v", err)
			}
			if workstationDef.Limits.MaxRetries != 3 || workstationDef.PromptTemplate != "Preserve {{ .WorkID }}.\n" {
				t.Fatalf("expected existing workstation definition to be preserved, got %#v", workstationDef)
			}

			if err := ExpandFactoryConfig(FactoryConfigExpandConfig{Path: tt.inputPath(dir), Output: io.Discard}); err != nil {
				t.Fatalf("ExpandFactoryConfig second run: %v", err)
			}
			if workerAgents != string(readCLITestFile(t, workerAgentsPath)) {
				t.Fatalf("worker AGENTS.md changed after idempotent expand")
			}
			if workstationAgents != string(readCLITestFile(t, workstationAgentsPath)) {
				t.Fatalf("workstation AGENTS.md changed after idempotent expand")
			}
		})
	}
}

func TestFlattenFactoryConfig_FlattensCanonicalWorkstationStopWords(t *testing.T) {
	dir := t.TempDir()
	writeCLITestFile(t, filepath.Join(dir, "factory.json"), `{
		"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
		"resources": [],
		"workers": [{
			"name":"executor",
			"type":"SCRIPT_WORKER",
			"command":"echo"
		}],
		"workstations": [{
			"name":"execute-story",
			"worker":"executor",
			"inputs":[{"workType":"story","state":"init"}],
			"outputs":[{"workType":"story","state":"complete"}]
		}]
	}`)
	writeCLITestFile(t, filepath.Join(dir, "workstations", "execute-story", "AGENTS.md"), `---
type: MODEL_WORKSTATION
worker: executor
stopWords:
  - DONE
---

Review the output.
`)

	flattened, err := factoryconfig.FlattenFactoryConfig(dir)
	if err != nil {
		t.Fatalf("FlattenFactoryConfig: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(flattened, &payload); err != nil {
		t.Fatalf("unmarshal flattened config: %v", err)
	}
	workstations := payload["workstations"].([]any)
	workstation := workstations[0].(map[string]any)
	stopWords := workstation["stopWords"].([]any)
	if len(stopWords) != 1 || stopWords[0] != "DONE" {
		t.Fatalf("expected canonical stopWords [DONE], got %#v", workstation["stopWords"])
	}
	if _, ok := workstation["stopToken"]; ok {
		t.Fatalf("expected canonical flattened config not to emit workstation stopToken, got %#v", workstation)
	}
}

// portos:func-length-exception owner=agent-factory reason=split-layout-preservation-fixture review=2026-07-18 removal=extract-shared-split-layout-fixture-before-next-cli-config-expand-change
func TestExpandFactoryConfig_PreservesExistingSplitDefinitionsWhenInlineDefinitionsMissing(t *testing.T) {
	tests := []struct {
		name      string
		inputPath func(string) string
	}{
		{
			name: "directory input",
			inputPath: func(dir string) string {
				return dir
			},
		},
		{
			name: "factory file beside split files",
			inputPath: func(dir string) string {
				return filepath.Join(dir, "factory.json")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			writeCLITestFile(t, filepath.Join(dir, "factory.json"), `{
				"workTypes": [{"name":"story","states":[{"name":"init","type":"INITIAL"},{"name":"complete","type":"TERMINAL"}]}],
				"resources": [],
				"workers": [{"name":"executor"}],
				"workstations": [{
					"name":"execute-story",
					"worker":"executor",
					"inputs":[{"workType":"story","state":"init"}],
					"outputs":[{"workType":"story","state":"complete"}],
					"resources":[],
					"stopWords":["DONE"]
				}]
			}`)
			workerAgentsPath := filepath.Join(dir, "workers", "executor", "AGENTS.md")
			writeCLITestFile(t, workerAgentsPath, `---
type: SCRIPT_WORKER
command: powershell
args:
  - -NoProfile
  - -Command
  - Write-Output preserved
executorProvider: local
timeout: 45m
stopToken: DONE
---
Existing worker body.
`)
			workstationAgentsPath := filepath.Join(dir, "workstations", "execute-story", "AGENTS.md")
			writeCLITestFile(t, workstationAgentsPath, `---
type: MODEL_WORKSTATION
worker: executor
promptFile: prompts/task.md
limits:
  maxRetries: 3
  maxExecutionTime: 15m
stopWords:
  - DONE
---
Existing workstation body.
`)
			promptPath := filepath.Join(dir, "workstations", "execute-story", "prompts", "task.md")
			writeCLITestFile(t, promptPath, "Preserve {{ .WorkID }}.\n")

			if err := ExpandFactoryConfig(FactoryConfigExpandConfig{Path: tt.inputPath(dir), Output: io.Discard}); err != nil {
				t.Fatalf("ExpandFactoryConfig: %v", err)
			}

			workerAgents := string(readCLITestFile(t, workerAgentsPath))
			assertExpandedAgentsFrontmatterUsesCamelCase(t, workerAgents, []string{
				"type: SCRIPT_WORKER",
				"stopToken: DONE",
			}, []string{
				"stop_token:",
			})
			if !strings.Contains(workerAgents, "Existing worker body.") {
				t.Fatalf("expected expanded worker AGENTS.md body to be preserved:\n%s", workerAgents)
			}

			workstationAgents := string(readCLITestFile(t, workstationAgentsPath))
			assertExpandedAgentsFrontmatterUsesCamelCase(t, workstationAgents, []string{
				"type: MODEL_WORKSTATION",
				"worker: executor",
				"promptFile: prompts/task.md",
				"maxRetries: 3",
				"maxExecutionTime: 15m",
				"stopWords:",
			}, []string{
				"prompt_file:",
				"max_retries:",
				"max_execution_time:",
				"stop_words:",
			})
			if !strings.Contains(workstationAgents, "Existing workstation body.") {
				t.Fatalf("expected expanded workstation AGENTS.md body to be preserved:\n%s", workstationAgents)
			}
			if got := string(readCLITestFile(t, promptPath)); got != "Preserve {{ .WorkID }}.\n" {
				t.Fatalf("prompt file content = %q, want preserved prompt file content", got)
			}

			workerDef, err := factoryconfig.LoadWorkerConfig(filepath.Join(dir, "workers", "executor"))
			if err != nil {
				t.Fatalf("LoadWorkerConfig: %v", err)
			}
			if workerDef.Type != interfaces.WorkerTypeScript || workerDef.Command != "powershell" || workerDef.Timeout != "45m" {
				t.Fatalf("expected existing worker definition to be preserved, got %#v", workerDef)
			}
			workstationDef, err := factoryconfig.LoadWorkstationConfig(filepath.Join(dir, "workstations", "execute-story"))
			if err != nil {
				t.Fatalf("LoadWorkstationConfig: %v", err)
			}
			if workstationDef.Limits.MaxRetries != 3 || workstationDef.PromptTemplate != "Preserve {{ .WorkID }}.\n" {
				t.Fatalf("expected existing workstation definition to be preserved, got %#v", workstationDef)
			}

			if err := ExpandFactoryConfig(FactoryConfigExpandConfig{Path: tt.inputPath(dir), Output: io.Discard}); err != nil {
				t.Fatalf("ExpandFactoryConfig second run: %v", err)
			}
			if workerAgents != string(readCLITestFile(t, workerAgentsPath)) {
				t.Fatalf("worker AGENTS.md changed after idempotent expand")
			}
			if workstationAgents != string(readCLITestFile(t, workstationAgentsPath)) {
				t.Fatalf("workstation AGENTS.md changed after idempotent expand")
			}
		})
	}
}

func TestExpandFactoryConfig_InvalidPathReturnsContext(t *testing.T) {
	err := ExpandFactoryConfig(FactoryConfigExpandConfig{Path: filepath.Join(t.TempDir(), "missing-factory.json"), Output: io.Discard})
	if err == nil {
		t.Fatal("expected missing factory config path to fail")
	}
	if !strings.Contains(err.Error(), "find factory config source") {
		t.Fatalf("error = %q, want source path context", err.Error())
	}
}

func writeCLITestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func readCLITestFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func writeCLIRequiredToolExecutable(t *testing.T, dir, baseName string) string {
	t.Helper()

	commandName := baseName
	content := "#!/bin/sh\nexit 0\n"
	if runtime.GOOS == "windows" {
		commandName += ".cmd"
		content = "@echo off\r\nexit /b 0\r\n"
	}

	path := filepath.Join(dir, commandName)
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write required tool %s: %v", path, err)
	}
	return baseName
}

func assertExpandedAgentsFrontmatterUsesCamelCase(t *testing.T, content string, want []string, disallowed []string) {
	t.Helper()
	for _, expected := range want {
		if !strings.Contains(content, expected) {
			t.Fatalf("expanded AGENTS.md missing %q:\n%s", expected, content)
		}
	}
	for _, retired := range disallowed {
		if strings.Contains(content, retired) {
			t.Fatalf("expanded AGENTS.md should not contain retired %q:\n%s", retired, content)
		}
	}
}

func mustReadCanonicalFactoryPayload(t *testing.T, path string) map[string]any {
	t.Helper()
	return mustDecodeCanonicalFactoryPayload(t, readCLITestFile(t, path))
}

func mustDecodeCanonicalFactoryPayload(t *testing.T, data []byte) map[string]any {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal canonical factory payload: %v", err)
	}
	return payload
}

func containsAll(value string, substrings ...string) bool {
	for _, substring := range substrings {
		if !strings.Contains(value, substring) {
			return false
		}
	}
	return true
}
