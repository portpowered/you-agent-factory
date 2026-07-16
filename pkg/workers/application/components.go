// Package application owns the constructed worker application dependencies
// shared by production and functional process graphs.
package application

import (
	"fmt"
	"net/http"

	"github.com/jonboulle/clockwork"
	"github.com/portpowered/infinite-you/pkg/workers"
	"github.com/portpowered/infinite-you/pkg/workers/agypty"
	workerexecutor "github.com/portpowered/infinite-you/pkg/workers/executor"
	hostedworkers "github.com/portpowered/infinite-you/pkg/workers/hosted"
	workerprocess "github.com/portpowered/infinite-you/pkg/workers/process"
	workerprovider "github.com/portpowered/infinite-you/pkg/workers/provider"
	"go.uber.org/zap"
)

// Edges contains the process-selected side-effect boundaries. Nil and empty
// values use the same package-owned defaults as production.
type Edges struct {
	ProviderCommandRunner workers.CommandRunner
	ScriptCommandRunner   workers.CommandRunner
	AgyPTYAllocator       agypty.PTYAllocator
	HostedHTTPClient      *http.Client
	HostedLinearEndpoint  string
	HostedSecretResolver  hostedworkers.SecretResolver
	HostedClock           clockwork.Clock
}

// Components contains validated factories and hosted dependencies consumed by
// runtime construction without selecting production versus functional edges.
type Components struct {
	Provider                *workerprovider.Factory
	Script                  *workerexecutor.ScriptFactory
	Hosted                  hostedworkers.Config
	ProviderCommandInjected bool
}

// New constructs the shared worker application components.
func New(logger *zap.Logger, edges Edges) (Components, error) {
	var providerFactory *workerprovider.Factory
	var err error
	if edges.ProviderCommandRunner == nil && edges.AgyPTYAllocator == nil {
		providerFactory, err = workerprovider.NewProductionFactory()
	} else {
		providerRunner := edges.ProviderCommandRunner
		if providerRunner == nil {
			providerRunner = workerprocess.ExecCommandRunner{}
		}
		allocator := edges.AgyPTYAllocator
		if allocator == nil {
			allocator, err = agypty.NewDefaultPlatformAllocatorFactory().NewAllocator()
			if err != nil {
				return Components{}, fmt.Errorf("construct worker application: create Agy PTY allocator: %w", err)
			}
		}
		providerFactory, err = workerprovider.NewFactory(workerprovider.ConstructionInput{
			CommandRunner: providerRunner, AgyPTYAllocator: allocator,
		})
	}
	if err != nil {
		return Components{}, fmt.Errorf("construct worker application: %w", err)
	}
	var scriptFactory *workerexecutor.ScriptFactory
	if edges.ScriptCommandRunner == nil {
		scriptFactory, err = workerexecutor.NewProductionScriptFactory()
	} else {
		scriptFactory, err = workerexecutor.NewScriptFactory(edges.ScriptCommandRunner)
	}
	if err != nil {
		return Components{}, fmt.Errorf("construct worker application: %w", err)
	}
	return Components{
		Provider:                providerFactory,
		Script:                  scriptFactory,
		ProviderCommandInjected: edges.ProviderCommandRunner != nil,
		Hosted: hostedworkers.NewConfig(hostedworkers.Config{
			Logger: logger, Clock: edges.HostedClock, HTTPClient: edges.HostedHTTPClient,
			SecretResolver: edges.HostedSecretResolver, LinearEndpoint: edges.HostedLinearEndpoint,
		}),
	}, nil
}

// WithCommandRunners adds per-runtime mock/replay wrappers without changing
// the process-selected PTY or hosted edges.
func (c Components) WithCommandRunners(providerRunner, scriptRunner workers.CommandRunner) (Components, error) {
	if c.Provider == nil || c.Script == nil {
		return Components{}, fmt.Errorf("construct worker application runtime: base components are required")
	}
	providerFactory, err := c.Provider.WithCommandRunner(providerRunner)
	if err != nil {
		return Components{}, err
	}
	scriptFactory, err := c.Script.WithCommandRunner(scriptRunner)
	if err != nil {
		return Components{}, err
	}
	c.Provider = providerFactory
	c.Script = scriptFactory
	c.ProviderCommandInjected = c.ProviderCommandInjected || providerRunner != nil
	return c, nil
}

// Valid reports whether the reusable worker factories were constructed.
func (c Components) Valid() bool { return c.Provider != nil && c.Script != nil }
