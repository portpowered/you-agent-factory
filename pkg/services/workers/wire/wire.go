// Package wire is the Workers service composition boundary.
//
// Wire performs construction only, returns the singular workers.Service root
// interface, and starts no lifecycle components. Parent-private runtime_assembly,
// workstations, and runners (agent/script/inference) owner wiring stays inside
// the owner service assembly path; peers depend on Service rather than owner
// internals or construction ports. Hosted runner ownership is not constructed.
package wire

import (
	"context"
	"fmt"
	"strings"

	"github.com/portpowered/infinite-you/pkg/services/workers"
	"github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners"
	runnerswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runners/wire"
	runtimeassemblywire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/runtime_assembly/wire"
	workstationswire "github.com/portpowered/infinite-you/pkg/services/workers/internal/services/workstations/wire"
	workerservice "github.com/portpowered/infinite-you/pkg/services/workers/service"
)

// NewService constructs an inert Workers root from construction ports. It
// composes the accepted root through parent-private runtime_assembly,
// workstations, and runners (agent/script/inference) owner construction
// without publishing owner types on the returned peer surface. Missing required
// construction ports fail with a deterministic construction error and a nil
// service.
func NewService(
	agentDependencies runners.AgentDependencies,
	scriptConfig runners.ScriptConfig,
	scriptDependencies runners.ScriptDependencies,
	inferenceConfig runners.InferenceConfig,
	inferenceDependencies runners.InferenceDependencies,
) (workers.Service, error) {
	if err := validateConstructionPorts(
		agentDependencies,
		scriptConfig,
		scriptDependencies,
		inferenceConfig,
		inferenceDependencies,
	); err != nil {
		return nil, err
	}
	agentRegistry, err := runnerswire.NewAgentRegistry(agentDependencies)
	if err != nil {
		return nil, fmt.Errorf("construct Workers: %w", err)
	}
	scriptRegistry, err := runnerswire.NewScriptRegistry(scriptConfig, scriptDependencies)
	if err != nil {
		return nil, fmt.Errorf("construct Workers: %w", err)
	}
	inferenceRegistry, err := runnerswire.NewInferenceRegistry(inferenceConfig, inferenceDependencies)
	if err != nil {
		return nil, fmt.Errorf("construct Workers: %w", err)
	}
	runnerRegistry, err := combineRunnerRegistries(agentRegistry, scriptRegistry, inferenceRegistry)
	if err != nil {
		return nil, err
	}
	runtimeAssembly, err := runtimeassemblywire.NewService(runnerRegistry, defaultBindingAssembler)
	if err != nil {
		return nil, err
	}
	return workerservice.NewRoot(runtimeAssembly, workstationswire.NewService())
}

func validateConstructionPorts(
	agentDependencies runners.AgentDependencies,
	scriptConfig runners.ScriptConfig,
	scriptDependencies runners.ScriptDependencies,
	inferenceConfig runners.InferenceConfig,
	inferenceDependencies runners.InferenceDependencies,
) error {
	if agentDependencies.Providers == nil {
		return fmt.Errorf("construct Workers: agent Providers service is required")
	}
	if agentDependencies.Publish == nil {
		return fmt.Errorf("construct Workers: agent progress publisher is required")
	}
	if strings.TrimSpace(scriptConfig.Command) == "" {
		return fmt.Errorf("construct Workers: script command is required")
	}
	if scriptDependencies.CommandRunner == nil {
		return fmt.Errorf("construct Workers: script command runner is required")
	}
	if scriptDependencies.FactoryDocs == nil {
		return fmt.Errorf("construct Workers: script Factory docs loader is required")
	}
	if scriptDependencies.Now == nil {
		return fmt.Errorf("construct Workers: script clock is required")
	}
	if scriptDependencies.Publish == nil {
		return fmt.Errorf("construct Workers: script progress publisher is required")
	}
	if scriptDependencies.Record == nil {
		return fmt.Errorf("construct Workers: script event recorder is required")
	}
	if strings.TrimSpace(inferenceConfig.Worker.Name) == "" {
		return fmt.Errorf("construct Workers: inference worker name is required")
	}
	if inferenceDependencies.Models == nil {
		return fmt.Errorf("construct Workers: inference Models service is required")
	}
	return nil
}

func combineRunnerRegistries(
	agentRegistry runners.Service,
	scriptRegistry runners.Service,
	inferenceRegistry runners.Service,
) (runners.Service, error) {
	registrations := make([]runners.Registration, 0, 3)
	for _, entry := range []struct {
		service  runners.Service
		identity string
	}{
		{agentRegistry, runners.AgentIdentity},
		{scriptRegistry, runners.ScriptIdentity},
		{inferenceRegistry, runners.InferenceIdentity},
	} {
		binding, err := entry.service.Resolve(runners.ResolutionRequest{
			Identity: entry.identity,
		})
		if err != nil {
			return nil, fmt.Errorf(
				"construct Workers runner registry: resolve %s runner: %w",
				entry.identity,
				err,
			)
		}
		registrations = append(registrations, runners.Registration{
			Identity: binding.Identity,
			Metadata: binding.Metadata,
			Runner:   binding.Runner,
		})
	}
	return runnerswire.NewService(registrations)
}

func defaultBindingAssembler(
	_ context.Context,
	role workers.RuntimeBuildRoleRequest,
	_ workers.RuntimeBuildOpeningOptions,
	selection workers.ResolvedRunnerSelection,
) (workers.AssembledRuntimeBinding, error) {
	return workers.AssembledRuntimeBinding{
		RoleName:        role.Name,
		RoleKind:        role.Kind,
		RunnerSelection: selection,
	}, nil
}
