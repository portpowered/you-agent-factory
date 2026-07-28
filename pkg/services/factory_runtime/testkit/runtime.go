// Package testkit exposes Factory Runtime fixture construction without placing
// concrete implementation constructors at the service root.
package testkit

import (
	factoryruntime "github.com/portpowered/infinite-you/pkg/services/factory_runtime"
	workflowvalidation "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/javascript/validation"
	petri "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/orchestrators/petri"
	factoryruntimejavascript "github.com/portpowered/infinite-you/pkg/services/factory_runtime/internal/services/orchestration/javascript"
)

// JavaScriptWorkflows constructs the concrete stateless JavaScript
// orchestrator capability for focused tests.
func JavaScriptWorkflows() factoryruntime.JavaScriptWorkflows {
	return factoryruntimejavascript.New(nil, nil, nil)
}

// NewPetriMarking constructs a Petri marking for tests that exercise the
// public Factory Runtime state contract.
func NewPetriMarking(netID string) *factoryruntime.PetriMarking {
	return petri.NewMarking(netID)
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
