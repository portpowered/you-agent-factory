package models

import "github.com/portpowered/infinite-you/pkg/services/work"

// LocalInvocationRequest describes one possible local-model invocation.
type LocalInvocationRequest struct {
	Holder           string
	Worker           LocalWorker
	Resources        []LocalResource
	Dispatch         work.WorkDispatch
	ModelOperation   string
	ModelBindings    []ResolvedModelOperationBinding
	WorkingDirectory string
}

// LocalWorker is the Models-owned projection of the authored Worker fields
// required to select and invoke a managed local runtime.
type LocalWorker struct {
	Name          string
	Type          string
	Model         string
	ModelLocality string
	Resources     []LocalResource
}

func (worker LocalWorker) UsesManagedRuntime() bool {
	return RuntimeWorker{Type: worker.Type, ModelLocality: worker.ModelLocality}.UsesManagedRuntime()
}

// LocalResource is the Models-owned projection of a Factory resource.
type LocalResource struct {
	ID         string
	Name       string
	Type       string
	Capacity   int
	Model      string
	Backend    string
	LoadPolicy string
	Provider   string
}

// LocalInvocationResult reports whether Models owned the invocation.
// When Handled is false, Workers should continue with its normal runner.
type LocalInvocationResult struct {
	Handled bool
	Content string
}
