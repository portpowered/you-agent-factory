package portableconfig

import (
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestApplyStarterWorkReplacesInputEntriesWithEligibleFiles(t *testing.T) {
	t.Parallel()

	factoryDir := t.TempDir()
	writeStarterWorkFile(t, factoryDir, "story/request/payload.json", `{"title":"portable"}`)
	writeStarterWorkFile(t, factoryDir, "story/request/draft.tmp", "ignored")
	writeStarterWorkFile(t, factoryDir, "story/payload.json", "wrong shape")
	writeStarterWorkFile(t, factoryDir, "unknown/request/payload.json", "unknown Work type")

	cfg := &factorydefinitions.FactoryConfig{
		WorkTypes: []factorydefinitions.WorkTypeConfig{{Name: "story"}},
		ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{
			BundledFiles: []factorydefinitions.BundledFileConfig{
				{
					Type:       factorydefinitions.BundledFileTypeInput,
					TargetPath: "factory/inputs/story/stale/payload.json",
				},
				{
					Type:       factorydefinitions.BundledFileTypeScript,
					TargetPath: "factory/scripts/run.sh",
				},
			},
		},
	}

	if err := applyStarterWork(factoryDir, cfg, platformfilesystem.Local{}); err != nil {
		t.Fatalf("ApplyStarterWork: %v", err)
	}

	files := cfg.ResourceManifest.BundledFiles
	if len(files) != 2 {
		t.Fatalf("bundled files = %#v, want retained script and collected input", files)
	}
	if files[0].TargetPath != "factory/inputs/story/request/payload.json" {
		t.Fatalf("input target = %q", files[0].TargetPath)
	}
	if files[0].Type != factorydefinitions.BundledFileTypeInput {
		t.Fatalf("input type = %q", files[0].Type)
	}
	if files[0].Content.Encoding != factorydefinitions.BundledFileEncodingUTF8 ||
		files[0].Content.Inline != `{"title":"portable"}` {
		t.Fatalf("input content = %#v", files[0].Content)
	}
	if files[1].TargetPath != "factory/scripts/run.sh" {
		t.Fatalf("retained target = %q", files[1].TargetPath)
	}
}

func TestApplyStarterWorkRemovesStaleInputsWhenInputsDirectoryIsAbsent(t *testing.T) {
	t.Parallel()

	cfg := &factorydefinitions.FactoryConfig{
		ResourceManifest: &factorydefinitions.PortableResourceManifestConfig{
			BundledFiles: []factorydefinitions.BundledFileConfig{{
				Type:       factorydefinitions.BundledFileTypeInput,
				TargetPath: "factory/inputs/story/stale/payload.json",
			}},
		},
	}

	if err := applyStarterWork(t.TempDir(), cfg, platformfilesystem.Local{}); err != nil {
		t.Fatalf("ApplyStarterWork: %v", err)
	}
	if len(cfg.ResourceManifest.BundledFiles) != 0 {
		t.Fatalf("bundled files = %#v, want stale inputs removed", cfg.ResourceManifest.BundledFiles)
	}
}

func writeStarterWorkFile(t *testing.T, factoryDir, relativePath, content string) {
	t.Helper()

	path := filepath.Join(factoryDir, factorydefinitions.InputsDir, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir starter Work parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write starter Work: %v", err)
	}
}
