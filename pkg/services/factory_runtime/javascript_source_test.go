package factory_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

const sourceValidWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
workflow.log("step");
workflow.artifact({ kind: "log", label: "step" });
const result = await agent.run({ prompt: "review" });
workflow.final({ ok: true, result });
pipeline([], function () {}, function () {});
`

var factoryWorkflowDefinitions = testJavaScriptWorkflows()

func TestResolve_WorkflowName_UsesProjectClaudeWorkflowsFirst(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, factory.WorkflowSourceProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "review.js"), []byte(sourceValidWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	ctx := testContext(t, projectRoot)
	resolution := factoryWorkflowDefinitions.ResolveSource(factory.WorkflowSourceRequest{
		Kind:  factory.WorkflowSourceKindWorkflowName,
		Value: "review",
	}, ctx)

	if !resolution.Found {
		t.Fatalf("resolution = %#v, want found project workflow", resolution)
	}
	if resolution.LookupStage != factory.WorkflowSourceLookupStageProjectClaude {
		t.Fatalf("lookup stage = %q, want %q", resolution.LookupStage, factory.WorkflowSourceLookupStageProjectClaude)
	}
	if resolution.SourceRef != factory.WorkflowSourceProjectClaudeWorkflowsDir+"/review.js" {
		t.Fatalf("source ref = %q", resolution.SourceRef)
	}
	if resolution.SourceHash == "" {
		t.Fatal("expected stable source hash")
	}
	if resolution.OrchestratorKind != interfaces.OrchestratorKindJavaScript {
		t.Fatalf("orchestrator kind = %q, want JAVASCRIPT", resolution.OrchestratorKind)
	}
}

func TestResolve_WorkflowName_PrefersProjectOverGlobal(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	homeDir := t.TempDir()
	projectDir := filepath.Join(projectRoot, factory.WorkflowSourceProjectClaudeWorkflowsDir)
	globalDir := filepath.Join(homeDir, ".you-agent-factory", "workflows")
	for _, dir := range []string{projectDir, globalDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectDir, "review.js"), []byte(sourceValidWorkflowSource), 0o600); err != nil {
		t.Fatalf("write project workflow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "review.js"), []byte("meta({name:'other'});"), 0o600); err != nil {
		t.Fatalf("write global workflow: %v", err)
	}

	ctx := testContextWithHome(t, projectRoot, homeDir)
	resolution := factoryWorkflowDefinitions.ResolveSource(factory.WorkflowSourceRequest{
		Kind:  factory.WorkflowSourceKindWorkflowName,
		Value: "review",
	}, ctx)

	if resolution.LookupStage != factory.WorkflowSourceLookupStageProjectClaude {
		t.Fatalf("lookup stage = %q, want project precedence", resolution.LookupStage)
	}
}

func TestResolve_WorkflowName_ReportsConflictForMultipleJavaScriptFactories(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	factoryRoot := filepath.Join(projectRoot, interfaces.FactoryDir)
	for _, name := range []string{"alpha-flow", "beta-flow"} {
		factoryDir := writeJavaScriptFactorySourceFixture(
			t,
			factoryRoot,
			name,
			"workflows/review.js",
		)
		workflowDir := filepath.Join(factoryDir, "workflows")
		if err := os.MkdirAll(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir workflows: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workflowDir, "review.js"), []byte(sourceValidWorkflowSource), 0o600); err != nil {
			t.Fatalf("write workflow: %v", err)
		}
	}

	ctx := testContext(t, projectRoot)
	resolution := factoryWorkflowDefinitions.ResolveSource(factory.WorkflowSourceRequest{
		Kind:  factory.WorkflowSourceKindWorkflowName,
		Value: "review",
	}, ctx)

	if resolution.Found {
		t.Fatalf("resolution = %#v, want conflict", resolution)
	}
	if len(resolution.Diagnostics) == 0 || resolution.Diagnostics[0].Code != factory.WorkflowSourceCodeConflict {
		t.Fatalf("diagnostics = %#v, want conflict", resolution.Diagnostics)
	}
}

func TestResolve_WorkflowFile_BypassesOrderedLookup(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	packageDir := filepath.Join(projectRoot, interfaces.FactoryDir, interfaces.WorkflowsDir)
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sourcePath := filepath.Join(packageDir, "review.js")
	if err := os.WriteFile(sourcePath, []byte(sourceValidWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	ctx := testContext(t, projectRoot)
	resolution := factoryWorkflowDefinitions.ResolveSource(factory.WorkflowSourceRequest{
		Kind:  factory.WorkflowSourceKindWorkflowFile,
		Value: interfaces.FactoryDir + "/workflows/review.js",
	}, ctx)

	if !resolution.Found || resolution.LookupStage != factory.WorkflowSourceLookupStageExplicitSourceKind {
		t.Fatalf("resolution = %#v, want explicit workflow file bypass", resolution)
	}
	if resolution.SourceRef != interfaces.FactoryDir+"/workflows/review.js" {
		t.Fatalf("source ref = %q", resolution.SourceRef)
	}
}

func TestResolve_FactoryID_ResolvesJavaScriptFactory(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	factoryRoot := filepath.Join(projectRoot, interfaces.FactoryDir)
	factoryDir := writeJavaScriptFactorySourceFixture(
		t,
		factoryRoot,
		"review-flow",
		"workflows/review.js",
	)
	workflowDir := filepath.Join(factoryDir, "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "review.js"), []byte(sourceValidWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	ctx := testContext(t, projectRoot)
	resolution := factoryWorkflowDefinitions.ResolveSource(factory.WorkflowSourceRequest{
		Kind:  factory.WorkflowSourceKindFactoryID,
		Value: "review-flow",
	}, ctx)

	if !resolution.Found || resolution.LookupStage != factory.WorkflowSourceLookupStageExplicitFactory {
		t.Fatalf("resolution = %#v, want explicit factory resolution", resolution)
	}
	if resolution.SourceRef != "factory:review-flow:workflows/review.js" {
		t.Fatalf("source ref = %q", resolution.SourceRef)
	}
	if !bytes.Contains(resolution.ArgsSchema, []byte(`"topic"`)) {
		t.Fatalf("args schema = %s, want authored factory schema", resolution.ArgsSchema)
	}
	if !bytes.Contains(resolution.DefaultPolicy, []byte(`"maxAgents":2`)) {
		t.Fatalf("default policy = %s, want authored factory policy", resolution.DefaultPolicy)
	}
}

func writeJavaScriptFactorySourceFixture(
	t *testing.T,
	factoryRoot string,
	name string,
	workflowPath string,
) string {
	t.Helper()

	factoryDir := filepath.Join(factoryRoot, name)
	if err := os.MkdirAll(factoryDir, 0o755); err != nil {
		t.Fatalf("create authored Factory fixture %s: %v", name, err)
	}
	if err := os.WriteFile(
		filepath.Join(factoryDir, interfaces.FactoryConfigFile),
		[]byte(javascriptFactoryPayload(name, workflowPath)),
		0o600,
	); err != nil {
		t.Fatalf("write authored Factory fixture %s: %v", name, err)
	}
	return factoryDir
}

func TestResolve_InlineWorkflow_ComputesStableHash(t *testing.T) {
	t.Parallel()
	ctx := testContext(t, t.TempDir())
	first := factoryWorkflowDefinitions.ResolveSource(factory.WorkflowSourceRequest{
		Kind:         factory.WorkflowSourceKindInlineWorkflow,
		InlineSource: sourceValidWorkflowSource,
	}, ctx)

	second := factoryWorkflowDefinitions.ResolveSource(factory.WorkflowSourceRequest{
		Kind:         factory.WorkflowSourceKindInlineWorkflow,
		InlineSource: sourceValidWorkflowSource,
	}, ctx)

	if !first.Found || first.SourceHash == "" || first.SourceHash != second.SourceHash {
		t.Fatalf("hashes = (%q, %q), want stable non-empty hash", first.SourceHash, second.SourceHash)
	}
}

func TestResolve_ArtifactRoot_RejectsSymlinkThatResolvesInsideRepo(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	ctx := testContext(t, projectRoot)

	insideRepo := filepath.Join(projectRoot, "artifacts")
	if err := os.MkdirAll(insideRepo, 0o755); err != nil {
		t.Fatalf("mkdir inside repo: %v", err)
	}
	outsideLink := filepath.Join(filepath.Dir(projectRoot), "outside-artifact-link")
	if err := os.Symlink(insideRepo, outsideLink); err != nil {
		if runtime.GOOS != "windows" {
			t.Fatalf("symlink outside->inside: %v", err)
		}
		t.Skipf("Windows symlink privilege unavailable: %v", err)
	}

	resolution := factoryWorkflowDefinitions.ResolveSource(factory.WorkflowSourceRequest{
		Kind:         factory.WorkflowSourceKindInlineWorkflow,
		InlineSource: sourceValidWorkflowSource,
		ArtifactRoot: outsideLink,
	}, ctx)

	if resolution.Found || resolution.ArtifactRoot.Allowed {
		t.Fatalf("symlinked artifact root = %#v, want rejection when resolved path is inside repo", resolution.ArtifactRoot)
	}
	if resolution.ArtifactRoot.Diagnostic == nil || resolution.ArtifactRoot.Diagnostic.Code != factory.WorkflowSourceCodeArtifactRootInsideRepo {
		t.Fatalf("diagnostic = %#v, want inside-repo rejection", resolution.ArtifactRoot.Diagnostic)
	}
}

func TestResolve_ArtifactRoot_RejectsRelativeAndInsideRepoPaths(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	ctx := testContext(t, projectRoot)

	relative := factoryWorkflowDefinitions.ResolveSource(factory.WorkflowSourceRequest{
		Kind:         factory.WorkflowSourceKindInlineWorkflow,
		InlineSource: sourceValidWorkflowSource,
		ArtifactRoot: "artifacts",
	}, ctx)

	if relative.Found || relative.ArtifactRoot.Allowed {
		t.Fatalf("relative artifact root = %#v, want rejection", relative.ArtifactRoot)
	}

	insideRepo := filepath.Join(projectRoot, "outside-artifacts")
	inside := factoryWorkflowDefinitions.ResolveSource(factory.WorkflowSourceRequest{
		Kind:         factory.WorkflowSourceKindInlineWorkflow,
		InlineSource: sourceValidWorkflowSource,
		ArtifactRoot: insideRepo,
	}, ctx)

	if inside.Found || inside.ArtifactRoot.Allowed {
		t.Fatalf("inside-repo artifact root = %#v, want rejection", inside.ArtifactRoot)
	}

	outsideRoot := filepath.Join(filepath.Dir(projectRoot), "workflow-artifacts")
	outside := factoryWorkflowDefinitions.ResolveSource(factory.WorkflowSourceRequest{
		Kind:         factory.WorkflowSourceKindInlineWorkflow,
		InlineSource: sourceValidWorkflowSource,
		ArtifactRoot: outsideRoot,
	}, ctx)

	if !outside.Found || !outside.ArtifactRoot.Allowed {
		t.Fatalf("outside artifact root = %#v, want acceptance", outside.ArtifactRoot)
	}
}

func TestResolveSource_ServiceRootResolvesWorkflowName(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, factory.WorkflowSourceProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "review.js"), []byte(sourceValidWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	ctx, err := factoryWorkflowDefinitions.DefaultSourceContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	req := factory.WorkflowSourceRequest{Kind: factory.WorkflowSourceKindWorkflowName, Value: "review"}

	resolution := factoryWorkflowDefinitions.ResolveSource(req, ctx)
	if !resolution.Found || resolution.SourceHash == "" || resolution.SourceRef != factory.WorkflowSourceProjectClaudeWorkflowsDir+"/review.js" {
		t.Fatalf("resolution = %#v, want resolved workflow name", resolution)
	}
}

func testContext(t *testing.T, projectRoot string) factory.WorkflowSourceContext {
	t.Helper()
	homeDir := t.TempDir()
	return testContextWithHome(t, projectRoot, homeDir)
}

func testContextWithHome(t *testing.T, projectRoot, homeDir string) factory.WorkflowSourceContext {
	t.Helper()
	globalFactoryRoot, err := interfaces.NamedFactoriesRootForHome(homeDir)
	if err != nil {
		t.Fatalf("GlobalNamedFactoryRootForHome: %v", err)
	}
	globalWorkflowRoot, err := factory.GlobalWorkflowRootForHome(homeDir)
	if err != nil {
		t.Fatalf("GlobalWorkflowRootForHome: %v", err)
	}
	projectFactoryRoot, err := interfaces.ProjectFactoriesRootForWorkingDir(projectRoot)
	if err != nil {
		t.Fatalf("DefaultProjectNamedFactoryRoot: %v", err)
	}
	return factory.WorkflowSourceContext{
		ProjectRoot:         projectRoot,
		PackageRoot:         filepath.Join(projectRoot, interfaces.FactoryDir),
		ProjectWorkflowRoot: filepath.Join(projectRoot, factory.WorkflowSourceProjectClaudeWorkflowsDir),
		GlobalWorkflowRoot:  globalWorkflowRoot,
		ProjectFactoryRoot:  projectFactoryRoot,
		GlobalFactoryRoot:   globalFactoryRoot,
	}
}

func javascriptFactoryPayload(name, sourceRef string) string {
	return `{
		"name":"` + name + `",
		"orchestrator":{
			"kind":"JAVASCRIPT",
			"javascript":{
				"sourceRef":"` + sourceRef + `",
				"argsSchema":{"type":"object","properties":{"topic":{"type":"string"}},"required":["topic"]},
				"defaultPolicy":{"mode":"READ_ONLY","maxAgents":2,"concurrency":2}
			}
		}
	}`
}
