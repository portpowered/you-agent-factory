// Package wire constructs the private Workers Runners subservice.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/inference"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/mock"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/script"
	internalservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/service"
	agentwire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent/wire"
)

// NewService validates registrations into one immutable private registry.
func NewService(registrations []runners.Registration) (runners.Service, error) {
	return internalservice.New(registrations)
}

// Focused leaf providers construct one strategy, without registry assembly.
var (
	NewAgentRunner     = agentwire.NewService
	NewScriptRunner    = script.New
	NewInferenceRunner = inference.New
	NewMockRunner      = mock.New
)

type ScriptRunnerConfig = script.Config
type InferenceRunnerConfig = inference.Config
type MockRunnerConfig = mock.Config

// NewProductionRegistry registers already completed production strategies.
func NewProductionRegistry(agent, script, inference workers.Runner) (runners.Service, error) {
	return internalservice.NewProduction(agent, script, inference)
}

// NewMockProductionRegistry registers completed production and explicit Mock strategies.
func NewMockProductionRegistry(agent, script, inference, mock workers.Runner) (runners.Service, error) {
	return internalservice.NewMockProduction(agent, script, inference, mock)
}
