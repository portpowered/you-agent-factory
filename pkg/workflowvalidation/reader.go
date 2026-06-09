package workflowvalidation

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FileSourceReader returns a reader that resolves workflow source refs relative to rootDir.
func FileSourceReader(rootDir string) factorySourceReader {
	return factorySourceReader{rootDir: rootDir}
}

type factorySourceReader struct {
	rootDir string
}

func (r factorySourceReader) ReadWorkflowSource(sourceRef string) (string, error) {
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
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	return string(content), nil
}
