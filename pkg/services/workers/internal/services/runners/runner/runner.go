// Package runner retains same-service compatibility names for the private
// Workers Runners subservice while canonical runner selection policy lives at
// pkg/services/workers.
package runner

import workers "github.com/portpowered/infinite-you/pkg/services/workers"

var V1BaselineCapabilities = workers.V1BaselineCapabilities
var NewCapabilities = workers.NewCapabilities
var BuiltInRunnerMetadata = workers.BuiltInRunnerMetadata
var IsBuiltInRunnerID = workers.IsBuiltInRunnerID
var ResolveOpenCodeAgent = workers.ResolveOpenCodeAgent
var ValidateOpenCodeAgentForRunnerSelection = workers.ValidateOpenCodeAgentForRunnerSelection
var ResolveRunnerSelection = workers.ResolveRunnerSelection
var NormalizeRunnerID = workers.NormalizeRunnerID
