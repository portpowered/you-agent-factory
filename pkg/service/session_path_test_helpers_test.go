package service

import (
	"path/filepath"
	"testing"

	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
)

func assertAbsoluteFactorySessionPaths(
	t *testing.T,
	summary *factoryapi.FactorySessionSummary,
	wantFolderPath string,
	wantFactoryDir string,
) {
	t.Helper()
	if summary == nil {
		t.Fatal("summary is required")
	}
	if !filepath.IsAbs(summary.FolderPath) {
		t.Fatalf("folderPath = %q, want absolute path", summary.FolderPath)
	}
	if !filepath.IsAbs(summary.FactoryDir) {
		t.Fatalf("factoryDir = %q, want absolute path", summary.FactoryDir)
	}
	if got, want := cleanResolvedPath(summary.FolderPath), cleanResolvedPath(wantFolderPath); got != want {
		t.Fatalf("folderPath = %q, want %q", got, want)
	}
	if got, want := cleanResolvedPath(summary.FactoryDir), cleanResolvedPath(wantFactoryDir); got != want {
		t.Fatalf("factoryDir = %q, want %q", got, want)
	}
}

func cleanResolvedPath(path string) string {
	cleaned := filepath.Clean(path)
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return cleaned
	}
	return filepath.Clean(resolved)
}
