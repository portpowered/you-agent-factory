package factorysessionexecution

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	interfaces "github.com/portpowered/infinite-you/pkg/services/factory_definitions"
	factory "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
)

type workflowBundleFileSystem struct{}

func (workflowBundleFileSystem) ReadDir(path string) ([]fs.DirEntry, error) {
	return os.ReadDir(path)
}

func (workflowBundleFileSystem) ReadFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func (workflowBundleFileSystem) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func workflowValidationLoadRequest(
	source Source,
	resolution factory.WorkflowSourceResolution,
	content string,
) factory.WorkflowValidationLoadRequest {
	request := factory.WorkflowValidationLoadRequest{
		SourceRef: resolution.SourceRef,
		Content:   content,
	}
	factoryRoot, entrySourceRef := factoryRootForWorkflowSource(source, resolution.SourceRef)
	if factoryRoot == "" {
		return request
	}
	if entrySourceRef != "" {
		request.SourceRef = entrySourceRef
	}
	request.FactoryRoot = factoryRoot
	request.BundleReader = factory.NewWorkflowSourceReader(factoryRoot, workflowBundleFileSystem{})
	return request
}

func factoryRootForWorkflowSource(source Source, resolvedSourceRef string) (string, string) {
	if source.Kind != factory.WorkflowSourceKindWorkflowFile {
		return "", ""
	}
	workflowFile := strings.TrimSpace(source.WorkflowFile)
	if workflowFile == "" {
		return "", ""
	}
	if !filepath.IsAbs(workflowFile) {
		return "", ""
	}
	factoryRoot := findFactoryRoot(filepath.Dir(workflowFile))
	if factoryRoot == "" {
		return "", ""
	}
	entrySourceRef, err := filepath.Rel(factoryRoot, workflowFile)
	if err != nil {
		return factoryRoot, filepath.ToSlash(strings.TrimSpace(resolvedSourceRef))
	}
	return factoryRoot, filepath.ToSlash(entrySourceRef)
}

func findFactoryRoot(startDir string) string {
	dir := filepath.Clean(startDir)
	for {
		if _, err := os.Stat(filepath.Join(dir, interfaces.FactoryConfigFile)); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
