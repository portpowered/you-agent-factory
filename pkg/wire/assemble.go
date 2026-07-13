package wire

import (
	"errors"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
)

func validateModelWorkers(services ModelWorkerServices) error {
	switch {
	case services.Models == nil:
		return errors.New("models is required")
	case services.Workers == nil:
		return errors.New("workers is required")
	case services.WorkerProvider == nil:
		return errors.New("worker/provider runtime builder is required")
	default:
		return nil
	}
}

func validateFactorySessions(services FactorySessionServices) error {
	switch {
	case services.FactoryDefinition == nil:
		return errors.New("factory definition is required")
	case services.FactorySessions == nil:
		return errors.New("Factory Session service is required")
	case isNil(services.DurableExecution):
		return errors.New("durable execution service is required")
	default:
		return nil
	}
}

func validateTransports(transports TransportLifecycles) error {
	switch {
	case isNil(transports.API):
		return errors.New("API transport lifecycle is required")
	case isNil(transports.CLI):
		return errors.New("CLI transport lifecycle is required")
	case isNil(transports.MCP):
		return errors.New("MCP transport lifecycle is required")
	default:
		return nil
	}
}

func validateSidecars(sidecars SidecarLifecycles) error {
	switch {
	case isNil(sidecars.Runtime):
		return errors.New("runtime sidecar lifecycle is required")
	case isNil(sidecars.Workers):
		return errors.New("workers sidecar lifecycle is required")
	case isNil(sidecars.Dashboard):
		return errors.New("dashboard sidecar lifecycle is required")
	default:
		return nil
	}
}

func newTransportDependencies(models ModelWorkerServices, sessions FactorySessionServices) TransportDependencies {
	return TransportDependencies{
		Models:            models.Models,
		FactoryDefinition: sessions.FactoryDefinition,
		FactorySessions:   sessions.FactorySessions,
		DurableExecution:  sessions.DurableExecution,
	}
}

func newSidecarDependencies(
	config *factoryconfig.LoadedFactoryConfig,
	runtime RuntimeDependencies,
	models ModelWorkerServices,
	sessions FactorySessionServices,
) SidecarDependencies {
	return SidecarDependencies{
		Config:           config,
		Runtime:          runtime,
		Models:           models.Models,
		Workers:          models.Workers,
		WorkerProvider:   models.WorkerProvider,
		FactorySessions:  sessions.FactorySessions,
		DurableExecution: sessions.DurableExecution,
	}
}

func newGraph(
	config *factoryconfig.LoadedFactoryConfig,
	runtime RuntimeDependencies,
	models ModelWorkerServices,
	sessions FactorySessionServices,
	transportDeps TransportDependencies,
	transports TransportLifecycles,
	sidecars SidecarLifecycles,
	resources *resourceSet,
) *Graph {
	return &Graph{
		Config:            config,
		Runtime:           runtime,
		Models:            models.Models,
		Workers:           models.Workers,
		WorkerProvider:    models.WorkerProvider,
		FactoryDefinition: sessions.FactoryDefinition,
		FactorySessions:   sessions.FactorySessions,
		DurableExecution:  sessions.DurableExecution,
		Transport:         transportDeps,
		Transports:        transports,
		Sidecars:          sidecars,
		resources:         resources,
	}
}
