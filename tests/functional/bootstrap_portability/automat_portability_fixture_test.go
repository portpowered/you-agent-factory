package bootstrap_portability

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/portpowered/agent-factory/pkg/cli"
	factoryconfig "github.com/portpowered/agent-factory/pkg/config"
	"github.com/portpowered/agent-factory/pkg/interfaces"
	"github.com/portpowered/agent-factory/pkg/testutil"
	"github.com/portpowered/agent-factory/pkg/workers"
	"github.com/portpowered/agent-factory/tests/functional/internal/support"
)

const (
	automatFixtureName          = "automat_portability_smoke"
	automatDependencyContract   = "portable-dependencies.json"
	automatWorkflowGuide        = "docs/portable-workflow.md"
	automatPrepareScript        = "scripts/prepare-automat-slice.ps1"
	automatVerifyToolsScript    = "scripts/verify-external-tools.ps1"
	automatPrepareWorkstation   = "prepare-automat-slice"
	automatVerifyWorkstation    = "check-tool-contract"
	automatPrepareWorker        = "prepare-workspace"
	automatVerifyExternalWorker = "verify-external-tools"
	automatExternalMangaka      = "mangaka.exe"
	automatExternalMagick       = "magick"
	automatDispatchReadyWorkID  = "work-automat-ready"
)

type automatDependencyContractFile struct {
	RequiredTools []automatRequiredTool `json:"requiredTools"`
}

type automatRequiredTool struct {
	Name    string `json:"name"`
	Purpose string `json:"purpose"`
	Bundled bool   `json:"bundled"`
}

func TestAutomatPortabilityFixture_ModelsBoundedPortableRuntimeLayout(t *testing.T) {
	dir := testutil.CopyFixtureDir(t, support.LegacyFixtureDir(t, automatFixtureName))
	activateAutomatRequiredToolsOnPath(t)

	loaded, err := factoryconfig.LoadRuntimeConfig(dir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(%s): %v", automatFixtureName, err)
	}

	assertAutomatFixtureFiles(t, dir)
	assertAutomatFixtureDocs(t, dir)
	assertAutomatDependencyContract(t, dir)
	assertAutomatFixtureWorkers(t, loaded)
	assertAutomatFixtureWorkstations(t, loaded)
	assertAutomatFixtureOmitsExternalBinaries(t, dir)
}

func TestAutomatPortabilityFixture_FlattenPreservesPortableBundleContract(t *testing.T) {
	authoredFactoryDir, flattenedCfg, _ := flattenAutomatFixture(t)
	if flattenedCfg.ResourceManifest == nil {
		t.Fatal("expected flattened automat fixture to include resourceManifest")
	}
	assertAutomatRequiredToolsManifest(t, flattenedCfg.ResourceManifest.RequiredTools)

	bundledFiles := bundledFilesByTarget(flattenedCfg.ResourceManifest.BundledFiles)
	assertAutomatBundledFileContent(t, bundledFiles, "factory/docs/portable-workflow.md", filepath.Join(authoredFactoryDir, automatWorkflowGuide))
	assertAutomatBundledFileContent(t, bundledFiles, "factory/scripts/prepare-automat-slice.ps1", filepath.Join(authoredFactoryDir, automatPrepareScript))
	assertAutomatBundledFileContent(t, bundledFiles, "factory/scripts/verify-external-tools.ps1", filepath.Join(authoredFactoryDir, automatVerifyToolsScript))

	dependencyFile, ok := bundledFiles["factory/"+automatDependencyContract]
	if !ok {
		t.Fatalf("expected flattened automat fixture to bundle %s: %#v", "factory/"+automatDependencyContract, flattenedCfg.ResourceManifest.BundledFiles)
	}

	var contract automatDependencyContractFile
	if err := json.Unmarshal([]byte(dependencyFile.Content.Inline), &contract); err != nil {
		t.Fatalf("unmarshal flattened dependency contract: %v", err)
	}
	if len(contract.RequiredTools) != 2 {
		t.Fatalf("flattened required tools = %#v, want two external tools", contract.RequiredTools)
	}
	assertAutomatRequiredTool(t, contract.RequiredTools, automatExternalMangaka)
	assertAutomatRequiredTool(t, contract.RequiredTools, automatExternalMagick)

	for _, bundledFile := range flattenedCfg.ResourceManifest.BundledFiles {
		lowerTarget := strings.ToLower(bundledFile.TargetPath)
		if strings.HasSuffix(lowerTarget, "/"+strings.ToLower(automatExternalMangaka)) ||
			strings.HasSuffix(lowerTarget, "/magick.exe") ||
			strings.HasSuffix(lowerTarget, "/"+strings.ToLower(automatExternalMagick)) {
			t.Fatalf("flattened bundle should not include external binary target %q", bundledFile.TargetPath)
		}
	}
}

func TestAutomatPortabilityFixture_ExpandRestoresPortableRuntimeLayout(t *testing.T) {
	authoredFactoryDir, _, expandedDir := flattenAndExpandAutomatFixture(t)

	assertAutomatExpandedBundledFile(t, expandedDir, automatWorkflowGuide, filepath.Join(authoredFactoryDir, automatWorkflowGuide))
	assertAutomatExpandedBundledFile(t, expandedDir, automatPrepareScript, filepath.Join(authoredFactoryDir, automatPrepareScript))
	assertAutomatExpandedBundledFile(t, expandedDir, automatVerifyToolsScript, filepath.Join(authoredFactoryDir, automatVerifyToolsScript))
	assertAutomatExpandedBundledFile(t, expandedDir, automatDependencyContract, filepath.Join(authoredFactoryDir, automatDependencyContract))

	activateAutomatRequiredToolsOnPath(t)
	loaded, err := factoryconfig.LoadRuntimeConfig(expandedDir, nil)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(expanded automat layout): %v", err)
	}
	if loaded.FactoryConfig() == nil || loaded.FactoryConfig().ResourceManifest == nil {
		t.Fatal("expected expanded automat layout to retain resource manifest")
	}
	assertAutomatRequiredToolsManifest(t, loaded.FactoryConfig().ResourceManifest.RequiredTools)
	bundledFiles := bundledFilesByTarget(loaded.FactoryConfig().ResourceManifest.BundledFiles)
	assertAutomatBundledFileContent(t, bundledFiles, "factory/docs/portable-workflow.md", filepath.Join(authoredFactoryDir, automatWorkflowGuide))
	assertAutomatBundledFileContent(t, bundledFiles, "factory/scripts/prepare-automat-slice.ps1", filepath.Join(authoredFactoryDir, automatPrepareScript))
	assertAutomatBundledFileContent(t, bundledFiles, "factory/scripts/verify-external-tools.ps1", filepath.Join(authoredFactoryDir, automatVerifyToolsScript))

	assertAutomatDependencyContract(t, expandedDir)
}

func TestAutomatPortabilityFixture_ExpandedLayoutIsDispatchReadyForBoundedSmoke(t *testing.T) {
	authoredFactoryDir, _, expandedDir := flattenAndExpandAutomatFixture(t)

	if err := os.RemoveAll(authoredFactoryDir); err != nil {
		t.Fatalf("remove authored fixture after expand: %v", err)
	}
	if _, err := os.Stat(authoredFactoryDir); !os.IsNotExist(err) {
		t.Fatalf("expected authored fixture to be removed before readiness smoke, stat err = %v", err)
	}

	testutil.WriteSeedRequest(t, expandedDir, interfaces.SubmitRequest{
		WorkID:     automatDispatchReadyWorkID,
		WorkTypeID: "chapter",
		TraceID:    "trace-automat-ready",
		Payload:    []byte("portable automat readiness"),
	})

	runner := &automatDispatchReadyRunner{
		expandedDir: expandedDir,
		authoredDir: authoredFactoryDir,
	}
	activateAutomatRequiredToolsOnPath(t)
	harness := testutil.NewServiceTestHarness(t, expandedDir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
	)

	harness.RunUntilComplete(t, 10*time.Second)

	harness.Assert().
		PlaceTokenCount("chapter:ready", 1).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:staged").
		HasNoTokenInPlace("chapter:failed")

	if issues := runner.Issues(); len(issues) > 0 {
		t.Fatalf("expanded automat readiness smoke issues:\n%s", strings.Join(issues, "\n"))
	}

	requests := runner.Requests()
	if len(requests) != 2 {
		t.Fatalf("dispatch-ready smoke should issue 2 script requests, got %d", len(requests))
	}
	if got := requests[0].WorkstationName; got != automatPrepareWorkstation {
		t.Fatalf("first workstation = %q, want %q", got, automatPrepareWorkstation)
	}
	if got := requests[1].WorkstationName; got != automatVerifyWorkstation {
		t.Fatalf("second workstation = %q, want %q", got, automatVerifyWorkstation)
	}

	prepareReq := requests[0]
	wantWorkDir := filepath.Join(expandedDir, "runtime", automatDispatchReadyWorkID)
	if prepareReq.WorkDir != wantWorkDir {
		t.Fatalf("prepare work dir = %q, want %q", prepareReq.WorkDir, wantWorkDir)
	}
	if !containsAutomatEnv(prepareReq.Env, "AUTOMAT_DEPENDENCY_CONTRACT="+automatDependencyContract) {
		t.Fatalf("prepare env missing dependency contract: %v", prepareReq.Env)
	}
	if !containsAutomatEnv(prepareReq.Env, "AUTOMAT_WORKFLOW_GUIDE="+automatWorkflowGuide) {
		t.Fatalf("prepare env missing workflow guide: %v", prepareReq.Env)
	}

	verifyReq := requests[1]
	if !containsAutomatEnv(verifyReq.Env, "AUTOMAT_DEPENDENCY_CONTRACT="+automatDependencyContract) {
		t.Fatalf("verify env missing dependency contract: %v", verifyReq.Env)
	}

	assertTokenPayload(t, harness.Marking(), "chapter:ready", "required-tools:"+automatExternalMangaka+","+automatExternalMagick)
}

func TestAutomatPortabilityFixture_IntegrationSmoke_CoversFlattenExpandAndBoundedReadiness(t *testing.T) {
	authoredFactoryDir, flattenedCfg, expandedDir := flattenAndExpandAutomatFixture(t)

	if flattenedCfg.ResourceManifest == nil {
		t.Fatal("expected flattened automat fixture to include resourceManifest")
	}
	assertAutomatRequiredToolsManifest(t, flattenedCfg.ResourceManifest.RequiredTools)

	bundledFiles := bundledFilesByTarget(flattenedCfg.ResourceManifest.BundledFiles)
	for targetLocation, sourcePath := range map[string]string{
		"factory/" + automatDependencyContract:      filepath.Join(authoredFactoryDir, automatDependencyContract),
		"factory/docs/portable-workflow.md":         filepath.Join(authoredFactoryDir, automatWorkflowGuide),
		"factory/scripts/prepare-automat-slice.ps1": filepath.Join(authoredFactoryDir, automatPrepareScript),
		"factory/scripts/verify-external-tools.ps1": filepath.Join(authoredFactoryDir, automatVerifyToolsScript),
	} {
		assertAutomatBundledFileContent(t, bundledFiles, targetLocation, sourcePath)
	}

	assertAutomatDependencyContract(t, expandedDir)
	assertAutomatExpandedBundledFile(t, expandedDir, automatWorkflowGuide, filepath.Join(authoredFactoryDir, automatWorkflowGuide))
	assertAutomatExpandedBundledFile(t, expandedDir, automatPrepareScript, filepath.Join(authoredFactoryDir, automatPrepareScript))
	assertAutomatExpandedBundledFile(t, expandedDir, automatVerifyToolsScript, filepath.Join(authoredFactoryDir, automatVerifyToolsScript))
	assertAutomatExpandedBundledFile(t, expandedDir, automatDependencyContract, filepath.Join(authoredFactoryDir, automatDependencyContract))

	if err := os.RemoveAll(authoredFactoryDir); err != nil {
		t.Fatalf("remove authored fixture after expand: %v", err)
	}

	testutil.WriteSeedRequest(t, expandedDir, interfaces.SubmitRequest{
		WorkID:     automatDispatchReadyWorkID,
		WorkTypeID: "chapter",
		TraceID:    "trace-automat-ready",
		Payload:    []byte("portable automat readiness"),
	})

	runner := &automatDispatchReadyRunner{
		expandedDir: expandedDir,
		authoredDir: authoredFactoryDir,
	}
	activateAutomatRequiredToolsOnPath(t)
	harness := testutil.NewServiceTestHarness(t, expandedDir,
		testutil.WithFullWorkerPoolAndScriptWrap(),
		testutil.WithCommandRunner(runner),
	)

	harness.RunUntilComplete(t, 10*time.Second)

	harness.Assert().
		PlaceTokenCount("chapter:ready", 1).
		HasNoTokenInPlace("chapter:init").
		HasNoTokenInPlace("chapter:staged").
		HasNoTokenInPlace("chapter:failed")

	if issues := runner.Issues(); len(issues) > 0 {
		t.Fatalf("automat integration smoke issues:\n%s", strings.Join(issues, "\n"))
	}

	assertTokenPayload(t, harness.Marking(), "chapter:ready", "required-tools:"+automatExternalMangaka+","+automatExternalMagick)
}

func flattenAutomatFixture(t *testing.T) (string, *interfaces.FactoryConfig, []byte) {
	t.Helper()

	projectDir := t.TempDir()
	authoredFactoryDir := filepath.Join(projectDir, "factory")
	copyFixtureIntoDir(t, support.LegacyFixtureDir(t, automatFixtureName), authoredFactoryDir)

	var flattenOut bytes.Buffer
	flattenCmd := cli.NewRootCommand()
	flattenCmd.SetOut(&flattenOut)
	flattenCmd.SetErr(&bytes.Buffer{})
	flattenCmd.SetArgs([]string{"config", "flatten", authoredFactoryDir})
	if err := flattenCmd.Execute(); err != nil {
		t.Fatalf("execute config flatten: %v", err)
	}

	flattenedCfg, err := factoryconfig.FactoryConfigFromOpenAPIJSON(flattenOut.Bytes())
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON(flattened automat fixture): %v", err)
	}

	return authoredFactoryDir, flattenedCfg, flattenOut.Bytes()
}

func flattenAndExpandAutomatFixture(t *testing.T) (string, *interfaces.FactoryConfig, string) {
	t.Helper()

	authoredFactoryDir, flattenedCfg, flattenedBytes := flattenAutomatFixture(t)

	expandedDir := t.TempDir()
	expandedFactoryPath := filepath.Join(expandedDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(expandedFactoryPath, flattenedBytes, 0o644); err != nil {
		t.Fatalf("write flattened automat factory.json: %v", err)
	}

	expandCmd := cli.NewRootCommand()
	expandCmd.SetOut(&bytes.Buffer{})
	expandCmd.SetErr(&bytes.Buffer{})
	expandCmd.SetArgs([]string{"config", "expand", expandedFactoryPath})
	if err := expandCmd.Execute(); err != nil {
		t.Fatalf("execute config expand: %v", err)
	}

	return authoredFactoryDir, flattenedCfg, expandedDir
}

func assertAutomatFixtureFiles(t *testing.T, dir string) {
	t.Helper()

	for _, relativePath := range []string{
		automatDependencyContract,
		automatWorkflowGuide,
		automatPrepareScript,
		automatVerifyToolsScript,
	} {
		if _, err := os.Stat(filepath.Join(dir, relativePath)); err != nil {
			t.Fatalf("expected fixture file %s: %v", relativePath, err)
		}
	}
}

func assertAutomatFixtureDocs(t *testing.T, dir string) {
	t.Helper()

	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatalf("read fixture README: %v", err)
	}
	readmeText := string(readme)
	for _, expected := range []string{
		"dispatch readiness",
		automatExternalMangaka,
		automatExternalMagick,
		automatDependencyContract,
	} {
		if !strings.Contains(readmeText, expected) {
			t.Fatalf("fixture README missing %q", expected)
		}
	}
}

func assertAutomatDependencyContract(t *testing.T, dir string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(dir, automatDependencyContract))
	if err != nil {
		t.Fatalf("read dependency contract: %v", err)
	}

	var contract automatDependencyContractFile
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatalf("unmarshal dependency contract: %v", err)
	}
	if len(contract.RequiredTools) != 2 {
		t.Fatalf("required tools = %#v, want two external tools", contract.RequiredTools)
	}

	toolNames := make(map[string]automatRequiredTool, len(contract.RequiredTools))
	for _, tool := range contract.RequiredTools {
		toolNames[tool.Name] = tool
		if tool.Bundled {
			t.Fatalf("required tool %q unexpectedly marked bundled", tool.Name)
		}
		if strings.TrimSpace(tool.Purpose) == "" {
			t.Fatalf("required tool %q missing purpose", tool.Name)
		}
	}
	for _, toolName := range []string{automatExternalMangaka, automatExternalMagick} {
		if _, ok := toolNames[toolName]; !ok {
			t.Fatalf("required tools missing %q: %#v", toolName, contract.RequiredTools)
		}
	}
}

func assertAutomatFixtureWorkers(t *testing.T, loaded *factoryconfig.LoadedFactoryConfig) {
	t.Helper()

	prepareWorker, ok := loaded.Worker(automatPrepareWorker)
	if !ok {
		t.Fatalf("expected worker %q", automatPrepareWorker)
	}
	if prepareWorker.Type != interfaces.WorkerTypeScript || prepareWorker.Command != "powershell" {
		t.Fatalf("prepare worker = %#v", prepareWorker)
	}
	if !containsFixtureString(prepareWorker.Args, automatPrepareScript) || !containsFixtureString(prepareWorker.Args, automatDependencyContract) {
		t.Fatalf("prepare worker args = %#v", prepareWorker.Args)
	}

	verifyWorker, ok := loaded.Worker(automatVerifyExternalWorker)
	if !ok {
		t.Fatalf("expected worker %q", automatVerifyExternalWorker)
	}
	if verifyWorker.Type != interfaces.WorkerTypeScript || verifyWorker.Command != "powershell" {
		t.Fatalf("verify worker = %#v", verifyWorker)
	}
	if !containsFixtureString(verifyWorker.Args, automatVerifyToolsScript) || !containsFixtureString(verifyWorker.Args, automatDependencyContract) {
		t.Fatalf("verify worker args = %#v", verifyWorker.Args)
	}
}

func assertAutomatFixtureWorkstations(t *testing.T, loaded *factoryconfig.LoadedFactoryConfig) {
	t.Helper()

	prepareWorkstation, ok := loaded.Workstation(automatPrepareWorkstation)
	if !ok {
		t.Fatalf("expected workstation %q", automatPrepareWorkstation)
	}
	if prepareWorkstation.Type != interfaces.WorkstationTypeModel || !prepareWorkstation.CopyReferencedScripts {
		t.Fatalf("prepare workstation = %#v", prepareWorkstation)
	}
	if prepareWorkstation.WorkingDirectory != "runtime/{{ (index .Inputs 0).WorkID }}" {
		t.Fatalf("prepare workstation working directory = %q", prepareWorkstation.WorkingDirectory)
	}
	if prepareWorkstation.Env["AUTOMAT_DEPENDENCY_CONTRACT"] != automatDependencyContract {
		t.Fatalf("prepare workstation env = %#v", prepareWorkstation.Env)
	}

	verifyWorkstation, ok := loaded.Workstation(automatVerifyWorkstation)
	if !ok {
		t.Fatalf("expected workstation %q", automatVerifyWorkstation)
	}
	if verifyWorkstation.Type != interfaces.WorkstationTypeModel || !verifyWorkstation.CopyReferencedScripts {
		t.Fatalf("verify workstation = %#v", verifyWorkstation)
	}
	if verifyWorkstation.Env["AUTOMAT_DEPENDENCY_CONTRACT"] != automatDependencyContract {
		t.Fatalf("verify workstation env = %#v", verifyWorkstation.Env)
	}
}

func assertAutomatFixtureOmitsExternalBinaries(t *testing.T, dir string) {
	t.Helper()

	disallowedNames := map[string]struct{}{
		automatExternalMangaka: {},
		automatExternalMagick:  {},
		"magick.exe":           {},
	}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if _, forbidden := disallowedNames[strings.ToLower(d.Name())]; forbidden {
			return fs.ErrPermission
		}
		return nil
	})
	if err != nil {
		t.Fatalf("fixture should not bundle external binaries: %v", err)
	}
}

func containsFixtureString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func activateAutomatRequiredToolsOnPath(t *testing.T) {
	t.Helper()

	binDir := t.TempDir()
	writeAutomatPathTool(t, filepath.Join(binDir, automatExternalMangaka), "")
	writeAutomatPathTool(t, filepath.Join(binDir, automatExternalMagick), "")
	writeAutomatPathTool(t, filepath.Join(binDir, "magick.cmd"), "@echo off\r\n")
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func writeAutomatPathTool(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake required tool %s: %v", path, err)
	}
}

func copyFixtureIntoDir(t *testing.T, srcDir, dstDir string) {
	t.Helper()

	if err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	}); err != nil {
		t.Fatalf("copy fixture into %s: %v", dstDir, err)
	}
}

func bundledFilesByTarget(bundledFiles []interfaces.BundledFileConfig) map[string]interfaces.BundledFileConfig {
	byTarget := make(map[string]interfaces.BundledFileConfig, len(bundledFiles))
	for _, bundledFile := range bundledFiles {
		byTarget[bundledFile.TargetPath] = bundledFile
	}
	return byTarget
}

func assertAutomatBundledFileContent(t *testing.T, bundledFiles map[string]interfaces.BundledFileConfig, targetLocation, sourcePath string) {
	t.Helper()

	bundledFile, ok := bundledFiles[targetLocation]
	if !ok {
		t.Fatalf("expected bundled file %s", targetLocation)
	}
	wantContent, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source bundled file %s: %v", sourcePath, err)
	}
	if bundledFile.Content.Inline != string(wantContent) {
		t.Fatalf("bundled file %s content mismatch", targetLocation)
	}
}

func assertAutomatRequiredToolsManifest(t *testing.T, tools []interfaces.RequiredToolConfig) {
	t.Helper()

	if len(tools) != 2 {
		t.Fatalf("resourceManifest.requiredTools = %#v, want two external tools", tools)
	}

	expected := map[string]string{
		automatExternalMangaka: "OCR and translation extraction remain external to the portable factory",
		automatExternalMagick:  "Image normalization remains external to the portable factory",
	}
	for _, tool := range tools {
		wantPurpose, ok := expected[tool.Name]
		if !ok {
			t.Fatalf("unexpected required tool %#v", tool)
		}
		if tool.Command != tool.Name {
			t.Fatalf("required tool %q command = %q, want %q", tool.Name, tool.Command, tool.Name)
		}
		if tool.Purpose != wantPurpose {
			t.Fatalf("required tool %q purpose = %q, want %q", tool.Name, tool.Purpose, wantPurpose)
		}
	}
}

func assertAutomatExpandedBundledFile(t *testing.T, expandedDir, relativePath, sourcePath string) {
	t.Helper()

	got, err := os.ReadFile(filepath.Join(expandedDir, relativePath))
	if err != nil {
		t.Fatalf("read expanded bundled file %s: %v", relativePath, err)
	}
	want, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read authored bundled file %s: %v", sourcePath, err)
	}
	if string(got) != string(want) {
		t.Fatalf("expanded bundled file %s content mismatch", relativePath)
	}
}

func assertAutomatRequiredTool(t *testing.T, tools []automatRequiredTool, name string) {
	t.Helper()

	for _, tool := range tools {
		if tool.Name != name {
			continue
		}
		if tool.Bundled {
			t.Fatalf("required tool %q unexpectedly marked bundled", tool.Name)
		}
		if strings.TrimSpace(tool.Purpose) == "" {
			t.Fatalf("required tool %q missing purpose", tool.Name)
		}
		return
	}
	t.Fatalf("required tools missing %q: %#v", name, tools)
}

type automatDispatchReadyRunner struct {
	expandedDir string
	authoredDir string

	mu       sync.Mutex
	requests []workers.CommandRequest
	issues   []string
}

func (r *automatDispatchReadyRunner) Run(_ context.Context, req workers.CommandRequest) (workers.CommandResult, error) {
	r.mu.Lock()
	r.requests = append(r.requests, workers.CommandRequest(interfaces.CloneSubprocessExecutionRequest(req)))
	r.mu.Unlock()

	scriptPath, issues := automatScriptPathAndIssues(req.Args)
	if _, err := os.Stat(r.authoredDir); !os.IsNotExist(err) {
		issues = append(issues, "authored fixture should stay removed during readiness smoke")
	}

	switch filepath.Base(scriptPath) {
	case filepath.Base(automatPrepareScript):
		stdout, prepareIssues := automatPrepareReadinessResult(r.expandedDir, req)
		issues = append(issues, prepareIssues...)
		if len(issues) > 0 {
			r.recordIssues(issues)
			return workers.CommandResult{Stderr: []byte(strings.Join(issues, "\n")), ExitCode: 1}, nil
		}
		return workers.CommandResult{Stdout: []byte(stdout)}, nil
	case filepath.Base(automatVerifyToolsScript):
		stdout, verifyIssues := automatVerifyReadinessResult(r.expandedDir, req)
		issues = append(issues, verifyIssues...)
		if len(issues) > 0 {
			r.recordIssues(issues)
			return workers.CommandResult{Stderr: []byte(strings.Join(issues, "\n")), ExitCode: 1}, nil
		}
		return workers.CommandResult{Stdout: []byte(stdout)}, nil
	default:
		issues = append(issues, "unexpected script request: "+strings.Join(req.Args, " "))
		r.recordIssues(issues)
		return workers.CommandResult{Stderr: []byte(strings.Join(issues, "\n")), ExitCode: 1}, nil
	}
}

func (r *automatDispatchReadyRunner) Requests() []workers.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]workers.CommandRequest, len(r.requests))
	for i := range r.requests {
		out[i] = workers.CommandRequest(interfaces.CloneSubprocessExecutionRequest(r.requests[i]))
	}
	return out
}

func (r *automatDispatchReadyRunner) Issues() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.issues...)
}

func (r *automatDispatchReadyRunner) recordIssues(issues []string) {
	if len(issues) == 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.issues = append(r.issues, issues...)
}

func automatScriptPathAndIssues(args []string) (string, []string) {
	for i := 0; i < len(args); i++ {
		if !strings.EqualFold(args[i], "-File") && !strings.EqualFold(args[i], "-f") {
			continue
		}
		if i+1 >= len(args) {
			return "", []string{"script request missing value after -File"}
		}
		return filepath.Clean(args[i+1]), nil
	}
	return "", []string{"script request missing -File script path"}
}

func automatPrepareReadinessIssues(expandedDir string, req workers.CommandRequest) []string {
	issues := []string{}
	for _, relativePath := range []string{
		automatPrepareScript,
		automatDependencyContract,
		automatWorkflowGuide,
	} {
		if _, err := os.Stat(filepath.Join(expandedDir, relativePath)); err != nil {
			issues = append(issues, "expanded layout missing "+relativePath+": "+err.Error())
		}
	}

	if req.WorkDir == "" {
		issues = append(issues, "prepare request missing working directory")
	}
	return issues
}

func automatPrepareReadinessResult(expandedDir string, req workers.CommandRequest) (string, []string) {
	issues := automatPrepareReadinessIssues(expandedDir, req)
	scriptContent, err := os.ReadFile(filepath.Join(expandedDir, automatPrepareScript))
	if err != nil {
		return "", append(issues, "read restored prepare script: "+err.Error())
	}
	if !strings.Contains(string(scriptContent), "Get-Content -Raw -LiteralPath $WorkflowGuide") {
		issues = append(issues, "restored prepare script should read the workflow guide")
	}
	if !strings.Contains(string(scriptContent), "ConvertFrom-Json") {
		issues = append(issues, "restored prepare script should parse the dependency contract")
	}
	if !strings.Contains(string(scriptContent), "dispatch-ready:") {
		issues = append(issues, "restored prepare script should emit dispatch-ready output")
	}

	guideContent, err := os.ReadFile(filepath.Join(expandedDir, automatWorkflowGuide))
	if err != nil {
		return "", append(issues, "read restored workflow guide: "+err.Error())
	}
	contract, contractIssues := loadAutomatDependencyContract(filepath.Join(expandedDir, automatDependencyContract))
	issues = append(issues, contractIssues...)
	if len(issues) > 0 {
		return "", issues
	}

	guideHeading := automatGuideHeading(string(guideContent))
	if guideHeading == "" {
		issues = append(issues, "workflow guide missing heading")
	}
	if !strings.Contains(string(guideContent), "Portable Workflow Slice") {
		issues = append(issues, "workflow guide missing portability heading")
	}
	requiredTools := automatRequiredToolNames(contract.RequiredTools)
	for _, toolName := range requiredTools {
		if !strings.Contains(string(guideContent), toolName) {
			issues = append(issues, "workflow guide missing declared tool "+toolName)
		}
	}
	if len(issues) > 0 {
		return "", issues
	}

	return "dispatch-ready:" + guideHeading + ":" + strings.Join(requiredTools, ","), nil
}

func automatVerifyReadinessResult(expandedDir string, req workers.CommandRequest) (string, []string) {
	issues := []string{}
	if _, err := os.Stat(filepath.Join(expandedDir, automatVerifyToolsScript)); err != nil {
		issues = append(issues, "expanded layout missing "+automatVerifyToolsScript+": "+err.Error())
	}

	scriptContent, err := os.ReadFile(filepath.Join(expandedDir, automatVerifyToolsScript))
	if err != nil {
		return "", append(issues, "read restored verify script: "+err.Error())
	}
	if !strings.Contains(string(scriptContent), "ConvertFrom-Json") {
		issues = append(issues, "restored verify script should parse the dependency contract")
	}
	if !strings.Contains(string(scriptContent), "requiredTools") {
		issues = append(issues, "restored verify script should read requiredTools from the dependency contract")
	}
	if !strings.Contains(string(scriptContent), "required-tools:") {
		issues = append(issues, "restored verify script should emit required-tools output")
	}

	contract, contractIssues := loadAutomatDependencyContract(filepath.Join(expandedDir, automatDependencyContract))
	issues = append(issues, contractIssues...)
	if len(contract.RequiredTools) != 2 {
		issues = append(issues, "expanded dependency contract should preserve 2 required tools")
	}
	for _, toolName := range []string{automatExternalMangaka, automatExternalMagick} {
		if !automatRequiredToolPresent(contract.RequiredTools, toolName) {
			issues = append(issues, "expanded dependency contract missing "+toolName)
		}
	}

	disallowedNames := map[string]struct{}{
		strings.ToLower(automatExternalMangaka): {},
		strings.ToLower(automatExternalMagick):  {},
		"magick.exe":                            {},
	}
	err = filepath.WalkDir(expandedDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || d.IsDir() {
			return walkErr
		}
		if _, forbidden := disallowedNames[strings.ToLower(d.Name())]; forbidden {
			issues = append(issues, "expanded layout unexpectedly bundled external binary "+path)
		}
		return nil
	})
	if err != nil {
		issues = append(issues, "walk expanded layout for external binaries: "+err.Error())
	}
	if len(issues) > 0 {
		return "", issues
	}
	return "required-tools:" + strings.Join(automatRequiredToolNames(contract.RequiredTools), ","), nil
}

func loadAutomatDependencyContract(path string) (automatDependencyContractFile, []string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return automatDependencyContractFile{}, []string{"read expanded dependency contract: " + err.Error()}
	}

	var contract automatDependencyContractFile
	if err := json.Unmarshal(data, &contract); err != nil {
		return automatDependencyContractFile{}, []string{"unmarshal expanded dependency contract: " + err.Error()}
	}
	return contract, nil
}

func automatGuideHeading(guide string) string {
	for _, line := range strings.Split(guide, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
	}
	return ""
}

func automatRequiredToolNames(tools []automatRequiredTool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}

func automatRequiredToolPresent(tools []automatRequiredTool, name string) bool {
	for _, tool := range tools {
		if tool.Name != name {
			continue
		}
		return !tool.Bundled && strings.TrimSpace(tool.Purpose) != ""
	}
	return false
}

func containsAutomatEnv(env []string, expected string) bool {
	for _, entry := range env {
		if entry == expected {
			return true
		}
	}
	return false
}
