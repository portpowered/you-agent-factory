// Package wire constructs the private Workers Runners subservice.
package wire

import (
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/inference"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/mock"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/script"
	internalservice "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/service"
	agentwire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/internal/services/agent/wire"
)

// Focused providers expose the owning constructors without delegate wrappers.
// Registry constructors validate and snapshot completed strategies; leaf
// constructors build one strategy without registry assembly.
var (
	NewService                = internalservice.New
	NewProductionRegistry     = internalservice.NewProduction
	NewMockProductionRegistry = internalservice.NewMockProduction
	NewAgentRunner            = agentwire.NewService
	NewScriptRunner           = script.New
	NewInferenceRunner        = inference.New
	NewMockRunner             = mock.New
)

type ScriptRunnerConfig = script.Config
type InferenceRunnerConfig = inference.Config
type MockRunnerConfig = mock.Config
