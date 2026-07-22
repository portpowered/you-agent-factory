package workflowvalidation

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SourceReader reads workflow source content for factory-relative refs.
type SourceReader interface {
	ReadWorkflowSource(sourceRef string) (string, error)
}

// SourceFileSystem is the exact read effect used by a relative workflow source
// reader. The JavaScript workflow service supplies its Wire-selected adapter.
type SourceFileSystem interface {
	ReadDir(string) ([]fs.DirEntry, error)
	ReadFile(string) ([]byte, error)
	Stat(string) (fs.FileInfo, error)
}

// FileSourceReader returns a reader that resolves workflow source refs relative to rootDir.
func FileSourceReader(rootDir string, files SourceFileSystem) SourceReader {
	if files == nil {
		panic("Factory Runtime workflow source filesystem is required")
	}
	return fileSourceReader{rootDir: rootDir, files: files}
}

type fileSourceReader struct {
	rootDir string
	files   SourceFileSystem
}

func (r fileSourceReader) ReadWorkflowSource(sourceRef string) (string, error) {
	ref := strings.TrimSpace(sourceRef)
	if ref == "" {
		return "", fmt.Errorf("workflow source ref is empty")
	}
	if filepath.IsAbs(ref) {
		return "", fmt.Errorf("workflow source ref must be factory-relative, not absolute")
	}
	clean := filepath.Clean(ref)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("workflow source ref %q escapes the factory root", sourceRef)
	}
	fullPath := filepath.Join(r.rootDir, clean)
	content, err := r.files.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
