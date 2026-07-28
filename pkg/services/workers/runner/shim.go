// Package runner is a transitional compile shim that re-exports runner selection
// policy helpers from the published Workers root contract. Canonical runner
// policy lives at pkg/services/workers/runner_policy_contracts.go and under
// pkg/services/workers/internal/services/runners/runner; baseline deletion of
// this path is owned by DEL-WRK.
package runner

import (
	"github.com/portpowered/infinite-you/pkg/services/workers"
	workerrunner "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/runner"
)

var (
	V1BaselineCapabilities                  = workers.V1BaselineCapabilities
	NewCapabilities                         = workers.NewCapabilities
	BuiltInRunnerMetadata                   = workers.BuiltInRunnerMetadata
	BuiltInRunnerStatus                     = workerrunner.BuiltInRunnerStatus
	IsBuiltInRunnerID                       = workers.IsBuiltInRunnerID
	ResolveOpenCodeAgent                    = workers.ResolveOpenCodeAgent
	ValidateOpenCodeAgentForRunnerSelection = workers.ValidateOpenCodeAgentForRunnerSelection
	ResolveRunnerSelection                  = workers.ResolveRunnerSelection
	NormalizeRunnerID                       = workers.NormalizeRunnerID
	ValidateBuiltInRunnerPrerequisites      = workerrunner.ValidateBuiltInRunnerPrerequisites
)
