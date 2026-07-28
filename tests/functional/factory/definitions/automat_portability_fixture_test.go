// artifact-contract-closeout destination package:
// ./tests/functional/factory/definitions -run 'TestAutomatPortabilityFixture_'
package definitions

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	factorymapping "github.com/portpowered/infinite-you/pkg/transports/mapping/factoryconfig"

	"github.com/portpowered/infinite-you/internal/testutil"
	platformprocess "github.com/portpowered/infinite-you/pkg/platform/process"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	"github.com/portpowered/infinite-you/pkg/services/work"
	factoryapi "github.com/portpowered/infinite-you/pkg/transports/http/generated"
	"github.com/portpowered/infinite-you/tests/functional/internal/support"
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

	loaded, err := support.LoadedFactory(t, dir)
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

	assertAutomatPersistedFactoryJSONUsesThinBundledFileContract(t, expandedDir, authoredFactoryDir)
	assertAutomatExpandedBundledFile(t, expandedDir, automatWorkflowGuide, filepath.Join(authoredFactoryDir, automatWorkflowGuide))
	assertAutomatExpandedBundledFile(t, expandedDir, automatPrepareScript, filepath.Join(authoredFactoryDir, automatPrepareScript))
	assertAutomatExpandedBundledFile(t, expandedDir, automatVerifyToolsScript, filepath.Join(authoredFactoryDir, automatVerifyToolsScript))
	assertAutomatExpandedBundledFile(t, expandedDir, automatDependencyContract, filepath.Join(authoredFactoryDir, automatDependencyContract))

	activateAutomatRequiredToolsOnPath(t)
	loaded, err := support.LoadedFactory(t, expandedDir)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig(expanded automat layout): %v", err)
	}
	if loaded.SupportingFiles == nil {
		t.Fatal("expected expanded automat layout to retain resource manifest")
	}
	assertAutomatRequiredToolsManifestAPI(t, loaded.SupportingFiles.RequiredTools)
	bundledFiles := bundledFilesByTargetAPI(loaded.SupportingFiles.BundledFiles)
	assertAutomatBundledFileEntryWithoutInlineAPI(t, bundledFiles, "factory/docs/portable-workflow.md")
	assertAutomatBundledFileEntryWithoutInlineAPI(t, bundledFiles, "factory/scripts/prepare-automat-slice.ps1")
	assertAutomatBundledFileEntryWithoutInlineAPI(t, bundledFiles, "factory/scripts/verify-external-tools.ps1")

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

	testutil.WriteSeedRequest(t, expandedDir, work.SubmitRequest{
		WorkID:     automatDispatchReadyWorkID,
		WorkTypeID: "chapter",
		TraceID:    "trace-automat-ready",
		Payload:    []byte("portable automat readiness"),
	})

	scriptRunner := &automatDispatchReadyRunner{
		expandedDir: expandedDir,
		authoredDir: authoredFactoryDir,
	}
	providerRunner := support.NewRecordingCommandRunner("automat dispatch readiness must not execute providers")
	activateAutomatRequiredToolsOnPath(t)
	_, listed := support.RunFactoryToCompletionWithEdgesAndWork(t, expandedDir, serviceedges.Edges{
		ScriptCommandRunner:   scriptRunner,
		ProviderCommandRunner: providerRunner,
	}, 10*time.Second)
	if providerRunner.CallCount() != 0 {
		t.Fatalf("provider command calls = %d, want 0 for script-only automat readiness smoke", providerRunner.CallCount())
	}
	for placeID, want := range map[string]int{
		"chapter:ready":  1,
		"chapter:init":   0,
		"chapter:staged": 0,
		"chapter:failed": 0,
	} {
		if got := support.CountWorkAtCustomerState(listed, placeID); got != want {
			t.Errorf("%s token count = %d, want %d", placeID, got, want)
		}
	}

	if issues := scriptRunner.Issues(); len(issues) > 0 {
		t.Fatalf("expanded automat readiness smoke issues:\n%s", strings.Join(issues, "\n"))
	}

	requests := scriptRunner.Requests()
	if len(requests) != 2 {
		t.Fatalf("dispatch-ready smoke should issue 2 script requests, got %d", len(requests))
	}
	if got, _ := automatScriptPathAndIssues(requests[0].Args); filepath.Base(got) != filepath.Base(automatPrepareScript) {
		t.Fatalf("first script = %q, want %q", got, automatPrepareScript)
	}
	if got, _ := automatScriptPathAndIssues(requests[1].Args); filepath.Base(got) != filepath.Base(automatVerifyToolsScript) {
		t.Fatalf("second script = %q, want %q", got, automatVerifyToolsScript)
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

	assertListedWorkPayload(t, listed, "chapter", "ready", "required-tools:"+automatExternalMangaka+","+automatExternalMagick)
}

func flattenAutomatFixture(t *testing.T) (string, *interfaces.FactoryConfig, []byte) {
	t.Helper()

	projectDir := t.TempDir()
	authoredFactoryDir := filepath.Join(projectDir, "factory")
	copyFixtureIntoDir(t, support.LegacyFixtureDir(t, automatFixtureName), authoredFactoryDir)

	providerRunner := support.NewRecordingCommandRunner("automat flatten must not execute providers")
	edges := serviceedges.Edges{ProviderCommandRunner: providerRunner}
	flattenInputs := support.FakeInputs(
		context.Background(),
		[]string{"you", "factory", "config", "flatten", authoredFactoryDir},
	)
	flattenInputs.Input.Env = os.Environ()
	flattenInputs.WorkingDirectory = projectDir
	if err := support.BuildProcess(t, edges).Execute(flattenInputs.Input); err != nil {
		t.Fatalf("execute config flatten: %v", err)
	}
	if providerRunner.CallCount() != 0 {
		t.Fatalf("provider command calls = %d, want 0 for automat flatten", providerRunner.CallCount())
	}

	flattenedBytes := []byte(flattenInputs.Stdout())
	flattenedCfg, err := factorymapping.FactoryConfigFromOpenAPIJSON(flattenedBytes)
	if err != nil {
		t.Fatalf("FactoryConfigFromOpenAPIJSON(flattened automat fixture): %v", err)
	}

	return authoredFactoryDir, flattenedCfg, flattenedBytes
}

func flattenAndExpandAutomatFixture(t *testing.T) (string, *interfaces.FactoryConfig, string) {
	t.Helper()

	authoredFactoryDir, flattenedCfg, flattenedBytes := flattenAutomatFixture(t)

	expandedDir := t.TempDir()
	expandedFactoryPath := filepath.Join(expandedDir, interfaces.FactoryConfigFile)
	if err := os.WriteFile(expandedFactoryPath, flattenedBytes, 0o644); err != nil {
		t.Fatalf("write flattened automat factory.json: %v", err)
	}
	copyAutomatPortableExportSidecars(t, authoredFactoryDir, expandedDir)

	providerRunner := support.NewRecordingCommandRunner("automat expand must not execute providers")
	edges := serviceedges.Edges{ProviderCommandRunner: providerRunner}
	expandInputs := support.FakeInputs(
		context.Background(),
		[]string{"you", "factory", "config", "expand", expandedFactoryPath},
	)
	expandInputs.Input.Env = os.Environ()
	expandInputs.WorkingDirectory = expandedDir
	if err := support.BuildProcess(t, edges).Execute(expandInputs.Input); err != nil {
		t.Fatalf("execute config expand: %v", err)
	}
	if providerRunner.CallCount() != 0 {
		t.Fatalf("provider command calls = %d, want 0 for automat expand", providerRunner.CallCount())
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

func assertAutomatFixtureWorkers(t *testing.T, loaded factoryapi.Factory) {
	t.Helper()

	prepareWorker, ok := support.FindFactoryWorker(loaded, automatPrepareWorker)
	if !ok {
		t.Fatalf("expected worker %q", automatPrepareWorker)
	}
	if prepareWorker.Type == nil || string(*prepareWorker.Type) != string(interfaces.WorkerTypeScript) || prepareWorker.Command == nil || *prepareWorker.Command != "powershell" {
		t.Fatalf("prepare worker = %#v", prepareWorker)
	}
	if prepareWorker.Args == nil || !containsFixtureString(*prepareWorker.Args, automatPrepareScript) || !containsFixtureString(*prepareWorker.Args, automatDependencyContract) {
		t.Fatalf("prepare worker args = %#v", prepareWorker.Args)
	}

	verifyWorker, ok := support.FindFactoryWorker(loaded, automatVerifyExternalWorker)
	if !ok {
		t.Fatalf("expected worker %q", automatVerifyExternalWorker)
	}
	if verifyWorker.Type == nil || string(*verifyWorker.Type) != string(interfaces.WorkerTypeScript) || verifyWorker.Command == nil || *verifyWorker.Command != "powershell" {
		t.Fatalf("verify worker = %#v", verifyWorker)
	}
	if verifyWorker.Args == nil || !containsFixtureString(*verifyWorker.Args, automatVerifyToolsScript) || !containsFixtureString(*verifyWorker.Args, automatDependencyContract) {
		t.Fatalf("verify worker args = %#v", verifyWorker.Args)
	}
}

func assertAutomatFixtureWorkstations(t *testing.T, loaded factoryapi.Factory) {
	t.Helper()

	prepareWorkstation, ok := support.FindFactoryWorkstation(loaded, automatPrepareWorkstation)
	if !ok {
		t.Fatalf("expected workstation %q", automatPrepareWorkstation)
	}
	if prepareWorkstation.Type == nil || string(*prepareWorkstation.Type) != string(interfaces.WorkstationTypeScript) || prepareWorkstation.CopyReferencedScripts == nil || !*prepareWorkstation.CopyReferencedScripts {
		t.Fatalf("prepare workstation = %#v", prepareWorkstation)
	}
	if prepareWorkstation.WorkingDirectory == nil || *prepareWorkstation.WorkingDirectory != "runtime/{{ (index .Inputs 0).WorkID }}" {
		t.Fatalf("prepare workstation working directory = %q", automatStringPtrValue(prepareWorkstation.WorkingDirectory))
	}
	if prepareWorkstation.Env == nil || (*prepareWorkstation.Env)["AUTOMAT_DEPENDENCY_CONTRACT"] != automatDependencyContract {
		t.Fatalf("prepare workstation env = %#v", prepareWorkstation.Env)
	}

	verifyWorkstation, ok := support.FindFactoryWorkstation(loaded, automatVerifyWorkstation)
	if !ok {
		t.Fatalf("expected workstation %q", automatVerifyWorkstation)
	}
	if verifyWorkstation.Type == nil || string(*verifyWorkstation.Type) != string(interfaces.WorkstationTypeScript) || verifyWorkstation.CopyReferencedScripts == nil || !*verifyWorkstation.CopyReferencedScripts {
		t.Fatalf("verify workstation = %#v", verifyWorkstation)
	}
	if verifyWorkstation.Env == nil || (*verifyWorkstation.Env)["AUTOMAT_DEPENDENCY_CONTRACT"] != automatDependencyContract {
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

func bundledFilesByTargetAPI(bundledFiles *[]factoryapi.BundledFile) map[string]factoryapi.BundledFile {
	if bundledFiles == nil {
		return nil
	}
	byTarget := make(map[string]factoryapi.BundledFile, len(*bundledFiles))
	for _, bundledFile := range *bundledFiles {
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

func assertAutomatBundledFileEntryWithoutInline(t *testing.T, bundledFiles map[string]interfaces.BundledFileConfig, targetLocation string) {
	t.Helper()

	bundledFile, ok := bundledFiles[targetLocation]
	if !ok {
		t.Fatalf("expected bundled file %s", targetLocation)
	}
	if bundledFile.Content.Inline != "" {
		t.Fatalf("expected bundled file %s to omit inline content, got %q", targetLocation, bundledFile.Content.Inline)
	}
}

func assertAutomatBundledFileEntryWithoutInlineAPI(t *testing.T, bundledFiles map[string]factoryapi.BundledFile, targetLocation string) {
	t.Helper()
	bundledFile, ok := bundledFiles[targetLocation]
	if !ok {
		t.Fatalf("expected bundled file %s", targetLocation)
	}
	if bundledFile.Content.Inline != "" {
		t.Fatalf("expected bundled file %s to omit inline content, got %q", targetLocation, bundledFile.Content.Inline)
	}
}

func copyAutomatPortableExportSidecars(t *testing.T, sourceDir, targetDir string) {
	t.Helper()

	for _, relativePath := range []string{
		automatDependencyContract,
		automatWorkflowGuide,
		automatPrepareScript,
		automatVerifyToolsScript,
	} {
		sourcePath := filepath.Join(sourceDir, relativePath)
		targetPath := filepath.Join(targetDir, relativePath)
		data, err := os.ReadFile(sourcePath)
		if err != nil {
			t.Fatalf("read sidecar %s: %v", sourcePath, err)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(targetPath), err)
		}
		if err := os.WriteFile(targetPath, data, 0o644); err != nil {
			t.Fatalf("write sidecar %s: %v", targetPath, err)
		}
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

func assertAutomatRequiredToolsManifestAPI(t *testing.T, tools *[]factoryapi.RequiredTool) {
	t.Helper()
	if tools == nil || len(*tools) != 2 {
		t.Fatalf("supportingFiles.requiredTools = %#v, want two external tools", tools)
	}
	expected := map[string]string{
		automatExternalMangaka: "OCR and translation extraction remain external to the portable factory",
		automatExternalMagick:  "Image normalization remains external to the portable factory",
	}
	for name, purpose := range expected {
		found := false
		for _, tool := range *tools {
			if tool.Name == name && tool.Purpose != nil && *tool.Purpose == purpose {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("supportingFiles.requiredTools missing %q: %#v", name, *tools)
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

func assertAutomatPersistedFactoryJSONUsesThinBundledFileContract(t *testing.T, expandedDir, authoredFactoryDir string) {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(expandedDir, interfaces.FactoryConfigFile))
	if err != nil {
		t.Fatalf("read expanded factory.json: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal expanded factory.json: %v", err)
	}

	bundledFiles := requireAutomatPersistedBundledFiles(t, payload)
	if len(bundledFiles) != 4 {
		t.Fatalf("expanded factory.json bundled files = %#v, want 4 entries", bundledFiles)
	}

	for _, thinTarget := range []struct {
		targetPath string
		wantType   string
	}{
		{targetPath: "factory/docs/portable-workflow.md", wantType: string(interfaces.BundledFileTypeDoc)},
		{targetPath: "factory/scripts/prepare-automat-slice.ps1", wantType: string(interfaces.BundledFileTypeScript)},
		{targetPath: "factory/scripts/verify-external-tools.ps1", wantType: string(interfaces.BundledFileTypeScript)},
	} {
		bundledFile, ok := findPersistedAutomatBundledFile(bundledFiles, thinTarget.targetPath)
		if !ok {
			t.Fatalf("expanded factory.json missing bundled file %s", thinTarget.targetPath)
		}
		if bundledFile.Type != thinTarget.wantType {
			t.Fatalf("expanded factory.json bundled file %s type = %q, want %q", thinTarget.targetPath, bundledFile.Type, thinTarget.wantType)
		}
		if bundledFile.Content.Encoding != string(interfaces.BundledFileEncodingUTF8) {
			t.Fatalf("expanded factory.json bundled file %s encoding = %q, want %q", thinTarget.targetPath, bundledFile.Content.Encoding, interfaces.BundledFileEncodingUTF8)
		}
		if bundledFile.Content.Inline != "" {
			t.Fatalf("expanded factory.json bundled file %s should omit inline content, got %q", thinTarget.targetPath, bundledFile.Content.Inline)
		}
	}

	dependencyTarget := "factory/" + automatDependencyContract
	dependencyFile, ok := findPersistedAutomatBundledFile(bundledFiles, dependencyTarget)
	if !ok {
		t.Fatalf("expanded factory.json missing bundled file %s", dependencyTarget)
	}
	if dependencyFile.Content.Encoding != string(interfaces.BundledFileEncodingUTF8) {
		t.Fatalf("expanded factory.json dependency contract encoding = %q, want %q", dependencyFile.Content.Encoding, interfaces.BundledFileEncodingUTF8)
	}
	wantDependencyInline, err := os.ReadFile(filepath.Join(authoredFactoryDir, automatDependencyContract))
	if err != nil {
		t.Fatalf("read authored dependency contract: %v", err)
	}
	if dependencyFile.Content.Inline != string(wantDependencyInline) {
		t.Fatalf("expanded factory.json dependency contract inline mismatch")
	}
}

type automatPersistedBundledFile struct {
	Type       string
	TargetPath string
	Content    struct {
		Encoding string
		Inline   string
	}
}

func requireAutomatPersistedBundledFiles(t *testing.T, payload map[string]any) []automatPersistedBundledFile {
	t.Helper()

	var supportingFiles map[string]any
	switch {
	case payload["resourceManifest"] != nil:
		var ok bool
		supportingFiles, ok = payload["resourceManifest"].(map[string]any)
		if !ok {
			t.Fatalf("expected resourceManifest object, got %#v", payload["resourceManifest"])
		}
	case payload["supportingFiles"] != nil:
		var ok bool
		supportingFiles, ok = payload["supportingFiles"].(map[string]any)
		if !ok {
			t.Fatalf("expected supportingFiles object, got %#v", payload["supportingFiles"])
		}
	default:
		t.Fatalf("expected expanded factory.json to include resourceManifest or supportingFiles")
	}

	entries, ok := supportingFiles["bundledFiles"].([]any)
	if !ok {
		t.Fatalf("expected bundledFiles array, got %#v", supportingFiles["bundledFiles"])
	}

	bundledFiles := make([]automatPersistedBundledFile, 0, len(entries))
	for _, entry := range entries {
		obj, ok := entry.(map[string]any)
		if !ok {
			t.Fatalf("expected bundled file object, got %#v", entry)
		}
		content, ok := obj["content"].(map[string]any)
		if !ok {
			t.Fatalf("expected bundled file content object, got %#v", obj["content"])
		}

		var bundledFile automatPersistedBundledFile
		bundledFile.Type, _ = obj["type"].(string)
		bundledFile.TargetPath, _ = obj["targetPath"].(string)
		bundledFile.Content.Encoding, _ = content["encoding"].(string)
		bundledFile.Content.Inline, _ = content["inline"].(string)
		bundledFiles = append(bundledFiles, bundledFile)
	}
	return bundledFiles
}

func findPersistedAutomatBundledFile(
	bundledFiles []automatPersistedBundledFile,
	targetPath string,
) (automatPersistedBundledFile, bool) {
	for _, bundledFile := range bundledFiles {
		if bundledFile.TargetPath == targetPath {
			return bundledFile, true
		}
	}
	return automatPersistedBundledFile{}, false
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
	requests []platformprocess.CommandRequest
	issues   []string
}

func (r *automatDispatchReadyRunner) Run(_ context.Context, req platformprocess.CommandRequest) (platformprocess.CommandResult, error) {
	r.mu.Lock()
	r.requests = append(r.requests, cloneAutomatProcessRequest(req))
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
			return platformprocess.CommandResult{Stderr: []byte(strings.Join(issues, "\n")), ExitCode: 1}, nil
		}
		return platformprocess.CommandResult{Stdout: []byte(stdout)}, nil
	case filepath.Base(automatVerifyToolsScript):
		stdout, verifyIssues := automatVerifyReadinessResult(r.expandedDir, req)
		issues = append(issues, verifyIssues...)
		if len(issues) > 0 {
			r.recordIssues(issues)
			return platformprocess.CommandResult{Stderr: []byte(strings.Join(issues, "\n")), ExitCode: 1}, nil
		}
		return platformprocess.CommandResult{Stdout: []byte(stdout)}, nil
	default:
		issues = append(issues, "unexpected script request: "+strings.Join(req.Args, " "))
		r.recordIssues(issues)
		return platformprocess.CommandResult{Stderr: []byte(strings.Join(issues, "\n")), ExitCode: 1}, nil
	}
}

func (r *automatDispatchReadyRunner) Requests() []platformprocess.CommandRequest {
	r.mu.Lock()
	defer r.mu.Unlock()

	out := make([]platformprocess.CommandRequest, len(r.requests))
	for i := range r.requests {
		out[i] = cloneAutomatProcessRequest(r.requests[i])
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

func cloneAutomatProcessRequest(req platformprocess.CommandRequest) platformprocess.CommandRequest {
	req.Args = append([]string(nil), req.Args...)
	req.Stdin = append([]byte(nil), req.Stdin...)
	req.Env = append([]string(nil), req.Env...)
	return req
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

func automatPrepareReadinessIssues(expandedDir string, req platformprocess.CommandRequest) []string {
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

func automatPrepareReadinessResult(expandedDir string, req platformprocess.CommandRequest) (string, []string) {
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

func automatVerifyReadinessResult(expandedDir string, req platformprocess.CommandRequest) (string, []string) {
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

func automatStringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func assertListedWorkPayload(
	t *testing.T,
	listed factoryapi.ListWorkResponse,
	workType string,
	state string,
	want string,
) {
	t.Helper()
	for _, item := range listed.Results {
		if item.WorkTypeName == nil || *item.WorkTypeName != workType ||
			item.State == nil || item.State.Name != state {
			continue
		}
		if item.Content == nil || len(*item.Content) == 0 {
			t.Fatalf("%s:%s Work has no public content", workType, state)
		}
		part, err := (*item.Content)[0].AsWorkTextContentPart()
		if err != nil {
			t.Fatalf("decode %s:%s Work text content: %v", workType, state, err)
		}
		if part.Text != want {
			t.Fatalf("expected payload %q, got %q", want, part.Text)
		}
		return
	}
	t.Fatalf("no listed Work found in %s:%s", workType, state)
}
