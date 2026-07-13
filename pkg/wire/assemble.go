package wire

import (
	"errors"

	factoryconfig "github.com/portpowered/infinite-you/pkg/config"
	factorydefinition "github.com/portpowered/infinite-you/pkg/factorydefinition/service"
	"github.com/portpowered/infinite-you/pkg/factorysessionexecution"
	factorysessionsservice "github.com/portpowered/infinite-you/pkg/factorysessions/service"
	modelservice "github.com/portpowered/infinite-you/pkg/models/service"
)

func validateModelWorkers(services phasedModelWorkerServices) error {
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

func validateFactorySessions(services phasedFactorySessionServices) error {
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

type phasedTransportDependencies struct {
	Models            *modelservice.Service
	FactoryDefinition *factorydefinition.Service
	FactorySessions   *factorysessionsservice.Service
	DurableExecution  factorysessionexecution.Service
}

func newTransportDependencies(models phasedModelWorkerServices, sessions phasedFactorySessionServices) phasedTransportDependencies {
	return phasedTransportDependencies{
		Models:            models.Models,
		FactoryDefinition: sessions.FactoryDefinition,
		FactorySessions:   sessions.FactorySessions,
		DurableExecution:  sessions.DurableExecution,
	}
}

func newSidecarDependencies(
	config *factoryconfig.LoadedFactoryConfig,
	runtime phasedRuntimeDependencies,
	models phasedModelWorkerServices,
	sessions phasedFactorySessionServices,
) phasedSidecarDependencies {
	return phasedSidecarDependencies{
		Config:           config,
		Runtime:          runtime,
		Models:           models.Models,
		Workers:          models.Workers,
		WorkerProvider:   models.WorkerProvider,
		FactorySessions:  sessions.FactorySessions,
		DurableExecution: sessions.DurableExecution,
	}
}

func newPhasedGraph(
	config *factoryconfig.LoadedFactoryConfig,
	runtime phasedRuntimeDependencies,
	models phasedModelWorkerServices,
	sessions phasedFactorySessionServices,
	transportDeps phasedTransportDependencies,
	transports TransportLifecycles,
	sidecars SidecarLifecycles,
	resources *resourceSet,
) *phasedGraph {
	return &phasedGraph{
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
