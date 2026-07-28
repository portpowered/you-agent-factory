package runner

import (
	workerexecution "github.com/portpowered/infinite-you/pkg/services/workers"
)

var (
	V1BaselineCapabilities                  = workerexecution.V1BaselineCapabilities
	NewCapabilities                         = workerexecution.NewCapabilities
	BuiltInRunnerMetadata                   = workerexecution.BuiltInRunnerMetadata
	IsBuiltInRunnerID                       = workerexecution.IsBuiltInRunnerID
	ResolveOpenCodeAgent                    = workerexecution.ResolveOpenCodeAgent
	ValidateOpenCodeAgentForRunnerSelection = workerexecution.ValidateOpenCodeAgentForRunnerSelection
	ResolveRunnerSelection                  = workerexecution.ResolveRunnerSelection
	NormalizeRunnerID                       = workerexecution.NormalizeRunnerID
)
