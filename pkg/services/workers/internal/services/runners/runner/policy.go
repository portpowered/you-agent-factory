package runner

import (
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

var (
	V1BaselineCapabilities = workerexecution.V1BaselineCapabilities
	NewCapabilities        = workerexecution.NewCapabilities
	BuiltInRunnerMetadata  = workerexecution.BuiltInRunnerMetadata
	IsBuiltInRunnerID      = workerexecution.IsBuiltInRunnerID
	ResolveRunnerSelection = workerexecution.ResolveRunnerSelection
	NormalizeRunnerID      = workerexecution.NormalizeRunnerID
)
