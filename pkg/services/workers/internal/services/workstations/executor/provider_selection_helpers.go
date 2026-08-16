package executor

import (
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

func cloneContinuation(reference *workerexecution.ProviderContinuationRef) *workerexecution.ProviderContinuationRef {
	if reference == nil {
		return nil
	}
	cloned := reference.Clone()
	return &cloned
}
