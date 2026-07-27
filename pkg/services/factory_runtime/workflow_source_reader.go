package factory

import workflowvalidation "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/javascript/validation"

// NewWorkflowSourceReader returns a reader that resolves workflow source refs
// relative to one factory root directory.
func NewWorkflowSourceReader(rootDir string, files WorkflowSourceFileSystem) WorkflowSourceReader {
	return workflowvalidation.FileSourceReader(rootDir, files)
}
