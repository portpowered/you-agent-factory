package replay

import (
	factoryapi "github.com/portpowered/infinite-you/pkg/api/generated"
	"github.com/portpowered/infinite-you/pkg/config"
	"github.com/portpowered/infinite-you/pkg/interfaces"
)

func generatedFactoryAPIFromConfig(cfg *interfaces.FactoryConfig) factoryapi.Factory {
	return config.FactoryConfigToOpenAPI(cfg)
}

func generatedWorkstationAPIFromConfig(name string, cfg interfaces.FactoryWorkstationConfig) factoryapi.Workstation {
	workstation := config.WorkstationConfigToOpenAPI(cfg)
	if workstation.Name == "" {
		workstation.Name = name
	}
	return workstation
}

func generatedWorkerAPIFromConfig(name string, cfg interfaces.WorkerConfig) factoryapi.Worker {
	worker := config.WorkerConfigToOpenAPI(cfg)
	if worker.Name == "" {
		worker.Name = name
	}
	return worker
}

func factoryConfigFromGeneratedAPI(generated factoryapi.Factory) (*interfaces.FactoryConfig, error) {
	cfg, err := config.FactoryConfigFromOpenAPI(generated)
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func workstationConfigFromGeneratedAPI(workstation factoryapi.Workstation) (interfaces.FactoryWorkstationConfig, error) {
	return config.WorkstationConfigFromOpenAPI(workstation)
}
