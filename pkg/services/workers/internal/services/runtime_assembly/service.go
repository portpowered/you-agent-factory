// Package runtimeassembly defines the Workers-owned private capability for
// inert assembly of detached runtime bindings from explicit root inputs.
package runtimeassembly

import (
	"context"

	"github.com/portpowered/infinite-you/pkg/services/workers"
)

// RunnerResolver resolves one explicit runner identity without consulting
// Factory Session state. The bool reports whether the identity is recognized.
type RunnerResolver func(
	context.Context,
	string,
) (workers.ResolvedRunnerSelection, bool, error)

// BindingAssembler constructs one inert binding from a snapshotted request.
// It must not start execution or retain mutable opening options.
type BindingAssembler func(
	context.Context,
	workers.RuntimeBuildRoleRequest,
	workers.RuntimeBuildOpeningOptions,
	workers.ResolvedRunnerSelection,
) (workers.AssembledRuntimeBinding, error)

// Service atomically assembles detached runtime binding snapshots.
type Service interface {
	Build(
		context.Context,
		workers.RuntimeBuildRequest,
	) (workers.RuntimeBuildResult, error)
}
