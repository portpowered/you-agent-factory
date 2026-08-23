// Package testkit exposes Factory Runtime fixture construction under the owner
// internal tree so tests do not depend on a public service-root test-support package.
package testkit

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	factoryruntimejavascript "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/javascript"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/javascript/validation"
)

// JavaScriptWorkflows constructs the concrete stateless JavaScript
// orchestrator capability for focused tests.
func JavaScriptWorkflows() factoryruntime.JavaScriptWorkflows {
	return factoryruntimejavascript.New(nil, nil, nil)
}

// NewFileWorkflowSourceReader constructs a filesystem-backed workflow source
// reader for validation tests.
func NewFileWorkflowSourceReader(root string, files workflowvalidation.SourceFileSystem) factoryruntime.WorkflowSourceReader {
	return workflowvalidation.FileSourceReader(root, files)
}

// HashWorkflowSource returns the canonical workflow source digest for fixture
// assertions.
func HashWorkflowSource(source []byte) string {
	return workflowvalidation.SourceHash(source)
}
