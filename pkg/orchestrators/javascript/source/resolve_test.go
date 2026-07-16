package workflowsource_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
	"github.com/portpowered/infinite-you/pkg/orchestrators/javascript/source"
	cliworkflowsource "github.com/portpowered/infinite-you/pkg/transports/cli/workflowsource"
	"github.com/portpowered/infinite-you/pkg/transports/mapping"
)

const validWorkflowSource = `
meta({ name: "review", version: 1 });
phase("setup");
log("starting");
workflow.log("step");
workflow.artifact({ kind: "log", label: "step" });
const result = await agent.run({ prompt: "review" });
workflow.final({ ok: true, result });
pipeline([], function () {}, function () {});
`

func TestResolve_WorkflowName_UsesProjectClaudeWorkflowsFirst(t *testing.T) {
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "review.js"), []byte(validWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	ctx := testContext(t, projectRoot)
	resolution := workflowsource.Resolve(workflowsource.Request{
		Kind:  workflowsource.KindWorkflowName,
		Value: "review",
	}, ctx)

	if !resolution.Found {
		t.Fatalf("resolution = %#v, want found project workflow", resolution)
	}
	if resolution.LookupStage != workflowsource.LookupStageProjectClaude {
		t.Fatalf("lookup stage = %q, want %q", resolution.LookupStage, workflowsource.LookupStageProjectClaude)
	}
	if resolution.SourceRef != workflowsource.ProjectClaudeWorkflowsDir+"/review.js" {
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
	projectRoot := t.TempDir()
	homeDir := t.TempDir()
	projectDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	globalDir := filepath.Join(homeDir, ".you-agent-factory", "workflows")
	for _, dir := range []string{projectDir, globalDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
	}
	if err := os.WriteFile(filepath.Join(projectDir, "review.js"), []byte(validWorkflowSource), 0o600); err != nil {
		t.Fatalf("write project workflow: %v", err)
	}
	if err := os.WriteFile(filepath.Join(globalDir, "review.js"), []byte("meta({name:'other'});"), 0o600); err != nil {
		t.Fatalf("write global workflow: %v", err)
	}

	ctx := testContextWithHome(t, projectRoot, homeDir)
	resolution := workflowsource.Resolve(workflowsource.Request{
		Kind:  workflowsource.KindWorkflowName,
		Value: "review",
	}, ctx)
	if resolution.LookupStage != workflowsource.LookupStageProjectClaude {
		t.Fatalf("lookup stage = %q, want project precedence", resolution.LookupStage)
	}
}

func TestResolve_WorkflowName_ReportsConflictForMultipleJavaScriptFactories(t *testing.T) {
	projectRoot := t.TempDir()
	factoryRoot := filepath.Join(projectRoot, interfaces.FactoryDir)
	for _, name := range []string{"alpha-flow", "beta-flow"} {
		factoryDir, err := factoryconfig.PersistNamedFactory(factoryRoot, name, []byte(javascriptFactoryPayload(name, "workflows/review.js")))
		if err != nil {
			t.Fatalf("PersistNamedFactory(%s): %v", name, err)
		}
		workflowDir := filepath.Join(factoryDir, "workflows")
		if err := os.MkdirAll(workflowDir, 0o755); err != nil {
			t.Fatalf("mkdir workflows: %v", err)
		}
		if err := os.WriteFile(filepath.Join(workflowDir, "review.js"), []byte(validWorkflowSource), 0o600); err != nil {
			t.Fatalf("write workflow: %v", err)
		}
	}

	ctx := testContext(t, projectRoot)
	resolution := workflowsource.Resolve(workflowsource.Request{
		Kind:  workflowsource.KindWorkflowName,
		Value: "review",
	}, ctx)
	if resolution.Found {
		t.Fatalf("resolution = %#v, want conflict", resolution)
	}
	if len(resolution.Diagnostics) == 0 || resolution.Diagnostics[0].Code != workflowsource.CodeSourceConflict {
		t.Fatalf("diagnostics = %#v, want conflict", resolution.Diagnostics)
	}
}

func TestResolve_WorkflowFile_BypassesOrderedLookup(t *testing.T) {
	projectRoot := t.TempDir()
	packageDir := filepath.Join(projectRoot, interfaces.FactoryDir, interfaces.WorkflowsDir)
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sourcePath := filepath.Join(packageDir, "review.js")
	if err := os.WriteFile(sourcePath, []byte(validWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	ctx := testContext(t, projectRoot)
	resolution := workflowsource.Resolve(workflowsource.Request{
		Kind:  workflowsource.KindWorkflowFile,
		Value: interfaces.FactoryDir + "/workflows/review.js",
	}, ctx)
	if !resolution.Found || resolution.LookupStage != workflowsource.LookupStageExplicitSourceKind {
		t.Fatalf("resolution = %#v, want explicit workflow file bypass", resolution)
	}
	if resolution.SourceRef != interfaces.FactoryDir+"/workflows/review.js" {
		t.Fatalf("source ref = %q", resolution.SourceRef)
	}
}

func TestResolve_FactoryID_ResolvesJavaScriptFactory(t *testing.T) {
	projectRoot := t.TempDir()
	factoryRoot := filepath.Join(projectRoot, interfaces.FactoryDir)
	factoryDir, err := factoryconfig.PersistNamedFactory(factoryRoot, "review-flow", []byte(javascriptFactoryPayload("review-flow", "workflows/review.js")))
	if err != nil {
		t.Fatalf("PersistNamedFactory: %v", err)
	}
	workflowDir := filepath.Join(factoryDir, "workflows")
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "review.js"), []byte(validWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	ctx := testContext(t, projectRoot)
	resolution := workflowsource.Resolve(workflowsource.Request{
		Kind:  workflowsource.KindFactoryID,
		Value: "review-flow",
	}, ctx)
	if !resolution.Found || resolution.LookupStage != workflowsource.LookupStageExplicitFactory {
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

func TestResolve_InlineWorkflow_ComputesStableHash(t *testing.T) {
	ctx := testContext(t, t.TempDir())
	first := workflowsource.Resolve(workflowsource.Request{
		Kind:         workflowsource.KindInlineWorkflow,
		InlineSource: validWorkflowSource,
	}, ctx)
	second := workflowsource.Resolve(workflowsource.Request{
		Kind:         workflowsource.KindInlineWorkflow,
		InlineSource: validWorkflowSource,
	}, ctx)
	if !first.Found || first.SourceHash == "" || first.SourceHash != second.SourceHash {
		t.Fatalf("hashes = (%q, %q), want stable non-empty hash", first.SourceHash, second.SourceHash)
	}
}

func TestResolve_ArtifactRoot_RejectsSymlinkThatResolvesInsideRepo(t *testing.T) {
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

	resolution := workflowsource.Resolve(workflowsource.Request{
		Kind:         workflowsource.KindInlineWorkflow,
		InlineSource: validWorkflowSource,
		ArtifactRoot: outsideLink,
	}, ctx)
	if resolution.Found || resolution.ArtifactRoot.Allowed {
		t.Fatalf("symlinked artifact root = %#v, want rejection when resolved path is inside repo", resolution.ArtifactRoot)
	}
	if resolution.ArtifactRoot.Diagnostic == nil || resolution.ArtifactRoot.Diagnostic.Code != workflowsource.CodeArtifactRootInsideRepo {
		t.Fatalf("diagnostic = %#v, want inside-repo rejection", resolution.ArtifactRoot.Diagnostic)
	}
}

func TestResolve_ArtifactRoot_RejectsRelativeAndInsideRepoPaths(t *testing.T) {
	projectRoot := t.TempDir()
	ctx := testContext(t, projectRoot)

	relative := workflowsource.Resolve(workflowsource.Request{
		Kind:         workflowsource.KindInlineWorkflow,
		InlineSource: validWorkflowSource,
		ArtifactRoot: "artifacts",
	}, ctx)
	if relative.Found || relative.ArtifactRoot.Allowed {
		t.Fatalf("relative artifact root = %#v, want rejection", relative.ArtifactRoot)
	}

	insideRepo := filepath.Join(projectRoot, "outside-artifacts")
	inside := workflowsource.Resolve(workflowsource.Request{
		Kind:         workflowsource.KindInlineWorkflow,
		InlineSource: validWorkflowSource,
		ArtifactRoot: insideRepo,
	}, ctx)
	if inside.Found || inside.ArtifactRoot.Allowed {
		t.Fatalf("inside-repo artifact root = %#v, want rejection", inside.ArtifactRoot)
	}

	outsideRoot := filepath.Join(filepath.Dir(projectRoot), "workflow-artifacts")
	outside := workflowsource.Resolve(workflowsource.Request{
		Kind:         workflowsource.KindInlineWorkflow,
		InlineSource: validWorkflowSource,
		ArtifactRoot: outsideRoot,
	}, ctx)
	if !outside.Found || !outside.ArtifactRoot.Allowed {
		t.Fatalf("outside artifact root = %#v, want acceptance", outside.ArtifactRoot)
	}
}

func TestCLINormalizeRequest_MatchesAPISurface(t *testing.T) {
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "review.js"), []byte(validWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	req := workflowsource.Request{Kind: workflowsource.KindWorkflowName, Value: "review"}

	apiResolution := apisurface.NormalizeWorkflowSourceRequest(req, ctx)
	cliResolution := cliworkflowsource.NormalizeRequest(req, ctx)
	if apiResolution.SourceHash != cliResolution.SourceHash || apiResolution.SourceRef != cliResolution.SourceRef {
		t.Fatalf("api = %#v, cli = %#v, want equivalent normalization", apiResolution, cliResolution)
	}
}

func TestNormalizeWorkflowSourceRequest_MatchesDirectResolver(t *testing.T) {
	projectRoot := t.TempDir()
	workflowDir := filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir)
	if err := os.MkdirAll(workflowDir, 0o755); err != nil {
		t.Fatalf("mkdir workflows: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workflowDir, "review.js"), []byte(validWorkflowSource), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	ctx, err := workflowsource.DefaultContext(projectRoot)
	if err != nil {
		t.Fatalf("DefaultContext: %v", err)
	}
	req := workflowsource.Request{Kind: workflowsource.KindWorkflowName, Value: "review"}

	direct := workflowsource.Resolve(req, ctx)
	viaSurface := apisurface.NormalizeWorkflowSourceRequest(req, ctx)
	if direct.SourceHash != viaSurface.SourceHash || direct.SourceRef != viaSurface.SourceRef || direct.Found != viaSurface.Found {
		t.Fatalf("direct = %#v, surface = %#v, want equivalent resolution", direct, viaSurface)
	}
}

func testContext(t *testing.T, projectRoot string) workflowsource.Context {
	t.Helper()
	homeDir := t.TempDir()
	return testContextWithHome(t, projectRoot, homeDir)
}

func testContextWithHome(t *testing.T, projectRoot, homeDir string) workflowsource.Context {
	t.Helper()
	globalFactoryRoot, err := factoryconfig.GlobalNamedFactoryRootForHome(homeDir)
	if err != nil {
		t.Fatalf("GlobalNamedFactoryRootForHome: %v", err)
	}
	globalWorkflowRoot, err := factoryconfig.GlobalWorkflowRootForHome(homeDir)
	if err != nil {
		t.Fatalf("GlobalWorkflowRootForHome: %v", err)
	}
	projectFactoryRoot, err := factoryconfig.DefaultProjectNamedFactoryRoot(projectRoot)
	if err != nil {
		t.Fatalf("DefaultProjectNamedFactoryRoot: %v", err)
	}
	return workflowsource.Context{
		ProjectRoot:         projectRoot,
		PackageRoot:         filepath.Join(projectRoot, interfaces.FactoryDir),
		ProjectWorkflowRoot: filepath.Join(projectRoot, workflowsource.ProjectClaudeWorkflowsDir),
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
