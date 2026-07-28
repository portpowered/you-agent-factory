// Package construction is a transitional compile shim that re-exports the
// runtime-assembly construction implementation from the private destination.
// Peers should construct through workers/wire; baseline deletion of this path is
// owned by DEL-WRK.
package construction

import (
	workerconstruction "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/construction"
)

// Builder constructs one configured worker without owning runtime lifecycle.
type Builder = workerconstruction.Builder

// RunnerDecorator adds session-owned behavior to a provider-backed runner.
type RunnerDecorator = workerconstruction.RunnerDecorator

// Result exposes both the dispatch executor and its direct-invocation boundary.
type Result = workerconstruction.Result

// Service is a stateless worker executor constructor.
type Service = workerconstruction.Service

// New constructs a worker executor service from process-owned factories.
var New = workerconstruction.New
