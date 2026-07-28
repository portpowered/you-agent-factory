// Package runner is a transitional compile shim that re-exports runner selection
// policy helpers from the private runners destination. Canonical runner policy
// lives at pkg/services/workers; baseline deletion of this path is owned by
// DEL-WRK.
package runner

import (
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/runner"
)

var (
	V1BaselineCapabilities                  = workerrunner.V1BaselineCapabilities
	NewCapabilities                         = workerrunner.NewCapabilities
	BuiltInRunnerMetadata                   = workerrunner.BuiltInRunnerMetadata
	IsBuiltInRunnerID                       = workerrunner.IsBuiltInRunnerID
	ResolveOpenCodeAgent                    = workerrunner.ResolveOpenCodeAgent
	ValidateOpenCodeAgentForRunnerSelection = workerrunner.ValidateOpenCodeAgentForRunnerSelection
	ResolveRunnerSelection                  = workerrunner.ResolveRunnerSelection
	NormalizeRunnerID                       = workerrunner.NormalizeRunnerID
)
