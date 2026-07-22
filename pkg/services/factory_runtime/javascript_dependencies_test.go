package factory_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimejavascript "github.com/portpowered/infinite-you/pkg/services/factory_runtime/javascript"
)

type localWorkflowSourceFiles struct{}

func (localWorkflowSourceFiles) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (localWorkflowSourceFiles) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (localWorkflowSourceFiles) Stat(path string) (fs.FileInfo, error)      { return os.Stat(path) }

func testJavaScriptWorkflows() factory.JavaScriptWorkflows {
	return factoryruntimejavascript.New(localWorkflowSourceFiles{}, os.UserHomeDir, filepath.EvalSymlinks)
}

func TestJavaScriptWorkflows_DefaultSourceContextRequiresSymlinkResolver(t *testing.T) {
	t.Parallel()
	workflows := factoryruntimejavascript.New(localWorkflowSourceFiles{}, os.UserHomeDir, nil)
	if _, err := workflows.DefaultSourceContext(t.TempDir()); err == nil {
		t.Fatal("DefaultSourceContext error = nil, want missing symlink resolver failure")
	}
}

func TestJavaScriptWorkflows_PreviewWorkflowOwnsSourceContextDefaults(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	workflows := testJavaScriptWorkflows()

	preview, err := workflows.PreviewWorkflow(context.Background(), factory.WorkflowPreviewInput{
		ProjectRoot: projectRoot,
		Source: factory.WorkflowSourceRequest{
			Kind:         factory.WorkflowSourceKindInlineWorkflow,
			InlineSource: sourceValidWorkflowSource,
		},
	})
	if err != nil {
		t.Fatalf("PreviewWorkflow: %v", err)
	}
	if !preview.Valid || !preview.SourceResolution.Found {
		t.Fatalf("preview = %#v, want valid inline source resolved with service-owned defaults", preview)
	}
}

func TestJavaScriptWorkflows_PreviewWorkflowRequiresProjectRoot(t *testing.T) {
	t.Parallel()
	_, err := testJavaScriptWorkflows().PreviewWorkflow(context.Background(), factory.WorkflowPreviewInput{})
	if err == nil || err.Error() != "project root is required" {
		t.Fatalf("PreviewWorkflow error = %v, want project root requirement", err)
	}
}

func TestJavaScriptWorkflows_PreviewWorkflowRequiresCallerContext(t *testing.T) {
	t.Parallel()
	_, err := testJavaScriptWorkflows().PreviewWorkflow(nil, factory.WorkflowPreviewInput{ProjectRoot: t.TempDir()})
	if err == nil || err.Error() != "workflow preview context is required" {
		t.Fatalf("PreviewWorkflow(nil) error = %v, want required-context failure", err)
	}
}

func TestJavaScriptWorkflows_PreviewWorkflowHonorsCallerCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := testJavaScriptWorkflows().PreviewWorkflow(ctx, factory.WorkflowPreviewInput{ProjectRoot: t.TempDir()})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("PreviewWorkflow(canceled) error = %v, want context.Canceled", err)
	}
}

func TestJavaScriptWorkflows_ResolveSourceUsesInjectedSymlinkResolver(t *testing.T) {
	t.Parallel()
	projectRoot := t.TempDir()
	requestedRoot := filepath.Join(filepath.Dir(projectRoot), "external-artifacts")
	resolverCalls := 0
	resolveSymlinks := func(path string) (string, error) {
		resolverCalls++
		if filepath.Clean(path) == filepath.Clean(requestedRoot) {
			return projectRoot, nil
		}
		return path, nil
	}
	workflows := factoryruntimejavascript.New(localWorkflowSourceFiles{}, os.UserHomeDir, resolveSymlinks)

	resolution := workflows.ResolveSource(factory.WorkflowSourceRequest{
		Kind:         factory.WorkflowSourceKindInlineWorkflow,
		InlineSource: sourceValidWorkflowSource,
		ArtifactRoot: requestedRoot,
	}, factory.WorkflowSourceContext{ProjectRoot: projectRoot})

	if resolution.ArtifactRoot.Allowed || resolution.ArtifactRoot.Diagnostic == nil {
		t.Fatalf("artifact root = %#v, want injected resolved-inside-project rejection", resolution.ArtifactRoot)
	}
	if resolution.ArtifactRoot.Diagnostic.Code != factory.WorkflowSourceCodeArtifactRootInsideRepo {
		t.Fatalf("diagnostic code = %q, want %q", resolution.ArtifactRoot.Diagnostic.Code, factory.WorkflowSourceCodeArtifactRootInsideRepo)
	}
	if resolverCalls == 0 {
		t.Fatal("symlink resolver calls = 0, want injected resolver use")
	}
}
