package portableconfig

import (
	"os"
	"path/filepath"
	"testing"

	platformfilesystem "github.com/portpowered/infinite-you/pkg/platform/filesystem"
	factorydefinitions "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
)

func TestApplySupportedFilesCollectsPortableFactoryContent(t *testing.T) {
	t.Parallel()

	factoryDir := filepath.Join(t.TempDir(), "factory")
	writePortableContentFile(t, factoryDir, "scripts/run.sh", "echo portable\n")
	writePortableContentFile(t, factoryDir, "scripts/ignored.tmp", "ignored")
	writePortableContentFile(t, factoryDir, "docs/README.md", "# Portable\n")
	writePortableContentFile(
		t,
		factoryDir,
		"portable-dependencies.json",
		`{"tools":["go"]}`,
	)

	cfg := &factorydefinitions.FactoryConfig{}
	if err := applySupportedFiles(
		factoryDir,
		cfg,
		true,
		true,
		platformfilesystem.Local{},
	); err != nil {
		t.Fatalf("ApplySupportedFiles: %v", err)
	}

	if cfg.ResourceManifest == nil {
		t.Fatal("resource manifest is nil")
	}
	files := cfg.ResourceManifest.BundledFiles
	if len(files) != 3 {
		t.Fatalf("bundled files = %#v, want doc, helper, and script", files)
	}
	assertPortableContentFile(
		t,
		files[0],
		factorydefinitions.BundledFileTypeDoc,
		"factory/docs/README.md",
		"# Portable\n",
	)
	assertPortableContentFile(
		t,
		files[1],
		factorydefinitions.BundledFileTypeRootHelper,
		"factory/portable-dependencies.json",
		`{"tools":["go"]}`,
	)
	assertPortableContentFile(
		t,
		files[2],
		factorydefinitions.BundledFileTypeScript,
		"factory/scripts/run.sh",
		"echo portable\n",
	)
}

func writePortableContentFile(t *testing.T, factoryDir, relativePath, content string) {
	t.Helper()

	path := filepath.Join(factoryDir, filepath.FromSlash(relativePath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir portable content parent: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write portable content: %v", err)
	}
}

func assertPortableContentFile(
	t *testing.T,
	got factorydefinitions.BundledFileConfig,
	wantType string,
	wantTarget string,
	wantInline string,
) {
	t.Helper()

	if got.Type != wantType ||
		got.TargetPath != wantTarget ||
		got.Content.Encoding != factorydefinitions.BundledFileEncodingUTF8 ||
		got.Content.Inline != wantInline {
		t.Fatalf(
			"bundled file = %#v, want type=%q target=%q inline=%q",
			got,
			wantType,
			wantTarget,
			wantInline,
		)
	}
}
