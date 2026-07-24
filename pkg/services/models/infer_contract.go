package models

import (
	"fmt"
	"strings"
)

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
