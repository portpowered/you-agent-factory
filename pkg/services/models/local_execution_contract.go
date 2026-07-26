package models

import (
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/work"
)

// LocalInvocationRequest is the plain infer request on the Models root.
// Peers supply Worker/dispatch/bindings vocabulary without importing nested
// inference or local-execution implementation types. Validate with
// ValidateLocalInvocationRequest before InvokeLocal when failing closed on
// managed-runtime inputs.
type LocalInvocationRequest struct {
	Scope            RuntimeScopeRef
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

// LocalInvocationResult is the plain infer result on the Models root. Handled
// true means Models owned the invocation and Content carries the Models-owned
// outcome; Handled false means Models declined and Workers continue with its
// normal runner. Readiness-blocked and unsupported-response-mode failures are
// distinct typed errors (ErrMissing, ErrLoading, ErrFailed, ErrUnsupported,
// ErrUnsupportedResponseMode), not Handled=false.
type LocalInvocationResult struct {
	Handled bool
	Content string
}

// ValidateLocalInvocationRequest checks the plain infer/local-invocation
// request. Managed-runtime workers with an empty Model fail closed as
// ErrNotFound without touching nested inference or local-execution packages.
// Non-managed workers remain valid so InvokeLocal can return Handled=false.
func ValidateLocalInvocationRequest(request LocalInvocationRequest) error {
	if request.Worker.UsesManagedRuntime() && strings.TrimSpace(request.Worker.Model) == "" {
		return fmt.Errorf("%w: empty managed runtime model name", ErrNotFound)
	}
	return nil
}
