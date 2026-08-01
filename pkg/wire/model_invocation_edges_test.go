package wire

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"runtime"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	serviceedges "github.com/portpowered/infinite-you/pkg/services/edges"
	factorysessions "github.com/portpowered/infinite-you/pkg/services/factory_sessions"
	models "github.com/portpowered/infinite-you/pkg/services/models"
)

func TestModelsServiceIsConstructedOnceAndOpensRuntimeScopeOnSameRoot(t *testing.T) {
	t.Parallel()

	root, err := provideModelsService(serviceedges.Edges{})
	if err != nil {
		t.Fatalf("provideModelsService: %v", err)
	}
	if _, err := root.ListCatalog(context.Background(), models.ListModelsRequest{}); err == nil {
		t.Fatal("unbound Models service unexpectedly accepted a catalog operation")
	}
	opened, err := root.OpenRuntimeScope(context.Background(), models.OpenRuntimeScopeRequest{
		Config: models.RuntimeScopeConfig{
			CacheDirectory: t.TempDir(),
			Runtime:        models.RuntimeConfig{},
		},
	})
	if err != nil {
		t.Fatalf("OpenRuntimeScope: %v", err)
	}
	if opened.Scope.IsZero() {
		t.Fatal("OpenRuntimeScope returned a zero scope")
	}
	if _, err := root.ListCatalog(context.Background(), models.ListModelsRequest{
		Scope: opened.Scope,
	}); err != nil {
		t.Fatalf("same process-scoped Models root rejected its opened scope: %v", err)
	}
	closed, err := root.CloseRuntimeScope(context.Background(), models.CloseRuntimeScopeRequest{
		Scope: opened.Scope,
	})
	if err != nil {
		t.Fatalf("CloseRuntimeScope: %v", err)
	}
	if !closed.Closed || closed.Scope != opened.Scope {
		t.Fatalf("CloseRuntimeScope result = %#v, want issued scope closed", closed)
	}
}

type workingDirectoryOverride struct{}

func (*workingDirectoryOverride) Getwd() (string, error) { return "override", nil }

type artifactWriteCloser struct{ bytes.Buffer }

func (*artifactWriteCloser) Close() error { return nil }

type invocationArtifactFileSystemOverride struct {
	opened  string
	created string
	output  *artifactWriteCloser
}

type portableFileSystemOverride struct {
	platformfilesystem.Local
	walked bool
}

func (f *portableFileSystemOverride) WalkDir(string, fs.WalkDirFunc) error {
	f.walked = true
	return nil
}

func TestFactoryDefinitionPortableFileSystemPreservesOverrideAndSelectsDefault(t *testing.T) {
	t.Parallel()

	selectedDefault := provideFactoryDefinitionPortableFileSystem(serviceedges.Edges{})
	if _, ok := selectedDefault.(platformfilesystem.Local); !ok {
		t.Fatalf("default portable filesystem = %T, want platform local adapter", selectedDefault)
	}
	if err := selectedDefault.WalkDir(t.TempDir(), func(string, fs.DirEntry, error) error { return nil }); err != nil {
		t.Fatalf("default portable directory walker: %v", err)
	}

	override := &portableFileSystemOverride{}
	selected := provideFactoryDefinitionPortableFileSystem(serviceedges.Edges{
		FactoryDefinitionPortableFileSystem: override,
	})
	if selected != override {
		t.Fatal("portable filesystem override was not selected")
	}
	if err := selected.WalkDir("unused", nil); err != nil {
		t.Fatalf("portable directory walker override: %v", err)
	}
	if !override.walked {
		t.Fatal("portable directory walker override was not selected")
	}
}

func (s *invocationArtifactFileSystemOverride) Open(path string) (io.ReadCloser, error) {
	s.opened = path
	return io.NopCloser(bytes.NewBufferString("audio")), nil
}

func (s *invocationArtifactFileSystemOverride) Create(path string) (io.WriteCloser, error) {
	s.created = path
	s.output = &artifactWriteCloser{}
	return s.output, nil
}

func TestModelInvocationEdgesPreserveOverridesAndSelectPlatformDefaults(t *testing.T) {
	t.Parallel()

	if _, ok := provideFactorySessionsWorkingDirectory(serviceedges.Edges{}).(platformfilesystem.Local); !ok {
		t.Fatalf("default working-directory edge = %T, want platform filesystem adapter", provideFactorySessionsWorkingDirectory(serviceedges.Edges{}))
	}
	workingOverride := &workingDirectoryOverride{}
	if got := provideFactorySessionsWorkingDirectory(serviceedges.Edges{FactorySessionsWorkingDirectory: workingOverride}); got != workingOverride {
		t.Fatalf("working-directory override = %#v, want original override", got)
	}

	filesystemOverride := &invocationArtifactFileSystemOverride{}
	exporter, err := provideModelInvocationArtifactExporter(serviceedges.Edges{
		ModelInvocationArtifactFileSystem: filesystemOverride,
	})
	if err != nil {
		t.Fatalf("provideModelInvocationArtifactExporter: %v", err)
	}
	if err := exporter.ExportInvocationArtifact("runtime.wav", "customer.wav"); err != nil {
		t.Fatalf("ExportInvocationArtifact: %v", err)
	}
	if filesystemOverride.opened != "runtime.wav" || filesystemOverride.created != "customer.wav" || filesystemOverride.output.String() != "audio" {
		t.Fatalf("artifact override observed (%q, %q, %q)", filesystemOverride.opened, filesystemOverride.created, filesystemOverride.output.String())
	}
	if got := provideModelInvocationTimeout(); got != factorysessions.DefaultModelInvocationTimeout {
		t.Fatalf("model invocation timeout = %v, want %v", got, factorysessions.DefaultModelInvocationTimeout)
	}
}

func TestModelAssetHostPlatformPreservesOverrideAndSelectsProcessDefault(t *testing.T) {
	t.Parallel()

	if got := provideModelAssetHostPlatform(serviceedges.Edges{}); got != (models.AssetHostPlatform{
		OperatingSystem: runtime.GOOS,
		Architecture:    runtime.GOARCH,
	}) {
		t.Fatalf("default model asset host platform = %#v, want current process platform", got)
	}

	override := models.AssetHostPlatform{OperatingSystem: "customer-os", Architecture: "customer-arch"}
	if got := provideModelAssetHostPlatform(serviceedges.Edges{ModelAssetHostPlatform: override}); got != override {
		t.Fatalf("model asset host platform override = %#v, want %#v", got, override)
	}
}
