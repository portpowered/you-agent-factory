// Package workeridentity owns the minimal Work identity read vocabulary.
package workeridentity

import "context"

// Work contains selected authority and presentation facts.
type Work struct {
	WorkID string
	Name   string
}

// Reader selects published identity without materializing unrelated Work.
type Reader interface {
	ReadWorkerSessionWork(context.Context, string) (Work, error)
}

// RuntimeResolver binds only the selected read capability.
type RuntimeResolver interface {
	ResolveWorkerSessionWorkRuntime(string) (Reader, error)
}

// AdapterResolver binds the private state-access adapter to the same reader.
type AdapterResolver interface {
	ResolveWorkerSessionWorkAdapter(string) (Reader, error)
}
